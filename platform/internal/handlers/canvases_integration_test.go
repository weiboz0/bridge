package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/store"
)

const canvasHandlerTestDatabaseURL = "postgresql://work@127.0.0.1:5432/bridge_test"
const canvasHandlerDBTimeout = 5 * time.Second

type canvasHandlerFixture struct {
	db       *sql.DB
	h        *CanvasHandler
	router   chi.Router
	teacher  *store.RegisteredUser
	student  *store.RegisteredUser
	outsider *store.RegisteredUser
	session  *store.LiveSession
}

func newCanvasHandlerFixture(t *testing.T) *canvasHandlerFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), canvasHandlerDBTimeout)
	defer cancel()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = canvasHandlerTestDatabaseURL
	}
	cfg, err := pgx.ParseConfig(url)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(cfg.Database, "_test"), "refusing non-test database %q", cfg.Database)
	db, err := sql.Open("pgx", url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.PingContext(ctx))
	var actual string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT current_database()").Scan(&actual))
	require.True(t, strings.HasSuffix(actual, "_test"), "connected to non-test database %q", actual)

	mkUser := func(label string) *store.RegisteredUser {
		u := insertFixtureUser(t, db, store.RegisterInput{Name: label, Email: fmt.Sprintf("%s-%s@example.com", t.Name(), label), Password: "testpassword123"})
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), canvasHandlerDBTimeout)
			defer cleanupCancel()
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM session_participants WHERE user_id = $1", u.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}
	teacher, student, outsider := mkUser("teacher"), mkUser("student"), mkUser("outsider")
	sessions := store.NewSessionStore(db)
	session, err := sessions.CreateSession(ctx, store.CreateSessionInput{TeacherID: teacher.ID, Title: "Canvas session"})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), canvasHandlerDBTimeout)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM sessions WHERE id = $1", session.ID)
	})
	_, err = sessions.JoinSession(ctx, session.ID, student.ID)
	require.NoError(t, err)

	h := &CanvasHandler{Sessions: sessions, Canvases: store.NewCanvasStore(db)}
	r := chi.NewRouter()
	h.Routes(r)
	return &canvasHandlerFixture{db: db, h: h, router: r, teacher: teacher, student: student, outsider: outsider, session: session}
}

func (fx *canvasHandlerFixture) claims(user *store.RegisteredUser) *auth.Claims {
	return &auth.Claims{UserID: user.ID, Email: user.Email, Name: user.Name}
}

func newRealtimeHandlerForCanvasFixture(fx *canvasHandlerFixture) *RealtimeHandler {
	return &RealtimeHandler{
		Sessions:              fx.h.Sessions,
		Canvases:              fx.h.Canvases,
		Users:                 store.NewUserStore(fx.db),
		HocuspocusTokenSecret: rtSecret,
	}
}

func (fx *canvasHandlerFixture) addUser(t *testing.T, label string) *store.RegisteredUser {
	t.Helper()
	u := insertFixtureUser(t, fx.db, store.RegisterInput{Name: label, Email: fmt.Sprintf("%s-%s@example.com", t.Name(), label), Password: "testpassword123"})
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), canvasHandlerDBTimeout)
		defer cleanupCancel()
		_, _ = fx.db.ExecContext(cleanupCtx, "DELETE FROM session_participants WHERE user_id = $1", u.ID)
		_, _ = fx.db.ExecContext(cleanupCtx, "DELETE FROM users WHERE id = $1", u.ID)
	})
	return u
}

func (fx *canvasHandlerFixture) request(t *testing.T, method, path string, body any, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(encoded))
		req.Header.Set("Content-Type", "application/json")
	}
	req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)
	return w
}

func TestCanvasHandler_CreateCanvas_MemberOwnsCanvas(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	w := fx.request(t, http.MethodPost, "/api/sessions/"+fx.session.ID+"/canvases", map[string]string{"title": "Student board", "visibility": "private"}, fx.claims(fx.student))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var canvas store.Canvas
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &canvas))
	require.Equal(t, fx.student.ID, canvas.OwnerID)
	require.Equal(t, "private", canvas.Visibility)
}

func TestCanvasRoutes_ComposeWithSessionRoutes(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	r := chi.NewRouter()
	(&SessionHandler{Sessions: fx.h.Sessions, Broadcaster: events.NewBroadcaster()}).Routes(r)
	fx.h.Routes(r)
	request := func(method, path string, body any) *httptest.ResponseRecorder {
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			reader = bytes.NewReader(encoded)
		}
		req := httptest.NewRequest(method, path, reader).WithContext(auth.ContextWithClaims(context.Background(), fx.claims(fx.teacher)))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, http.StatusOK, request(http.MethodGet, "/api/sessions/public", nil).Code)
	canvas := request(http.MethodPost, "/api/sessions/"+fx.session.ID+"/canvases", map[string]string{"title": "Board", "visibility": "private"})
	require.Equal(t, http.StatusCreated, canvas.Code, canvas.Body.String())
	end := request(http.MethodPost, "/api/sessions/"+fx.session.ID+"/end", nil)
	require.Equal(t, http.StatusOK, end.Code, end.Body.String())
}

func TestCanvasHandler_MutationAuthAndEndedArchive(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	created := fx.request(t, http.MethodPost, "/api/sessions/"+fx.session.ID+"/canvases", map[string]string{"title": "Student board", "visibility": "private"}, fx.claims(fx.student))
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var canvas store.Canvas
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &canvas))

	for _, tc := range []struct {
		name, method, path string
		body               any
		claims             *auth.Claims
		want               int
	}{
		{"outsider create", http.MethodPost, "/api/sessions/" + fx.session.ID + "/canvases", map[string]string{"title": "No", "visibility": "private"}, fx.claims(fx.outsider), http.StatusForbidden},
		{"non-owner patch", http.MethodPatch, "/api/sessions/" + fx.session.ID + "/canvases/" + canvas.ID, map[string]string{"title": "No"}, fx.claims(fx.teacher), http.StatusForbidden},
		{"owner title", http.MethodPatch, "/api/sessions/" + fx.session.ID + "/canvases/" + canvas.ID, map[string]string{"title": "Renamed"}, fx.claims(fx.student), http.StatusOK},
		{"equal visibility rejected", http.MethodPatch, "/api/sessions/" + fx.session.ID + "/canvases/" + canvas.ID, map[string]string{"visibility": "private"}, fx.claims(fx.student), http.StatusBadRequest},
		{"host floor", http.MethodPatch, "/api/sessions/" + fx.session.ID + "/settings", map[string]string{"canvasFloor": "host"}, fx.claims(fx.teacher), http.StatusOK},
		{"non-host floor", http.MethodPatch, "/api/sessions/" + fx.session.ID + "/settings", map[string]string{"canvasFloor": "participants"}, fx.claims(fx.student), http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := fx.request(t, tc.method, tc.path, tc.body, tc.claims)
			require.Equal(t, tc.want, w.Code, w.Body.String())
		})
	}

	_, err := fx.h.Sessions.EndSession(context.Background(), fx.session.ID)
	require.NoError(t, err)
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/sessions/" + fx.session.ID + "/canvases", map[string]string{"title": "Late", "visibility": "host"}},
		{http.MethodPatch, "/api/sessions/" + fx.session.ID + "/canvases/" + canvas.ID, map[string]string{"title": "Late"}},
		{http.MethodDelete, "/api/sessions/" + fx.session.ID + "/canvases/" + canvas.ID, nil},
		{http.MethodPatch, "/api/sessions/" + fx.session.ID + "/settings", map[string]string{"canvasFloor": "participants"}},
	} {
		w := fx.request(t, tc.method, tc.path, tc.body, fx.claims(fx.student))
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	}
}

func TestCanvasHandler_ActiveFreezeReturnsStable409WithoutWrites(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	canvas, err := fx.h.Canvases.CreateCanvas(context.Background(), store.CreateCanvasInput{SessionID: fx.session.ID, OwnerID: fx.student.ID, Title: "unchanged", Visibility: "private"})
	require.NoError(t, err)
	_, err = fx.db.ExecContext(context.Background(), `UPDATE sessions SET canvas_freeze_token = '11111111-1111-1111-1111-111111111111', canvas_freeze_until = clock_timestamp() + interval '15 seconds' WHERE id = $1`, fx.session.ID)
	require.NoError(t, err)
	for _, tc := range []struct {
		method, path string
		body         any
		claims       *auth.Claims
	}{
		{http.MethodPost, "/api/sessions/" + fx.session.ID + "/canvases", map[string]string{"title": "blocked", "visibility": "private"}, fx.claims(fx.student)},
		{http.MethodPatch, "/api/sessions/" + fx.session.ID + "/canvases/" + canvas.ID, map[string]string{"title": "changed"}, fx.claims(fx.student)},
		{http.MethodDelete, "/api/sessions/" + fx.session.ID + "/canvases/" + canvas.ID, nil, fx.claims(fx.student)},
		{http.MethodPatch, "/api/sessions/" + fx.session.ID + "/settings", map[string]string{"canvasFloor": "host"}, fx.claims(fx.teacher)},
	} {
		w := fx.request(t, tc.method, tc.path, tc.body, tc.claims)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		var body map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, "session_end_in_progress", body["code"])
	}
	var title, floor string
	var count int
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT count(*) FROM session_canvases WHERE session_id = $1`, fx.session.ID).Scan(&count))
	assert.Equal(t, 1, count, "blocked create must not insert a canvas row")
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT title FROM session_canvases WHERE id = $1`, canvas.ID).Scan(&title))
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT canvas_floor FROM sessions WHERE id = $1`, fx.session.ID).Scan(&floor))
	assert.Equal(t, "unchanged", title)
	assert.Equal(t, "private", floor)
}

func TestCanvasHandler_DeleteOwnerOnly(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	canvas, err := fx.h.Canvases.CreateCanvas(context.Background(), store.CreateCanvasInput{SessionID: fx.session.ID, OwnerID: fx.student.ID, Title: "Delete me", Visibility: "private"})
	require.NoError(t, err)
	path := "/api/sessions/" + fx.session.ID + "/canvases/" + canvas.ID
	require.Equal(t, http.StatusForbidden, fx.request(t, http.MethodDelete, path, nil, fx.claims(fx.teacher)).Code)
	require.Equal(t, http.StatusNoContent, fx.request(t, http.MethodDelete, path, nil, fx.claims(fx.student)).Code)
	require.Equal(t, http.StatusNotFound, fx.request(t, http.MethodDelete, path, nil, fx.claims(fx.student)).Code)
}

func TestCanvasHandler_ValidationErrorsAreBadRequest(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	for _, body := range []map[string]string{{"title": "", "visibility": "private"}, {"title": " \t", "visibility": "private"}, {"title": "Board", "visibility": "unknown"}} {
		w := fx.request(t, http.MethodPost, "/api/sessions/"+fx.session.ID+"/canvases", body, fx.claims(fx.student))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
	canvas, err := fx.h.Canvases.CreateCanvas(context.Background(), store.CreateCanvasInput{SessionID: fx.session.ID, OwnerID: fx.student.ID, Title: "Board", Visibility: "private"})
	require.NoError(t, err)
	w := fx.request(t, http.MethodPatch, "/api/sessions/"+fx.session.ID+"/canvases/"+canvas.ID, map[string]string{"visibility": "unknown"}, fx.claims(fx.student))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	w = fx.request(t, http.MethodPatch, "/api/sessions/"+fx.session.ID+"/canvases/"+canvas.ID, map[string]string{"title": " \t"}, fx.claims(fx.student))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	w = fx.request(t, http.MethodPatch, "/api/sessions/"+fx.session.ID+"/canvases/"+canvas.ID, map[string]string{"title": strings.Repeat("x", store.MaxCanvasTitleRunes+1)}, fx.claims(fx.student))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	w = fx.request(t, http.MethodPatch, "/api/sessions/"+fx.session.ID+"/settings", map[string]string{"canvasFloor": "session"}, fx.claims(fx.teacher))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCanvases_ListEndedArchive_ByRole(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	present, left, invitee := fx.addUser(t, "present"), fx.addUser(t, "left"), fx.addUser(t, "invitee")
	_, err := fx.h.Sessions.JoinSession(context.Background(), fx.session.ID, present.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.JoinSession(context.Background(), fx.session.ID, left.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.LeaveSession(context.Background(), fx.session.ID, left.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.AddParticipant(context.Background(), fx.session.ID, invitee.ID, fx.teacher.ID)
	require.NoError(t, err)
	for _, visibility := range []string{"private", "host", "participants", "session"} {
		_, err = fx.h.Canvases.CreateCanvas(context.Background(), store.CreateCanvasInput{SessionID: fx.session.ID, OwnerID: fx.student.ID, Title: visibility, Visibility: visibility})
		require.NoError(t, err)
	}
	_, err = fx.h.Sessions.EndSession(context.Background(), fx.session.ID)
	require.NoError(t, err)
	list := func(user *store.RegisteredUser) []store.Canvas {
		w := fx.request(t, http.MethodGet, "/api/sessions/"+fx.session.ID+"/canvases", nil, fx.claims(user))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var payload struct {
			Items []store.Canvas `json:"items"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
		return payload.Items
	}
	require.Len(t, list(fx.student), 4)
	require.Len(t, list(fx.teacher), 3)
	require.Len(t, list(present), 2)
	require.Len(t, list(left), 2)
	require.Empty(t, list(invitee))
	require.Empty(t, list(fx.outsider))
}

// Phase 9 deliberately cuts the floor control away from generic session
// settings.  A compatibility alias would let an old client bypass the exact
// teacher-only schema and makes a partial producer/consumer deploy unsafe.
func TestCanvasSettings_AtomicRouteCutoverRejectsLegacySettingsRoute(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	legacy := fx.request(t, http.MethodPatch, "/api/sessions/"+fx.session.ID+"/settings", map[string]string{"canvasFloor": "host"}, fx.claims(fx.teacher))
	require.Equal(t, http.StatusNotFound, legacy.Code, legacy.Body.String())
}

func TestCanvasSettings_GetAndPatchUseExactTeacherOnlySchemas(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	path := "/api/sessions/" + fx.session.ID + "/canvas-settings"

	get := fx.request(t, http.MethodGet, path, nil, fx.claims(fx.teacher))
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	assert.JSONEq(t, `{"canvasFloor":"private"}`, get.Body.String())

	patched := fx.request(t, http.MethodPatch, path, map[string]string{"canvasFloor": "participants"}, fx.claims(fx.teacher))
	require.Equal(t, http.StatusOK, patched.Code, patched.Body.String())
	assert.JSONEq(t, `{"canvasFloor":"participants"}`, patched.Body.String())

	for _, tc := range []struct {
		name string
		body any
		user *store.RegisteredUser
		want int
	}{
		{"student forbidden", map[string]string{"canvasFloor": "host"}, fx.student, http.StatusForbidden},
		{"outsider forbidden", map[string]string{"canvasFloor": "host"}, fx.outsider, http.StatusForbidden},
		{"session floor rejected", map[string]string{"canvasFloor": "session"}, fx.teacher, http.StatusBadRequest},
		{"unknown field rejected", map[string]string{"canvasFloor": "host", "status": "ended"}, fx.teacher, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := fx.request(t, http.MethodPatch, path, tc.body, fx.claims(tc.user))
			require.Equal(t, tc.want, w.Code, w.Body.String())
		})
	}
}

func TestCanvasSettings_EndedTeacherReadsDurableTrueFalseAndNull(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"confirmed", true, `{"canvasFloor":"private","whiteboardServerArchiveComplete":true}`},
		{"degraded", false, `{"canvasFloor":"private","whiteboardServerArchiveComplete":false}`},
		{"durable null omitted", nil, `{"canvasFloor":"private"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newCanvasHandlerFixture(t)
			_, err := fx.db.ExecContext(context.Background(), `UPDATE sessions SET status = 'ended', whiteboard_server_archive_complete = $1 WHERE id = $2`, tc.value, fx.session.ID)
			require.NoError(t, err)
			w := fx.request(t, http.MethodGet, "/api/sessions/"+fx.session.ID+"/canvas-settings", nil, fx.claims(fx.teacher))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.JSONEq(t, tc.want, w.Body.String())
		})
	}
}

func TestCanvasHandler_CreateCanvas_DeniesInvitedAndUnrepresentedAdministrator(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	invitee := fx.addUser(t, "invitee")
	require.NoError(t, func() error {
		_, err := fx.h.Sessions.AddParticipant(context.Background(), fx.session.ID, invitee.ID, fx.teacher.ID)
		return err
	}())

	create := func(claims *auth.Claims) *httptest.ResponseRecorder {
		return fx.request(t, http.MethodPost, "/api/sessions/"+fx.session.ID+"/canvases", map[string]string{"title": "new board", "visibility": "private"}, claims)
	}
	require.Equal(t, http.StatusCreated, create(fx.claims(fx.teacher)).Code, "represented teacher remains allowed")
	require.Equal(t, http.StatusCreated, create(fx.claims(fx.student)).Code, "currently present participant remains allowed")
	require.Equal(t, http.StatusForbidden, create(fx.claims(invitee)).Code, "invited is not present")
	admin := fx.claims(fx.outsider)
	admin.IsPlatformAdmin = true
	require.Equal(t, http.StatusForbidden, create(admin).Code, "platform admin has no independent creator bypass")
}
