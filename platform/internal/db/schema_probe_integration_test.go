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

// TestCheckSchemaProbe_MissingColumn drops `revoked_at` from parent_links,
// registers a re-CREATE cleanup, then asserts that CheckSchemaProbe returns
// *ErrSchemaSentinelMissing with Kind=="column" and Name=="revoked_at".
//
// Isolation pattern: DDL committed (not rolled back) so the probe's separate
// connection pool can observe the change. t.Cleanup re-adds the column so
// subsequent test runs start from a clean state.
func TestCheckSchemaProbe_MissingColumn(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `ALTER TABLE parent_links DROP COLUMN revoked_at`)
	require.NoError(t, err, "precondition: drop revoked_at")

	// Register cleanup IMMEDIATELY after the destructive DDL succeeds so it
	// runs even if the assertion below panics or the test is skipped.
	t.Cleanup(func() {
		_, cleanupErr := db.ExecContext(ctx, `ALTER TABLE parent_links ADD COLUMN revoked_at timestamptz`)
		if cleanupErr != nil {
			t.Errorf("cleanup: failed to restore revoked_at: %v", cleanupErr)
		}
	})

	probeErr := CheckSchemaProbe(ctx, db)
	require.Error(t, probeErr, "probe should fail when revoked_at is missing")

	var sentinel *ErrSchemaSentinelMissing
	require.True(t, errors.As(probeErr, &sentinel), "error should be *ErrSchemaSentinelMissing, got %T: %v", probeErr, probeErr)
	assert.Equal(t, "column", sentinel.Kind)
	assert.Equal(t, "revoked_at", sentinel.Name)
}

// TestCheckSchemaProbe_MissingConstraint drops `parent_links_status_check`,
// registers a re-ADD cleanup, then asserts that CheckSchemaProbe returns
// *ErrSchemaSentinelMissing with Kind=="constraint" and
// Name=="parent_links_status_check".
func TestCheckSchemaProbe_MissingConstraint(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `ALTER TABLE parent_links DROP CONSTRAINT parent_links_status_check`)
	require.NoError(t, err, "precondition: drop parent_links_status_check")

	t.Cleanup(func() {
		_, cleanupErr := db.ExecContext(ctx,
			`ALTER TABLE parent_links ADD CONSTRAINT parent_links_status_check CHECK (status IN ('active', 'revoked'))`)
		if cleanupErr != nil {
			t.Errorf("cleanup: failed to restore parent_links_status_check: %v", cleanupErr)
		}
	})

	probeErr := CheckSchemaProbe(ctx, db)
	require.Error(t, probeErr, "probe should fail when parent_links_status_check is missing")

	var sentinel *ErrSchemaSentinelMissing
	require.True(t, errors.As(probeErr, &sentinel), "error should be *ErrSchemaSentinelMissing, got %T: %v", probeErr, probeErr)
	assert.Equal(t, "constraint", sentinel.Kind)
	assert.Equal(t, "parent_links_status_check", sentinel.Name)
}

// TestCheckSchemaProbe_MissingIndex drops `parent_links_active_uniq`,
// registers a re-CREATE cleanup, then asserts that CheckSchemaProbe returns
// *ErrSchemaSentinelMissing with Kind=="index" and
// Name=="parent_links_active_uniq".
func TestCheckSchemaProbe_MissingIndex(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `DROP INDEX parent_links_active_uniq`)
	require.NoError(t, err, "precondition: drop parent_links_active_uniq")

	t.Cleanup(func() {
		_, cleanupErr := db.ExecContext(ctx,
			`CREATE UNIQUE INDEX parent_links_active_uniq ON parent_links (parent_user_id, child_user_id) WHERE status = 'active'`)
		if cleanupErr != nil {
			t.Errorf("cleanup: failed to restore parent_links_active_uniq: %v", cleanupErr)
		}
	})

	probeErr := CheckSchemaProbe(ctx, db)
	require.Error(t, probeErr, "probe should fail when parent_links_active_uniq is missing")

	var sentinel *ErrSchemaSentinelMissing
	require.True(t, errors.As(probeErr, &sentinel), "error should be *ErrSchemaSentinelMissing, got %T: %v", probeErr, probeErr)
	assert.Equal(t, "index", sentinel.Kind)
	assert.Equal(t, "parent_links_active_uniq", sentinel.Name)
}
