package db

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// integrationDB fails closed unless both the parsed connection URL and the
// database selected by PostgreSQL name a *_test database. Some probe tests
// execute destructive DDL, so either signal alone is insufficient.
func integrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set -- skipping integration test")
	}
	parsed, err := url.Parse(databaseURL)
	require.NoError(t, err, "DATABASE_URL must parse before integration DDL")
	parsedName := strings.TrimPrefix(parsed.EscapedPath(), "/")
	decodedName, err := url.PathUnescape(parsedName)
	require.NoError(t, err, "DATABASE_URL database path must decode")
	require.True(t, strings.HasSuffix(decodedName, "_test"),
		"refusing schema-probe integration DDL: DATABASE_URL database %q does not end in _test", decodedName)

	db, err := Open(databaseURL)
	require.NoError(t, err)
	var currentDatabase string
	require.NoError(t, db.QueryRow(`SELECT current_database()`).Scan(&currentDatabase))
	require.True(t, strings.HasSuffix(currentDatabase, "_test"),
		"refusing schema-probe integration DDL: connected database %q does not end in _test", currentDatabase)
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
	// bridge_test is fully migrated, so session_canvases and its related
	// session/enum sentinels exist.
	err := CheckSchemaProbe(context.Background(), db)
	require.NoError(t, err)
}

func TestExpectedSchemaProbe_TracksSessionCanvases(t *testing.T) {
	require.Equal(t, "session_canvases", ExpectedSchemaProbe)
	require.Len(t, ExpectedSchemaSentinels.Tables, 2)
	primary := ExpectedSchemaSentinels.Tables[0]
	require.Equal(t, "session_canvases", primary.Table)
	require.ElementsMatch(t, []string{
		"id", "session_id", "owner_id", "title", "visibility", "yjs_state", "plain_text", "created_at", "updated_at",
	}, primary.Columns)
	require.Empty(t, primary.Constraints)
	require.ElementsMatch(t, []string{
		"session_canvases_session_idx", "session_canvases_session_owner_idx",
	}, primary.Indexes)
	require.Equal(t, SchemaTableSentinels{Table: "sessions", Columns: []string{"canvas_floor"}}, ExpectedSchemaSentinels.Tables[1])
	require.Equal(t, []SchemaEnumSentinel{{Name: "canvas_visibility", Labels: []string{"private", "host", "participants", "session"}}}, ExpectedSchemaSentinels.Enums)
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
	assert.Contains(t, msg, "approved database-change workflow")
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
