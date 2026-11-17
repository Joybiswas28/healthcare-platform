package auth

import (
	"context"
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

// Authenticate requires a valid "Authorization: Bearer <token>" header.
func Authenticate(tm *TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || raw == "" {
				httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			claims, err := tm.Verify(raw)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, err.Error())
				return
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
