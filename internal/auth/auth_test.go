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

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"01712345678", "01712345678", true},
		{"+8801712345678", "01712345678", true},
		{"8801712345678", "01712345678", true},
		{"017-1234 5678", "01712345678", true},
		{"01212345678", "", false}, // 012 is not a valid operator prefix
		{"0171234567", "", false},  // too short
		{"abc", "", false},
	}
	for _, c := range cases {
		got, err := NormalizePhone(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q, ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
}

func TestTokenRoundTripAndExpiry(t *testing.T) {
	tm := NewTokenManager("test-secret-test-secret-test-secret", time.Hour)
	tok, _, err := tm.Issue("user-1", user.RoleDoctor, 0)
	if err != nil {
		t.Fatal(err)
	}
	c, err := tm.Verify(tok)
	if err != nil || c.Subject != "user-1" || c.Role != user.RoleDoctor {
		t.Fatalf("Verify = %+v, %v", c, err)
	}

	tm.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := tm.Verify(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token: got %v, want ErrInvalidToken", err)
	}

	other := NewTokenManager("another-secret-another-secret-xx", time.Hour)
	if _, err := other.Verify(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong secret: got %v, want ErrInvalidToken", err)
	}
}

// memStore is an in-memory UserStore for service tests.
type memStore struct{ byPhone map[string]user.User }

func (m *memStore) Create(_ context.Context, u user.User) (user.User, error) {
	if _, ok := m.byPhone[u.Phone]; ok {
		return user.User{}, user.ErrPhoneTaken
	}
	u.ID, u.Status = "id-"+u.Phone, "active"
	m.byPhone[u.Phone] = u
	return u, nil
}

func (m *memStore) GetByPhone(_ context.Context, p string) (user.User, error) {
	if u, ok := m.byPhone[p]; ok {
		return u, nil
	}
	return user.User{}, user.ErrNotFound
}

func (m *memStore) GetByID(_ context.Context, id string) (user.User, error) {
	for _, u := range m.byPhone {
		if u.ID == id {
			return u, nil
		}
	}
	return user.User{}, user.ErrNotFound
}

func (m *memStore) UpdatePassword(_ context.Context, id, hash string) error {
	for p, u := range m.byPhone {
		if u.ID == id {
			u.PasswordHash = hash
			m.byPhone[p] = u
			return nil
		}
	}
	return user.ErrNotFound
}

// fixedOTP accepts only the code "123456".
type fixedOTP struct{}

func (fixedOTP) Consume(_ context.Context, _, _, code string) error {
	if code != "123456" {
		return ValidationError{"incorrect code"}
	}
	return nil
}

func TestRegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	svc := NewService(&memStore{byPhone: map[string]user.User{}}, NewTokenManager("test-secret-test-secret-test-secret", time.Hour), fixedOTP{})

	reg := func(name, phone string, role user.Role, otp string) (AuthResult, error) {
		return svc.Register(ctx, RegisterInput{Name: name, Phone: phone, Password: "secret123", Role: role, OTPCode: otp})
	}

	if _, err := reg("Rahim", "01812345678", "", ""); !errors.Is(err, ErrValidation) {
		t.Fatalf("missing otp: got %v", err)
	}
	if _, err := reg("Rahim", "01812345678", "", "000000"); !errors.Is(err, ErrValidation) {
		t.Fatalf("wrong otp: got %v", err)
	}

	res, err := reg("Rahim", "+8801812345678", "", "123456")
	if err != nil {
		t.Fatal(err)
	}
	if res.User.Role != user.RolePatient || res.User.Phone != "01812345678" || res.AccessToken == "" {
		t.Fatalf("unexpected result: %+v", res)
	}

	if _, err := reg("X", "01812345678", "", "123456"); !errors.Is(err, ErrValidation) {
		t.Fatalf("short name: got %v", err)
	}
	if _, err := reg("Admin", "01912345678", user.RoleAdmin, "123456"); !errors.Is(err, ErrValidation) {
		t.Fatalf("admin self-register must be rejected, got %v", err)
	}
	if _, err := reg("Rahim", "01812345678", "", "123456"); !errors.Is(err, user.ErrPhoneTaken) {
		t.Fatalf("duplicate phone: got %v", err)
	}

	if _, err := svc.Login(ctx, LoginInput{Phone: "01812345678", Password: "secret123"}); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := svc.Login(ctx, LoginInput{Phone: "01812345678", Password: "wrong-pass"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: got %v", err)
	}

	if err := svc.ResetPassword(ctx, ResetPasswordInput{Phone: "01812345678", OTPCode: "123456", NewPassword: "brand-new-pass"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, LoginInput{Phone: "01812345678", Password: "brand-new-pass"}); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}

func TestRequireRole(t *testing.T) {
	tm := NewTokenManager("test-secret-test-secret-test-secret", time.Hour)
	h := Authenticate(tm, nil)(RequireRole(user.RoleDoctor)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	call := func(header string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	doc, _, _ := tm.Issue("d1", user.RoleDoctor, 0)
	pat, _, _ := tm.Issue("p1", user.RolePatient, 0)

	if got := call(""); got != http.StatusUnauthorized {
		t.Errorf("no token: %d", got)
	}
	if got := call("Bearer " + pat); got != http.StatusForbidden {
		t.Errorf("patient: %d", got)
	}
	if got := call("Bearer " + doc); got != http.StatusNoContent {
		t.Errorf("doctor: %d", got)
	}
}
