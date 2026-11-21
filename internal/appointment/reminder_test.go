package appointment

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestReminders(t *testing.T) {
	f := setup(t, 10)
	ctx := context.Background()
	const date = "2026-11-24" // next Tuesday; the session starts at 17:00

	p1, p2, p3 := f.patient(t, 1), f.patient(t, 2), f.patient(t, 3)
	booked, err := f.store.Book(ctx, p1, f.scheduleID, date)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := f.store.Book(ctx, p2, f.scheduleID, date)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Cancel(ctx, p2, cancelled.ID); err != nil {
		t.Fatal(err)
	}
	// p3 books on the Monday morning: too recent for the evening reminder,
	// but old enough for the one before the session.
	late, err := f.store.Book(ctx, p3, f.scheduleID, date)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE appointments SET created_at = $2 WHERE id = $1`,
		late.ID, time.Date(2026, 11, 23, 10, 0, 0, 0, Dhaka)); err != nil {
		t.Fatal(err)
	}

	at := func(day, hour, minute int) {
		f.store.now = func() time.Time { return time.Date(2026, 11, day, hour, minute, 0, 0, Dhaka) }
	}
	send := func(want int, when string) {
		t.Helper()
		n, err := f.store.SendReminders(ctx)
		if err != nil || n != want {
			t.Fatalf("%s: sent %d reminders (%v), want %d", when, n, err, want)
		}
	}
	reminders := func(patient string) []string {
		t.Helper()
		rows, err := f.pool.Query(ctx, `SELECT body FROM notifications
			WHERE user_id = $1 AND kind = 'appointment_reminder' AND sms_status = 'pending' ORDER BY id`, patient)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var b string
			if err := rows.Scan(&b); err != nil {
				t.Fatal(err)
			}
			out = append(out, b)
		}
		return out
	}

	at(23, 17, 59)
	send(0, "Monday before 6 PM")
	at(23, 18, 5)
	send(1, "Monday evening")
	send(0, "Monday evening again")
	at(24, 14, 30)
	send(0, "two and a half hours before")
	at(24, 15, 10)
	send(2, "under two hours before")
	send(0, "under two hours before, again")
	at(24, 17, 30)
	send(0, "after the session started")

	got := reminders(p1)
	if len(got) != 2 || !strings.Contains(got[0], "tomorrow") || !strings.Contains(got[1], "today") ||
		!strings.Contains(got[1], "5:00 PM") || !strings.Contains(got[1], booked.TicketCode) {
		t.Fatalf("p1 reminders = %q", got)
	}
	if got := reminders(p2); len(got) != 0 {
		t.Fatalf("cancelled booking got reminders: %q", got)
	}
	if got := reminders(p3); len(got) != 1 || !strings.Contains(got[0], "today") {
		t.Fatalf("late booking reminders = %q", got)
	}
}

func TestClampToDay(t *testing.T) {
	if got := clampToDay(time.Date(2026, 11, 24, 23, 0, 0, 0, Dhaka), 2*time.Hour); got != "23:59:59" {
		t.Fatalf("late evening window ends at %s", got)
	}
	if got := clampToDay(time.Date(2026, 11, 24, 15, 10, 0, 0, Dhaka), 2*time.Hour); got != "17:10:00" {
		t.Fatalf("afternoon window ends at %s", got)
	}
}
