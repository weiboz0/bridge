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
// Plan 094 extends the probe to multi-object migrations. The latest migration
// creates session_canvases, alters sessions, and creates canvas_visibility;
// all three outputs are part of one atomic end-state contract.
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

// ExpectedSchemaProbe is the primary public table created by the latest
// schema-affecting migration. Boot-time check verifies the table
// exists; mismatch → refuse to start.
const ExpectedSchemaProbe = "session_canvases"

// SchemaTableSentinels enumerates the objects that must exist on one table.
// The current migration has no named constraints, but Constraints remains a
// first-class field so future migrations keep the generic probe coverage.
type SchemaTableSentinels struct {
	Table             string
	Columns           []string
	ColumnDefinitions []SchemaColumnSentinel
	Constraints       []string
	Indexes           []string
}

// SchemaColumnSentinel pins physical properties that are security-relevant
// for nullable lifecycle state, beyond mere column existence.
type SchemaColumnSentinel struct {
	Name     string
	DataType string
	Nullable bool
}

// SchemaEnumSentinel requires a PostgreSQL enum's labels in their declared
// order. Ordering is part of the canvas visibility contract.
type SchemaEnumSentinel struct {
	Name   string
	Labels []string
}

// SchemaSentinels is the complete latest-migration end-state contract. A
// migration can create one primary table while also altering other tables or
// defining enum types, so it intentionally models multiple objects.
type SchemaSentinels struct {
	Tables []SchemaTableSentinels
	Enums  []SchemaEnumSentinel
}

// ExpectedSchemaSentinels is the sentinel set for the latest
// schema-affecting migration (`drizzle/0028_session_canvases.sql`).
//
// Bump rule: every PR that adds or modifies a schema-affecting
// migration MUST update this struct. The CI parity test verifies
// bidirectional parity with the migration source.
var ExpectedSchemaSentinels = SchemaSentinels{
	Tables: []SchemaTableSentinels{
		{
			Table: "session_canvases",
			Columns: []string{
				"id", "session_id", "owner_id", "title", "visibility", "yjs_state", "created_at", "updated_at",
			},
			Indexes: []string{
				"session_canvases_session_idx", "session_canvases_session_owner_idx",
			},
		},
		{
			Table:   "sessions",
			Columns: []string{"canvas_floor", "canvas_freeze_token", "canvas_freeze_until", "whiteboard_server_archive_complete"},
			ColumnDefinitions: []SchemaColumnSentinel{
				{Name: "canvas_freeze_token", DataType: "uuid", Nullable: true},
				{Name: "canvas_freeze_until", DataType: "timestamp with time zone", Nullable: true},
				{Name: "whiteboard_server_archive_complete", DataType: "boolean", Nullable: true},
			},
			Constraints: []string{"sessions_canvas_freeze_lease_pair"},
		},
	},
	Enums: []SchemaEnumSentinel{
		{Name: "canvas_visibility", Labels: []string{"private", "host", "participants", "session"}},
	},
}
