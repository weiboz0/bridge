package db

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckSchemaProbe_HappyPath_AllSentinels verifies that a fully-migrated
// bridge_test database passes all session_canvases, sessions, and enum walks.
func TestCheckSchemaProbe_HappyPath_AllSentinels(t *testing.T) {
	db := integrationDB(t)
	err := CheckSchemaProbe(context.Background(), db)
	require.NoError(t, err, "fully-migrated bridge_test DB should pass all sentinels")
}

// TestCheckSchemaProbe_MissingColumn drops `yjs_state` from session_canvases,
// registers a re-CREATE cleanup, then asserts that CheckSchemaProbe returns
// *ErrSchemaSentinelMissing with Kind=="column" and Name=="yjs_state".
//
// Isolation pattern: DDL committed (not rolled back) so the probe's separate
// connection pool can observe the change. t.Cleanup re-adds the column so
// subsequent test runs start from a clean state.
func TestCheckSchemaProbe_MissingColumn(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `ALTER TABLE session_canvases DROP COLUMN yjs_state`)
	require.NoError(t, err, "precondition: drop yjs_state")

	// Register cleanup IMMEDIATELY after the destructive DDL succeeds so it
	// runs even if the assertion below panics or the test is skipped.
	t.Cleanup(func() {
		_, cleanupErr := db.ExecContext(ctx, `ALTER TABLE session_canvases ADD COLUMN IF NOT EXISTS yjs_state text`)
		if cleanupErr != nil {
			t.Errorf("cleanup: failed to restore description: %v", cleanupErr)
		}
	})

	probeErr := CheckSchemaProbe(ctx, db)
	require.Error(t, probeErr, "probe should fail when yjs_state is missing")

	var sentinel *ErrSchemaSentinelMissing
	require.True(t, errors.As(probeErr, &sentinel), "error should be *ErrSchemaSentinelMissing, got %T: %v", probeErr, probeErr)
	assert.Equal(t, "column", sentinel.Kind)
	assert.Equal(t, "yjs_state", sentinel.Name)
}

// TestCheckConstraints_InjectsNamedConstraint keeps the generic named
// constraint walk covered even though the current migration has no named
// constraints. The fixture is isolated to bridge_test and cleaned up.
func TestCheckConstraints_InjectsNamedConstraint(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	const fixtureTable = "schema_probe_constraint_fixture"
	const fixtureConstraint = "schema_probe_constraint_fixture_positive"

	_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS schema_probe_constraint_fixture`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := db.ExecContext(ctx, `DROP TABLE IF EXISTS schema_probe_constraint_fixture`)
		if cleanupErr != nil {
			t.Errorf("cleanup: failed to drop synthetic constraint fixture: %v", cleanupErr)
		}
	})
	_, err = db.ExecContext(ctx, `
		CREATE TABLE schema_probe_constraint_fixture (
			value integer,
			CONSTRAINT schema_probe_constraint_fixture_positive CHECK (value > 0)
		)`)
	require.NoError(t, err)

	require.NoError(t, checkConstraints(ctx, db, SchemaSentinels{
		Tables: []SchemaTableSentinels{{
			Table:       fixtureTable,
			Constraints: []string{fixtureConstraint},
		}},
	}))
}

// TestCheckSchemaProbe_MissingIndex drops `session_canvases_session_idx`,
// registers a re-CREATE cleanup, then asserts that CheckSchemaProbe returns
// *ErrSchemaSentinelMissing with Kind=="index" and
// Name=="session_canvases_session_idx".
func TestCheckSchemaProbe_MissingIndex(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `DROP INDEX session_canvases_session_idx`)
	require.NoError(t, err, "precondition: drop session_canvases_session_idx")

	t.Cleanup(func() {
		_, cleanupErr := db.ExecContext(ctx,
			`CREATE INDEX IF NOT EXISTS session_canvases_session_idx ON session_canvases (session_id)`)
		if cleanupErr != nil {
			t.Errorf("cleanup: failed to restore session_canvases_session_idx: %v", cleanupErr)
		}
	})

	probeErr := CheckSchemaProbe(ctx, db)
	require.Error(t, probeErr, "probe should fail when session_canvases_session_idx is missing")

	var sentinel *ErrSchemaSentinelMissing
	require.True(t, errors.As(probeErr, &sentinel), "error should be *ErrSchemaSentinelMissing, got %T: %v", probeErr, probeErr)
	assert.Equal(t, "index", sentinel.Kind)
	assert.Equal(t, "session_canvases_session_idx", sentinel.Name)
}

func TestCheckEnums_ExactOrderReturnsTypedDiagnostic(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	const fixtureEnum = "schema_probe_enum_fixture"

	_, err := db.ExecContext(ctx, `DROP TYPE IF EXISTS schema_probe_enum_fixture`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := db.ExecContext(ctx, `DROP TYPE IF EXISTS schema_probe_enum_fixture`)
		if cleanupErr != nil {
			t.Errorf("cleanup: failed to drop synthetic enum fixture: %v", cleanupErr)
		}
	})
	_, err = db.ExecContext(ctx, `CREATE TYPE schema_probe_enum_fixture AS ENUM ('first', 'second')`)
	require.NoError(t, err)

	probeErr := checkEnums(ctx, db, SchemaSentinels{Enums: []SchemaEnumSentinel{{
		Name: fixtureEnum, Labels: []string{"second", "first"},
	}}})
	require.Error(t, probeErr)
	var mismatch *ErrSchemaEnumMismatch
	require.True(t, errors.As(probeErr, &mismatch), "expected typed enum mismatch, got %T: %v", probeErr, probeErr)
	assert.Equal(t, fixtureEnum, mismatch.Enum)
	assert.Equal(t, []string{"second", "first"}, mismatch.Expected)
	assert.Equal(t, []string{"first", "second"}, mismatch.Actual)
}
