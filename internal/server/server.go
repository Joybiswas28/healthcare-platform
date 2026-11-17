// Package server wires HTTP routes and middleware.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/appointment"
	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/consultation"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/platform/httpx"
	"healthcare-platform/internal/user"
	"healthcare-platform/internal/wallet"
)

type Deps struct {
	Log           *slog.Logger
	DB            *pgxpool.Pool
	Tokens        *auth.TokenManager
	Auth          *auth.Handler
	Doctors       *doctor.Handler
	Appointments  *appointment.Handler
	Wallet        *wallet.Handler
	Consultations *consultation.Handler
	// DevRoutes enables endpoints that simulate external services (payment gateway).
	DevRoutes bool
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Log))
	r.Use(middleware.Recoverer)
	r.Use(exceptWebSocket(middleware.Timeout(30 * time.Second)))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := d.DB.Ping(ctx); err != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		// Public
		r.Post("/auth/register", d.Auth.Register)
		r.Post("/auth/login", d.Auth.Login)
		r.Get("/doctors", d.Doctors.Search)
		r.Get("/doctors/{doctorID}", d.Doctors.Get)
		r.Get("/schedules/{scheduleID}/availability", d.Appointments.Availability)
		r.Get("/token-packages", d.Wallet.Packages)

		// Authenticated
		r.Group(func(r chi.Router) {
			r.Use(auth.Authenticate(d.Tokens))
			r.Get("/me", d.Auth.Me)

			// Patient
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireRole(user.RolePatient))
				r.Post("/appointments", d.Appointments.Book)
				r.Get("/appointments", d.Appointments.Mine)
				r.Post("/appointments/{appointmentID}/cancel", d.Appointments.Cancel)

				r.Get("/wallet", d.Wallet.Wallet)
				r.Get("/wallet/history", d.Wallet.History)
				r.Post("/wallet/purchases", d.Wallet.StartPurchase)
				r.Get("/wallet/purchases/{paymentID}", d.Wallet.GetPurchase)
				if d.DevRoutes {
					r.Post("/dev/payments/{paymentID}/succeed", d.Wallet.DevSucceed)
				}
			})

			// Consultations (patient and doctor; the store checks who may do what)
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireRole(user.RolePatient, user.RoleDoctor))
				c := d.Consultations
				r.With(auth.RequireRole(user.RolePatient)).Post("/consultations", c.Request)
				r.Get("/consultations", c.List)
				r.Get("/consultations/{consultationID}", c.Get())
				r.With(auth.RequireRole(user.RoleDoctor)).Post("/consultations/{consultationID}/accept", c.Accept())
				r.With(auth.RequireRole(user.RoleDoctor)).Post("/consultations/{consultationID}/reject", c.Reject)
				r.With(auth.RequireRole(user.RolePatient)).Post("/consultations/{consultationID}/confirm", c.Confirm())
				r.With(auth.RequireRole(user.RoleDoctor)).Post("/consultations/{consultationID}/complete", c.Complete())
				r.Post("/consultations/{consultationID}/cancel", c.Cancel())
				r.Get("/consultations/{consultationID}/messages", c.Messages)
				r.Post("/consultations/{consultationID}/messages", c.PostMessage)
				r.Post("/consultations/{consultationID}/attachments", c.Upload)
				r.Get("/consultations/{consultationID}/ws", c.Stream)
				r.Get("/attachments/{attachmentID}", c.Download)

				// Public medical concerns
				r.Get("/concerns", c.Concerns)
				r.Get("/concerns/{concernID}", c.Concern)
				r.With(auth.RequireRole(user.RolePatient)).Post("/concerns", c.PostConcern)
				r.With(auth.RequireRole(user.RolePatient)).Post("/concerns/{concernID}/close", c.CloseConcern)
				r.With(auth.RequireRole(user.RoleDoctor)).Post("/concerns/{concernID}/offers", c.MakeOffer)
				r.With(auth.RequireRole(user.RoleDoctor)).Post("/offers/{offerID}/withdraw", c.WithdrawOffer)
				r.With(auth.RequireRole(user.RolePatient)).Post("/offers/{offerID}/accept", c.AcceptOffer)
			})

			// Doctor
			r.Route("/doctor", func(r chi.Router) {
				r.Use(auth.RequireRole(user.RoleDoctor))
				r.Get("/profile", d.Doctors.MyProfile)
				r.Put("/profile", d.Doctors.UpdateMyProfile)
				r.Put("/consultation-settings", d.Doctors.UpdateConsultationSettings)
				r.Get("/earnings", d.Wallet.Earnings)
				r.Post("/withdrawals", d.Wallet.RequestWithdrawal)
				r.Get("/withdrawals", d.Wallet.MyWithdrawals)
				r.Get("/offers", d.Consultations.MyOffers)
				r.Get("/chambers", d.Doctors.MyChambers)
				r.Post("/chambers", d.Doctors.CreateChamber)
				r.Put("/chambers/{chamberID}", d.Doctors.UpdateChamber)
				r.Post("/chambers/{chamberID}/schedules", d.Doctors.AddSchedule)
				r.Delete("/schedules/{scheduleID}", d.Doctors.RemoveSchedule)
				r.Get("/appointments", d.Appointments.DoctorDay)
				r.Post("/appointments/{appointmentID}/complete", d.Appointments.Complete)
				r.Post("/appointments/{appointmentID}/no-show", d.Appointments.NoShow)
				r.Get("/tickets/{code}", d.Appointments.Ticket)
			})

			// Admin
			r.Route("/admin", func(r chi.Router) {
				r.Use(auth.RequireRole(user.RoleAdmin))
				r.Get("/doctors", d.Doctors.AdminList)
				r.Post("/doctors/{doctorID}/verification", d.Doctors.AdminVerify)
				r.Put("/doctors/{doctorID}/commission", d.Doctors.AdminSetCommission)
				r.Get("/withdrawals", d.Wallet.AdminWithdrawals)
				r.Post("/withdrawals/{withdrawalID}/process", d.Wallet.ProcessWithdrawal)
			})
		})
	})

	return r
}

// exceptWebSocket applies mw to every request except WebSocket upgrades, which are long-lived.
func exceptWebSocket(mw func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				next.ServeHTTP(w, r)
				return
			}
			wrapped.ServeHTTP(w, r)
		})
	}
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.InfoContext(r.Context(), "http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
