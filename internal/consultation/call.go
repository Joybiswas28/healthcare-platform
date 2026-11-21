package consultation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/httpx"
)

// Video calls run browser to browser (WebRTC). The server relays the setup
// messages ("signals") between the patient and the doctor of an active
// consultation, and tells browsers which STUN/TURN servers to use.

// Signal kinds. offer, answer and candidate carry WebRTC data the server does
// not read; hangup and decline end or refuse a call.
var signalKinds = map[string]bool{"offer": true, "answer": true, "candidate": true, "hangup": true, "decline": true}

const (
	maxSignalPayload = 64 << 10
	// signalTTL is how long a signal is worth delivering: a call that has
	// rung this long without an answer has been given up on.
	signalTTL = 90 * time.Second
	// Signals older than this are deleted as new ones arrive.
	signalKeep = 10 * time.Minute
)

type Signal struct {
	ID        int64     `json:"id"`
	SenderID  string    `json:"sender_id"`
	Kind      string    `json:"kind"`
	Payload   *string   `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

type SignalInput struct {
	Kind    string `json:"kind"`
	Payload string `json:"payload"`
}

// ICEServer is a STUN or TURN server, in the shape RTCPeerConnection takes.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// DefaultICEServers is a public STUN server. It lets most home and office
// connections reach each other; some mobile networks also need a TURN server.
var DefaultICEServers = []ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}

// SendSignal relays one call signal to the other participant. Calls can only
// start and connect in an active consultation; hangup and decline are always
// accepted so a call can end cleanly when the consultation closes.
func (s *Store) SendSignal(ctx context.Context, userID, id string, in SignalInput) error {
	if !signalKinds[in.Kind] {
		return apperr.Validation("kind must be offer, answer, candidate, hangup or decline")
	}
	if len(in.Payload) > maxSignalPayload {
		return apperr.Validation("payload is too large")
	}
	c, err := s.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	if c.Status != StatusActive && in.Kind != "hangup" && in.Kind != "decline" {
		return apperr.Conflict("video calls are only possible while the consultation is active (it is %s)", c.Status)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM call_signals WHERE consultation_id = $1 AND created_at < now() - $2::interval`,
		id, fmt.Sprintf("%d seconds", int(signalKeep.Seconds()))); err != nil {
		return err
	}
	var payload *string
	if in.Payload != "" {
		payload = &in.Payload
	}
	if _, err := tx.Exec(ctx, `INSERT INTO call_signals (consultation_id, sender_id, kind, payload) VALUES ($1, $2, $3, $4)`,
		id, userID, in.Kind, payload); err != nil {
		return err
	}
	if in.Kind == "offer" {
		other, caller := c.PatientID, c.DoctorName
		if userID == c.PatientID {
			other, caller = c.DoctorID, c.PatientName
		}
		if err := notify.Enqueue(ctx, tx, notify.Msg{
			UserID: other, Kind: notify.KindConsultationCall, Title: "Incoming video call",
			Body: fmt.Sprintf("%s is calling you about: %s", caller, c.Subject),
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Signals returns recent signals from the other participant after the given id.
func (s *Store) Signals(ctx context.Context, userID, id string, after int64) ([]Signal, error) {
	rows, err := s.db.Query(ctx, `
		SELECT g.id, g.sender_id, g.kind, g.payload, g.created_at
		FROM call_signals g
		JOIN consultations c ON c.id = g.consultation_id
		WHERE g.consultation_id = $1 AND (c.patient_id = $2 OR c.doctor_id = $2)
		  AND g.sender_id <> $2 AND g.id > $3 AND g.created_at > now() - $4::interval
		ORDER BY g.id LIMIT 200`,
		id, userID, after, fmt.Sprintf("%d seconds", int(signalTTL.Seconds())))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Signal
	for rows.Next() {
		var g Signal
		if err := rows.Scan(&g.ID, &g.SenderID, &g.Kind, &g.Payload, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// WithICEServers sets the STUN/TURN servers handed to browsers.
func (h *Handler) WithICEServers(servers []ICEServer) *Handler {
	h.ice = servers
	return h
}

// GET /consultations/{consultationID}/call
func (h *Handler) CallConfig(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "consultationID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	c, err := h.store.Get(r.Context(), caller(r).UserID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ice := h.ice
	if len(ice) == 0 {
		ice = DefaultICEServers
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ice_servers": ice, "available": c.Status == StatusActive})
}

// POST /consultations/{consultationID}/call/signals
func (h *Handler) PostSignal(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "consultationID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSignalPayload+4096)
	var in SignalInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.SendSignal(r.Context(), caller(r).UserID, id, in); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CallEvents streams the other participant's call signals as Server-Sent
// Events (event: signal), starting with those of the last 90 seconds so a
// browser that opens the page while being called still sees the offer.
//
//	GET /consultations/{consultationID}/call/events
func (h *Handler) CallEvents(w http.ResponseWriter, r *http.Request) {
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
	var last int64
	if v, err := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); err == nil && v > 0 {
		last = v
	}

	wake, unsubscribe := h.hub.Subscribe(id)
	defer unsubscribe()

	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache, no-transform")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ctx := r.Context()
	flush := func() error {
		sigs, err := h.store.Signals(ctx, userID, id, last)
		if err != nil {
			return err
		}
		for i := range sigs {
			data, err := json.Marshal(sigs[i])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "id: %d\nevent: signal\ndata: %s\n\n", sigs[i].ID, data); err != nil {
				return err
			}
			last = sigs[i].ID
		}
		return rc.Flush()
	}

	if _, err := fmt.Fprint(w, "retry: 2000\n\n"); err != nil {
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
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case <-wake:
			if err := flush(); err != nil {
				return
			}
		}
	}
}
