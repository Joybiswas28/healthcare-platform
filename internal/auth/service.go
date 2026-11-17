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
}

type Service struct {
	users  UserStore
	tokens *TokenManager
}

func NewService(users UserStore, tokens *TokenManager) *Service {
	return &Service{users: users, tokens: tokens}
}

type RegisterInput struct {
	Name     string    `json:"name"`
	Phone    string    `json:"phone"`
	Password string    `json:"password"`
	Role     user.Role `json:"role"`
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

// Only these roles may self-register; the rest are created by admins after verification.
var selfRegisterRoles = map[user.Role]bool{user.RolePatient: true, user.RoleDoctor: true}

func (s *Service) Register(ctx context.Context, in RegisterInput) (AuthResult, error) {
	name := strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(name) < 2 {
		return AuthResult{}, ValidationError{"name must be at least 2 characters"}
	}
	phone, err := NormalizePhone(in.Phone)
	if err != nil {
		return AuthResult{}, ValidationError{err.Error()}
	}
	if len(in.Password) < 8 || len(in.Password) > 72 {
		return AuthResult{}, ValidationError{"password must be 8-72 characters"}
	}
	if in.Role == "" {
		in.Role = user.RolePatient
	}
	if !selfRegisterRoles[in.Role] {
		return AuthResult{}, ValidationError{"role must be 'patient' or 'doctor'"}
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
