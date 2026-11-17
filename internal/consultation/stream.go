package consultation

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/httpx"
)

// Event is what the server sends over the WebSocket.
type Event struct {
	Type    string   `json:"type"` // "message" | "error"
	Message *Message `json:"message,omitempty"`
	Error   string   `json:"error,omitempty"`
}

const (
	pingInterval = 30 * time.Second
	writeTimeout = 10 * time.Second
	batchSize    = 100
)

// Stream is a WebSocket for live chat:
//
//	GET /consultations/{consultationID}/ws?access_token=<jwt>&after=<last seen message id>
//
// The server first sends every message after `after`, then pushes new ones as they
// arrive. The client may send {"body": "..."} to post a message (same rules as the
// REST endpoint). Browsers cannot set headers on WebSocket requests, so the token
// is accepted as a query parameter on this route.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "consultationID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	userID := caller(r).UserID
	if _, err := h.store.Get(r.Context(), userID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)

	// Subscribe before reading the backlog so nothing slips through the gap.
	wake, unsubscribe := h.hub.Subscribe(id)
	defer unsubscribe()

	// The HTTP server's read/write timeouts must not apply to a long-lived socket.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	// Auth is by bearer token, not cookies, so a cross-origin page cannot ride on a
	// user's session; the origin check is therefore not needed for safety.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return // Accept has already written the HTTP error
	}
	defer conn.CloseNow()
	conn.SetReadLimit(16 << 10)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	send := func(ev Event) error {
		wctx, wcancel := context.WithTimeout(ctx, writeTimeout)
		defer wcancel()
		return wsjson.Write(wctx, conn, ev)
	}

	// Reader: messages typed by this client.
	go func() {
		defer cancel()
		for {
			var in messageInput
			if err := wsjson.Read(ctx, conn, &in); err != nil {
				return
			}
			if _, err := h.store.PostMessage(ctx, userID, id, in.Body); err != nil {
				msg := "could not send message"
				if e, ok := apperr.As(err); ok {
					msg = e.Msg
				} else {
					h.log.ErrorContext(ctx, "ws post message", "consultation", id, "err", err)
				}
				if send(Event{Type: "error", Error: msg}) != nil {
					return
				}
			}
			// The saved message comes back to every client (including this one) via the hub.
		}
	}()

	last := max(after, 0)
	flush := func() error {
		for {
			msgs, err := h.store.Messages(ctx, userID, id, last, batchSize)
			if err != nil {
				return err
			}
			for i := range msgs {
				if err := send(Event{Type: "message", Message: &msgs[i]}); err != nil {
					return err
				}
				last = msgs[i].ID
			}
			if len(msgs) < batchSize {
				return nil
			}
		}
	}

	if err := flush(); err != nil {
		return
	}
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "")
			return
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		case <-wake:
			if err := flush(); err != nil {
				return
			}
		}
	}
}
