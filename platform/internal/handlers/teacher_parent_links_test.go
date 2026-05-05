package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 070 phase 3 — teacher class-detail parent-link popover.
//
// Each test seeds an org + class + instructor + TA + outsider plus
// a parent + child enrolled in the class. The handler is mounted on
// a real chi router so URL params resolve as in production.

type teacherParentLinksFixture struct {
	router       chi.Router
	orgID        string
	otherOrgID   string
	classID      string
	otherClassID string
	instructor   *store.RegisteredUser
	ta           *store.RegisteredUser
	student      *store.RegisteredUser // enrolled in classID
	parent       *store.RegisteredUser
	outsider     *store.RegisteredUser // no membership anywhere
	parentLinks  *store.ParentLinkStore
}

func newTeacherParentLinksFixture(t *testing.T, suffix string) *teacherParentLinksFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()
	users := store.NewUserStore(db)
	orgs := store.NewOrgStore(db)
	courses := store.NewCourseStore(db)
	classes := store.NewClassStore(db)
	links := store.NewParentLinkStore(db)

	h := &TeacherParentLinksHandler{
		Classes:     classes,
		Orgs:        orgs,
		ParentLinks: links,
	}

	tag := func(s string) string { return "tpl-" + suffix + "-" + s + "-" + uuid.NewString()[:8] }

	mkUser := func(label string) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name:     "TPL " + label,
			Email:    tag(label) + "@example.com",
			Password: "testpassword123",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM parent_links WHERE parent_user_id = $1 OR child_user_id = $1 OR created_by = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM class_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	mkOrg := func(label string) *store.Org {
		org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
			Name: "TPL Org " + label,
			Slug: tag("org-" + label),
			Type: "school",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id IN (SELECT id FROM classes WHERE org_id = $1)", org.ID)
			db.ExecContext(ctx, "DELETE FROM classes WHERE org_id = $1", org.ID)
			db.ExecContext(ctx, "DELETE FROM courses WHERE org_id = $1", org.ID)
			db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
			db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
		})
		return org
	}

	mkClass := func(orgID, creatorID, label string) *store.Class {
		course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
			Title:      "TPL Course " + label,
			OrgID:      orgID,
			CreatedBy:  creatorID,
			GradeLevel: "9-12",
			Language:   "python",
		})
		require.NoError(t, err)
		cls, err := classes.CreateClass(ctx, store.CreateClassInput{
			Title:     "TPL Class " + label,
			Term:      "fall-2026",
			CourseID:  course.ID,
			OrgID:     orgID,
			CreatedBy: creatorID,
		})
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `UPDATE classes SET status = 'active' WHERE id = $1`, cls.ID)
		require.NoError(t, err)
		return cls
	}

	addMember := func(classID, userID, role string) {
		_, err := classes.AddClassMember(ctx, store.AddClassMemberInput{
			ClassID: classID, UserID: userID, Role: role,
		})
		require.NoError(t, err)
	}

	instructor := mkUser("instructor")
	ta := mkUser("ta")
	student := mkUser("student")
	parent := mkUser("parent")
	outsider := mkUser("outsider")

	org := mkOrg("primary")
	otherOrg := mkOrg("other")

	cls := mkClass(org.ID, instructor.ID, "primary")
	otherCls := mkClass(otherOrg.ID, instructor.ID, "other")

	addMember(cls.ID, instructor.ID, "instructor")
	addMember(cls.ID, ta.ID, "ta")
	addMember(cls.ID, student.ID, "student")

	r := chi.NewRouter()
	h.Routes(r)

	return &teacherParentLinksFixture{
		router: r,
		orgID:  org.ID, otherOrgID: otherOrg.ID,
		classID: cls.ID, otherClassID: otherCls.ID,
		instructor: instructor, ta: ta, student: student, parent: parent, outsider: outsider,
		parentLinks: links,
	}
}

func (fx *teacherParentLinksFixture) claimsFor(u *store.RegisteredUser, platformAdmin bool) *auth.Claims {
	return &auth.Claims{UserID: u.ID, Email: u.Email, Name: u.Name, IsPlatformAdmin: platformAdmin}
}

func (fx *teacherParentLinksFixture) doGet(t *testing.T, path string, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if claims != nil {
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)
	return w
}

// ---------- happy path + visibility ----------

func TestTeacherParentLinks_Instructor_HappyPath(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	ctx := context.Background()

	_, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.student.ID, fx.instructor.ID)
	require.NoError(t, err)

	w := fx.doGet(t, "/api/teacher/classes/"+fx.classID+"/parent-links", fx.claimsFor(fx.instructor, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var rows []store.TeacherParentLinkRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, fx.parent.ID, rows[0].ParentUserID)
	assert.Equal(t, fx.parent.Email, rows[0].ParentEmail)
	assert.Equal(t, fx.student.ID, rows[0].StudentUserID)
}

func TestTeacherParentLinks_TA_AlsoHasAccess(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	ctx := context.Background()

	_, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.student.ID, fx.instructor.ID)
	require.NoError(t, err)

	w := fx.doGet(t, "/api/teacher/classes/"+fx.classID+"/parent-links", fx.claimsFor(fx.ta, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeacherParentLinks_OrgAdmin_AlsoHasAccess(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	ctx := context.Background()

	// Promote outsider to org_admin in the class's org, no class
	// membership.
	_, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.student.ID, fx.instructor.ID)
	require.NoError(t, err)
	orgs := store.NewOrgStore(integrationDB(t))
	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: fx.orgID, UserID: fx.outsider.ID, Role: "org_admin", Status: "active",
	})
	require.NoError(t, err)

	w := fx.doGet(t, "/api/teacher/classes/"+fx.classID+"/parent-links", fx.claimsFor(fx.outsider, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeacherParentLinks_PlatformAdmin_Bypass(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	w := fx.doGet(t, "/api/teacher/classes/"+fx.classID+"/parent-links", fx.claimsFor(fx.outsider, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeacherParentLinks_EmptyClass_ReturnsEmptyArray(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	// No parent_link seeded.
	w := fx.doGet(t, "/api/teacher/classes/"+fx.classID+"/parent-links", fx.claimsFor(fx.instructor, false))
	require.Equal(t, http.StatusOK, w.Code)
	var rows []store.TeacherParentLinkRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	assert.Equal(t, 0, len(rows))
}

// Codex post-impl review (plan 070 phase 3) flagged the
// archived-class loose query as a blocker. ListByClass now joins
// `classes` and filters `c.status = 'active'`, mirroring ListByOrg.
// This test locks the regression: a parent-link to a student in an
// archived class must NOT surface.
func TestTeacherParentLinks_ArchivedClass_NotShown(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	ctx := context.Background()

	_, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.student.ID, fx.instructor.ID)
	require.NoError(t, err)

	// Archive the class. The teacher remains an instructor on the
	// row (so the auth gate still passes), but the archived-status
	// filter on the SQL drops the link from the response.
	db := integrationDB(t)
	_, err = db.ExecContext(ctx, `UPDATE classes SET status = 'archived' WHERE id = $1`, fx.classID)
	require.NoError(t, err)

	w := fx.doGet(t, "/api/teacher/classes/"+fx.classID+"/parent-links", fx.claimsFor(fx.instructor, false))
	require.Equal(t, http.StatusOK, w.Code)

	var rows []store.TeacherParentLinkRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	assert.Equal(t, 0, len(rows), "archived-class parent links must not surface")
}

func TestTeacherParentLinks_RevokedLink_NotShown(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	ctx := context.Background()
	link, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.student.ID, fx.instructor.ID)
	require.NoError(t, err)
	_, err = fx.parentLinks.RevokeLink(ctx, link.ID)
	require.NoError(t, err)

	w := fx.doGet(t, "/api/teacher/classes/"+fx.classID+"/parent-links", fx.claimsFor(fx.instructor, false))
	require.Equal(t, http.StatusOK, w.Code)
	var rows []store.TeacherParentLinkRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	assert.Equal(t, 0, len(rows), "revoked link must not surface in the read view")
}

// ---------- denials ----------

func TestTeacherParentLinks_Student_Denied(t *testing.T) {
	// Class members who are STUDENTS don't pass — the response
	// includes parent emails which are PII the help-queue UI never
	// needs. Same gate ListMembers uses.
	fx := newTeacherParentLinksFixture(t, t.Name())
	w := fx.doGet(t, "/api/teacher/classes/"+fx.classID+"/parent-links", fx.claimsFor(fx.student, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "student role must be denied (404 to avoid leaking existence)")
}

func TestTeacherParentLinks_Outsider_Denied(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	w := fx.doGet(t, "/api/teacher/classes/"+fx.classID+"/parent-links", fx.claimsFor(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeacherParentLinks_NoClaims_401(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	w := fx.doGet(t, "/api/teacher/classes/"+fx.classID+"/parent-links", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTeacherParentLinks_NonExistentClass_404(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	w := fx.doGet(t, "/api/teacher/classes/"+uuid.NewString()+"/parent-links", fx.claimsFor(fx.instructor, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeacherParentLinks_MalformedClassID_400(t *testing.T) {
	fx := newTeacherParentLinksFixture(t, t.Name())
	w := fx.doGet(t, "/api/teacher/classes/not-a-uuid/parent-links", fx.claimsFor(fx.instructor, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
