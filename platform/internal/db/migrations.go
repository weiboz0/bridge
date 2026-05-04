package db

// Plan 068 phase 3 — startup migration health check.
//
// Bridge applies migrations 0003+ via `psql -f` (TODO.md:10), so
// counting rows in `drizzle.__drizzle_migrations` is unreliable —
// hand-applied migrations leave NO tracking row. Instead, the boot
// check verifies that the latest schema-affecting migration's target
// table EXISTS via `to_regclass()`. This is a "schema-probe":
// it asks "is the END STATE of the schema present?" rather than
// "did Drizzle's migrator log every step?"
//
// Limitation: a single-table presence check can't distinguish a
// fully-migrated DB from a stale DB where this one table happens to
// have been hotfixed in isolation. That's an acceptable risk —
// the realistic failure mode is "operator forgot to apply migrations"
// which would leave the LATEST table missing, not "operator
// surgically applied just the latest migration in isolation". If a
// stronger check is needed later, switch to a multi-sentinel probe
// (one column from each milestone migration).
//
// Bump procedure: when adding a new migration that creates a table
// (most migrations do), update ExpectedSchemaProbe to the new table
// name in the SAME PR. The CI parity test
// (`schema_probe_parity_test.go`) catches PRs that bump the constant
// without an actual CREATE TABLE in drizzle/*.sql, OR add a CREATE
// TABLE without bumping the constant.
//
// For migrations that don't create a table (e.g., dropping a column),
// leave the constant at the previous CREATE-TABLE-bearing migration's
// target. The probe still validates that the prior schema state is
// present; the column-drop is a non-schema-shape change that doesn't
// need its own probe.

// ExpectedSchemaProbe is the public table name created by the latest
// schema-affecting migration. Boot-time check verifies the table
// exists; mismatch → refuse to start.
const ExpectedSchemaProbe = "parent_links"
