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
	"github.com/weiboz0/bridge/platform/internal/store"
)

// orgListFixture is the world for testing the read-only org list endpoints.
// One org with one teacher, one student, one outside-org user, one admin,
// plus a course and a class with two members so the counts query has
// something to count.
type orgListFixture struct {
	db       *sql.DB
	h        *OrgDashboardHandler
	teacher  *store.RegisteredUser
	student  *store.RegisteredUser
	outsider *store.RegisteredUser
	admin    *store.RegisteredUser
	orgAdmin *store.RegisteredUser
	orgID    string
	courseID string
	classID  string
}

func newOrgListFixture(t *testing.T, suffix string) *orgListFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()

	orgs := store.NewOrgStore(db)
	users := store.NewUserStore(db)
	courses := store.NewCourseStore(db)
	classes := store.NewClassStore(db)
	stats := store.NewStatsStore(db)

	h := &OrgDashboardHandler{
		Orgs:    orgs,
		Courses: courses,
		Classes: classes,
		Stats:   stats,
	}

	mkUser := func(label string) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name:     "User " + label,
			Email:    label + "@example.com",
			Password: "testpassword123",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM class_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	fx := &orgListFixture{db: db, h: h}
	fx.teacher = mkUser(suffix + "-teacher")
	fx.student = mkUser(suffix + "-student")
	fx.outsider = mkUser(suffix + "-outsider")
	fx.admin = mkUser(suffix + "-admin")
	fx.orgAdmin = mkUser(suffix + "-orgadmin")

	org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name:         "OrgList " + suffix,
		Slug:         "orglist-" + suffix,
		Type:         "school",
		ContactEmail: suffix + "@example.com",
		ContactName:  "Admin " + suffix,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})
	// CreateOrg defaults to status='pending'. Activate so it resembles a
	// real running org (org_status='active' is required by the
	// authorizeOrgAdmin helper for missing-orgId resolution).
	_, err = orgs.UpdateOrgStatus(ctx, org.ID, "active")
	require.NoError(t, err)
	fx.orgID = org.ID

	for _, m := range []struct {
		userID, role string
	}{
		{fx.teacher.ID, "teacher"},
		{fx.student.ID, "student"},
		{fx.orgAdmin.ID, "org_admin"},
	} {
		_, err := orgs.AddOrgMember(ctx, store.AddMemberInput{
			OrgID: org.ID, UserID: m.userID, Role: m.role, Status: "active",
		})
		require.NoError(t, err)
	}

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

	return fx
}

func (fx *orgListFixture) call(t *testing.T, method, path string, claims *auth.Claims) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req = withClaims(req, claims)
	w := httptest.NewRecorder()
	switch path {
	case "/api/org/teachers?orgId=" + fx.orgID, "/api/org/teachers":
		fx.h.ListTeachers(w, req)
	case "/api/org/students?orgId=" + fx.orgID, "/api/org/students":
		fx.h.ListStudents(w, req)
	case "/api/org/courses?orgId=" + fx.orgID, "/api/org/courses":
		fx.h.ListCourses(w, req)
	case "/api/org/classes?orgId=" + fx.orgID, "/api/org/classes":
		fx.h.ListClasses(w, req)
	default:
		t.Fatalf("unexpected path: %s", path)
	}
	return w.Code, w.Body.Bytes()
}

func TestOrgList_Teachers_OrgAdmin(t *testing.T) {
	fx := newOrgListFixture(t, "teachers-admin")
	code, body := fx.call(t, http.MethodGet, "/api/org/teachers?orgId="+fx.orgID,
		&auth.Claims{UserID: fx.orgAdmin.ID})
	require.Equal(t, http.StatusOK, code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(body, &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, fx.teacher.ID, rows[0]["userId"])
	assert.Equal(t, "teacher", rows[0]["role"])
}

func TestOrgList_Students_OrgAdmin(t *testing.T) {
	fx := newOrgListFixture(t, "students-admin")
	code, body := fx.call(t, http.MethodGet, "/api/org/students?orgId="+fx.orgID,
		&auth.Claims{UserID: fx.orgAdmin.ID})
	require.Equal(t, http.StatusOK, code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(body, &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, fx.student.ID, rows[0]["userId"])
}

func TestOrgList_Courses_OrgAdmin(t *testing.T) {
	fx := newOrgListFixture(t, "courses-admin")
	code, body := fx.call(t, http.MethodGet, "/api/org/courses?orgId="+fx.orgID,
		&auth.Claims{UserID: fx.orgAdmin.ID})
	require.Equal(t, http.StatusOK, code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(body, &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, fx.courseID, rows[0]["id"])
	assert.Equal(t, "python", rows[0]["language"])
}

func TestOrgList_Classes_OrgAdmin_HasCounts(t *testing.T) {
	fx := newOrgListFixture(t, "classes-admin")
	code, body := fx.call(t, http.MethodGet, "/api/org/classes?orgId="+fx.orgID,
		&auth.Claims{UserID: fx.orgAdmin.ID})
	require.Equal(t, http.StatusOK, code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(body, &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, fx.classID, rows[0]["id"])
	// CreateClass adds the creator as instructor; we then added one student.
	assert.Equal(t, float64(1), rows[0]["instructorCount"])
	assert.Equal(t, float64(1), rows[0]["studentCount"])
	assert.NotEmpty(t, rows[0]["courseTitle"])
}

func TestOrgList_PlatformAdmin(t *testing.T) {
	fx := newOrgListFixture(t, "platform-admin")
	code, _ := fx.call(t, http.MethodGet, "/api/org/teachers?orgId="+fx.orgID,
		&auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true})
	assert.Equal(t, http.StatusOK, code)
}

func TestOrgList_AdminImpersonating(t *testing.T) {
	// Per plan 039 correction: claims.ImpersonatedBy != "" preserves admin
	// equivalence for read endpoints. The impersonated user (outsider)
	// would be blocked otherwise.
	fx := newOrgListFixture(t, "imp-admin")
	code, _ := fx.call(t, http.MethodGet, "/api/org/teachers?orgId="+fx.orgID,
		&auth.Claims{UserID: fx.outsider.ID, ImpersonatedBy: fx.admin.ID})
	assert.Equal(t, http.StatusOK, code)
}

func TestOrgList_NonAdmin_Forbidden(t *testing.T) {
	fx := newOrgListFixture(t, "non-admin")
	for _, path := range []string{
		"/api/org/teachers?orgId=" + fx.orgID,
		"/api/org/students?orgId=" + fx.orgID,
		"/api/org/courses?orgId=" + fx.orgID,
		"/api/org/classes?orgId=" + fx.orgID,
	} {
		code, _ := fx.call(t, http.MethodGet, path,
			&auth.Claims{UserID: fx.teacher.ID}) // teacher, not org_admin
		assert.Equal(t, http.StatusForbidden, code, path)
	}
}

func TestOrgList_NoClaims(t *testing.T) {
	fx := newOrgListFixture(t, "no-claims")
	for _, path := range []string{
		"/api/org/teachers?orgId=" + fx.orgID,
		"/api/org/students?orgId=" + fx.orgID,
		"/api/org/courses?orgId=" + fx.orgID,
		"/api/org/classes?orgId=" + fx.orgID,
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		switch {
		case path == "/api/org/teachers?orgId="+fx.orgID:
			fx.h.ListTeachers(w, req)
		case path == "/api/org/students?orgId="+fx.orgID:
			fx.h.ListStudents(w, req)
		case path == "/api/org/courses?orgId="+fx.orgID:
			fx.h.ListCourses(w, req)
		case path == "/api/org/classes?orgId="+fx.orgID:
			fx.h.ListClasses(w, req)
		}
		assert.Equal(t, http.StatusUnauthorized, w.Code, path)
	}
}

func TestOrgList_MissingOrgId_UsesCallerOrg(t *testing.T) {
	// orgId omitted → handler resolves to the caller's first org_admin
	// membership. Same behavior as the dashboard endpoint.
	fx := newOrgListFixture(t, "missing-orgid")
	code, body := fx.call(t, http.MethodGet, "/api/org/teachers",
		&auth.Claims{UserID: fx.orgAdmin.ID})
	require.Equal(t, http.StatusOK, code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(body, &rows))
	require.Len(t, rows, 1)
}

func TestOrgList_EmptyOrg_ReturnsEmptyArrays(t *testing.T) {
	// New org with only an org_admin — no teachers, students, courses,
	// or classes. Endpoints should return [], not 404.
	fx := newOrgListFixture(t, "empty-org")
	ctx := context.Background()
	emptyOrg, err := store.NewOrgStore(fx.db).CreateOrg(ctx, store.CreateOrgInput{
		Name:         "Empty Org",
		Slug:         "empty-orglist-" + "abc",
		Type:         "school",
		ContactEmail: "empty@example.com",
		ContactName:  "Admin",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", emptyOrg.ID)
		fx.db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", emptyOrg.ID)
	})
	_, err = store.NewOrgStore(fx.db).AddOrgMember(ctx, store.AddMemberInput{
		OrgID: emptyOrg.ID, UserID: fx.orgAdmin.ID, Role: "org_admin", Status: "active",
	})
	require.NoError(t, err)

	for _, path := range []string{
		"/api/org/teachers?orgId=" + emptyOrg.ID,
		"/api/org/students?orgId=" + emptyOrg.ID,
		"/api/org/courses?orgId=" + emptyOrg.ID,
		"/api/org/classes?orgId=" + emptyOrg.ID,
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = withClaims(req, &auth.Claims{UserID: fx.orgAdmin.ID})
		w := httptest.NewRecorder()
		switch {
		case path == "/api/org/teachers?orgId="+emptyOrg.ID:
			fx.h.ListTeachers(w, req)
		case path == "/api/org/students?orgId="+emptyOrg.ID:
			fx.h.ListStudents(w, req)
		case path == "/api/org/courses?orgId="+emptyOrg.ID:
			fx.h.ListCourses(w, req)
		case path == "/api/org/classes?orgId="+emptyOrg.ID:
			fx.h.ListClasses(w, req)
		}
		assert.Equal(t, http.StatusOK, w.Code, path)
		var rows []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows), path)
		assert.Empty(t, rows, path)
	}
}
