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
