package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/user"
)

const (
	PurposeRegister      = "register"
	PurposeResetPassword = "reset_password"

	otpTTL         = 5 * time.Minute
	otpCooldown    = time.Minute // between two codes to the same number
	otpHourlyLimit = 5           // codes per number per hour
	otpMaxAttempts = 5           // wrong guesses before a code is burned
)

var ErrTooManyRequests = errors.New("too many code requests; please wait a minute and try again")

// SMSSender delivers a text message. notify.LogSender and real SMS providers satisfy it.
type SMSSender interface {
	Send(ctx context.Context, phone, text string) error
}

// OTPStore issues and checks 6-digit one-time codes sent by SMS.
// Only an HMAC of each code is stored.
type OTPStore struct {
	db    *pgxpool.Pool
	sms   SMSSender
	users UserStore
	key   []byte
	now   func() time.Time
}

func NewOTPStore(db *pgxpool.Pool, sms SMSSender, users UserStore, secret string) *OTPStore {
	key := sha256.Sum256([]byte("otp:" + secret))
	return &OTPStore{db: db, sms: sms, users: users, key: key[:], now: time.Now}
}

func (o *OTPStore) hash(phone, purpose, code string) string {
	mac := hmac.New(sha256.New, o.key)
	mac.Write([]byte(purpose + "|" + phone + "|" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

// Request sends a code to rawPhone for the given purpose.
//
// For registration, an already registered number is refused. For password reset,
// an unknown number gets no SMS but the call still succeeds, so the endpoint cannot
// be used to find out which numbers have accounts.
func (o *OTPStore) Request(ctx context.Context, rawPhone, purpose string) error {
	phone, err := NormalizePhone(rawPhone)
	if err != nil {
		return ValidationError{err.Error()}
	}
	if purpose != PurposeRegister && purpose != PurposeResetPassword {
		return ValidationError{"purpose must be 'register' or 'reset_password'"}
	}

	_, err = o.users.GetByPhone(ctx, phone)
	exists := err == nil
	if err != nil && !errors.Is(err, user.ErrNotFound) {
		return err
	}
	if purpose == PurposeRegister && exists {
		return user.ErrPhoneTaken
	}
	if purpose == PurposeResetPassword && !exists {
		return nil
	}

	now := o.now()
	var lastHour int
	var last *time.Time
	if err := o.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE created_at > $2), max(created_at)
		FROM otp_codes WHERE phone = $1 AND created_at > $2`,
		phone, now.Add(-time.Hour)).Scan(&lastHour, &last); err != nil {
		return err
	}
	if lastHour >= otpHourlyLimit || (last != nil && now.Sub(*last) < otpCooldown) {
		return ErrTooManyRequests
	}

	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%06d", n.Int64())

	var id int64
	if err := o.db.QueryRow(ctx, `
		INSERT INTO otp_codes (phone, purpose, code_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		phone, purpose, o.hash(phone, purpose, code), now.Add(otpTTL), now).Scan(&id); err != nil {
		return err
	}

	text := fmt.Sprintf("Your verification code is %s. It expires in %d minutes. Never share this code with anyone.",
		code, int(otpTTL.Minutes()))
	if err := o.sms.Send(ctx, phone, text); err != nil {
		// Don't let a failed send count against the user's limit.
		_, _ = o.db.Exec(ctx, `DELETE FROM otp_codes WHERE id = $1`, id)
		return fmt.Errorf("send otp: %w", err)
	}
	return nil
}

// Consume checks a code and marks it used. Each code works once, and is burned
// after too many wrong guesses.
func (o *OTPStore) Consume(ctx context.Context, phone, purpose, code string) error {
	tx, err := o.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	var stored string
	var attempts int
	err = tx.QueryRow(ctx, `
		SELECT id, code_hash, attempts FROM otp_codes
		WHERE phone = $1 AND purpose = $2 AND consumed_at IS NULL AND expires_at > $3
		ORDER BY created_at DESC, id DESC LIMIT 1
		FOR UPDATE`, phone, purpose, o.now()).Scan(&id, &stored, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return ValidationError{"the code is invalid or has expired; request a new one"}
	}
	if err != nil {
		return err
	}
	if attempts >= otpMaxAttempts {
		return ValidationError{"too many wrong attempts; request a new code"}
	}
	if !hmac.Equal([]byte(stored), []byte(o.hash(phone, purpose, code))) {
		if _, err := tx.Exec(ctx, `UPDATE otp_codes SET attempts = attempts + 1 WHERE id = $1`, id); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return ValidationError{"incorrect code"}
	}
	if _, err := tx.Exec(ctx, `UPDATE otp_codes SET consumed_at = $2 WHERE id = $1`, id, o.now()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
