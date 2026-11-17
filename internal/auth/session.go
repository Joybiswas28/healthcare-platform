package auth

import (
	"context"
	"errors"
	"sync"
	"time"

	"healthcare-platform/internal/user"
)

// ErrSessionRevoked means the token was valid when issued but the account has since
// been suspended or its password reset.
var ErrSessionRevoked = errors.New("session is no longer valid; please log in again")

// AuthStateStore reads a user's status and token version.
type AuthStateStore interface {
	AuthState(ctx context.Context, id string) (status string, tokenVersion int, err error)
}

// SessionChecker rejects tokens of suspended accounts and tokens issued before the
// user's last password reset or suspension. Lookups are cached briefly, so a revoked
// token stops working within CacheTTL while most requests need no database query.
type SessionChecker struct {
	store AuthStateStore
	ttl   time.Duration
	now   func() time.Time

	mu    sync.Mutex
	cache map[string]sessionEntry
}

type sessionEntry struct {
	status  string
	version int
	at      time.Time
}

const CacheTTL = 30 * time.Second

func NewSessionChecker(store AuthStateStore) *SessionChecker {
	return &SessionChecker{store: store, ttl: CacheTTL, now: time.Now, cache: map[string]sessionEntry{}}
}

func (c *SessionChecker) Check(ctx context.Context, claims *Claims) error {
	now := c.now()
	c.mu.Lock()
	e, ok := c.cache[claims.Subject]
	c.mu.Unlock()

	if !ok || now.Sub(e.at) > c.ttl {
		status, version, err := c.store.AuthState(ctx, claims.Subject)
		if errors.Is(err, user.ErrNotFound) {
			return ErrSessionRevoked
		}
		if err != nil {
			return err
		}
		e = sessionEntry{status: status, version: version, at: now}
		c.mu.Lock()
		if len(c.cache) > 100_000 { // crude bound; entries are tiny and short-lived
			c.cache = map[string]sessionEntry{}
		}
		c.cache[claims.Subject] = e
		c.mu.Unlock()
	}
	if e.status != "active" || claims.TokenVersion != e.version {
		return ErrSessionRevoked
	}
	return nil
}

// Forget drops a cached entry so a change on this server takes effect at once.
func (c *SessionChecker) Forget(userID string) {
	c.mu.Lock()
	delete(c.cache, userID)
	c.mu.Unlock()
}
