package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 043 Phase 1 P0 security tests. Reuses the sessionPageFixture from
// sessions_page_integration_test.go which already wires teacher/student/
// outsider/admin/orgAdmin users + a class + a session.

// --- GetClass (ClassHandler) -------------------------------------------------

func newClassHandlerForFixture(fx *sessionPageFixture) *ClassHandler {
	return &ClassHandler{
		Classes: store.NewClassStore(fx.db),
		Orgs:    store.NewOrgStore(fx.db),
		Users:   store.NewUserStore(fx.db),
	}
}

func callGetClass(t *testing.T, ch *ClassHandler, classID string, claims *auth.Claims) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/classes/"+classID, nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"id": classID})
	w := httptest.NewRecorder()
	ch.GetClass(w, req)
	return w.Code, w.Body.Bytes()
}

func TestGetClass_NoClaims_Unauthorized(t *testing.T) {
	fx := newSessionPageFixture(t, "gc-noclaims")
	ch := newClassHandlerForFixture(fx)
	req := httptest.NewRequest(http.MethodGet, "/api/classes/"+fx.classID, nil)
	req = withChiParams(req, map[string]string{"id": fx.classID})
	w := httptest.NewRecorder()
	ch.GetClass(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetClass_ClassMember_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "gc-member")
	ch := newClassHandlerForFixture(fx)
	code, _ := callGetClass(t, ch, fx.classID, &auth.Claims{UserID: fx.student.ID})
	assert.Equal(t, http.StatusOK, code)
}

func TestGetClass_Teacher_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "gc-teacher")
	ch := newClassHandlerForFixture(fx)
	code, _ := callGetClass(t, ch, fx.classID, &auth.Claims{UserID: fx.teacher.ID})
	assert.Equal(t, http.StatusOK, code)
}

func TestGetClass_OrgAdmin_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "gc-orgadmin")
	ch := newClassHandlerForFixture(fx)
	code, _ := callGetClass(t, ch, fx.classID, &auth.Claims{UserID: fx.orgAdmin.ID})
	assert.Equal(t, http.StatusOK, code)
}

func TestGetClass_PlatformAdmin_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "gc-admin")
	ch := newClassHandlerForFixture(fx)
	code, _ := callGetClass(t, ch, fx.classID, &auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true})
	assert.Equal(t, http.StatusOK, code)
}

func TestGetClass_AdminImpersonating_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "gc-imp")
	ch := newClassHandlerForFixture(fx)
	code, _ := callGetClass(t, ch, fx.classID, &auth.Claims{
		UserID:         fx.outsider.ID,
		ImpersonatedBy: fx.admin.ID,
	})
	assert.Equal(t, http.StatusOK, code)
}

// The pre-043 bug: any authenticated user could read any class metadata by
// ID. This test fails closed — outsider gets 404.
func TestGetClass_Outsider_404(t *testing.T) {
	fx := newSessionPageFixture(t, "gc-outsider")
	ch := newClassHandlerForFixture(fx)
	code, _ := callGetClass(t, ch, fx.classID, &auth.Claims{UserID: fx.outsider.ID})
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetClass_NonexistentClass_404(t *testing.T) {
	fx := newSessionPageFixture(t, "gc-missing")
	ch := newClassHandlerForFixture(fx)
	bogus := "00000000-0000-0000-0000-000000000abc"
	code, _ := callGetClass(t, ch, bogus, &auth.Claims{UserID: fx.teacher.ID, IsPlatformAdmin: true})
	assert.Equal(t, http.StatusNotFound, code)
}

// --- JoinSession (SessionHandler) --------------------------------------------

func callJoinSession(t *testing.T, h *SessionHandler, sessionID string, claims *auth.Claims) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessionID+"/join", nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"id": sessionID})
	w := httptest.NewRecorder()
	h.JoinSession(w, req)
	return w.Code, w.Body.Bytes()
}

func TestJoinSession_ClassMember_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "js-member")
	code, _ := callJoinSession(t, fx.h, fx.sessionID, &auth.Claims{UserID: fx.student.ID, Name: "Student"})
	assert.Equal(t, http.StatusOK, code)
}

func TestJoinSession_Outsider_403(t *testing.T) {
	fx := newSessionPageFixture(t, "js-outsider")
	code, _ := callJoinSession(t, fx.h, fx.sessionID, &auth.Claims{UserID: fx.outsider.ID})
	assert.Equal(t, http.StatusForbidden, code)
}

func TestJoinSession_PlatformAdmin_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "js-admin")
	code, _ := callJoinSession(t, fx.h, fx.sessionID, &auth.Claims{
		UserID: fx.admin.ID, IsPlatformAdmin: true, Name: "Admin",
	})
	assert.Equal(t, http.StatusOK, code)
}

func TestJoinSession_AdminImpersonatingOutsider_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "js-imp")
	code, _ := callJoinSession(t, fx.h, fx.sessionID, &auth.Claims{
		UserID:         fx.outsider.ID,
		ImpersonatedBy: fx.admin.ID,
	})
	assert.Equal(t, http.StatusOK, code)
}

// Pre-invited via AddParticipant (status=invited) → should be allowed.
func TestJoinSession_PreInvited_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "js-invited")
	ctx := context.Background()
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.outsider.ID, fx.teacher.ID)
	require.NoError(t, err)

	code, _ := callJoinSession(t, fx.h, fx.sessionID, &auth.Claims{UserID: fx.outsider.ID})
	assert.Equal(t, http.StatusOK, code)
}

// status='left' must NOT grant re-entry (Codex correction #2).
func TestJoinSession_LeftStatus_Forbidden(t *testing.T) {
	fx := newSessionPageFixture(t, "js-left")
	ctx := context.Background()
	// Teacher invites outsider, outsider joins, then leaves.
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.outsider.ID, fx.teacher.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.JoinSession(ctx, fx.sessionID, fx.outsider.ID)
	require.NoError(t, err)
	_, err = fx.h.Sessions.LeaveSession(ctx, fx.sessionID, fx.outsider.ID)
	require.NoError(t, err)

	// Re-join attempt rejected — outsider must be re-invited.
	code, _ := callJoinSession(t, fx.h, fx.sessionID, &auth.Claims{UserID: fx.outsider.ID})
	assert.Equal(t, http.StatusForbidden, code)
}

// --- GetStudentPage with pre-invitation (Task 1.2b) --------------------------

// Pre-invited non-class-member must be able to load /student-page so the
// page can run /join. Pre-043: GetStudentPage returned 403 because outsider
// isn't a class member, blocking the entire flow.
func TestGetStudentPage_PreInvitedOutsider_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "sp-invited")
	ctx := context.Background()
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.outsider.ID, fx.teacher.ID)
	require.NoError(t, err)

	code, payload := fx.callStudentPage(t, &auth.Claims{UserID: fx.outsider.ID, Email: fx.outsider.Email})
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, payload)
	assert.Equal(t, fx.sessionID, payload.Session.ID)
}

// --- ListByClass / GetActiveByClass / GetSessionTopics (Task 1.3) -----------
//
// These three endpoints used to leak data by classID alone. Test that
// outsiders are now blocked.

func TestListByClass_Outsider_404(t *testing.T) {
	fx := newSessionPageFixture(t, "lbc-outsider")
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/by-class/"+fx.classID, nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.outsider.ID}),
		map[string]string{"classId": fx.classID})
	w := httptest.NewRecorder()
	fx.h.ListByClass(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestListByClass_ClassMember_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "lbc-member")
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/by-class/"+fx.classID, nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.student.ID}),
		map[string]string{"classId": fx.classID})
	w := httptest.NewRecorder()
	fx.h.ListByClass(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetActiveByClass_Outsider_404(t *testing.T) {
	fx := newSessionPageFixture(t, "abc-outsider")
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/active/"+fx.classID, nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.outsider.ID}),
		map[string]string{"classId": fx.classID})
	w := httptest.NewRecorder()
	fx.h.GetActiveByClass(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetActiveByClass_OrgAdmin_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "abc-orgadmin")
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/active/"+fx.classID, nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.orgAdmin.ID}),
		map[string]string{"classId": fx.classID})
	w := httptest.NewRecorder()
	fx.h.GetActiveByClass(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetSessionTopics_Outsider_404(t *testing.T) {
	fx := newSessionPageFixture(t, "gst-outsider")
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+fx.sessionID+"/topics", nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.outsider.ID}),
		map[string]string{"id": fx.sessionID})
	w := httptest.NewRecorder()
	fx.h.GetSessionTopics(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetSessionTopics_ClassMember_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "gst-member")
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+fx.sessionID+"/topics", nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.student.ID}),
		map[string]string{"id": fx.sessionID})
	w := httptest.NewRecorder()
	fx.h.GetSessionTopics(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Sanity: response is JSON array (empty or populated).
	var topics []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &topics))
}
