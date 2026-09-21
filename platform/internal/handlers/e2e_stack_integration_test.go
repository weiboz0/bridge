package handlers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 094 Phase 14 — live half of the E2E stack attestation.
//
// The whole point of the attestation is that it reads shared PostgreSQL
// session state, so the lock-visibility rules can only be proved against a
// real cluster. These tests take advisory locks on a second connection and
// read pg_locks; they insert no rows and therefore survive a concurrent
// truncation of the shared _test database.
//
// They reuse integrationDB (problems_integration_test.go), which skips when
// DATABASE_URL is unset and fails hard unless both the parsed and the live
// database name end in _test.

const e2eStackIntegrationTimeout = 15 * time.Second

// e2eStackLiveName returns the live database name of the handler's own pool.
func e2eStackLiveName(t *testing.T, db *sql.DB) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), e2eStackIntegrationTimeout)
	defer cancel()
	var name string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name))
	require.True(t, strings.HasSuffix(name, "_test"))
	return name
}

// e2eStackGateKey composes the gate's one-key bigint exactly as the contract
// specifies, independently of the production composition in the handler.
func e2eStackGateKey(objID int64) int64 {
	return int64(store.E2EStackLockClass)<<32 | objID
}

// e2eStackExpectedFingerprint recomputes the fingerprint from the contract
// definition rather than calling the production helper, so a change to the
// helper cannot silently change what this test accepts.
func e2eStackExpectedFingerprint(nonce, database string) string {
	digest := sha256.Sum256(append(append([]byte(nonce), 0x00), []byte(database)...))
	return hex.EncodeToString(digest[:])
}

// e2eStackHoldLocks opens a transaction on a second pool, runs every supplied
// statement in it, and returns a release function. Transaction-scoped advisory
// locks vanish on rollback, so nothing can be stranded.
func e2eStackHoldLocks(t *testing.T, holder *sql.DB, statements [][]any) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), e2eStackIntegrationTimeout)
	tx, err := holder.BeginTx(ctx, nil)
	require.NoError(t, err)
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		_ = tx.Rollback()
		cancel()
	}
	t.Cleanup(release)
	for _, statement := range statements {
		query, _ := statement[0].(string)
		_, err := tx.ExecContext(ctx, query, statement[1:]...)
		require.NoError(t, err, "holding %q", query)
	}
	return release
}

// e2eStackGrantedLocks counts granted advisory locks in the current database
// for one object id, proving a test's setup really took effect.
func e2eStackGrantedLocks(t *testing.T, db *sql.DB, objID int64) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), e2eStackIntegrationTimeout)
	defer cancel()
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM pg_locks l JOIN pg_database d ON d.oid = l.database
		WHERE d.datname = current_database() AND l.locktype = 'advisory'
		  AND l.objid = $1::int8::oid AND l.granted`, objID).Scan(&count))
	return count
}

func e2eStackLiveRouter(t *testing.T, db *sql.DB) chi.Router {
	t.Helper()
	return buildE2EStackRouter(t, true, E2EStackHandlerConfig{
		DB:          db,
		DatabaseURL: os.Getenv("DATABASE_URL"),
	})
}

// The gate holds a transaction-scoped one-key advisory lock on its own
// connection; the API process must see it through its own, separate pool.
func TestE2EStack_AttestsWhenGateLockHeld(t *testing.T) {
	db := integrationDB(t)
	holder := integrationDB(t)
	liveName := e2eStackLiveName(t, db)
	router := e2eStackLiveRouter(t, db)

	nonce := strings.Repeat("0", 64) // the shared vector's first case
	objID := e2eStackObjectID(nonce)

	// No lock yet: the same request must be refused.
	before := capture(e2eStackGet(t, router, e2eStackAttestURL(nonce)))
	absent := capture(e2eStackGet(t, router, e2eStackAbsentPath))
	require.Equal(t, http.StatusNotFound, before.status)
	require.Equal(t, absent.body, before.body)

	release := e2eStackHoldLocks(t, holder, [][]any{
		{"SELECT pg_advisory_xact_lock($1::bigint)", e2eStackGateKey(objID)},
	})
	require.Equal(t, 1, e2eStackGrantedLocks(t, db, objID), "the gate lock must really be held")

	rec := e2eStackGet(t, router, e2eStackAttestURL(nonce))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		Fingerprint string `json:"fingerprint"`
		Instance    string `json:"instance"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, e2eStackExpectedFingerprint(nonce, liveName), body.Fingerprint,
		"the fingerprint must salt the nonce with the LIVE database name")
	assert.NotEmpty(t, body.Instance)

	// Pin against the committed cross-language vector when the live database
	// is the one the vector names.
	vector := loadE2EStackVector(t)
	for _, tc := range vector.Cases {
		if tc.Nonce == nonce && tc.Database == liveName {
			assert.Equal(t, tc.Fingerprint, body.Fingerprint, "live fingerprint drifted from the shared vector")
			assert.Equal(t, tc.Objid, objID)
		}
	}

	// Releasing the gate's transaction releases the lock, and attestation
	// stops immediately — the proof is live, not cached.
	release()
	require.Equal(t, 0, e2eStackGrantedLocks(t, db, objID))
	after := capture(e2eStackGet(t, router, e2eStackAttestURL(nonce)))
	assert.Equal(t, http.StatusNotFound, after.status)
	assert.Equal(t, absent.body, after.body)
	assert.Equal(t, absent.header, after.header)
}

// LIMITATION: creating a second database needs DDL, which these tests are
// forbidden to run, so cross-cluster/cross-database isolation is proved here
// by its two observable components instead: the observe query is scoped to
// current_database() (pinned byte for byte in TestE2EStack_MatchesSharedVector)
// and the reserved (classid, objid) pair is matched exactly, so a lock that
// differs in either coordinate is invisible.
func TestE2EStack_LockInAnotherDatabaseIsNotSeen(t *testing.T) {
	db := integrationDB(t)
	holder := integrationDB(t)
	router := e2eStackLiveRouter(t, db)

	nonce := strings.Repeat("a5", 32)
	objID := e2eStackObjectID(nonce)
	otherObjID := e2eStackObjectID(strings.Repeat("f", 64))
	require.NotEqual(t, objID, otherObjID)

	vector := loadE2EStackVector(t)
	foreignClass := vector.DisjointFrom.SessionLifecycleLockClass
	require.NotEqual(t, int64(store.E2EStackLockClass), foreignClass)

	cases := []struct {
		name string
		key  int64
	}{
		{"right objid, foreign lock class", foreignClass<<32 | objID},
		{"right lock class, different objid", e2eStackGateKey(otherObjID)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			release := e2eStackHoldLocks(t, holder, [][]any{
				{"SELECT pg_advisory_xact_lock($1::bigint)", tc.key},
			})
			defer release()

			got := capture(e2eStackGet(t, router, e2eStackAttestURL(nonce)))
			absent := capture(e2eStackGet(t, router, e2eStackAbsentPath))
			assert.Equal(t, http.StatusNotFound, got.status,
				"only the exact (classid, objid, objsubid=1) triple may attest")
			assert.Equal(t, absent.body, got.body)
		})
	}

	// Positive control: the correct key on the same connection does attest,
	// so the refusals above are not an artefact of the setup.
	release := e2eStackHoldLocks(t, holder, [][]any{
		{"SELECT pg_advisory_xact_lock($1::bigint)", e2eStackGateKey(objID)},
	})
	defer release()
	rec := e2eStackGet(t, router, e2eStackAttestURL(nonce))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// Bridge's own advisory locks must never be mistaken for the gate's: the
// two-key lifecycle form has objsubid = 2, and sessions.go's one-key
// hashtext form always widens to a high word of 0 or 0xFFFFFFFF.
func TestE2EStack_LifecycleLockIsNotMistakenForGateLock(t *testing.T) {
	db := integrationDB(t)
	holder := integrationDB(t)
	router := e2eStackLiveRouter(t, db)

	vector := loadE2EStackVector(t)
	nonce := strings.Repeat("0123456789abcdef", 4)
	objID := e2eStackObjectID(nonce)
	require.Less(t, objID, int64(1)<<31)

	lifecycleClass := vector.DisjointFrom.SessionLifecycleLockClass
	replacementClass := vector.DisjointFrom.ClassReplacementLockClass
	require.NotZero(t, lifecycleClass)
	require.NotZero(t, replacementClass)

	// Four impostors, all sharing the gate's object id as their low word.
	release := e2eStackHoldLocks(t, holder, [][]any{
		// Two-key lifecycle locks: same low word, objsubid = 2.
		{"SELECT pg_advisory_xact_lock($1::int4, $2::int4)", lifecycleClass, objID},
		{"SELECT pg_advisory_xact_lock($1::int4, $2::int4)", replacementClass, objID},
		// One-key int4 hashtext locks widened to bigint: high word 0 …
		{"SELECT pg_advisory_xact_lock($1::bigint)", objID},
		// … and high word 0xFFFFFFFF (a negative hashtext result).
		{"SELECT pg_advisory_xact_lock($1::bigint)", objID - (int64(1) << 32)},
	})
	defer release()

	// All four are really held in this database on the same low word.
	require.GreaterOrEqual(t, e2eStackGrantedLocks(t, db, objID), 4,
		"the impostor locks must really be held for the refusal to mean anything")

	got := capture(e2eStackGet(t, router, e2eStackAttestURL(nonce)))
	absent := capture(e2eStackGet(t, router, e2eStackAbsentPath))
	assert.Equal(t, http.StatusNotFound, got.status,
		"a lifecycle or hashtext lock must never be accepted as the gate's lock")
	assert.Equal(t, absent.body, got.body)
	assert.Equal(t, absent.header, got.header)

	// The reserved high word is what separates them; with it, the very same
	// object id attests.
	gate := e2eStackHoldLocks(t, holder, [][]any{
		{"SELECT pg_advisory_xact_lock($1::bigint)", e2eStackGateKey(objID)},
	})
	defer gate()
	rec := e2eStackGet(t, router, e2eStackAttestURL(nonce))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}
