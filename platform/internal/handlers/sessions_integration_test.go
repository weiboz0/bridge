package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// sessionFixture is the world a session integration test runs against.
type sessionFixture struct {
	db          *sql.DB
	h           *SessionHandler
	router      chi.Router
	orgs        *store.OrgStore
	classes     *store.ClassStore
	teacher     *store.RegisteredUser
	student     *store.RegisteredUser
	otherUser   *store.RegisteredUser // not a participant or teacher
	orgID       string
	classID     string
	sessionID   string
	session     *store.LiveSession
}

func newSessionFixture(t *testing.T, suffix string) *sessionFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()

	orgs := store.NewOrgStore(db)
	users := store.NewUserStore(db)
	courses := store.NewCourseStore(db)
	classes := store.NewClassStore(db)
	sessions := store.NewSessionStore(db)

	broadcaster := events.NewBroadcaster()

	h := &SessionHandler{
		Sessions:    sessions,
		Classes:     classes,
		Orgs:        orgs,
		Broadcaster: broadcaster,
	}

	// Create org, teacher, students
	org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name: "Org " + suffix, Slug: "org-" + suffix,
		Type: "school", ContactEmail: suffix + "@example.com", ContactName: "Admin",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	mkUser := func(label string) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name: "User " + label, Email: label + "@example.com", Password: "testpassword123",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM session_participants WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	teacher := mkUser(suffix + "-teacher")
	student := mkUser(suffix + "-student")
	otherUser := mkUser(suffix + "-other")

	// Add teacher to org
	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: org.ID, UserID: teacher.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)

	// Create course + class
	course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: org.ID, CreatedBy: teacher.ID, Title: "Course " + suffix, GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, err := classes.CreateClass(ctx, store.CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Class " + suffix, CreatedBy: teacher.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM sessions WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	// Create a live session
	session, err := sessions.CreateSession(ctx, store.CreateSessionInput{
		ClassID: class.ID, TeacherID: teacher.ID,
	})
	require.NoError(t, err)

	// Build router with all session routes
	r := chi.NewRouter()
	h.Routes(r)

	fx := &sessionFixture{
		db:        db,
		h:         h,
		router:    r,
		orgs:      orgs,
		classes:   classes,
		teacher:   teacher,
		student:   student,
		otherUser: otherUser,
		orgID:     org.ID,
		classID:   class.ID,
		sessionID: session.ID,
		session:   session,
	}
	return fx
}

func (fx *sessionFixture) claims(u *store.RegisteredUser, admin bool) *auth.Claims {
	return &auth.Claims{UserID: u.ID, Email: u.Email, Name: u.Name, IsPlatformAdmin: admin}
}

// doRequest executes a request through the Chi router with auth claims injected.
func (fx *sessionFixture) doRequest(t *testing.T, method, path string, body any, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if claims != nil {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	}
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)
	return w
}

// ------------------- PATCH /api/sessions/{id} -------------------

func TestSessionHandler_PatchSession_TeacherUpdatesTitle(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"title": "New Title"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	assert.Equal(t, "New Title", session.Title)
}

func TestSessionHandler_PatchSession_TeacherUpdatesSettings(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"settings": `{"mode":"collaborative"}`}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	assert.Contains(t, session.Settings, `"mode"`)
	assert.Contains(t, session.Settings, `"collaborative"`)
}

func TestSessionHandler_PatchSession_TeacherUpdatesInviteExpiry(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	future := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC()
	body := map[string]any{"inviteExpiresAt": future.Format(time.RFC3339)}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	require.NotNil(t, session.InviteExpiresAt)
	assert.WithinDuration(t, future, *session.InviteExpiresAt, time.Second)
}

func TestSessionHandler_PatchSession_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"title": "Hacked"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_PatchSession_AdminAllowed(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"title": "Admin Edit"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, fx.claims(fx.otherUser, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_PatchSession_NotFound(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"title": "Ghost"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/00000000-0000-0000-0000-000000000000", body, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSessionHandler_PatchSession_Unauthenticated(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"title": "Anon"}
	w := fx.doRequest(t, http.MethodPatch, "/api/sessions/"+fx.sessionID, body, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ------------------- POST /api/sessions/{id}/rotate-invite -------------------

func TestSessionHandler_RotateInviteToken_Teacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// First rotation
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var session1 store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session1))
	require.NotNil(t, session1.InviteToken)
	firstToken := *session1.InviteToken

	// Second rotation — token should change
	w2 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w2.Code)

	var session2 store.LiveSession
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &session2))
	require.NotNil(t, session2.InviteToken)
	assert.NotEqual(t, firstToken, *session2.InviteToken, "rotated token should differ")
}

func TestSessionHandler_RotateInviteToken_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_RotateInviteToken_NotFound(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/00000000-0000-0000-0000-000000000000/rotate-invite", nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ------------------- DELETE /api/sessions/{id}/invite -------------------

func TestSessionHandler_RevokeInviteToken_Teacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// Generate a token first
	fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))

	// Revoke
	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/invite", nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSessionHandler_RevokeInviteToken_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/invite", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_RevokeInviteToken_ThenTokenJoin404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// Rotate to get a token
	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w1.Code)
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &session))
	token := *session.InviteToken

	// Revoke
	w2 := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusNoContent, w2.Code)

	// Try joining with revoked token
	w3 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusNotFound, w3.Code)
}

// ------------------- POST /api/s/{token}/join -------------------

func TestSessionHandler_TokenJoin_HappyPath(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// Rotate to get a token
	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w1.Code)
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &session))
	token := *session.InviteToken

	// Join via token
	w2 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	require.Equal(t, http.StatusOK, w2.Code, "body=%s", w2.Body.String())

	var result map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &result))
	assert.Equal(t, fx.sessionID, result["sessionId"])
	assert.NotNil(t, result["participant"])
}

func TestSessionHandler_TokenJoin_UnknownToken404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/s/nonexistent_token_12345/join", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSessionHandler_TokenJoin_ExpiredToken410(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Rotate to get a token
	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w1.Code)
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &session))
	token := *session.InviteToken

	// Set expiry to the past
	past := time.Now().Add(-1 * time.Hour)
	_, err := fx.h.Sessions.SetInviteExpiry(ctx, fx.sessionID, &past)
	require.NoError(t, err)

	// Try joining
	w2 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusGone, w2.Code)
}

func TestSessionHandler_TokenJoin_EndedSession410(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Rotate to get a token
	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w1.Code)
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &session))
	token := *session.InviteToken

	// End the session
	_, err := fx.h.Sessions.EndSession(ctx, fx.sessionID)
	require.NoError(t, err)

	// Try joining
	w2 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusGone, w2.Code)
}

func TestSessionHandler_TokenJoin_AlreadyParticipant_Idempotent(t *testing.T) {
	fx := newSessionFixture(t, t.Name())

	// Rotate to get a token
	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/rotate-invite", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w1.Code)
	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &session))
	token := *session.InviteToken

	// Join twice
	w2 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	require.Equal(t, http.StatusOK, w2.Code)

	w3 := fx.doRequest(t, http.MethodPost, "/api/s/"+token+"/join", nil, fx.claims(fx.student, false))
	require.Equal(t, http.StatusOK, w3.Code, "second join should also succeed (idempotent)")
}

func TestSessionHandler_TokenJoin_Unauthenticated401(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/s/some_token/join", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ------------------- POST /api/sessions/{id}/end (moved from PATCH) -------------------

func TestSessionHandler_EndSession_ViaPost(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var session store.LiveSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	assert.Equal(t, "ended", session.Status)
}

func TestSessionHandler_EndSession_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/end", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ------------------- POST /api/sessions/{id}/participants -------------------

func TestSessionHandler_AddParticipant_TeacherAddsByUserId(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"userId": fx.student.ID}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var p store.SessionParticipant
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	assert.Equal(t, fx.student.ID, p.StudentID)
	assert.Equal(t, "invited", p.Status)
	assert.NotNil(t, p.InvitedBy)
	assert.Equal(t, fx.teacher.ID, *p.InvitedBy)
}

func TestSessionHandler_AddParticipant_TeacherAddsByEmail(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"email": fx.student.Email}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var p store.SessionParticipant
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	assert.Equal(t, fx.student.ID, p.StudentID)
}

func TestSessionHandler_AddParticipant_UnknownEmail404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"email": "nonexistent-user@example.com"}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSessionHandler_AddParticipant_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"userId": fx.otherUser.ID}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_AddParticipant_PlatformAdmin(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"userId": fx.student.ID}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.otherUser, true))
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestSessionHandler_AddParticipant_Unauthenticated401(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"userId": fx.student.ID}
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSessionHandler_AddParticipant_Idempotent(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	body := map[string]any{"userId": fx.student.ID}

	w1 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusCreated, w1.Code)

	// Adding the same user again should also succeed (idempotent).
	w2 := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/participants", body, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusCreated, w2.Code, "second add should be idempotent")
}

// ------------------- DELETE /api/sessions/{id}/participants/{userId} -------------------

func TestSessionHandler_RemoveParticipant_Teacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Add a participant first
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.student.ID, fx.teacher.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/participants/"+fx.student.ID, nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSessionHandler_RemoveParticipant_VerifyAccessRevoked(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Add student as a participant (direct add, not class member)
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.otherUser.ID, fx.teacher.ID)
	require.NoError(t, err)

	// Verify access before removal
	allowed, _, err := fx.h.Sessions.CanAccessSession(ctx, fx.sessionID, fx.otherUser.ID)
	require.NoError(t, err)
	assert.True(t, allowed, "participant should have access before removal")

	// Remove the participant
	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/participants/"+fx.otherUser.ID, nil, fx.claims(fx.teacher, false))
	require.Equal(t, http.StatusNoContent, w.Code)

	// Verify access is revoked
	allowed, _, err = fx.h.Sessions.CanAccessSession(ctx, fx.sessionID, fx.otherUser.ID)
	require.NoError(t, err)
	assert.False(t, allowed, "participant should lose access after removal")
}

func TestSessionHandler_RemoveParticipant_NonTeacher403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/participants/"+fx.student.ID, nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_RemoveParticipant_NotFound404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	// Try to remove a non-existent participant
	w := fx.doRequest(t, http.MethodDelete, "/api/sessions/"+fx.sessionID+"/participants/"+fx.otherUser.ID, nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ------------------- GET /api/sessions/{id} (tightened access) -------------------

func TestSessionHandler_GetSession_AccessTeacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetSession_AccessClassMember(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Add student to the class
	_, err := fx.classes.AddClassMember(ctx, store.AddClassMemberInput{
		ClassID: fx.classID, UserID: fx.student.ID, Role: "student",
	})
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetSession_AccessTokenJoinedParticipant(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Add otherUser as a participant (simulating token join)
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.otherUser.ID, fx.teacher.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetSession_AccessRandomUser404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	// otherUser is not teacher, not a class member, not a participant
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "random user should get 404 (not leak existence)")
}

func TestSessionHandler_GetSession_AccessPlatformAdmin(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID, nil, fx.claims(fx.otherUser, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

// ------------------- GET /api/sessions/{id}/participants (tightened roster access) -------------------

func TestSessionHandler_GetParticipants_AccessTeacher(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/participants", nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetParticipants_AccessRegularParticipant403(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	ctx := context.Background()

	// Add student as a participant
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.student.ID, fx.teacher.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/participants", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code, "regular participant should not read the roster")
}

func TestSessionHandler_GetParticipants_AccessPlatformAdmin(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/participants", nil, fx.claims(fx.otherUser, true))
	assert.Equal(t, http.StatusOK, w.Code)
}
