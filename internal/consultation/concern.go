package consultation

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
	"healthcare-platform/internal/wallet"
)

const (
	ConcernOpen      = "open"
	ConcernClosed    = "closed"
	ConcernConverted = "converted" // an offer was accepted and became a consultation

	OfferPending   = "pending"
	OfferAccepted  = "accepted"
	OfferDeclined  = "declined"
	OfferWithdrawn = "withdrawn"

	// MaxOpenConcerns limits how many concerns a patient may have open at once.
	MaxOpenConcerns = 3
)

// Concern is a patient's public medical question. PatientID is only filled in
// for the patient who owns it; doctors browsing the feed never see who posted it.
type Concern struct {
	ID             string     `json:"id"`
	PatientID      string     `json:"patient_id,omitempty"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Specialization *string    `json:"specialization"`
	Status         string     `json:"status"`
	ConsultationID *string    `json:"consultation_id,omitempty"`
	OfferCount     int        `json:"offer_count"`
	CreatedAt      time.Time  `json:"created_at"`
	ClosedAt       *time.Time `json:"closed_at"`

	Offers  []Offer `json:"offers,omitempty"`   // patient view
	MyOffer *Offer  `json:"my_offer,omitempty"` // doctor view
}

type Offer struct {
	ID              string    `json:"id"`
	ConcernID       string    `json:"concern_id"`
	DoctorID        string    `json:"doctor_id"`
	DoctorName      string    `json:"doctor_name"`
	Specialization  *string   `json:"specialization"`
	Degrees         *string   `json:"degrees"`
	ExperienceYears *int      `json:"experience_years"`
	Message         string    `json:"message"`
	FeeTokens       int       `json:"fee_tokens"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

type ConcernInput struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Specialization string `json:"specialization"` // optional: which kind of doctor should look
}

const concernColumns = `
c.id, c.patient_id, c.title, c.description, c.specialization, c.status, c.consultation_id,
(SELECT count(*) FROM concern_offers o WHERE o.concern_id = c.id AND o.status IN ('pending', 'accepted')),
c.created_at, c.closed_at`

func scanConcern(row pgx.Row) (Concern, error) {
	var c Concern
	err := row.Scan(&c.ID, &c.PatientID, &c.Title, &c.Description, &c.Specialization, &c.Status,
		&c.ConsultationID, &c.OfferCount, &c.CreatedAt, &c.ClosedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Concern{}, apperr.NotFound("concern not found")
	}
	return c, err
}

func collectConcerns(rows pgx.Rows, err error) ([]Concern, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Concern, error) { return scanConcern(r) })
}

const offerSelect = `
SELECT o.id, o.concern_id, o.doctor_id, u.name, d.specialization, d.degrees, d.experience_years,
       o.message, o.fee_tokens, o.status, o.created_at
FROM concern_offers o
JOIN users u           ON u.id = o.doctor_id
JOIN doctor_profiles d ON d.user_id = o.doctor_id`

func scanOffer(row pgx.Row) (Offer, error) {
	var o Offer
	err := row.Scan(&o.ID, &o.ConcernID, &o.DoctorID, &o.DoctorName, &o.Specialization, &o.Degrees,
		&o.ExperienceYears, &o.Message, &o.FeeTokens, &o.Status, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Offer{}, apperr.NotFound("offer not found")
	}
	return o, err
}

// ---------------------------------------------------------------- patient side

func (s *Store) PostConcern(ctx context.Context, patientID string, in ConcernInput) (Concern, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Specialization = strings.TrimSpace(in.Specialization)
	if n := utf8.RuneCountInString(in.Title); n < 5 || n > 150 {
		return Concern{}, apperr.Validation("title must be 5-150 characters")
	}
	if n := utf8.RuneCountInString(in.Description); n < 20 || n > 5000 {
		return Concern{}, apperr.Validation("description must be 20-5000 characters")
	}
	if utf8.RuneCountInString(in.Specialization) > 100 {
		return Concern{}, apperr.Validation("specialization must be at most 100 characters")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Concern{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialise concern creation per patient so the open-concern limit holds.
	// NO KEY UPDATE does not block foreign-key checks against the user row.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR NO KEY UPDATE`, patientID); err != nil {
		return Concern{}, err
	}
	var open int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM concerns WHERE patient_id = $1 AND status = 'open'`, patientID).Scan(&open); err != nil {
		return Concern{}, err
	}
	if open >= MaxOpenConcerns {
		return Concern{}, apperr.Conflict("you can have at most %d open concerns; close one first", MaxOpenConcerns)
	}

	var spec *string
	if in.Specialization != "" {
		spec = &in.Specialization
	}
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO concerns (patient_id, title, description, specialization)
		VALUES ($1, $2, $3, $4) RETURNING id`, patientID, in.Title, in.Description, spec).Scan(&id); err != nil {
		return Concern{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Concern{}, err
	}
	return s.PatientConcern(ctx, patientID, id)
}

func (s *Store) MyConcerns(ctx context.Context, patientID string, limit, offset int) ([]Concern, error) {
	return collectConcerns(s.db.Query(ctx, `SELECT `+concernColumns+` FROM concerns c
		WHERE c.patient_id = $1 ORDER BY c.created_at DESC, c.id LIMIT $2 OFFSET $3`, patientID, limit, offset))
}

// PatientConcern returns the patient's own concern together with every offer on it.
func (s *Store) PatientConcern(ctx context.Context, patientID, id string) (Concern, error) {
	c, err := scanConcern(s.db.QueryRow(ctx,
		`SELECT `+concernColumns+` FROM concerns c WHERE c.id = $1 AND c.patient_id = $2`, id, patientID))
	if err != nil {
		return Concern{}, err
	}
	rows, err := s.db.Query(ctx, offerSelect+`
		WHERE o.concern_id = $1 AND o.status <> 'withdrawn' ORDER BY o.created_at, o.id`, id)
	if err != nil {
		return Concern{}, err
	}
	c.Offers, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Offer, error) { return scanOffer(r) })
	return c, err
}

// CloseConcern withdraws a concern; pending offers on it are declined.
func (s *Store) CloseConcern(ctx context.Context, patientID, id string) (Concern, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Concern{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE concerns SET status = 'closed', closed_at = $3
		WHERE id = $1 AND patient_id = $2 AND status = 'open'`, id, patientID, s.now())
	if err != nil {
		return Concern{}, err
	}
	if tag.RowsAffected() == 0 {
		c, err := s.PatientConcern(ctx, patientID, id)
		if err != nil {
			return Concern{}, err
		}
		return Concern{}, apperr.Conflict("this concern is already %s", c.Status)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE concern_offers SET status = 'declined', updated_at = now()
		WHERE concern_id = $1 AND status = 'pending'`, id); err != nil {
		return Concern{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Concern{}, err
	}
	return s.PatientConcern(ctx, patientID, id)
}

// AcceptOffer turns a concern into a paid, active consultation with the offering
// doctor. The concern row is locked, so even if the patient accepts two offers at
// once only one succeeds and tokens are taken only once.
func (s *Store) AcceptOffer(ctx context.Context, patientID, offerID string) (Consultation, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Consultation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		concernID, doctorID, owner, concernStatus, offerStatus string
		title, description, doctorStatus                       string
		feeTokens, commissionBps                               int
	)
	err = tx.QueryRow(ctx, `
		SELECT o.concern_id, o.doctor_id, o.fee_tokens, o.commission_bps, o.status,
		       c.patient_id, c.status, c.title, c.description, d.verification_status
		FROM concern_offers o
		JOIN concerns c        ON c.id = o.concern_id
		JOIN doctor_profiles d ON d.user_id = o.doctor_id
		WHERE o.id = $1
		FOR UPDATE OF c, o`, offerID).Scan(&concernID, &doctorID, &feeTokens, &commissionBps, &offerStatus,
		&owner, &concernStatus, &title, &description, &doctorStatus)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != patientID) {
		return Consultation{}, apperr.NotFound("offer not found")
	}
	if err != nil {
		return Consultation{}, err
	}
	if concernStatus != ConcernOpen {
		return Consultation{}, apperr.Conflict("this concern is already %s", concernStatus)
	}
	if offerStatus != OfferPending {
		return Consultation{}, apperr.Conflict("this offer is %s", offerStatus)
	}
	if doctorStatus != "approved" {
		return Consultation{}, apperr.Conflict("this doctor can no longer take consultations")
	}

	now := s.now()
	var consultationID string
	err = tx.QueryRow(ctx, `
		INSERT INTO consultations
		    (patient_id, doctor_id, subject, description, fee_tokens, commission_bps,
		     status, responded_at, confirmed_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $7, $7)
		RETURNING id`,
		patientID, doctorID, title, description, feeTokens, commissionBps, now).Scan(&consultationID)
	if database.UniqueViolationOn(err, "consultations_one_open_per_pair") {
		return Consultation{}, apperr.Conflict("you already have an open consultation with this doctor")
	}
	if err != nil {
		return Consultation{}, err
	}
	if err := wallet.Hold(ctx, tx, now, patientID, consultationID, feeTokens); err != nil {
		return Consultation{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE concern_offers
		SET status = CASE WHEN id = $2 THEN 'accepted' ELSE 'declined' END, updated_at = $3
		WHERE concern_id = $1 AND status = 'pending'`, concernID, offerID, now); err != nil {
		return Consultation{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE concerns SET status = 'converted', consultation_id = $2, closed_at = $3 WHERE id = $1`,
		concernID, consultationID, now); err != nil {
		return Consultation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Consultation{}, err
	}
	return s.Get(ctx, patientID, consultationID)
}

// ---------------------------------------------------------------- doctor side

func hideOwner(cs []Concern) []Concern {
	for i := range cs {
		cs[i].PatientID = ""
	}
	return cs
}

// OpenConcerns is the feed doctors browse. It never includes who posted a concern.
func (s *Store) OpenConcerns(ctx context.Context, specialization string, limit, offset int) ([]Concern, error) {
	pattern := ""
	if sp := strings.TrimSpace(specialization); sp != "" {
		pattern = "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(sp) + "%"
	}
	cs, err := collectConcerns(s.db.Query(ctx, `SELECT `+concernColumns+` FROM concerns c
		WHERE c.status = 'open' AND ($1 = '' OR c.specialization ILIKE $1)
		ORDER BY c.created_at DESC, c.id LIMIT $2 OFFSET $3`, pattern, limit, offset))
	return hideOwner(cs), err
}

// DoctorConcern shows a concern to a doctor: any open concern, or a closed one they offered on.
func (s *Store) DoctorConcern(ctx context.Context, doctorID, id string) (Concern, error) {
	c, err := scanConcern(s.db.QueryRow(ctx, `SELECT `+concernColumns+` FROM concerns c
		WHERE c.id = $1 AND (c.status = 'open'
		      OR EXISTS (SELECT 1 FROM concern_offers o WHERE o.concern_id = c.id AND o.doctor_id = $2))`,
		id, doctorID))
	if err != nil {
		return Concern{}, err
	}
	c.PatientID = ""
	if c.ConsultationID != nil {
		// Only the doctor whose offer was accepted may know which consultation it became.
		var accepted bool
		_ = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM concern_offers
			WHERE concern_id = $1 AND doctor_id = $2 AND status = 'accepted')`, id, doctorID).Scan(&accepted)
		if !accepted {
			c.ConsultationID = nil
		}
	}
	o, err := scanOffer(s.db.QueryRow(ctx, offerSelect+` WHERE o.concern_id = $1 AND o.doctor_id = $2`, id, doctorID))
	if err == nil {
		c.MyOffer = &o
	} else if !apperr.Is(err, apperr.KindNotFound) {
		return Concern{}, err
	}
	return c, nil
}

// MakeOffer lets a verified doctor who takes online consultations offer to help.
// A withdrawn offer may be renewed; otherwise a doctor gets one offer per concern.
func (s *Store) MakeOffer(ctx context.Context, doctorID, concernID, message string) (Offer, error) {
	message = strings.TrimSpace(message)
	if n := utf8.RuneCountInString(message); n < 10 || n > 1000 {
		return Offer{}, apperr.Validation("message must be 10-1000 characters")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Offer{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM concerns WHERE id = $1 FOR SHARE`, concernID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Offer{}, apperr.NotFound("concern not found")
	}
	if err != nil {
		return Offer{}, err
	}
	if status != ConcernOpen {
		return Offer{}, apperr.Conflict("this concern is no longer open")
	}

	var enabled bool
	var fee, bps int
	err = tx.QueryRow(ctx, `
		SELECT consultation_enabled, consultation_fee_tokens, commission_bps FROM doctor_profiles
		WHERE user_id = $1 AND verification_status = 'approved'`, doctorID).Scan(&enabled, &fee, &bps)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !enabled) {
		return Offer{}, apperr.Forbidden("turn on online consultations (and be verified) before making offers")
	}
	if err != nil {
		return Offer{}, err
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO concern_offers (concern_id, doctor_id, message, fee_tokens, commission_bps)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (concern_id, doctor_id) DO UPDATE
		    SET message = EXCLUDED.message, fee_tokens = EXCLUDED.fee_tokens,
		        commission_bps = EXCLUDED.commission_bps, status = 'pending', updated_at = now()
		    WHERE concern_offers.status = 'withdrawn'
		RETURNING id`, concernID, doctorID, message, fee, bps).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Offer{}, apperr.Conflict("you have already made an offer on this concern")
	}
	if err != nil {
		return Offer{}, err
	}
	o, err := scanOffer(tx.QueryRow(ctx, offerSelect+` WHERE o.id = $1`, id))
	if err != nil {
		return Offer{}, err
	}
	return o, tx.Commit(ctx)
}

func (s *Store) WithdrawOffer(ctx context.Context, doctorID, offerID string) (Offer, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE concern_offers SET status = 'withdrawn', updated_at = now()
		WHERE id = $1 AND doctor_id = $2 AND status = 'pending'`, offerID, doctorID)
	if err != nil {
		return Offer{}, err
	}
	o, err := scanOffer(s.db.QueryRow(ctx, offerSelect+` WHERE o.id = $1 AND o.doctor_id = $2`, offerID, doctorID))
	if err != nil {
		return Offer{}, err
	}
	if tag.RowsAffected() == 0 {
		return Offer{}, apperr.Conflict("only pending offers can be withdrawn (this one is %s)", o.Status)
	}
	return o, nil
}

// MyOffers lists a doctor's offers, newest first.
func (s *Store) MyOffers(ctx context.Context, doctorID string, limit, offset int) ([]Offer, error) {
	rows, err := s.db.Query(ctx, offerSelect+`
		WHERE o.doctor_id = $1 ORDER BY o.created_at DESC, o.id LIMIT $2 OFFSET $3`, doctorID, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Offer, error) { return scanOffer(r) })
}
