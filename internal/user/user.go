// Package user defines the user model and its PostgreSQL repository.
package user

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/platform/database"
)

type Role string

const (
	RolePatient       Role = "patient"
	RoleDoctor        Role = "doctor"
	RoleHospitalAdmin Role = "hospital_admin"
	RolePharmacy      Role = "pharmacy"
	RoleRider         Role = "rider"
	RoleAdmin         Role = "admin"
)

var (
	ErrNotFound   = errors.New("user not found")
	ErrPhoneTaken = errors.New("phone number already registered")
)

type User struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Phone        string    `json:"phone"`
	Email        *string   `json:"email,omitempty"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	Status       string    `json:"status"`
	TokenVersion int       `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const userColumns = `id, name, phone, email, password_hash, role, status, token_version, created_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Name, &u.Phone, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.TokenVersion, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// Create inserts a user. Doctors also get an empty doctor profile awaiting verification.
func (r *Repository) Create(ctx context.Context, u User) (User, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := scanUser(tx.QueryRow(ctx,
		`INSERT INTO users (name, phone, email, password_hash, role)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+userColumns,
		u.Name, u.Phone, u.Email, u.PasswordHash, u.Role,
	))
	if err != nil {
		if database.IsUniqueViolation(err) {
			return User{}, ErrPhoneTaken
		}
		return User{}, err
	}

	if created.Role == RoleDoctor {
		if _, err := tx.Exec(ctx, `INSERT INTO doctor_profiles (user_id) VALUES ($1)`, created.ID); err != nil {
			return User{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return created, nil
}

func (r *Repository) GetByPhone(ctx context.Context, phone string) (User, error) {
	return scanUser(r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE phone = $1`, phone))
}

// UpdatePassword sets a new password and signs the user out everywhere.
func (r *Repository) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET password_hash = $2, token_version = token_version + 1, updated_at = now()
		WHERE id = $1`, id, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AuthState returns what token checks need: whether the account is active and
// the current token version.
func (r *Repository) AuthState(ctx context.Context, id string) (status string, tokenVersion int, err error) {
	err = r.db.QueryRow(ctx, `SELECT status, token_version FROM users WHERE id = $1`, id).Scan(&status, &tokenVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	return status, tokenVersion, err
}

// SetStatus suspends or reactivates an account. Suspending signs the user out everywhere.
// Admin accounts cannot be changed this way.
func (r *Repository) SetStatus(ctx context.Context, id, status string) (User, error) {
	u, err := scanUser(r.db.QueryRow(ctx, `
		UPDATE users SET status = $2, token_version = token_version + 1, updated_at = now()
		WHERE id = $1 AND role <> 'admin'
		RETURNING `+userColumns, id, status))
	return u, err
}

func (r *Repository) GetByID(ctx context.Context, id string) (User, error) {
	return scanUser(r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}
