package auth

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"
)

// captureSMS records messages instead of sending them.
type captureSMS struct {
	mu   sync.Mutex
	sent map[string][]string
	fail bool
}

func (c *captureSMS) Send(_ context.Context, phone, text string) error {
	if c.fail {
		return errors.New("provider down")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent[phone] = append(c.sent[phone], text)
	return nil
}

var sixDigits = regexp.MustCompile(`\b\d{6}\b`)

func (c *captureSMS) lastCode(t *testing.T, phone string) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.sent[phone]
	if len(msgs) == 0 {
		t.Fatalf("no SMS sent to %s", phone)
	}
	return sixDigits.FindString(msgs[len(msgs)-1])
}

func TestOTP(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	sms := &captureSMS{sent: map[string][]string{}}
	users := user.NewRepository(pool)
	o := NewOTPStore(pool, sms, users, "test-secret-test-secret-test-secret")
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	o.now = func() time.Time { return now }
	advance := func(d time.Duration) { now = now.Add(d) }

	// Register code flow.
	if err := o.Request(ctx, "+8801711111111", PurposeRegister); err != nil {
		t.Fatal(err)
	}
	code := sms.lastCode(t, "01711111111")
	if err := o.Request(ctx, "01711111111", PurposeRegister); !errors.Is(err, ErrTooManyRequests) {
		t.Fatalf("second request within a minute: %v", err)
	}
	if err := o.Consume(ctx, "01711111111", PurposeResetPassword, code); !errors.Is(err, ErrValidation) {
		t.Fatalf("code used for the wrong purpose: %v", err)
	}
	if err := o.Consume(ctx, "01711111111", PurposeRegister, "000000"); !errors.Is(err, ErrValidation) {
		t.Fatalf("wrong code: %v", err)
	}
	if err := o.Consume(ctx, "01711111111", PurposeRegister, code); err != nil {
		t.Fatalf("right code: %v", err)
	}
	if err := o.Consume(ctx, "01711111111", PurposeRegister, code); !errors.Is(err, ErrValidation) {
		t.Fatalf("code reused: %v", err)
	}

	// Expiry.
	advance(2 * time.Minute)
	if err := o.Request(ctx, "01711111111", PurposeRegister); err != nil {
		t.Fatal(err)
	}
	code = sms.lastCode(t, "01711111111")
	advance(6 * time.Minute)
	if err := o.Consume(ctx, "01711111111", PurposeRegister, code); !errors.Is(err, ErrValidation) {
		t.Fatalf("expired code accepted: %v", err)
	}

	// Brute force: the code is burned after 5 wrong attempts.
	if err := o.Request(ctx, "01711111111", PurposeRegister); err != nil {
		t.Fatal(err)
	}
	code = sms.lastCode(t, "01711111111")
	for range otpMaxAttempts {
		_ = o.Consume(ctx, "01711111111", PurposeRegister, "999999")
	}
	if err := o.Consume(ctx, "01711111111", PurposeRegister, code); !errors.Is(err, ErrValidation) {
		t.Fatalf("right code accepted after too many wrong attempts: %v", err)
	}

	// Hourly limit: 5 codes per number per hour (3 used so far).
	for range 2 {
		advance(2 * time.Minute)
		if err := o.Request(ctx, "01711111111", PurposeRegister); err != nil {
			t.Fatal(err)
		}
	}
	advance(2 * time.Minute)
	if err := o.Request(ctx, "01711111111", PurposeRegister); !errors.Is(err, ErrTooManyRequests) {
		t.Fatalf("sixth code in an hour: %v", err)
	}

	// Registered numbers can't get register codes; unknown numbers silently get no reset code.
	dbtest.CreateUser(t, pool, "Existing", "01822222222", user.RolePatient)
	if err := o.Request(ctx, "01822222222", PurposeRegister); !errors.Is(err, user.ErrPhoneTaken) {
		t.Fatalf("register code for existing number: %v", err)
	}
	if err := o.Request(ctx, "01933333333", PurposeResetPassword); err != nil {
		t.Fatalf("reset for unknown number should look successful: %v", err)
	}
	if len(sms.sent["01933333333"]) != 0 {
		t.Fatal("SMS sent to a number with no account")
	}
	if err := o.Request(ctx, "01822222222", PurposeResetPassword); err != nil || sms.lastCode(t, "01822222222") == "" {
		t.Fatalf("reset code for existing number: %v", err)
	}

	// A failed SMS doesn't use up the user's allowance.
	sms.fail = true
	if err := o.Request(ctx, "01644444444", PurposeRegister); err == nil {
		t.Fatal("expected send failure")
	}
	sms.fail = false
	if err := o.Request(ctx, "01644444444", PurposeRegister); err != nil {
		t.Fatalf("retry after failed send: %v", err)
	}
}
