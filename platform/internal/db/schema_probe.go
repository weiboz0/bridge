package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CheckSchemaProbe verifies that the schema's end-state matches the
// latest migration. Plan 068 phase 3 introduced a single-table
// `to_regclass()` check; Plan 076 added table sentinels and Plan 094
// made the contract multi-object (tables plus ordered enum labels).
// Used as a boot-time gate: any miss → refuse to start rather than
// serve requests against a stale schema.
//
// Returns:
//
//   - nil — schema is up-to-date.
//   - *ErrSchemaProbeMissing — the probe table is not present.
//   - *ErrSchemaSentinelMissing — a column/constraint/index from
//     ExpectedSchemaSentinels is absent on the probe table.
//   - *ErrSchemaEnumMismatch — an expected enum is absent or its labels differ.
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

	if err := checkTableExists(ctx, sqlDB, ExpectedSchemaProbe); err != nil {
		return err
	}
	if !hasTable(ExpectedSchemaSentinels, ExpectedSchemaProbe) {
		return fmt.Errorf("db.CheckSchemaProbe: ExpectedSchemaProbe %q is absent from ExpectedSchemaSentinels.Tables", ExpectedSchemaProbe)
	}
	for _, table := range ExpectedSchemaSentinels.Tables {
		if table.Table == "" {
			return errors.New("db.CheckSchemaProbe: empty table sentinel (build configuration error)")
		}
		if err := checkTableExists(ctx, sqlDB, table.Table); err != nil {
			return err
		}
	}

	// Step 2-5: multi-object sentinel walks.
	if err := checkColumns(ctx, sqlDB, ExpectedSchemaSentinels); err != nil {
		return err
	}
	if err := checkConstraints(ctx, sqlDB, ExpectedSchemaSentinels); err != nil {
		return err
	}
	if err := checkIndexes(ctx, sqlDB, ExpectedSchemaSentinels); err != nil {
		return err
	}
	if err := checkEnums(ctx, sqlDB, ExpectedSchemaSentinels); err != nil {
		return err
	}
	return nil
}

func hasTable(s SchemaSentinels, tableName string) bool {
	for _, table := range s.Tables {
		if table.Table == tableName {
			return true
		}
	}
	return false
}

func checkTableExists(ctx context.Context, sqlDB *sql.DB, tableName string) error {
	var result sql.NullString
	if err := sqlDB.QueryRowContext(ctx, `SELECT to_regclass($1)::text`, "public."+tableName).Scan(&result); err != nil {
		return fmt.Errorf("db.CheckSchemaProbe: table query failed: %w", err)
	}
	if !result.Valid {
		return &ErrSchemaProbeMissing{Table: tableName}
	}
	return nil
}

func checkColumns(ctx context.Context, sqlDB *sql.DB, s SchemaSentinels) error {
	for _, table := range s.Tables {
		for _, col := range table.Columns {
			var found sql.NullString
			err := sqlDB.QueryRowContext(ctx, `
			SELECT column_name
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = $1
			  AND column_name = $2
		`, table.Table, col).Scan(&found)
			if errors.Is(err, sql.ErrNoRows) {
				return &ErrSchemaSentinelMissing{Table: table.Table, Kind: "column", Name: col}
			}
			if err != nil {
				return fmt.Errorf("db.CheckSchemaProbe: column query failed for %q: %w", col, err)
			}
		}
	}
	return nil
}

func checkConstraints(ctx context.Context, sqlDB *sql.DB, s SchemaSentinels) error {
	for _, table := range s.Tables {
		for _, name := range table.Constraints {
			var found sql.NullString
			err := sqlDB.QueryRowContext(ctx, `
			SELECT c.conname
			FROM pg_constraint c
			JOIN pg_class t ON c.conrelid = t.oid
			JOIN pg_namespace n ON t.relnamespace = n.oid
			WHERE n.nspname = 'public'
			  AND t.relname = $1
			  AND c.conname = $2
		`, table.Table, name).Scan(&found)
			if errors.Is(err, sql.ErrNoRows) {
				return &ErrSchemaSentinelMissing{Table: table.Table, Kind: "constraint", Name: name}
			}
			if err != nil {
				return fmt.Errorf("db.CheckSchemaProbe: constraint query failed for %q: %w", name, err)
			}
		}
	}
	return nil
}

func checkIndexes(ctx context.Context, sqlDB *sql.DB, s SchemaSentinels) error {
	for _, table := range s.Tables {
		for _, name := range table.Indexes {
			var found sql.NullString
			err := sqlDB.QueryRowContext(ctx, `
			SELECT indexname
			FROM pg_indexes
			WHERE schemaname = 'public'
			  AND tablename = $1
			  AND indexname = $2
		`, table.Table, name).Scan(&found)
			if errors.Is(err, sql.ErrNoRows) {
				return &ErrSchemaSentinelMissing{Table: table.Table, Kind: "index", Name: name}
			}
			if err != nil {
				return fmt.Errorf("db.CheckSchemaProbe: index query failed for %q: %w", name, err)
			}
		}
	}
	return nil
}

func checkEnums(ctx context.Context, sqlDB *sql.DB, s SchemaSentinels) error {
	for _, enum := range s.Enums {
		rows, err := sqlDB.QueryContext(ctx, `
			SELECT e.enumlabel
			FROM pg_type t
			JOIN pg_namespace n ON n.oid = t.typnamespace
			JOIN pg_enum e ON e.enumtypid = t.oid
			WHERE n.nspname = 'public' AND t.typname = $1
			ORDER BY e.enumsortorder
		`, enum.Name)
		if err != nil {
			return fmt.Errorf("db.CheckSchemaProbe: enum query failed for %q: %w", enum.Name, err)
		}
		var actual []string
		for rows.Next() {
			var label string
			if err := rows.Scan(&label); err != nil {
				rows.Close()
				return fmt.Errorf("db.CheckSchemaProbe: enum scan failed for %q: %w", enum.Name, err)
			}
			actual = append(actual, label)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("db.CheckSchemaProbe: enum rows failed for %q: %w", enum.Name, err)
		}
		rows.Close()
		if !sameStrings(enum.Labels, actual) {
			return &ErrSchemaEnumMismatch{Enum: enum.Name, Expected: enum.Labels, Actual: actual}
		}
	}
	return nil
}

func sameStrings(expected, actual []string) bool {
	if len(expected) != len(actual) {
		return false
	}
	for i := range expected {
		if expected[i] != actual[i] {
			return false
		}
	}
	return true
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
		"schema probe missing: table %q does not exist. The latest schema end state is incomplete. Do not blindly re-run the whole migration; inspect the latest drizzle/*.sql and apply the exact missing DDL through the approved database-change workflow, then restart the server.",
		e.Table,
	)
}

// ErrSchemaSentinelMissing indicates a column / constraint / index
// from ExpectedSchemaSentinels is absent. Re-running a whole migration can
// fail after a partial apply (for example, CREATE TYPE is not idempotent), so
// the operator must apply the exact missing DDL from the migration source.
type ErrSchemaSentinelMissing struct {
	Table string
	Kind  string // "column" | "constraint" | "index"
	Name  string
}

// ErrSchemaEnumMismatch indicates a missing enum or an enum whose labels do
// not exactly match the migration declaration in PostgreSQL sort order.
type ErrSchemaEnumMismatch struct {
	Enum     string
	Expected []string
	Actual   []string
}

func (e *ErrSchemaEnumMismatch) Error() string {
	return fmt.Sprintf(
		"schema enum mismatch: enum %q has labels %v; expected %v in this order. Apply the missing enum DDL from the latest drizzle/*.sql manually, then restart.",
		e.Enum, e.Actual, e.Expected,
	)
}

func (e *ErrSchemaSentinelMissing) Error() string {
	switch e.Kind {
	case "column":
		return fmt.Sprintf(
			"schema sentinel missing: column %q on table %q is absent. Do not blindly re-run the whole migration; apply `ALTER TABLE %s ADD COLUMN %s ...` using the exact definition from the latest drizzle/*.sql, then restart.",
			e.Name, e.Table, e.Table, e.Name,
		)
	case "constraint":
		return fmt.Sprintf(
			"schema sentinel missing: constraint %q on table %q is absent. Do not blindly re-run the whole migration; apply `ALTER TABLE %s ADD CONSTRAINT %s ...` from the latest drizzle/*.sql, then restart.",
			e.Name, e.Table, e.Table, e.Name,
		)
	case "index":
		return fmt.Sprintf(
			"schema sentinel missing: index %q on table %q is absent. Do not blindly re-run the whole migration; apply `CREATE INDEX %s ON %s ...` (or `CREATE UNIQUE INDEX ... WHERE ...` for partial-uniques) from the latest drizzle/*.sql, then restart.",
			e.Name, e.Table, e.Name, e.Table,
		)
	default:
		return fmt.Sprintf(
			"schema sentinel missing: %s %q on table %q is absent. Apply the corresponding DDL from the latest drizzle/*.sql, then restart.",
			e.Kind, e.Name, e.Table,
		)
	}
}
