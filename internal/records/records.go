// Package records keeps a patient's own health records (lab reports, imaging,
// old prescriptions) for themself and their family members, and shares them
// with doctors: with the doctor of a chamber appointment, or as a copy sent
// into a consultation chat.
package records

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/family"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/storage"
)

const (
	MaxFileBytes = 10 << 20 // 10 MB
	// MaxRecords caps how many records one account keeps.
	MaxRecords = 300
)

var Kinds = map[string]bool{
	"lab_report": true, "imaging": true, "prescription": true, "discharge": true, "vaccination": true, "other": true,
}

type Record struct {
	ID             string    `json:"id"`
	FamilyMemberID *string   `json:"family_member_id"` // null: the account holder
	PersonName     string    `json:"person_name"`
	Title          string    `json:"title"`
	Kind           string    `json:"kind"`
	RecordDate     *string   `json:"record_date"` // YYYY-MM-DD
	Note           *string   `json:"note"`
	FileName       string    `json:"file_name"`
	ContentType    string    `json:"content_type"`
	SizeBytes      int64     `json:"size_bytes"`
	CreatedAt      time.Time `json:"created_at"`
	SharedWith     []Share   `json:"shared_with"` // chamber appointments whose doctor can see it
}

// Share is a chamber appointment whose doctor can see the record.
type Share struct {
	AppointmentID string `json:"appointment_id"`
	DoctorName    string `json:"doctor_name"`
	Date          string `json:"date"`
}

// Meta is what the patient says about a record.
type Meta struct {
	Title          string `json:"title"`
	Kind           string `json:"kind"`
	RecordDate     string `json:"record_date"`
	Note           string `json:"note"`
	FamilyMemberID string `json:"family_member_id"`
}

func (m *Meta) normalize(today time.Time) error {
	m.Title = strings.Join(strings.Fields(m.Title), " ")
	m.Kind = strings.TrimSpace(m.Kind)
	m.RecordDate = strings.TrimSpace(m.RecordDate)
	m.Note = strings.TrimSpace(m.Note)
	m.FamilyMemberID = strings.TrimSpace(m.FamilyMemberID)
	if m.Kind == "" {
		m.Kind = "other"
	}
	switch {
	case utf8.RuneCountInString(m.Title) < 2 || utf8.RuneCountInString(m.Title) > 150:
		return apperr.Validation("title must be 2-150 characters")
	case !Kinds[m.Kind]:
		return apperr.Validation("kind must be lab_report, imaging, prescription, discharge, vaccination or other")
	case utf8.RuneCountInString(m.Note) > 2000:
		return apperr.Validation("note must be at most 2000 characters")
	case m.FamilyMemberID != "" && uuid.Validate(m.FamilyMemberID) != nil:
		return apperr.Validation("family_member_id must be a UUID")
	}
	if m.RecordDate != "" {
		d, err := time.Parse(time.DateOnly, m.RecordDate)
		if err != nil {
			return apperr.Validation("record_date must be YYYY-MM-DD")
		}
		if d.After(today) {
			return apperr.Validation("record_date cannot be in the future")
		}
	}
	return nil
}

type Store struct {
	db    *pgxpool.Pool
	files storage.Storage
	now   func() time.Time
}

func NewStore(db *pgxpool.Pool, files storage.Storage) *Store {
	return &Store{db: db, files: files, now: time.Now}
}

var errNotFound = apperr.NotFound("record not found")

func (s *Store) today() time.Time { return s.now().UTC().Add(6 * time.Hour) }

const recordSelect = `
SELECT r.id, r.family_member_id, COALESCE(fm.name, u.name), r.title, r.kind,
       to_char(r.record_date, 'YYYY-MM-DD'), r.note, r.file_name, r.content_type, r.size_bytes, r.created_at,
       COALESCE((
         SELECT json_agg(json_build_object('appointment_id', a.id, 'doctor_name', du.name,
                                           'date', to_char(a.appointment_date, 'YYYY-MM-DD'))
                         ORDER BY a.appointment_date DESC)
         FROM record_shares rs
         JOIN appointments a ON a.id = rs.appointment_id
         JOIN users du ON du.id = a.doctor_id
         WHERE rs.record_id = r.id), '[]')
FROM health_records r
JOIN users u ON u.id = r.owner_id
LEFT JOIN family_members fm ON fm.id = r.family_member_id`

func scan(row pgx.Row) (Record, error) {
	var r Record
	err := row.Scan(&r.ID, &r.FamilyMemberID, &r.PersonName, &r.Title, &r.Kind, &r.RecordDate, &r.Note,
		&r.FileName, &r.ContentType, &r.SizeBytes, &r.CreatedAt, &r.SharedWith)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, errNotFound
	}
	return r, err
}

func collect(rows pgx.Rows, err error) ([]Record, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Record, error) { return scan(r) })
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// List returns the owner's records, newest first. person filters them: ""
// for everyone, "self" for the account holder, or a family member's id.
func (s *Store) List(ctx context.Context, ownerID, person string) ([]Record, error) {
	switch {
	case person == "":
		return collect(s.db.Query(ctx, recordSelect+` WHERE r.owner_id = $1
			ORDER BY r.record_date DESC NULLS LAST, r.created_at DESC`, ownerID))
	case person == "self":
		return collect(s.db.Query(ctx, recordSelect+` WHERE r.owner_id = $1 AND r.family_member_id IS NULL
			ORDER BY r.record_date DESC NULLS LAST, r.created_at DESC`, ownerID))
	case uuid.Validate(person) == nil:
		return collect(s.db.Query(ctx, recordSelect+` WHERE r.owner_id = $1 AND r.family_member_id = $2
			ORDER BY r.record_date DESC NULLS LAST, r.created_at DESC`, ownerID, person))
	default:
		return nil, apperr.Validation("person must be self or a family member id")
	}
}

func (s *Store) get(ctx context.Context, ownerID, id string) (Record, error) {
	return scan(s.db.QueryRow(ctx, recordSelect+` WHERE r.id = $1 AND r.owner_id = $2`, id, ownerID))
}

// Add stores a new record. The file is written first and removed again if
// the database insert fails.
func (s *Store) Add(ctx context.Context, ownerID string, m Meta, fileName, contentType string, file io.Reader) (Record, error) {
	if err := m.normalize(s.today()); err != nil {
		return Record{}, err
	}
	if err := family.Check(ctx, s.db, ownerID, m.FamilyMemberID); err != nil {
		return Record{}, err
	}
	var n int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM health_records WHERE owner_id = $1`, ownerID).Scan(&n); err != nil {
		return Record{}, err
	}
	if n >= MaxRecords {
		return Record{}, apperr.Conflict("you can keep at most %d records; delete old ones first", MaxRecords)
	}

	key := uuid.NewString()
	size, err := s.files.Put(ctx, key, file)
	if err != nil {
		_ = s.files.Delete(ctx, key)
		return Record{}, err
	}
	if size == 0 {
		_ = s.files.Delete(ctx, key)
		return Record{}, apperr.Validation("file is empty")
	}
	var id string
	err = s.db.QueryRow(ctx, `
		INSERT INTO health_records (owner_id, family_member_id, title, kind, record_date, note,
		                            file_name, content_type, size_bytes, storage_key)
		VALUES ($1, $2, $3, $4, $5::date, $6, $7, $8, $9, $10) RETURNING id`,
		ownerID, nullIfEmpty(m.FamilyMemberID), m.Title, m.Kind, nullIfEmpty(m.RecordDate), nullIfEmpty(m.Note),
		fileName, contentType, size, key).Scan(&id)
	if err != nil {
		_ = s.files.Delete(ctx, key)
		return Record{}, err
	}
	return s.get(ctx, ownerID, id)
}

func (s *Store) Update(ctx context.Context, ownerID, id string, m Meta) (Record, error) {
	if err := m.normalize(s.today()); err != nil {
		return Record{}, err
	}
	if err := family.Check(ctx, s.db, ownerID, m.FamilyMemberID); err != nil {
		return Record{}, err
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE health_records SET family_member_id = $3, title = $4, kind = $5, record_date = $6::date, note = $7, updated_at = now()
		WHERE id = $1 AND owner_id = $2`,
		id, ownerID, nullIfEmpty(m.FamilyMemberID), m.Title, m.Kind, nullIfEmpty(m.RecordDate), nullIfEmpty(m.Note))
	if err != nil {
		return Record{}, err
	}
	if tag.RowsAffected() == 0 {
		return Record{}, errNotFound
	}
	return s.get(ctx, ownerID, id)
}

// Delete removes a record, its shares and its file. Copies already sent into
// a consultation chat are separate files and stay.
func (s *Store) Delete(ctx context.Context, ownerID, id string) error {
	var key string
	err := s.db.QueryRow(ctx, `DELETE FROM health_records WHERE id = $1 AND owner_id = $2 RETURNING storage_key`,
		id, ownerID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	_ = s.files.Delete(ctx, key) // the row is gone; a leftover file is unreachable
	return nil
}

// File is a record's file and how to serve it.
type File struct {
	FileName    string
	ContentType string
	SizeBytes   int64
}

// Open returns the record's file if userID owns the record, or is the doctor
// of a live appointment it was shared with.
func (s *Store) Open(ctx context.Context, userID, id string) (File, io.ReadCloser, error) {
	var f File
	var key string
	err := s.db.QueryRow(ctx, `
		SELECT r.file_name, r.content_type, r.size_bytes, r.storage_key
		FROM health_records r
		WHERE r.id = $1 AND (r.owner_id = $2 OR EXISTS (
			SELECT 1 FROM record_shares rs JOIN appointments a ON a.id = rs.appointment_id
			WHERE rs.record_id = r.id AND a.doctor_id = $2 AND a.status IN ('confirmed', 'completed')))`,
		id, userID).Scan(&f.FileName, &f.ContentType, &f.SizeBytes, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, nil, errNotFound
	}
	if err != nil {
		return File{}, nil, err
	}
	rc, err := s.files.Open(ctx, key)
	if err != nil {
		return File{}, nil, err
	}
	return f, rc, nil
}

// ShareWithAppointment lets the doctor of one of the owner's confirmed or
// completed appointments see the record.
func (s *Store) ShareWithAppointment(ctx context.Context, ownerID, id, appointmentID string) (Record, error) {
	tag, err := s.db.Exec(ctx, `
		INSERT INTO record_shares (record_id, appointment_id)
		SELECT r.id, a.id FROM health_records r, appointments a
		WHERE r.id = $1 AND r.owner_id = $2 AND a.id = $3 AND a.patient_id = $2
		  AND a.status IN ('confirmed', 'completed')
		ON CONFLICT DO NOTHING`, id, ownerID, appointmentID)
	if err != nil {
		return Record{}, err
	}
	if tag.RowsAffected() == 0 {
		r, err := s.get(ctx, ownerID, id)
		if err != nil {
			return Record{}, err
		}
		for _, sh := range r.SharedWith {
			if sh.AppointmentID == appointmentID {
				return r, nil // already shared
			}
		}
		return Record{}, apperr.Conflict("records can be shared only with your confirmed or completed appointments")
	}
	return s.get(ctx, ownerID, id)
}

func (s *Store) Unshare(ctx context.Context, ownerID, id, appointmentID string) (Record, error) {
	if _, err := s.db.Exec(ctx, `
		DELETE FROM record_shares rs USING health_records r
		WHERE rs.record_id = r.id AND r.id = $1 AND r.owner_id = $2 AND rs.appointment_id = $3`,
		id, ownerID, appointmentID); err != nil {
		return Record{}, err
	}
	return s.get(ctx, ownerID, id)
}

// SendToConsultation posts a copy of the record's file into an active
// consultation chat, as if the patient had uploaded it there. It returns the
// chat message id.
func (s *Store) SendToConsultation(ctx context.Context, ownerID, id, consultationID string) (int64, error) {
	var status string
	err := s.db.QueryRow(ctx, `SELECT status FROM consultations WHERE id = $1 AND patient_id = $2`,
		consultationID, ownerID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, apperr.NotFound("consultation not found")
	}
	if err != nil {
		return 0, err
	}
	if status != "active" {
		return 0, apperr.Conflict("files can only be shared while the consultation is active (it is %s)", status)
	}

	f, rc, err := s.Open(ctx, ownerID, id)
	if err != nil {
		return 0, err
	}
	key := uuid.NewString()
	_, err = s.files.Put(ctx, key, rc)
	rc.Close()
	if err != nil {
		_ = s.files.Delete(ctx, key)
		return 0, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		_ = s.files.Delete(ctx, key)
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var attID string
	err = tx.QueryRow(ctx, `
		INSERT INTO attachments (consultation_id, uploader_id, file_name, content_type, size_bytes, storage_key)
		SELECT id, $2, $3, $4, $5, $6 FROM consultations WHERE id = $1 AND patient_id = $2 AND status = 'active'
		RETURNING id`, consultationID, ownerID, f.FileName, f.ContentType, f.SizeBytes, key).Scan(&attID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = apperr.Conflict("the consultation is no longer active")
	}
	var msgID int64
	if err == nil {
		err = tx.QueryRow(ctx, `INSERT INTO consultation_messages (consultation_id, sender_id, attachment_id)
			VALUES ($1, $2, $3) RETURNING id`, consultationID, ownerID, attID).Scan(&msgID)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		_ = s.files.Delete(ctx, key)
		return 0, err
	}
	return msgID, nil
}

// ForAppointment lists the records shared with an appointment, for its doctor.
func (s *Store) ForAppointment(ctx context.Context, doctorID, appointmentID string) ([]Record, error) {
	var ok bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM appointments WHERE id = $1 AND doctor_id = $2)`,
		appointmentID, doctorID).Scan(&ok); err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.NotFound("appointment not found")
	}
	rs, err := collect(s.db.Query(ctx, recordSelect+`
		JOIN record_shares sh ON sh.record_id = r.id
		JOIN appointments ap ON ap.id = sh.appointment_id
		WHERE sh.appointment_id = $1 AND ap.status IN ('confirmed', 'completed')
		ORDER BY r.record_date DESC NULLS LAST, r.created_at DESC`, appointmentID))
	for i := range rs {
		rs[i].SharedWith = nil // the doctor doesn't need the patient's other doctors
	}
	return rs, err
}
