package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CheckSchemaProbe verifies that the table named `ExpectedSchemaProbe`
// exists in the `public` schema. Used as a boot-time gate: if the
// expected end-state table is missing, the server refuses to start
// rather than serving requests against a stale schema.
//
// Returns:
//
//   - nil — schema is up-to-date.
//   - ErrSchemaProbeMissing — the probe table is not present. The
//     accompanying error message identifies the missing table and
//     points at the manual-apply workflow.
//   - other errors — connection / DB-level failures (caller should
//     treat as fatal regardless of probe state).
//
// The query uses `to_regclass()` rather than `information_schema` to
// stay quoting-safe against the schema name and to return NULL
// (rather than 0 rows) for a missing table.
func CheckSchemaProbe(ctx context.Context, sqlDB *sql.DB) error {
	if sqlDB == nil {
		return errors.New("db.CheckSchemaProbe: nil DB handle")
	}
	if ExpectedSchemaProbe == "" {
		return errors.New("db.CheckSchemaProbe: ExpectedSchemaProbe is empty (build configuration error)")
	}
	qualified := "public." + ExpectedSchemaProbe
	var result sql.NullString
	err := sqlDB.QueryRowContext(ctx, `SELECT to_regclass($1)::text`, qualified).Scan(&result)
	if err != nil {
		return fmt.Errorf("db.CheckSchemaProbe: query failed: %w", err)
	}
	if !result.Valid {
		return &ErrSchemaProbeMissing{Table: ExpectedSchemaProbe}
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
