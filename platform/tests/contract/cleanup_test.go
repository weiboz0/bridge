package contract

import (
	"database/sql"
	"log"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestMain runs cleanup after all contract tests to remove test data
// from whichever database the Go server is connected to.
func TestMain(m *testing.M) {
	code := m.Run()

	// Clean up test data created during contract tests
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://work@127.0.0.1:5432/bridge"
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Printf("contract test cleanup: failed to connect to DB: %v", err)
		os.Exit(code)
	}
	defer db.Close()

	// Delete test users and their related records
	queries := []string{
		`DELETE FROM auth_providers WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'contract-%@example.com')`,
		`DELETE FROM org_memberships WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'contract-%@example.com')`,
		`DELETE FROM org_memberships WHERE org_id IN (SELECT id FROM organizations WHERE slug LIKE 'contract-%' OR slug LIKE 'dup-slug-%')`,
		`DELETE FROM organizations WHERE slug LIKE 'contract-%' OR slug LIKE 'dup-slug-%'`,
		`DELETE FROM users WHERE email LIKE 'contract-%@example.com'`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			log.Printf("contract test cleanup: %v", err)
		}
	}

	os.Exit(code)
}
