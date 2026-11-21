// Package server wires HTTP routes and middleware.
package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/api"
	"healthcare-platform/internal/appointment"
	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/consultation"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/family"
	"healthcare-platform/internal/hospital"
	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/payment/sslcommerz"
	"healthcare-platform/internal/pharmacy"
	"healthcare-platform/internal/platform/httpx"
	"healthcare-platform/internal/prescription"
	"healthcare-platform/internal/records"
	"healthcare-platform/internal/user"
	"healthcare-platform/internal/wallet"
)

type Deps struct {
	Log           *slog.Logger
	DB            *pgxpool.Pool
	Tokens        *auth.TokenManager
	Auth          *auth.Handler
	Doctors       *doctor.Handler
	Family        *family.Handler
	Records       *records.Handler
	Appointments  *appointment.Handler
	Wallet        *wallet.Handler
	Consultations *consultation.Handler
	Prescriptions *prescription.Handler
	Hospitals     *hospital.Handler
	Pharmacies    *pharmacy.Handler
	Notifications *notify.Handler
	// SSLCommerz is nil when the gateway is not configured.
	SSLCommerz *sslcommerz.Callbacks
	// DevRoutes enables endpoints that simulate external services (payment gateway).
	DevRoutes bool

	Sessions   *auth.SessionChecker
	AdminUsers *auth.AdminHandler

	// TrustProxy: take the client IP from X-Forwarded-For / X-Real-IP. Enable only
	// behind a reverse proxy that sets these headers itself; otherwise clients
	// could fake their IP and dodge rate limits.
	TrustProxy bool
	// CORSOrigins are the browser origins (e.g. https://app.example.com) allowed to
	// call the API. Empty means no cross-origin browser access.
	CORSOrigins []string
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if d.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(securityHeaders)
	if len(d.CORSOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins: d.CORSOrigins,
			AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
			AllowedHeaders: []string{"Authorization", "Content-Type"},
			MaxAge:         600,
		}))
	}
	r.Use(requestLogger(d.Log))
	r.Use(middleware.Recoverer)
	r.Use(rateLimit(600, time.Minute)) // generous per-IP ceiling for everything
	r.Use(exceptStreaming(middleware.Timeout(30 * time.Second)))

	// Tight per-IP limits where guessing is the attack: passwords, OTPs, prescription codes.
	strict := rateLimit(20, time.Minute)

	r.Get("/openapi.yaml", api.SpecHandler)
	r.Get("/docs", api.DocsHandler)

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
		r.With(strict).Post("/auth/otp", d.Auth.RequestOTP)
		r.With(strict).Post("/auth/register", d.Auth.Register)
		r.With(strict).Post("/auth/login", d.Auth.Login)
		r.With(strict).Post("/auth/password/reset", d.Auth.ResetPassword)
		r.With(strict).Get("/prescriptions/verify/{code}", d.Prescriptions.Verify)
		r.Get("/hospitals", d.Hospitals.Search)
		r.Get("/hospitals/{hospitalID}", d.Hospitals.Public)
		r.Get("/hospitals/{hospitalID}/photos/{photoID}", d.Hospitals.Photo)
		r.Get("/pharmacies", d.Pharmacies.Nearby)
		r.Get("/pharmacies/{pharmacyID}", d.Pharmacies.Public)
		if d.SSLCommerz != nil {
			r.Post("/payments/sslcommerz/ipn", d.SSLCommerz.IPN)
			r.Post("/payments/sslcommerz/success", d.SSLCommerz.Success)
			r.Post("/payments/sslcommerz/fail", d.SSLCommerz.Fail)
			r.Post("/payments/sslcommerz/cancel", d.SSLCommerz.Cancel)
		}
		r.Get("/doctors", d.Doctors.Search)
		r.Get("/doctors/{doctorID}", d.Doctors.Get)
		r.Get("/schedules/{scheduleID}/availability", d.Appointments.Availability)
		r.Get("/token-packages", d.Wallet.Packages)

		// Authenticated
		r.Group(func(r chi.Router) {
			r.Use(auth.Authenticate(d.Tokens, d.Sessions))
			r.Get("/me", d.Auth.Me)
			r.Put("/me", d.Auth.UpdateProfile)
			// Guessing the current password is the attack here, so it gets the tight limit.
			r.With(strict).Post("/me/password", d.Auth.ChangePassword)

			r.Get("/notifications", d.Notifications.List)
			r.Post("/notifications/read-all", d.Notifications.MarkAllRead)
			r.Post("/notifications/{notificationID}/read", d.Notifications.MarkRead)

			// Prescriptions (patients read theirs; doctors issue and revoke)
			r.Group(func(r chi.Router) {
				rx := d.Prescriptions
				r.Use(auth.RequireRole(user.RolePatient, user.RoleDoctor))
				r.Get("/prescriptions", rx.List)
				r.Get("/prescriptions/{prescriptionID}", rx.Get)
				r.Get("/prescriptions/{prescriptionID}/print", rx.Print)
				r.With(auth.RequireRole(user.RoleDoctor)).Post("/prescriptions", rx.Issue)
				r.With(auth.RequireRole(user.RoleDoctor)).Post("/prescriptions/{prescriptionID}/revoke", rx.Revoke)
			})

			// Patient
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireRole(user.RolePatient))
				r.Post("/appointments", d.Appointments.Book)
				r.Get("/appointments", d.Appointments.Mine)
				r.Post("/appointments/{appointmentID}/cancel", d.Appointments.Cancel)
				r.Post("/appointments/{appointmentID}/review", d.Doctors.ReviewAppointment)
				r.Post("/consultations/{consultationID}/review", d.Doctors.ReviewConsultation)
				r.Get("/reviews/mine", d.Doctors.MyReviews)

				r.Get("/family", d.Family.List)
				r.Post("/family", d.Family.Add)
				r.Put("/family/{memberID}", d.Family.Update)
				r.Delete("/family/{memberID}", d.Family.Remove)

				r.Get("/records", d.Records.List)
				r.Post("/records", d.Records.Add)
				r.Put("/records/{recordID}", d.Records.Update)
				r.Delete("/records/{recordID}", d.Records.Delete)
				r.Post("/records/{recordID}/share", d.Records.Share)
				r.Delete("/records/{recordID}/shares/{appointmentID}", d.Records.Unshare)

				r.Get("/wallet", d.Wallet.Wallet)
				r.Get("/wallet/history", d.Wallet.History)
				r.Post("/wallet/purchases", d.Wallet.StartPurchase)
				r.Get("/wallet/purchases/{paymentID}", d.Wallet.GetPurchase)
				if d.DevRoutes {
					r.Post("/dev/payments/{paymentID}/succeed", d.Wallet.DevSucceed)
				}

				ph := d.Pharmacies
				r.Post("/medicine-requests", ph.CreateRequest)
				r.Get("/medicine-requests", ph.MyRequests)
				r.Get("/medicine-requests/{requestID}", ph.MyRequest)
				r.Post("/medicine-requests/{requestID}/cancel", ph.CancelRequest)
				r.Get("/medicine-requests/{requestID}/attachment", ph.MyAttachment)
				r.Post("/quotes/{quoteID}/accept", ph.AcceptQuote)
				r.Get("/medicine-orders", ph.MyOrders)
				r.Get("/medicine-orders/{orderID}", ph.MyOrder)
				r.Post("/medicine-orders/{orderID}/cancel", ph.CancelMyOrder)
				r.Post("/medicine-orders/{orderID}/review", ph.ReviewOrder)
			})

			// Pharmacies (owner checked per pharmacy)
			r.Group(func(r chi.Router) {
				ph := d.Pharmacies
				r.Use(auth.RequireRole(user.RolePharmacy))
				r.Post("/pharmacies", ph.Create)
				r.Get("/manage/pharmacies", ph.Mine)
				r.Route("/manage/pharmacies/{pharmacyID}", func(r chi.Router) {
					r.Use(ph.Owner)
					r.Get("/", ph.Get)
					r.Put("/", ph.Update)
					r.Get("/requests", ph.Feed)
					r.Get("/requests/{requestID}", ph.Request)
					r.Get("/requests/{requestID}/attachment", ph.RequestAttachment)
					r.Post("/requests/{requestID}/quote", ph.Quote)
					r.Post("/quotes/{quoteID}/withdraw", ph.WithdrawQuote)
					r.Get("/orders", ph.Orders)
					r.Get("/orders/{orderID}", ph.Order)
					r.Post("/orders/{orderID}/preparing", ph.Advance(pharmacy.OrderPreparing))
					r.Post("/orders/{orderID}/ready", ph.Advance(pharmacy.OrderReady))
					r.Post("/orders/{orderID}/dispatch", ph.Dispatch)
					r.Post("/orders/{orderID}/complete", ph.Complete)
					r.Post("/orders/{orderID}/cancel", ph.Cancel)
					r.Get("/statement", ph.Statement)
				})
			})

			// Riders
			r.Route("/rider", func(r chi.Router) {
				ph := d.Pharmacies
				r.Use(auth.RequireRole(user.RoleRider))
				r.Get("/profile", ph.RiderProfile)
				r.Put("/profile", ph.RegisterRider)
				r.Put("/availability", ph.Availability)
				r.Put("/location", ph.Location)
				r.Get("/jobs", ph.Jobs)
				r.Post("/jobs/{orderID}/accept", ph.AcceptJob)
				r.Get("/orders", ph.RiderOrders)
				r.Get("/orders/{orderID}", ph.RiderOrder)
				r.Post("/orders/{orderID}/release", ph.ReleaseJob)
				r.Post("/orders/{orderID}/picked-up", ph.PickedUp)
				r.Post("/orders/{orderID}/delivered", ph.Delivered)
				r.Get("/earnings", ph.RiderEarnings)
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
				r.Get("/consultations/{consultationID}/events", c.Events)
				r.Get("/consultations/{consultationID}/call", c.CallConfig)
				r.Post("/consultations/{consultationID}/call/signals", c.PostSignal)
				r.Get("/consultations/{consultationID}/call/events", c.CallEvents)
				r.Get("/attachments/{attachmentID}", c.Download)
				r.Get("/records/{recordID}/file", d.Records.File)

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
				r.Get("/reviews", d.Doctors.DoctorReviews)
				r.Put("/reviews/{reviewID}/reply", d.Doctors.ReplyToReview)
				r.Get("/hospitals", d.Hospitals.Invitations)
				r.Post("/hospitals/{hospitalID}/accept", d.Hospitals.AcceptInvitation())
				r.Post("/hospitals/{hospitalID}/decline", d.Hospitals.DeclineInvitation())
				r.Post("/hospitals/{hospitalID}/leave", d.Hospitals.Leave)
				r.Get("/chambers", d.Doctors.MyChambers)
				r.Post("/chambers", d.Doctors.CreateChamber)
				r.Put("/chambers/{chamberID}", d.Doctors.UpdateChamber)
				r.Post("/chambers/{chamberID}/schedules", d.Doctors.AddSchedule)
				r.Delete("/schedules/{scheduleID}", d.Doctors.RemoveSchedule)
				r.Get("/appointments", d.Appointments.DoctorDay)
				r.Post("/appointments/{appointmentID}/complete", d.Appointments.Complete)
				r.Post("/appointments/{appointmentID}/no-show", d.Appointments.NoShow)
				r.Get("/tickets/{code}", d.Appointments.Ticket)
				r.Get("/appointments/{appointmentID}/records", d.Records.ForAppointment)
			})

			// Hospital management (hospital_admin accounts; membership checked per hospital)
			r.Group(func(r chi.Router) {
				hs := d.Hospitals
				r.Use(auth.RequireRole(user.RoleHospitalAdmin))
				r.Post("/hospitals", hs.Create)
				r.Get("/manage/hospitals", hs.Mine)
				r.Route("/manage/hospitals/{hospitalID}", func(r chi.Router) {
					r.Use(hs.Member)
					r.Get("/", hs.Get)
					r.Put("/", hs.Update)
					r.Get("/photos", hs.ListPhotos)
					r.Post("/photos", hs.UploadPhoto)
					r.Delete("/photos/{photoID}", hs.DeletePhoto)
					r.Post("/photos/{photoID}/logo", hs.SetLogo)

					r.Get("/doctors", hs.Staff)
					r.Post("/doctors", hs.Invite)
					r.Put("/doctors/{doctorID}", hs.UpdateStaff)
					r.Delete("/doctors/{doctorID}", hs.RemoveStaff)
					r.Post("/doctors/{doctorID}/chambers", hs.CreateChamber)

					r.Get("/chambers", hs.Chambers)
					r.Put("/chambers/{chamberID}", hs.UpdateChamber)
					r.Post("/chambers/{chamberID}/schedules", hs.AddSchedule)
					r.Delete("/schedules/{scheduleID}", hs.RemoveSchedule)

					r.Get("/appointments", hs.Appointments)
					r.Post("/appointments/{appointmentID}/cancel", hs.CancelAppointment)
					r.Post("/appointments/{appointmentID}/reschedule", hs.RescheduleAppointment)

					r.Get("/members", hs.Members)
					r.With(hs.OwnerOnly).Post("/members", hs.AddMember)
					r.With(hs.OwnerOnly).Delete("/members/{userID}", hs.RemoveMember)
				})
			})

			// Admin
			r.Route("/admin", func(r chi.Router) {
				r.Use(auth.RequireRole(user.RoleAdmin))
				r.Get("/doctors", d.Doctors.AdminList)
				r.Post("/doctors/{doctorID}/verification", d.Doctors.AdminVerify)
				r.Put("/doctors/{doctorID}/commission", d.Doctors.AdminSetCommission)
				r.Get("/hospitals", d.Hospitals.AdminList)
				r.Post("/hospitals/{hospitalID}/verification", d.Hospitals.AdminVerify)
				r.Get("/pharmacies", d.Pharmacies.AdminPharmacies)
				r.Post("/pharmacies/{pharmacyID}/verification", d.Pharmacies.AdminVerifyPharmacy)
				r.Get("/riders", d.Pharmacies.AdminRiders)
				r.Get("/users", d.AdminUsers.Search)
				r.Post("/users/{userID}/status", d.AdminUsers.SetStatus)
				r.Post("/riders/{userID}/verification", d.Pharmacies.AdminVerifyRider)
				r.Get("/withdrawals", d.Wallet.AdminWithdrawals)
				r.Post("/withdrawals/{withdrawalID}/process", d.Wallet.ProcessWithdrawal)
			})
		})
	})

	return r
}

// securityHeaders sets defensive headers on every response. The API serves JSON,
// so pages from it may not be framed and content types are never sniffed.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimit allows n requests per window per client IP (r.RemoteAddr, which RealIP
// rewrites only when the proxy is trusted).
func rateLimit(n int, window time.Duration) func(http.Handler) http.Handler {
	return httprate.Limit(n, window,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				return r.RemoteAddr, nil
			}
			return host, nil
		}),
		httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
			httpx.Error(w, http.StatusTooManyRequests, "too many requests; please slow down")
		}),
	)
}

// exceptStreaming applies mw to every request except long-lived ones: WebSocket
// upgrades and Server-Sent Event streams.
func exceptStreaming(mw func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
				strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
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
