package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	emailFlag := flag.String("email", "admin@efi.et", "Admin user email")
	passFlag := flag.String("password", "AdminSecurePass123!", "Admin user password")
	roleFlag := flag.String("role", "admin", "User role (admin or moderator)")
	dbURLFlag := flag.String("db", "", "Database URL (defaults to APP_DATABASE_URL or ADMIN_DATABASE_URL)")
	flag.Parse()

	email := strings.ToLower(strings.TrimSpace(*emailFlag))
	password := *passFlag
	role := strings.ToLower(strings.TrimSpace(*roleFlag))

	if email == "" {
		log.Fatal("email cannot be empty")
	}
	if len(password) < 8 {
		log.Fatal("password must be at least 8 characters long")
	}
	if role != "admin" && role != "moderator" {
		log.Fatalf("invalid role %q: must be 'admin' or 'moderator'", role)
	}

	dbURL := *dbURLFlag
	if dbURL == "" {
		dbURL = os.Getenv("APP_DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = os.Getenv("ADMIN_DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://efi_app:efi_app_pass@localhost:5432/efi_dev?sslmode=disable"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("failed to hash password: %v", err)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var (
		id        int64
		resEmail  string
		resRole   string
		createdAt time.Time
	)

	query := `
		INSERT INTO users (email, password_hash, role, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (email) DO UPDATE
		SET password_hash = EXCLUDED.password_hash,
		    role = EXCLUDED.role
		RETURNING id, email, role, created_at;
	`

	err = db.QueryRowContext(ctx, query, email, string(hash), role).Scan(&id, &resEmail, &resRole, &createdAt)
	if err != nil {
		log.Fatalf("failed to seed admin user: %v", err)
	}

	fmt.Printf("Successfully seeded user [ID: %d, Email: %s, Role: %s, Created: %s]\n", id, resEmail, resRole, createdAt.Format(time.RFC3339))
}
