// Package server wires HTTP routes and middleware.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/appointment"
	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/platform/httpx"
	"healthcare-platform/internal/user"
)

type Deps struct {
	Log          *slog.Logger
	DB           *pgxpool.Pool
	Tokens       *auth.TokenManager
	Auth         *auth.Handler
	Doctors      *doctor.Handler
	Appointments *appointment.Handler
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

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
			})

			// Doctor
			r.Route("/doctor", func(r chi.Router) {
				r.Use(auth.RequireRole(user.RoleDoctor))
				r.Get("/profile", d.Doctors.MyProfile)
				r.Put("/profile", d.Doctors.UpdateMyProfile)
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
			})
		})
	})

	return r
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
