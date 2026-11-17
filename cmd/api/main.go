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
	"healthcare-platform/internal/platform/config"
	"healthcare-platform/internal/platform/database"
	"healthcare-platform/internal/platform/storage"
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

	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)
	authSvc := auth.NewService(user.NewRepository(pool), tokens)
	walletStore := wallet.NewStore(pool)

	go expireTokensPeriodically(ctx, walletStore, log)

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: server.NewRouter(server.Deps{
			Log:           log,
			DB:            pool,
			Tokens:        tokens,
			Auth:          auth.NewHandler(authSvc, log),
			Doctors:       doctor.NewHandler(doctor.NewStore(pool), log),
			Appointments:  appointment.NewHandler(appointment.NewStore(pool), log),
			Wallet:        wallet.NewHandler(walletStore, log),
			Consultations: consultation.NewHandler(consultation.NewStore(pool, files), log),
			DevRoutes:     !cfg.IsProduction(),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
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
