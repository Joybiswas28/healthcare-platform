package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"healthcare-platform/internal/user"
)

type fakeStates struct {
	status  string
	version int
	calls   int
}

func (f *fakeStates) AuthState(_ context.Context, _ string) (string, int, error) {
	f.calls++
	return f.status, f.version, nil
}

func TestSessionChecker(t *testing.T) {
	tm := NewTokenManager("test-secret-test-secret-test-secret", time.Hour)
	states := &fakeStates{status: "active", version: 3}
	sc := NewSessionChecker(states)
	now := time.Now()
	sc.now = func() time.Time { return now }

	h := Authenticate(tm, sc)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	call := func(tv int) int {
		tok, _, _ := tm.Issue("u1", user.RolePatient, tv)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := call(3); got != 204 {
		t.Fatalf("current token: %d", got)
	}
	if got := call(2); got != 401 {
		t.Fatalf("token from before a password reset: %d", got)
	}
	if states.calls != 1 {
		t.Fatalf("lookups = %d, want 1 (cached)", states.calls)
	}

	// Suspension takes effect once the cache entry expires (or at once via Forget).
	states.status = "suspended"
	if got := call(3); got != 204 {
		t.Fatalf("still cached: %d", got)
	}
	sc.Forget("u1")
	if got := call(3); got != 401 {
		t.Fatalf("suspended account: %d", got)
	}
	states.status = "active"
	now = now.Add(CacheTTL + time.Second)
	if got := call(3); got != 204 {
		t.Fatalf("reactivated after cache expiry: %d", got)
	}
}

func TestLoginRejectsSuspendedAccount(t *testing.T) {
	ctx := context.Background()
	store := &memStore{byPhone: map[string]user.User{}}
	svc := NewService(store, NewTokenManager("test-secret-test-secret-test-secret", time.Hour), fixedOTP{})
	if _, err := svc.Register(ctx, RegisterInput{Name: "Rahim", Phone: "01812345678", Password: "secret123", OTPCode: "123456"}); err != nil {
		t.Fatal(err)
	}
	u := store.byPhone["01812345678"]
	u.Status = "suspended"
	store.byPhone["01812345678"] = u
	if _, err := svc.Login(ctx, LoginInput{Phone: "01812345678", Password: "secret123"}); !errors.Is(err, ErrAccountSuspended) {
		t.Fatalf("suspended login: %v", err)
	}
	if _, err := svc.Login(ctx, LoginInput{Phone: "01899999999", Password: "secret123"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown phone: %v", err)
	}
}
