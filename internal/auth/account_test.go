package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"healthcare-platform/internal/user"
)

func newAccountService(t *testing.T) (*Service, *TokenManager, AuthResult) {
	t.Helper()
	tm := NewTokenManager("test-secret-test-secret-test-secret", time.Hour)
	svc := NewService(&memStore{byPhone: map[string]user.User{}}, tm, fixedOTP{})
	res, err := svc.Register(context.Background(), RegisterInput{
		Name: "Karim Uddin", Phone: "01811111111", Password: "old-password", OTPCode: "123456",
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, tm, res
}

func TestUpdateProfile(t *testing.T) {
	ctx := context.Background()
	svc, _, reg := newAccountService(t)
	id := reg.User.ID

	u, err := svc.UpdateProfile(ctx, id, ProfileInput{Name: "  Karim U.  ", Email: " Karim@Example.COM "})
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "Karim U." || u.Email == nil || *u.Email != "karim@example.com" {
		t.Fatalf("name/email not trimmed and lower-cased: %q %v", u.Name, u.Email)
	}

	if u, _ = svc.UpdateProfile(ctx, id, ProfileInput{Name: "Karim"}); u.Email != nil {
		t.Fatalf("an empty email should clear it, got %q", *u.Email)
	}

	for _, bad := range []ProfileInput{
		{Name: "K"},
		{Name: "Karim", Email: "not-an-email"},
		{Name: "Karim", Email: "Karim <karim@example.com>"},
	} {
		if _, err := svc.UpdateProfile(ctx, id, bad); !errors.Is(err, ErrValidation) {
			t.Errorf("%+v: want a validation error, got %v", bad, err)
		}
	}
}

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	svc, tm, reg := newAccountService(t)
	id := reg.User.ID

	if _, err := svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: "wrong", NewPassword: "new-password"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("wrong current password: got %v", err)
	}
	if _, err := svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: "old-password", NewPassword: "short"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("too short: got %v", err)
	}
	if _, err := svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: "old-password", NewPassword: "old-password"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("same password: got %v", err)
	}

	res, err := svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: "old-password", NewPassword: "new-password"})
	if err != nil {
		t.Fatal(err)
	}

	// The fresh token carries the new version, so the session checker keeps this
	// device logged in while every older token stops matching.
	oldClaims, _ := tm.Verify(reg.AccessToken)
	newClaims, err := tm.Verify(res.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if newClaims.TokenVersion != res.User.TokenVersion || oldClaims.TokenVersion == newClaims.TokenVersion {
		t.Fatalf("token versions: old %d, new %d, user %d", oldClaims.TokenVersion, newClaims.TokenVersion, res.User.TokenVersion)
	}

	if _, err := svc.Login(ctx, LoginInput{Phone: "01811111111", Password: "old-password"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password still works: %v", err)
	}
	if _, err := svc.Login(ctx, LoginInput{Phone: "01811111111", Password: "new-password"}); err != nil {
		t.Fatalf("new password: %v", err)
	}
}
