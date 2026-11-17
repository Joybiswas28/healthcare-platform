package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"healthcare-platform/internal/user"
)

var (
	ErrInvalidCredentials = errors.New("invalid phone or password")
	ErrValidation         = errors.New("validation failed")
)

// UserStore is the subset of the user repository the auth service needs.
type UserStore interface {
	Create(ctx context.Context, u user.User) (user.User, error)
	GetByPhone(ctx context.Context, phone string) (user.User, error)
	GetByID(ctx context.Context, id string) (user.User, error)
	UpdatePassword(ctx context.Context, id, passwordHash string) error
}

// PhoneVerifier checks a one-time code sent to a phone (OTPStore in production).
type PhoneVerifier interface {
	Consume(ctx context.Context, phone, purpose, code string) error
}

type Service struct {
	users  UserStore
	tokens *TokenManager
	otp    PhoneVerifier
}

func NewService(users UserStore, tokens *TokenManager, otp PhoneVerifier) *Service {
	return &Service{users: users, tokens: tokens, otp: otp}
}

type RegisterInput struct {
	Name     string    `json:"name"`
	Phone    string    `json:"phone"`
	Password string    `json:"password"`
	Role     user.Role `json:"role"`
	OTPCode  string    `json:"otp_code"` // from POST /auth/otp with purpose "register"
}

type ResetPasswordInput struct {
	Phone       string `json:"phone"`
	OTPCode     string `json:"otp_code"` // from POST /auth/otp with purpose "reset_password"
	NewPassword string `json:"new_password"`
}

func validPassword(p string) error {
	if len(p) < 8 || len(p) > 72 {
		return ValidationError{"password must be 8-72 characters"}
	}
	return nil
}

type LoginInput struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type AuthResult struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	User        user.User `json:"user"`
}

// ValidationError carries a user-facing message and wraps ErrValidation.
type ValidationError struct{ Msg string }

func (e ValidationError) Error() string { return e.Msg }
func (e ValidationError) Unwrap() error { return ErrValidation }

// Only these roles may self-register (doctors and hospitals are then verified by
// admins); the rest are created by admins.
var selfRegisterRoles = map[user.Role]bool{
	user.RolePatient: true, user.RoleDoctor: true, user.RoleHospitalAdmin: true,
	user.RolePharmacy: true, user.RoleRider: true,
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (AuthResult, error) {
	name := strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(name) < 2 {
		return AuthResult{}, ValidationError{"name must be at least 2 characters"}
	}
	phone, err := NormalizePhone(in.Phone)
	if err != nil {
		return AuthResult{}, ValidationError{err.Error()}
	}
	if err := validPassword(in.Password); err != nil {
		return AuthResult{}, err
	}
	if in.Role == "" {
		in.Role = user.RolePatient
	}
	if !selfRegisterRoles[in.Role] {
		return AuthResult{}, ValidationError{"role must be patient, doctor, hospital_admin, pharmacy or rider"}
	}
	if strings.TrimSpace(in.OTPCode) == "" {
		return AuthResult{}, ValidationError{"otp_code is required; request one with POST /auth/otp"}
	}
	if err := s.otp.Consume(ctx, phone, PurposeRegister, strings.TrimSpace(in.OTPCode)); err != nil {
		return AuthResult{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResult{}, err
	}

	u, err := s.users.Create(ctx, user.User{Name: name, Phone: phone, PasswordHash: string(hash), Role: in.Role})
	if err != nil {
		return AuthResult{}, err
	}
	return s.issue(u)
}

func (s *Service) Login(ctx context.Context, in LoginInput) (AuthResult, error) {
	phone, err := NormalizePhone(in.Phone)
	if err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}
	u, err := s.users.GetByPhone(ctx, phone)
	if errors.Is(err, user.ErrNotFound) {
		return AuthResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return AuthResult{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		return AuthResult{}, ErrInvalidCredentials
	}
	if u.Status != "active" {
		return AuthResult{}, ErrInvalidCredentials
	}
	return s.issue(u)
}

// ResetPassword sets a new password after the user proves they hold the phone.
// Tokens issued before the reset stay valid until they expire.
func (s *Service) ResetPassword(ctx context.Context, in ResetPasswordInput) error {
	phone, err := NormalizePhone(in.Phone)
	if err != nil {
		return ValidationError{err.Error()}
	}
	if err := validPassword(in.NewPassword); err != nil {
		return err
	}
	if err := s.otp.Consume(ctx, phone, PurposeResetPassword, strings.TrimSpace(in.OTPCode)); err != nil {
		return err
	}
	u, err := s.users.GetByPhone(ctx, phone)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, u.ID, string(hash))
}

func (s *Service) Me(ctx context.Context, userID string) (user.User, error) {
	return s.users.GetByID(ctx, userID)
}

func (s *Service) issue(u user.User) (AuthResult, error) {
	token, exp, err := s.tokens.Issue(u.ID, u.Role)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{AccessToken: token, ExpiresAt: exp, User: u}, nil
}
