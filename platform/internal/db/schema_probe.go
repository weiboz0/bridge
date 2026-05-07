package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CheckSchemaProbe verifies that the schema's end-state matches the
// latest migration. Plan 068 phase 3 introduced a single-table
// `to_regclass()` check; Plan 076 extends it with column / constraint
// / index sentinels. Used as a boot-time gate: any miss → refuse to
// start rather than serve requests against a stale schema.
//
// Returns:
//
//   - nil — schema is up-to-date.
//   - *ErrSchemaProbeMissing — the probe table is not present.
//   - *ErrSchemaSentinelMissing — a column/constraint/index from
//     ExpectedSchemaSentinels is absent on the probe table.
//   - other errors — connection / DB-level failures (caller should
//     treat as fatal regardless of probe state).
//
// The query uses `to_regclass()` rather than `information_schema` to
// stay quoting-safe against the schema name and to return NULL
// (rather than 0 rows) for a missing table. Sentinel queries use
// `information_schema.columns`, `pg_constraint` joined to `pg_class`,
// and `pg_indexes` — all filtered by the table name to avoid matching
// same-named objects on other tables.
func CheckSchemaProbe(ctx context.Context, sqlDB *sql.DB) error {
	if sqlDB == nil {
		return errors.New("db.CheckSchemaProbe: nil DB handle")
	}
	if ExpectedSchemaProbe == "" {
		return errors.New("db.CheckSchemaProbe: ExpectedSchemaProbe is empty (build configuration error)")
	}

	// Step 1: table existence (Plan 068 phase 3 — kept for
	// backward-compatible error type and the quoting-safe
	// to_regclass() pattern).
	qualified := "public." + ExpectedSchemaProbe
	var result sql.NullString
	if err := sqlDB.QueryRowContext(ctx, `SELECT to_regclass($1)::text`, qualified).Scan(&result); err != nil {
		return fmt.Errorf("db.CheckSchemaProbe: query failed: %w", err)
	}
	if !result.Valid {
		return &ErrSchemaProbeMissing{Table: ExpectedSchemaProbe}
	}

	// Step 2-4: sentinel walks. Skip if the sentinel struct's Table
	// field is empty (defensive — the parity test asserts it
	// matches ExpectedSchemaProbe).
	if ExpectedSchemaSentinels.Table == "" {
		return nil
	}
	if err := checkColumns(ctx, sqlDB, ExpectedSchemaSentinels); err != nil {
		return err
	}
	if err := checkConstraints(ctx, sqlDB, ExpectedSchemaSentinels); err != nil {
		return err
	}
	if err := checkIndexes(ctx, sqlDB, ExpectedSchemaSentinels); err != nil {
		return err
	}
	return nil
}

func checkColumns(ctx context.Context, sqlDB *sql.DB, s SchemaSentinels) error {
	for _, col := range s.Columns {
		var found sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_name
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = $1
			  AND column_name = $2
		`, s.Table, col).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return &ErrSchemaSentinelMissing{Table: s.Table, Kind: "column", Name: col}
		}
		if err != nil {
			return fmt.Errorf("db.CheckSchemaProbe: column query failed for %q: %w", col, err)
		}
	}
	return nil
}

func checkConstraints(ctx context.Context, sqlDB *sql.DB, s SchemaSentinels) error {
	for _, name := range s.Constraints {
		var found sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT c.conname
			FROM pg_constraint c
			JOIN pg_class t ON c.conrelid = t.oid
			JOIN pg_namespace n ON t.relnamespace = n.oid
			WHERE n.nspname = 'public'
			  AND t.relname = $1
			  AND c.conname = $2
		`, s.Table, name).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return &ErrSchemaSentinelMissing{Table: s.Table, Kind: "constraint", Name: name}
		}
		if err != nil {
			return fmt.Errorf("db.CheckSchemaProbe: constraint query failed for %q: %w", name, err)
		}
	}
	return nil
}

func checkIndexes(ctx context.Context, sqlDB *sql.DB, s SchemaSentinels) error {
	for _, name := range s.Indexes {
		var found sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT indexname
			FROM pg_indexes
			WHERE schemaname = 'public'
			  AND tablename = $1
			  AND indexname = $2
		`, s.Table, name).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return &ErrSchemaSentinelMissing{Table: s.Table, Kind: "index", Name: name}
		}
		if err != nil {
			return fmt.Errorf("db.CheckSchemaProbe: index query failed for %q: %w", name, err)
		}
	}
	return nil
}

// ErrSchemaProbeMissing indicates the expected schema-probe table is
// not present. The Error() text is intended for operator log lines —
// it names the missing table and points at the manual-apply workflow
// so the next action is unambiguous.
type ErrSchemaProbeMissing struct {
	Table string
}

func (e *ErrSchemaProbeMissing) Error() string {
	return fmt.Sprintf(
		"schema probe missing: table %q does not exist. The latest migration has not been applied. Run `psql $DATABASE_URL -f drizzle/<latest>.sql` (Bridge applies 0003+ via psql -f per TODO.md:10) and restart the server.",
		e.Table,
	)
}

// ErrSchemaSentinelMissing indicates a column / constraint / index
// from ExpectedSchemaSentinels is absent on the probe table. Re-running
// the migration file is a NO-OP because `CREATE TABLE IF NOT EXISTS`
// short-circuits when the table already exists — operator must apply
// the specific ALTER / CREATE INDEX statement manually. The Error()
// text directs them to the migration source for the exact DDL.
type ErrSchemaSentinelMissing struct {
	Table string
	Kind  string // "column" | "constraint" | "index"
	Name  string
}

func (e *ErrSchemaSentinelMissing) Error() string {
	switch e.Kind {
	case "column":
		return fmt.Sprintf(
			"schema sentinel missing: column %q on table %q is absent. Re-running the migration file is a no-op (CREATE TABLE IF NOT EXISTS). Apply `ALTER TABLE %s ADD COLUMN %s ...` manually using the column definition from the latest drizzle/*.sql, then restart.",
			e.Name, e.Table, e.Table, e.Name,
		)
	case "constraint":
		return fmt.Sprintf(
			"schema sentinel missing: constraint %q on table %q is absent. Re-running the migration file is a no-op. Apply `ALTER TABLE %s ADD CONSTRAINT %s ...` from the latest drizzle/*.sql, then restart.",
			e.Name, e.Table, e.Table, e.Name,
		)
	case "index":
		return fmt.Sprintf(
			"schema sentinel missing: index %q on table %q is absent. Re-running the migration file is a no-op. Apply `CREATE INDEX %s ON %s ...` (or `CREATE UNIQUE INDEX ... WHERE ...` for partial-uniques) from the latest drizzle/*.sql, then restart.",
			e.Name, e.Table, e.Name, e.Table,
		)
	default:
		return fmt.Sprintf(
			"schema sentinel missing: %s %q on table %q is absent. Apply the corresponding DDL from the latest drizzle/*.sql, then restart.",
			e.Kind, e.Name, e.Table,
		)
	}
}
