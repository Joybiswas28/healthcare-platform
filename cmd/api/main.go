package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"healthcare-platform/internal/appointment"
	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/consultation"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/family"
	"healthcare-platform/internal/hospital"
	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/payment/sslcommerz"
	"healthcare-platform/internal/pharmacy"
	"healthcare-platform/internal/platform/config"
	"healthcare-platform/internal/platform/database"
	"healthcare-platform/internal/platform/storage"
	"healthcare-platform/internal/prescription"
	"healthcare-platform/internal/realtime"
	"healthcare-platform/internal/records"
	"healthcare-platform/internal/server"
	"healthcare-platform/internal/user"
	"healthcare-platform/internal/wallet"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load() // .env is optional; real env vars take precedence

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var handler slog.Handler = slog.NewTextHandler(os.Stdout, nil)
	if cfg.IsProduction() {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	}
	log := slog.New(handler)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := database.Migrate(pool); err != nil {
		return err
	}
	log.Info("database migrated")

	files, err := storage.NewLocal(cfg.UploadDir)
	if err != nil {
		return err
	}

	// SMS: no provider account yet. In development messages (and OTP codes) are
	// logged; in production their text is never logged.
	var sms notify.Sender = notify.LogSender{Log: log, Redact: cfg.IsProduction()}
	if cfg.IsProduction() {
		log.Warn("no SMS provider configured: SMS are NOT delivered, so nobody can receive OTP codes")
	}

	users := user.NewRepository(pool)
	sessions := auth.NewSessionChecker(users)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)
	otp := auth.NewOTPStore(pool, sms, users, cfg.JWTSecret)
	authSvc := auth.NewService(users, tokens, otp)
	walletStore := wallet.NewStore(pool)

	// Payments: SSLCommerz when configured, otherwise the development simulator.
	var gateway wallet.Gateway = wallet.DevGateway{}
	var sslCallbacks *sslcommerz.Callbacks
	if cfg.SSLCommerzStoreID != "" {
		client := sslcommerz.New(sslcommerz.Config{
			StoreID:       cfg.SSLCommerzStoreID,
			StorePassword: cfg.SSLCommerzStorePassword,
			Sandbox:       cfg.SSLCommerzSandbox,
			PublicBaseURL: cfg.PublicBaseURL,
		})
		gateway = client
		sslCallbacks = sslcommerz.NewCallbacks(client, walletStore, cfg.FrontendURL, log)
		log.Info("payments: SSLCommerz", "sandbox", cfg.SSLCommerzSandbox)
	} else {
		log.Info("payments: development simulator (set SSLCOMMERZ_STORE_ID to use SSLCommerz)")
	}

	doctorStore := doctor.NewStore(pool)
	appointmentStore := appointment.NewStore(pool)

	go expireTokensPeriodically(ctx, walletStore, log)
	go notify.NewWorker(pool, sms, log).Run(ctx)
	go appointmentStore.RunReminders(ctx, log)

	hub := realtime.NewHub()
	go realtime.Listen(ctx, pool, hub, log)

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: server.NewRouter(server.Deps{
			Log:          log,
			DB:           pool,
			Tokens:       tokens,
			Auth:         auth.NewHandler(authSvc, otp, log).WithSessions(sessions),
			Doctors:      doctor.NewHandler(doctorStore, log),
			Family:       family.NewHandler(family.NewStore(pool), log),
			Records:      records.NewHandler(records.NewStore(pool, files), log),
			Appointments: appointment.NewHandler(appointmentStore, log),
			Hospitals: hospital.NewHandler(hospital.NewStore(pool, doctorStore, files),
				doctorStore, appointmentStore, log),
			Pharmacies:    pharmacy.NewHandler(pharmacy.NewStore(pool, files, cfg.PharmacyCommissionBps), log),
			Wallet:        wallet.NewHandler(walletStore, gateway, log),
			Consultations: consultation.NewHandler(consultation.NewStore(pool, files), hub, log),
			Prescriptions: prescription.NewHandler(prescription.NewStore(pool), cfg.FrontendURL, log),
			Notifications: notify.NewHandler(notify.NewStore(pool), log),
			SSLCommerz:    sslCallbacks,
			DevRoutes:     !cfg.IsProduction() && gateway.Name() == wallet.ProviderDev,
			Sessions:      sessions,
			AdminUsers:    auth.NewAdminHandler(users, sessions, log),
			TrustProxy:    cfg.TrustProxy,
			CORSOrigins:   cfg.CORSOrigins,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		// Generous enough for a 10 MB upload on a slow mobile connection; handlers
		// still limit body sizes and the router times out ordinary requests at 30 s.
		ReadTimeout:  2 * time.Minute,
		WriteTimeout: 2 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// expireTokensPeriodically books expired consultation tokens as platform revenue,
// once at startup and then every hour.
func expireTokensPeriodically(ctx context.Context, ws *wallet.Store, log *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if n, err := ws.ExpireTokens(ctx); err != nil {
			if ctx.Err() == nil {
				log.Error("token expiry failed", "err", err)
			}
		} else if n > 0 {
			log.Info("expired tokens", "tokens", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
