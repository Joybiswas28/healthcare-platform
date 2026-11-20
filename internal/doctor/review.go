package doctor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
)

// Review is a patient's rating of a doctor after a finished visit. It never
// carries the patient's identity: the doctor and the public see only the
// rating, the comment, where the visit happened and when.
type Review struct {
	ID        string     `json:"id"`
	Rating    int        `json:"rating"`
	Comment   *string    `json:"comment"`
	Source    string     `json:"source"` // "chamber" or "online"
	Reply     *string    `json:"reply"`
	RepliedAt *time.Time `json:"replied_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// MyReview is a review as its author sees it, with the visit it belongs to.
type MyReview struct {
	Review
	DoctorID       string  `json:"doctor_id"`
	AppointmentID  *string `json:"appointment_id"`
	ConsultationID *string `json:"consultation_id"`
}

type ReviewInput struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

type ReplyInput struct {
	Reply string `json:"reply"`
}

const (
	maxReviewComment = 1000
	maxPublicReviews = 20
)

func (in *ReviewInput) normalize() error {
	in.Comment = strings.TrimSpace(in.Comment)
	switch {
	case in.Rating < 1 || in.Rating > 5:
		return apperr.Validation("rating must be 1-5")
	case utf8.RuneCountInString(in.Comment) > maxReviewComment:
		return apperr.Validation("comment must be at most %d characters", maxReviewComment)
	}
	return nil
}

const reviewColumns = `r.id, r.rating, r.comment,
       CASE WHEN r.appointment_id IS NOT NULL THEN 'chamber' ELSE 'online' END,
       r.reply, r.replied_at, r.created_at`

func reviewDest(v *Review) []any {
	return []any{&v.ID, &v.Rating, &v.Comment, &v.Source, &v.Reply, &v.RepliedAt, &v.CreatedAt}
}

// visit describes the two kinds of visit a patient can review.
type visit struct {
	table, column, done, what string
}

var (
	chamberVisit = visit{"appointments", "appointment_id", "completed", "appointment"}
	onlineVisit  = visit{"consultations", "consultation_id", "completed", "consultation"}
)

// ReviewAppointment rates the doctor of a completed chamber appointment.
func (s *Store) ReviewAppointment(ctx context.Context, patientID, appointmentID string, in ReviewInput) (Review, error) {
	return s.review(ctx, chamberVisit, patientID, appointmentID, in)
}

// ReviewConsultation rates the doctor of a completed online consultation.
func (s *Store) ReviewConsultation(ctx context.Context, patientID, consultationID string, in ReviewInput) (Review, error) {
	return s.review(ctx, onlineVisit, patientID, consultationID, in)
}

func (s *Store) review(ctx context.Context, v visit, patientID, visitID string, in ReviewInput) (Review, error) {
	if err := in.normalize(); err != nil {
		return Review{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Review{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var out Review
	var doctorID string
	err = tx.QueryRow(ctx, `
		WITH added AS (
			INSERT INTO doctor_reviews (doctor_id, patient_id, `+v.column+`, rating, comment)
			SELECT x.doctor_id, x.patient_id, x.id, $3, $4 FROM `+v.table+` x
			WHERE x.id = $1 AND x.patient_id = $2 AND x.status = $5
			ON CONFLICT DO NOTHING
			RETURNING *
		)
		SELECT `+reviewColumns+`, r.doctor_id FROM added r`,
		visitID, patientID, in.Rating, nullIfEmpty(in.Comment), v.done).
		Scan(append(reviewDest(&out), &doctorID)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Review{}, s.whyNoReview(ctx, v, patientID, visitID)
	}
	if err != nil {
		return Review{}, err
	}
	if err := notify.Enqueue(ctx, tx, notify.Msg{
		UserID: doctorID, Kind: notify.KindDoctorReview, Title: "New review",
		Body: fmt.Sprintf("A patient rated their %s %d out of 5.", v.what, in.Rating),
	}); err != nil {
		return Review{}, err
	}
	return out, tx.Commit(ctx)
}

// whyNoReview explains why a review was not added.
func (s *Store) whyNoReview(ctx context.Context, v visit, patientID, visitID string) error {
	var status string
	var reviewed bool
	err := s.db.QueryRow(ctx, `
		SELECT x.status, EXISTS (SELECT 1 FROM doctor_reviews r WHERE r.`+v.column+` = x.id)
		FROM `+v.table+` x WHERE x.id = $1 AND x.patient_id = $2`, visitID, patientID).Scan(&status, &reviewed)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return apperr.NotFound("%s not found", v.what)
	case err != nil:
		return err
	case reviewed:
		return apperr.Conflict("you have already reviewed this %s", v.what)
	default:
		return apperr.Conflict("you can review a %s once it is completed", v.what)
	}
}

// MyReviews lists the reviews a patient has written, newest first.
func (s *Store) MyReviews(ctx context.Context, patientID string) ([]MyReview, error) {
	rows, err := s.db.Query(ctx, `SELECT `+reviewColumns+`, r.doctor_id, r.appointment_id, r.consultation_id
		FROM doctor_reviews r WHERE r.patient_id = $1 ORDER BY r.created_at DESC`, patientID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (MyReview, error) {
		var m MyReview
		err := row.Scan(append(reviewDest(&m.Review), &m.DoctorID, &m.AppointmentID, &m.ConsultationID)...)
		return m, err
	})
}

// DoctorReviews lists the reviews of a doctor, newest first.
func (s *Store) DoctorReviews(ctx context.Context, doctorID string, limit, offset int) ([]Review, error) {
	rows, err := s.db.Query(ctx, `SELECT `+reviewColumns+` FROM doctor_reviews r
		WHERE r.doctor_id = $1 ORDER BY r.created_at DESC, r.id LIMIT $2 OFFSET $3`, doctorID, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Review, error) {
		var v Review
		err := row.Scan(reviewDest(&v)...)
		return v, err
	})
}

// Reply sets, or replaces, the doctor's public answer to a review of them.
// An empty reply removes it.
func (s *Store) Reply(ctx context.Context, doctorID, reviewID string, in ReplyInput) (Review, error) {
	in.Reply = strings.TrimSpace(in.Reply)
	if utf8.RuneCountInString(in.Reply) > maxReviewComment {
		return Review{}, apperr.Validation("reply must be at most %d characters", maxReviewComment)
	}
	var v Review
	err := s.db.QueryRow(ctx, `
		UPDATE doctor_reviews r
		SET reply = $3, replied_at = CASE WHEN $3::text IS NULL THEN NULL ELSE now() END
		WHERE r.id = $1 AND r.doctor_id = $2
		RETURNING `+reviewColumns, reviewID, doctorID, nullIfEmpty(in.Reply)).Scan(reviewDest(&v)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Review{}, apperr.NotFound("review not found")
	}
	return v, err
}
