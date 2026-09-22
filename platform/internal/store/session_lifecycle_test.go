package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plan 094 Phase 14 — the gate's advisory-lock namespace.
//
// E2EStackLockClass is the high word of a one-key bigint advisory lock held by
// the local gate while every E2E service proves it can see it. If that class
// ever collided with a lifecycle class, a real Bridge transaction could either
// block the gate or be mistaken for it, and the E2E tier would be authorised
// to mutate a stack the gate never proved anything about.
func TestE2EStackLockClass_DisjointFromLifecycleClasses(t *testing.T) {
	classes := map[string]int32{
		"E2EStackLockClass":         E2EStackLockClass,
		"sessionLifecycleLockClass": sessionLifecycleLockClass,
		"classReplacementLockClass": classReplacementLockClass,
	}
	seen := map[int32]string{}
	for name, value := range classes {
		if other, duplicate := seen[value]; duplicate {
			t.Fatalf("advisory lock classes %s and %s share the value %d", other, name, value)
		}
		seen[value] = name
	}
	require.Len(t, seen, 3, "the three reserved lock classes must be pairwise distinct")

	// The one-key pg_advisory_xact_lock(hashtext(...)) form in sessions.go
	// widens an int4 to a high word of 0 (non-negative) or 0xFFFFFFFF
	// (negative); the reserved class must be neither, or a routine session
	// lock would be indistinguishable from the gate's.
	asUint := uint32(E2EStackLockClass)
	assert.NotEqual(t, uint32(0), asUint, "a zero high word is exactly what a positive hashtext lock produces")
	assert.NotEqual(t, uint32(0xFFFFFFFF), asUint, "an all-ones high word is exactly what a negative hashtext lock produces")
	assert.Positive(t, E2EStackLockClass, "the class is composed with <<32 and must not sign-extend")

	// The composed key stays a positive bigint across the whole 31-bit object
	// id range, so PostgreSQL never sees a negative key and the decimal form
	// the gate script computes in BigInt matches Go's int64.
	for _, objid := range []int64{0, 1, math.MaxInt32 - 1, math.MaxInt32} {
		key := int64(E2EStackLockClass)<<32 | objid
		assert.Positive(t, key, "composed key for objid %d must be a positive bigint", objid)
		assert.Equal(t, int64(E2EStackLockClass), key>>32, "the high word must survive composition")
		assert.Equal(t, objid, key&0xFFFFFFFF, "the low word must survive composition")
	}

	// The same three values are pinned in the cross-language contract vector
	// that Go, Bun, Vitest, and the gate selftests all assert.
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "tests", "e2e-stack-vector.json"))
	require.NoError(t, err)
	var vector struct {
		Contract struct {
			LockClass int64 `json:"lockClass"`
		} `json:"contract"`
		DisjointFrom struct {
			SessionLifecycleLockClass int64   `json:"sessionLifecycleLockClass"`
			ClassReplacementLockClass int64   `json:"classReplacementLockClass"`
			HashtextOneKeyHighWords   []int64 `json:"hashtextOneKeyHighWords"`
		} `json:"disjointFrom"`
		ComposeKeyBoundaries []struct {
			Objid int64  `json:"objid"`
			Key   string `json:"key"`
		} `json:"composeKeyBoundaries"`
	}
	require.NoError(t, json.Unmarshal(raw, &vector))
	assert.Equal(t, vector.Contract.LockClass, int64(E2EStackLockClass))
	assert.Equal(t, vector.DisjointFrom.SessionLifecycleLockClass, int64(sessionLifecycleLockClass))
	assert.Equal(t, vector.DisjointFrom.ClassReplacementLockClass, int64(classReplacementLockClass))
	require.NotEmpty(t, vector.DisjointFrom.HashtextOneKeyHighWords)
	for _, highWord := range vector.DisjointFrom.HashtextOneKeyHighWords {
		assert.NotEqual(t, highWord, int64(asUint), "the reserved class must differ from every hashtext high word")
	}
}

func TestSessionLifecycleAdvisoryKeyVectors(t *testing.T) {
	for _, tc := range []struct {
		prefix string
		want   int32
	}{
		{"00000000", 0},
		{"12345678", 305419896},
		{"7fffffff", 2147483647},
		{"80000000", -2147483648},
		{"ffffffff", -1},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			id := tc.prefix + "-0000-0000-0000-000000000000"
			got, err := sessionLifecycleAdvisoryKey(id)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSessionLifecycleUsesSharedAndExclusiveTransactionLocks(t *testing.T) {
	db := testDB(t)
	observer := testDB(t)
	probe := testDB(t)
	ctx := context.Background()
	id := "12345678-0000-0000-0000-000000000000"
	sharedOne, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer sharedOne.Rollback()
	require.NoError(t, lockSessionLifecycle(ctx, sharedOne, id, true))
	sharedTwo, err := observer.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, lockSessionLifecycle(ctx, sharedTwo, id, true), "two shared xact locks must coexist")
	require.NoError(t, sharedTwo.Rollback())
	require.NoError(t, sharedOne.Rollback())

	exclusive, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = exclusive.Rollback() })
	require.NoError(t, lockSessionLifecycle(ctx, exclusive, id, false))
	var holderPID int
	require.NoError(t, exclusive.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID))
	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	t.Cleanup(cancel)
	waiterPID := make(chan int, 1)
	completed := make(chan error, 1)
	go func() {
		tx, err := observer.BeginTx(timeoutCtx, nil)
		if err == nil {
			defer tx.Rollback()
			var pid int
			err = tx.QueryRowContext(timeoutCtx, `SELECT pg_backend_pid()`).Scan(&pid)
			if err == nil {
				waiterPID <- pid
			}
			if err == nil {
				err = lockSessionLifecycle(timeoutCtx, tx, id, true)
			}
			_ = tx.Rollback()
		}
		completed <- err
	}()
	var waiter int
	select {
	case waiter = <-waiterPID:
	case <-timeoutCtx.Done():
		t.Fatal("waiter did not publish backend PID")
	}
	require.NoError(t, waitForCanvasSessionLock(timeoutCtx, probe, waiter, holderPID), "waiter must be blocked on holder's advisory lock")
	select {
	case err := <-completed:
		t.Fatalf("shared lock unexpectedly bypassed exclusive lock: %v", err)
	default:
	}
	require.NoError(t, exclusive.Commit())
	select {
	case err := <-completed:
		require.NoError(t, err)
	case <-timeoutCtx.Done():
		t.Fatal("waiter did not complete after advisory-lock release")
	}
}

func TestSessionLifecycleReplacementOrderUsesDerivedKeyThenUUID(t *testing.T) {
	ids := []string{
		"12345678-0000-0000-0000-000000000002",
		"80000000-0000-0000-0000-000000000001",
		"12345678-0000-0000-0000-000000000001",
	}
	got, err := sortedSessionLifecycleIDs(ids)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"80000000-0000-0000-0000-000000000001",
		"12345678-0000-0000-0000-000000000001",
		"12345678-0000-0000-0000-000000000002",
	}, got)
}

func TestCreateSessionCollisionKeysLockInUUIDOrderUnderLegacyPressure(t *testing.T) {
	db := testDB(t)
	observer := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name()+uuid.NewString())
	ids := []string{"12345678-0000-0000-0000-000000000002", "12345678-0000-0000-0000-000000000001"}
	for _, id := range ids {
		_, err := db.ExecContext(ctx, `INSERT INTO sessions (id, class_id, teacher_id, title, status, settings, started_at, visibility) VALUES ($1, $2, $3, $4, 'live', '{}', clock_timestamp(), 'unlisted')`, id, classID, teacherID, id)
		require.NoError(t, err)
	}
	locked := make(chan []string, 1)
	sessions.testHooks = &sessionStoreTestHooks{lockedLifecycleIDs: func(ids []string) { locked <- append([]string(nil), ids...) }}
	legacy, err := observer.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer legacy.Rollback()
	_, err = legacy.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "session_create:"+teacherID)
	require.NoError(t, err)
	_, err = sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "replacement"})
	require.NoError(t, err)
	assert.Equal(t, []string{ids[1], ids[0]}, <-locked)
}

func TestReplacementRacePreservesExplicitConfirmedResult(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name()+uuid.NewString())
	prior, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "prior"})
	require.NoError(t, err)
	token := uuid.NewString()
	_, err = acquireSessionFreezeLease(ctx, db, prior.ID, token)
	require.NoError(t, err)

	guarded, release := make(chan struct{}), make(chan struct{})
	sessions.testHooks = &sessionStoreTestHooks{afterClassGuard: func() { close(guarded); <-release }}
	createDone := make(chan error, 1)
	go func() {
		_, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "replacement"})
		createDone <- err
	}()
	<-guarded
	require.NoError(t, completeSessionConfirmed(ctx, testDB(t), prior.ID, token, nil))
	close(release)
	require.NoError(t, <-createDone)
	var archive bool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT whiteboard_server_archive_complete FROM sessions WHERE id = $1`, prior.ID).Scan(&archive))
	assert.True(t, archive, "replacement must not overwrite a confirmed archive result")
}

func TestCreateSessionReplacementRaceWinsBeforeExplicitEndWithoutDeadlock(t *testing.T) {
	db := testDB(t)
	observer := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name()+uuid.NewString())
	prior, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "prior"})
	require.NoError(t, err)
	token := uuid.NewString()
	_, err = acquireSessionFreezeLease(ctx, db, prior.ID, token)
	require.NoError(t, err)

	locked, release := make(chan struct{}), make(chan struct{})
	sessions.testHooks = &sessionStoreTestHooks{afterLifecycleLocks: func() { close(locked); <-release }}
	createDone := make(chan error, 1)
	go func() {
		_, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "replacement"})
		createDone <- err
	}()
	<-locked
	endDone := make(chan error, 1)
	go func() { endDone <- completeSessionConfirmed(ctx, observer, prior.ID, token, nil) }()
	select {
	case err := <-endDone:
		t.Fatalf("confirmed end bypassed replacement lifecycle lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-createDone)
	assert.ErrorIs(t, <-endDone, ErrSessionEndInProgress)
	var archive bool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT whiteboard_server_archive_complete FROM sessions WHERE id = $1`, prior.ID).Scan(&archive))
	assert.False(t, archive, "replacement's degraded result is the only allowed result when it acquired the lifecycle lock first")
}

func TestSessionLifecycleLeaseUsesDatabaseClockAndExactToken(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "lease"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, session.ID) })

	token := uuid.NewString()
	lease, err := acquireSessionFreezeLease(ctx, db, session.ID, token)
	require.NoError(t, err)
	assert.Equal(t, token, lease.Token)
	assert.Greater(t, lease.Remaining, 14*time.Second)
	assert.LessOrEqual(t, lease.Remaining, 15*time.Second)

	allowed, err := validateSessionFreezeLease(ctx, db, session.ID, token)
	require.NoError(t, err)
	assert.True(t, allowed.Allowed)
	assert.Positive(t, allowed.Remaining.Milliseconds())

	foreign, err := acquireSessionFreezeLease(ctx, db, session.ID, uuid.NewString())
	assert.ErrorIs(t, err, ErrSessionEndInProgress)
	assert.Empty(t, foreign.Token)
}

func TestSessionLifecycleDatabaseClockExpiresThenReplacesLease(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name()+uuid.NewString())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "expiry replacement"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, session.ID) })
	first, second := uuid.NewString(), uuid.NewString()
	_, err = acquireSessionFreezeLease(ctx, db, session.ID, first)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE sessions SET canvas_freeze_until = clock_timestamp() - interval '1 millisecond' WHERE id = $1`, session.ID)
	require.NoError(t, err)
	lease, err := acquireSessionFreezeLease(ctx, db, session.ID, second)
	require.NoError(t, err)
	assert.Equal(t, second, lease.Token)
	assert.Greater(t, lease.Remaining, 14*time.Second)
}

func TestSessionLifecycleConfirmedEndRejectsMatchingExpiredLeaseWithoutWrites(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	canvases := NewCanvasStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name()+uuid.NewString())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "confirmed expiry"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM sessions WHERE id = $1`, session.ID) })
	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "state", Visibility: "private"})
	require.NoError(t, err)
	token := uuid.NewString()
	_, err = acquireSessionFreezeLease(ctx, db, session.ID, token)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE sessions SET canvas_freeze_until = clock_timestamp() - interval '1 millisecond' WHERE id = $1`, session.ID)
	require.NoError(t, err)
	err = completeSessionConfirmed(ctx, db, session.ID, token, []CanvasSnapshot{{CanvasID: canvas.ID, YjsState: "must-not-write"}})
	assert.ErrorIs(t, err, ErrSessionEndInProgress)
	var status string
	var archive sql.NullBool
	var state sql.NullString
	require.NoError(t, db.QueryRowContext(ctx, `SELECT status, whiteboard_server_archive_complete FROM sessions WHERE id = $1`, session.ID).Scan(&status, &archive))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT yjs_state FROM session_canvases WHERE id = $1`, canvas.ID).Scan(&state))
	assert.Equal(t, "live", status)
	assert.False(t, archive.Valid)
	assert.False(t, state.Valid)
}

func TestSessionLifecycleRejectsMalformedLeasePairs(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name()+uuid.NewString())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "pair"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM sessions WHERE id = $1`, session.ID) })
	_, err = db.ExecContext(ctx, `UPDATE sessions SET canvas_freeze_token = $2::uuid, canvas_freeze_until = NULL WHERE id = $1`, session.ID, uuid.NewString())
	require.Error(t, err)
	_, err = db.ExecContext(ctx, `UPDATE sessions SET canvas_freeze_token = NULL, canvas_freeze_until = clock_timestamp() WHERE id = $1`, session.ID)
	require.Error(t, err)
}

func TestSessionLifecycleAlreadyEndedPreservesNullableArchiveAndEmptyTokenCleanup(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name()+uuid.NewString())
	for _, tc := range []struct {
		name    string
		archive any
		want    *bool
	}{
		{"true", true, lifecycleBoolPtr(true)},
		{"false", false, lifecycleBoolPtr(false)},
		{"null", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: t.Name()})
			require.NoError(t, err)
			t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM sessions WHERE id = $1`, session.ID) })
			token := uuid.NewString()
			_, err = db.ExecContext(ctx, `UPDATE sessions SET status='ended', whiteboard_server_archive_complete=$2, canvas_freeze_token=$3::uuid, canvas_freeze_until=clock_timestamp()-interval '1 second' WHERE id=$1`, session.ID, tc.archive, token)
			require.NoError(t, err)
			result, err := completeSessionDegradedResult(ctx, db, session.ID, "")
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.WhiteboardServerArchiveComplete)
			var residue sql.NullString
			require.NoError(t, db.QueryRowContext(ctx, `SELECT canvas_freeze_token FROM sessions WHERE id=$1`, session.ID).Scan(&residue))
			assert.False(t, residue.Valid)
		})
	}
}

func TestEndSession_OverlappingRequestsCannotClearForeignLease(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name()+uuid.NewString())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "unexpired residue"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM sessions WHERE id = $1`, session.ID) })
	token := uuid.NewString()
	_, err = acquireSessionFreezeLease(ctx, db, session.ID, token)
	require.NoError(t, err)
	_, err = sessions.EndSession(ctx, session.ID)
	assert.ErrorIs(t, err, ErrSessionEndInProgress)
	var saved string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT canvas_freeze_token FROM sessions WHERE id=$1`, session.ID).Scan(&saved))
	assert.Equal(t, token, saved)
}

func TestEndSession_DifferentExpiredTokenEndsDegradedClearsLeaseWithoutReusingFreeze(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name()+uuid.NewString())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "expired foreign lease"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM sessions WHERE id = $1`, session.ID) })

	ownerToken, staleToken := uuid.NewString(), uuid.NewString()
	_, err = db.ExecContext(ctx, `UPDATE sessions
		SET canvas_freeze_token = $2::uuid,
			canvas_freeze_until = clock_timestamp() - interval '1 millisecond'
		WHERE id = $1`, session.ID, ownerToken)
	require.NoError(t, err)

	result, err := completeSessionDegradedResult(ctx, db, session.ID, staleToken)
	require.NoError(t, err)
	require.NotNil(t, result.WhiteboardServerArchiveComplete)
	assert.False(t, *result.WhiteboardServerArchiveComplete)
	var status string
	var lease sql.NullString
	require.NoError(t, db.QueryRowContext(ctx, `SELECT status, canvas_freeze_token FROM sessions WHERE id = $1`, session.ID).Scan(&status, &lease))
	assert.Equal(t, "ended", status)
	assert.False(t, lease.Valid, "an expired foreign operation cannot be reused or retained")
}

func lifecycleBoolPtr(value bool) *bool { return &value }

func TestEndSession_BatchFailureRollsBackTrueAndSnapshots(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "confirmed rollback"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, session.ID) })

	token := uuid.NewString()
	_, err = acquireSessionFreezeLease(ctx, db, session.ID, token)
	require.NoError(t, err)
	err = completeSessionConfirmed(ctx, db, session.ID, token, []CanvasSnapshot{{CanvasID: uuid.NewString(), YjsState: "snapshot"}})
	assert.ErrorIs(t, err, ErrSessionSnapshotCountMismatch)

	var status string
	var archiveComplete sql.NullBool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT status, whiteboard_server_archive_complete FROM sessions WHERE id = $1`, session.ID).Scan(&status, &archiveComplete))
	assert.Equal(t, "live", status)
	assert.False(t, archiveComplete.Valid)
}

func TestSessionLifecycleConfirmedEndPersistsBundleAndZeroSnapshot(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	canvases := NewCanvasStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())

	newSession := func(t *testing.T) *LiveSession {
		t.Helper()
		session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: t.Name()})
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, session.ID) })
		return session
	}
	t.Run("bundle", func(t *testing.T) {
		session := newSession(t)
		canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "state", Visibility: "private"})
		require.NoError(t, err)
		token := uuid.NewString()
		_, err = acquireSessionFreezeLease(ctx, db, session.ID, token)
		require.NoError(t, err)
		require.NoError(t, completeSessionConfirmed(ctx, db, session.ID, token, []CanvasSnapshot{{CanvasID: canvas.ID, YjsState: "final"}}))
		var status, state string
		var complete bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT status, whiteboard_server_archive_complete FROM sessions WHERE id = $1`, session.ID).Scan(&status, &complete))
		require.NoError(t, db.QueryRowContext(ctx, `SELECT yjs_state FROM session_canvases WHERE id = $1`, canvas.ID).Scan(&state))
		assert.Equal(t, "ended", status)
		assert.True(t, complete)
		assert.Equal(t, "final", state)
	})
	t.Run("zero snapshot", func(t *testing.T) {
		session := newSession(t)
		token := uuid.NewString()
		_, err := acquireSessionFreezeLease(ctx, db, session.ID, token)
		require.NoError(t, err)
		require.NoError(t, completeSessionConfirmed(ctx, db, session.ID, token, nil))
		var complete bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT whiteboard_server_archive_complete FROM sessions WHERE id = $1`, session.ID).Scan(&complete))
		assert.True(t, complete)
	})
}

func TestSessionLifecycleDegradedEndClearsOnlyMatchingLease(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "degraded"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, session.ID) })

	token := uuid.NewString()
	_, err = acquireSessionFreezeLease(ctx, db, session.ID, token)
	require.NoError(t, err)
	err = completeSessionDegraded(ctx, db, session.ID, uuid.NewString())
	assert.ErrorIs(t, err, ErrSessionEndInProgress)

	var saved string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT canvas_freeze_token FROM sessions WHERE id = $1`, session.ID).Scan(&saved))
	assert.Equal(t, token, saved)
	require.NoError(t, completeSessionDegraded(ctx, db, session.ID, token))

	var status string
	var archiveComplete bool
	var cleared sql.NullString
	require.NoError(t, db.QueryRowContext(ctx, `SELECT status, whiteboard_server_archive_complete, canvas_freeze_token FROM sessions WHERE id = $1`, session.ID).Scan(&status, &archiveComplete, &cleared))
	assert.Equal(t, "ended", status)
	assert.False(t, archiveComplete)
	assert.False(t, cleared.Valid)
}

func TestSessionLifecycleAbortAndEndedCleanupRespectLeaseOwnership(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())

	newSession := func(t *testing.T) *LiveSession {
		t.Helper()
		session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: t.Name()})
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, session.ID) })
		return session
	}

	t.Run("abort clears only matching live token", func(t *testing.T) {
		session := newSession(t)
		matching, foreign := uuid.NewString(), uuid.NewString()
		_, err := acquireSessionFreezeLease(ctx, db, session.ID, matching)
		require.NoError(t, err)
		require.NoError(t, abortSessionFreezeLease(ctx, db, session.ID, foreign))
		var token string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT canvas_freeze_token FROM sessions WHERE id = $1`, session.ID).Scan(&token))
		assert.Equal(t, matching, token)
		require.NoError(t, abortSessionFreezeLease(ctx, db, session.ID, matching))
		var cleared sql.NullString
		require.NoError(t, db.QueryRowContext(ctx, `SELECT canvas_freeze_token FROM sessions WHERE id = $1`, session.ID).Scan(&cleared))
		assert.False(t, cleared.Valid)
	})

	for _, tc := range []struct {
		name              string
		expired, matching bool
		wantConflict      bool
	}{
		{"matching unexpired", false, true, false},
		{"matching expired", true, true, false},
		{"different expired", true, false, false},
		{"different unexpired", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := newSession(t)
			owner, request := uuid.NewString(), uuid.NewString()
			if tc.matching {
				request = owner
			}
			_, err := acquireSessionFreezeLease(ctx, db, session.ID, owner)
			require.NoError(t, err)
			if tc.expired {
				_, err = db.ExecContext(ctx, `UPDATE sessions SET canvas_freeze_until = clock_timestamp() - interval '1 second' WHERE id = $1`, session.ID)
				require.NoError(t, err)
			}
			result, err := completeSessionDegradedResult(ctx, db, session.ID, request)
			if tc.wantConflict {
				assert.ErrorIs(t, err, ErrSessionEndInProgress)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result.WhiteboardServerArchiveComplete)
			assert.False(t, *result.WhiteboardServerArchiveComplete)
			var status string
			var residue sql.NullString
			require.NoError(t, db.QueryRowContext(ctx, `SELECT status, canvas_freeze_token FROM sessions WHERE id = $1`, session.ID).Scan(&status, &residue))
			assert.Equal(t, "ended", status)
			assert.False(t, residue.Valid)
		})
	}

	for _, complete := range []bool{true, false} {
		t.Run("already ended archive result", func(t *testing.T) {
			session := newSession(t)
			token := uuid.NewString()
			_, err := db.ExecContext(ctx, `UPDATE sessions SET status = 'ended', whiteboard_server_archive_complete = $2, canvas_freeze_token = $3::uuid, canvas_freeze_until = clock_timestamp() - interval '1 second' WHERE id = $1`, session.ID, complete, token)
			require.NoError(t, err)
			result, err := completeSessionDegradedResult(ctx, db, session.ID, uuid.NewString())
			require.NoError(t, err)
			require.NotNil(t, result.WhiteboardServerArchiveComplete)
			assert.Equal(t, complete, *result.WhiteboardServerArchiveComplete)
			var archive bool
			var residue sql.NullString
			require.NoError(t, db.QueryRowContext(ctx, `SELECT whiteboard_server_archive_complete, canvas_freeze_token FROM sessions WHERE id = $1`, session.ID).Scan(&archive, &residue))
			assert.Equal(t, complete, archive)
			assert.False(t, residue.Valid)
		})
	}
}
