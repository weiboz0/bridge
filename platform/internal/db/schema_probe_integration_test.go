package db

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckSchemaProbe_HappyPath_AllSentinels verifies that a fully-migrated
// bridge_test database passes all sentinel walks: table, columns, constraints,
// and indexes. Complements TestCheckSchemaProbe_HappyPath which predates the
// Plan 076 sentinel extension.
func TestCheckSchemaProbe_HappyPath_AllSentinels(t *testing.T) {
	db := integrationDB(t)
	err := CheckSchemaProbe(context.Background(), db)
	require.NoError(t, err, "fully-migrated bridge_test DB should pass all sentinels")
}

// TestCheckSchemaProbe_MissingColumn drops `description` from books,
// registers a re-CREATE cleanup, then asserts that CheckSchemaProbe returns
// *ErrSchemaSentinelMissing with Kind=="column" and Name=="description".
// (Drops a current ExpectedSchemaSentinels.Columns entry; plan 088 retired
// the prior parent_links.revoked_at sentinel.)
//
// Isolation pattern: DDL committed (not rolled back) so the probe's separate
// connection pool can observe the change. t.Cleanup re-adds the column so
// subsequent test runs start from a clean state.
func TestCheckSchemaProbe_MissingColumn(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `ALTER TABLE books DROP COLUMN description`)
	require.NoError(t, err, "precondition: drop description")

	// Register cleanup IMMEDIATELY after the destructive DDL succeeds so it
	// runs even if the assertion below panics or the test is skipped.
	t.Cleanup(func() {
		// Match drizzle/0026_books_and_chapters.sql — description text NOT NULL DEFAULT ''.
		_, cleanupErr := db.ExecContext(ctx, `ALTER TABLE books ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT ''`)
		if cleanupErr != nil {
			t.Errorf("cleanup: failed to restore description: %v", cleanupErr)
		}
	})

	probeErr := CheckSchemaProbe(ctx, db)
	require.Error(t, probeErr, "probe should fail when description is missing")

	var sentinel *ErrSchemaSentinelMissing
	require.True(t, errors.As(probeErr, &sentinel), "error should be *ErrSchemaSentinelMissing, got %T: %v", probeErr, probeErr)
	assert.Equal(t, "column", sentinel.Kind)
	assert.Equal(t, "description", sentinel.Name)
}

// TestCheckSchemaProbe_MissingConstraint drops `books_scope_id_required`,
// registers a re-ADD cleanup, then asserts that CheckSchemaProbe returns
// *ErrSchemaSentinelMissing with Kind=="constraint" and
// Name=="books_scope_id_required". (Drops a current
// ExpectedSchemaSentinels.Constraints entry; plan 088 retired the prior
// parent_links_status_check sentinel.)
func TestCheckSchemaProbe_MissingConstraint(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `ALTER TABLE books DROP CONSTRAINT books_scope_id_required`)
	require.NoError(t, err, "precondition: drop books_scope_id_required")

	t.Cleanup(func() {
		// Match drizzle/0026_books_and_chapters.sql:14-17.
		_, cleanupErr := db.ExecContext(ctx,
			`ALTER TABLE books ADD CONSTRAINT books_scope_id_required CHECK (
				(scope = 'platform' AND scope_id IS NULL) OR
				(scope = 'org'      AND scope_id IS NOT NULL)
			)`)
		if cleanupErr != nil {
			t.Errorf("cleanup: failed to restore books_scope_id_required: %v", cleanupErr)
		}
	})

	probeErr := CheckSchemaProbe(ctx, db)
	require.Error(t, probeErr, "probe should fail when books_scope_id_required is missing")

	var sentinel *ErrSchemaSentinelMissing
	require.True(t, errors.As(probeErr, &sentinel), "error should be *ErrSchemaSentinelMissing, got %T: %v", probeErr, probeErr)
	assert.Equal(t, "constraint", sentinel.Kind)
	assert.Equal(t, "books_scope_id_required", sentinel.Name)
}

// TestCheckSchemaProbe_MissingIndex drops `books_scope_idx`,
// registers a re-CREATE cleanup, then asserts that CheckSchemaProbe returns
// *ErrSchemaSentinelMissing with Kind=="index" and Name=="books_scope_idx".
// (Drops a current ExpectedSchemaSentinels.Indexes entry; plan 088 retired
// the prior parent_links_active_uniq sentinel.)
func TestCheckSchemaProbe_MissingIndex(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `DROP INDEX books_scope_idx`)
	require.NoError(t, err, "precondition: drop books_scope_idx")

	t.Cleanup(func() {
		// Match drizzle/0026_books_and_chapters.sql — composite index on
		// (scope, scope_id).
		_, cleanupErr := db.ExecContext(ctx,
			`CREATE INDEX IF NOT EXISTS books_scope_idx ON books (scope, scope_id)`)
		if cleanupErr != nil {
			t.Errorf("cleanup: failed to restore books_scope_idx: %v", cleanupErr)
		}
	})

	probeErr := CheckSchemaProbe(ctx, db)
	require.Error(t, probeErr, "probe should fail when books_scope_idx is missing")

	var sentinel *ErrSchemaSentinelMissing
	require.True(t, errors.As(probeErr, &sentinel), "error should be *ErrSchemaSentinelMissing, got %T: %v", probeErr, probeErr)
	assert.Equal(t, "index", sentinel.Kind)
	assert.Equal(t, "books_scope_idx", sentinel.Name)
}
