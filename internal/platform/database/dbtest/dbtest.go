// Package dbtest gives integration tests an isolated, migrated PostgreSQL schema.
//
// Tests are skipped unless TEST_DATABASE_URL is set. Each call creates a fresh
// schema and drops it when the test ends, so packages can run in parallel.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/platform/database"
	"healthcare-platform/internal/user"
)

func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	schema := "test_" + hex.EncodeToString(b)

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(ctx)
	})

	if err := database.Migrate(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// CreateUser inserts a user with a dummy password hash and returns its id.
func CreateUser(t *testing.T, pool *pgxpool.Pool, name, phone string, role user.Role) string {
	t.Helper()
	u, err := user.NewRepository(pool).Create(context.Background(), user.User{
		Name: name, Phone: phone, PasswordHash: "x", Role: role,
	})
	if err != nil {
		t.Fatalf("create user %s: %v", phone, err)
	}
	return u.ID
}
