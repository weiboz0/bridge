package contract

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contractCleanupTimeout = 5 * time.Second

func TestResolveContractCleanupURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantCleanup bool
	}{
		{
			name: "absent URL skips without fallback",
		},
		{
			name:        "accepts test database with safe query options",
			url:         "postgresql://work@127.0.0.1:5432/bridge_test?sslmode=disable&application_name=contract-tests",
			wantCleanup: true,
		},
		{
			name:        "accepts percent decoded test database",
			url:         "postgresql://work@127.0.0.1:5432/bridge%5Ftest?sslmode=disable",
			wantCleanup: true,
		},
		{
			name: "rejects non test database",
			url:  "postgresql://work@127.0.0.1:5432/bridge",
		},
		{
			name: "rejects literal fragment",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test#fragment",
		},
		{
			name: "rejects encoded fragment",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test%23fragment",
		},
		{
			name: "rejects authority multi host",
			url:  "postgresql://work@host-one,host-two:5432/bridge_test",
		},
		{
			name: "rejects query multi host routing",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?host=host-one,host-two",
		},
		{
			name: "rejects encoded query multi host routing",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?host=host-one%2Chost-two",
		},
		{
			name: "rejects repeated query host routing",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?host=host-one&host=host-two",
		},
		{
			name: "rejects hostaddr multi host routing",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?hostaddr=127.0.0.1,127.0.0.2",
		},
		{
			name: "rejects encoded hostaddr multi host routing",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?hostaddr=127.0.0.1%2C127.0.0.2",
		},
		{
			name: "rejects single host override",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?host=other",
		},
		{
			name: "rejects case insensitive host override",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?HOST=other",
		},
		{
			name: "rejects port override",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?port=5433",
		},
		{
			name: "rejects dbname override",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?dbname=bridge",
		},
		{
			name: "rejects database override",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?database=bridge",
		},
		{
			name: "rejects service routing",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?service=unsafe",
		},
		{
			name: "rejects target session routing",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?target_session_attrs=read-write",
		},
		{
			name: "rejects load balance routing",
			url:  "postgresql://work@127.0.0.1:5432/bridge_test?load_balance_hosts=enable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, cleanup, err := resolveContractCleanupURL(tt.url)
			if tt.wantCleanup {
				require.NoError(t, err)
				assert.Equal(t, tt.url, url)
				assert.True(t, cleanup)
				return
			}
			if tt.url == "" {
				require.NoError(t, err)
				assert.Empty(t, url)
				assert.False(t, cleanup)
				return
			}
			require.Error(t, err)
			assert.Empty(t, url)
			assert.False(t, cleanup)
		})
	}
}

func TestValidateTestDatabaseURLParity(t *testing.T) {
	storeValidator := formattedValidatorDeclaration(t, filepath.Join("..", "..", "internal", "store", "orgs_test.go"))
	contractValidator := formattedValidatorDeclaration(t, "cleanup_test.go")
	assert.Equal(t, storeValidator, contractValidator)
}

func formattedValidatorDeclaration(t *testing.T, path string) string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "validateTestDatabaseURL" {
			continue
		}
		var formatted bytes.Buffer
		require.NoError(t, format.Node(&formatted, fset, function))
		return formatted.String()
	}
	t.Fatalf("validateTestDatabaseURL not found in %s", path)
	return ""
}

func TestCleanupExitCode(t *testing.T) {
	assert.Equal(t, 1, cleanupExitCode(0))
	assert.Equal(t, 2, cleanupExitCode(2))
}

// TestMain runs cleanup after all contract tests to remove test data
// from a verified test database.
func TestMain(m *testing.M) {
	code := m.Run()
	if err := cleanupContractTestData(os.Getenv("DATABASE_URL")); err != nil {
		log.Printf("contract test cleanup: %v", err)
		code = cleanupExitCode(code)
	}
	os.Exit(code)
}

func cleanupExitCode(testCode int) int {
	if testCode != 0 {
		return testCode
	}
	return 1
}

func resolveContractCleanupURL(dbURL string) (string, bool, error) {
	if dbURL == "" {
		return "", false, nil
	}
	if _, err := validateTestDatabaseURL(dbURL); err != nil {
		return "", false, err
	}
	return dbURL, true, nil
}

func cleanupContractTestData(dbURL string) (cleanupErr error) {
	validatedURL, shouldCleanup, err := resolveContractCleanupURL(dbURL)
	if err != nil || !shouldCleanup {
		return err
	}

	db, err := sql.Open("pgx", validatedURL)
	if err != nil {
		return fmt.Errorf("failed to open verified test database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("close verified test database: %w", err))
		}
	}()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	pingCtx, pingCancel := context.WithTimeout(context.Background(), contractCleanupTimeout)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("verified test database ping failed: %w", err)
	}

	databaseCtx, databaseCancel := context.WithTimeout(context.Background(), contractCleanupTimeout)
	defer databaseCancel()
	var liveDatabase string
	if err := db.QueryRowContext(databaseCtx, "SELECT current_database()").Scan(&liveDatabase); err != nil {
		return fmt.Errorf("verified test database name query failed: %w", err)
	}
	if !strings.HasSuffix(liveDatabase, "_test") {
		return fmt.Errorf("connected database must end in _test")
	}

	// Delete test users and their related records
	queries := []string{
		`DELETE FROM auth_providers WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'contract-%@example.com')`,
		`DELETE FROM org_memberships WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'contract-%@example.com')`,
		`DELETE FROM org_memberships WHERE org_id IN (SELECT id FROM organizations WHERE slug LIKE 'contract-%' OR slug LIKE 'dup-slug-%')`,
		`DELETE FROM organizations WHERE slug LIKE 'contract-%' OR slug LIKE 'dup-slug-%'`,
		`DELETE FROM users WHERE email LIKE 'contract-%@example.com'`,
	}

	for _, q := range queries {
		deleteCtx, deleteCancel := context.WithTimeout(context.Background(), contractCleanupTimeout)
		_, err := db.ExecContext(deleteCtx, q)
		deleteCancel()
		if err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete contract fixture rows: %w", err))
		}
	}
	return cleanupErr
}

func validateTestDatabaseURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("database URL is empty")
	}
	if strings.Contains(rawURL, "#") || strings.Contains(strings.ToLower(rawURL), "%23") {
		return "", fmt.Errorf("database URL must not contain a fragment")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("database URL is invalid: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", fmt.Errorf("database URL must use a PostgreSQL scheme")
	}
	if parsed.Host == "" || strings.Contains(parsed.Host, ",") || strings.Contains(parsed.Hostname(), ",") {
		return "", fmt.Errorf("database URL must name one host")
	}
	for key := range parsed.Query() {
		switch strings.ToLower(key) {
		case "host", "hostaddr", "port", "dbname", "database", "service", "servicefile", "target_session_attrs", "load_balance_hosts":
			return "", fmt.Errorf("database URL must not override connection routing")
		}
	}

	escapedPath := strings.TrimPrefix(parsed.EscapedPath(), "/")
	databaseName, err := url.PathUnescape(escapedPath)
	if err != nil {
		return "", fmt.Errorf("database URL path is invalid: %w", err)
	}
	if databaseName == "" || strings.Contains(databaseName, "/") {
		return "", fmt.Errorf("database URL must name one database")
	}
	config, err := pgx.ParseConfig(rawURL)
	if err != nil {
		return "", fmt.Errorf("database URL pgx configuration is invalid: %w", err)
	}
	if config.Host == "" || len(config.Fallbacks) != 0 {
		return "", fmt.Errorf("database URL must resolve to one host")
	}
	if config.Database == "" || !strings.HasSuffix(config.Database, "_test") {
		return "", fmt.Errorf("database URL database must end in _test")
	}
	return config.Database, nil
}
