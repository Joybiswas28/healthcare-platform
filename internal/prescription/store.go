// Package prescription issues digital prescriptions.
//
// A doctor issues a prescription for an online consultation or a chamber
// appointment they handled. Doctor and patient details are copied in at issue time.
// Prescriptions are never edited; a wrong one is revoked and a new one issued.
// Each has a short public code that pharmacies use to check it is genuine.
package prescription

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

const (
	StatusIssued  = "issued"
	StatusRevoked = "revoked"
)

// Dhaka is used for "today" when validating follow-up dates.
var Dhaka = time.FixedZone("Asia/Dhaka", 6*60*60)

type Medicine struct {
	Name         string `json:"name"`         // e.g. "Napa"
	Strength     string `json:"strength"`     // e.g. "500 mg"
	Form         string `json:"form"`         // e.g. "tablet", "syrup"
	Dosage       string `json:"dosage"`       // e.g. "1+0+1"
	Duration     string `json:"duration"`     // e.g. "5 days"
	Instructions string `json:"instructions"` // e.g. "after meal"
}

type Prescription struct {
	ID             string  `json:"id"`
	Code           string  `json:"code"`
	DoctorID       string  `json:"doctor_id"`
	PatientID      string  `json:"patient_id"`
	ConsultationID *string `json:"consultation_id"`
	AppointmentID  *string `json:"appointment_id"`

	DoctorName           string  `json:"doctor_name"`
	DoctorDegrees        *string `json:"doctor_degrees"`
	DoctorSpecialization *string `json:"doctor_specialization"`
	DoctorBMDC           string  `json:"doctor_bmdc"`
	PatientName          string  `json:"patient_name"`
	PatientAge           *string `json:"patient_age"`
	PatientGender        *string `json:"patient_gender"`

	ChiefComplaints *string    `json:"chief_complaints"`
	Diagnosis       *string    `json:"diagnosis"`
	Medicines       []Medicine `json:"medicines"`
	Investigations  []string   `json:"investigations"`
	Advice          *string    `json:"advice"`
	FollowUpDate    *string    `json:"follow_up_date"`

	Status        string     `json:"status"`
	RevokedReason *string    `json:"revoked_reason"`
	IssuedAt      time.Time  `json:"issued_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
}

// Verification is what anyone holding the code (e.g. a pharmacist) may see.
// The patient's name is masked.
type Verification struct {
	Code                 string     `json:"code"`
	Status               string     `json:"status"`
	IssuedAt             time.Time  `json:"issued_at"`
	DoctorName           string     `json:"doctor_name"`
	DoctorBMDC           string     `json:"doctor_bmdc"`
	DoctorSpecialization *string    `json:"doctor_specialization"`
	PatientName          string     `json:"patient_name"`
	Medicines            []Medicine `json:"medicines"`
}

type IssueInput struct {
	ConsultationID  string     `json:"consultation_id"`
	AppointmentID   string     `json:"appointment_id"`
	PatientAge      string     `json:"patient_age"`
	PatientGender   string     `json:"patient_gender"`
	ChiefComplaints string     `json:"chief_complaints"`
	Diagnosis       string     `json:"diagnosis"`
	Medicines       []Medicine `json:"medicines"`
	Investigations  []string   `json:"investigations"`
	Advice          string     `json:"advice"`
	FollowUpDate    string     `json:"follow_up_date"` // YYYY-MM-DD, optional
}

type Store struct {
	db  *pgxpool.Pool
	now func() time.Time
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db, now: time.Now} }

// ---------------------------------------------------------------- validation

func clean(s string) string { return strings.TrimSpace(s) }

func tooLong(s string, max int) bool { return utf8.RuneCountInString(s) > max }

func (s *Store) validate(in *IssueInput) error {
	in.ConsultationID, in.AppointmentID = clean(in.ConsultationID), clean(in.AppointmentID)
	if (in.ConsultationID == "") == (in.AppointmentID == "") {
		return apperr.Validation("give exactly one of consultation_id or appointment_id")
	}
	in.PatientAge = clean(in.PatientAge)
	in.PatientGender = strings.ToLower(clean(in.PatientGender))
	in.ChiefComplaints, in.Diagnosis, in.Advice = clean(in.ChiefComplaints), clean(in.Diagnosis), clean(in.Advice)
	in.FollowUpDate = clean(in.FollowUpDate)

	switch {
	case tooLong(in.PatientAge, 20):
		return apperr.Validation("patient_age must be at most 20 characters (e.g. \"34 years\", \"8 months\")")
	case in.PatientGender != "" && in.PatientGender != "male" && in.PatientGender != "female" && in.PatientGender != "other":
		return apperr.Validation("patient_gender must be male, female or other")
	case in.ChiefComplaints == "" && in.Diagnosis == "":
		return apperr.Validation("give chief_complaints or diagnosis")
	case tooLong(in.ChiefComplaints, 1000), tooLong(in.Diagnosis, 1000):
		return apperr.Validation("chief_complaints and diagnosis must be at most 1000 characters")
	case tooLong(in.Advice, 2000):
		return apperr.Validation("advice must be at most 2000 characters")
	case len(in.Medicines) == 0 && len(in.Investigations) == 0 && in.Advice == "":
		return apperr.Validation("a prescription needs at least one medicine, investigation or advice")
	case len(in.Medicines) > 30:
		return apperr.Validation("at most 30 medicines")
	case len(in.Investigations) > 20:
		return apperr.Validation("at most 20 investigations")
	}

	for i := range in.Medicines {
		m := &in.Medicines[i]
		m.Name, m.Strength, m.Form = clean(m.Name), clean(m.Strength), clean(m.Form)
		m.Dosage, m.Duration, m.Instructions = clean(m.Dosage), clean(m.Duration), clean(m.Instructions)
		switch {
		case utf8.RuneCountInString(m.Name) < 2 || tooLong(m.Name, 100):
			return apperr.Validation("medicine %d: name must be 2-100 characters", i+1)
		case m.Dosage == "" || tooLong(m.Dosage, 50):
			return apperr.Validation("medicine %d: dosage is required (e.g. 1+0+1), at most 50 characters", i+1)
		case m.Duration == "" || tooLong(m.Duration, 50):
			return apperr.Validation("medicine %d: duration is required (e.g. 7 days), at most 50 characters", i+1)
		case tooLong(m.Strength, 50), tooLong(m.Form, 30), tooLong(m.Instructions, 200):
			return apperr.Validation("medicine %d: strength, form or instructions too long", i+1)
		}
	}
	for i := range in.Investigations {
		in.Investigations[i] = clean(in.Investigations[i])
		if in.Investigations[i] == "" || tooLong(in.Investigations[i], 100) {
			return apperr.Validation("investigation %d must be 1-100 characters", i+1)
		}
	}
	if in.Investigations == nil {
		in.Investigations = []string{}
	}
	if in.Medicines == nil {
		in.Medicines = []Medicine{}
	}

	if in.FollowUpDate != "" {
		d, err := time.ParseInLocation(time.DateOnly, in.FollowUpDate, Dhaka)
		if err != nil {
			return apperr.Validation("follow_up_date must be YYYY-MM-DD")
		}
		n := s.now().In(Dhaka)
		today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, Dhaka)
		if !d.After(today) || d.After(today.AddDate(1, 0, 0)) {
			return apperr.Validation("follow_up_date must be within the next year")
		}
	}
	return nil
}

// ---------------------------------------------------------------- issue

// Issue creates a prescription. The doctor must be verified and must be the doctor
// of the consultation (active or completed) or appointment (confirmed or completed,
// today or earlier).
func (s *Store) Issue(ctx context.Context, doctorID string, in IssueInput) (Prescription, error) {
	if err := s.validate(&in); err != nil {
		return Prescription{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Prescription{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var patientID string
	if in.ConsultationID != "" {
		var status string
		err = tx.QueryRow(ctx, `SELECT patient_id, status FROM consultations WHERE id = $1 AND doctor_id = $2`,
			in.ConsultationID, doctorID).Scan(&patientID, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return Prescription{}, apperr.NotFound("consultation not found")
		}
		if err == nil && status != "active" && status != "completed" {
			return Prescription{}, apperr.Conflict("prescriptions can only be issued for active or completed consultations")
		}
	} else {
		var status string
		var future bool
		err = tx.QueryRow(ctx, `
			SELECT patient_id, status, appointment_date > $3::date FROM appointments WHERE id = $1 AND doctor_id = $2`,
			in.AppointmentID, doctorID, s.now().In(Dhaka).Format(time.DateOnly)).Scan(&patientID, &status, &future)
		if errors.Is(err, pgx.ErrNoRows) {
			return Prescription{}, apperr.NotFound("appointment not found")
		}
		if err == nil && (future || (status != "confirmed" && status != "completed")) {
			return Prescription{}, apperr.Conflict("prescriptions can only be issued for today's or past appointments that were not cancelled")
		}
	}
	if err != nil {
		return Prescription{}, err
	}

	var doctorName, verification string
	var degrees, specialization, bmdc *string
	var patientName string
	err = tx.QueryRow(ctx, `
		SELECT du.name, d.degrees, d.specialization, d.bmdc_reg_no, d.verification_status, pu.name
		FROM doctor_profiles d
		JOIN users du ON du.id = d.user_id
		JOIN users pu ON pu.id = $2
		WHERE d.user_id = $1`, doctorID, patientID).Scan(&doctorName, &degrees, &specialization, &bmdc, &verification, &patientName)
	if err != nil {
		return Prescription{}, err
	}
	if verification != "approved" || bmdc == nil {
		return Prescription{}, apperr.Forbidden("only verified doctors can issue prescriptions")
	}

	meds, _ := json.Marshal(in.Medicines)
	tests, _ := json.Marshal(in.Investigations)
	code, err := newCode()
	if err != nil {
		return Prescription{}, err
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO prescriptions
		    (code, doctor_id, patient_id, consultation_id, appointment_id,
		     doctor_name, doctor_degrees, doctor_specialization, doctor_bmdc,
		     patient_name, patient_age, patient_gender,
		     chief_complaints, diagnosis, medicines, investigations, advice, follow_up_date, issued_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18::date, $19)
		RETURNING id`,
		code, doctorID, patientID, nullIf(in.ConsultationID), nullIf(in.AppointmentID),
		doctorName, degrees, specialization, *bmdc,
		patientName, nullIf(in.PatientAge), nullIf(in.PatientGender),
		nullIf(in.ChiefComplaints), nullIf(in.Diagnosis), meds, tests, nullIf(in.Advice), nullIf(in.FollowUpDate),
		s.now()).Scan(&id)
	if err != nil {
		return Prescription{}, err
	}

	if err := notify.Enqueue(ctx, tx, notify.Msg{
		UserID: patientID,
		Kind:   notify.KindPrescriptionIssued,
		Title:  "New prescription",
		Body:   fmt.Sprintf("%s has issued you a prescription. Code: %s", doctorName, code),
		SMS:    true,
	}); err != nil {
		return Prescription{}, err
	}

	p, err := s.get(ctx, tx, `p.id = $1`, id)
	if err != nil {
		return Prescription{}, err
	}
	return p, tx.Commit(ctx)
}

// ---------------------------------------------------------------- read

const selectColumns = `
SELECT p.id, p.code, p.doctor_id, p.patient_id, p.consultation_id, p.appointment_id,
       p.doctor_name, p.doctor_degrees, p.doctor_specialization, p.doctor_bmdc,
       p.patient_name, p.patient_age, p.patient_gender,
       p.chief_complaints, p.diagnosis, p.medicines, p.investigations, p.advice,
       to_char(p.follow_up_date, 'YYYY-MM-DD'),
       p.status, p.revoked_reason, p.issued_at, p.revoked_at
FROM prescriptions p`

func scan(row pgx.Row) (Prescription, error) {
	var p Prescription
	var meds, tests []byte
	err := row.Scan(&p.ID, &p.Code, &p.DoctorID, &p.PatientID, &p.ConsultationID, &p.AppointmentID,
		&p.DoctorName, &p.DoctorDegrees, &p.DoctorSpecialization, &p.DoctorBMDC,
		&p.PatientName, &p.PatientAge, &p.PatientGender,
		&p.ChiefComplaints, &p.Diagnosis, &meds, &tests, &p.Advice, &p.FollowUpDate,
		&p.Status, &p.RevokedReason, &p.IssuedAt, &p.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Prescription{}, apperr.NotFound("prescription not found")
	}
	if err != nil {
		return Prescription{}, err
	}
	if err := json.Unmarshal(meds, &p.Medicines); err != nil {
		return Prescription{}, err
	}
	if err := json.Unmarshal(tests, &p.Investigations); err != nil {
		return Prescription{}, err
	}
	return p, nil
}

func (s *Store) get(ctx context.Context, q database.Querier, where string, args ...any) (Prescription, error) {
	return scan(q.QueryRow(ctx, selectColumns+` WHERE `+where, args...))
}

// Get returns a prescription to its doctor or patient.
func (s *Store) Get(ctx context.Context, userID, id string) (Prescription, error) {
	return s.get(ctx, s.db, `p.id = $1 AND (p.patient_id = $2 OR p.doctor_id = $2)`, id, userID)
}

// List returns prescriptions where the user is the patient (asDoctor=false) or the doctor.
// consultationID / appointmentID optionally narrow the list.
func (s *Store) List(ctx context.Context, userID string, asDoctor bool, consultationID, appointmentID string, limit, offset int) ([]Prescription, error) {
	col := "p.patient_id"
	if asDoctor {
		col = "p.doctor_id"
	}
	rows, err := s.db.Query(ctx, selectColumns+`
		WHERE `+col+` = $1
		  AND ($2 = '' OR p.consultation_id::text = $2)
		  AND ($3 = '' OR p.appointment_id::text = $3)
		ORDER BY p.issued_at DESC, p.id
		LIMIT $4 OFFSET $5`, userID, consultationID, appointmentID, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Prescription, error) { return scan(r) })
}

// Revoke withdraws a prescription the doctor issued.
func (s *Store) Revoke(ctx context.Context, doctorID, id, reason string) (Prescription, error) {
	reason = clean(reason)
	if n := utf8.RuneCountInString(reason); n < 5 || n > 500 {
		return Prescription{}, apperr.Validation("reason must be 5-500 characters")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE prescriptions SET status = 'revoked', revoked_reason = $3, revoked_at = $4
		WHERE id = $1 AND doctor_id = $2 AND status = 'issued'`, id, doctorID, reason, s.now())
	if err != nil {
		return Prescription{}, err
	}
	p, err := s.get(ctx, s.db, `p.id = $1 AND p.doctor_id = $2`, id, doctorID)
	if err != nil {
		return Prescription{}, err
	}
	if tag.RowsAffected() == 0 {
		return Prescription{}, apperr.Conflict("this prescription is already revoked")
	}
	return p, nil
}

// Verify looks up a prescription by its public code.
func (s *Store) Verify(ctx context.Context, code string) (Verification, error) {
	code = strings.ToUpper(strings.ReplaceAll(clean(code), "-", ""))
	p, err := s.get(ctx, s.db, `p.code = $1`, code)
	if err != nil {
		return Verification{}, err
	}
	return Verification{
		Code:                 p.Code,
		Status:               p.Status,
		IssuedAt:             p.IssuedAt,
		DoctorName:           p.DoctorName,
		DoctorBMDC:           p.DoctorBMDC,
		DoctorSpecialization: p.DoctorSpecialization,
		PatientName:          MaskName(p.PatientName),
		Medicines:            p.Medicines,
	}, nil
}

// MaskName keeps the first letter of each word: "Karim Uddin" -> "K**** U****".
func MaskName(name string) string {
	words := strings.Fields(name)
	for i, w := range words {
		r := []rune(w)
		words[i] = string(r[0]) + strings.Repeat("*", len(r)-1)
	}
	return strings.Join(words, " ")
}

// codeAlphabet avoids look-alike characters (0/O, 1/I/L) since codes are read aloud and typed.
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// newCode returns a 10-character code (about 49 bits of randomness), each character
// drawn uniformly from codeAlphabet.
func newCode() (string, error) {
	b := make([]byte, 10)
	n := big.NewInt(int64(len(codeAlphabet)))
	for i := range b {
		k, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[k.Int64()]
	}
	return string(b), nil
}

func nullIf(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
