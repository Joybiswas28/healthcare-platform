package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"
)

type flakySender struct {
	failures int // fail this many times, then succeed
	sent     []string
}

func (f *flakySender) Send(_ context.Context, phone, text string) error {
	if f.failures > 0 {
		f.failures--
		return errors.New("provider timeout")
	}
	f.sent = append(f.sent, phone+": "+text)
	return nil
}

func TestInboxAndSMSWorker(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	u := dbtest.CreateUser(t, pool, "Karim", "01811111111", user.RolePatient)
	other := dbtest.CreateUser(t, pool, "Rahim", "01822222222", user.RolePatient)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(Enqueue(ctx, pool, Msg{UserID: u, Kind: "a", Title: "In-app only", Body: "hello"}))
	must(Enqueue(ctx, pool, Msg{UserID: u, Kind: "b", Title: "With SMS", Body: "your serial is 4", SMS: true}))
	must(Enqueue(ctx, pool, Msg{UserID: other, Kind: "c", Title: "Doomed SMS", Body: "never arrives", SMS: true}))

	sender := &flakySender{failures: 1}
	w := NewWorker(pool, sender, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// First run: the first SMS fails (retried later), the second goes out.
	if n, err := w.SendPending(ctx); err != nil || n != 1 {
		t.Fatalf("run 1: sent %d, %v", n, err)
	}
	if n, _ := w.SendPending(ctx); n != 1 || len(sender.sent) != 2 {
		t.Fatalf("run 2: sent %d, total %v", n, sender.sent)
	}
	if n, _ := w.SendPending(ctx); n != 0 {
		t.Fatalf("nothing should be left, sent %d", n)
	}

	// A message that keeps failing is given up after MaxSMSAttempts.
	must(Enqueue(ctx, pool, Msg{UserID: u, Kind: "d", Title: "x", Body: "y", SMS: true}))
	sender.failures = 100
	for range MaxSMSAttempts + 2 {
		_, _ = w.SendPending(ctx)
	}
	var status string
	var attempts int
	must(pool.QueryRow(ctx, `SELECT sms_status, sms_attempts FROM notifications WHERE kind = 'd'`).Scan(&status, &attempts))
	if status != "failed" || attempts != MaxSMSAttempts {
		t.Fatalf("status=%s attempts=%d", status, attempts)
	}

	s := NewStore(pool)
	list, unread, err := s.List(ctx, u, false, 20, 0)
	if err != nil || len(list) != 3 || unread != 3 || list[0].Kind != "d" {
		t.Fatalf("list: %d items, %d unread, %v", len(list), unread, err)
	}
	must(s.MarkRead(ctx, u, list[0].ID))
	if err := s.MarkRead(ctx, other, list[1].ID); err == nil {
		t.Fatal("marked someone else's notification as read")
	}
	if _, unread, _ = s.List(ctx, u, false, 20, 0); unread != 2 {
		t.Fatalf("unread after one read = %d", unread)
	}
	must(s.MarkAllRead(ctx, u))
	if only, unread, _ := s.List(ctx, u, true, 20, 0); unread != 0 || len(only) != 0 {
		t.Fatalf("after read-all: %d unread", unread)
	}
}
