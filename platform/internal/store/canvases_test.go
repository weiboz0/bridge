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

// Plan 094 R2-7: AuthorizeCanvasDocument used to re-implement the session
// access rule inline because it must run inside the lifecycle-locked
// transaction. It now calls the same evaluator CanAccessSession calls, and this
// test is what pins the two together: for every live row of the shared matrix
// the canvas authorizer's SessionAccess equals CanAccessSession's verdict for
// the same user and session — including the Plan-090 cross-org row, where a
// hand-rolled copy that reached the public clause before the class check would
// hand another organization's class session to an outsider.
//
// Ended sessions deliberately diverge: CanAccessSession denies everyone with
// "ended", while the archive keeps the host's session-level access. That
// difference is asserted here rather than left implicit.
func TestAuthorizeCanvasDocument_SessionAccessMatchesCanAccessSession(t *testing.T) {
	db := canvasTestDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)

	matrix := buildSessionAccessMatrix(t, db)
	live, endedRows := 0, 0
	for _, tc := range matrix {
		if tc.canvasID == "" {
			continue // the unknown-session row has no canvas document to authorize
		}
		if tc.live {
			live++
		} else {
			endedRows++
		}
		t.Run(tc.name, func(t *testing.T) {
			access, err := canvases.AuthorizeCanvasDocument(ctx, tc.canvasID, tc.sessionID, tc.userID)
			require.NoError(t, err)
			require.NotNil(t, access)

			if !tc.live {
				require.Equal(t, "ended", access.SessionStatus)
				assert.Equal(t, tc.teacher, access.SessionAccess,
					"after end, session-level access is the host's archive right and nobody else's")
				return
			}
			require.Equal(t, "live", access.SessionStatus)
			allowed, reason, err := sessions.CanAccessSession(ctx, tc.sessionID, tc.userID)
			require.NoError(t, err)
			require.Equal(t, tc.allowed, allowed, "matrix row disagrees with CanAccessSession (%s)", reason)
			assert.Equal(t, allowed, access.SessionAccess,
				"the canvas authorizer must reach the same verdict as the shared session-access rule")
		})
	}
	require.Greater(t, live, 0, "the matrix must contain live rows")
	require.Greater(t, endedRows, 0, "the matrix must contain ended rows")
}

// ListVisibleCanvases reads the session state and evaluates access on one
// snapshot (R2-6). A session-visible canvas is therefore offered to exactly the
// callers the shared rule allows, and to nobody else.
func TestListVisibleCanvases_SessionScopedVisibilityFollowsSharedRule(t *testing.T) {
	db := canvasTestDB(t)
	ctx := context.Background()
	canvases := NewCanvasStore(db)
	sessions := NewSessionStore(db)

	for _, tc := range buildSessionAccessMatrix(t, db) {
		if !tc.live || tc.canvasID == "" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.ExecContext(ctx, `UPDATE session_canvases SET visibility = 'session' WHERE id = $1`, tc.canvasID)
			require.NoError(t, err)
			t.Cleanup(func() {
				db.ExecContext(ctx, `UPDATE session_canvases SET visibility = 'private' WHERE id = $1`, tc.canvasID)
			})
			allowed, _, err := sessions.CanAccessSession(ctx, tc.sessionID, tc.userID)
			require.NoError(t, err)
			require.Equal(t, tc.allowed, allowed)

			visible, err := canvases.ListVisibleCanvases(ctx, tc.sessionID, tc.userID)
			require.NoError(t, err)
			found := false
			for _, canvas := range visible {
				if canvas.ID == tc.canvasID {
					found = true
				}
			}
			// The host owns every matrix canvas, so ownership alone would show
			// it to them; everyone else sees it only through the shared rule.
			if tc.teacher {
				assert.True(t, found, "the owner always sees their own canvas")
				return
			}
			assert.Equal(t, allowed, found, "a session-visible canvas follows the shared session-access rule")
		})
	}
}

// Plan 094 R2-3 at the store boundary: the canvas-ownership decision is made
// before the ended/freezing conflict, so a caller who does not own the canvas
// is told the same thing in every lifecycle state, and a canvas id that does
// not exist is "not found" rather than a lifecycle conflict.
func TestCanvasStore_MutationsDenyNonOwnerBeforeTerminalState(t *testing.T) {
	const missingCanvas = "00000000-0000-4000-8000-0000000000ff"
	for _, state := range []string{"live", "frozen", "ended"} {
		t.Run(state, func(t *testing.T) {
			db := canvasTestDB(t)
			ctx := context.Background()
			canvases := NewCanvasStore(db)
			sessions := NewSessionStore(db)
			users := NewUserStore(db)
			suffix := strings.ReplaceAll(t.Name(), "/", "-")
			_, teacherID := setupSessionTest(t, db, suffix)
			owner := createTestUser(t, db, users, suffix+"-owner")

			session, err := sessions.CreateSession(ctx, CreateSessionInput{TeacherID: teacherID, Title: "owner guard"})
			require.NoError(t, err)
			t.Cleanup(func() {
				db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1", session.ID)
				db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID)
			})
			_, err = sessions.JoinSession(ctx, session.ID, owner.ID)
			require.NoError(t, err)
			canvas, err := canvases.CreateCanvas(ctx, CreateCanvasInput{
				SessionID: session.ID, OwnerID: owner.ID, Title: "owned", Visibility: "private",
			})
			require.NoError(t, err)

			switch state {
			case "frozen":
				_, err = acquireSessionFreezeLease(ctx, db, session.ID, uuid.NewString())
				require.NoError(t, err)
			case "ended":
				_, err = sessions.EndSession(ctx, session.ID)
				require.NoError(t, err)
			}

			// The session host is not this canvas's owner.
			_, err = canvases.UpdateCanvas(ctx, session.ID, canvas.ID, teacherID, strPtr("stolen"), nil)
			assert.ErrorIs(t, err, ErrCanvasOwnerUnauthorized)
			deleted, err := canvases.DeleteCanvas(ctx, session.ID, canvas.ID, teacherID)
			assert.ErrorIs(t, err, ErrCanvasOwnerUnauthorized)
			assert.False(t, deleted)

			updated, err := canvases.UpdateCanvas(ctx, session.ID, missingCanvas, owner.ID, strPtr("ghost"), nil)
			require.NoError(t, err, "a missing canvas is absence, not a lifecycle conflict")
			assert.Nil(t, updated)
			deleted, err = canvases.DeleteCanvas(ctx, session.ID, missingCanvas, owner.ID)
			require.NoError(t, err, "a missing canvas is absence, not a lifecycle conflict")
			assert.False(t, deleted)

			var title string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT title FROM session_canvases WHERE id = $1`, canvas.ID).Scan(&title))
			assert.Equal(t, "owned", title, "no rejected mutation may change the canvas")
		})
	}
}

// --- Plan 094 R2-13: one snapshot per ListVisibleCanvases call ---

// canvasSnapshotScenario is a LIVE, class-bound session whose visible canvases
// depend on LIVE rules only: both boards are owned by the host, so neither
// caller can reach one through ownership.
//
//	member    — class member AND a 'present' participant
//	classOnly — class member only
type canvasSnapshotScenario struct {
	sessionID   string
	classID     string
	teacherID   string
	memberID    string
	classOnlyID string
	lateBoardID string
}

func setupCanvasSnapshotScenario(t *testing.T, db *sql.DB, suffix string) canvasSnapshotScenario {
	t.Helper()
	ctx := context.Background()
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classes := NewClassStore(db)
	canvases := NewCanvasStore(db)

	classID, teacherID := setupSessionTest(t, db, suffix)
	member := createTestUser(t, db, users, suffix+"-member")
	classOnly := createTestUser(t, db, users, suffix+"-classonly")
	for _, user := range []*RegisteredUser{member, classOnly} {
		_, err := classes.AddClassMember(ctx, AddClassMemberInput{ClassID: classID, UserID: user.ID, Role: "student"})
		require.NoError(t, err)
		userID := user.ID
		t.Cleanup(func() {
			db.ExecContext(ctx, `DELETE FROM class_memberships WHERE class_id = $1 AND user_id = $2`, classID, userID)
		})
	}

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID: strPtr(classID), TeacherID: teacherID, Title: "snapshot session", Visibility: "unlisted",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM session_canvases WHERE session_id = $1`, session.ID)
		db.ExecContext(ctx, `DELETE FROM session_participants WHERE session_id = $1`, session.ID)
		db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, session.ID)
	})
	_, err = sessions.JoinSession(ctx, session.ID, member.ID)
	require.NoError(t, err)

	participantsBoard, err := canvases.CreateCanvas(ctx, CreateCanvasInput{
		SessionID: session.ID, OwnerID: teacherID, Title: "participants board", Visibility: "participants",
	})
	require.NoError(t, err)
	sessionBoard, err := canvases.CreateCanvas(ctx, CreateCanvasInput{
		SessionID: session.ID, OwnerID: teacherID, Title: "session board", Visibility: "session",
	})
	require.NoError(t, err)
	require.Equal(t, "participants", participantsBoard.Visibility)
	require.Equal(t, "session", sessionBoard.Visibility)

	return canvasSnapshotScenario{
		sessionID:   session.ID,
		classID:     classID,
		teacherID:   teacherID,
		memberID:    member.ID,
		classOnlyID: classOnly.ID,
		lateBoardID: uuid.NewString(),
	}
}

// commitConcurrentEnd is the interleaved write: a SECOND connection, outside
// every list transaction, ends the session, revokes BOTH callers' access
// facts, inserts a canvas owned by `member`, and commits.
func (sc canvasSnapshotScenario) commitConcurrentEnd(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`UPDATE sessions SET status = 'ended', ended_at = clock_timestamp() WHERE id = $1`, sc.sessionID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx,
		`DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2`, sc.sessionID, sc.memberID)
	require.NoError(t, err)
	for _, userID := range []string{sc.memberID, sc.classOnlyID} {
		_, err = tx.ExecContext(ctx,
			`DELETE FROM class_memberships WHERE class_id = $1 AND user_id = $2`, sc.classID, userID)
		require.NoError(t, err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO session_canvases (id, session_id, owner_id, title, visibility, created_at, updated_at)
		 VALUES ($1, $2, $3, 'late board', 'private', clock_timestamp(), clock_timestamp())`,
		sc.lateBoardID, sc.sessionID, sc.memberID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

// listVisibleCanvasesTwoReads is the PRE-R2-6 implementation, copied from
// `b32901e~1` and changed in exactly one way: it calls `pause` at the point the
// production hook now fires, immediately after its first session read. Every
// visibility predicate is byte-for-byte the old one, so the only variable
// between this control and production is WHERE THE FACTS COME FROM — a second
// statement on the pool versus one REPEATABLE READ snapshot.
func listVisibleCanvasesTwoReads(
	ctx context.Context, db *sql.DB, sessions *SessionStore, sessionID, userID string, pause func(),
) ([]Canvas, error) {
	session, err := sessions.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return []Canvas{}, nil
	}
	pause()

	rows, err := db.QueryContext(ctx,
		`SELECT `+canvasColumns+` FROM session_canvases WHERE session_id = $1 ORDER BY created_at, id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all := []Canvas{}
	for rows.Next() {
		canvas, err := scanCanvas(rows)
		if err != nil {
			return nil, err
		}
		all = append(all, *canvas)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	participant, err := sessions.GetSessionParticipant(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	present := participant != nil && participant.Status == "present"
	formerParticipant := participant != nil && (participant.Status == "present" || participant.Status == "left")

	visible := []Canvas{}
	if session.Status == "ended" {
		for _, canvas := range all {
			if canvas.OwnerID == userID ||
				(session.TeacherID == userID && canvas.Visibility != "private") ||
				(formerParticipant && (canvas.Visibility == "participants" || canvas.Visibility == "session")) {
				visible = append(visible, canvas)
			}
		}
		return visible, nil
	}

	sessionAllowed, _, err := sessions.CanAccessSession(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	for _, canvas := range all {
		if canvas.OwnerID == userID ||
			(session.TeacherID == userID && canvas.Visibility != "private") ||
			(present && (canvas.Visibility == "participants" || canvas.Visibility == "session")) ||
			(sessionAllowed && canvas.Visibility == "session") {
			visible = append(visible, canvas)
		}
	}
	return visible, nil
}

// runCanvasSnapshotInterleaving lists for both callers concurrently, holds both
// calls at their first-session-read point, commits the concurrent end, then
// releases them and returns each caller's canvas titles.
func runCanvasSnapshotInterleaving(
	t *testing.T, db *sql.DB, sc canvasSnapshotScenario,
	build func(pause func()) func(userID string) ([]Canvas, error),
) (memberTitles, classOnlyTitles []string) {
	t.Helper()
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	releaseOnce := sync.Once{}
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAll()

	list := build(func() {
		arrived <- struct{}{}
		<-release
	})

	type listing struct {
		titles []string
		err    error
	}
	run := func(userID string, out chan<- listing) {
		visible, err := list(userID)
		titles := make([]string, 0, len(visible))
		for _, canvas := range visible {
			titles = append(titles, canvas.Title)
		}
		out <- listing{titles: titles, err: err}
	}
	memberResult := make(chan listing, 1)
	classOnlyResult := make(chan listing, 1)
	go run(sc.memberID, memberResult)
	go run(sc.classOnlyID, classOnlyResult)

	for i := 0; i < 2; i++ {
		select {
		case <-arrived:
		case <-time.After(15 * time.Second):
			releaseAll()
			t.Fatal("a list call never reached its post-session-read pause")
		}
	}

	sc.commitConcurrentEnd(t, db)
	releaseAll()

	collect := func(from chan listing, name string) []string {
		t.Helper()
		select {
		case got := <-from:
			require.NoError(t, got.err, "%s list failed", name)
			return got.titles
		case <-time.After(15 * time.Second):
			t.Fatalf("%s list never returned", name)
			return nil
		}
	}
	return collect(memberResult, "member"), collect(classOnlyResult, "classOnly")
}

// Plan 094 R2-13: ListVisibleCanvases must decide EVERYTHING from ONE snapshot.
//
// R2-13 observed that TestListVisibleCanvases_SessionScopedVisibilityFollowsSharedRule
// uses static state, so it would also pass the old two-read implementation and
// therefore does not regression-test R2-6. This test is the interleaving that
// static matrix cannot express: the session ends, BOTH callers' access facts
// vanish, and a new canvas appears — all committed by a second connection in
// the middle of the list call, between the session read and every read that
// follows it. `afterListSessionRead` is nil in production and fires exactly
// there.
//
// The list runs in a REPEATABLE READ READ ONLY transaction whose snapshot was
// taken by the session read, so every later read must still see the pre-commit
// world: session live, memberships and participant row present, and the
// concurrently inserted canvas absent.
//
// The `two reads (pre-R2-6 control)` subtest proves this test BITES by running
// the OLD implementation verbatim through the SAME interleaving. What the old
// code returns, precisely — it read the session with GetSession on the pool and
// then re-read the verdict through CanAccessSession, each its own statement,
// with the canvas and participant reads in between:
//
//	member    — GetSession sees "live" and takes the live branch; the canvas
//	            read (now after the commit) INCLUDES "late board"; the
//	            participant read finds no row, so present=false;
//	            CanAccessSession re-reads the session, sees "ended", returns
//	            (false, "ended"), so sessionAllowed=false. Visible collapses to
//	            what member owns: exactly ["late board"].
//	classOnly — same re-read, no participant row, owns nothing: exactly [].
//
// That is the inverse of the snapshot expectation on every element: the two
// boards the live snapshot implies are missing, and the canvas no snapshot
// taken before the commit can contain is present.
func TestListVisibleCanvases_UsesOneSnapshotAcrossAConcurrentEnd(t *testing.T) {
	t.Run("one snapshot (production)", func(t *testing.T) {
		db := canvasTestDB(t)
		ctx := context.Background()
		sc := setupCanvasSnapshotScenario(t, db, t.Name())

		hooked := NewCanvasStore(db)
		memberTitles, classOnlyTitles := runCanvasSnapshotInterleaving(t, db, sc,
			func(pause func()) func(string) ([]Canvas, error) {
				hooked.testHooks = &canvasStoreTestHooks{afterListSessionRead: pause}
				return func(userID string) ([]Canvas, error) {
					return hooked.ListVisibleCanvases(ctx, sc.sessionID, userID)
				}
			})

		// Every decision came from the one snapshot the session read took.
		assert.ElementsMatch(t, []string{"participants board", "session board"}, memberTitles,
			"the present-participant and class-member facts must be read in the session row's own snapshot")
		assert.NotContains(t, memberTitles, "late board",
			"a canvas inserted after the snapshot cannot be inside it; a second read would have found it")
		assert.ElementsMatch(t, []string{"session board"}, classOnlyTitles,
			"class membership is evaluated inside the same snapshot as the session row")
		assert.NotContains(t, classOnlyTitles, "late board")

		// The concurrent write really committed: a later call sees the ended world.
		var status string
		var canvasCount int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT status FROM sessions WHERE id = $1`, sc.sessionID).Scan(&status))
		assert.Equal(t, "ended", status)
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT count(*) FROM session_canvases WHERE session_id = $1`, sc.sessionID).Scan(&canvasCount))
		assert.Equal(t, 3, canvasCount, "the concurrently inserted canvas is committed")

		plain := NewCanvasStore(db)
		afterMember, err := plain.ListVisibleCanvases(ctx, sc.sessionID, sc.memberID)
		require.NoError(t, err)
		afterMemberTitles := make([]string, 0, len(afterMember))
		for _, canvas := range afterMember {
			afterMemberTitles = append(afterMemberTitles, canvas.Title)
		}
		assert.ElementsMatch(t, []string{"late board"}, afterMemberTitles,
			"a call made after the commit applies the archive rules: no participant row, so only the owned canvas")

		afterClassOnly, err := plain.ListVisibleCanvases(ctx, sc.sessionID, sc.classOnlyID)
		require.NoError(t, err)
		assert.Empty(t, afterClassOnly, "an ended session's archive gives a former class member nothing")
	})

	// The control: the same interleaving against the implementation R2-13 says
	// the old test could not distinguish. If these expectations ever coincided
	// with the production ones above, the test would have stopped biting.
	t.Run("two reads (pre-R2-6 control)", func(t *testing.T) {
		db := canvasTestDB(t)
		ctx := context.Background()
		sc := setupCanvasSnapshotScenario(t, db, t.Name())
		sessions := NewSessionStore(db)

		memberTitles, classOnlyTitles := runCanvasSnapshotInterleaving(t, db, sc,
			func(pause func()) func(string) ([]Canvas, error) {
				return func(userID string) ([]Canvas, error) {
					return listVisibleCanvasesTwoReads(ctx, db, sessions, sc.sessionID, userID, pause)
				}
			})

		assert.ElementsMatch(t, []string{"late board"}, memberTitles,
			"two reads: the live branch is taken from the first read while the verdict comes from the second")
		assert.ElementsMatch(t, []string{}, classOnlyTitles,
			"two reads: the re-read says 'ended', so the class member loses the session board")

		// And therefore it disagrees with the snapshot answer on every element.
		assert.NotContains(t, memberTitles, "participants board")
		assert.NotContains(t, memberTitles, "session board")
		assert.NotContains(t, classOnlyTitles, "session board")
	})
}
