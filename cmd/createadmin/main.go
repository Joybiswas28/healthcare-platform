// Command createadmin creates a platform administrator account.
//
//	go run ./cmd/createadmin -name "Admin" -phone 01700000000 -password "a-strong-password"
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/platform/database"
	"healthcare-platform/internal/user"
)

func main() {
	name := flag.String("name", "", "admin full name")
	phone := flag.String("phone", "", "admin mobile number (01XXXXXXXXX)")
	password := flag.String("password", "", "password (min 12 characters)")
	flag.Parse()

	if err := run(*name, *phone, *password); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(name, rawPhone, password string) error {
	_ = godotenv.Load()
	if name == "" || rawPhone == "" {
		return fmt.Errorf("-name and -phone are required")
	}
	if len(password) < 12 || len(password) > 72 {
		return fmt.Errorf("-password must be 12-72 characters")
	}
	phone, err := auth.NormalizePhone(rawPhone)
	if err != nil {
		return err
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrate(pool); err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u, err := user.NewRepository(pool).Create(ctx, user.User{
		Name: name, Phone: phone, PasswordHash: string(hash), Role: user.RoleAdmin,
	})
	if err != nil {
		return err
	}
	fmt.Printf("admin created: id=%s phone=%s\n", u.ID, u.Phone)
	return nil
}
