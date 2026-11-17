package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"healthcare-platform/internal/user"
)

var ErrInvalidToken = errors.New("invalid or expired token")

type Claims struct {
	Role user.Role `json:"role"`
	jwt.RegisteredClaims
}

// TokenManager issues and verifies HS256-signed JWT access tokens.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), ttl: ttl, now: time.Now}
}

func (m *TokenManager) Issue(userID string, role user.Role) (token string, expiresAt time.Time, err error) {
	now := m.now()
	expiresAt = now.Add(m.ttl)
	claims := Claims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return token, expiresAt, nil
}

func (m *TokenManager) Verify(token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(m.now),
		jwt.WithExpirationRequired(),
	)
	if err != nil || claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
