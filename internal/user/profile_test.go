package user_test

import (
	"context"
	"errors"
	"testing"

	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"
)

func TestUpdateProfileEmailIsUnique(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := user.NewRepository(pool)
	a := dbtest.CreateUser(t, pool, "Karim Uddin", "01811111111", user.RolePatient)
	b := dbtest.CreateUser(t, pool, "Nusrat Jahan", "01822222222", user.RolePatient)

	email := "karim@example.com"
	u, err := repo.UpdateProfile(ctx, a, "Karim U.", &email)
	if err != nil || u.Name != "Karim U." || u.Email == nil || *u.Email != email {
		t.Fatalf("update: %+v %v", u, err)
	}
	if _, err := repo.UpdateProfile(ctx, b, "Nusrat Jahan", &email); !errors.Is(err, user.ErrEmailTaken) {
		t.Fatalf("second account with the same email: got %v, want ErrEmailTaken", err)
	}
	if u, err := repo.UpdateProfile(ctx, a, "Karim U.", nil); err != nil || u.Email != nil {
		t.Fatalf("clearing the email: %+v %v", u, err)
	}
	if _, err := repo.UpdateProfile(ctx, b, "Nusrat Jahan", &email); err != nil {
		t.Fatalf("email is free again after clearing: %v", err)
	}
}
