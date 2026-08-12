package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	defer exclusive.Rollback()
	require.NoError(t, lockSessionLifecycle(ctx, exclusive, id, false))
	blocked := make(chan error, 1)
	go func() {
		tx, err := observer.BeginTx(ctx, nil)
		if err == nil {
			err = lockSessionLifecycle(ctx, tx, id, true)
			_ = tx.Rollback()
		}
		blocked <- err
	}()
	select {
	case err := <-blocked:
		t.Fatalf("shared lock unexpectedly bypassed exclusive lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	require.NoError(t, exclusive.Commit())
	require.NoError(t, <-blocked)
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

func TestSessionLifecycleConfirmedEndRollsBackWhenSnapshotCountMismatches(t *testing.T) {
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
			assert.False(t, result.WhiteboardServerArchiveComplete)
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
			assert.Equal(t, complete, result.WhiteboardServerArchiveComplete)
			var archive bool
			var residue sql.NullString
			require.NoError(t, db.QueryRowContext(ctx, `SELECT whiteboard_server_archive_complete, canvas_freeze_token FROM sessions WHERE id = $1`, session.ID).Scan(&archive, &residue))
			assert.Equal(t, complete, archive)
			assert.False(t, residue.Valid)
		})
	}
}
