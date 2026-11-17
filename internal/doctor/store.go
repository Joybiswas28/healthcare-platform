package doctor

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

var errDoctorNotFound = apperr.NotFound("doctor not found")

// ---------------------------------------------------------------- profile

const profileSelect = `
SELECT u.id, u.name, d.bmdc_reg_no, d.specialization, d.degrees, d.experience_years,
       d.bio, d.verification_status, d.verification_note
FROM doctor_profiles d
JOIN users u ON u.id = d.user_id`

func scanProfile(row pgx.Row) (Profile, error) {
	var p Profile
	err := row.Scan(&p.UserID, &p.Name, &p.BMDCRegNo, &p.Specialization, &p.Degrees,
		&p.ExperienceYears, &p.Bio, &p.VerificationStatus, &p.VerificationNote)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, errDoctorNotFound
	}
	return p, err
}

func (s *Store) GetProfile(ctx context.Context, doctorID string) (Profile, error) {
	return scanProfile(s.db.QueryRow(ctx, profileSelect+` WHERE d.user_id = $1`, doctorID))
}

// UpdateProfile saves a doctor's professional details. A rejected doctor who
// resubmits goes back to "pending". A verified doctor cannot change their BMDC number.
func (s *Store) UpdateProfile(ctx context.Context, doctorID string, in ProfileInput) (Profile, error) {
	if err := in.normalize(); err != nil {
		return Profile{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var curBMDC *string
	var status string
	err = tx.QueryRow(ctx,
		`SELECT bmdc_reg_no, verification_status FROM doctor_profiles WHERE user_id = $1 FOR UPDATE`,
		doctorID).Scan(&curBMDC, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, errDoctorNotFound
	}
	if err != nil {
		return Profile{}, err
	}

	if status == StatusApproved && curBMDC != nil && *curBMDC != in.BMDCRegNo {
		return Profile{}, apperr.Validation("a verified doctor cannot change their BMDC number; contact support")
	}
	if status == StatusRejected {
		status = StatusPending
	}

	_, err = tx.Exec(ctx, `
		UPDATE doctor_profiles
		SET bmdc_reg_no = $2, specialization = $3, degrees = $4, experience_years = $5,
		    bio = $6, verification_status = $7, updated_at = now()
		WHERE user_id = $1`,
		doctorID, in.BMDCRegNo, in.Specialization, in.Degrees, in.ExperienceYears,
		nullIfEmpty(in.Bio), status)
	if database.IsUniqueViolation(err) {
		return Profile{}, apperr.Conflict("this BMDC registration number is already registered")
	}
	if err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Profile{}, err
	}
	return s.GetProfile(ctx, doctorID)
}

// ---------------------------------------------------------------- chambers

const chamberColumns = `id, doctor_id, name, address, district, area, phone, fee_paisa, booking_enabled`

func scanChamber(row pgx.Row) (Chamber, error) {
	var c Chamber
	err := row.Scan(&c.ID, &c.DoctorID, &c.Name, &c.Address, &c.District, &c.Area, &c.Phone,
		&c.FeePaisa, &c.BookingEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return Chamber{}, apperr.NotFound("chamber not found")
	}
	c.Schedules = []Schedule{}
	return c, err
}

func (s *Store) CreateChamber(ctx context.Context, doctorID string, in ChamberInput) (Chamber, error) {
	if err := in.normalize(); err != nil {
		return Chamber{}, err
	}
	return scanChamber(s.db.QueryRow(ctx, `
		INSERT INTO chambers (doctor_id, name, address, district, area, phone, fee_paisa, booking_enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+chamberColumns,
		doctorID, in.Name, in.Address, in.District, nullIfEmpty(in.Area), nullIfEmpty(in.Phone),
		in.FeePaisa, *in.BookingEnabled))
}

func (s *Store) UpdateChamber(ctx context.Context, doctorID, chamberID string, in ChamberInput) (Chamber, error) {
	if err := in.normalize(); err != nil {
		return Chamber{}, err
	}
	c, err := scanChamber(s.db.QueryRow(ctx, `
		UPDATE chambers
		SET name = $3, address = $4, district = $5, area = $6, phone = $7, fee_paisa = $8,
		    booking_enabled = $9, updated_at = now()
		WHERE id = $1 AND doctor_id = $2
		RETURNING `+chamberColumns,
		chamberID, doctorID, in.Name, in.Address, in.District, nullIfEmpty(in.Area),
		nullIfEmpty(in.Phone), in.FeePaisa, *in.BookingEnabled))
	if err != nil {
		return Chamber{}, err
	}
	if err := s.attachSchedules(ctx, []*Chamber{&c}); err != nil {
		return Chamber{}, err
	}
	return c, nil
}

// ListChambers returns a doctor's chambers with their active schedules.
// With onlyBookable, chambers that have booking switched off are left out.
func (s *Store) ListChambers(ctx context.Context, doctorID string, onlyBookable bool) ([]Chamber, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+chamberColumns+` FROM chambers
		WHERE doctor_id = $1 AND (NOT $2 OR booking_enabled)
		ORDER BY created_at, id`, doctorID, onlyBookable)
	if err != nil {
		return nil, err
	}
	chambers, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Chamber, error) { return scanChamber(r) })
	if err != nil {
		return nil, err
	}
	ptrs := make([]*Chamber, len(chambers))
	for i := range chambers {
		ptrs[i] = &chambers[i]
	}
	return chambers, s.attachSchedules(ctx, ptrs)
}

func (s *Store) attachSchedules(ctx context.Context, chambers []*Chamber) error {
	if len(chambers) == 0 {
		return nil
	}
	byID := make(map[string]*Chamber, len(chambers))
	ids := make([]string, 0, len(chambers))
	for _, c := range chambers {
		byID[c.ID] = c
		ids = append(ids, c.ID)
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+scheduleColumns+` FROM chamber_schedules
		WHERE chamber_id = ANY($1::uuid[]) AND active
		ORDER BY weekday, start_time`, ids)
	if err != nil {
		return err
	}
	schedules, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Schedule, error) { return scanSchedule(r) })
	if err != nil {
		return err
	}
	for _, sc := range schedules {
		c := byID[sc.ChamberID]
		c.Schedules = append(c.Schedules, sc)
	}
	return nil
}

// ---------------------------------------------------------------- schedules

const scheduleColumns = `id, chamber_id, weekday, to_char(start_time, 'HH24:MI'), to_char(end_time, 'HH24:MI'), max_patients`

func scanSchedule(row pgx.Row) (Schedule, error) {
	var sc Schedule
	err := row.Scan(&sc.ID, &sc.ChamberID, &sc.Weekday, &sc.StartTime, &sc.EndTime, &sc.MaxPatients)
	return sc, err
}

// AddSchedule adds a weekly session to one of the doctor's chambers. Sessions of the
// same doctor may not overlap on the same weekday, even across different chambers.
func (s *Store) AddSchedule(ctx context.Context, doctorID, chamberID string, in ScheduleInput) (Schedule, error) {
	if err := in.normalize(); err != nil {
		return Schedule{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Schedule{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the doctor's profile row so concurrent AddSchedule calls cannot both
	// pass the overlap check.
	var owner string
	err = tx.QueryRow(ctx, `
		SELECT c.doctor_id FROM chambers c
		JOIN doctor_profiles d ON d.user_id = c.doctor_id
		WHERE c.id = $1 AND c.doctor_id = $2
		FOR UPDATE OF d`, chamberID, doctorID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return Schedule{}, apperr.NotFound("chamber not found")
	}
	if err != nil {
		return Schedule{}, err
	}

	var overlaps bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM chamber_schedules s
			JOIN chambers c ON c.id = s.chamber_id
			WHERE c.doctor_id = $1 AND s.active AND s.weekday = $2
			  AND s.start_time < $4::time AND s.end_time > $3::time
		)`, doctorID, in.Weekday, in.StartTime, in.EndTime).Scan(&overlaps)
	if err != nil {
		return Schedule{}, err
	}
	if overlaps {
		return Schedule{}, apperr.Conflict("this time overlaps with another of your sessions on the same day")
	}

	sc, err := scanSchedule(tx.QueryRow(ctx, `
		INSERT INTO chamber_schedules (chamber_id, weekday, start_time, end_time, max_patients)
		VALUES ($1, $2, $3::time, $4::time, $5)
		RETURNING `+scheduleColumns,
		chamberID, in.Weekday, in.StartTime, in.EndTime, in.MaxPatients))
	if err != nil {
		return Schedule{}, err
	}
	return sc, tx.Commit(ctx)
}

// RemoveSchedule deactivates a session. Existing appointments for it stay valid.
func (s *Store) RemoveSchedule(ctx context.Context, doctorID, scheduleID string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE chamber_schedules s SET active = false
		FROM chambers c
		WHERE s.id = $1 AND c.id = s.chamber_id AND c.doctor_id = $2 AND s.active`,
		scheduleID, doctorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("schedule not found")
	}
	return nil
}

// ---------------------------------------------------------------- public search

type SearchParams struct {
	Name           string
	Specialization string
	Location       string // matches chamber district or area
	Limit, Offset  int
}

// Search lists approved, active doctors.
func (s *Store) Search(ctx context.Context, p SearchParams) ([]Summary, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.id, u.name, d.specialization, d.degrees, d.experience_years,
		       COALESCE(array_agg(DISTINCT c.district) FILTER (WHERE c.id IS NOT NULL), '{}')::text[],
		       MIN(c.fee_paisa)
		FROM doctor_profiles d
		JOIN users u ON u.id = d.user_id
		LEFT JOIN chambers c ON c.doctor_id = d.user_id AND c.booking_enabled
		WHERE d.verification_status = 'approved' AND u.status = 'active'
		  AND ($1 = '' OR u.name ILIKE $1)
		  AND ($2 = '' OR d.specialization ILIKE $2)
		  AND ($3 = '' OR EXISTS (
		        SELECT 1 FROM chambers c2
		        WHERE c2.doctor_id = d.user_id AND c2.booking_enabled
		          AND (c2.district ILIKE $3 OR c2.area ILIKE $3)))
		GROUP BY u.id, u.name, d.specialization, d.degrees, d.experience_years
		ORDER BY u.name, u.id
		LIMIT $4 OFFSET $5`,
		containsPattern(p.Name), containsPattern(p.Specialization), containsPattern(p.Location),
		p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Summary, error) {
		var sm Summary
		err := r.Scan(&sm.ID, &sm.Name, &sm.Specialization, &sm.Degrees, &sm.ExperienceYears,
			&sm.Districts, &sm.MinFeePaisa)
		return sm, err
	})
}

// GetPublic returns an approved doctor's profile with bookable chambers and schedules.
func (s *Store) GetPublic(ctx context.Context, doctorID string) (PublicDoctor, error) {
	p, err := scanProfile(s.db.QueryRow(ctx,
		profileSelect+` WHERE d.user_id = $1 AND d.verification_status = 'approved' AND u.status = 'active'`,
		doctorID))
	if err != nil {
		return PublicDoctor{}, err
	}
	p.VerificationNote = nil
	chambers, err := s.ListChambers(ctx, doctorID, true)
	if err != nil {
		return PublicDoctor{}, err
	}
	return PublicDoctor{Profile: p, Chambers: chambers}, nil
}

// ---------------------------------------------------------------- admin

func (s *Store) ListForAdmin(ctx context.Context, status string, limit, offset int) ([]AdminDoctor, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.id, u.name, d.bmdc_reg_no, d.specialization, d.degrees, d.experience_years,
		       d.bio, d.verification_status, d.verification_note, u.phone, u.created_at
		FROM doctor_profiles d
		JOIN users u ON u.id = d.user_id
		WHERE ($1 = '' OR d.verification_status = $1)
		ORDER BY u.created_at, u.id
		LIMIT $2 OFFSET $3`, status, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdminDoctor, error) {
		var a AdminDoctor
		err := r.Scan(&a.UserID, &a.Name, &a.BMDCRegNo, &a.Specialization, &a.Degrees,
			&a.ExperienceYears, &a.Bio, &a.VerificationStatus, &a.VerificationNote, &a.Phone, &a.CreatedAt)
		return a, err
	})
}

// Verify approves or rejects a doctor. Approval needs a complete profile;
// rejection needs a note explaining why.
func (s *Store) Verify(ctx context.Context, doctorID string, in VerifyInput) (Profile, error) {
	in.Note = strings.TrimSpace(in.Note)
	switch in.Status {
	case StatusApproved:
	case StatusRejected:
		if in.Note == "" {
			return Profile{}, apperr.Validation("note is required when rejecting a doctor")
		}
	default:
		return Profile{}, apperr.Validation("status must be 'approved' or 'rejected'")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var bmdc, spec *string
	err = tx.QueryRow(ctx,
		`SELECT bmdc_reg_no, specialization FROM doctor_profiles WHERE user_id = $1 FOR UPDATE`,
		doctorID).Scan(&bmdc, &spec)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, errDoctorNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	if in.Status == StatusApproved && (bmdc == nil || spec == nil) {
		return Profile{}, apperr.Validation("doctor has not completed their profile (BMDC number, specialization)")
	}

	_, err = tx.Exec(ctx, `
		UPDATE doctor_profiles
		SET verification_status = $2, verification_note = $3,
		    verified_at = CASE WHEN $2 = 'approved' THEN now() END, updated_at = now()
		WHERE user_id = $1`, doctorID, in.Status, nullIfEmpty(in.Note))
	if err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Profile{}, err
	}
	return s.GetProfile(ctx, doctorID)
}
