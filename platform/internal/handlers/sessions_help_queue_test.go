package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 063 — auth matrix for /api/sessions/{id}/help-queue (GET +
// POST) and /events (SSE). Pre-063 these handlers only checked
// `claims != nil`; outsiders could read the help queue or subscribe
// to live events for any session by UUID.

// addClassMember helper — adds the given user to fx.classID with
// the requested role. Cleanup is handled by the fixture's class
// teardown.
func addClassMember(t *testing.T, fx *sessionFixture, userID, role string) {
	t.Helper()
	_, err := fx.classes.AddClassMember(context.Background(), store.AddClassMemberInput{
		ClassID: fx.classID, UserID: userID, Role: role,
	})
	require.NoError(t, err)
}

// addOrgMember helper — adds the given user to fx.orgID with the
// given role + active status.
func addOrgMember(t *testing.T, fx *sessionFixture, userID, role string) {
	t.Helper()
	_, err := fx.orgs.AddOrgMember(context.Background(), store.AddMemberInput{
		OrgID: fx.orgID, UserID: userID, Role: role, Status: "active",
	})
	require.NoError(t, err)
}

// --- GetHelpQueue (teacher-only roster) ---

func TestSessionHandler_GetHelpQueue_TeacherOK(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/help-queue", nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetHelpQueue_PlatformAdminOK(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/help-queue", nil, fx.claims(fx.otherUser, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetHelpQueue_StudentInClassDenied(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	addClassMember(t, fx, fx.student.ID, "student")
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/help-queue", nil, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusForbidden, w.Code, "students must not see the help queue roster")
}

func TestSessionHandler_GetHelpQueue_OutsiderDenied(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/help-queue", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSessionHandler_GetHelpQueue_ClassInstructorOK(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	// otherUser is added as a class instructor (not the session
	// teacher) — should get authority via the class-instructor path.
	addClassMember(t, fx, fx.otherUser.ID, "instructor")
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/help-queue", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetHelpQueue_ClassTAOK(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	addClassMember(t, fx, fx.otherUser.ID, "ta")
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/help-queue", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetHelpQueue_OrgAdminOK(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	addOrgMember(t, fx, fx.otherUser.ID, "org_admin")
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/help-queue", nil, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_GetHelpQueue_NoClaims_401(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/"+fx.sessionID+"/help-queue", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSessionHandler_GetHelpQueue_NotFound_404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/sessions/00000000-0000-0000-0000-000000000099/help-queue", nil, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- ToggleHelp (class member self-flag) ---

func TestSessionHandler_ToggleHelp_StudentInClassOK(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	addClassMember(t, fx, fx.student.ID, "student")
	// Student must be a session participant for UpdateParticipantStatus
	// to find the row. Pre-create one.
	_, err := fx.h.Sessions.JoinSession(context.Background(), fx.sessionID, fx.student.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/help-queue",
		map[string]any{"raised": true}, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_ToggleHelp_OutsiderDenied(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/help-queue",
		map[string]any{"raised": true}, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusForbidden, w.Code, "outsider cannot raise/lower hand for any session")
}

func TestSessionHandler_ToggleHelp_TeacherCanFlagSelf(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	// Session teacher pre-joins as a participant — they should be
	// able to use the help-queue endpoint for themselves (admins/
	// teachers participate via the same session_participants row
	// when they join the session).
	_, err := fx.h.Sessions.JoinSession(context.Background(), fx.sessionID, fx.teacher.ID)
	require.NoError(t, err)
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/"+fx.sessionID+"/help-queue",
		map[string]any{"raised": false}, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionHandler_ToggleHelp_NotFound_404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/sessions/00000000-0000-0000-0000-000000000099/help-queue",
		map[string]any{"raised": true}, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- SessionEvents (SSE — class members can subscribe) ---
//
// SSE responses are open-ended; for these tests we construct a
// short-lived request whose context is cancelled almost
// immediately, then assert the status code that came back BEFORE
// any SSE data went out. Auth-deny paths return cleanly because
// the gate runs BEFORE the SSE headers.

// callSSE invokes SessionEvents with a very short timeout. Returns
// the response code. For the success path the connection opens,
// flushes the "connected" event, and the context cancels.
func callSSE(t *testing.T, fx *sessionFixture, sessionID string, claims *auth.Claims) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID+"/events", nil)
	if claims != nil {
		req = req.WithContext(auth.ContextWithClaims(ctx, claims))
	} else {
		req = req.WithContext(ctx)
	}
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)
	return w.Code
}

func TestSessionHandler_SessionEvents_StudentInClassOK(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	addClassMember(t, fx, fx.student.ID, "student")
	code := callSSE(t, fx, fx.sessionID, fx.claims(fx.student, false))
	assert.Equal(t, http.StatusOK, code, "class member should subscribe to live events")
}

func TestSessionHandler_SessionEvents_TeacherOK(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	code := callSSE(t, fx, fx.sessionID, fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusOK, code)
}

func TestSessionHandler_SessionEvents_OutsiderDenied(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	code := callSSE(t, fx, fx.sessionID, fx.claims(fx.otherUser, false))
	assert.Equal(t, http.StatusForbidden, code, "outsider must NOT subscribe to a session they aren't in")
}

func TestSessionHandler_SessionEvents_PlatformAdminOK(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	code := callSSE(t, fx, fx.sessionID, fx.claims(fx.otherUser, true))
	assert.Equal(t, http.StatusOK, code)
}

func TestSessionHandler_SessionEvents_NoClaims_401(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	code := callSSE(t, fx, fx.sessionID, nil)
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestSessionHandler_SessionEvents_NotFound_404(t *testing.T) {
	fx := newSessionFixture(t, t.Name())
	code := callSSE(t, fx, "00000000-0000-0000-0000-000000000099", fx.claims(fx.teacher, false))
	assert.Equal(t, http.StatusNotFound, code)
}
