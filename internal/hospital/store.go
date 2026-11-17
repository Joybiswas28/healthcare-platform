package hospital

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
	"healthcare-platform/internal/platform/storage"
	"healthcare-platform/internal/user"
)

type Store struct {
	db      *pgxpool.Pool
	doctors *doctor.Store
	files   storage.Storage
	now     func() time.Time
}

func NewStore(db *pgxpool.Pool, doctors *doctor.Store, files storage.Storage) *Store {
	return &Store{db: db, doctors: doctors, files: files, now: time.Now}
}

var errHospitalNotFound = apperr.NotFound("hospital not found")

func photoURL(hospitalID, photoID string) string {
	return "/api/v1/hospitals/" + hospitalID + "/photos/" + photoID
}

const hospitalColumns = `
h.id, h.name, h.registration_no, h.description, h.address, h.district, h.area, h.phone, h.email, h.website,
h.departments, h.facilities, h.visiting_hours, h.latitude::float8, h.longitude::float8, h.logo_photo_id,
h.verification_status, h.verification_note,
(SELECT count(*) FROM hospital_doctors hd WHERE hd.hospital_id = h.id AND hd.status = 'active'),
h.created_at, h.verified_at`

func scanHospital(row pgx.Row, extra ...any) (Hospital, error) {
	var h Hospital
	var logo *string
	dest := []any{&h.ID, &h.Name, &h.RegistrationNo, &h.Description, &h.Address, &h.District, &h.Area,
		&h.Phone, &h.Email, &h.Website, &h.Departments, &h.Facilities, &h.VisitingHours,
		&h.Latitude, &h.Longitude, &logo, &h.VerificationStatus, &h.VerificationNote,
		&h.DoctorCount, &h.CreatedAt, &h.VerifiedAt}
	err := row.Scan(append(dest, extra...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Hospital{}, errHospitalNotFound
	}
	if err != nil {
		return Hospital{}, err
	}
	if logo != nil {
		u := photoURL(h.ID, *logo)
		h.LogoURL = &u
	}
	if h.Latitude != nil {
		u := fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.6f,%.6f", *h.Latitude, *h.Longitude)
		h.MapURL = &u
	}
	return h, nil
}

func collectHospitals(rows pgx.Rows, err error) ([]Hospital, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Hospital, error) { return scanHospital(r) })
}

// ---------------------------------------------------------------- membership

// Role returns the user's role at the hospital, or NotFound if they are not a member.
func (s *Store) Role(ctx context.Context, userID, hospitalID string) (string, error) {
	var role string
	err := s.db.QueryRow(ctx, `SELECT role FROM hospital_members WHERE hospital_id = $1 AND user_id = $2`,
		hospitalID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errHospitalNotFound
	}
	return role, err
}

// ---------------------------------------------------------------- create / edit

// Create registers a hospital; the creator becomes its owner. It stays hidden until
// a platform admin verifies it.
func (s *Store) Create(ctx context.Context, userID string, in HospitalInput) (Hospital, error) {
	if err := in.normalize(); err != nil {
		return Hospital{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Hospital{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO hospitals (name, registration_no, description, address, district, area, phone, email, website,
		                       departments, facilities, visiting_hours, latitude, longitude, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id`,
		in.Name, in.RegistrationNo, nullIfEmpty(in.Description), in.Address, in.District, nullIfEmpty(in.Area),
		in.Phone, nullIfEmpty(in.Email), nullIfEmpty(in.Website), in.Departments, in.Facilities,
		nullIfEmpty(in.VisitingHours), in.Latitude, in.Longitude, userID).Scan(&id)
	if database.IsUniqueViolation(err) {
		return Hospital{}, apperr.Conflict("a hospital with this registration number already exists")
	}
	if err != nil {
		return Hospital{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO hospital_members (hospital_id, user_id, role) VALUES ($1, $2, 'owner')`, id, userID); err != nil {
		return Hospital{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Hospital{}, err
	}
	return s.GetForMember(ctx, userID, id)
}

func (s *Store) GetForMember(ctx context.Context, userID, hospitalID string) (Hospital, error) {
	var role string
	h, err := scanHospital(s.db.QueryRow(ctx, `SELECT `+hospitalColumns+`, m.role
		FROM hospitals h JOIN hospital_members m ON m.hospital_id = h.id AND m.user_id = $2
		WHERE h.id = $1`, hospitalID, userID), &role)
	h.MyRole = role
	return h, err
}

// ListMine lists the hospitals the user helps manage.
func (s *Store) ListMine(ctx context.Context, userID string) ([]Hospital, error) {
	rows, err := s.db.Query(ctx, `SELECT `+hospitalColumns+`, m.role
		FROM hospitals h JOIN hospital_members m ON m.hospital_id = h.id
		WHERE m.user_id = $1 ORDER BY h.name`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Hospital, error) {
		var role string
		h, err := scanHospital(r, &role)
		h.MyRole = role
		return h, err
	})
}

// Update edits the hospital profile. A verified hospital cannot change its
// registration number; a rejected one goes back to pending when it resubmits.
// The hospital's chambers pick up the new name and address.
func (s *Store) Update(ctx context.Context, userID, hospitalID string, in HospitalInput) (Hospital, error) {
	if err := in.normalize(); err != nil {
		return Hospital{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Hospital{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var regNo, status string
	err = tx.QueryRow(ctx, `SELECT registration_no, verification_status FROM hospitals WHERE id = $1 FOR UPDATE`,
		hospitalID).Scan(&regNo, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Hospital{}, errHospitalNotFound
	}
	if err != nil {
		return Hospital{}, err
	}
	if status == StatusApproved && regNo != in.RegistrationNo {
		return Hospital{}, apperr.Validation("a verified hospital cannot change its registration number; contact support")
	}
	if status == StatusRejected {
		status = StatusPending
	}

	_, err = tx.Exec(ctx, `
		UPDATE hospitals SET name = $2, registration_no = $3, description = $4, address = $5, district = $6,
		       area = $7, phone = $8, email = $9, website = $10, departments = $11, facilities = $12,
		       visiting_hours = $13, latitude = $14, longitude = $15, verification_status = $16, updated_at = now()
		WHERE id = $1`,
		hospitalID, in.Name, in.RegistrationNo, nullIfEmpty(in.Description), in.Address, in.District,
		nullIfEmpty(in.Area), in.Phone, nullIfEmpty(in.Email), nullIfEmpty(in.Website), in.Departments,
		in.Facilities, nullIfEmpty(in.VisitingHours), in.Latitude, in.Longitude, status)
	if database.IsUniqueViolation(err) {
		return Hospital{}, apperr.Conflict("a hospital with this registration number already exists")
	}
	if err != nil {
		return Hospital{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE chambers SET name = $2, address = $3, district = $4, area = $5, phone = $6, updated_at = now()
		WHERE hospital_id = $1`,
		hospitalID, in.Name, in.Address, in.District, nullIfEmpty(in.Area), in.Phone); err != nil {
		return Hospital{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Hospital{}, err
	}
	return s.GetForMember(ctx, userID, hospitalID)
}

// ---------------------------------------------------------------- public

type SearchParams struct {
	Name       string
	Location   string // district or area
	Department string
	Facility   string
	Limit      int
	Offset     int
}

func pattern(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s) + "%"
}

// Search lists verified hospitals.
func (s *Store) Search(ctx context.Context, p SearchParams) ([]Hospital, error) {
	return collectHospitals(s.db.Query(ctx, `SELECT `+hospitalColumns+` FROM hospitals h
		WHERE h.verification_status = 'approved'
		  AND ($1 = '' OR h.name ILIKE $1)
		  AND ($2 = '' OR h.district ILIKE $2 OR h.area ILIKE $2)
		  AND ($3 = '' OR EXISTS (SELECT 1 FROM unnest(h.departments) d WHERE d ILIKE $3))
		  AND ($4 = '' OR EXISTS (SELECT 1 FROM unnest(h.facilities) f WHERE f ILIKE $4))
		ORDER BY h.name, h.id
		LIMIT $5 OFFSET $6`,
		pattern(p.Name), pattern(p.Location), pattern(p.Department), pattern(p.Facility), p.Limit, p.Offset))
}

// Public returns a verified hospital's page: profile, photos, and its doctors with
// the chambers and schedules patients can book.
func (s *Store) Public(ctx context.Context, hospitalID string) (PublicHospital, error) {
	h, err := scanHospital(s.db.QueryRow(ctx, `SELECT `+hospitalColumns+` FROM hospitals h
		WHERE h.id = $1 AND h.verification_status = 'approved'`, hospitalID))
	if err != nil {
		return PublicHospital{}, err
	}
	h.VerificationNote = nil
	photos, err := s.ListPhotos(ctx, hospitalID)
	if err != nil {
		return PublicHospital{}, err
	}
	staff, err := s.staff(ctx, hospitalID, true)
	if err != nil {
		return PublicHospital{}, err
	}
	for i := range staff {
		staff[i].Chambers, err = s.doctors.ListHospitalChambers(ctx, hospitalID, staff[i].DoctorID, true)
		if err != nil {
			return PublicHospital{}, err
		}
	}
	return PublicHospital{Hospital: h, Photos: photos, Doctors: staff}, nil
}

// ---------------------------------------------------------------- admin

func (s *Store) AdminList(ctx context.Context, status string, limit, offset int) ([]Hospital, error) {
	return collectHospitals(s.db.Query(ctx, `SELECT `+hospitalColumns+` FROM hospitals h
		WHERE ($1 = '' OR h.verification_status = $1)
		ORDER BY h.created_at, h.id LIMIT $2 OFFSET $3`, status, limit, offset))
}

// Verify approves or rejects a hospital (rejection needs a note) and tells its managers.
func (s *Store) Verify(ctx context.Context, hospitalID string, in VerifyInput) (Hospital, error) {
	in.Note = strings.TrimSpace(in.Note)
	switch in.Status {
	case StatusApproved:
	case StatusRejected:
		if in.Note == "" {
			return Hospital{}, apperr.Validation("note is required when rejecting a hospital")
		}
	default:
		return Hospital{}, apperr.Validation("status must be 'approved' or 'rejected'")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Hospital{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var name string
	err = tx.QueryRow(ctx, `
		UPDATE hospitals SET verification_status = $2, verification_note = $3,
		       verified_at = CASE WHEN $2 = 'approved' THEN now() END, updated_at = now()
		WHERE id = $1 RETURNING name`, hospitalID, in.Status, nullIfEmpty(in.Note)).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Hospital{}, errHospitalNotFound
	}
	if err != nil {
		return Hospital{}, err
	}

	body := name + " has been verified and is now visible to patients."
	title := "Hospital verified"
	if in.Status == StatusRejected {
		title = "Hospital verification unsuccessful"
		body = name + " could not be verified: " + in.Note + ". Please update the details and resubmit."
	}
	if err := s.notifyMembers(ctx, tx, hospitalID, notify.KindHospitalVerification, title, body, true); err != nil {
		return Hospital{}, err
	}
	h, err := scanHospital(tx.QueryRow(ctx, `SELECT `+hospitalColumns+` FROM hospitals h WHERE h.id = $1`, hospitalID))
	if err != nil {
		return Hospital{}, err
	}
	return h, tx.Commit(ctx)
}

func (s *Store) notifyMembers(ctx context.Context, q database.Querier, hospitalID, kind, title, body string, sms bool) error {
	rows, err := q.Query(ctx, `SELECT user_id FROM hospital_members WHERE hospital_id = $1`, hospitalID)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := notify.Enqueue(ctx, q, notify.Msg{UserID: id, Kind: kind, Title: title, Body: body, SMS: sms}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- members

func (s *Store) Members(ctx context.Context, hospitalID string) ([]Member, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.id, u.name, u.phone, m.role, m.created_at
		FROM hospital_members m JOIN users u ON u.id = m.user_id
		WHERE m.hospital_id = $1 ORDER BY m.role, u.name`, hospitalID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Member])
}

// AddMember gives another hospital-admin account access to the hospital as staff
// (e.g. a receptionist who manages bookings).
func (s *Store) AddMember(ctx context.Context, hospitalID, phone string) (Member, error) {
	var userID string
	var role user.Role
	err := s.db.QueryRow(ctx, `SELECT id, role FROM users WHERE phone = $1 AND status = 'active'`, phone).Scan(&userID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, apperr.NotFound("no account with this phone number; ask them to register as a hospital admin first")
	}
	if err != nil {
		return Member{}, err
	}
	if role != user.RoleHospitalAdmin {
		return Member{}, apperr.Validation("only hospital admin accounts can be added as hospital staff")
	}
	_, err = s.db.Exec(ctx,
		`INSERT INTO hospital_members (hospital_id, user_id, role) VALUES ($1, $2, 'staff')`, hospitalID, userID)
	if database.IsUniqueViolation(err) {
		return Member{}, apperr.Conflict("this person already manages the hospital")
	}
	if err != nil {
		return Member{}, err
	}
	ms, err := s.Members(ctx, hospitalID)
	if err != nil {
		return Member{}, err
	}
	for _, m := range ms {
		if m.UserID == userID {
			return m, nil
		}
	}
	return Member{}, errors.New("member vanished")
}

// RemoveMember removes a staff member. Owners cannot be removed this way.
func (s *Store) RemoveMember(ctx context.Context, hospitalID, userID string) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM hospital_members WHERE hospital_id = $1 AND user_id = $2 AND role = 'staff'`, hospitalID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("staff member not found (owners cannot be removed)")
	}
	return nil
}
