package consultation

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/realtime"
	"healthcare-platform/internal/user"
)

type sseEvent struct {
	id   string
	name string
	data string
}

// readEvent reads the next real event, skipping comments (heartbeats) and the retry hint.
func readEvent(t *testing.T, r *bufio.Reader) sseEvent {
	t.Helper()
	var ev sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			if ev.name != "" {
				return ev
			}
		case strings.HasPrefix(line, "id: "):
			ev.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			ev.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			ev.data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestEventsStreamDeliversAndResumes(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	doc := f.onlineDoctor(t, 1)
	patient := f.patientWithTokens(t, 1)
	outsider := f.patientWithTokens(t, 0)
	c, _ := f.store.Request(ctx, patient, request("Event stream test", doc))
	f.store.Accept(ctx, doc, c.ID)
	if _, err := f.store.Confirm(ctx, patient, c.ID); err != nil {
		t.Fatal(err)
	}
	first, _ := f.store.PostMessage(ctx, patient, c.ID, "before connecting")

	hub := realtime.NewHub()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ready := make(chan struct{})
	go realtime.Listen(realtime.WithReady(ctx, ready), f.pool, hub, log)
	<-ready

	tm := auth.NewTokenManager("test-secret-test-secret-test-secret", time.Hour)
	r := chi.NewRouter()
	r.With(auth.Authenticate(tm, nil)).Get("/consultations/{consultationID}/events", NewHandler(f.store, hub, log).Events)
	srv := httptest.NewServer(r)
	defer srv.Close()

	open := func(userID string, role user.Role, lastEventID string) *http.Response {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/consultations/"+c.ID+"/events", nil)
		tok, _, _ := tm.Issue(userID, role, 0)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "text/event-stream")
		if lastEventID != "" {
			req.Header.Set("Last-Event-ID", lastEventID)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	if resp := open(outsider, user.RolePatient, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("outsider: status %d, want 404", resp.StatusCode)
	}

	resp := open(doc, user.RoleDoctor, "")
	if ct := resp.Header.Get("Content-Type"); resp.StatusCode != http.StatusOK || ct != "text/event-stream" {
		t.Fatalf("status %d, content type %q", resp.StatusCode, ct)
	}
	stream := bufio.NewReader(resp.Body)

	ev := readEvent(t, stream)
	if ev.name != "message" || ev.id != strconv.FormatInt(first.ID, 10) {
		t.Fatalf("backlog event = %+v", ev)
	}

	// Posted elsewhere (REST), delivered live.
	live, _ := f.store.PostMessage(ctx, patient, c.ID, "are you there?")
	ev = readEvent(t, stream)
	var m Message
	if err := json.Unmarshal([]byte(ev.data), &m); err != nil || m.ID != live.ID || *m.Body != "are you there?" {
		t.Fatalf("live event = %+v (%v)", ev, err)
	}
	resp.Body.Close()

	// A reconnect with Last-Event-ID gets only what came after it.
	missed, _ := f.store.PostMessage(ctx, doc, c.ID, "sent while you were away")
	resp = open(patient, user.RolePatient, strconv.FormatInt(live.ID, 10))
	defer resp.Body.Close()
	ev = readEvent(t, bufio.NewReader(resp.Body))
	if ev.id != strconv.FormatInt(missed.ID, 10) {
		t.Fatalf("after resume got event %s, want %d", ev.id, missed.ID)
	}
}
