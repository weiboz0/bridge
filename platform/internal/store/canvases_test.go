package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const defaultCanvasTestDatabaseURL = "postgresql://work@127.0.0.1:5432/bridge_test"

func canvasTestDatabaseURL(candidate string) (string, error) {
	if candidate == "" {
		candidate = defaultCanvasTestDatabaseURL
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", fmt.Errorf("parse canvas test database URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", fmt.Errorf("canvas test database URL must use postgres: %q", parsed.Scheme)
	}
	database, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil {
		return "", fmt.Errorf("decode canvas test database name: %w", err)
	}
	if database == "" || (!strings.HasSuffix(database, "_test") && database != "bridge_test") {
		return "", fmt.Errorf("refusing non-test canvas database %q", database)
	}
	return candidate, nil
}

func canvasTestDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL, err := canvasTestDatabaseURL(os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)
	db, err := sql.Open("pgx", databaseURL)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(context.Background()))
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestCanvasTestDatabaseURLRejectsUnsafeDatabase(t *testing.T) {
	_, err := canvasTestDatabaseURL("postgresql://work@127.0.0.1:5432/bridge")
	assert.Error(t, err)

	url, err := canvasTestDatabaseURL("postgresql://work@127.0.0.1:5432/bridge_test")
	require.NoError(t, err)
	assert.Equal(t, "postgresql://work@127.0.0.1:5432/bridge_test", url)

	url, err = canvasTestDatabaseURL("")
	require.NoError(t, err)
	assert.Equal(t, defaultCanvasTestDatabaseURL, url)
}

func TestCanvasStore_DefaultFloor(t *testing.T) {
	db := canvasTestDB(t)
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
	db := canvasTestDB(t)
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	// A connection-local pg_temp schema is unique to this dedicated connection,
	// requires no database-level CREATE privilege, and is dropped on Close.
	_, err = conn.ExecContext(ctx, `CREATE TEMP TABLE users (id uuid PRIMARY KEY)`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `
		CREATE TEMP TABLE sessions (
			id uuid PRIMARY KEY,
			teacher_id uuid NOT NULL REFERENCES users(id)
		)`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `SET search_path TO pg_temp`)
	require.NoError(t, err)
	userID := uuid.NewString()
	sessionID := uuid.NewString()
	_, err = conn.ExecContext(ctx, `INSERT INTO users (id) VALUES ($1)`, userID)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO sessions (id, teacher_id) VALUES ($1, $2)`, sessionID, userID)
	require.NoError(t, err)

	migration, err := os.ReadFile("../../../drizzle/0028_session_canvases.sql")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, string(migration))
	require.NoError(t, err)

	var floor, defaultValue, enumOrder string
	var nullable bool
	require.NoError(t, conn.QueryRowContext(ctx,
		`SELECT canvas_floor FROM sessions WHERE id = $1`, sessionID,
	).Scan(&floor))
	require.NoError(t, conn.QueryRowContext(ctx, `
		SELECT column_default, is_nullable = 'YES'
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'sessions' AND column_name = 'canvas_floor'`,
	).Scan(&defaultValue, &nullable))
	require.NoError(t, conn.QueryRowContext(ctx, `
		SELECT string_agg(e.enumlabel, ',' ORDER BY e.enumsortorder)
		FROM pg_enum e
		JOIN pg_type t ON t.oid = e.enumtypid
		WHERE t.typname = 'canvas_visibility' AND t.typnamespace = current_schema()::regnamespace`,
	).Scan(&enumOrder))
	assert.Equal(t, "private", floor)
	assert.Contains(t, defaultValue, "private")
	assert.False(t, nullable)
	assert.Equal(t, "private,host,participants,session", enumOrder)
}

func TestCanvasStore_CreateGreatestWithSessionFloor(t *testing.T) {
	db := canvasTestDB(t)
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
	db := canvasTestDB(t)
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
	db := canvasTestDB(t)
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
	db := canvasTestDB(t)
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

func TestCanvasStore_LowerFloorLeavesExistingCanvasesUnchanged(t *testing.T) {
	db := canvasTestDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Canvas floor lower"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Board", Visibility: "private"})
	require.NoError(t, err)

	_, err = canvases.SetSessionCanvasFloor(ctx, session.ID, teacherID, "participants")
	require.NoError(t, err)
	floor, err := canvases.SetSessionCanvasFloor(ctx, session.ID, teacherID, "private")
	require.NoError(t, err)
	assert.Equal(t, "private", floor)

	updated, err := canvases.GetCanvas(ctx, session.ID, canvas.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "participants", updated.Visibility)
}

func TestCanvasStore_ListVisible_ByRole(t *testing.T) {
	db := canvasTestDB(t)
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
	db := canvasTestDB(t)
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
	db := canvasTestDB(t)
	ctx := context.Background()
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name()+"-"+uuid.NewString())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Create race"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })

	createStore := NewCanvasStore(db)
	floorStore := NewCanvasStore(db)
	createRead := make(chan struct{})
	releaseCreate := make(chan struct{})
	floorRead := make(chan struct{})
	createStore.testHooks = &canvasStoreTestHooks{afterSessionLock: func(operation canvasStoreOperation) {
		if operation == canvasStoreOperationCreate {
			close(createRead)
			<-releaseCreate
		}
	}}
	floorStore.testHooks = &canvasStoreTestHooks{afterSessionLock: func(operation canvasStoreOperation) {
		if operation == canvasStoreOperationSetFloor {
			close(floorRead)
		}
	}}

	errs := make(chan error, 2)
	go func() {
		_, err := createStore.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Race board", Visibility: "private"})
		errs <- err
	}()
	<-createRead
	go func() {
		_, err := floorStore.SetSessionCanvasFloor(ctx, session.ID, teacherID, "participants")
		errs <- err
	}()
	// The create transaction has already read its floor. Releasing it now lets
	// the floor transaction obtain the same lock only after create commits.
	// Without the session-row lock, floor could commit before this stale create.
	close(releaseCreate)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	<-floorRead

	var belowFloor int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM session_canvases c
		JOIN sessions s ON s.id = c.session_id
		WHERE c.session_id = $1 AND c.visibility < s.canvas_floor`, session.ID,
	).Scan(&belowFloor))
	assert.Zero(t, belowFloor)
}

func TestCanvasStore_ConcurrentSetVisibilityVsRaiseFloor_NoneBelowFloor(t *testing.T) {
	db := canvasTestDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name()+"-"+uuid.NewString())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Visibility race"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Race board", Visibility: "private"})
	require.NoError(t, err)

	visibilityStore := NewCanvasStore(db)
	floorStore := NewCanvasStore(db)
	visibilityRead := make(chan struct{})
	releaseVisibility := make(chan struct{})
	floorRead := make(chan struct{})
	visibilityStore.testHooks = &canvasStoreTestHooks{afterSessionLock: func(operation canvasStoreOperation) {
		if operation == canvasStoreOperationSetVisibility {
			close(visibilityRead)
			<-releaseVisibility
		}
	}}
	floorStore.testHooks = &canvasStoreTestHooks{afterSessionLock: func(operation canvasStoreOperation) {
		if operation == canvasStoreOperationSetFloor {
			close(floorRead)
		}
	}}

	errs := make(chan error, 2)
	go func() {
		_, err := visibilityStore.SetCanvasVisibility(ctx, session.ID, canvas.ID, teacherID, "host")
		errs <- err
	}()
	<-visibilityRead
	go func() {
		_, err := floorStore.SetSessionCanvasFloor(ctx, session.ID, teacherID, "participants")
		errs <- err
	}()
	close(releaseVisibility)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	<-floorRead

	var belowFloor int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM session_canvases c
		JOIN sessions s ON s.id = c.session_id
		WHERE c.session_id = $1 AND c.visibility < s.canvas_floor`, session.ID,
	).Scan(&belowFloor))
	assert.Zero(t, belowFloor)
}

func TestCanvasStore_PerSessionCap(t *testing.T) {
	db := canvasTestDB(t)
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
	db := canvasTestDB(t)
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
	db := canvasTestDB(t)
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
	db := canvasTestDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name()+"-owner")
	_, otherTeacherID := setupSessionTest(t, db, t.Name()+"-other")
	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID:    strPtr(classID),
		TeacherID:  teacherID,
		Title:      "Tenant canvas",
		Visibility: "public",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	_, err = canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Session board", Visibility: "session"})
	require.NoError(t, err)

	visible, err := canvases.ListVisibleCanvases(ctx, session.ID, otherTeacherID)
	require.NoError(t, err)
	assert.Empty(t, visible)
}
