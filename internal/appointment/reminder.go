package appointment

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/notify"
)

const (
	// DayBeforeHour is when, Dhaka time, the evening-before reminders go out.
	DayBeforeHour = 18
	// SoonBefore is how long before the session starts the second reminder goes out.
	SoonBefore = 2 * time.Hour
	// A booking made this recently already got its confirmation SMS, so it
	// skips the reminder that would follow straight after it.
	dayBeforeQuiet = 12 * time.Hour
	soonQuiet      = time.Hour
)

// SendReminders sends whichever appointment reminders are due now and returns
// how many it sent. Each appointment is claimed with SKIP LOCKED and marked in
// the same transaction as its notification, so no reminder goes out twice.
func (s *Store) SendReminders(ctx context.Context) (int, error) {
	now := s.now().In(Dhaka)
	today := now.Format(time.DateOnly)
	sent := 0

	if now.Hour() >= DayBeforeHour {
		tomorrow := now.AddDate(0, 0, 1).Format(time.DateOnly)
		n, err := s.remind(ctx, "reminded_day_before_at", `
			SELECT a.id FROM appointments a
			WHERE a.status = 'confirmed' AND a.reminded_day_before_at IS NULL
			  AND a.appointment_date = $1::date AND a.created_at < $2
			FOR UPDATE OF a SKIP LOCKED`,
			[]any{tomorrow, now.Add(-dayBeforeQuiet)}, dayBeforeText)
		sent += n
		if err != nil {
			return sent, err
		}
	}

	// Sessions starting within SoonBefore from now, later today.
	n, err := s.remind(ctx, "reminded_soon_at", `
		SELECT a.id FROM appointments a
		JOIN chamber_schedules s ON s.id = a.schedule_id
		WHERE a.status = 'confirmed' AND a.reminded_soon_at IS NULL
		  AND a.appointment_date = $1::date
		  AND s.start_time > $2::time AND s.start_time <= $3::time
		  AND a.created_at < $4
		FOR UPDATE OF a SKIP LOCKED`,
		[]any{today, now.Format("15:04:05"), clampToDay(now, SoonBefore), now.Add(-soonQuiet)}, soonText)
	return sent + n, err
}

// clampToDay is now+d as a time of day, capped at the end of today so the
// window never wraps past midnight.
func clampToDay(now time.Time, d time.Duration) string {
	later := now.Add(d)
	if later.Day() != now.Day() {
		return "23:59:59"
	}
	return later.Format("15:04:05")
}

func (s *Store) remind(ctx context.Context, column, due string, args []any, text func(Appointment) string) (int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, due, args...)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE appointments SET `+column+` = now() WHERE id = ANY($1)`, ids); err != nil {
		return 0, err
	}
	rows, err = tx.Query(ctx, appointmentSelect+` WHERE a.id = ANY($1)`, ids)
	if err != nil {
		return 0, err
	}
	as, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Appointment, error) { return scanAppointment(r) })
	if err != nil {
		return 0, err
	}
	for _, a := range as {
		if err := notify.Enqueue(ctx, tx, notify.Msg{
			UserID: a.PatientID, Kind: notify.KindAppointmentReminder, Title: "Appointment reminder", Body: text(a), SMS: true,
		}); err != nil {
			return 0, err
		}
	}
	return len(as), tx.Commit(ctx)
}

func dayBeforeText(a Appointment) string {
	return fmt.Sprintf("%sReminder: tomorrow, serial %d with %s at %s, session from %s. Ticket: %s",
		forWhom(a), a.SerialNo, a.DoctorName, place(a), clock12(a.StartTime), a.TicketCode)
}

func soonText(a Appointment) string {
	return fmt.Sprintf("%sReminder: serial %d with %s at %s today, session from %s. Ticket: %s",
		forWhom(a), a.SerialNo, a.DoctorName, place(a), clock12(a.StartTime), a.TicketCode)
}

func place(a Appointment) string {
	if a.Room != nil && *a.Room != "" {
		return a.ChamberName + ", " + *a.Room
	}
	return a.ChamberName
}

// clock12 turns "17:00" into "5:00 PM".
func clock12(hhmm string) string {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return hhmm
	}
	return t.Format("3:04 PM")
}

// RunReminders sends due reminders every minute until ctx ends.
func (s *Store) RunReminders(ctx context.Context, log *slog.Logger) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if n, err := s.SendReminders(ctx); err != nil && ctx.Err() == nil {
			log.Error("appointment reminders", "err", err)
		} else if n > 0 {
			log.Info("appointment reminders sent", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
