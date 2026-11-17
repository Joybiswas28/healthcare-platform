package appointment

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

// Operations hospitals perform on bookings made at their chambers.

// ListForHospital returns the day's bookings at a hospital, optionally for one doctor.
func (s *Store) ListForHospital(ctx context.Context, hospitalID, doctorID, rawDate string) ([]Appointment, error) {
	d, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(rawDate), Dhaka)
	if err != nil {
		return nil, apperr.Validation("date must be in YYYY-MM-DD format")
	}
	return s.list(ctx, `
		WHERE c.hospital_id = $1 AND a.appointment_date = $2::date AND ($3 = '' OR a.doctor_id::text = $3)
		ORDER BY du.name, s.start_time, a.serial_no`, hospitalID, d.Format(time.DateOnly), doctorID)
}

func (s *Store) GetForHospital(ctx context.Context, hospitalID, id string) (Appointment, error) {
	return s.getOne(ctx, `a.id = $1 AND c.hospital_id = $2`, id, hospitalID)
}

// lockForHospital locks an upcoming confirmed booking at the hospital.
func (s *Store) lockForHospital(ctx context.Context, tx pgx.Tx, hospitalID, id string) (Appointment, error) {
	a, err := scanAppointment(tx.QueryRow(ctx,
		appointmentSelect+` WHERE a.id = $1 AND c.hospital_id = $2 FOR UPDATE OF a`, id, hospitalID))
	if err != nil {
		return Appointment{}, err
	}
	if a.Status != StatusConfirmed || a.Date < s.Today() {
		return Appointment{}, apperr.Conflict("only upcoming confirmed appointments can be changed (this one is %s, %s)", a.Status, a.Date)
	}
	return a, nil
}

// CancelByHospital cancels a booking (e.g. the doctor is unavailable) and texts the patient.
func (s *Store) CancelByHospital(ctx context.Context, hospitalID, id, reason string) (Appointment, error) {
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 5 || n > 300 {
		return Appointment{}, apperr.Validation("reason must be 5-300 characters")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Appointment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	a, err := s.lockForHospital(ctx, tx, hospitalID, id)
	if err != nil {
		return Appointment{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE appointments SET status = 'cancelled', cancelled_by = 'hospital', cancel_reason = $2, updated_at = now()
		WHERE id = $1`, id, reason); err != nil {
		return Appointment{}, err
	}
	if err := notify.Enqueue(ctx, tx, notify.Msg{
		UserID: a.PatientID,
		Kind:   notify.KindAppointmentCancelled,
		Title:  "Appointment cancelled",
		Body: fmt.Sprintf("%s cancelled your appointment with %s on %s (serial %d): %s",
			deref(a.HospitalName), a.DoctorName, a.Date, a.SerialNo, reason),
		SMS: true,
	}); err != nil {
		return Appointment{}, err
	}
	a, err = scanAppointment(tx.QueryRow(ctx, appointmentSelect+` WHERE a.id = $1`, id))
	if err != nil {
		return Appointment{}, err
	}
	return a, tx.Commit(ctx)
}

// Reschedule moves a booking to another session of the same doctor at the same
// hospital. The patient gets a new serial and ticket; the old booking is marked
// rescheduled and points at the new one.
func (s *Store) Reschedule(ctx context.Context, hospitalID, id, newScheduleID, rawDate string) (Appointment, error) {
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

	old, err := s.lockForHospital(ctx, tx, hospitalID, id)
	if err != nil {
		return Appointment{}, err
	}
	if old.ScheduleID == newScheduleID && old.Date == dateStr {
		return Appointment{}, apperr.Validation("the new session and date are the same as the current ones")
	}

	ss, err := loadSession(ctx, tx, newScheduleID, true)
	if err != nil {
		return Appointment{}, err
	}
	if ss.HospitalID == nil || *ss.HospitalID != hospitalID || ss.DoctorID != old.DoctorID {
		return Appointment{}, apperr.Validation("the new session must be with the same doctor at this hospital")
	}
	if !ss.Bookable {
		return Appointment{}, apperr.Conflict("the new session is not taking bookings")
	}
	if err := s.checkDate(ss, date); err != nil {
		return Appointment{}, err
	}
	active, maxSerial, err := countBooked(ctx, tx, newScheduleID, dateStr)
	if err != nil {
		return Appointment{}, err
	}
	if active >= ss.MaxPatients {
		return Appointment{}, apperr.Conflict("the new session is fully booked")
	}

	// Free the old booking first so the one-booking-per-session rule doesn't trip
	// when moving within the same schedule.
	if _, err := tx.Exec(ctx, `UPDATE appointments SET status = 'rescheduled', updated_at = now() WHERE id = $1`, id); err != nil {
		return Appointment{}, err
	}
	code, err := newTicketCode()
	if err != nil {
		return Appointment{}, err
	}
	var newID string
	err = tx.QueryRow(ctx, `
		INSERT INTO appointments
		    (ticket_code, schedule_id, chamber_id, doctor_id, patient_id, appointment_date, serial_no, fee_paisa)
		VALUES ($1, $2, $3, $4, $5, $6::date, $7, $8)
		RETURNING id`,
		code, newScheduleID, ss.ChamberID, ss.DoctorID, old.PatientID, dateStr, maxSerial+1, ss.FeePaisa,
	).Scan(&newID)
	if database.UniqueViolationOn(err, "appointments_one_active_per_patient") {
		return Appointment{}, apperr.Conflict("the patient already has a booking in that session")
	}
	if err != nil {
		return Appointment{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE appointments SET rescheduled_to = $2 WHERE id = $1`, id, newID); err != nil {
		return Appointment{}, err
	}

	a, err := scanAppointment(tx.QueryRow(ctx, appointmentSelect+` WHERE a.id = $1`, newID))
	if err != nil {
		return Appointment{}, err
	}
	if err := notify.Enqueue(ctx, tx, notify.Msg{
		UserID: a.PatientID,
		Kind:   notify.KindAppointmentRescheduled,
		Title:  "Appointment rescheduled",
		Body: fmt.Sprintf("%s moved your appointment with %s to %s, %s-%s. New serial %d, ticket %s.",
			deref(a.HospitalName), a.DoctorName, a.Date, a.StartTime, a.EndTime, a.SerialNo, a.TicketCode),
		SMS: true,
	}); err != nil {
		return Appointment{}, err
	}
	return a, tx.Commit(ctx)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
