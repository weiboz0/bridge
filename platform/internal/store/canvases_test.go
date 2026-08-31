package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultCanvasTestDatabaseURL = "postgresql://work@127.0.0.1:5432/bridge_test"
	canvasTestDBTimeout          = 5 * time.Second
)

func canvasTestDatabaseURL(candidate string) (string, error) {
	if candidate == "" {
		candidate = defaultCanvasTestDatabaseURL
	}
	config, err := pgx.ParseConfig(candidate)
	if err != nil {
		return "", fmt.Errorf("parse canvas test database URL: %w", err)
	}
	if err := validateCanvasTestDatabaseName(config.Database); err != nil {
		return "", err
	}
	return candidate, nil
}

func validateCanvasTestDatabaseName(database string) error {
	if database == "" || (!strings.HasSuffix(database, "_test") && database != "bridge_test") {
		return fmt.Errorf("refusing non-test canvas database %q", database)
	}
	return nil
}

func canvasTestDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL, err := canvasTestDatabaseURL(os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)
	db, err := sql.Open("pgx", databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), canvasTestDBTimeout)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
	var connectedDatabase string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&connectedDatabase))
	require.NoError(t, validateCanvasTestDatabaseName(connectedDatabase))
	return db
}

func waitForCanvasSessionLock(ctx context.Context, observer *sql.DB, waiterPID, holderPID int) error {
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		var blocked bool
		err := observer.QueryRowContext(ctx,
			`SELECT $2 = ANY(pg_blocking_pids($1))`, waiterPID, holderPID,
		).Scan(&blocked)
		if err != nil {
			return err
		}
		if blocked {
			return nil
		}
		select {
		case <-timeout.C:
			return fmt.Errorf("backend %d did not report blocking on session-lock holder %d", waiterPID, holderPID)
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func receiveCanvasPID(ctx context.Context, pid <-chan int, name string) (int, error) {
	select {
	case value := <-pid:
		return value, nil
	case <-ctx.Done():
		return 0, fmt.Errorf("wait for %s backend PID: %w", name, ctx.Err())
	}
}

func receiveCanvasOperation(ctx context.Context, result <-chan error, name string) error {
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return fmt.Errorf("wait for %s operation: %w", name, ctx.Err())
	}
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

func TestCanvasTestDatabaseURLRejectsDatabaseOverrides(t *testing.T) {
	for _, candidate := range []string{
		"postgresql://work@127.0.0.1:5432/bridge_test?dbname=bridge",
		"postgresql://work@127.0.0.1:5432/bridge_test?database=bridge",
	} {
		t.Run(candidate, func(t *testing.T) {
			_, err := canvasTestDatabaseURL(candidate)
			assert.Error(t, err)
		})
	}
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

func TestCanvasStore_CreateValidatesTitleCharacterLimit(t *testing.T) {
	db := canvasTestDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Canvas title limit"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })

	_, err = canvases.CreateCanvas(ctx, CreateCanvasInput{
		SessionID: session.ID, OwnerID: teacherID, Title: strings.Repeat("x", 256), Visibility: "private",
	})
	assert.ErrorIs(t, err, ErrCanvasTitleTooLong)

	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{
		SessionID: session.ID, OwnerID: teacherID, Title: strings.Repeat("é", 255), Visibility: "private",
	})
	require.NoError(t, err)
	assert.Equal(t, 255, utf8.RuneCountInString(canvas.Title))
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

	var floor, defaultValue, enumOrder, canvasIDDefault string
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
	require.NoError(t, conn.QueryRowContext(ctx, `
		SELECT column_default
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'session_canvases' AND column_name = 'id'`,
	).Scan(&canvasIDDefault))
	var generatedCanvasID string
	require.NoError(t, conn.QueryRowContext(ctx, `
		INSERT INTO session_canvases (session_id, owner_id, title, visibility)
		VALUES ($1, $2, 'migration default', 'private')
		RETURNING id`, sessionID, userID,
	).Scan(&generatedCanvasID))
	assert.Equal(t, "private", floor)
	assert.Contains(t, defaultValue, "private")
	assert.False(t, nullable)
	assert.Equal(t, "private,host,participants,session", enumOrder)
	assert.Contains(t, canvasIDDefault, "gen_random_uuid")
	assert.NotEmpty(t, generatedCanvasID)
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

func TestCanvasMutationsRejectUnexpiredSessionFreezeLease(t *testing.T) {
	db := canvasTestDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "frozen canvas"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, session.ID) })
	_, err = acquireSessionFreezeLease(ctx, db, session.ID, uuid.NewString())
	require.NoError(t, err)
	_, err = canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "blocked", Visibility: "private"})
	assert.ErrorIs(t, err, ErrSessionEndInProgress)
}

func TestCanvasAuthorization_TakesSharedLifecycleLockBeforeEveryAuthorizationRead(t *testing.T) {
	// This precise canvas/session API prevents the old canvas-ID discovery query
	// from reading before the lock key is known.  It is a deliberate RED
	// signature assertion as well as a real blocking proof once implemented.
	type sessionBoundAuthorizer interface {
		AuthorizeCanvasDocument(context.Context, string, string, string) (*CanvasDocumentAccess, error)
	}
	db := canvasTestDB(t)
	observer := canvasTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "locked canvas authorization"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM sessions WHERE id = $1", session.ID) })
	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Board", Visibility: "private"})
	require.NoError(t, err)

	authorizer, ok := any(canvases).(sessionBoundAuthorizer)
	require.True(t, ok, "canvas authorization must require the supplied session ID before it can issue any authorization read")

	holder, err := observer.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Rollback() })
	require.NoError(t, lockSessionLifecycle(ctx, holder, session.ID, false))
	key, err := sessionLifecycleAdvisoryKey(session.ID)
	require.NoError(t, err)

	completed := make(chan error, 1)
	go func() {
		_, err := authorizer.AuthorizeCanvasDocument(ctx, canvas.ID, session.ID, teacherID)
		completed <- err
	}()

	var waiterPID int
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for waiterPID == 0 {
		err = observer.QueryRowContext(ctx, `SELECT pid FROM pg_locks
			WHERE locktype = 'advisory' AND classid = $1::oid AND objid = $2::oid AND NOT granted
			LIMIT 1`, sessionLifecycleLockClass, int64(uint32(key))).Scan(&waiterPID)
		if err == nil {
			break
		}
		if err != sql.ErrNoRows {
			require.NoError(t, err)
		}
		select {
		case <-deadline.C:
			t.Fatal("canvas authorization did not block on the shared lifecycle lock")
		case <-time.After(10 * time.Millisecond):
		}
	}

	var authorizationReads int
	require.NoError(t, observer.QueryRowContext(ctx, `SELECT count(*)
		FROM pg_locks locks JOIN pg_class relation ON relation.oid = locks.relation
		WHERE locks.pid = $1 AND locks.granted
		  AND relation.relname = ANY(ARRAY['session_canvases', 'sessions', 'users', 'session_participants', 'classes', 'class_memberships'])`, waiterPID).Scan(&authorizationReads))
	assert.Zero(t, authorizationReads, "no canvas, session, user, participant, class, or membership read may complete before lifecycle-lock release")
	select {
	case err := <-completed:
		t.Fatalf("canvas authorization bypassed the exclusive lifecycle lock: %v", err)
	default:
	}
	require.NoError(t, holder.Commit())
	select {
	case err := <-completed:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("canvas authorization did not complete after lifecycle-lock release")
	}
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
	_, err = sessions.JoinSession(ctx, session.ID, owner.ID)
	require.NoError(t, err)
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
	observer := canvasTestDB(t)
	operationCtx, cancelOperations := context.WithTimeout(ctx, canvasTestDBTimeout)
	t.Cleanup(cancelOperations)
	holderPID := make(chan int)
	createRead := make(chan struct{})
	releaseCreate := make(chan struct{})
	var releaseCreateOnce sync.Once
	releaseCreateLock := func() { releaseCreateOnce.Do(func() { close(releaseCreate) }) }
	t.Cleanup(releaseCreateLock)
	floorPID := make(chan int)
	createStore.testHooks = &canvasStoreTestHooks{
		beforeSessionLock: func(operation canvasStoreOperation, pid int) {
			if operation == canvasStoreOperationCreate {
				select {
				case holderPID <- pid:
				case <-operationCtx.Done():
				}
			}
		},
		afterSessionLock: func(operation canvasStoreOperation, _ int) {
			if operation == canvasStoreOperationCreate {
				close(createRead)
				select {
				case <-releaseCreate:
				case <-operationCtx.Done():
				}
			}
		},
	}
	floorStore.testHooks = &canvasStoreTestHooks{beforeSessionLock: func(operation canvasStoreOperation, pid int) {
		if operation == canvasStoreOperationSetFloor {
			select {
			case floorPID <- pid:
			case <-operationCtx.Done():
			}
		}
	}}

	errs := make(chan error, 2)
	go func() {
		_, err := createStore.CreateCanvas(operationCtx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Race board", Visibility: "private"})
		errs <- err
	}()
	holder, receiveErr := receiveCanvasPID(operationCtx, holderPID, "create")
	require.NoError(t, receiveErr)
	select {
	case <-createRead:
	case <-operationCtx.Done():
		require.NoError(t, operationCtx.Err())
	}
	go func() {
		_, err := floorStore.SetSessionCanvasFloor(operationCtx, session.ID, teacherID, "participants")
		errs <- err
	}()
	waiter, receiveErr := receiveCanvasPID(operationCtx, floorPID, "floor")
	require.NoError(t, receiveErr)
	lockErr := waitForCanvasSessionLock(operationCtx, observer, waiter, holder)
	// Release only after PostgreSQL confirms the floor transaction is blocked
	// behind create's stale-floor read. Removing create's FOR UPDATE times out.
	releaseCreateLock()
	require.NoError(t, lockErr)
	require.NoError(t, receiveCanvasOperation(operationCtx, errs, "create"))
	require.NoError(t, receiveCanvasOperation(operationCtx, errs, "floor"))

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
	observer := canvasTestDB(t)
	operationCtx, cancelOperations := context.WithTimeout(ctx, canvasTestDBTimeout)
	t.Cleanup(cancelOperations)
	holderPID := make(chan int)
	visibilityRead := make(chan struct{})
	releaseVisibility := make(chan struct{})
	var releaseVisibilityOnce sync.Once
	releaseVisibilityLock := func() { releaseVisibilityOnce.Do(func() { close(releaseVisibility) }) }
	t.Cleanup(releaseVisibilityLock)
	floorPID := make(chan int)
	visibilityStore.testHooks = &canvasStoreTestHooks{
		beforeSessionLock: func(operation canvasStoreOperation, pid int) {
			if operation == canvasStoreOperationSetVisibility {
				select {
				case holderPID <- pid:
				case <-operationCtx.Done():
				}
			}
		},
		afterSessionLock: func(operation canvasStoreOperation, _ int) {
			if operation == canvasStoreOperationSetVisibility {
				close(visibilityRead)
				select {
				case <-releaseVisibility:
				case <-operationCtx.Done():
				}
			}
		},
	}
	floorStore.testHooks = &canvasStoreTestHooks{beforeSessionLock: func(operation canvasStoreOperation, pid int) {
		if operation == canvasStoreOperationSetFloor {
			select {
			case floorPID <- pid:
			case <-operationCtx.Done():
			}
		}
	}}

	errs := make(chan error, 2)
	go func() {
		_, err := visibilityStore.SetCanvasVisibility(operationCtx, session.ID, canvas.ID, teacherID, "host")
		errs <- err
	}()
	holder, receiveErr := receiveCanvasPID(operationCtx, holderPID, "visibility")
	require.NoError(t, receiveErr)
	select {
	case <-visibilityRead:
	case <-operationCtx.Done():
		require.NoError(t, operationCtx.Err())
	}
	go func() {
		_, err := floorStore.SetSessionCanvasFloor(operationCtx, session.ID, teacherID, "participants")
		errs <- err
	}()
	waiter, receiveErr := receiveCanvasPID(operationCtx, floorPID, "floor")
	require.NoError(t, receiveErr)
	lockErr := waitForCanvasSessionLock(operationCtx, observer, waiter, holder)
	// Removing SetCanvasVisibility's FOR UPDATE leaves no observed blocker and
	// fails before the paused stale visibility read can be released.
	releaseVisibilityLock()
	require.NoError(t, lockErr)
	require.NoError(t, receiveCanvasOperation(operationCtx, errs, "visibility"))
	require.NoError(t, receiveCanvasOperation(operationCtx, errs, "floor"))

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
	_, err = sessions.JoinSession(ctx, session.ID, owner.ID)
	require.NoError(t, err)
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

func TestCanvasStore_MutationsRejectEndedSession(t *testing.T) {
	db := canvasTestDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)
	_, teacherID := setupSessionTest(t, db, t.Name())
	session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "Archive"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID) })
	canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Board", Visibility: "private"})
	require.NoError(t, err)
	_, err = sessions.EndSession(ctx, session.ID)
	require.NoError(t, err)

	_, err = canvases.CreateCanvas(ctx, CreateCanvasInput{SessionID: session.ID, OwnerID: teacherID, Title: "Late board", Visibility: "private"})
	assert.ErrorIs(t, err, ErrSessionEnded)
	_, err = canvases.UpdateCanvas(ctx, session.ID, canvas.ID, teacherID, strPtr("Late title"), nil)
	assert.ErrorIs(t, err, ErrSessionEnded)
	_, err = canvases.SetCanvasVisibility(ctx, session.ID, canvas.ID, teacherID, "host")
	assert.ErrorIs(t, err, ErrSessionEnded)
	_, err = canvases.SetSessionCanvasFloor(ctx, session.ID, teacherID, "host")
	assert.ErrorIs(t, err, ErrSessionEnded)
	_, err = canvases.DeleteCanvas(ctx, session.ID, canvas.ID, teacherID)
	assert.ErrorIs(t, err, ErrSessionEnded)
}
