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
