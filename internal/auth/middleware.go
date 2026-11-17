package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"healthcare-platform/internal/platform/httpx"
	"healthcare-platform/internal/user"
)

type ctxKey struct{}

// Principal is the authenticated caller attached to the request context.
type Principal struct {
	UserID string
	Role   user.Role
}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// Authenticate requires a valid "Authorization: Bearer <token>" header. When
// sessions is not nil, it also rejects tokens of suspended accounts and tokens
// issued before a password reset.
func Authenticate(tm *TokenManager, sessions *SessionChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			// Browsers cannot set headers on WebSocket requests, so for an upgrade
			// the token may come in the access_token query parameter instead.
			if !ok && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				raw = r.URL.Query().Get("access_token")
				ok = raw != ""
			}
			if !ok || raw == "" {
				httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			claims, err := tm.Verify(raw)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, err.Error())
				return
			}
			if sessions != nil {
				if err := sessions.Check(r.Context(), claims); err != nil {
					if errors.Is(err, ErrSessionRevoked) {
						httpx.Error(w, http.StatusUnauthorized, err.Error())
					} else {
						httpx.Error(w, http.StatusServiceUnavailable, "could not verify session")
					}
					return
				}
			}
			ctx := context.WithValue(r.Context(), ctxKey{}, Principal{UserID: claims.Subject, Role: claims.Role})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole allows the request only if the caller has one of the given roles.
// Must run after Authenticate.
func RequireRole(roles ...user.Role) func(http.Handler) http.Handler {
	allowed := make(map[user.Role]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := FromContext(r.Context())
			if !ok {
				httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
				return
			}
			if !allowed[p.Role] {
				httpx.Error(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
