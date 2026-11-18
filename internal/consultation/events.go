package consultation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"healthcare-platform/internal/platform/httpx"
)

// heartbeatInterval keeps proxies and the browser from closing an idle stream.
const heartbeatInterval = 25 * time.Second

// Events streams new chat messages as Server-Sent Events:
//
//	GET /consultations/{consultationID}/events?after=<last seen message id>
//
// It uses the normal Authorization header, so a website can relay it through its
// own server without ever handing the token to the browser (unlike the WebSocket,
// which needs the token in the URL). Each message is one event whose id is the
// message id; a reconnecting EventSource sends it back as Last-Event-ID and the
// stream resumes after it. Messages are posted with the REST endpoint.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
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
	last := resumeAfter(r)

	// Subscribe before reading the backlog so nothing slips through the gap.
	wake, unsubscribe := h.hub.Subscribe(id)
	defer unsubscribe()

	rc := http.NewResponseController(w)
	// The server's read/write timeouts must not cut a long-lived stream. (The read
	// side is how the server notices the client went away.)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	// no-transform stops proxies (and Next.js) from compressing and so buffering the stream.
	hdr.Set("Cache-Control", "no-cache, no-transform")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ctx := r.Context()
	flush := func() error {
		for {
			msgs, err := h.store.Messages(ctx, userID, id, last, batchSize)
			if err != nil {
				return err
			}
			for i := range msgs {
				data, err := json.Marshal(msgs[i])
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(w, "id: %d\nevent: message\ndata: %s\n\n", msgs[i].ID, data); err != nil {
					return err
				}
				last = msgs[i].ID
			}
			if err := rc.Flush(); err != nil {
				return err
			}
			if len(msgs) < batchSize {
				return nil
			}
		}
	}

	// Ask EventSource to wait 3 s before reconnecting, then send the backlog.
	if _, err := fmt.Fprint(w, "retry: 3000\n\n"); err != nil {
		return
	}
	if err := flush(); err != nil {
		return
	}
	beat := time.NewTicker(heartbeatInterval)
	defer beat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-beat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		case <-wake:
			if err := flush(); err != nil {
				return
			}
		}
	}
}

// resumeAfter is the last message id the client has: Last-Event-ID on a
// reconnect, otherwise the after query parameter.
func resumeAfter(r *http.Request) int64 {
	for _, v := range []string{r.Header.Get("Last-Event-ID"), r.URL.Query().Get("after")} {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
