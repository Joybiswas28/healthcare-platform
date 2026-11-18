package user_test

import (
	"context"
	"testing"

	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"
)

func TestSearch(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := user.NewRepository(pool)
	dbtest.CreateUser(t, pool, "Karim Uddin", "01811111111", user.RolePatient)
	dbtest.CreateUser(t, pool, "Dr Karima Akter", "01722222222", user.RoleDoctor)
	dbtest.CreateUser(t, pool, "Rashed 100%", "01933333333", user.RoleRider)

	names := func(q, role string) []string {
		t.Helper()
		list, err := repo.Search(ctx, q, role, 20, 0)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, u := range list {
			out = append(out, u.Name)
		}
		return out
	}

	if got := names("karim", ""); len(got) != 2 {
		t.Errorf("name match is case-insensitive and partial: got %v", got)
	}
	if got := names("karim", "doctor"); len(got) != 1 || got[0] != "Dr Karima Akter" {
		t.Errorf("role filter: got %v", got)
	}
	if got := names("0172222", ""); len(got) != 1 || got[0] != "Dr Karima Akter" {
		t.Errorf("phone match: got %v", got)
	}
	if got := names("%", ""); len(got) != 1 || got[0] != "Rashed 100%" {
		t.Errorf("%% must match literally, not everything: got %v", got)
	}
	if got := names("", ""); len(got) != 3 {
		t.Errorf("empty query lists everyone: got %v", got)
	}
}
