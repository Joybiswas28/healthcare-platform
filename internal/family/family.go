// Package family lets a patient keep profiles for family members who have no
// account of their own, and book appointments and consultations for them.
package family

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

// MaxMembers caps how many family members one account can keep.
const MaxMembers = 10

var Relations = map[string]bool{"spouse": true, "child": true, "parent": true, "sibling": true, "other": true}

type Member struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Relation  string    `json:"relation"`
	Gender    *string   `json:"gender"`
	BirthDate *string   `json:"birth_date"` // YYYY-MM-DD
	CreatedAt time.Time `json:"created_at"`
}

type Input struct {
	Name      string `json:"name"`
	Relation  string `json:"relation"`
	Gender    string `json:"gender"`
	BirthDate string `json:"birth_date"`
}

func (in *Input) normalize(today time.Time) error {
	in.Name = strings.Join(strings.Fields(in.Name), " ")
	in.Relation = strings.TrimSpace(in.Relation)
	in.Gender = strings.TrimSpace(in.Gender)
	in.BirthDate = strings.TrimSpace(in.BirthDate)
	if n := utf8.RuneCountInString(in.Name); n < 2 || n > 100 {
		return apperr.Validation("name must be 2-100 characters")
	}
	if !Relations[in.Relation] {
		return apperr.Validation("relation must be spouse, child, parent, sibling or other")
	}
	switch in.Gender {
	case "", "male", "female", "other":
	default:
		return apperr.Validation("gender must be male, female or other")
	}
	if in.BirthDate != "" {
		d, err := time.Parse(time.DateOnly, in.BirthDate)
		if err != nil {
			return apperr.Validation("birth_date must be YYYY-MM-DD")
		}
		if d.After(today) || d.Before(today.AddDate(-130, 0, 0)) {
			return apperr.Validation("birth_date must be in the past 130 years")
		}
	}
	return nil
}

type Store struct {
	db  *pgxpool.Pool
	now func() time.Time
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db, now: time.Now} }

var errNotFound = apperr.NotFound("family member not found")

const columns = `id, name, relation, gender, to_char(birth_date, 'YYYY-MM-DD'), created_at`

func scan(row pgx.Row) (Member, error) {
	var m Member
	err := row.Scan(&m.ID, &m.Name, &m.Relation, &m.Gender, &m.BirthDate, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, errNotFound
	}
	return m, err
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Store) today() time.Time {
	return s.now().UTC().Add(6 * time.Hour) // Asia/Dhaka, no daylight saving
}

// List returns the account's family members, oldest first.
func (s *Store) List(ctx context.Context, ownerID string) ([]Member, error) {
	rows, err := s.db.Query(ctx, `SELECT `+columns+` FROM family_members
		WHERE owner_id = $1 AND removed_at IS NULL ORDER BY created_at, id`, ownerID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Member, error) { return scan(r) })
}

func (s *Store) Add(ctx context.Context, ownerID string, in Input) (Member, error) {
	if err := in.normalize(s.today()); err != nil {
		return Member{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Member{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Lock the owner so two parallel adds can't both pass the limit.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, ownerID); err != nil {
		return Member{}, err
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM family_members WHERE owner_id = $1 AND removed_at IS NULL`,
		ownerID).Scan(&n); err != nil {
		return Member{}, err
	}
	if n >= MaxMembers {
		return Member{}, apperr.Conflict("you can keep at most %d family members", MaxMembers)
	}
	m, err := scan(tx.QueryRow(ctx, `
		INSERT INTO family_members (owner_id, name, relation, gender, birth_date)
		VALUES ($1, $2, $3, $4, $5::date) RETURNING `+columns,
		ownerID, in.Name, in.Relation, nullIfEmpty(in.Gender), nullIfEmpty(in.BirthDate)))
	if err != nil {
		return Member{}, err
	}
	return m, tx.Commit(ctx)
}

func (s *Store) Update(ctx context.Context, ownerID, id string, in Input) (Member, error) {
	if err := in.normalize(s.today()); err != nil {
		return Member{}, err
	}
	return scan(s.db.QueryRow(ctx, `
		UPDATE family_members SET name = $3, relation = $4, gender = $5, birth_date = $6::date, updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND removed_at IS NULL RETURNING `+columns,
		id, ownerID, in.Name, in.Relation, nullIfEmpty(in.Gender), nullIfEmpty(in.BirthDate)))
}

// Remove hides a member. Bookings already made for them are kept.
func (s *Store) Remove(ctx context.Context, ownerID, id string) error {
	tag, err := s.db.Exec(ctx, `UPDATE family_members SET removed_at = now()
		WHERE id = $1 AND owner_id = $2 AND removed_at IS NULL`, id, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound
	}
	return nil
}

// Check confirms that memberID is a current family member of ownerID; other
// packages call it before booking for a member. An empty memberID means the
// account holder and always passes.
func Check(ctx context.Context, q database.Querier, ownerID, memberID string) error {
	if memberID == "" {
		return nil
	}
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM family_members
		WHERE id = $1 AND owner_id = $2 AND removed_at IS NULL)`, memberID, ownerID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return errNotFound
	}
	return nil
}
