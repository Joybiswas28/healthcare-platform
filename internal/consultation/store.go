// Package consultation implements private online consultations.
//
// Lifecycle:
//
//	requested --doctor accepts--> accepted --patient confirms (tokens held)--> active
//	    |                             |                                       |
//	    +--doctor rejects--> rejected  +--patient cancels--> cancelled          +--doctor completes (tokens released)--> completed
//	    +--patient cancels--> cancelled                                        +--doctor cancels (tokens refunded)--> cancelled
//
// Messages and file uploads are only allowed while the consultation is active.
package consultation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/family"
	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
	"healthcare-platform/internal/platform/storage"
	"healthcare-platform/internal/user"
	"healthcare-platform/internal/wallet"
)

const (
	StatusRequested = "requested"
	StatusAccepted  = "accepted"
	StatusRejected  = "rejected"
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)

type Consultation struct {
	ID            string     `json:"id"`
	PatientID     string     `json:"patient_id"`
	PatientName   string     `json:"patient_name"`
	DoctorID      string     `json:"doctor_id"`
	DoctorName    string     `json:"doctor_name"`
	Subject       string     `json:"subject"`
	Description   string     `json:"description"`
	FeeTokens     int        `json:"fee_tokens"`
	CommissionBps int        `json:"-"`
	Status        string     `json:"status"`
	RejectReason  *string    `json:"reject_reason"`
	RequestedAt   time.Time  `json:"requested_at"`
	RespondedAt   *time.Time `json:"responded_at"`
	ConfirmedAt   *time.Time `json:"confirmed_at"`
	ClosedAt      *time.Time `json:"closed_at"`

	// Set when the consultation is for a family member of the account holder; the
	// patient_* fields then describe that member.
	FamilyMemberID   *string `json:"family_member_id"`
	Relation         *string `json:"relation"`
	PatientGender    *string `json:"patient_gender"`
	PatientBirthDate *string `json:"patient_birth_date"`
	AccountName      string  `json:"account_name"` // the account holder who asked
}

type Attachment struct {
	ID          string `json:"id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type Message struct {
	ID         int64       `json:"id"`
	SenderID   string      `json:"sender_id"`
	SenderName string      `json:"sender_name"`
	Body       *string     `json:"body"`
	Attachment *Attachment `json:"attachment"`
	CreatedAt  time.Time   `json:"created_at"`
}

type RequestInput struct {
	DoctorID       string `json:"doctor_id"`
	Subject        string `json:"subject"`
	Description    string `json:"description"`
	FamilyMemberID string `json:"family_member_id"` // empty: for the account holder
}

type Store struct {
	db    *pgxpool.Pool
	files storage.Storage
	now   func() time.Time
}

func NewStore(db *pgxpool.Pool, files storage.Storage) *Store {
	return &Store{db: db, files: files, now: time.Now}
}

var errNotFound = apperr.NotFound("consultation not found")

const consultationSelect = `
SELECT c.id, c.patient_id, COALESCE(fm.name, pu.name), c.doctor_id, du.name, c.subject, c.description,
       c.fee_tokens, c.commission_bps, c.status, c.reject_reason,
       c.requested_at, c.responded_at, c.confirmed_at, c.closed_at,
       c.family_member_id, fm.relation, fm.gender, to_char(fm.birth_date, 'YYYY-MM-DD'), pu.name
FROM consultations c
JOIN users pu ON pu.id = c.patient_id
JOIN users du ON du.id = c.doctor_id
LEFT JOIN family_members fm ON fm.id = c.family_member_id`

func scan(row pgx.Row) (Consultation, error) {
	var c Consultation
	err := row.Scan(&c.ID, &c.PatientID, &c.PatientName, &c.DoctorID, &c.DoctorName, &c.Subject,
		&c.Description, &c.FeeTokens, &c.CommissionBps, &c.Status, &c.RejectReason,
		&c.RequestedAt, &c.RespondedAt, &c.ConfirmedAt, &c.ClosedAt,
		&c.FamilyMemberID, &c.Relation, &c.PatientGender, &c.PatientBirthDate, &c.AccountName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Consultation{}, errNotFound
	}
	return c, err
}

// ---------------------------------------------------------------- reads

// Get returns a consultation if userID is its patient or doctor.
func (s *Store) Get(ctx context.Context, userID, id string) (Consultation, error) {
	return scan(s.db.QueryRow(ctx,
		consultationSelect+` WHERE c.id = $1 AND (c.patient_id = $2 OR c.doctor_id = $2)`, id, userID))
}

// List returns the caller's consultations, newest first, optionally filtered by status.
func (s *Store) List(ctx context.Context, userID string, role user.Role, status string, limit, offset int) ([]Consultation, error) {
	col := "c.patient_id"
	if role == user.RoleDoctor {
		col = "c.doctor_id"
	}
	rows, err := s.db.Query(ctx, consultationSelect+`
		WHERE `+col+` = $1 AND ($2 = '' OR c.status = $2)
		ORDER BY c.requested_at DESC, c.id
		LIMIT $3 OFFSET $4`, userID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Consultation, error) { return scan(r) })
}

// ---------------------------------------------------------------- lifecycle

// Request creates a consultation request. The doctor's current fee and the
// platform commission are copied onto it, so later price changes do not affect it.
func (s *Store) Request(ctx context.Context, patientID string, in RequestInput) (Consultation, error) {
	in.Subject = strings.TrimSpace(in.Subject)
	in.Description = strings.TrimSpace(in.Description)
	if n := utf8.RuneCountInString(in.Subject); n < 5 || n > 150 {
		return Consultation{}, apperr.Validation("subject must be 5-150 characters")
	}
	if n := utf8.RuneCountInString(in.Description); n < 20 || n > 5000 {
		return Consultation{}, apperr.Validation("description must be 20-5000 characters")
	}
	if in.DoctorID == patientID {
		return Consultation{}, apperr.Validation("you cannot consult yourself")
	}

	var enabled bool
	var fee, bps int
	err := s.db.QueryRow(ctx, `
		SELECT d.consultation_enabled, d.consultation_fee_tokens, d.commission_bps
		FROM doctor_profiles d JOIN users u ON u.id = d.user_id
		WHERE d.user_id = $1 AND d.verification_status = 'approved' AND u.status = 'active'`,
		in.DoctorID).Scan(&enabled, &fee, &bps)
	if errors.Is(err, pgx.ErrNoRows) {
		return Consultation{}, apperr.NotFound("doctor not found")
	}
	if err != nil {
		return Consultation{}, err
	}
	if !enabled {
		return Consultation{}, apperr.Conflict("this doctor is not taking online consultations")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Consultation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := family.Check(ctx, tx, patientID, in.FamilyMemberID); err != nil {
		return Consultation{}, err
	}
	var member *string
	if in.FamilyMemberID != "" {
		member = &in.FamilyMemberID
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO consultations (patient_id, family_member_id, doctor_id, subject, description, fee_tokens, commission_bps)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		patientID, member, in.DoctorID, in.Subject, in.Description, fee, bps).Scan(&id)
	if database.UniqueViolationOn(err, "consultations_one_open_per_pair") {
		if member != nil {
			return Consultation{}, apperr.Conflict("this family member already has an open consultation with this doctor")
		}
		return Consultation{}, apperr.Conflict("you already have an open consultation with this doctor")
	}
	if err != nil {
		return Consultation{}, err
	}
	if err := notify.Enqueue(ctx, tx, notify.Msg{
		UserID: in.DoctorID,
		Kind:   notify.KindConsultationRequested,
		Title:  "New consultation request",
		Body:   "New online consultation request: " + in.Subject,
		SMS:    true,
	}); err != nil {
		return Consultation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Consultation{}, err
	}
	return s.Get(ctx, patientID, id)
}

// mutate locks the consultation row, lets fn validate and apply a change inside the
// same transaction, then returns the fresh row.
func (s *Store) mutate(ctx context.Context, userID, id string, fn func(tx pgx.Tx, c Consultation) error) (Consultation, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Consultation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	c, err := scan(tx.QueryRow(ctx,
		consultationSelect+` WHERE c.id = $1 AND (c.patient_id = $2 OR c.doctor_id = $2) FOR UPDATE OF c`,
		id, userID))
	if err != nil {
		return Consultation{}, err
	}
	if err := fn(tx, c); err != nil {
		return Consultation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Consultation{}, err
	}
	return s.Get(ctx, userID, id)
}

func requireStatus(c Consultation, want string, action string) error {
	if c.Status != want {
		return apperr.Conflict("cannot %s a consultation that is %s", action, c.Status)
	}
	return nil
}

func (s *Store) Accept(ctx context.Context, doctorID, id string) (Consultation, error) {
	return s.mutate(ctx, doctorID, id, func(tx pgx.Tx, c Consultation) error {
		if c.DoctorID != doctorID {
			return apperr.Forbidden("only the doctor can accept this consultation")
		}
		if err := requireStatus(c, StatusRequested, "accept"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE consultations SET status = 'accepted', responded_at = $2, updated_at = $2 WHERE id = $1`,
			id, s.now()); err != nil {
			return err
		}
		return notify.Enqueue(ctx, tx, notify.Msg{
			UserID: c.PatientID,
			Kind:   notify.KindConsultationAccepted,
			Title:  "Consultation accepted",
			Body:   fmt.Sprintf("%s accepted your consultation request. Confirm to start (%d tokens).", c.DoctorName, c.FeeTokens),
			SMS:    true,
		})
	})
}

func (s *Store) Reject(ctx context.Context, doctorID, id, reason string) (Consultation, error) {
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 5 || n > 1000 {
		return Consultation{}, apperr.Validation("reason must be 5-1000 characters")
	}
	return s.mutate(ctx, doctorID, id, func(tx pgx.Tx, c Consultation) error {
		if c.DoctorID != doctorID {
			return apperr.Forbidden("only the doctor can reject this consultation")
		}
		if err := requireStatus(c, StatusRequested, "reject"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE consultations
			SET status = 'rejected', reject_reason = $2, responded_at = $3, closed_at = $3, updated_at = $3
			WHERE id = $1`, id, reason, s.now()); err != nil {
			return err
		}
		return notify.Enqueue(ctx, tx, notify.Msg{
			UserID: c.PatientID,
			Kind:   notify.KindConsultationRejected,
			Title:  "Consultation declined",
			Body:   fmt.Sprintf("%s could not take your consultation: %s", c.DoctorName, reason),
		})
	})
}

// Confirm is the patient agreeing to pay: the fee is moved from their tokens into escrow.
func (s *Store) Confirm(ctx context.Context, patientID, id string) (Consultation, error) {
	return s.mutate(ctx, patientID, id, func(tx pgx.Tx, c Consultation) error {
		if c.PatientID != patientID {
			return apperr.Forbidden("only the patient can confirm this consultation")
		}
		if err := requireStatus(c, StatusAccepted, "confirm"); err != nil {
			return err
		}
		now := s.now()
		if err := wallet.Hold(ctx, tx, now, patientID, id, c.FeeTokens); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE consultations SET status = 'active', confirmed_at = $2, updated_at = $2 WHERE id = $1`,
			id, now)
		return err
	})
}

// Complete closes an active consultation and pays the doctor.
func (s *Store) Complete(ctx context.Context, doctorID, id string) (Consultation, error) {
	return s.mutate(ctx, doctorID, id, func(tx pgx.Tx, c Consultation) error {
		if c.DoctorID != doctorID {
			return apperr.Forbidden("only the doctor can complete this consultation")
		}
		if err := requireStatus(c, StatusActive, "complete"); err != nil {
			return err
		}
		if err := wallet.Release(ctx, tx, id, c.DoctorID, c.FeeTokens, c.CommissionBps); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE consultations SET status = 'completed', closed_at = $2, updated_at = $2 WHERE id = $1`,
			id, s.now()); err != nil {
			return err
		}
		return notify.Enqueue(ctx, tx, notify.Msg{
			UserID: c.PatientID,
			Kind:   notify.KindConsultationCompleted,
			Title:  "Consultation completed",
			Body:   fmt.Sprintf("Your consultation with %s is complete. Messages and prescriptions remain available.", c.DoctorName),
		})
	})
}

// Cancel: a patient may withdraw before the consultation starts (no money has moved).
// A doctor may cancel an active consultation, which refunds the patient in full.
func (s *Store) Cancel(ctx context.Context, userID, id string) (Consultation, error) {
	return s.mutate(ctx, userID, id, func(tx pgx.Tx, c Consultation) error {
		switch {
		case userID == c.PatientID && (c.Status == StatusRequested || c.Status == StatusAccepted):
		case userID == c.DoctorID && c.Status == StatusActive:
			if err := wallet.Refund(ctx, tx, id, c.PatientID); err != nil {
				return err
			}
		case userID == c.PatientID && c.Status == StatusActive:
			return apperr.Conflict("an active consultation can only be cancelled by the doctor; contact support for help")
		case userID == c.DoctorID && c.Status == StatusRequested:
			return apperr.Conflict("use reject to decline a consultation request")
		default:
			return apperr.Conflict("cannot cancel a consultation that is %s", c.Status)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE consultations SET status = 'cancelled', closed_at = $2, updated_at = $2 WHERE id = $1`,
			id, s.now()); err != nil {
			return err
		}
		// Tell the other side.
		if userID == c.DoctorID {
			return notify.Enqueue(ctx, tx, notify.Msg{
				UserID: c.PatientID,
				Kind:   notify.KindConsultationCancelled,
				Title:  "Consultation cancelled",
				Body:   fmt.Sprintf("%s cancelled your consultation. %d tokens have been returned to your wallet.", c.DoctorName, c.FeeTokens),
				SMS:    true,
			})
		}
		return notify.Enqueue(ctx, tx, notify.Msg{
			UserID: c.DoctorID,
			Kind:   notify.KindConsultationCancelled,
			Title:  "Consultation request withdrawn",
			Body:   "The patient withdrew their request: " + c.Subject,
		})
	})
}

// ---------------------------------------------------------------- messages & files

// inactiveError explains why the caller cannot post to a consultation.
func (s *Store) inactiveError(ctx context.Context, userID, id string) error {
	c, err := s.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	return apperr.Conflict("messages can only be sent while the consultation is active (it is %s)", c.Status)
}

func (s *Store) PostMessage(ctx context.Context, userID, id, body string) (Message, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < 1 || n > 4000 {
		return Message{}, apperr.Validation("message must be 1-4000 characters")
	}
	var msgID int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO consultation_messages (consultation_id, sender_id, body)
		SELECT id, $2, $3 FROM consultations
		WHERE id = $1 AND status = 'active' AND (patient_id = $2 OR doctor_id = $2)
		RETURNING id`, id, userID, body).Scan(&msgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, s.inactiveError(ctx, userID, id)
	}
	if err != nil {
		return Message{}, err
	}
	return s.message(ctx, msgID)
}

const messageSelect = `
SELECT m.id, m.sender_id, u.name, m.body, a.id, a.file_name, a.content_type, a.size_bytes, m.created_at
FROM consultation_messages m
JOIN users u            ON u.id = m.sender_id
LEFT JOIN attachments a ON a.id = m.attachment_id`

func scanMessage(row pgx.Row) (Message, error) {
	var m Message
	var aID, aName, aType *string
	var aSize *int64
	if err := row.Scan(&m.ID, &m.SenderID, &m.SenderName, &m.Body, &aID, &aName, &aType, &aSize, &m.CreatedAt); err != nil {
		return Message{}, err
	}
	if aID != nil {
		m.Attachment = &Attachment{ID: *aID, FileName: *aName, ContentType: *aType, SizeBytes: *aSize}
	}
	return m, nil
}

func (s *Store) message(ctx context.Context, id int64) (Message, error) {
	return scanMessage(s.db.QueryRow(ctx, messageSelect+` WHERE m.id = $1`, id))
}

// Messages returns messages with id greater than after, oldest first. Clients poll
// with the last id they have seen.
func (s *Store) Messages(ctx context.Context, userID, id string, after int64, limit int) ([]Message, error) {
	if _, err := s.Get(ctx, userID, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, messageSelect+`
		WHERE m.consultation_id = $1 AND m.id > $2
		ORDER BY m.id LIMIT $3`, id, after, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Message, error) { return scanMessage(r) })
}

// Upload stores a file (e.g. a lab report) and posts it as a message.
func (s *Store) Upload(ctx context.Context, userID, id, fileName, contentType string, r io.Reader) (Message, error) {
	c, err := s.Get(ctx, userID, id)
	if err != nil {
		return Message{}, err
	}
	if c.Status != StatusActive {
		return Message{}, apperr.Conflict("files can only be shared while the consultation is active (it is %s)", c.Status)
	}

	key := uuid.NewString()
	size, err := s.files.Put(ctx, key, r)
	if err != nil {
		return Message{}, err
	}
	if size == 0 {
		_ = s.files.Delete(ctx, key)
		return Message{}, apperr.Validation("file is empty")
	}

	msgID, err := s.saveUpload(ctx, userID, id, fileName, contentType, size, key)
	if err != nil {
		_ = s.files.Delete(ctx, key)
		return Message{}, err
	}
	return s.message(ctx, msgID)
}

func (s *Store) saveUpload(ctx context.Context, userID, id, fileName, contentType string, size int64, key string) (int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var attID string
	err = tx.QueryRow(ctx, `
		INSERT INTO attachments (consultation_id, uploader_id, file_name, content_type, size_bytes, storage_key)
		SELECT id, $2, $3, $4, $5, $6 FROM consultations
		WHERE id = $1 AND status = 'active' AND (patient_id = $2 OR doctor_id = $2)
		RETURNING id`, id, userID, fileName, contentType, size, key).Scan(&attID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, s.inactiveError(ctx, userID, id)
	}
	if err != nil {
		return 0, err
	}
	var msgID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO consultation_messages (consultation_id, sender_id, attachment_id)
		VALUES ($1, $2, $3) RETURNING id`, id, userID, attID).Scan(&msgID); err != nil {
		return 0, err
	}
	return msgID, tx.Commit(ctx)
}

// OpenAttachment returns a file if userID is a participant of its consultation.
// The caller must close the reader.
func (s *Store) OpenAttachment(ctx context.Context, userID, attachmentID string) (Attachment, io.ReadCloser, error) {
	var a Attachment
	var key string
	err := s.db.QueryRow(ctx, `
		SELECT a.id, a.file_name, a.content_type, a.size_bytes, a.storage_key
		FROM attachments a JOIN consultations c ON c.id = a.consultation_id
		WHERE a.id = $1 AND (c.patient_id = $2 OR c.doctor_id = $2)`,
		attachmentID, userID).Scan(&a.ID, &a.FileName, &a.ContentType, &a.SizeBytes, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, nil, apperr.NotFound("file not found")
	}
	if err != nil {
		return Attachment{}, nil, err
	}
	rc, err := s.files.Open(ctx, key)
	if err != nil {
		return Attachment{}, nil, err
	}
	return a, rc, nil
}
