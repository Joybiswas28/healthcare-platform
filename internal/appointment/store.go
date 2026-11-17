// Package appointment implements serial-based chamber booking (e-ticketing).
//
// A patient books a weekly session (schedule) on a specific date and gets the next
// serial number for that session, plus a short ticket code to show at the chamber.
package appointment

import (
	"context"
	"crypto/rand"
	"encoding/base32"
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
)

// Dhaka is Bangladesh time. Bangladesh has no DST, so a fixed zone is exact.
var Dhaka = time.FixedZone("Asia/Dhaka", 6*60*60)

// BookingWindowDays is how far ahead patients may book.
const BookingWindowDays = 30

const (
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
	StatusCompleted = "completed"
	StatusNoShow    = "no_show"
)

type Appointment struct {
	ID              string    `json:"id"`
	TicketCode      string    `json:"ticket_code"`
	Status          string    `json:"status"`
	Date            string    `json:"date"` // YYYY-MM-DD
	SerialNo        int       `json:"serial_no"`
	FeePaisa        int64     `json:"fee_paisa"`
	StartTime       string    `json:"start_time"`
	EndTime         string    `json:"end_time"`
	ScheduleID      string    `json:"schedule_id"`
	DoctorID        string    `json:"doctor_id"`
	DoctorName      string    `json:"doctor_name"`
	ChamberID       string    `json:"chamber_id"`
	ChamberName     string    `json:"chamber_name"`
	ChamberAddress  string    `json:"chamber_address"`
	ChamberDistrict string    `json:"chamber_district"`
	PatientID       string    `json:"patient_id"`
	PatientName     string    `json:"patient_name"`
	PatientPhone    string    `json:"patient_phone"`
	HospitalID      *string   `json:"hospital_id"`
	HospitalName    *string   `json:"hospital_name"`
	Room            *string   `json:"room"`
	CancelledBy     *string   `json:"cancelled_by"`
	CancelReason    *string   `json:"cancel_reason"`
	RescheduledTo   *string   `json:"rescheduled_to"`
	CreatedAt       time.Time `json:"created_at"`
}

type Availability struct {
	ScheduleID  string `json:"schedule_id"`
	Date        string `json:"date"`
	MaxPatients int    `json:"max_patients"`
	Booked      int    `json:"booked"`
	Remaining   int    `json:"remaining"`
}

type Store struct {
	db  *pgxpool.Pool
	now func() time.Time
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db, now: time.Now} }

var errAppointmentNotFound = apperr.NotFound("appointment not found")

// ---------------------------------------------------------------- dates

func (s *Store) today() time.Time {
	n := s.now().In(Dhaka)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, Dhaka)
}

// Today returns today's date in Bangladesh as YYYY-MM-DD.
func (s *Store) Today() string { return s.today().Format(time.DateOnly) }

func (s *Store) parseBookableDate(raw string) (time.Time, error) {
	d, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(raw), Dhaka)
	if err != nil {
		return time.Time{}, apperr.Validation("date must be in YYYY-MM-DD format")
	}
	today := s.today()
	if d.Before(today) {
		return time.Time{}, apperr.Validation("date is in the past")
	}
	if d.After(today.AddDate(0, 0, BookingWindowDays)) {
		return time.Time{}, apperr.Validation("bookings open only %d days in advance", BookingWindowDays)
	}
	return d, nil
}

// ---------------------------------------------------------------- schedule lookup

type session struct {
	ChamberID   string
	HospitalID  *string
	DoctorID    string
	Weekday     int
	EndTime     string // "HH:MM"
	MaxPatients int
	FeePaisa    int64
	Bookable    bool
}

func loadSession(ctx context.Context, q database.Querier, scheduleID string, lock bool) (session, error) {
	sql := `
		SELECT c.id, c.hospital_id, c.doctor_id, s.weekday, to_char(s.end_time, 'HH24:MI'), s.max_patients, c.fee_paisa,
		       (s.active AND d.verification_status = 'approved' AND u.status = 'active' AND ` + doctor.LiveChamberSQL("c") + `)
		FROM chamber_schedules s
		JOIN chambers c        ON c.id = s.chamber_id
		JOIN doctor_profiles d ON d.user_id = c.doctor_id
		JOIN users u           ON u.id = c.doctor_id
		WHERE s.id = $1`
	if lock {
		sql += ` FOR UPDATE OF s`
	}
	var ss session
	err := q.QueryRow(ctx, sql, scheduleID).Scan(&ss.ChamberID, &ss.HospitalID, &ss.DoctorID, &ss.Weekday, &ss.EndTime,
		&ss.MaxPatients, &ss.FeePaisa, &ss.Bookable)
	if errors.Is(err, pgx.ErrNoRows) {
		return session{}, apperr.NotFound("schedule not found")
	}
	return ss, err
}

// checkDate ensures the session actually runs on date and has not already ended today.
func (s *Store) checkDate(ss session, date time.Time) error {
	if int(date.Weekday()) != ss.Weekday {
		return apperr.Validation("this session runs only on %ss", time.Weekday(ss.Weekday))
	}
	if date.Equal(s.today()) && s.now().In(Dhaka).Format("15:04") >= ss.EndTime {
		return apperr.Validation("today's session has already ended")
	}
	return nil
}

func countBooked(ctx context.Context, q database.Querier, scheduleID, date string) (active, maxSerial int, err error) {
	err = q.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status NOT IN ('cancelled', 'rescheduled')), COALESCE(max(serial_no), 0)
		FROM appointments
		WHERE schedule_id = $1 AND appointment_date = $2::date`, scheduleID, date).Scan(&active, &maxSerial)
	return active, maxSerial, err
}

// ---------------------------------------------------------------- queries

const appointmentSelect = `
SELECT a.id, a.ticket_code, a.status, to_char(a.appointment_date, 'YYYY-MM-DD'), a.serial_no, a.fee_paisa,
       to_char(s.start_time, 'HH24:MI'), to_char(s.end_time, 'HH24:MI'), a.schedule_id,
       a.doctor_id, du.name, c.id, c.name, c.address, c.district,
       a.patient_id, pu.name, pu.phone, c.hospital_id, h.name, c.room,
       a.cancelled_by, a.cancel_reason, a.rescheduled_to, a.created_at
FROM appointments a
JOIN chamber_schedules s ON s.id = a.schedule_id
JOIN chambers c          ON c.id = a.chamber_id
JOIN users du            ON du.id = a.doctor_id
JOIN users pu            ON pu.id = a.patient_id
LEFT JOIN hospitals h    ON h.id = c.hospital_id`

func scanAppointment(row pgx.Row) (Appointment, error) {
	var a Appointment
	err := row.Scan(&a.ID, &a.TicketCode, &a.Status, &a.Date, &a.SerialNo, &a.FeePaisa,
		&a.StartTime, &a.EndTime, &a.ScheduleID,
		&a.DoctorID, &a.DoctorName, &a.ChamberID, &a.ChamberName, &a.ChamberAddress, &a.ChamberDistrict,
		&a.PatientID, &a.PatientName, &a.PatientPhone, &a.HospitalID, &a.HospitalName, &a.Room,
		&a.CancelledBy, &a.CancelReason, &a.RescheduledTo, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Appointment{}, errAppointmentNotFound
	}
	return a, err
}

func (s *Store) getOne(ctx context.Context, where string, args ...any) (Appointment, error) {
	return scanAppointment(s.db.QueryRow(ctx, appointmentSelect+` WHERE `+where, args...))
}

func (s *Store) list(ctx context.Context, tail string, args ...any) ([]Appointment, error) {
	rows, err := s.db.Query(ctx, appointmentSelect+` `+tail, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Appointment, error) { return scanAppointment(r) })
}

// ---------------------------------------------------------------- operations

// Availability reports how many serials are left for a session on a date.
func (s *Store) Availability(ctx context.Context, scheduleID, rawDate string) (Availability, error) {
	date, err := s.parseBookableDate(rawDate)
	if err != nil {
		return Availability{}, err
	}
	ss, err := loadSession(ctx, s.db, scheduleID, false)
	if err != nil {
		return Availability{}, err
	}
	if !ss.Bookable {
		return Availability{}, apperr.NotFound("schedule not found")
	}
	if err := s.checkDate(ss, date); err != nil {
		return Availability{}, err
	}
	dateStr := date.Format(time.DateOnly)
	booked, _, err := countBooked(ctx, s.db, scheduleID, dateStr)
	if err != nil {
		return Availability{}, err
	}
	return Availability{
		ScheduleID:  scheduleID,
		Date:        dateStr,
		MaxPatients: ss.MaxPatients,
		Booked:      booked,
		Remaining:   max(ss.MaxPatients-booked, 0),
	}, nil
}

// Book reserves the next serial for a patient. The schedule row is locked for the
// duration of the transaction, so concurrent bookings for the same session queue up
// and can never exceed max_patients or receive the same serial.
func (s *Store) Book(ctx context.Context, patientID, scheduleID, rawDate string) (Appointment, error) {
	date, err := s.parseBookableDate(rawDate)
	if err != nil {
		return Appointment{}, err
	}
	dateStr := date.Format(time.DateOnly)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Appointment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ss, err := loadSession(ctx, tx, scheduleID, true)
	if err != nil {
		return Appointment{}, err
	}
	if !ss.Bookable {
		return Appointment{}, apperr.Conflict("this doctor is not accepting bookings for this session")
	}
	if ss.DoctorID == patientID {
		return Appointment{}, apperr.Validation("you cannot book an appointment with yourself")
	}
	if err := s.checkDate(ss, date); err != nil {
		return Appointment{}, err
	}

	active, maxSerial, err := countBooked(ctx, tx, scheduleID, dateStr)
	if err != nil {
		return Appointment{}, err
	}
	if active >= ss.MaxPatients {
		return Appointment{}, apperr.Conflict("this session is fully booked")
	}

	code, err := newTicketCode()
	if err != nil {
		return Appointment{}, err
	}
	// Serials are never reused: a cancelled serial 3 stays cancelled and the next booking gets 4.
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO appointments
		    (ticket_code, schedule_id, chamber_id, doctor_id, patient_id, appointment_date, serial_no, fee_paisa)
		VALUES ($1, $2, $3, $4, $5, $6::date, $7, $8)
		RETURNING id`,
		code, scheduleID, ss.ChamberID, ss.DoctorID, patientID, dateStr, maxSerial+1, ss.FeePaisa,
	).Scan(&id)
	if database.UniqueViolationOn(err, "appointments_one_active_per_patient") {
		return Appointment{}, apperr.Conflict("you already have a booking for this session")
	}
	if err != nil {
		return Appointment{}, err
	}
	a, err := scanAppointment(tx.QueryRow(ctx, appointmentSelect+` WHERE a.id = $1`, id))
	if err != nil {
		return Appointment{}, err
	}
	if err := notify.Enqueue(ctx, tx, notify.Msg{
		UserID: patientID,
		Kind:   notify.KindAppointmentBooked,
		Title:  "Appointment confirmed",
		Body: fmt.Sprintf("Serial %d with %s at %s on %s, %s-%s. Ticket: %s",
			a.SerialNo, a.DoctorName, a.ChamberName, a.Date, a.StartTime, a.EndTime, a.TicketCode),
		SMS: true,
	}); err != nil {
		return Appointment{}, err
	}
	return a, tx.Commit(ctx)
}

// ListForPatient returns a patient's appointments, newest date first.
func (s *Store) ListForPatient(ctx context.Context, patientID string, limit, offset int) ([]Appointment, error) {
	return s.list(ctx, `
		WHERE a.patient_id = $1
		ORDER BY a.appointment_date DESC, s.start_time DESC, a.serial_no
		LIMIT $2 OFFSET $3`, patientID, limit, offset)
}

// ListForDoctor returns the doctor's patient list for one day, in serial order per session.
func (s *Store) ListForDoctor(ctx context.Context, doctorID, rawDate string) ([]Appointment, error) {
	d, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(rawDate), Dhaka)
	if err != nil {
		return nil, apperr.Validation("date must be in YYYY-MM-DD format")
	}
	return s.list(ctx, `
		WHERE a.doctor_id = $1 AND a.appointment_date = $2::date
		ORDER BY s.start_time, a.serial_no`, doctorID, d.Format(time.DateOnly))
}

// Cancel lets a patient cancel one of their own upcoming, confirmed appointments.
func (s *Store) Cancel(ctx context.Context, patientID, id string) (Appointment, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE appointments SET status = 'cancelled', cancelled_by = 'patient', updated_at = now()
		WHERE id = $1 AND patient_id = $2 AND status = 'confirmed' AND appointment_date >= $3::date`,
		id, patientID, s.Today())
	if err != nil {
		return Appointment{}, err
	}
	a, err := s.getOne(ctx, `a.id = $1 AND a.patient_id = $2`, id, patientID)
	if err != nil {
		return Appointment{}, err
	}
	if tag.RowsAffected() == 0 {
		return Appointment{}, apperr.Conflict("only upcoming confirmed appointments can be cancelled")
	}
	return a, nil
}

// SetOutcome lets the doctor mark a confirmed appointment from today or earlier
// as completed or no_show.
func (s *Store) SetOutcome(ctx context.Context, doctorID, id, status string) (Appointment, error) {
	if status != StatusCompleted && status != StatusNoShow {
		return Appointment{}, apperr.Validation("status must be completed or no_show")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE appointments SET status = $3, updated_at = now()
		WHERE id = $1 AND doctor_id = $2 AND status = 'confirmed' AND appointment_date <= $4::date`,
		id, doctorID, status, s.Today())
	if err != nil {
		return Appointment{}, err
	}
	a, err := s.getOne(ctx, `a.id = $1 AND a.doctor_id = $2`, id, doctorID)
	if err != nil {
		return Appointment{}, err
	}
	if tag.RowsAffected() == 0 {
		return Appointment{}, apperr.Conflict("only confirmed appointments from today or earlier can be updated")
	}
	return a, nil
}

// FindByTicket lets a doctor (or their chamber staff) look up a ticket shown by a patient.
func (s *Store) FindByTicket(ctx context.Context, doctorID, code string) (Appointment, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	return s.getOne(ctx, `a.ticket_code = $1 AND a.doctor_id = $2`, code, doctorID)
}

// newTicketCode returns an 8-character code such as "K7Q2M4XA" (40 random bits).
func newTicketCode() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.EncodeToString(b), nil
}
