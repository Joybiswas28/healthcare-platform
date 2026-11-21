package consultation

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/realtime"
	"healthcare-platform/internal/user"
)

func TestCallSignals(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	doc := f.onlineDoctor(t, 1)
	patient := f.patientWithTokens(t, 1)
	outsider := f.patientWithTokens(t, 0)
	c, _ := f.store.Request(ctx, patient, request("Video call test", doc))

	// No calls before the consultation is active, except ending one.
	err := f.store.SendSignal(ctx, doc, c.ID, SignalInput{Kind: "offer", Payload: "{}"})
	wantKind(t, err, apperr.KindConflict, "offer while requested")
	if err := f.store.SendSignal(ctx, doc, c.ID, SignalInput{Kind: "hangup"}); err != nil {
		t.Fatalf("hangup while requested: %v", err)
	}

	f.store.Accept(ctx, doc, c.ID)
	if _, err := f.store.Confirm(ctx, patient, c.ID); err != nil {
		t.Fatal(err)
	}
	wantKind(t, f.store.SendSignal(ctx, doc, c.ID, SignalInput{Kind: "shout"}), apperr.KindValidation, "unknown kind")
	wantKind(t, f.store.SendSignal(ctx, outsider, c.ID, SignalInput{Kind: "offer"}), apperr.KindNotFound, "outsider")

	if err := f.store.SendSignal(ctx, doc, c.ID, SignalInput{Kind: "offer", Payload: `{"type":"offer","sdp":"v=0"}`}); err != nil {
		t.Fatal(err)
	}
	var rings int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND kind = 'consultation_call'`, patient).Scan(&rings); err != nil || rings != 1 {
		t.Fatalf("patient call notifications = %d, %v", rings, err)
	}

	// Each side sees only the other's signals.
	got, err := f.store.Signals(ctx, patient, c.ID, 0)
	if err != nil || len(got) != 2 || got[0].Kind != "hangup" || got[1].Kind != "offer" {
		t.Fatalf("patient sees %+v, %v", got, err)
	}
	if mine, _ := f.store.Signals(ctx, doc, c.ID, 0); len(mine) != 0 {
		t.Fatalf("doctor sees own signals: %+v", mine)
	}
	if theirs, _ := f.store.Signals(ctx, outsider, c.ID, 0); len(theirs) != 0 {
		t.Fatalf("outsider sees %+v", theirs)
	}
	// Old signals are not delivered.
	if _, err := f.pool.Exec(ctx, `UPDATE call_signals SET created_at = now() - interval '5 minutes' WHERE kind = 'hangup'`); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.store.Signals(ctx, patient, c.ID, 0); len(got) != 1 || got[0].Kind != "offer" {
		t.Fatalf("after ageing: %+v", got)
	}

	// The event stream delivers the backlog and then new signals live.
	hub := realtime.NewHub()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ready := make(chan struct{})
	go realtime.Listen(realtime.WithReady(ctx, ready), f.pool, hub, log)
	<-ready
	tm := auth.NewTokenManager("test-secret-test-secret-test-secret", time.Hour)
	h := NewHandler(f.store, hub, log)
	r := chi.NewRouter()
	r.With(auth.Authenticate(tm, nil)).Get("/consultations/{consultationID}/call/events", h.CallEvents)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/consultations/"+c.ID+"/call/events", nil)
	tok, _, _ := tm.Issue(patient, user.RolePatient, 0)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("open stream: %v, %v", resp, err)
	}
	defer resp.Body.Close()
	stream := bufio.NewReader(resp.Body)
	if ev := readEvent(t, stream); ev.name != "signal" {
		t.Fatalf("backlog = %+v", ev)
	}
	if err := f.store.SendSignal(ctx, doc, c.ID, SignalInput{Kind: "candidate", Payload: `{"candidate":"x"}`}); err != nil {
		t.Fatal(err)
	}
	ev := readEvent(t, stream)
	var s Signal
	if err := json.Unmarshal([]byte(ev.data), &s); err != nil || s.Kind != "candidate" || s.SenderID != doc {
		t.Fatalf("live signal = %+v (%v)", ev, err)
	}
}
