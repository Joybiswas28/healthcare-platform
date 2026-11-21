// Package notify stores in-app notifications and delivers them by SMS.
//
// Callers add a notification inside the same database transaction as the event it
// describes (Enqueue). A background worker (Worker.Run) later sends the ones marked
// for SMS, retrying failures, so a slow or broken SMS provider never blocks or
// rolls back the business operation.
package notify

import (
	"context"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

// Notification kinds.
const (
	KindAppointmentBooked      = "appointment_booked"
	KindAppointmentCancelled   = "appointment_cancelled"
	KindAppointmentRescheduled = "appointment_rescheduled"
	KindAppointmentReminder    = "appointment_reminder"
	KindHospitalVerification   = "hospital_verification"
	KindHospitalInvitation     = "hospital_invitation"
	KindHospitalStaffChange    = "hospital_staff_change"
	KindPharmacyVerification   = "pharmacy_verification"
	KindRiderVerification      = "rider_verification"
	KindMedicineRequest        = "medicine_request"
	KindMedicineQuote          = "medicine_quote"
	KindMedicineOrder          = "medicine_order"
	KindConsultationRequested  = "consultation_requested"
	KindConsultationAccepted   = "consultation_accepted"
	KindConsultationRejected   = "consultation_rejected"
	KindConsultationCompleted  = "consultation_completed"
	KindConsultationCancelled  = "consultation_cancelled"
	KindOfferReceived          = "offer_received"
	KindOfferAccepted          = "offer_accepted"
	KindPrescriptionIssued     = "prescription_issued"
	KindDoctorVerification     = "doctor_verification"
	KindDoctorReview           = "doctor_review"
	KindWithdrawalProcessed    = "withdrawal_processed"
	KindTokenPurchaseCompleted = "token_purchase_completed"
)

type Notification struct {
	ID        int64      `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// Msg describes one notification to a user.
type Msg struct {
	UserID string
	Kind   string
	Title  string
	Body   string
	SMS    bool // also send Body by SMS
}

// Enqueue records a notification using q, which should be the caller's transaction.
func Enqueue(ctx context.Context, q database.Querier, m Msg) error {
	var smsStatus *string
	if m.SMS {
		s := "pending"
		smsStatus = &s
	}
	_, err := q.Exec(ctx, `
		INSERT INTO notifications (user_id, kind, title, body, sms_status) VALUES ($1, $2, $3, $4, $5)`,
		m.UserID, m.Kind, m.Title, truncate(m.Body, 480), smsStatus)
	return err
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// ---------------------------------------------------------------- in-app inbox

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) List(ctx context.Context, userID string, unreadOnly bool, limit, offset int) ([]Notification, int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, kind, title, body, read_at, created_at FROM notifications
		WHERE user_id = $1 AND (NOT $2 OR read_at IS NULL)
		ORDER BY id DESC LIMIT $3 OFFSET $4`, userID, unreadOnly, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	ns, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Notification])
	if err != nil {
		return nil, 0, err
	}
	var unread int
	err = s.db.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&unread)
	return ns, unread, err
}

func (s *Store) MarkRead(ctx context.Context, userID string, id int64) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("notification not found")
	}
	return nil
}

func (s *Store) MarkAllRead(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID)
	return err
}

// ---------------------------------------------------------------- SMS

// Sender delivers a text message to a Bangladeshi mobile number (01XXXXXXXXX).
type Sender interface {
	Send(ctx context.Context, phone, text string) error
}

// LogSender writes messages to the log instead of sending them. Used until an SMS
// provider account is configured. With Redact set (always in production) the text is
// left out, because it can contain one-time passwords that must never reach logs.
type LogSender struct {
	Log    *slog.Logger
	Redact bool
}

func (l LogSender) Send(ctx context.Context, phone, text string) error {
	if l.Redact {
		l.Log.WarnContext(ctx, "SMS NOT SENT: no SMS provider configured", "to", phone, "length", len(text))
		return nil
	}
	l.Log.InfoContext(ctx, "SMS (not sent: development mode)", "to", phone, "text", text)
	return nil
}

// MaxSMSAttempts is how many times the worker tries to send one SMS.
const MaxSMSAttempts = 5

type Worker struct {
	db       *pgxpool.Pool
	sender   Sender
	log      *slog.Logger
	interval time.Duration
}

func NewWorker(db *pgxpool.Pool, sender Sender, log *slog.Logger) *Worker {
	return &Worker{db: db, sender: sender, log: log, interval: 5 * time.Second}
}

// Run sends pending SMS until ctx ends.
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		if _, err := w.SendPending(ctx); err != nil && ctx.Err() == nil {
			w.log.Error("sms worker", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// SendPending sends one batch of pending SMS and returns how many were sent.
// Rows are locked with SKIP LOCKED so several servers can run workers safely.
func (w *Worker) SendPending(ctx context.Context) (int, error) {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT n.id, u.phone, n.body, n.sms_attempts
		FROM notifications n JOIN users u ON u.id = n.user_id
		WHERE n.sms_status = 'pending'
		ORDER BY n.id
		LIMIT 50
		FOR UPDATE OF n SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	type job struct {
		id       int64
		phone    string
		body     string
		attempts int
	}
	jobs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (job, error) {
		var j job
		return j, r.Scan(&j.id, &j.phone, &j.body, &j.attempts)
	})
	if err != nil {
		return 0, err
	}

	sent := 0
	for _, j := range jobs {
		sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := w.sender.Send(sctx, j.phone, j.body)
		cancel()
		if err == nil {
			sent++
			_, err = tx.Exec(ctx, `
				UPDATE notifications SET sms_status = 'sent', sent_at = now(), sms_attempts = sms_attempts + 1, sms_error = NULL
				WHERE id = $1`, j.id)
		} else {
			status := "pending"
			if j.attempts+1 >= MaxSMSAttempts {
				status = "failed"
			}
			w.log.Warn("sms send failed", "notification", j.id, "attempt", j.attempts+1, "err", err)
			_, err = tx.Exec(ctx, `
				UPDATE notifications SET sms_status = $2, sms_attempts = sms_attempts + 1, sms_error = $3
				WHERE id = $1`, j.id, status, truncate(err.Error(), 300))
		}
		if err != nil {
			return sent, err
		}
	}
	return sent, tx.Commit(ctx)
}
