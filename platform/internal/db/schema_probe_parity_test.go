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

// Plan 094 — multi-object sentinel parity. The regexes match
// drizzle/0028_session_canvases.sql's CREATE TABLE, ALTER TABLE ADD COLUMN,
// CREATE INDEX, and CREATE TYPE AS ENUM forms. Future migrations using more
// unusual DDL styles may need parser updates. Comments are stripped first so
// commented-out DDL does not register as a real declaration.
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
	filename, content := findLatestCreateTableMigration(t)
	declared := extractDeclaredSchema(content)
	require.True(t, hasTable(ExpectedSchemaSentinels, ExpectedSchemaProbe),
		"ExpectedSchemaProbe %q must be one of ExpectedSchemaSentinels.Tables", ExpectedSchemaProbe)
	assertTableSentinelParity(t, filename, declared.Tables, ExpectedSchemaSentinels.Tables)
	assertEnumSentinelParity(t, filename, declared.Enums, ExpectedSchemaSentinels.Enums)
}

func TestExtractDeclaredSchema_CapturesAlterColumnAndOrderedEnum(t *testing.T) {
	_, content := findLatestCreateTableMigration(t)
	declared := extractDeclaredSchema(content)
	require.Contains(t, declared.Tables, SchemaTableSentinels{
		Table:   "sessions",
		Columns: []string{"canvas_floor", "canvas_freeze_token", "canvas_freeze_until", "whiteboard_server_archive_complete"},
	})
	require.Contains(t, declared.Enums, SchemaEnumSentinel{
		Name:   "canvas_visibility",
		Labels: []string{"private", "host", "participants", "session"},
	})
}

var (
	alterTableAddColumnRE = regexp.MustCompile(`(?ims)ALTER\s+TABLE\s+"?(\w+)"?\s+ADD\s+COLUMN(?:\s+IF\s+NOT\s+EXISTS)?\s+"?(\w+)"?\s+\w+`)
	indexTableRE          = regexp.MustCompile(`(?im)^\s*CREATE\s+(?:UNIQUE\s+)?INDEX(?:\s+IF\s+NOT\s+EXISTS)?\s+(\w+)\s+ON\s+"?(\w+)"?`)
	createEnumRE          = regexp.MustCompile(`(?is)CREATE\s+TYPE\s+"?(\w+)"?\s+AS\s+ENUM\s*\(([^)]*)\)`)
	enumLabelRE           = regexp.MustCompile(`'((?:''|[^'])*)'`)
)

// extractDeclaredSchema reads the latest migration as a multi-object schema
// contract: CREATE TABLE columns/constraints, ALTER TABLE ADD COLUMN, indexes
// by owning table, and CREATE TYPE AS ENUM labels in declaration order.
func extractDeclaredSchema(content string) SchemaSentinels {
	stripped := lineCommentRE.ReplaceAllString(content, "")
	tables := make(map[string]*SchemaTableSentinels)
	getTable := func(name string) *SchemaTableSentinels {
		if table := tables[name]; table != nil {
			return table
		}
		table := &SchemaTableSentinels{Table: name}
		tables[name] = table
		return table
	}

	matches := createTableRE.FindAllStringSubmatch(stripped, -1)
	bodies := extractCreateTableBodies(stripped)
	for i, match := range matches {
		if i >= len(bodies) {
			break
		}
		table := getTable(match[1])
		for _, column := range columnLineRE.FindAllStringSubmatch(bodies[i], -1) {
			ident := column[1]
			switch strings.ToUpper(ident) {
			case "CONSTRAINT", "PRIMARY", "UNIQUE", "CREATE", "FOREIGN", "CHECK":
				continue
			}
			table.Columns = append(table.Columns, ident)
		}
		for _, constraint := range constraintNameRE.FindAllStringSubmatch(bodies[i], -1) {
			table.Constraints = append(table.Constraints, constraint[1])
		}
	}
	for _, match := range alterTableAddColumnRE.FindAllStringSubmatch(stripped, -1) {
		getTable(match[1]).Columns = append(getTable(match[1]).Columns, match[2])
	}
	for _, match := range indexTableRE.FindAllStringSubmatch(stripped, -1) {
		getTable(match[2]).Indexes = append(getTable(match[2]).Indexes, match[1])
	}

	var result SchemaSentinels
	for _, table := range tables {
		result.Tables = append(result.Tables, *table)
	}
	sort.Slice(result.Tables, func(i, j int) bool { return result.Tables[i].Table < result.Tables[j].Table })
	for _, match := range createEnumRE.FindAllStringSubmatch(stripped, -1) {
		enum := SchemaEnumSentinel{Name: match[1]}
		for _, label := range enumLabelRE.FindAllStringSubmatch(match[2], -1) {
			enum.Labels = append(enum.Labels, strings.ReplaceAll(label[1], "''", "'"))
		}
		result.Enums = append(result.Enums, enum)
	}
	sort.Slice(result.Enums, func(i, j int) bool { return result.Enums[i].Name < result.Enums[j].Name })
	return result
}

func assertTableSentinelParity(t *testing.T, filename string, declared, expected []SchemaTableSentinels) {
	t.Helper()
	declaredByName := make(map[string]SchemaTableSentinels, len(declared))
	for _, table := range declared {
		declaredByName[table.Table] = table
	}
	expectedByName := make(map[string]SchemaTableSentinels, len(expected))
	for _, table := range expected {
		expectedByName[table.Table] = table
	}
	for name, actual := range declaredByName {
		expectedTable, ok := expectedByName[name]
		require.True(t, ok, "%s declares table object %q missing from ExpectedSchemaSentinels.Tables", filename, name)
		require.Empty(t, setDiff(stringSet(actual.Columns), stringSet(expectedTable.Columns)), "%s table %q has column(s) missing from sentinels", filename, name)
		require.Empty(t, setDiff(stringSet(actual.Constraints), stringSet(expectedTable.Constraints)), "%s table %q has constraint(s) missing from sentinels", filename, name)
		require.Empty(t, setDiff(stringSet(actual.Indexes), stringSet(expectedTable.Indexes)), "%s table %q has index(es) missing from sentinels", filename, name)
	}
	for name, expectedTable := range expectedByName {
		actual, ok := declaredByName[name]
		require.True(t, ok, "ExpectedSchemaSentinels.Tables contains stale table %q not declared in %s", name, filename)
		require.Empty(t, setDiff(stringSet(expectedTable.Columns), stringSet(actual.Columns)), "ExpectedSchemaSentinels table %q has stale/typo column(s)", name)
		require.Empty(t, setDiff(stringSet(expectedTable.Constraints), stringSet(actual.Constraints)), "ExpectedSchemaSentinels table %q has stale/typo constraint(s)", name)
		require.Empty(t, setDiff(stringSet(expectedTable.Indexes), stringSet(actual.Indexes)), "ExpectedSchemaSentinels table %q has stale/typo index(es)", name)
	}
}

func assertEnumSentinelParity(t *testing.T, filename string, declared, expected []SchemaEnumSentinel) {
	t.Helper()
	declaredByName := make(map[string]SchemaEnumSentinel, len(declared))
	for _, enum := range declared {
		declaredByName[enum.Name] = enum
	}
	expectedByName := make(map[string]SchemaEnumSentinel, len(expected))
	for _, enum := range expected {
		expectedByName[enum.Name] = enum
	}
	for name, actual := range declaredByName {
		expectedEnum, ok := expectedByName[name]
		require.True(t, ok, "%s declares enum %q missing from ExpectedSchemaSentinels.Enums", filename, name)
		require.Equal(t, actual.Labels, expectedEnum.Labels, "%s enum %q labels must match exactly in declaration order", filename, name)
	}
	for name, expectedEnum := range expectedByName {
		actual, ok := declaredByName[name]
		require.True(t, ok, "ExpectedSchemaSentinels.Enums contains stale enum %q not declared in %s", name, filename)
		require.Equal(t, actual.Labels, expectedEnum.Labels, "ExpectedSchemaSentinels enum %q labels are stale, typoed, or out of order", name)
	}
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
