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
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// These tests exercise ProblemHandler against a live Postgres. They are
// gated on DATABASE_URL, matching the convention in internal/store tests.

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set -- skipping integration test")
	}
	db, err := sql.Open("pgx", url)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// problemFixture is the world an integration test runs against: two orgs, a
// handful of users wired into them in various roles, plus a fully-built
// ProblemHandler.
type problemFixture struct {
	db       *sql.DB
	h        *ProblemHandler
	org1     *store.Org
	org2     *store.Org
	admin    *store.RegisteredUser // platform admin
	teacher1 *store.RegisteredUser // org1 teacher
	student1 *store.RegisteredUser // org1 student
	student2 *store.RegisteredUser // org2 student (for cross-org checks)
	outsider *store.RegisteredUser // no orgs
	courseID string
	topicID  string
	classID  string // class for attachment-grant tests
}

type problemListPayload struct {
	Items      []store.Problem `json:"items"`
	NextCursor *string         `json:"nextCursor"`
}

// newProblemFixture builds a clean-slate handler + users + orgs.
// Cleanup is registered for all rows the fixture creates.
func newProblemFixture(t *testing.T, suffix string) *problemFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()

	orgs := store.NewOrgStore(db)
	users := store.NewUserStore(db)
	courses := store.NewCourseStore(db)
	topics := store.NewTopicStore(db)
	classes := store.NewClassStore(db)

	h := &ProblemHandler{
		Problems:      store.NewProblemStore(db),
		TestCases:     store.NewTestCaseStore(db),
		Attempts:      store.NewAttemptStore(db),
		Solutions:     store.NewProblemSolutionStore(db),
		TopicProblems: store.NewTopicProblemStore(db),
		Topics:        topics,
		Courses:       courses,
		Orgs:          orgs,
	}

	mkOrg := func(label string) *store.Org {
		org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
			Name:         "Org " + label,
			Slug:         "org-" + label,
			Type:         "school",
			ContactEmail: label + "@example.com",
			ContactName:  "Admin " + label,
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
			db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
		})
		return org
	}
	mkUser := func(label string) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name:     "User " + label,
			Email:    label + "@example.com",
			Password: "testpassword123",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM attempts WHERE user_id = $1 OR problem_id IN (SELECT id FROM problems WHERE created_by = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM test_cases WHERE problem_id IN (SELECT id FROM problems WHERE created_by = $1) OR owner_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM topic_problems WHERE problem_id IN (SELECT id FROM problems WHERE created_by = $1) OR attached_by = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM problem_solutions WHERE problem_id IN (SELECT id FROM problems WHERE created_by = $1) OR created_by = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM problems WHERE forked_from IN (SELECT id FROM problems WHERE created_by = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM problems WHERE created_by = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM class_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	fx := &problemFixture{db: db, h: h}
	fx.org1 = mkOrg(suffix + "-1")
	fx.org2 = mkOrg(suffix + "-2")
	fx.admin = mkUser(suffix + "-admin")
	fx.teacher1 = mkUser(suffix + "-teacher1")
	fx.student1 = mkUser(suffix + "-student1")
	fx.student2 = mkUser(suffix + "-student2")
	fx.outsider = mkUser(suffix + "-outsider")

	addMember := func(org *store.Org, userID, role string) {
		_, err := orgs.AddOrgMember(ctx, store.AddMemberInput{
			OrgID: org.ID, UserID: userID, Role: role, Status: "active",
		})
		require.NoError(t, err)
	}
	addMember(fx.org1, fx.teacher1.ID, "teacher")
	addMember(fx.org1, fx.student1.ID, "student")
	addMember(fx.org2, fx.student2.ID, "student")

	// Build a course + topic + class, owned by teacher1 in org1.
	course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID:      fx.org1.ID,
		CreatedBy:  fx.teacher1.ID,
		Title:      "Course " + suffix,
		GradeLevel: "K-5",
		Language:   "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })
	fx.courseID = course.ID

	topic, err := topics.CreateTopic(ctx, store.CreateTopicInput{
		CourseID: course.ID,
		Title:    "Arrays",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID) })
	fx.topicID = topic.ID

	// Create a class so student1 can gain attachment-grant access.
	// CreateClass auto-inserts the creator as instructor and generates a
	// join code, so we only need to add student1 explicitly.
	class, err := classes.CreateClass(ctx, store.CreateClassInput{
		CourseID: course.ID, OrgID: fx.org1.ID,
		Title: "Section A " + suffix, Term: "Fall",
		CreatedBy: fx.teacher1.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})
	fx.classID = class.ID
	_, err = classes.AddClassMember(ctx, store.AddClassMemberInput{
		ClassID: class.ID, UserID: fx.student1.ID, Role: "student",
	})
	require.NoError(t, err)

	return fx
}

func (fx *problemFixture) claims(u *store.RegisteredUser, platformAdmin bool) *auth.Claims {
	return &auth.Claims{UserID: u.ID, Email: u.Email, Name: u.Name, IsPlatformAdmin: platformAdmin}
}

// mkProblem creates a problem in the fixture with given scope/status and
// returns the row. CreatedBy defaults to the first non-nil of
// "scope owner if personal", "teacher1 if org1", "student2 if org2",
// "admin if platform". An optional overrideCreatedBy can be passed to
// force a specific creator.
func (fx *problemFixture) mkProblem(
	t *testing.T, scope string, scopeID *string, status, title string, overrideCreatedBy ...string,
) *store.Problem {
	t.Helper()
	ctx := context.Background()
	createdBy := fx.admin.ID
	if scope == "personal" && scopeID != nil {
		createdBy = *scopeID
	} else if scope == "org" {
		if scopeID != nil && *scopeID == fx.org2.ID {
			createdBy = fx.student2.ID
		} else {
			createdBy = fx.teacher1.ID
		}
	}
	if len(overrideCreatedBy) > 0 && overrideCreatedBy[0] != "" {
		createdBy = overrideCreatedBy[0]
	}
	p, err := fx.h.Problems.CreateProblem(ctx, store.CreateProblemInput{
		Scope: scope, ScopeID: scopeID, Title: title,
		Description: "desc", Status: status, CreatedBy: createdBy,
	})
	require.NoError(t, err)
	require.NotNil(t, p)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", p.ID) })
	return p
}

// doGet issues an authed GET against handler h.
func doGet(t *testing.T, h http.HandlerFunc, path string, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if claims != nil {
		req = withClaims(req, claims)
	}
	if params != nil {
		req = withChiParams(req, params)
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func doPostJSON(t *testing.T, h http.HandlerFunc, path string, body any, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	if claims != nil {
		req = withClaims(req, claims)
	}
	if params != nil {
		req = withChiParams(req, params)
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func doDelete(t *testing.T, h http.HandlerFunc, path string, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	if claims != nil {
		req = withClaims(req, claims)
	}
	if params != nil {
		req = withChiParams(req, params)
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

// ------------------- View access matrix -------------------

func TestProblemHandler_View_PlatformPublished_AnyoneViews(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "Global")
	for _, u := range []*store.RegisteredUser{fx.teacher1, fx.student1, fx.student2, fx.outsider} {
		w := doGet(t, fx.h.GetProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(u, false))
		assert.Equal(t, http.StatusOK, w.Code, "user %s should see published platform problem", u.Email)
	}
}

func TestProblemHandler_View_PlatformDraft_NonAdmin404(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "draft", "Draft Global")
	w := doGet(t, fx.h.GetProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProblemHandler_View_PlatformDraft_AdminCan(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "draft", "Draft Global")
	w := doGet(t, fx.h.GetProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProblemHandler_View_OrgPublished_SameOrg(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "Org1 Pub")
	w := doGet(t, fx.h.GetProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProblemHandler_View_OrgPublished_OtherOrg404(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "Org1 Pub")
	w := doGet(t, fx.h.GetProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.student2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProblemHandler_View_OrgDraft_TeacherCan(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "Org1 Draft")
	w := doGet(t, fx.h.GetProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProblemHandler_View_OrgDraft_StudentCant(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "Org1 Draft")
	w := doGet(t, fx.h.GetProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProblemHandler_View_Personal_OwnerCan(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	uid := fx.student1.ID
	p := fx.mkProblem(t, "personal", &uid, "draft", "Mine")
	w := doGet(t, fx.h.GetProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProblemHandler_View_Personal_OtherCant(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	uid := fx.student1.ID
	p := fx.mkProblem(t, "personal", &uid, "draft", "Mine")
	w := doGet(t, fx.h.GetProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.student2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProblemHandler_View_AttachmentGrant_ClassMember(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	// Platform-published so attachment is not strictly needed — but the goal
	// here is to verify attachment grant works for a problem that would
	// otherwise be hidden. Use a personal-scope problem owned by someone
	// else, then attach it to the course's topic.
	otherUID := fx.outsider.ID
	p := fx.mkProblem(t, "personal", &otherUID, "published", "Shared via topic")
	ctx := context.Background()
	_, err := fx.h.TopicProblems.Attach(ctx, fx.topicID, p.ID, 0, fx.teacher1.ID)
	require.NoError(t, err)
	// student1 is a class member of the class under topic's course.
	w := doGet(t, fx.h.GetProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusOK, w.Code, "attachment grants view through class membership")
}

// ------------------- CreateProblem auth matrix -------------------

func TestProblemHandler_Create_Platform_AdminOK(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	body := map[string]any{"scope": "platform", "title": "P"}
	w := doPostJSON(t, fx.h.CreateProblem, "/api/problems", body, nil, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestProblemHandler_Create_Platform_NonAdmin403(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	body := map[string]any{"scope": "platform", "title": "P"}
	w := doPostJSON(t, fx.h.CreateProblem, "/api/problems", body, nil, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestProblemHandler_Create_Org_TeacherOK(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	body := map[string]any{"scope": "org", "scopeId": fx.org1.ID, "title": "P"}
	w := doPostJSON(t, fx.h.CreateProblem, "/api/problems", body, nil, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestProblemHandler_Create_Org_Student403(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	body := map[string]any{"scope": "org", "scopeId": fx.org1.ID, "title": "P"}
	w := doPostJSON(t, fx.h.CreateProblem, "/api/problems", body, nil, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestProblemHandler_Create_Org_Outsider403(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	body := map[string]any{"scope": "org", "scopeId": fx.org1.ID, "title": "P"}
	w := doPostJSON(t, fx.h.CreateProblem, "/api/problems", body, nil, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestProblemHandler_Create_Personal_SelfOK(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	body := map[string]any{"scope": "personal", "scopeId": fx.student1.ID, "title": "P"}
	w := doPostJSON(t, fx.h.CreateProblem, "/api/problems", body, nil, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestProblemHandler_Create_Personal_OtherID403(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	body := map[string]any{"scope": "personal", "scopeId": fx.student2.ID, "title": "P"}
	w := doPostJSON(t, fx.h.CreateProblem, "/api/problems", body, nil, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestProblemHandler_Create_InvalidScope400(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	body := map[string]any{"scope": "garbage", "title": "P"}
	w := doPostJSON(t, fx.h.CreateProblem, "/api/problems", body, nil, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProblemHandler_Create_EmptyTitle400(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	body := map[string]any{"scope": "platform", "title": ""}
	w := doPostJSON(t, fx.h.CreateProblem, "/api/problems", body, nil, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ------------------- Publish / Archive / Unarchive -------------------

func TestProblemHandler_Publish_DraftToPublished(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "D")
	w := doPostJSON(t, fx.h.PublishProblem, "/api/problems/"+p.ID+"/publish", map[string]any{}, map[string]string{"id": p.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProblemHandler_Publish_TwiceReturns409(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "D")
	w := doPostJSON(t, fx.h.PublishProblem, "/api/problems/"+p.ID+"/publish", map[string]any{}, map[string]string{"id": p.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)
	w = doPostJSON(t, fx.h.PublishProblem, "/api/problems/"+p.ID+"/publish", map[string]any{}, map[string]string{"id": p.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestProblemHandler_Publish_StudentForbidden(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "D")
	w := doPostJSON(t, fx.h.PublishProblem, "/api/problems/"+p.ID+"/publish", map[string]any{}, map[string]string{"id": p.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestProblemHandler_Archive_PublishedToArchived(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "P")
	w := doPostJSON(t, fx.h.ArchiveProblem, "/api/problems/"+p.ID+"/archive", map[string]any{}, map[string]string{"id": p.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProblemHandler_Archive_FromDraft409(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "D")
	w := doPostJSON(t, fx.h.ArchiveProblem, "/api/problems/"+p.ID+"/archive", map[string]any{}, map[string]string{"id": p.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestProblemHandler_Unarchive_ArchivedToPublished(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "archived", "A")
	w := doPostJSON(t, fx.h.UnarchiveProblem, "/api/problems/"+p.ID+"/unarchive", map[string]any{}, map[string]string{"id": p.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

// ------------------- ForkProblem -------------------

func TestProblemHandler_Fork_DefaultsToSingleOrg(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	// Source: platform published — teacher1 can view.
	p := fx.mkProblem(t, "platform", nil, "published", "Src")
	w := doPostJSON(t, fx.h.ForkProblem, "/api/problems/"+p.ID+"/fork", map[string]any{}, map[string]string{"id": p.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	var forked store.Problem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &forked))
	assert.Equal(t, "org", forked.Scope, "default target for single-org caller is org")
	require.NotNil(t, forked.ScopeID)
	assert.Equal(t, fx.org1.ID, *forked.ScopeID)
}

func TestProblemHandler_Fork_DefaultsToPersonalForZeroOrg(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "Src")
	w := doPostJSON(t, fx.h.ForkProblem, "/api/problems/"+p.ID+"/fork", map[string]any{}, map[string]string{"id": p.ID}, fx.claims(fx.outsider, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	var forked store.Problem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &forked))
	assert.Equal(t, "personal", forked.Scope)
	require.NotNil(t, forked.ScopeID)
	assert.Equal(t, fx.outsider.ID, *forked.ScopeID)
}

func TestProblemHandler_Fork_Unviewable404(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	// Source is a draft in org1 — invisible to student2.
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "Hidden")
	w := doPostJSON(t, fx.h.ForkProblem, "/api/problems/"+p.ID+"/fork", map[string]any{}, map[string]string{"id": p.ID}, fx.claims(fx.student2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProblemHandler_Fork_WithExplicitTargetUnauthorized403(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "Src")
	// outsider tries to fork to org1 — not a member.
	body := map[string]any{"targetScope": "org", "targetScopeId": fx.org1.ID}
	w := doPostJSON(t, fx.h.ForkProblem, "/api/problems/"+p.ID+"/fork", body, map[string]string{"id": p.ID}, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ------------------- DeleteProblem guards -------------------

func TestProblemHandler_Delete_Clean_204(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "personal", &fx.student1.ID, "draft", "Mine")
	w := doDelete(t, fx.h.DeleteProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestProblemHandler_Delete_WithAttempts_409(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "personal", &fx.student1.ID, "published", "Mine")
	ctx := context.Background()
	_, err := fx.h.Attempts.CreateAttempt(ctx, store.CreateAttemptInput{
		ProblemID: p.ID, UserID: fx.student1.ID, Language: "python",
	})
	require.NoError(t, err)
	w := doDelete(t, fx.h.DeleteProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "attempts")
}

func TestProblemHandler_Delete_Attached_409(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "Attached")
	ctx := context.Background()
	_, err := fx.h.TopicProblems.Attach(ctx, fx.topicID, p.ID, 0, fx.teacher1.ID)
	require.NoError(t, err)
	w := doDelete(t, fx.h.DeleteProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "attached")
}

func TestProblemHandler_Delete_Unauthorized403(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "T")
	w := doDelete(t, fx.h.DeleteProblem, "/api/problems/"+p.ID, map[string]string{"id": p.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ------------------- ListProblems -------------------

func TestProblemHandler_List_AccessibleDefault(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	// Platform-published → visible; org1-published → visible to teacher1;
	// org2-draft → hidden.
	pubPlat := fx.mkProblem(t, "platform", nil, "published", "Plat")
	pubOrg1 := fx.mkProblem(t, "org", &fx.org1.ID, "published", "Org1Pub")
	hidden := fx.mkProblem(t, "org", &fx.org2.ID, "draft", "Org2Draft")

	w := doGet(t, fx.h.ListProblems, "/api/problems", nil, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)
	var payload problemListPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	ids := map[string]bool{}
	for _, p := range payload.Items {
		ids[p.ID] = true
	}
	assert.True(t, ids[pubPlat.ID])
	assert.True(t, ids[pubOrg1.ID])
	assert.False(t, ids[hidden.ID], "org2-draft must not leak to teacher1")
}

func TestProblemHandler_List_AttachmentGrantVisibleInBrowse(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	otherUID := fx.outsider.ID
	p := fx.mkProblem(t, "personal", &otherUID, "published", "Attached Personal")
	ctx := context.Background()
	_, err := fx.h.TopicProblems.Attach(ctx, fx.topicID, p.ID, 0, fx.teacher1.ID)
	require.NoError(t, err)

	w := doGet(t, fx.h.ListProblems, "/api/problems", nil, fx.claims(fx.student1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var payload problemListPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	found := false
	for _, item := range payload.Items {
		if item.ID == p.ID {
			found = true
		}
	}
	assert.True(t, found, "attached problem should appear in browse/search results")
}

func TestProblemHandler_List_DefaultExcludesArchived(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	archived := fx.mkProblem(t, "personal", &fx.student1.ID, "archived", "Archived")
	published := fx.mkProblem(t, "personal", &fx.student1.ID, "published", "Published")

	w := doGet(t, fx.h.ListProblems, "/api/problems", nil, fx.claims(fx.student1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var payload problemListPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	ids := map[string]bool{}
	for _, item := range payload.Items {
		ids[item.ID] = true
	}
	assert.True(t, ids[published.ID])
	assert.False(t, ids[archived.ID], "archived rows should stay out of default browse/search")
}

func TestProblemHandler_List_PaginationReturnsNextCursor(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p1 := fx.mkProblem(t, "platform", nil, "published", "P1")
	p2 := fx.mkProblem(t, "platform", nil, "published", "P2")
	p3 := fx.mkProblem(t, "platform", nil, "published", "P3")

	first := doGet(t, fx.h.ListProblems, "/api/problems?limit=2", nil, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, first.Code)

	var firstPayload problemListPayload
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstPayload))
	require.Len(t, firstPayload.Items, 2)
	require.NotNil(t, firstPayload.NextCursor, "page 1 should advertise a next cursor")

	second := doGet(t, fx.h.ListProblems, "/api/problems?limit=2&cursor="+*firstPayload.NextCursor, nil, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, second.Code)

	var secondPayload problemListPayload
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondPayload))
	require.Len(t, secondPayload.Items, 1)
	assert.Nil(t, secondPayload.NextCursor)

	seen := map[string]bool{}
	for _, item := range append(firstPayload.Items, secondPayload.Items...) {
		seen[item.ID] = true
	}
	assert.True(t, seen[p1.ID])
	assert.True(t, seen[p2.ID])
	assert.True(t, seen[p3.ID])
}

func TestProblemHandler_List_ScopeFilterInvalid400(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	w := doGet(t, fx.h.ListProblems, "/api/problems?scope=garbage", nil, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProblemHandler_List_InvalidLimit400(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	w := doGet(t, fx.h.ListProblems, "/api/problems?limit=notanumber", nil, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ------------------- ListProblemsByTopic -------------------

func TestProblemHandler_ListByTopic_MemberCanView(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "T")
	ctx := context.Background()
	_, err := fx.h.TopicProblems.Attach(ctx, fx.topicID, p.ID, 0, fx.teacher1.ID)
	require.NoError(t, err)
	w := doGet(t, fx.h.ListProblemsByTopic, fmt.Sprintf("/api/topics/%s/problems", fx.topicID),
		map[string]string{"topicId": fx.topicID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProblemHandler_ListByTopic_NonMember404(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	w := doGet(t, fx.h.ListProblemsByTopic, fmt.Sprintf("/api/topics/%s/problems", fx.topicID),
		map[string]string{"topicId": fx.topicID}, fx.claims(fx.outsider, false))
	// canViewTopic returns false + status=0 for non-members → 403.
	// The topic and course both exist, so no 404 case here; 403 is the answer.
	assert.Equal(t, http.StatusForbidden, w.Code)
}
