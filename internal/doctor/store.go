package doctor

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

var errDoctorNotFound = apperr.NotFound("doctor not found")

// ---------------------------------------------------------------- profile

const profileColumns = `u.id, u.name, d.bmdc_reg_no, d.specialization, d.degrees, d.experience_years,
       d.bio, d.verification_status, d.verification_note,
       d.consultation_enabled, d.consultation_fee_tokens, d.commission_bps,
       (SELECT avg(rv.rating)::float8 FROM doctor_reviews rv WHERE rv.doctor_id = u.id),
       (SELECT count(*) FROM doctor_reviews rv WHERE rv.doctor_id = u.id)`

const profileSelect = `SELECT ` + profileColumns + `
FROM doctor_profiles d
JOIN users u ON u.id = d.user_id`

func profileDest(p *Profile) []any {
	return []any{&p.UserID, &p.Name, &p.BMDCRegNo, &p.Specialization, &p.Degrees,
		&p.ExperienceYears, &p.Bio, &p.VerificationStatus, &p.VerificationNote,
		&p.ConsultationEnabled, &p.ConsultationFeeTokens, &p.CommissionBps,
		&p.Rating, &p.ReviewCount}
}

func scanProfile(row pgx.Row) (Profile, error) {
	var p Profile
	err := row.Scan(profileDest(&p)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, errDoctorNotFound
	}
	return p, err
}

// SetConsultationSettings turns online consultations on or off and sets the fee in tokens.
// Only verified doctors can turn them on.
func (s *Store) SetConsultationSettings(ctx context.Context, doctorID string, in ConsultationSettingsInput) (Profile, error) {
	if in.FeeTokens < 1 || in.FeeTokens > 20 {
		return Profile{}, apperr.Validation("fee_tokens must be between 1 and 20")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE doctor_profiles SET consultation_enabled = $2, consultation_fee_tokens = $3, updated_at = now()
		WHERE user_id = $1 AND (NOT $2 OR verification_status = 'approved')`,
		doctorID, in.Enabled, in.FeeTokens)
	if err != nil {
		return Profile{}, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.GetProfile(ctx, doctorID); err != nil {
			return Profile{}, err
		}
		return Profile{}, apperr.Conflict("your profile must be verified before you can offer online consultations")
	}
	return s.GetProfile(ctx, doctorID)
}

// SetCommission sets the platform's share of this doctor's consultation fees.
// It applies to consultations requested from now on.
func (s *Store) SetCommission(ctx context.Context, doctorID string, in CommissionInput) (Profile, error) {
	if in.CommissionBps < 0 || in.CommissionBps > 10000 {
		return Profile{}, apperr.Validation("commission_bps must be between 0 and 10000")
	}
	tag, err := s.db.Exec(ctx,
		`UPDATE doctor_profiles SET commission_bps = $2, updated_at = now() WHERE user_id = $1`,
		doctorID, in.CommissionBps)
	if err != nil {
		return Profile{}, err
	}
	if tag.RowsAffected() == 0 {
		return Profile{}, errDoctorNotFound
	}
	return s.GetProfile(ctx, doctorID)
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

// chamberColumns must be used against the bare table name "chambers" (no alias),
// including in INSERT/UPDATE ... RETURNING.
const chamberColumns = `id, doctor_id, name, address, district, area, phone, fee_paisa, booking_enabled,
	hospital_id, (SELECT h.name FROM hospitals h WHERE h.id = chambers.hospital_id), room`

func scanChamber(row pgx.Row) (Chamber, error) {
	var c Chamber
	err := row.Scan(&c.ID, &c.DoctorID, &c.Name, &c.Address, &c.District, &c.Area, &c.Phone,
		&c.FeePaisa, &c.BookingEnabled, &c.HospitalID, &c.HospitalName, &c.Room)
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

// UpdateChamber edits one of the doctor's own chambers. Hospital chambers are
// managed by the hospital and cannot be changed here.
func (s *Store) UpdateChamber(ctx context.Context, doctorID, chamberID string, in ChamberInput) (Chamber, error) {
	if err := in.normalize(); err != nil {
		return Chamber{}, err
	}
	c, err := scanChamber(s.db.QueryRow(ctx, `
		UPDATE chambers
		SET name = $3, address = $4, district = $5, area = $6, phone = $7, fee_paisa = $8,
		    booking_enabled = $9, updated_at = now()
		WHERE id = $1 AND doctor_id = $2 AND hospital_id IS NULL
		RETURNING `+chamberColumns,
		chamberID, doctorID, in.Name, in.Address, in.District, nullIfEmpty(in.Area),
		nullIfEmpty(in.Phone), in.FeePaisa, *in.BookingEnabled))
	if apperr.Is(err, apperr.KindNotFound) {
		return Chamber{}, apperr.NotFound("chamber not found (hospital chambers are managed by the hospital)")
	}
	if err != nil {
		return Chamber{}, err
	}
	if err := s.attachSchedules(ctx, []*Chamber{&c}); err != nil {
		return Chamber{}, err
	}
	return c, nil
}

// LiveChamberSQL is a SQL condition that is true when the chamber aliased as c can
// take bookings: booking is on and, for a hospital chamber, the hospital is verified
// and the doctor is still on its staff.
func LiveChamberSQL(c string) string {
	return `(` + c + `.booking_enabled AND (` + c + `.hospital_id IS NULL OR EXISTS (
		SELECT 1 FROM hospitals lh JOIN hospital_doctors lhd ON lhd.hospital_id = lh.id
		WHERE lh.id = ` + c + `.hospital_id AND lh.verification_status = 'approved'
		  AND lhd.doctor_id = ` + c + `.doctor_id AND lhd.status = 'active')))`
}

// ListChambers returns a doctor's chambers (their own and hospital ones) with active
// schedules. With onlyBookable, chambers that cannot take bookings are left out.
func (s *Store) ListChambers(ctx context.Context, doctorID string, onlyBookable bool) ([]Chamber, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+chamberColumns+` FROM chambers
		WHERE doctor_id = $1 AND (NOT $2 OR `+LiveChamberSQL("chambers")+`)
		ORDER BY created_at, id`, doctorID, onlyBookable)
	if err != nil {
		return nil, err
	}
	return s.collectChambers(ctx, rows)
}

// ListHospitalChambers returns a hospital's chambers, optionally for one doctor
// and optionally only those taking bookings.
func (s *Store) ListHospitalChambers(ctx context.Context, hospitalID, doctorID string, onlyBookable bool) ([]Chamber, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+chamberColumns+` FROM chambers
		WHERE hospital_id = $1 AND ($2 = '' OR doctor_id::text = $2)
		  AND (NOT $3 OR `+LiveChamberSQL("chambers")+`)
		ORDER BY created_at, id`, hospitalID, doctorID, onlyBookable)
	if err != nil {
		return nil, err
	}
	return s.collectChambers(ctx, rows)
}

func (s *Store) collectChambers(ctx context.Context, rows pgx.Rows) ([]Chamber, error) {
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

// ChamberWithSchedules loads one chamber with its active schedules.
func (s *Store) ChamberWithSchedules(ctx context.Context, chamberID string) (Chamber, error) {
	c, err := scanChamber(s.db.QueryRow(ctx, `SELECT `+chamberColumns+` FROM chambers WHERE id = $1`, chamberID))
	if err != nil {
		return Chamber{}, err
	}
	return c, s.attachSchedules(ctx, []*Chamber{&c})
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

// Scope says who is managing a chamber's schedule: the doctor (their own chambers
// only) or a hospital (its chambers only). Exactly one field is set.
type Scope struct {
	DoctorID   string
	HospitalID string
}

func DoctorScope(doctorID string) Scope     { return Scope{DoctorID: doctorID} }
func HospitalScope(hospitalID string) Scope { return Scope{HospitalID: hospitalID} }

// scopeSQL matches chambers (aliased c) within the scope, using parameters $a and $b.
func scopeSQL(a, b string) string {
	return `((` + a + ` <> '' AND c.doctor_id::text = ` + a + ` AND c.hospital_id IS NULL)
	      OR (` + b + ` <> '' AND c.hospital_id::text = ` + b + `))`
}

// AddSchedule adds a weekly session to a chamber in scope. Sessions of the same
// doctor may not overlap on the same weekday, even across different chambers and
// hospitals.
func (s *Store) AddSchedule(ctx context.Context, scope Scope, chamberID string, in ScheduleInput) (Schedule, error) {
	if err := in.normalize(); err != nil {
		return Schedule{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Schedule{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the doctor's profile row so concurrent AddSchedule calls (from the doctor
	// or any hospital) cannot both pass the overlap check.
	var doctorID string
	err = tx.QueryRow(ctx, `
		SELECT c.doctor_id FROM chambers c
		JOIN doctor_profiles d ON d.user_id = c.doctor_id
		WHERE c.id = $1 AND `+scopeSQL("$2", "$3")+`
		FOR UPDATE OF d`, chamberID, scope.DoctorID, scope.HospitalID).Scan(&doctorID)
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
		return Schedule{}, apperr.Conflict("this time overlaps with another session of this doctor on the same day")
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

// RemoveSchedule deactivates a session in scope. Existing appointments for it stay valid.
func (s *Store) RemoveSchedule(ctx context.Context, scope Scope, scheduleID string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE chamber_schedules s SET active = false
		FROM chambers c
		WHERE s.id = $1 AND c.id = s.chamber_id AND s.active AND `+scopeSQL("$2", "$3"),
		scheduleID, scope.DoctorID, scope.HospitalID)
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
	OnlineOnly     bool   // only doctors taking online consultations
	Sort           string // "" (by name), "rating" or "fee"
	Limit, Offset  int
}

// searchOrder maps SearchParams.Sort to an ORDER BY clause.
var searchOrder = map[string]string{
	"":       "u.name, u.id",
	"rating": "rating DESC NULLS LAST, review_count DESC, u.name, u.id",
	"fee":    "MIN(c.fee_paisa) NULLS LAST, u.name, u.id",
}

// Search lists approved, active doctors.
func (s *Store) Search(ctx context.Context, p SearchParams) ([]Summary, error) {
	order, ok := searchOrder[p.Sort]
	if !ok {
		return nil, apperr.Validation("sort must be rating or fee")
	}
	rows, err := s.db.Query(ctx, `
		SELECT u.id, u.name, d.specialization, d.degrees, d.experience_years,
		       COALESCE(array_agg(DISTINCT c.district) FILTER (WHERE c.id IS NOT NULL), '{}')::text[],
		       MIN(c.fee_paisa), d.consultation_enabled, d.consultation_fee_tokens,
		       (SELECT avg(rv.rating)::float8 FROM doctor_reviews rv WHERE rv.doctor_id = u.id) AS rating,
		       (SELECT count(*) FROM doctor_reviews rv WHERE rv.doctor_id = u.id) AS review_count
		FROM doctor_profiles d
		JOIN users u ON u.id = d.user_id
		LEFT JOIN chambers c ON c.doctor_id = d.user_id AND `+LiveChamberSQL("c")+`
		WHERE d.verification_status = 'approved' AND u.status = 'active'
		  AND (NOT $6 OR d.consultation_enabled)
		  AND ($1 = '' OR u.name ILIKE $1)
		  AND ($2 = '' OR d.specialization ILIKE $2)
		  AND ($3 = '' OR EXISTS (
		        SELECT 1 FROM chambers c2
		        WHERE c2.doctor_id = d.user_id AND `+LiveChamberSQL("c2")+`
		          AND (c2.district ILIKE $3 OR c2.area ILIKE $3)))
		GROUP BY u.id, u.name, d.specialization, d.degrees, d.experience_years,
		         d.consultation_enabled, d.consultation_fee_tokens
		ORDER BY `+order+`
		LIMIT $4 OFFSET $5`,
		containsPattern(p.Name), containsPattern(p.Specialization), containsPattern(p.Location),
		p.Limit, p.Offset, p.OnlineOnly)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Summary, error) {
		var sm Summary
		err := r.Scan(&sm.ID, &sm.Name, &sm.Specialization, &sm.Degrees, &sm.ExperienceYears,
			&sm.Districts, &sm.MinFeePaisa, &sm.ConsultationEnabled, &sm.ConsultationFeeTokens,
			&sm.Rating, &sm.ReviewCount)
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
	p.CommissionBps = nil
	chambers, err := s.ListChambers(ctx, doctorID, true)
	if err != nil {
		return PublicDoctor{}, err
	}
	reviews, err := s.DoctorReviews(ctx, doctorID, maxPublicReviews, 0)
	if err != nil {
		return PublicDoctor{}, err
	}
	return PublicDoctor{Profile: p, Chambers: chambers, Reviews: reviews}, nil
}

// ---------------------------------------------------------------- admin

func (s *Store) ListForAdmin(ctx context.Context, status string, limit, offset int) ([]AdminDoctor, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+profileColumns+`, u.phone, u.created_at
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
		err := r.Scan(append(profileDest(&a.Profile), &a.Phone, &a.CreatedAt)...)
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
	msg := notify.Msg{UserID: doctorID, Kind: notify.KindDoctorVerification, SMS: true,
		Title: "Profile verified",
		Body:  "Your doctor profile has been verified. You can now accept bookings and online consultations."}
	if in.Status == StatusRejected {
		msg.Title = "Profile verification unsuccessful"
		msg.Body = "Your doctor profile could not be verified: " + in.Note + ". Please update your profile and resubmit."
	}
	if err := notify.Enqueue(ctx, tx, msg); err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Profile{}, err
	}
	return s.GetProfile(ctx, doctorID)
}
