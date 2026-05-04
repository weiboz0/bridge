package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// integrationDB returns a real DB handle for the local bridge_test
// database; tests that don't need a DB use the unit-style branches
// instead.
func integrationDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set -- skipping integration test")
	}
	db, err := Open(url)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCheckSchemaProbe_NilDB(t *testing.T) {
	err := CheckSchemaProbe(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil DB")
}

func TestCheckSchemaProbe_HappyPath(t *testing.T) {
	db := integrationDB(t)
	// bridge_test is fully migrated, so parent_links exists.
	err := CheckSchemaProbe(context.Background(), db)
	require.NoError(t, err)
}

func TestCheckSchemaProbe_NullToRegclass(t *testing.T) {
	// to_regclass returns NULL for a non-existent table without
	// raising. Verify by probing a deliberately-bogus name through
	// the same query the probe uses, so the probe's NullString
	// branch is exercised against real Postgres semantics.
	db := integrationDB(t)
	var result sql.NullString
	err := db.QueryRowContext(
		context.Background(),
		`SELECT to_regclass($1)::text`,
		"public.this_table_intentionally_does_not_exist_plan_068",
	).Scan(&result)
	require.NoError(t, err)
	assert.False(t, result.Valid, "to_regclass should return NULL (not error) for a missing table")
}

func TestErrSchemaProbeMissing_Format(t *testing.T) {
	// Direct test of the error shape so callers can rely on the
	// wording. The actual missing-table integration path requires a
	// full schema teardown which is more invasive than the value warrants;
	// the format check + NilDB + HappyPath cover the surface.
	err := &ErrSchemaProbeMissing{Table: "fake_table"}
	msg := err.Error()
	assert.Contains(t, msg, "fake_table")
	assert.Contains(t, msg, "drizzle/")
	assert.Contains(t, msg, "psql")
}

func TestCheckSchemaProbe_TypedError(t *testing.T) {
	// errors.As should work to extract the typed error and inspect the
	// missing table name programmatically.
	err := &ErrSchemaProbeMissing{Table: "x"}
	var typed *ErrSchemaProbeMissing
	assert.True(t, errors.As(err, &typed))
	assert.Equal(t, "x", typed.Table)
}

func TestCheckSchemaProbe_EmptyExpectedSchemaProbe(t *testing.T) {
	// Defensive — guards against a future refactor that nukes the const.
	// We can't easily mutate ExpectedSchemaProbe at runtime in Go without
	// reflection hacks; instead, this test exists as a sentinel to
	// document the contract.
	assert.NotEmpty(t, ExpectedSchemaProbe, "ExpectedSchemaProbe must be a non-empty table name")
}
