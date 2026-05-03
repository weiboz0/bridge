package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// sessionPageFixture spins up the world for testing the consolidated
// teacher-page / student-page endpoints. Uses the same DATABASE_URL
// gating as other handler integration tests.
type sessionPageFixture struct {
	db        *sql.DB
	h         *SessionHandler
	teacher   *store.RegisteredUser
	student   *store.RegisteredUser
	outsider  *store.RegisteredUser
	admin     *store.RegisteredUser
	orgAdmin  *store.RegisteredUser
	orgID     string
	courseID  string
	classID   string
	sessionID string
}

func newSessionPageFixture(t *testing.T, suffix string) *sessionPageFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()

	orgs := store.NewOrgStore(db)
	users := store.NewUserStore(db)
	courses := store.NewCourseStore(db)
	topics := store.NewTopicStore(db)
	classes := store.NewClassStore(db)
	sessions := store.NewSessionStore(db)
	units := store.NewTeachingUnitStore(db)

	h := &SessionHandler{
		Sessions:      sessions,
		Classes:       classes,
		Courses:       courses,
		Topics:        topics,
		TeachingUnits: units,
		Orgs:          orgs,
		ParentLinks:   store.NewParentLinkStore(db), // Plan 064.
		Broadcaster:   events.NewBroadcaster(),
	}

	mkUser := func(label string) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name:     "User " + label,
			Email:    label + "@example.com",
			Password: "testpassword123",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM session_participants WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM class_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	fx := &sessionPageFixture{db: db, h: h}
	fx.teacher = mkUser(suffix + "-teacher")
	fx.student = mkUser(suffix + "-student")
	fx.outsider = mkUser(suffix + "-outsider")
	fx.admin = mkUser(suffix + "-admin")
	fx.orgAdmin = mkUser(suffix + "-orgadmin")

	org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name:         "Org " + suffix,
		Slug:         "spo-" + suffix,
		Type:         "school",
		ContactEmail: suffix + "@example.com",
		ContactName:  "Admin " + suffix,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})
	fx.orgID = org.ID

	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: org.ID, UserID: fx.teacher.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)
	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: org.ID, UserID: fx.student.ID, Role: "student", Status: "active",
	})
	require.NoError(t, err)
	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: org.ID, UserID: fx.orgAdmin.ID, Role: "org_admin", Status: "active",
	})
	require.NoError(t, err)

	course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID:      org.ID,
		CreatedBy:  fx.teacher.ID,
		Title:      "Course " + suffix,
		GradeLevel: "K-5",
		Language:   "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })
	fx.courseID = course.ID

	loopsTopic, err := topics.CreateTopic(ctx, store.CreateTopicInput{
		CourseID: course.ID,
		Title:    "Loops",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", course.ID) })

	class, err := classes.CreateClass(ctx, store.CreateClassInput{
		CourseID:  course.ID,
		OrgID:     org.ID,
		Title:     "Class " + suffix,
		Term:      "fall",
		CreatedBy: fx.teacher.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})
	fx.classID = class.ID

	_, err = classes.AddClassMember(ctx, store.AddClassMemberInput{
		ClassID: class.ID, UserID: fx.student.ID, Role: "student",
	})
	require.NoError(t, err)

	classID := class.ID
	// Plan 048 phase 1: snapshot the course's topic into session_topics
	// so the teacher-page payload can read the agenda from the same
	// place the student page does.
	session, err := sessions.CreateSession(ctx, store.CreateSessionInput{
		ClassID:   &classID,
		TeacherID: fx.teacher.ID,
		Title:     "Session " + suffix,
		TopicIDs:  []string{loopsTopic.ID},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1", session.ID)
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE id = $1", session.ID)
	})
	fx.sessionID = session.ID

	return fx
}

func (fx *sessionPageFixture) callTeacherPage(t *testing.T, claims *auth.Claims) (int, *teacherPagePayload) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+fx.sessionID+"/teacher-page", nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"id": fx.sessionID})
	w := httptest.NewRecorder()
	fx.h.GetTeacherPage(w, req)
	if w.Code != http.StatusOK {
		return w.Code, nil
	}
	var payload teacherPagePayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	return w.Code, &payload
}

func (fx *sessionPageFixture) callStudentPage(t *testing.T, claims *auth.Claims) (int, *studentPagePayload) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+fx.sessionID+"/student-page", nil)
	req = withChiParams(withClaims(req, claims), map[string]string{"id": fx.sessionID})
	w := httptest.NewRecorder()
	fx.h.GetStudentPage(w, req)
	if w.Code != http.StatusOK {
		return w.Code, nil
	}
	var payload studentPagePayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	return w.Code, &payload
}

func TestGetTeacherPage_Teacher(t *testing.T) {
	fx := newSessionPageFixture(t, "tp-teacher")
	code, payload := fx.callTeacherPage(t, &auth.Claims{UserID: fx.teacher.ID, Email: fx.teacher.Email, Name: fx.teacher.Name})
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, payload)
	assert.Equal(t, fx.sessionID, payload.Session.ID)
	require.NotNil(t, payload.ClassID)
	assert.Equal(t, fx.classID, *payload.ClassID)
	assert.Equal(t, "/teacher/classes/"+fx.classID, payload.ReturnPath)
	assert.Equal(t, "python", payload.EditorMode)
	assert.GreaterOrEqual(t, len(payload.CourseTopics), 1)
}

func TestGetTeacherPage_PlatformAdmin(t *testing.T) {
	fx := newSessionPageFixture(t, "tp-admin")
	code, payload := fx.callTeacherPage(t, &auth.Claims{
		UserID: fx.admin.ID, Email: fx.admin.Email, Name: fx.admin.Name, IsPlatformAdmin: true,
	})
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, payload)
	assert.Equal(t, fx.sessionID, payload.Session.ID)
}

// Per CLAUDE.md / plan 039 corrections: middleware impersonation rewrites
// claims (UserID becomes target, IsPlatformAdmin cleared, ImpersonatedBy set).
// We preserve admin equivalence by checking ImpersonatedBy != "" — admin can
// still see the teacher dashboard while impersonating any user.
func TestGetTeacherPage_AdminImpersonating(t *testing.T) {
	fx := newSessionPageFixture(t, "tp-imp")
	// Admin impersonating the outsider (a non-teacher, non-class-member).
	code, payload := fx.callTeacherPage(t, &auth.Claims{
		UserID:          fx.outsider.ID,
		Email:           fx.outsider.Email,
		Name:            fx.outsider.Name,
		IsPlatformAdmin: false,
		ImpersonatedBy:  fx.admin.ID,
	})
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, payload)
}

func TestGetTeacherPage_Student_Forbidden(t *testing.T) {
	fx := newSessionPageFixture(t, "tp-student")
	code, _ := fx.callTeacherPage(t, &auth.Claims{UserID: fx.student.ID, Email: fx.student.Email})
	assert.Equal(t, http.StatusForbidden, code)
}

func TestGetTeacherPage_Outsider_Forbidden(t *testing.T) {
	fx := newSessionPageFixture(t, "tp-outsider")
	code, _ := fx.callTeacherPage(t, &auth.Claims{UserID: fx.outsider.ID, Email: fx.outsider.Email})
	assert.Equal(t, http.StatusForbidden, code)
}

// Org admin for the class's org should see the teacher dashboard (matches
// the legacy isSessionAuthority semantics in sessions.go). Codex review of
// 039 flagged this branch as untested — fix.
func TestGetTeacherPage_OrgAdmin(t *testing.T) {
	fx := newSessionPageFixture(t, "tp-orgadmin")
	code, payload := fx.callTeacherPage(t, &auth.Claims{
		UserID: fx.orgAdmin.ID, Email: fx.orgAdmin.Email, Name: fx.orgAdmin.Name,
	})
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, payload)
	assert.Equal(t, fx.sessionID, payload.Session.ID)
}

// Plan 048 phase 1: GetTeacherPage reads the agenda from session_topics
// (the same source the student page uses), not from class.course.topics.
// This test inserts an extra course topic that is NOT in session_topics
// and asserts the teacher payload does NOT include it. It also unlinks
// the existing session_topics row and asserts the payload becomes empty
// — proving the read truly comes from session_topics, not from
// class.course.topics.
func TestGetTeacherPage_ReadsFromSessionTopics(t *testing.T) {
	fx := newSessionPageFixture(t, "tp-snap")
	ctx := context.Background()

	// The fixture created one topic + one session_topics row. Add a
	// SECOND course topic but DON'T put it in session_topics — pre-048
	// this would have shown up in the payload. Post-048 it must NOT.
	var notSnapshottedID string
	err := fx.db.QueryRowContext(ctx,
		`INSERT INTO topics (id, course_id, title, sort_order, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, 'Not Snapshotted', 1, now(), now())
		 RETURNING id`,
		fx.courseID,
	).Scan(&notSnapshottedID)
	require.NoError(t, err)

	code, payload := fx.callTeacherPage(t,
		&auth.Claims{UserID: fx.teacher.ID, Email: fx.teacher.Email, Name: fx.teacher.Name})
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, payload)
	assert.Len(t, payload.CourseTopics, 1, "agenda must come from session_topics, not class.course.topics")
	for _, topic := range payload.CourseTopics {
		assert.NotEqual(t, notSnapshottedID, topic.TopicID,
			"topic that's only on the course (not on the session) must not appear")
	}

	// Unlink the only session_topics row. Payload should now be empty.
	_, err = fx.db.ExecContext(ctx,
		"DELETE FROM session_topics WHERE session_id = $1",
		fx.sessionID,
	)
	require.NoError(t, err)

	code, payload = fx.callTeacherPage(t,
		&auth.Claims{UserID: fx.teacher.ID, Email: fx.teacher.Email, Name: fx.teacher.Name})
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, payload)
	assert.Empty(t, payload.CourseTopics, "session with no session_topics must return empty agenda")
}

func TestGetTeacherPage_MissingSession_404(t *testing.T) {
	fx := newSessionPageFixture(t, "tp-missing")
	bogusID := "00000000-0000-0000-0000-000000000099"
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+bogusID+"/teacher-page", nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.teacher.ID, IsPlatformAdmin: true}),
		map[string]string{"id": bogusID})
	w := httptest.NewRecorder()
	fx.h.GetTeacherPage(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetStudentPage_EnrolledStudent(t *testing.T) {
	fx := newSessionPageFixture(t, "sp-student")
	code, payload := fx.callStudentPage(t, &auth.Claims{UserID: fx.student.ID, Email: fx.student.Email})
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, payload)
	assert.Equal(t, fx.sessionID, payload.Session.ID)
	require.NotNil(t, payload.ClassID)
	assert.Equal(t, fx.classID, *payload.ClassID)
	assert.Equal(t, "/student/classes/"+fx.classID, payload.ReturnPath)
	assert.Equal(t, "python", payload.EditorMode)
}

func TestGetStudentPage_Teacher(t *testing.T) {
	// The teacher counts as session authority — should be able to load
	// the student-page payload too (used for impersonating-student review).
	fx := newSessionPageFixture(t, "sp-teacher")
	code, _ := fx.callStudentPage(t, &auth.Claims{UserID: fx.teacher.ID, Email: fx.teacher.Email})
	assert.Equal(t, http.StatusOK, code)
}

func TestGetStudentPage_Outsider_Forbidden(t *testing.T) {
	fx := newSessionPageFixture(t, "sp-outsider")
	code, _ := fx.callStudentPage(t, &auth.Claims{UserID: fx.outsider.ID, Email: fx.outsider.Email})
	assert.Equal(t, http.StatusForbidden, code)
}

func TestGetStudentPage_PlatformAdmin(t *testing.T) {
	fx := newSessionPageFixture(t, "sp-admin")
	code, _ := fx.callStudentPage(t, &auth.Claims{
		UserID: fx.admin.ID, Email: fx.admin.Email, IsPlatformAdmin: true,
	})
	assert.Equal(t, http.StatusOK, code)
}

func TestGetStudentPage_AdminImpersonating(t *testing.T) {
	fx := newSessionPageFixture(t, "sp-imp")
	code, _ := fx.callStudentPage(t, &auth.Claims{
		UserID:          fx.outsider.ID,
		Email:           fx.outsider.Email,
		IsPlatformAdmin: false,
		ImpersonatedBy:  fx.admin.ID,
	})
	assert.Equal(t, http.StatusOK, code)
}

func TestGetStudentPage_EndedSession_404(t *testing.T) {
	fx := newSessionPageFixture(t, "sp-ended")
	ctx := context.Background()
	_, err := fx.h.Sessions.EndSession(ctx, fx.sessionID)
	require.NoError(t, err)

	code, _ := fx.callStudentPage(t, &auth.Claims{UserID: fx.student.ID, Email: fx.student.Email})
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetStudentPage_MissingSession_404(t *testing.T) {
	fx := newSessionPageFixture(t, "sp-missing")
	bogusID := "00000000-0000-0000-0000-000000000098"
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+bogusID+"/student-page", nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.student.ID}),
		map[string]string{"id": bogusID})
	w := httptest.NewRecorder()
	fx.h.GetStudentPage(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
