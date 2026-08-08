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

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

const canvasHandlerTestDatabaseURL = "postgresql://work@127.0.0.1:5432/bridge_test"

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
	var actual string
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT current_database()").Scan(&actual))
	require.True(t, strings.HasSuffix(actual, "_test"), "connected to non-test database %q", actual)

	users := store.NewUserStore(db)
	mkUser := func(label string) *store.RegisteredUser {
		u, err := users.RegisterUser(context.Background(), store.RegisterInput{Name: label, Email: fmt.Sprintf("%s-%s@example.com", t.Name(), label), Password: "testpassword123"})
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = db.ExecContext(context.Background(), "DELETE FROM session_participants WHERE user_id = $1", u.ID)
			_, _ = db.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}
	teacher, student, outsider := mkUser("teacher"), mkUser("student"), mkUser("outsider")
	sessions := store.NewSessionStore(db)
	session, err := sessions.CreateSession(context.Background(), store.CreateSessionInput{TeacherID: teacher.ID, Title: "Canvas session"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM sessions WHERE id = $1", session.ID) })
	_, err = sessions.JoinSession(context.Background(), session.ID, student.ID)
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
	u, err := store.NewUserStore(fx.db).RegisterUser(context.Background(), store.RegisterInput{Name: label, Email: fmt.Sprintf("%s-%s@example.com", t.Name(), label), Password: "testpassword123"})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = fx.db.ExecContext(context.Background(), "DELETE FROM session_participants WHERE user_id = $1", u.ID)
		_, _ = fx.db.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", u.ID)
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
	for _, body := range []map[string]string{{"title": "", "visibility": "private"}, {"title": "Board", "visibility": "unknown"}} {
		w := fx.request(t, http.MethodPost, "/api/sessions/"+fx.session.ID+"/canvases", body, fx.claims(fx.student))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
	canvas, err := fx.h.Canvases.CreateCanvas(context.Background(), store.CreateCanvasInput{SessionID: fx.session.ID, OwnerID: fx.student.ID, Title: "Board", Visibility: "private"})
	require.NoError(t, err)
	w := fx.request(t, http.MethodPatch, "/api/sessions/"+fx.session.ID+"/canvases/"+canvas.ID, map[string]string{"visibility": "unknown"}, fx.claims(fx.student))
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
