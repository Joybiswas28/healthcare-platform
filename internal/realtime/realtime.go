// Package realtime fans out database notifications to connected clients.
//
// PostgreSQL announces each new consultation message with NOTIFY (see migration
// 00004). Every API server LISTENs and wakes the WebSocket connections watching
// that consultation, so chat works the same with one server or many.
package realtime

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Channel is the PostgreSQL NOTIFY channel for new consultation messages.
const Channel = "consultation_messages"

// Hub delivers wake-up signals to subscribers of a topic (a consultation id).
// Signals carry no data: a woken subscriber fetches whatever is new. Each
// subscriber channel holds at most one pending signal, so a slow client never
// blocks the hub and bursts are coalesced.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: map[string]map[chan struct{}]struct{}{}}
}

// Subscribe returns a channel that is signalled when topic has news, and a
// function that must be called to unsubscribe.
func (h *Hub) Subscribe(topic string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	if h.subs[topic] == nil {
		h.subs[topic] = map[chan struct{}]struct{}{}
	}
	h.subs[topic][ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.subs[topic], ch)
		if len(h.subs[topic]) == 0 {
			delete(h.subs, topic)
		}
		h.mu.Unlock()
	}
}

// Publish wakes every subscriber of topic.
func (h *Hub) Publish(topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[topic] {
		signal(ch)
	}
}

// Broadcast wakes every subscriber, e.g. after a reconnect when notifications may have been missed.
func (h *Hub) Broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.subs {
		for ch := range set {
			signal(ch)
		}
	}
}

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default: // already has a pending signal
	}
}

// Listen forwards NOTIFY events to the hub until ctx ends, reconnecting on errors.
// It holds one dedicated database connection.
func Listen(ctx context.Context, pool *pgxpool.Pool, hub *Hub, log *slog.Logger) {
	for {
		err := listenOnce(ctx, pool, hub)
		if ctx.Err() != nil {
			return
		}
		log.Warn("realtime listener disconnected; retrying", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

type readyKey struct{}

// WithReady returns a context that makes Listen close ready once LISTEN is active.
// Tests use it to avoid publishing before the listener is in place.
func WithReady(ctx context.Context, ready chan struct{}) context.Context {
	return context.WithValue(ctx, readyKey{}, ready)
}

func listenOnce(ctx context.Context, pool *pgxpool.Pool, hub *Hub) error {
	pc, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	// Take the connection out of the pool for good: a LISTENing connection must not be reused.
	conn := pc.Hijack()
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	if ready, ok := ctx.Value(readyKey{}).(chan struct{}); ok {
		select {
		case <-ready:
		default:
			close(ready)
		}
	}
	// Anything sent while we were disconnected was missed: tell everyone to re-check.
	hub.Broadcast()

	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		if topic, _, ok := strings.Cut(n.Payload, ":"); ok {
			hub.Publish(topic)
		}
	}
}
