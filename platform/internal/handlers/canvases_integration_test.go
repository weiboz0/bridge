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
const canvasControlTestSecret = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

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
		Sessions:                fx.h.Sessions,
		Canvases:                fx.h.Canvases,
		Users:                   store.NewUserStore(fx.db),
		HocuspocusTokenSecret:   rtSecret,
		HocuspocusControlSecret: canvasControlTestSecret,
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

func TestCanvasHandler_CreateCanvasMissingSessionReturns404(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	w := fx.request(t, http.MethodPost, "/api/sessions/00000000-0000-4000-8000-000000000000/canvases", map[string]string{
		"title": "Missing session board", "visibility": "private",
	}, fx.claims(fx.teacher))
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	require.NotEqual(t, "null\n", w.Body.String(), "a missing session must not serialize as a created null canvas")
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
		{"host floor", http.MethodPatch, "/api/sessions/" + fx.session.ID + "/canvas-settings", map[string]string{"canvasFloor": "host"}, fx.claims(fx.teacher), http.StatusOK},
		{"non-host floor", http.MethodPatch, "/api/sessions/" + fx.session.ID + "/canvas-settings", map[string]string{"canvasFloor": "participants"}, fx.claims(fx.student), http.StatusForbidden},
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
	} {
		w := fx.request(t, tc.method, tc.path, tc.body, fx.claims(fx.student))
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	}
	// The floor route is teacher-only, so the student above is never its
	// authorized caller: authorization answers first, and only the teacher
	// reaches the ended-session conflict.
	settings := "/api/sessions/" + fx.session.ID + "/canvas-settings"
	floor := map[string]string{"canvasFloor": "participants"}
	w := fx.request(t, http.MethodPatch, settings, floor, fx.claims(fx.student))
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	w = fx.request(t, http.MethodPatch, settings, floor, fx.claims(fx.teacher))
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

// Plan 094 Phase 4 acceptance: once the session is ended, no mutating canvas
// endpoint may succeed again.  The caller here is the owning participant who
// is still `present` — the most privileged live mutator there is — so a 409
// proves the terminal state, not authorization, is doing the work.
//
// The teacher-only floor route keeps its deliberate ordering: authorization
// answers before the terminal-state conflict, so a non-teacher still gets 403
// there and only the teacher reaches the 409.
func TestCanvases_MutatingEndpointsReject_WhenEnded(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	created := fx.request(t, http.MethodPost, "/api/sessions/"+fx.session.ID+"/canvases", map[string]string{"title": "Owned board", "visibility": "private"}, fx.claims(fx.student))
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var canvas store.Canvas
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &canvas))

	_, err := fx.h.Sessions.EndSession(context.Background(), fx.session.ID)
	require.NoError(t, err)

	for _, tc := range []struct {
		name, method, path string
		body               any
	}{
		{"create", http.MethodPost, "/api/sessions/" + fx.session.ID + "/canvases", map[string]string{"title": "Late", "visibility": "host"}},
		{"patch canvas", http.MethodPatch, "/api/sessions/" + fx.session.ID + "/canvases/" + canvas.ID, map[string]string{"title": "Late"}},
		{"delete canvas", http.MethodDelete, "/api/sessions/" + fx.session.ID + "/canvases/" + canvas.ID, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := fx.request(t, tc.method, tc.path, tc.body, fx.claims(fx.student))
			require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		})
	}

	settings := "/api/sessions/" + fx.session.ID + "/canvas-settings"
	floor := map[string]string{"canvasFloor": "participants"}
	nonTeacher := fx.request(t, http.MethodPatch, settings, floor, fx.claims(fx.student))
	require.Equal(t, http.StatusForbidden, nonTeacher.Code, nonTeacher.Body.String())
	teacher := fx.request(t, http.MethodPatch, settings, floor, fx.claims(fx.teacher))
	require.Equal(t, http.StatusConflict, teacher.Code, teacher.Body.String())

	// The rejections are real: nothing was created, renamed, deleted, or
	// re-floored behind them.
	var count int
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT count(*) FROM session_canvases WHERE session_id = $1`, fx.session.ID).Scan(&count))
	assert.Equal(t, 1, count, "a rejected create must not insert a canvas row")
	var title, storedFloor string
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT title FROM session_canvases WHERE id = $1`, canvas.ID).Scan(&title))
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT canvas_floor FROM sessions WHERE id = $1`, fx.session.ID).Scan(&storedFloor))
	assert.Equal(t, "Owned board", title)
	assert.Equal(t, "private", storedFloor)
}

func TestCanvasMutations_BlockBehindEndLifecycleLock(t *testing.T) {
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
		{http.MethodPatch, "/api/sessions/" + fx.session.ID + "/canvas-settings", map[string]string{"canvasFloor": "host"}, fx.claims(fx.teacher)},
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
	w = fx.request(t, http.MethodPatch, "/api/sessions/"+fx.session.ID+"/canvas-settings", map[string]string{"canvasFloor": "session"}, fx.claims(fx.teacher))
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

func TestCanvasSettings_AllAllowedFloorsAndTrailingJSONAreExact(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	path := "/api/sessions/" + fx.session.ID + "/canvas-settings"
	for _, floor := range []string{"private", "host", "participants"} {
		w := fx.request(t, http.MethodPatch, path, map[string]string{"canvasFloor": floor}, fx.claims(fx.teacher))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.JSONEq(t, `{"canvasFloor":"`+floor+`"}`, w.Body.String())
	}
	request := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"canvasFloor":"host"}{}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(auth.ContextWithClaims(request.Context(), fx.claims(fx.teacher)))
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, request)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCanvasHandler_PublicClasslessOutsiderAndConcurrentCapCannotCreate(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	_, err := fx.db.ExecContext(context.Background(), `UPDATE sessions SET visibility='public' WHERE id=$1`, fx.session.ID)
	require.NoError(t, err)
	path := "/api/sessions/" + fx.session.ID + "/canvases"
	require.Equal(t, http.StatusForbidden, fx.request(t, http.MethodPost, path, map[string]string{"title": "outsider", "visibility": "private"}, fx.claims(fx.outsider)).Code)

	// Put the session immediately below its cap.  The two simultaneous creator
	// requests must serialize under the lifecycle/session lock: exactly one may
	// receive the final slot, even on a public class-less session.
	for i := 0; i < store.MaxSessionCanvases-1; i++ {
		_, err := fx.h.Canvases.CreateCanvas(context.Background(), store.CreateCanvasInput{SessionID: fx.session.ID, OwnerID: fx.student.ID, Title: fmt.Sprintf("seed-%d", i), Visibility: "private"})
		require.NoError(t, err)
	}
	start := make(chan struct{})
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			<-start
			payload, _ := json.Marshal(map[string]string{"title": fmt.Sprintf("race-%d", i), "visibility": "private"})
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(auth.ContextWithClaims(req.Context(), fx.claims(fx.student)))
			w := httptest.NewRecorder()
			fx.router.ServeHTTP(w, req)
			results <- w.Code
		}(i)
	}
	close(start)
	first, second := <-results, <-results
	require.ElementsMatch(t, []int{http.StatusCreated, http.StatusConflict}, []int{first, second})
	var count int
	require.NoError(t, fx.db.QueryRowContext(context.Background(), `SELECT count(*) FROM session_canvases WHERE session_id=$1`, fx.session.ID).Scan(&count))
	require.Equal(t, store.MaxSessionCanvases, count)
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

func TestCanvasSettings_ExactAuthorizationAndMissingEndedMatrix(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	path := "/api/sessions/" + fx.session.ID + "/canvas-settings"
	missing := "/api/sessions/00000000-0000-4000-8000-000000000000/canvas-settings"
	require.Equal(t, http.StatusNotFound, fx.request(t, http.MethodGet, missing, nil, fx.claims(fx.teacher)).Code)
	require.Equal(t, http.StatusNotFound, fx.request(t, http.MethodPatch, missing, map[string]string{"canvasFloor": "host"}, fx.claims(fx.teacher)).Code)
	admin := fx.claims(fx.outsider)
	admin.IsPlatformAdmin = true
	require.Equal(t, http.StatusForbidden, fx.request(t, http.MethodGet, path, nil, admin).Code)
	require.Equal(t, http.StatusForbidden, fx.request(t, http.MethodPatch, path, map[string]string{"canvasFloor": "host"}, admin).Code)
	impersonatingTeacher := fx.claims(fx.teacher)
	impersonatingTeacher.ImpersonatedBy = fx.outsider.ID
	require.Equal(t, http.StatusOK, fx.request(t, http.MethodGet, path, nil, impersonatingTeacher).Code)
	_, err := fx.db.ExecContext(context.Background(), `UPDATE sessions SET status='ended' WHERE id=$1`, fx.session.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, fx.request(t, http.MethodPatch, path, map[string]string{"canvasFloor": "host"}, fx.claims(fx.student)).Code)
	require.Equal(t, http.StatusConflict, fx.request(t, http.MethodPatch, path, map[string]string{"canvasFloor": "host"}, fx.claims(fx.teacher)).Code)
}

// Plan 094 R2-3: every canvas mutation answers authorization BEFORE the
// terminal-state conflict, so a caller who was never authorized to mutate
// learns nothing about the session's lifecycle from the response.  Before this
// change an outsider's create returned 409 on a freezing or ended session and
// 403 only while it was live, and a non-owner update/delete returned 409 on an
// ended session, which is exactly the live/ended oracle this pins shut.
func TestCanvases_AuthorizationPrecedesTerminalState(t *testing.T) {
	const missingSession = "00000000-0000-4000-8000-000000000000"
	const missingCanvas = "00000000-0000-4000-8000-0000000000ff"

	type mutationRow struct {
		name, method, path string
		body               any
		claims             *auth.Claims
		want               map[string]int
	}

	for _, state := range []string{"live", "frozen", "ended"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			fx := newCanvasHandlerFixture(t)
			invitee := fx.addUser(t, "invitee")
			_, err := fx.h.Sessions.AddParticipant(ctx, fx.session.ID, invitee.ID, fx.teacher.ID)
			require.NoError(t, err)
			departed := fx.addUser(t, "departed")
			_, err = fx.h.Sessions.JoinSession(ctx, fx.session.ID, departed.ID)
			require.NoError(t, err)
			_, err = fx.h.Sessions.LeaveSession(ctx, fx.session.ID, departed.ID)
			require.NoError(t, err)
			canvas, err := fx.h.Canvases.CreateCanvas(ctx, store.CreateCanvasInput{
				SessionID: fx.session.ID, OwnerID: fx.student.ID, Title: "Owned board", Visibility: "private",
			})
			require.NoError(t, err)

			switch state {
			case "frozen":
				// The real production lease, taken exactly as EndSession takes it.
				prep, prepErr := fx.h.Sessions.PrepareSessionEnd(ctx, fx.session.ID)
				require.NoError(t, prepErr)
				require.NotEmpty(t, prep.Token)
			case "ended":
				_, endErr := fx.h.Sessions.EndSession(ctx, fx.session.ID)
				require.NoError(t, endErr)
			}

			createPath := "/api/sessions/" + fx.session.ID + "/canvases"
			canvasPath := createPath + "/" + canvas.ID
			missingCanvasPath := createPath + "/" + missingCanvas
			missingSessionCreate := "/api/sessions/" + missingSession + "/canvases"
			missingSessionCanvas := missingSessionCreate + "/" + canvas.ID
			newBoard := map[string]string{"title": "New board", "visibility": "private"}
			rename := map[string]string{"title": "Renamed"}

			forbidden := map[string]int{"live": http.StatusForbidden, "frozen": http.StatusForbidden, "ended": http.StatusForbidden}
			missing := map[string]int{"live": http.StatusNotFound, "frozen": http.StatusNotFound, "ended": http.StatusNotFound}
			okCreate := map[string]int{"live": http.StatusCreated, "frozen": http.StatusConflict, "ended": http.StatusConflict}
			okUpdate := map[string]int{"live": http.StatusOK, "frozen": http.StatusConflict, "ended": http.StatusConflict}
			okDelete := map[string]int{"live": http.StatusNoContent, "frozen": http.StatusConflict, "ended": http.StatusConflict}

			// Unauthorized callers first: none of them may change anything, and
			// none of them may learn the lifecycle state from the status code.
			rejected := []mutationRow{
				{"outsider create", http.MethodPost, createPath, newBoard, fx.claims(fx.outsider), forbidden},
				{"invited-only create", http.MethodPost, createPath, newBoard, fx.claims(invitee), forbidden},
				{"left participant create", http.MethodPost, createPath, newBoard, fx.claims(departed), forbidden},
				{"outsider update", http.MethodPatch, canvasPath, rename, fx.claims(fx.outsider), forbidden},
				{"invited-only update", http.MethodPatch, canvasPath, rename, fx.claims(invitee), forbidden},
				{"left participant update", http.MethodPatch, canvasPath, rename, fx.claims(departed), forbidden},
				{"session teacher update of another owner's canvas", http.MethodPatch, canvasPath, rename, fx.claims(fx.teacher), forbidden},
				{"outsider delete", http.MethodDelete, canvasPath, nil, fx.claims(fx.outsider), forbidden},
				{"invited-only delete", http.MethodDelete, canvasPath, nil, fx.claims(invitee), forbidden},
				{"left participant delete", http.MethodDelete, canvasPath, nil, fx.claims(departed), forbidden},
				{"session teacher delete of another owner's canvas", http.MethodDelete, canvasPath, nil, fx.claims(fx.teacher), forbidden},
				{"owner update of missing canvas", http.MethodPatch, missingCanvasPath, rename, fx.claims(fx.student), missing},
				{"owner delete of missing canvas", http.MethodDelete, missingCanvasPath, nil, fx.claims(fx.student), missing},
				{"outsider update of missing canvas", http.MethodPatch, missingCanvasPath, rename, fx.claims(fx.outsider), missing},
				{"outsider delete of missing canvas", http.MethodDelete, missingCanvasPath, nil, fx.claims(fx.outsider), missing},
				{"teacher create on missing session", http.MethodPost, missingSessionCreate, newBoard, fx.claims(fx.teacher), missing},
				{"outsider create on missing session", http.MethodPost, missingSessionCreate, newBoard, fx.claims(fx.outsider), missing},
				{"owner update on missing session", http.MethodPatch, missingSessionCanvas, rename, fx.claims(fx.student), missing},
				{"owner delete on missing session", http.MethodDelete, missingSessionCanvas, nil, fx.claims(fx.student), missing},
			}
			for _, tc := range rejected {
				t.Run(tc.name, func(t *testing.T) {
					w := fx.request(t, tc.method, tc.path, tc.body, tc.claims)
					require.Equal(t, tc.want[state], w.Code, w.Body.String())
					require.NotContains(t, w.Body.String(), "session_end_in_progress", "an unauthorized caller must not be told a lease is held")
				})
			}
			assertCanvasStateUnchanged(t, fx, canvas.ID, 1, "Owned board")

			// Authorized callers still meet the terminal state, in exactly the
			// order the lifecycle requires.
			authorized := []mutationRow{
				{"teacher create", http.MethodPost, createPath, map[string]string{"title": "Teacher board", "visibility": "private"}, fx.claims(fx.teacher), okCreate},
				{"present participant create", http.MethodPost, createPath, newBoard, fx.claims(fx.student), okCreate},
				{"owner update", http.MethodPatch, canvasPath, rename, fx.claims(fx.student), okUpdate},
				{"owner delete", http.MethodDelete, canvasPath, nil, fx.claims(fx.student), okDelete},
			}
			for _, tc := range authorized {
				t.Run(tc.name, func(t *testing.T) {
					w := fx.request(t, tc.method, tc.path, tc.body, tc.claims)
					require.Equal(t, tc.want[state], w.Code, w.Body.String())
					switch state {
					case "frozen":
						require.JSONEq(t, `{"error":"Session end in progress","code":"session_end_in_progress"}`, w.Body.String())
					case "ended":
						require.JSONEq(t, `{"error":"Session has ended"}`, w.Body.String())
					}
				})
			}

			if state == "live" {
				var remaining int
				require.NoError(t, fx.db.QueryRowContext(ctx, `SELECT count(*) FROM session_canvases WHERE session_id = $1`, fx.session.ID).Scan(&remaining))
				assert.Equal(t, 2, remaining, "two authorized creates landed and the owner's own canvas was deleted")
				var deleted int
				require.NoError(t, fx.db.QueryRowContext(ctx, `SELECT count(*) FROM session_canvases WHERE id = $1`, canvas.ID).Scan(&deleted))
				assert.Zero(t, deleted, "the owner's delete must remove the row")
				return
			}
			assertCanvasStateUnchanged(t, fx, canvas.ID, 1, "Owned board")
		})
	}
}

// assertCanvasStateUnchanged proves the rejections above were real: no canvas
// was inserted, renamed, or deleted behind them, and the floor is untouched.
func assertCanvasStateUnchanged(t *testing.T, fx *canvasHandlerFixture, canvasID string, wantCount int, wantTitle string) {
	t.Helper()
	ctx := context.Background()
	var count int
	require.NoError(t, fx.db.QueryRowContext(ctx, `SELECT count(*) FROM session_canvases WHERE session_id = $1`, fx.session.ID).Scan(&count))
	assert.Equal(t, wantCount, count, "a rejected mutation must not insert or delete a canvas row")
	var title string
	require.NoError(t, fx.db.QueryRowContext(ctx, `SELECT title FROM session_canvases WHERE id = $1`, canvasID).Scan(&title))
	assert.Equal(t, wantTitle, title, "a rejected mutation must not rename a canvas")
	var floor string
	require.NoError(t, fx.db.QueryRowContext(ctx, `SELECT canvas_floor FROM sessions WHERE id = $1`, fx.session.ID).Scan(&floor))
	assert.Equal(t, "private", floor)
}

// Plan 094 R2-3, stated as the property itself: an authenticated user who holds
// nothing but a session UUID gets byte-identical answers whether that session
// is live, freezing, or ended.  Before the fix, create answered 403 live but
// 409 while freezing and after end.
func TestCanvases_OutsiderCannotDistinguishSessionState(t *testing.T) {
	const missingCanvas = "00000000-0000-4000-8000-0000000000ff"
	type observation struct {
		Code int
		Body string
	}
	observed := map[string]map[string]observation{}

	for _, state := range []string{"live", "frozen", "ended"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			fx := newCanvasHandlerFixture(t)
			canvas, err := fx.h.Canvases.CreateCanvas(ctx, store.CreateCanvasInput{
				SessionID: fx.session.ID, OwnerID: fx.student.ID, Title: "Owned board", Visibility: "private",
			})
			require.NoError(t, err)
			switch state {
			case "frozen":
				_, prepErr := fx.h.Sessions.PrepareSessionEnd(ctx, fx.session.ID)
				require.NoError(t, prepErr)
			case "ended":
				_, endErr := fx.h.Sessions.EndSession(ctx, fx.session.ID)
				require.NoError(t, endErr)
			}

			createPath := "/api/sessions/" + fx.session.ID + "/canvases"
			canvasPath := createPath + "/" + canvas.ID
			missingCanvasPath := createPath + "/" + missingCanvas
			record := func(name, method, path string, body any) {
				w := fx.request(t, method, path, body, fx.claims(fx.outsider))
				observed[state][name] = observation{Code: w.Code, Body: w.Body.String()}
			}
			observed[state] = map[string]observation{}
			record("create", http.MethodPost, createPath, map[string]string{"title": "No", "visibility": "private"})
			record("update", http.MethodPatch, canvasPath, map[string]string{"title": "No"})
			record("delete", http.MethodDelete, canvasPath, nil)
			record("update missing canvas", http.MethodPatch, missingCanvasPath, map[string]string{"title": "No"})
			record("delete missing canvas", http.MethodDelete, missingCanvasPath, nil)

			// Nothing the outsider sent may have changed the database.
			assertCanvasStateUnchanged(t, fx, canvas.ID, 1, "Owned board")
		})
	}

	require.Len(t, observed, 3)
	for _, operation := range []string{"create", "update", "delete", "update missing canvas", "delete missing canvas"} {
		t.Run(operation, func(t *testing.T) {
			live := observed["live"][operation]
			require.NotZero(t, live.Code)
			assert.Equal(t, live, observed["frozen"][operation], "a freezing session must answer an outsider exactly as a live one does")
			assert.Equal(t, live, observed["ended"][operation], "an ended session must answer an outsider exactly as a live one does")
		})
	}
	assert.Equal(t, http.StatusForbidden, observed["live"]["create"].Code)
	assert.Equal(t, http.StatusForbidden, observed["live"]["update"].Code)
	assert.Equal(t, http.StatusForbidden, observed["live"]["delete"].Code)
}

func TestCanvasHandler_CreateCanvas_DeniesInvitedAndUnrepresentedAdministrator(t *testing.T) {
	fx := newCanvasHandlerFixture(t)
	invitee := fx.addUser(t, fmt.Sprintf("invitee-%d", time.Now().UnixNano()))
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
	_, err := fx.h.Sessions.JoinSession(context.Background(), fx.session.ID, invitee.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.LeaveSession(context.Background(), fx.session.ID, invitee.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, create(fx.claims(invitee)).Code, "left participant is not currently present")
	impersonatingOutsider := fx.claims(fx.outsider)
	impersonatingOutsider.ImpersonatedBy = fx.teacher.ID
	require.Equal(t, http.StatusForbidden, create(impersonatingOutsider).Code, "impersonation does not create a separate creator bypass")
}
