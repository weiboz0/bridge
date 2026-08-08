package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanvasStore_DefaultFloor(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		TeacherID: teacherID,
		Title:     "Canvas floor",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })

	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{
		SessionID:  session.ID,
		OwnerID:    teacherID,
		Title:      "Private request",
		Visibility: "private",
	})
	require.NoError(t, err)
	assert.Equal(t, "private", canvas.Visibility)

	var floor string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT canvas_floor FROM sessions WHERE id = $1`, session.ID).Scan(&floor))
	assert.Equal(t, "private", floor)
}

func TestCanvasStore_MigrationBackfillsCanvasFloor(t *testing.T) {
	migration, err := os.ReadFile("../../../drizzle/0028_session_canvases.sql")
	require.NoError(t, err)
	sql := string(migration)
	assert.True(t, strings.Contains(sql, "UPDATE sessions\nSET canvas_floor = 'private'\nWHERE canvas_floor IS NULL"))
	assert.True(t, strings.Contains(sql, "ALTER COLUMN canvas_floor SET DEFAULT 'private'"))
	assert.True(t, strings.Contains(sql, "ALTER COLUMN canvas_floor SET NOT NULL"))
}

func TestCanvasStore_CreateGreatestWithSessionFloor(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())

	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Canvas greatest"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	_, err = canvases.SetSessionCanvasFloor(ctx, session.ID, teacherID, "host")
	require.NoError(t, err)

	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Board", Visibility: "private"})
	require.NoError(t, err)
	assert.Equal(t, "host", canvas.Visibility)
}

func TestCanvasStore_GetCanvasScopesToSession(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())

	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Canvas get"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Board", Visibility: "private"})
	require.NoError(t, err)

	fetched, err := canvases.GetCanvas(ctx, session.ID, canvas.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, canvas.ID, fetched.ID)

	notFound, err := canvases.GetCanvas(ctx, "00000000-0000-0000-0000-000000000000", canvas.ID)
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestCanvasStore_SetVisibility_RejectsTightenAndBelowFloor(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())

	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Canvas visibility"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Board", Visibility: "host"})
	require.NoError(t, err)

	updated, err := canvases.SetCanvasVisibility(ctx, session.ID, canvas.ID, teacherID, "participants")
	require.NoError(t, err)
	assert.Equal(t, "participants", updated.Visibility)

	_, err = canvases.SetCanvasVisibility(ctx, session.ID, canvas.ID, teacherID, "host")
	assert.ErrorIs(t, err, ErrCanvasVisibilityTighten)
	_, err = canvases.SetCanvasVisibility(ctx, session.ID, canvas.ID, teacherID, "participants")
	assert.ErrorIs(t, err, ErrCanvasVisibilityTighten)

	_, err = canvases.SetSessionCanvasFloor(ctx, session.ID, teacherID, "participants")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE session_canvases SET visibility = 'private' WHERE id = $1`, canvas.ID)
	require.NoError(t, err)
	_, err = canvases.SetCanvasVisibility(ctx, session.ID, canvas.ID, teacherID, "host")
	assert.ErrorIs(t, err, ErrCanvasBelowFloor)
}

func TestCanvasStore_SetFloor_RejectsSession(t *testing.T) {
	_, err := NewCanvasStore(nil).SetSessionCanvasFloor(context.Background(), "ignored", "ignored", "session")
	assert.ErrorIs(t, err, ErrCanvasFloorTooLoose)
}

func TestCanvasStore_RaiseFloor_BumpsCanvases(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())

	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Canvas floor raise"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Board", Visibility: "private"})
	require.NoError(t, err)

	floor, err := canvases.SetSessionCanvasFloor(ctx, session.ID, teacherID, "participants")
	require.NoError(t, err)
	assert.Equal(t, "participants", floor)

	updated, err := canvases.GetCanvas(ctx, session.ID, canvas.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "participants", updated.Visibility)

	_, err = canvases.SetSessionCanvasFloor(ctx, session.ID, teacherID, "session")
	assert.ErrorIs(t, err, ErrCanvasFloorTooLoose)
}

func TestCanvasStore_ListVisible_ByRole(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	owner := createTestUser(t, db, users, t.Name()+"-owner")
	participant := createTestUser(t, db, users, t.Name()+"-participant")
	invitee := createTestUser(t, db, users, t.Name()+"-invitee")
	outsider := createTestUser(t, db, users, t.Name()+"-outsider")

	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Canvas list"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	_, err = sessions.JoinSession(ctx, session.ID, participant.ID)
	require.NoError(t, err)
	_, err = sessions.AddParticipant(ctx, session.ID, invitee.ID, teacherID)
	require.NoError(t, err)

	for _, input := range []CreateCanvasInput{
		{SessionID: session.ID, OwnerID: owner.ID, Title: "private", Visibility: "private"},
		{SessionID: session.ID, OwnerID: owner.ID, Title: "host", Visibility: "host"},
		{SessionID: session.ID, OwnerID: owner.ID, Title: "participants", Visibility: "participants"},
		{SessionID: session.ID, OwnerID: owner.ID, Title: "session", Visibility: "session"},
	} {
		_, err = canvases.CreateCanvas(ctx, input)
		require.NoError(t, err)
	}

	assertCanvasTitles := func(userID string, want ...string) {
		t.Helper()
		got, err := canvases.ListVisibleCanvases(ctx, session.ID, userID)
		require.NoError(t, err)
		titles := make([]string, 0, len(got))
		for _, canvas := range got {
			titles = append(titles, canvas.Title)
		}
		assert.ElementsMatch(t, want, titles)
	}
	assertCanvasTitles(owner.ID, "private", "host", "participants", "session")
	assertCanvasTitles(teacherID, "host", "participants", "session")
	assertCanvasTitles(participant.ID, "participants", "session")
	assertCanvasTitles(invitee.ID, "session")
	assertCanvasTitles(outsider.ID)
}

func TestCanvasStore_DeleteCanvasRemovesPersistedDocument(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())

	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Canvas delete"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Board", Visibility: "private"})
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE session_canvases SET yjs_state = 'persisted' WHERE id = $1`, canvas.ID)
	require.NoError(t, err)

	deleted, err := canvases.DeleteCanvas(ctx, session.ID, canvas.ID, teacherID)
	require.NoError(t, err)
	assert.True(t, deleted)

	fetched, err := canvases.GetCanvas(ctx, session.ID, canvas.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched)
}

func TestCanvasStore_ConcurrentCreateVsRaiseFloor_NoneBelowFloor(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Create race"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })

	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		_, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Race board", Visibility: "private"})
		errs <- err
	}()
	go func() {
		<-start
		_, err := canvases.SetSessionCanvasFloor(ctx, session.ID, teacherID, "participants")
		errs <- err
	}()
	close(start)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)

	var belowFloor int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM session_canvases c
		JOIN sessions s ON s.id = c.session_id
		WHERE c.session_id = $1 AND c.visibility < s.canvas_floor`, session.ID,
	).Scan(&belowFloor))
	assert.Zero(t, belowFloor)
}

func TestCanvasStore_ConcurrentSetVisibilityVsRaiseFloor_NoneBelowFloor(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Visibility race"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Race board", Visibility: "private"})
	require.NoError(t, err)

	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		_, err := canvases.SetCanvasVisibility(ctx, session.ID, canvas.ID, teacherID, "host")
		errs <- err
	}()
	go func() {
		<-start
		_, err := canvases.SetSessionCanvasFloor(ctx, session.ID, teacherID, "participants")
		errs <- err
	}()
	close(start)
	for range 2 {
		err := <-errs
		require.True(t,
			err == nil || errors.Is(err, ErrCanvasBelowFloor) || errors.Is(err, ErrCanvasVisibilityTighten),
			"unexpected concurrent error: %v", err,
		)
	}

	var belowFloor int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM session_canvases c
		JOIN sessions s ON s.id = c.session_id
		WHERE c.session_id = $1 AND c.visibility < s.canvas_floor`, session.ID,
	).Scan(&belowFloor))
	assert.Zero(t, belowFloor)
}

func TestCanvasStore_PerSessionCap(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Canvas cap"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })

	for i := 0; i < MaxSessionCanvases; i++ {
		_, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: fmt.Sprintf("Board %d", i), Visibility: "private"})
		require.NoError(t, err)
	}
	_, err = canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Over cap", Visibility: "private"})
	assert.ErrorIs(t, err, ErrCanvasCapReached)
}

func TestCanvasStore_EnumOrdinalOrder(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	var ordered bool
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT 'private'::canvas_visibility < 'host'::canvas_visibility
		   AND 'host'::canvas_visibility < 'participants'::canvas_visibility
		   AND 'participants'::canvas_visibility < 'session'::canvas_visibility`,
	).Scan(&ordered))
	assert.True(t, ordered)

	var missingFloor int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE canvas_floor IS NULL`).Scan(&missingFloor))
	assert.Zero(t, missingFloor, "migration must backfill existing sessions before setting NOT NULL")
}

func TestCanvases_ListEndedArchive_ByRole(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	owner := createTestUser(t, db, users, t.Name()+"-owner")
	present := createTestUser(t, db, users, t.Name()+"-present")
	left := createTestUser(t, db, users, t.Name()+"-left")
	invitee := createTestUser(t, db, users, t.Name()+"-invitee")
	outsider := createTestUser(t, db, users, t.Name()+"-outsider")

	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Canvas archive"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	_, err = sessions.JoinSession(ctx, session.ID, present.ID)
	require.NoError(t, err)
	_, err = sessions.JoinSession(ctx, session.ID, left.ID)
	require.NoError(t, err)
	_, err = sessions.LeaveSession(ctx, session.ID, left.ID)
	require.NoError(t, err)
	_, err = sessions.AddParticipant(ctx, session.ID, invitee.ID, teacherID)
	require.NoError(t, err)
	for _, input := range []CreateCanvasInput{
		{SessionID: session.ID, OwnerID: owner.ID, Title: "private", Visibility: "private"},
		{SessionID: session.ID, OwnerID: owner.ID, Title: "host", Visibility: "host"},
		{SessionID: session.ID, OwnerID: owner.ID, Title: "participants", Visibility: "participants"},
		{SessionID: session.ID, OwnerID: owner.ID, Title: "session", Visibility: "session"},
	} {
		_, err = canvases.CreateCanvas(ctx, input)
		require.NoError(t, err)
	}
	_, err = sessions.EndSession(ctx, session.ID)
	require.NoError(t, err)

	assertCanvasTitles := func(userID string, want ...string) {
		t.Helper()
		got, err := canvases.ListVisibleCanvases(ctx, session.ID, userID)
		require.NoError(t, err)
		titles := make([]string, 0, len(got))
		for _, canvas := range got {
			titles = append(titles, canvas.Title)
		}
		assert.ElementsMatch(t, want, titles)
	}
	assertCanvasTitles(owner.ID, "private", "host", "participants", "session")
	assertCanvasTitles(teacherID, "host", "participants", "session")
	assertCanvasTitles(present.ID, "participants", "session")
	assertCanvasTitles(left.ID, "participants", "session")
	assertCanvasTitles(invitee.ID)
	assertCanvasTitles(outsider.ID)
}

func TestCanvases_CrossOrgIsolation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name()+"-owner")
	_, otherTeacherID := setupSessionTest(t, db, t.Name()+"-other")
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Tenant canvas"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	_, err = canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Private board", Visibility: "private"})
	require.NoError(t, err)

	visible, err := canvases.ListVisibleCanvases(ctx, session.ID, otherTeacherID)
	require.NoError(t, err)
	assert.Empty(t, visible)
}
