package appointment

import (
	"context"
	"strings"
	"testing"
	"time"

	"healthcare-platform/internal/platform/apperr"
)

func TestLiveQueue(t *testing.T) {
	f := setup(t, 10) // Tuesday 17:00-21:00, 10 places: 24 minutes each before it starts
	ctx := context.Background()
	doc := QueueActor{DoctorID: f.doctorID}

	var patients []string
	var ids []string
	for i := 1; i <= 7; i++ {
		p := f.patient(t, i)
		a, err := f.store.Book(ctx, p, f.scheduleID, today)
		if err != nil {
			t.Fatal(err)
		}
		patients, ids = append(patients, p), append(ids, a.ID)
	}
	if _, err := f.store.Cancel(ctx, patients[2], ids[2]); err != nil { // serial 3
		t.Fatal(err)
	}
	at := func(hour, minute int) {
		f.store.now = func() time.Time { return time.Date(2026, 11, 17, hour, minute, 0, 0, Dhaka) }
	}
	turn := func(i int) Turn {
		t.Helper()
		tn, err := f.store.TurnFor(ctx, patients[i-1], ids[i-1])
		if err != nil {
			t.Fatal(err)
		}
		return tn
	}
	alerts := func(i int) []string {
		t.Helper()
		rows, err := f.pool.Query(ctx, `SELECT title || ': ' || body FROM notifications
			WHERE user_id = $1 AND kind = 'queue_turn' AND sms_status = 'pending' ORDER BY id`, patients[i-1])
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return out
	}

	// Before the session: a place in line but no estimate yet.
	if tn := turn(4); tn.Status != TurnWaiting || tn.Ahead != 2 || tn.EstimatedMinutes != nil ||
		tn.Queue == nil || tn.Queue.Started || tn.Queue.MinutesPerPatient != 24 || tn.Queue.Booked != 6 {
		t.Fatalf("before the start: %+v %+v", tn, tn.Queue)
	}

	// Only the chamber's doctor (or its hospital) runs the queue.
	_, err := f.store.SetServing(ctx, QueueActor{DoctorID: patients[0]}, f.scheduleID, 0)
	wantKind(t, err, apperr.KindNotFound, "someone else's session")
	_, err = f.store.SetServing(ctx, QueueActor{HospitalID: "00000000-0000-0000-0000-000000000001"}, f.scheduleID, 0)
	wantKind(t, err, apperr.KindNotFound, "a hospital the chamber isn't part of")

	at(17, 0)
	q, err := f.store.SetServing(ctx, doc, f.scheduleID, 0)
	if err != nil || q.NowServing != 1 || !q.Started || q.Waiting != 5 {
		t.Fatalf("start: %+v, %v", q, err)
	}
	// Serial 1 goes in; 2, 4 and 5 have at most two patients before them.
	if got := alerts(1); len(got) != 1 || !strings.HasPrefix(got[0], "It's your turn") {
		t.Fatalf("serial 1 alerts = %q", got)
	}
	if got := alerts(2); len(got) != 1 || !strings.Contains(got[0], "you are next") {
		t.Fatalf("serial 2 alerts = %q", got)
	}
	if got := alerts(5); len(got) != 1 || !strings.Contains(got[0], "2 patients are before you") {
		t.Fatalf("serial 5 alerts = %q", got)
	}
	if got := alerts(6); len(got) != 0 {
		t.Fatalf("serial 6 alerted too early: %q", got)
	}

	at(17, 10)
	if q, err = f.store.SetServing(ctx, doc, f.scheduleID, 0); err != nil || q.NowServing != 2 {
		t.Fatalf("next: %+v, %v", q, err)
	}
	if got := alerts(6); len(got) != 1 {
		t.Fatalf("serial 6 alerts = %q", got)
	}
	if got := alerts(2); len(got) != 1 {
		t.Fatalf("serial 2 alerted twice: %q", got)
	}

	at(17, 30)
	// Next skips the cancelled serial 3.
	if q, err = f.store.SetServing(ctx, doc, f.scheduleID, 0); err != nil || q.NowServing != 4 || q.MinutesPerPatient != 15 {
		t.Fatalf("next past a cancelled serial: %+v, %v", q, err)
	}
	if tn := turn(7); tn.Status != TurnWaiting || tn.Ahead != 2 || tn.EstimatedMinutes == nil || *tn.EstimatedMinutes != 45 {
		t.Fatalf("serial 7 at 17:30: %+v", tn)
	}
	if tn := turn(4); tn.Status != TurnNow {
		t.Fatalf("serial 4: %+v", tn)
	}
	if tn := turn(1); tn.Status != TurnPassed {
		t.Fatalf("serial 1, still not marked seen: %+v", tn)
	}
	if tn := turn(3); tn.Status != TurnClosed || tn.Queue != nil {
		t.Fatalf("cancelled booking: %+v", tn)
	}
	if _, err := f.store.TurnFor(ctx, patients[0], ids[6]); !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("someone else's booking: %v", err)
	}

	// Setting a serial by hand: it must be a real booking today.
	_, err = f.store.SetServing(ctx, doc, f.scheduleID, 3)
	wantKind(t, err, apperr.KindValidation, "a cancelled serial")
	_, err = f.store.SetServing(ctx, doc, f.scheduleID, 99)
	wantKind(t, err, apperr.KindValidation, "a serial nobody has")
	if q, err = f.store.SetServing(ctx, doc, f.scheduleID, 7); err != nil || q.NowServing != 7 || q.Waiting != 0 {
		t.Fatalf("jump to 7: %+v, %v", q, err)
	}
	_, err = f.store.SetServing(ctx, doc, f.scheduleID, 0)
	wantKind(t, err, apperr.KindConflict, "next after the last serial")
	if q, err := f.store.QueueToday(ctx, doc, f.scheduleID); err != nil || q.NowServing != 7 {
		t.Fatalf("queue today: %+v, %v", q, err)
	}

	// The session runs on Tuesdays only.
	f.store.now = func() time.Time { return time.Date(2026, 11, 18, 17, 0, 0, 0, Dhaka) }
	_, err = f.store.SetServing(ctx, doc, f.scheduleID, 0)
	wantKind(t, err, apperr.KindConflict, "a Wednesday")
	if tn := turn(5); tn.Status != TurnClosed {
		t.Fatalf("the next day: %+v", tn)
	}
}
