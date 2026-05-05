package handlers

import (
	"bytes"
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

// Plan 070 phase 1 — org-admin parent-link CRUD tests.
//
// Each test sets up a clean fixture (one org, one class, an
// org_admin, a parent, a child enrolled as a student, plus a
// non-admin teacher and a "second org" pair for cross-org guards).
// The handler is mounted on a real chi router so URL params resolve
// the same way they do in production.

type orgParentLinksFixture struct {
	router      chi.Router
	orgID       string
	otherOrgID  string
	classID     string
	otherClassID string
	admin       *store.RegisteredUser // org_admin in orgID
	teacher     *store.RegisteredUser // teacher in orgID, NOT admin
	parent      *store.RegisteredUser // unrelated user → "parent" in tests
	child       *store.RegisteredUser // student in classID
	otherChild  *store.RegisteredUser // student in otherClassID (other org)
	parentLinks *store.ParentLinkStore
	orgs        *store.OrgStore
}

func newOrgParentLinksFixture(t *testing.T, suffix string) *orgParentLinksFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()
	users := store.NewUserStore(db)
	orgs := store.NewOrgStore(db)
	courses := store.NewCourseStore(db)
	classes := store.NewClassStore(db)
	links := store.NewParentLinkStore(db)

	h := &OrgParentLinksHandler{
		Orgs:        orgs,
		ParentLinks: links,
		Users:       users,
	}

	tag := func(s string) string { return "opl-" + suffix + "-" + s + "-" + uuid.NewString()[:8] }

	mkUser := func(label string) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name:     "OPL " + label,
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
			Name: "OPL Org " + label,
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
			Title:      "OPL Course " + label,
			OrgID:      orgID,
			CreatedBy:  creatorID,
			GradeLevel: "9-12",
			Language:   "python",
		})
		require.NoError(t, err)
		cls, err := classes.CreateClass(ctx, store.CreateClassInput{
			Title:     "OPL Class " + label,
			Term:      "fall-2026",
			CourseID:  course.ID,
			OrgID:     orgID,
			CreatedBy: creatorID,
		})
		require.NoError(t, err)
		// CreateClass defaults status='active' but assert it explicitly.
		_, err = db.ExecContext(ctx, `UPDATE classes SET status = 'active' WHERE id = $1`, cls.ID)
		require.NoError(t, err)
		return cls
	}

	addStudent := func(classID, userID string) {
		_, err := classes.AddClassMember(ctx, store.AddClassMemberInput{
			ClassID: classID, UserID: userID, Role: "student",
		})
		require.NoError(t, err)
	}

	addOrgRole := func(orgID, userID, role string) {
		_, err := orgs.AddOrgMember(ctx, store.AddMemberInput{
			OrgID: orgID, UserID: userID, Role: role, Status: "active",
		})
		require.NoError(t, err)
	}

	admin := mkUser("admin")
	teacher := mkUser("teacher")
	parent := mkUser("parent")
	child := mkUser("child")
	otherChild := mkUser("otherchild")

	org := mkOrg("primary")
	otherOrg := mkOrg("other")

	addOrgRole(org.ID, admin.ID, "org_admin")
	addOrgRole(org.ID, teacher.ID, "teacher")

	cls := mkClass(org.ID, admin.ID, "primary")
	otherCls := mkClass(otherOrg.ID, admin.ID, "other")

	addStudent(cls.ID, child.ID)
	addStudent(otherCls.ID, otherChild.ID)

	r := chi.NewRouter()
	h.Routes(r)

	return &orgParentLinksFixture{
		router: r,
		orgID:  org.ID, otherOrgID: otherOrg.ID,
		classID: cls.ID, otherClassID: otherCls.ID,
		admin: admin, teacher: teacher, parent: parent, child: child, otherChild: otherChild,
		parentLinks: links, orgs: orgs,
	}
}

func (fx *orgParentLinksFixture) claimsFor(u *store.RegisteredUser, platformAdmin bool) *auth.Claims {
	return &auth.Claims{UserID: u.ID, Email: u.Email, Name: u.Name, IsPlatformAdmin: platformAdmin}
}

func (fx *orgParentLinksFixture) doRequest(t *testing.T, method, path string, body any, claims *auth.Claims) *httptest.ResponseRecorder {
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
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)
	return w
}

// ---------- ListByOrg ----------

func TestOrgParentLinks_List_HappyPath(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	ctx := context.Background()

	// Seed an active link.
	_, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.child.ID, fx.admin.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/orgs/"+fx.orgID+"/parent-links", nil, fx.claimsFor(fx.admin, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var rows []store.ParentLinkRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, fx.parent.ID, rows[0].ParentUserID)
	assert.Equal(t, fx.child.ID, rows[0].ChildUserID)
	assert.Equal(t, fx.parent.Email, rows[0].ParentEmail)
	assert.Equal(t, fx.child.Name, rows[0].ChildName)
	require.NotNil(t, rows[0].ClassID)
	assert.Equal(t, fx.classID, *rows[0].ClassID)
}

func TestOrgParentLinks_List_FilterByParentEmail(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	ctx := context.Background()
	_, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.child.ID, fx.admin.ID)
	require.NoError(t, err)

	// Use the first 4 chars of the parent's email as a prefix filter.
	prefix := fx.parent.Email[:4]
	path := "/api/orgs/" + fx.orgID + "/parent-links?parent=" + prefix
	w := fx.doRequest(t, http.MethodGet, path, nil, fx.claimsFor(fx.admin, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var rows []store.ParentLinkRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	assert.Len(t, rows, 1)

	// Bogus prefix → no results.
	w = fx.doRequest(t, http.MethodGet, "/api/orgs/"+fx.orgID+"/parent-links?parent=zzz-no-match-zzz", nil, fx.claimsFor(fx.admin, false))
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	assert.Len(t, rows, 0)
}

func TestOrgParentLinks_List_NonAdminForbidden(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())

	// teacher has org membership but not org_admin role → 403.
	w := fx.doRequest(t, http.MethodGet, "/api/orgs/"+fx.orgID+"/parent-links", nil, fx.claimsFor(fx.teacher, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// Codex post-impl Q1 — coverage gap: an org_admin in a DIFFERENT
// org should also receive 403 when hitting our org's endpoint.
// requireOrgAdmin only accepts active org_admin in the targeted org.
func TestOrgParentLinks_List_CrossOrgAdminForbidden(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	ctx := context.Background()

	// Make `other` an org_admin in fx.otherOrgID so they have a real
	// admin role somewhere — just not in fx.orgID.
	_, err := fx.orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: fx.otherOrgID, UserID: fx.parent.ID, Role: "org_admin", Status: "active",
	})
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/orgs/"+fx.orgID+"/parent-links", nil, fx.claimsFor(fx.parent, false))
	assert.Equal(t, http.StatusForbidden, w.Code, "cross-org admin must not reach another org's endpoint")
}

func TestOrgParentLinks_List_PlatformAdminBypass(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())

	// Platform admin without an org membership row should still pass.
	w := fx.doRequest(t, http.MethodGet, "/api/orgs/"+fx.orgID+"/parent-links", nil, fx.claimsFor(fx.parent, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestOrgParentLinks_List_NoClaims(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/orgs/"+fx.orgID+"/parent-links", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------- CreateLink ----------

func TestOrgParentLinks_Create_HappyPath_AlsoUpsertsOrgMembership(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	ctx := context.Background()

	body := map[string]any{
		"parentEmail": fx.parent.Email,
		"childUserId": fx.child.ID,
	}
	w := fx.doRequest(t, http.MethodPost, "/api/orgs/"+fx.orgID+"/parent-links", body, fx.claimsFor(fx.admin, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	// Verify the link landed.
	links, err := fx.parentLinks.ListByParent(ctx, fx.parent.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, fx.child.ID, links[0].ChildUserID)
	assert.Equal(t, "active", links[0].Status)

	// Decisions §3 — the parent now has an active org_memberships row
	// in this org so they can reach /parent on next sign-in.
	roles, err := fx.orgs.GetUserRolesInOrg(ctx, fx.orgID, fx.parent.ID)
	require.NoError(t, err)
	hasParent := false
	for _, m := range roles {
		if m.Role == "parent" && m.Status == "active" {
			hasParent = true
		}
	}
	assert.True(t, hasParent, "parent must have active org_memberships{role:'parent'} row")
}

func TestOrgParentLinks_Create_ReactivatesSuspendedMembership(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	ctx := context.Background()

	// Pre-seed a suspended parent membership — simulates a parent
	// whose previous link cycle was revoked + suspended manually.
	_, err := fx.orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: fx.orgID, UserID: fx.parent.ID, Role: "parent", Status: "suspended",
	})
	require.NoError(t, err)

	body := map[string]any{
		"parentEmail": fx.parent.Email,
		"childUserId": fx.child.ID,
	}
	w := fx.doRequest(t, http.MethodPost, "/api/orgs/"+fx.orgID+"/parent-links", body, fx.claimsFor(fx.admin, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	// Membership should now be active (UpsertActiveMembership flips it).
	roles, err := fx.orgs.GetUserRolesInOrg(ctx, fx.orgID, fx.parent.ID)
	require.NoError(t, err)
	for _, m := range roles {
		if m.Role == "parent" {
			assert.Equal(t, "active", m.Status, "previously-suspended parent membership must be reactivated")
		}
	}
}

func TestOrgParentLinks_Create_UnknownParentEmail_404(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	body := map[string]any{
		"parentEmail": "not-a-real-user@nowhere.example",
		"childUserId": fx.child.ID,
	}
	w := fx.doRequest(t, http.MethodPost, "/api/orgs/"+fx.orgID+"/parent-links", body, fx.claimsFor(fx.admin, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestOrgParentLinks_Create_CrossOrgChild_403(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	body := map[string]any{
		"parentEmail": fx.parent.Email,
		"childUserId": fx.otherChild.ID, // belongs to otherOrg
	}
	w := fx.doRequest(t, http.MethodPost, "/api/orgs/"+fx.orgID+"/parent-links", body, fx.claimsFor(fx.admin, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestOrgParentLinks_Create_AlreadyLinked_409(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	ctx := context.Background()

	_, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.child.ID, fx.admin.ID)
	require.NoError(t, err)

	body := map[string]any{
		"parentEmail": fx.parent.Email,
		"childUserId": fx.child.ID,
	}
	w := fx.doRequest(t, http.MethodPost, "/api/orgs/"+fx.orgID+"/parent-links", body, fx.claimsFor(fx.admin, false))
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestOrgParentLinks_Create_MissingFields_400(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())

	for _, body := range []map[string]any{
		{"parentEmail": "", "childUserId": fx.child.ID},
		{"parentEmail": fx.parent.Email, "childUserId": ""},
	} {
		w := fx.doRequest(t, http.MethodPost, "/api/orgs/"+fx.orgID+"/parent-links", body, fx.claimsFor(fx.admin, false))
		assert.Equal(t, http.StatusBadRequest, w.Code, "body=%v", body)
	}
}

func TestOrgParentLinks_Create_NonAdminForbidden(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	body := map[string]any{
		"parentEmail": fx.parent.Email,
		"childUserId": fx.child.ID,
	}
	w := fx.doRequest(t, http.MethodPost, "/api/orgs/"+fx.orgID+"/parent-links", body, fx.claimsFor(fx.teacher, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ---------- RevokeLink ----------

func TestOrgParentLinks_Revoke_HappyPath(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	ctx := context.Background()
	link, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.child.ID, fx.admin.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodDelete, "/api/orgs/"+fx.orgID+"/parent-links/"+link.ID, nil, fx.claimsFor(fx.admin, false))
	require.Equal(t, http.StatusOK, w.Code)

	got, err := fx.parentLinks.GetLink(ctx, link.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "revoked", got.Status)
}

func TestOrgParentLinks_Revoke_CrossOrgLink_404(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	ctx := context.Background()

	// Link the parent to the OTHER-ORG child first (legitimate at
	// the platform level — admin-of-orgID shouldn't be able to
	// revoke it through this endpoint).
	link, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.otherChild.ID, fx.admin.ID)
	require.NoError(t, err)

	// Try to revoke through the orgID endpoint.
	w := fx.doRequest(t, http.MethodDelete, "/api/orgs/"+fx.orgID+"/parent-links/"+link.ID, nil, fx.claimsFor(fx.admin, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "cross-org revoke must surface as 404 (no leak)")

	// Confirm the link is still active.
	got, err := fx.parentLinks.GetLink(ctx, link.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "active", got.Status)
}

func TestOrgParentLinks_Revoke_NonExistent_404(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodDelete, "/api/orgs/"+fx.orgID+"/parent-links/"+uuid.NewString(), nil, fx.claimsFor(fx.admin, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestOrgParentLinks_Revoke_NonAdminForbidden(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	ctx := context.Background()
	link, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.child.ID, fx.admin.ID)
	require.NoError(t, err)
	w := fx.doRequest(t, http.MethodDelete, "/api/orgs/"+fx.orgID+"/parent-links/"+link.ID, nil, fx.claimsFor(fx.teacher, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ---------- ListEligibleChildren ----------

func TestOrgParentLinks_EligibleChildren_HappyPath(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/orgs/"+fx.orgID+"/eligible-children", nil, fx.claimsFor(fx.admin, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var children []store.EligibleChild
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &children))
	require.Len(t, children, 1, "fixture seeds exactly one student in this org")
	assert.Equal(t, fx.child.ID, children[0].UserID)
	assert.Equal(t, fx.child.Email, children[0].Email)
}

func TestOrgParentLinks_EligibleChildren_ScopedToOrg(t *testing.T) {
	// otherOrg has its own student; querying orgID must not surface them.
	fx := newOrgParentLinksFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/orgs/"+fx.orgID+"/eligible-children", nil, fx.claimsFor(fx.admin, false))
	require.Equal(t, http.StatusOK, w.Code)

	var children []store.EligibleChild
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &children))
	for _, c := range children {
		assert.NotEqual(t, fx.otherChild.ID, c.UserID, "otherOrg student must not appear")
	}
}

func TestOrgParentLinks_EligibleChildren_NonAdminForbidden(t *testing.T) {
	fx := newOrgParentLinksFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodGet, "/api/orgs/"+fx.orgID+"/eligible-children", nil, fx.claimsFor(fx.teacher, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}
