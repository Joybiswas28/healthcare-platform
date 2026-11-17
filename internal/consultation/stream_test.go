package consultation

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/realtime"
	"healthcare-platform/internal/user"
)

func consultationOff() doctor.ConsultationSettingsInput {
	return doctor.ConsultationSettingsInput{Enabled: false, FeeTokens: 1}
}

func TestStreamDeliversMessagesLive(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	doc := f.onlineDoctor(t, 1)
	patient := f.patientWithTokens(t, 1)
	outsider := f.patientWithTokens(t, 0)
	c, _ := f.store.Request(ctx, patient, request("Live chat test", doc))
	f.store.Accept(ctx, doc, c.ID)
	if _, err := f.store.Confirm(ctx, patient, c.ID); err != nil {
		t.Fatal(err)
	}
	first, err := f.store.PostMessage(ctx, patient, c.ID, "sent before connecting")
	if err != nil {
		t.Fatal(err)
	}

	hub := realtime.NewHub()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ready := make(chan struct{})
	go realtime.Listen(realtime.WithReady(ctx, ready), f.pool, hub, log)
	<-ready

	tm := auth.NewTokenManager("test-secret-test-secret-test-secret", time.Hour)
	h := NewHandler(f.store, hub, log)
	r := chi.NewRouter()
	r.With(auth.Authenticate(tm)).Get("/consultations/{consultationID}/ws", h.Stream)
	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := func(userID string, role user.Role) string {
		tok, _, _ := tm.Issue(userID, role)
		return "ws" + strings.TrimPrefix(srv.URL, "http") + "/consultations/" + c.ID + "/ws?access_token=" + tok
	}

	// Someone who is not part of the consultation is refused before the upgrade.
	if _, resp, err := websocket.Dial(ctx, wsURL(outsider, user.RolePatient), nil); err == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("outsider dial: err=%v resp=%v", err, resp)
	}

	conn, _, err := websocket.Dial(ctx, wsURL(doc, user.RoleDoctor), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	next := func() Event {
		t.Helper()
		var ev Event
		rctx, rcancel := context.WithTimeout(ctx, 5*time.Second)
		defer rcancel()
		if err := wsjson.Read(rctx, conn, &ev); err != nil {
			t.Fatalf("read: %v", err)
		}
		return ev
	}

	// Backlog first.
	if ev := next(); ev.Type != "message" || ev.Message.ID != first.ID {
		t.Fatalf("backlog event = %+v", ev)
	}

	// A message posted elsewhere (e.g. the patient's phone using REST) arrives live.
	if _, err := f.store.PostMessage(ctx, patient, c.ID, "are you there, doctor?"); err != nil {
		t.Fatal(err)
	}
	if ev := next(); ev.Message == nil || *ev.Message.Body != "are you there, doctor?" {
		t.Fatalf("live event = %+v", ev)
	}

	// Sending over the socket saves the message and echoes it back.
	if err := wsjson.Write(ctx, conn, map[string]string{"body": "yes, I am here"}); err != nil {
		t.Fatal(err)
	}
	if ev := next(); ev.Message == nil || ev.Message.SenderID != doc {
		t.Fatalf("echo event = %+v", ev)
	}

	// Invalid input produces an error event, and the connection stays usable.
	if err := wsjson.Write(ctx, conn, map[string]string{"body": "   "}); err != nil {
		t.Fatal(err)
	}
	if ev := next(); ev.Type != "error" {
		t.Fatalf("expected error event, got %+v", ev)
	}

	msgs, _ := f.store.Messages(ctx, patient, c.ID, 0, 50)
	if len(msgs) != 3 {
		t.Fatalf("stored messages = %d, want 3", len(msgs))
	}
	conn.Close(websocket.StatusNormalClosure, "")
}
