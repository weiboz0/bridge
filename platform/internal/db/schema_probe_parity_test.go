package db

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Plan 068 phase 3 — CI parity test for ExpectedSchemaProbe.
//
// Walks `drizzle/*.sql` in sort-order, finds the latest file that
// contains a CREATE TABLE statement, extracts the table name, and
// asserts it matches the ExpectedSchemaProbe constant. Two failure
// modes this catches:
//
//  1. PR adds a new migration with `CREATE TABLE foo` but doesn't
//     bump ExpectedSchemaProbe — the test sees the latest table in
//     drizzle/ doesn't match the constant and fails.
//
//  2. PR bumps ExpectedSchemaProbe to a name that doesn't exist in
//     any drizzle/*.sql CREATE TABLE — the test fails because the
//     constant can't be matched.
//
// What the test deliberately DOESN'T catch:
//
//   - Migrations that don't create a table (e.g., DROP COLUMN-only).
//     The probe is for end-state schema verification, and a
//     non-CREATE migration doesn't add a new table to probe. The
//     constant stays at the previous CREATE-TABLE-bearing migration's
//     target; that's intentional.
//
// The migrations directory is at `../../drizzle/` relative to this
// test file (platform/internal/db). If the layout changes, the test
// path needs to adjust.

var createTableRE = regexp.MustCompile(`(?im)^\s*CREATE TABLE(?:\s+IF NOT EXISTS)?\s+"?(\w+)"?`)

// migrationFilenameRE matches the standard `<NNNN>_<name>.sql` shape so
// stray non-migration .sql files (if any) get skipped. Drizzle's
// default convention is 4-digit zero-padded sequence prefix.
var migrationFilenameRE = regexp.MustCompile(`^\d{4}_.+\.sql$`)

// Plan 076 — sentinel parity. The line-based regexes match the SQL
// style used in drizzle/0024_parent_links.sql; future migrations using
// unusual styles (multi-line CONSTRAINT names, ALTER TABLE ADD CONSTRAINT)
// may need parser updates. Comments are stripped before matching to
// avoid commented-out DDL like `-- CONSTRAINT old_name` registering as
// a real declaration (DeepSeek round-1 NIT).
var (
	// CONSTRAINT <name> on its own line or after whitespace inside CREATE TABLE.
	constraintNameRE = regexp.MustCompile(`(?m)^\s*CONSTRAINT\s+(\w+)\b`)
	// CREATE [UNIQUE] INDEX [IF NOT EXISTS] <name> ON ...
	indexNameRE = regexp.MustCompile(`(?im)^\s*CREATE\s+(?:UNIQUE\s+)?INDEX(?:\s+IF\s+NOT\s+EXISTS)?\s+(\w+)\b`)
	// Column lines inside CREATE TABLE: leading-whitespace + identifier + type.
	// We only care about the first identifier per line. PRIMARY KEY / NOT NULL
	// suffix detection happens on the same line.
	columnLineRE = regexp.MustCompile(`(?m)^\s+"?(\w+)"?\s+\w+`)
	// Comment-stripper: remove everything after `--` to end-of-line.
	lineCommentRE = regexp.MustCompile(`--[^\n]*`)
)

func TestExpectedSchemaProbe_MatchesLatestCreateTableInDrizzle(t *testing.T) {
	// Resolve the drizzle dir relative to this test's compile location.
	// tests in `platform/internal/db/` need to walk up to repo root
	// then down to `drizzle/`.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	drizzleDir := filepath.Join(cwd, "..", "..", "..", "drizzle")
	entries, err := os.ReadDir(drizzleDir)
	require.NoError(t, err, "expected drizzle dir at %s", drizzleDir)

	var sqlFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !migrationFilenameRE.MatchString(entry.Name()) {
			continue
		}
		sqlFiles = append(sqlFiles, entry.Name())
	}
	require.NotEmpty(t, sqlFiles, "no migration .sql files found in %s", drizzleDir)
	sort.Strings(sqlFiles) // 4-digit prefix → lexicographic order matches numeric order

	// Walk back-to-front looking for the latest file with a
	// CREATE TABLE. Migrations that only DROP / ALTER skip past.
	var latestTable string
	var latestFile string
	for i := len(sqlFiles) - 1; i >= 0; i-- {
		path := filepath.Join(drizzleDir, sqlFiles[i])
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		matches := createTableRE.FindAllStringSubmatch(string(content), -1)
		if len(matches) == 0 {
			continue
		}
		// If a single migration creates multiple tables, take the LAST
		// one in file order — it's typically the dependent table.
		// (Currently no Bridge migration creates multiple tables so this
		// detail is just-in-case.)
		latestTable = matches[len(matches)-1][1]
		latestFile = sqlFiles[i]
		break
	}

	require.NotEmpty(t, latestTable,
		"no CREATE TABLE statement found in any drizzle/*.sql file — schema-probe parity test cannot determine the expected table",
	)

	require.Equal(t, ExpectedSchemaProbe, latestTable,
		"ExpectedSchemaProbe (%q) does not match the latest CREATE TABLE in drizzle/ (%q in %s).\n"+
			"Either bump ExpectedSchemaProbe in platform/internal/db/migrations.go to %q, "+
			"or revert the migration that introduced the new table.",
		ExpectedSchemaProbe, latestTable, latestFile, latestTable,
	)

	// Sanity: the latest file's basename should sort >= any other in
	// the list. Catches a layout glitch where the migration sequence
	// went non-monotonic.
	expectedLatest := sqlFiles[len(sqlFiles)-1]
	if strings.HasSuffix(expectedLatest, ".sql") && latestFile != expectedLatest {
		// Latest CREATE-TABLE file isn't the absolute latest file —
		// that's fine (an ALTER/DROP migration after the CREATE), but
		// the test logs both for visibility.
		t.Logf(
			"info: latest CREATE TABLE migration (%s) is not the highest-numbered migration (%s); "+
				"this is expected when the latest migration only drops/alters columns.",
			latestFile, expectedLatest,
		)
	}
}

// findLatestCreateTableMigration walks `drizzle/*.sql` in sort-order and
// returns (filename, content) of the latest migration that contains a
// CREATE TABLE statement. Shared by the table-name parity test above
// and the sentinel parity test below. Fails the test if no such file
// is found.
func findLatestCreateTableMigration(t *testing.T) (filename string, content string) {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	drizzleDir := filepath.Join(cwd, "..", "..", "..", "drizzle")
	entries, err := os.ReadDir(drizzleDir)
	require.NoError(t, err)

	var sqlFiles []string
	for _, entry := range entries {
		if entry.IsDir() || !migrationFilenameRE.MatchString(entry.Name()) {
			continue
		}
		sqlFiles = append(sqlFiles, entry.Name())
	}
	require.NotEmpty(t, sqlFiles)
	sort.Strings(sqlFiles)

	for i := len(sqlFiles) - 1; i >= 0; i-- {
		path := filepath.Join(drizzleDir, sqlFiles[i])
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		if createTableRE.FindString(string(raw)) == "" {
			continue
		}
		return sqlFiles[i], string(raw)
	}
	t.Fatal("no CREATE TABLE found in any drizzle/*.sql")
	return "", ""
}

// extractDeclaredNames parses constraint names, index names, and
// column names from a migration file's content. Comments are stripped
// first to avoid commented-out DDL registering as a real declaration
// (DeepSeek round-1 NIT). The regexes target the SQL style used in
// current Bridge migrations; future migrations using ALTER TABLE ADD
// CONSTRAINT or multi-line CONSTRAINT definitions may need parser
// updates.
//
// Columns are extracted ONLY from inside CREATE TABLE (...) blocks to
// avoid false positives from CREATE INDEX ... ON ... WHERE ... clauses
// (which contain identifier-shape tokens like `ON`, `WHERE`).
func extractDeclaredNames(content string) (constraints, indexes, columns []string) {
	stripped := lineCommentRE.ReplaceAllString(content, "")

	for _, m := range constraintNameRE.FindAllStringSubmatch(stripped, -1) {
		constraints = append(constraints, m[1])
	}
	for _, m := range indexNameRE.FindAllStringSubmatch(stripped, -1) {
		indexes = append(indexes, m[1])
	}

	// Columns: scope the column-line regex to CREATE TABLE bodies
	// only. Locate each `CREATE TABLE ... (` opening, then scan up to
	// the matching `);` closing.
	for _, body := range extractCreateTableBodies(stripped) {
		for _, m := range columnLineRE.FindAllStringSubmatch(body, -1) {
			ident := m[1]
			switch strings.ToUpper(ident) {
			case "CONSTRAINT", "PRIMARY", "UNIQUE", "CREATE", "FOREIGN", "CHECK":
				continue
			}
			columns = append(columns, ident)
		}
	}
	return
}

// extractCreateTableBodies returns the body text (between `(` and the
// matching `);`) of each CREATE TABLE statement in the input. Uses a
// simple paren-depth counter — fine for current Bridge migrations,
// which don't use parenthesized DEFAULT expressions. If a future
// migration adds something like `DEFAULT (now() AT TIME ZONE 'utc')`,
// this needs more care.
func extractCreateTableBodies(content string) []string {
	var bodies []string
	idxs := createTableRE.FindAllStringIndex(content, -1)
	for _, m := range idxs {
		// Find the first `(` after the CREATE TABLE match.
		start := strings.IndexByte(content[m[1]:], '(')
		if start < 0 {
			continue
		}
		start += m[1] + 1 // position just after '('
		depth := 1
		end := start
		for end < len(content) && depth > 0 {
			switch content[end] {
			case '(':
				depth++
			case ')':
				depth--
			}
			end++
		}
		if depth != 0 {
			continue // malformed; skip
		}
		bodies = append(bodies, content[start:end-1])
	}
	return bodies
}

func TestExpectedSchemaSentinels_BidirectionalParity(t *testing.T) {
	// Plan 076 — bidirectional parity. Forward direction catches a PR
	// that adds a CONSTRAINT or CREATE INDEX without updating
	// ExpectedSchemaSentinels. Reverse catches typos in the sentinel
	// struct + stale entries left after a future migration drops a
	// constraint or index. Mirrors plan 074's shadow-routes.test.ts
	// pattern (Kimi K2.6 round-1 NIT).
	require.Equal(t, ExpectedSchemaProbe, ExpectedSchemaSentinels.Table,
		"ExpectedSchemaSentinels.Table (%q) does not match ExpectedSchemaProbe (%q). "+
			"Both must point at the same latest-migration table.",
		ExpectedSchemaSentinels.Table, ExpectedSchemaProbe,
	)

	filename, content := findLatestCreateTableMigration(t)
	declConstraints, declIndexes, declColumns := extractDeclaredNames(content)

	sentinelConstraints := stringSet(ExpectedSchemaSentinels.Constraints)
	declConstraintsSet := stringSet(declConstraints)
	sentinelIndexes := stringSet(ExpectedSchemaSentinels.Indexes)
	declIndexesSet := stringSet(declIndexes)
	sentinelColumns := stringSet(ExpectedSchemaSentinels.Columns)
	declColumnsSet := stringSet(declColumns)

	// Forward: every name declared in DDL must be in the sentinel struct.
	missingFromSentinels := setDiff(declConstraintsSet, sentinelConstraints)
	require.Empty(t, missingFromSentinels,
		"%s declares constraint(s) not in ExpectedSchemaSentinels.Constraints: %v.\n"+
			"Add them to platform/internal/db/migrations.go.",
		filename, missingFromSentinels,
	)
	missingFromSentinels = setDiff(declIndexesSet, sentinelIndexes)
	require.Empty(t, missingFromSentinels,
		"%s declares index(es) not in ExpectedSchemaSentinels.Indexes: %v.\n"+
			"Add them to platform/internal/db/migrations.go.",
		filename, missingFromSentinels,
	)
	missingFromSentinels = setDiff(declColumnsSet, sentinelColumns)
	require.Empty(t, missingFromSentinels,
		"%s declares column(s) not in ExpectedSchemaSentinels.Columns: %v.\n"+
			"Add them to platform/internal/db/migrations.go.",
		filename, missingFromSentinels,
	)

	// Reverse: every entry in the sentinel struct must appear in the DDL.
	missingFromDecl := setDiff(sentinelConstraints, declConstraintsSet)
	require.Empty(t, missingFromDecl,
		"ExpectedSchemaSentinels.Constraints contains entry not declared in %s: %v.\n"+
			"Either remove the stale entry or fix the typo in platform/internal/db/migrations.go.",
		filename, missingFromDecl,
	)
	missingFromDecl = setDiff(sentinelIndexes, declIndexesSet)
	require.Empty(t, missingFromDecl,
		"ExpectedSchemaSentinels.Indexes contains entry not declared in %s: %v.\n"+
			"Either remove the stale entry or fix the typo.",
		filename, missingFromDecl,
	)
	missingFromDecl = setDiff(sentinelColumns, declColumnsSet)
	require.Empty(t, missingFromDecl,
		"ExpectedSchemaSentinels.Columns contains entry not declared in %s: %v.\n"+
			"Either remove the stale entry or fix the typo.",
		filename, missingFromDecl,
	)
}

func stringSet(xs []string) map[string]struct{} {
	out := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		out[x] = struct{}{}
	}
	return out
}

func setDiff(a, b map[string]struct{}) []string {
	var diff []string
	for x := range a {
		if _, ok := b[x]; !ok {
			diff = append(diff, x)
		}
	}
	sort.Strings(diff)
	return diff
}
