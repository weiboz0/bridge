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

// Pre-invited via AddParticipant (status=invited) → should be allowed AND
// the row's status should flip from 'invited' to 'present' with joined_at
// populated. Codex post-impl review caught that the original ON CONFLICT
// DO NOTHING left pre-invited rows stuck at 'invited' so the teacher's
// roster never reflected the actual join.
func TestJoinSession_PreInvited_Allowed(t *testing.T) {
	fx := newSessionPageFixture(t, "js-invited")
	ctx := context.Background()
	_, err := fx.h.Sessions.AddParticipant(ctx, fx.sessionID, fx.outsider.ID, fx.teacher.ID)
	require.NoError(t, err)

	code, _ := callJoinSession(t, fx.h, fx.sessionID, &auth.Claims{UserID: fx.outsider.ID})
	assert.Equal(t, http.StatusOK, code)

	// Status must now be 'present' and joined_at populated.
	row, err := fx.h.Sessions.GetSessionParticipant(ctx, fx.sessionID, fx.outsider.ID)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "present", row.Status, "pre-invited row should flip to present after join")
	assert.NotNil(t, row.JoinedAt, "joined_at must be set after the join transition")
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

// Plan 044 phase 1: GetSessionTopics surfaces unitId/unitTitle/
// unitMaterialType when a teaching_unit is linked to the topic via
// teaching_units.topic_id (and the unit's scope_id matches the topic's
// course org_id).
func TestGetSessionTopics_LinkedUnit_AppearsInResponse(t *testing.T) {
	fx := newSessionPageFixture(t, "gst-unit")
	ctx := context.Background()

	// Create a topic in fx's course (already wired by the fixture).
	courses := store.NewCourseStore(fx.db)
	course, err := courses.GetCourse(ctx, fx.courseID)
	require.NoError(t, err)
	require.NotNil(t, course)

	topics := store.NewTopicStore(fx.db)
	topic, err := topics.CreateTopic(ctx, store.CreateTopicInput{
		CourseID: fx.courseID,
		Title:    "Loops",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID)
	})

	// Link the topic to the session.
	_, err = fx.db.ExecContext(ctx,
		"INSERT INTO session_topics (session_id, topic_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		fx.sessionID, topic.ID,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM session_topics WHERE session_id = $1", fx.sessionID)
	})

	// Create a teaching_unit in the same org, linked to the topic.
	var unitID string
	err = fx.db.QueryRowContext(ctx,
		`INSERT INTO teaching_units
		 (id, scope, scope_id, title, summary, material_type, status, topic_id, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'org', $1, 'Loops Unit', '', 'slides', 'draft', $2, $3, now(), now())
		 RETURNING id`,
		fx.orgID, topic.ID, fx.teacher.ID,
	).Scan(&unitID)
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", unitID)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+fx.sessionID+"/topics", nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.student.ID}),
		map[string]string{"id": fx.sessionID})
	w := httptest.NewRecorder()
	fx.h.GetSessionTopics(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var topicRows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &topicRows))
	require.Len(t, topicRows, 1)
	assert.Equal(t, topic.ID, topicRows[0]["topicId"])
	assert.Equal(t, unitID, topicRows[0]["unitId"])
	assert.Equal(t, "Loops Unit", topicRows[0]["unitTitle"])
	assert.Equal(t, "slides", topicRows[0]["unitMaterialType"])
}

// Cross-org leak guard: a Unit whose scope_id mismatches the topic's
// course org_id must NOT surface in the response.
func TestGetSessionTopics_CrossOrgUnit_NotSurfaced(t *testing.T) {
	fx := newSessionPageFixture(t, "gst-cross")
	ctx := context.Background()

	topics := store.NewTopicStore(fx.db)
	topic, err := topics.CreateTopic(ctx, store.CreateTopicInput{
		CourseID: fx.courseID,
		Title:    "Loops",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID)
	})

	_, err = fx.db.ExecContext(ctx,
		"INSERT INTO session_topics (session_id, topic_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		fx.sessionID, topic.ID,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM session_topics WHERE session_id = $1", fx.sessionID)
	})

	// Create a different org and a Unit scoped to it, but linked to fx's topic.
	orgs := store.NewOrgStore(fx.db)
	otherOrg, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name: "Other Org", Slug: "other-" + topic.ID[:6], Type: "school",
		ContactEmail: "x@example.com", ContactName: "X",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", otherOrg.ID)
	})

	var leakedUnitID string
	err = fx.db.QueryRowContext(ctx,
		`INSERT INTO teaching_units
		 (id, scope, scope_id, title, summary, material_type, status, topic_id, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'org', $1, 'Leaked', '', 'notes', 'draft', $2, $3, now(), now())
		 RETURNING id`,
		otherOrg.ID, topic.ID, fx.teacher.ID,
	).Scan(&leakedUnitID)
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", leakedUnitID)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+fx.sessionID+"/topics", nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.student.ID}),
		map[string]string{"id": fx.sessionID})
	w := httptest.NewRecorder()
	fx.h.GetSessionTopics(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var topicRows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &topicRows))
	require.Len(t, topicRows, 1)
	// Unit should NOT surface — scope_id is the other org.
	assert.Nil(t, topicRows[0]["unitId"])
	assert.Nil(t, topicRows[0]["unitTitle"])
}
