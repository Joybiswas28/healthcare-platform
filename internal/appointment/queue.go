package appointment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

// Live serial status for today's sessions. The doctor, or staff of the
// hospital the chamber belongs to, record which serial is being seen; patients
// follow it from their phone, and are texted once when their turn is near.

// queueAlertAhead: a patient is told when this many patients or fewer are
// still before them.
const queueAlertAhead = 2

// Patient-facing states of a booking in today's queue.
const (
	TurnWaiting = "waiting"
	TurnNow     = "your_turn"
	TurnPassed  = "passed" // the doctor moved past this serial while it was still waiting
	TurnClosed  = "closed" // not today, or no longer a confirmed booking
)

// Queue is the state of one session today.
type Queue struct {
	ScheduleID string `json:"schedule_id"`
	Date       string `json:"date"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	Started    bool   `json:"started"`
	NowServing int    `json:"now_serving"` // 0 until the session starts
	Waiting    int    `json:"waiting"`     // confirmed bookings after now_serving
	Booked     int    `json:"booked"`      // bookings not cancelled or moved
	// From the session's pace so far, or its length and size before that.
	MinutesPerPatient int        `json:"minutes_per_patient"`
	UpdatedAt         *time.Time `json:"updated_at"`
}

// Turn is one patient's place in today's queue.
type Turn struct {
	AppointmentID    string `json:"appointment_id"`
	SerialNo         int    `json:"serial_no"`
	Status           string `json:"status"`
	Ahead            int    `json:"ahead"`             // confirmed patients still before this one (not counting the one being seen)
	EstimatedMinutes *int   `json:"estimated_minutes"` // once the session has started
	Queue            *Queue `json:"queue"`             // nil when closed
}

// QueueActor is who runs a queue: the chamber's doctor or its hospital.
type QueueActor struct {
	DoctorID   string
	HospitalID string
}

func (s *Store) queueSession(ctx context.Context, q database.Querier, actor QueueActor, scheduleID string) (session, error) {
	ss, err := loadSession(ctx, q, scheduleID, false)
	if err != nil {
		return session{}, err
	}
	mine := (actor.DoctorID != "" && ss.DoctorID == actor.DoctorID) ||
		(actor.HospitalID != "" && ss.HospitalID != nil && *ss.HospitalID == actor.HospitalID)
	if !mine {
		return session{}, apperr.NotFound("schedule not found")
	}
	return ss, nil
}

func (s *Store) loadQueue(ctx context.Context, q database.Querier, ss session, scheduleID, date string) (Queue, error) {
	qu := Queue{ScheduleID: scheduleID, Date: date, StartTime: ss.StartTime, EndTime: ss.EndTime}
	var advances int
	var startedAt, updatedAt time.Time
	err := q.QueryRow(ctx, `
		SELECT now_serving, advances, started_at, updated_at FROM session_queue
		WHERE schedule_id = $1 AND session_date = $2::date`, scheduleID, date).
		Scan(&qu.NowServing, &advances, &startedAt, &updatedAt)
	switch {
	case err == nil:
		qu.Started = true
		qu.UpdatedAt = &updatedAt
	case !errors.Is(err, pgx.ErrNoRows):
		return Queue{}, err
	}
	if err := q.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status NOT IN ('cancelled', 'rescheduled')),
		       count(*) FILTER (WHERE status = 'confirmed' AND serial_no > $3)
		FROM appointments WHERE schedule_id = $1 AND appointment_date = $2::date`,
		scheduleID, date, qu.NowServing).Scan(&qu.Booked, &qu.Waiting); err != nil {
		return Queue{}, err
	}
	qu.MinutesPerPatient = sessionPace(ss, qu.Started, advances, startedAt, s.now())
	return qu, nil
}

// sessionPace estimates minutes per patient: the real pace once the serial has
// moved on a few times, otherwise the session's length shared by its places.
func sessionPace(ss session, started bool, advances int, startedAt, now time.Time) int {
	if started && advances >= 2 {
		return min(max(int(now.Sub(startedAt).Minutes())/advances, 3), 60)
	}
	start, err1 := time.Parse("15:04", ss.StartTime)
	end, err2 := time.Parse("15:04", ss.EndTime)
	if err1 != nil || err2 != nil || !end.After(start) {
		return 10
	}
	return min(max(int(end.Sub(start).Minutes())/max(ss.MaxPatients, 1), 3), 30)
}

// QueueToday returns today's state of a session the actor runs.
func (s *Store) QueueToday(ctx context.Context, actor QueueActor, scheduleID string) (Queue, error) {
	ss, err := s.queueSession(ctx, s.db, actor, scheduleID)
	if err != nil {
		return Queue{}, err
	}
	return s.loadQueue(ctx, s.db, ss, scheduleID, s.Today())
}

// SetServing records which serial is being seen in today's session. serial 0
// means the next confirmed serial after the current one (or the first, to
// start the session). Patients whose turn is now near are notified once.
func (s *Store) SetServing(ctx context.Context, actor QueueActor, scheduleID string, serial int) (Queue, error) {
	if serial < 0 {
		return Queue{}, apperr.Validation("serial must be positive")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Queue{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ss, err := s.queueSession(ctx, tx, actor, scheduleID)
	if err != nil {
		return Queue{}, err
	}
	if int(s.today().Weekday()) != ss.Weekday {
		return Queue{}, apperr.Conflict("this session does not run today")
	}
	today := s.Today()
	// One update at a time per session (two staff pressing "next" together).
	if _, err := tx.Exec(ctx, `SELECT 1 FROM chamber_schedules WHERE id = $1 FOR UPDATE`, scheduleID); err != nil {
		return Queue{}, err
	}
	current := 0
	err = tx.QueryRow(ctx, `SELECT now_serving FROM session_queue WHERE schedule_id = $1 AND session_date = $2::date`,
		scheduleID, today).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Queue{}, err
	}

	if serial == 0 {
		var next *int
		if err := tx.QueryRow(ctx, `
			SELECT min(serial_no) FROM appointments
			WHERE schedule_id = $1 AND appointment_date = $2::date AND status = 'confirmed' AND serial_no > $3`,
			scheduleID, today, current).Scan(&next); err != nil {
			return Queue{}, err
		}
		if next == nil {
			return Queue{}, apperr.Conflict("no one else is waiting")
		}
		serial = *next
	} else {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM appointments
			WHERE schedule_id = $1 AND appointment_date = $2::date AND serial_no = $3 AND status NOT IN ('cancelled', 'rescheduled'))`,
			scheduleID, today, serial).Scan(&exists); err != nil {
			return Queue{}, err
		}
		if !exists {
			return Queue{}, apperr.Validation("there is no booking with serial %d today", serial)
		}
	}

	now := s.now()
	if _, err := tx.Exec(ctx, `
		INSERT INTO session_queue (schedule_id, session_date, now_serving, started_at, updated_at)
		VALUES ($1, $2::date, $3, $4, $4)
		ON CONFLICT (schedule_id, session_date) DO UPDATE SET
			now_serving = EXCLUDED.now_serving,
			updated_at  = EXCLUDED.updated_at,
			advances    = session_queue.advances + CASE WHEN EXCLUDED.now_serving > session_queue.now_serving THEN 1 ELSE 0 END`,
		scheduleID, today, serial, now); err != nil {
		return Queue{}, err
	}
	if err := s.alertNearTurn(ctx, tx, scheduleID, today, serial); err != nil {
		return Queue{}, err
	}
	qu, err := s.loadQueue(ctx, tx, ss, scheduleID, today)
	if err != nil {
		return Queue{}, err
	}
	return qu, tx.Commit(ctx)
}

// alertNearTurn notifies, once each, the patients now being seen or with at
// most queueAlertAhead confirmed patients before them.
func (s *Store) alertNearTurn(ctx context.Context, tx pgx.Tx, scheduleID, date string, serving int) error {
	rows, err := tx.Query(ctx, `
		SELECT id, serial_no, queue_alerted_at IS NOT NULL FROM appointments
		WHERE schedule_id = $1 AND appointment_date = $2::date AND status = 'confirmed' AND serial_no >= $3
		ORDER BY serial_no LIMIT $4`, scheduleID, date, serving, queueAlertAhead+2)
	if err != nil {
		return err
	}
	type waiting struct {
		id      string
		serial  int
		alerted bool
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (waiting, error) {
		var w waiting
		err := r.Scan(&w.id, &w.serial, &w.alerted)
		return w, err
	})
	if err != nil {
		return err
	}

	ahead := 0
	for _, w := range list {
		now := w.serial == serving
		if !now {
			if ahead > queueAlertAhead {
				break
			}
			ahead++
		}
		if w.alerted {
			continue
		}
		a, err := scanAppointment(tx.QueryRow(ctx, appointmentSelect+` WHERE a.id = $1`, w.id))
		if err != nil {
			return err
		}
		msg := notify.Msg{UserID: a.PatientID, Kind: notify.KindQueueTurn, SMS: true}
		where := a.ChamberName
		if a.Room != nil && *a.Room != "" {
			where += ", " + *a.Room
		}
		if now {
			msg.Title = "It's your turn"
			msg.Body = fmt.Sprintf("%s%s is now seeing serial %d at %s. Please go in.", forWhom(a), a.DoctorName, a.SerialNo, where)
		} else {
			// ahead now counts this patient too.
			before := "you are next"
			switch n := ahead - 1; {
			case n == 1:
				before = "1 patient is before you"
			case n > 1:
				before = fmt.Sprintf("%d patients are before you", n)
			}
			msg.Title = "Your turn is near"
			msg.Body = fmt.Sprintf("%s%s is now seeing serial %d at %s. Your serial is %d: %s. Please be at the chamber.",
				forWhom(a), a.DoctorName, serving, where, a.SerialNo, before)
		}
		if err := notify.Enqueue(ctx, tx, msg); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE appointments SET queue_alerted_at = now() WHERE id = $1`, w.id); err != nil {
			return err
		}
	}
	return nil
}

// TurnFor returns a patient's place in today's queue for one of their bookings.
func (s *Store) TurnFor(ctx context.Context, patientID, appointmentID string) (Turn, error) {
	a, err := s.getOne(ctx, `a.id = $1 AND a.patient_id = $2`, appointmentID, patientID)
	if err != nil {
		return Turn{}, err
	}
	t := Turn{AppointmentID: a.ID, SerialNo: a.SerialNo, Status: TurnClosed}
	if a.Status != StatusConfirmed || a.Date != s.Today() {
		return t, nil
	}
	ss, err := loadSession(ctx, s.db, a.ScheduleID, false)
	if err != nil {
		return Turn{}, err
	}
	qu, err := s.loadQueue(ctx, s.db, ss, a.ScheduleID, a.Date)
	if err != nil {
		return Turn{}, err
	}
	t.Queue = &qu
	switch {
	case qu.Started && a.SerialNo == qu.NowServing:
		t.Status = TurnNow
	case qu.Started && a.SerialNo < qu.NowServing:
		t.Status = TurnPassed
	default:
		t.Status = TurnWaiting
		if err := s.db.QueryRow(ctx, `
			SELECT count(*) FROM appointments
			WHERE schedule_id = $1 AND appointment_date = $2::date AND status = 'confirmed'
			  AND serial_no > $3 AND serial_no < $4`,
			a.ScheduleID, a.Date, qu.NowServing, a.SerialNo).Scan(&t.Ahead); err != nil {
			return Turn{}, err
		}
		if qu.Started {
			m := (t.Ahead + 1) * qu.MinutesPerPatient
			t.EstimatedMinutes = &m
		}
	}
	return t, nil
}
