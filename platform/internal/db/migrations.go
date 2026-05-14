package db

// Plan 068 phase 3 / Plan 076 — startup migration health check.
//
// Bridge applies migrations 0003+ via `psql -f` (TODO.md:10), so
// counting rows in `drizzle.__drizzle_migrations` is unreliable —
// hand-applied migrations leave NO tracking row. Instead, the boot
// check verifies that the latest schema-affecting migration's outputs
// EXIST in the live database. This is a "schema-probe": it asks
// "is the END STATE of the schema present?" rather than "did Drizzle's
// migrator log every step?"
//
// Plan 076 hardening: the probe checks more than table presence. It
// verifies that the latest migration's COLUMNS, named CONSTRAINTS, and
// INDEXES are all present, catching partial-migration cases where the
// table was created but `ALTER TABLE ADD CONSTRAINT` or `CREATE INDEX`
// failed. Failure mode is fail-fast at boot.
//
// Scope: latest-migration-only. Earlier migrations' constraints and
// indexes are NOT probed. The realistic failure mode is "operator
// forgot to apply the LATEST migration", which leaves the latest
// table OR its constraints/indexes missing — both are caught.
// Hand-applying just the latest migration's CREATE TABLE without the
// inline constraints would also be caught. Surgically applying old
// migrations in isolation is theoretical and out of scope.
//
// Foreign-key constraint NAMES are NOT in the sentinel list: PG
// auto-names them from column names, so a column rename in a future
// migration would falsely trip the probe. Inline `REFERENCES` is also
// syntactically bound to `CREATE TABLE` — a partial failure where the
// table exists but FKs don't is impossible (Postgres rejects the
// whole CREATE TABLE if any inline constraint is malformed).
//
// FK actions (`ON DELETE CASCADE` etc.), CHECK expression text, and
// index DDL are NOT verified — that would require parsing
// `pg_constraint.consrc` / `pg_indexes.indexdef` and string-comparing
// against the migration source, which is fragile. Name presence catches
// the realistic failure mode (whole `CREATE` statement missing).
//
// Enum-value checks are deferred. Earlier migrations declare enums
// (auth_provider, editor_mode, grade_level, user_role in 0000;
// signup_intent in 0021; schedule_status in 0023) but the latest
// migration `parent_links` uses varchar+CHECK instead. When a future
// migration adds a new enum or alters an existing one's values,
// extend SchemaSentinels with an Enums field.
//
// Bump procedure: when adding a new schema-affecting migration that
// creates a table (most migrations do), update ExpectedSchemaProbe AND
// ExpectedSchemaSentinels in the SAME PR. The CI parity test
// (`schema_probe_parity_test.go`) catches PRs that bump one but not
// the other, by parsing the latest drizzle/*.sql file's CREATE TABLE
// / CONSTRAINT / CREATE INDEX declarations and asserting BIDIRECTIONAL
// parity with the sentinel struct (forward catches omissions; reverse
// catches typos and stale ghosts).
//
// For migrations that DROP a constraint or index without creating a
// new table, ExpectedSchemaProbe doesn't change; the maintainer must
// MANUALLY remove the dropped name from ExpectedSchemaSentinels.
// Code reviewers enforce this at PR-time.
//
// For migrations that don't create a table at all (e.g., dropping a
// column), leave both constants at the previous CREATE-TABLE-bearing
// migration's targets. The probe still validates that the prior
// schema state is present.

// ExpectedSchemaProbe is the public table name created by the latest
// schema-affecting migration. Boot-time check verifies the table
// exists; mismatch → refuse to start.
const ExpectedSchemaProbe = "books"

// SchemaSentinels enumerates the columns, named constraints, and
// indexes that must be present on the ExpectedSchemaProbe table for
// the schema to be considered fully migrated. Plan 076.
type SchemaSentinels struct {
	// Table is the table the sentinels apply to. Always equal to
	// ExpectedSchemaProbe; the field is here so the boot probe can
	// take a single SchemaSentinels value rather than threading two
	// constants together.
	Table string
	// Columns are the column names that must exist on Table. Order
	// is irrelevant; all entries are checked.
	Columns []string
	// Constraints are the named CHECK / UNIQUE / PRIMARY KEY
	// constraints (matched by `pg_constraint.conname`, scoped to
	// Table via the join). FK constraints are intentionally absent
	// (see file-level comment).
	Constraints []string
	// Indexes are the named btree / partial / unique indexes
	// (matched by `pg_indexes.indexname`, scoped to Table via
	// `pg_indexes.tablename`). Includes partial-uniques even though
	// they enforce constraint-level invariants — they live in
	// `pg_indexes`, not `pg_constraint`.
	Indexes []string
}

// ExpectedSchemaSentinels is the sentinel set for the latest
// schema-affecting migration (`drizzle/0026_books_and_chapters.sql`).
//
// Bump rule: every PR that adds or modifies a schema-affecting
// migration MUST update this struct. The CI parity test verifies
// bidirectional parity with the migration source.
var ExpectedSchemaSentinels = SchemaSentinels{
	Table: "books",
	Columns: []string{
		"id",
		"title",
		"description",
		"scope",
		"scope_id",
		"created_by",
		"created_at",
		"updated_at",
	},
	Constraints: []string{
		"books_scope_id_required",
	},
	Indexes: []string{
		"books_scope_idx",
		"books_created_by_idx",
		"chapters_book_idx",
	},
}
