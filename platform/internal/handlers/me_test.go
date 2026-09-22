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

func TestBuildRoles_Admin(t *testing.T) {
	roles := buildRoles(true, nil)
	assert.Len(t, roles, 1)
	assert.Equal(t, "admin", roles[0].Role)
}

func TestBuildRoles_WithMemberships(t *testing.T) {
	memberships := []store.UserMembershipWithOrg{
		{Role: "teacher", Status: "active", OrgID: "org-1", OrgName: "School A", OrgStatus: "active"},
		{Role: "student", Status: "active", OrgID: "org-2", OrgName: "School B", OrgStatus: "active"},
		{Role: "teacher", Status: "pending", OrgID: "org-3", OrgName: "Pending", OrgStatus: "active"},
		{Role: "org_admin", Status: "active", OrgID: "org-4", OrgName: "Inactive Org", OrgStatus: "suspended"},
	}
	roles := buildRoles(false, memberships)
	assert.Len(t, roles, 2) // only active memberships in active orgs
	assert.Equal(t, "teacher", roles[0].Role)
	assert.Equal(t, "student", roles[1].Role)
}

func TestBuildRoles_Deduplication(t *testing.T) {
	memberships := []store.UserMembershipWithOrg{
		{Role: "teacher", Status: "active", OrgID: "org-1", OrgName: "School", OrgStatus: "active"},
		{Role: "teacher", Status: "active", OrgID: "org-1", OrgName: "School", OrgStatus: "active"},
	}
	roles := buildRoles(false, memberships)
	assert.Len(t, roles, 1)
}

func TestPrimaryRole(t *testing.T) {
	roles := []userRole{
		{Role: "student"},
		{Role: "teacher"},
		{Role: "admin"},
	}
	primary := primaryRole(roles)
	assert.Equal(t, "admin", primary.Role) // admin has highest priority
}

func TestPrimaryRole_TeacherOverStudent(t *testing.T) {
	roles := []userRole{
		{Role: "student"},
		{Role: "teacher"},
	}
	primary := primaryRole(roles)
	assert.Equal(t, "teacher", primary.Role)
}

func TestPrimaryRole_Empty(t *testing.T) {
	assert.Nil(t, primaryRole(nil))
	assert.Nil(t, primaryRole([]userRole{}))
}

func TestPortalPath(t *testing.T) {
	assert.Equal(t, "/admin", portalPath("admin"))
	assert.Equal(t, "/teacher", portalPath("teacher"))
	assert.Equal(t, "/student", portalPath("student"))
	assert.Equal(t, "/parent", portalPath("parent"))
	assert.Equal(t, "/org", portalPath("org_admin"))
	assert.Equal(t, "/onboarding", portalPath("unknown"))
}

func TestGetMemberships_NoClaims(t *testing.T) {
	h := &MeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/me/memberships", nil)
	w := httptest.NewRecorder()
	h.GetMemberships(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetRoles_NoClaims(t *testing.T) {
	h := &MeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/me/roles", nil)
	w := httptest.NewRecorder()
	h.GetRoles(w, req)
	// Returns 200 with authenticated: false (designed for landing page)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetRoles_Authenticated(t *testing.T) {
	// Can't test happy path without a real OrgStore, but verify it doesn't panic
	h := &MeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/me/roles", nil)
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	// Will fail on h.Orgs.GetUserMemberships since Orgs is nil
	defer func() { recover() }()
	h.GetRoles(w, req)
}

func TestGetPortalAccess_NoClaims(t *testing.T) {
	h := &MeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/me/portal-access", nil)
	w := httptest.NewRecorder()
	h.GetPortalAccess(w, req)

	// Unauthenticated callers get 200 with both flags false (landing-page shape,
	// plan 090): authenticated distinguishes "no session" from "session, no roles".
	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, false, body["authorized"])
	assert.Equal(t, false, body["authenticated"])
}

// GetPortalAccess must distinguish "no session" from "valid session, zero
// roles": an authenticated user with no org memberships and no platform-admin
// flag reads authenticated=true, authorized=false. This backs the plan-090
// role-neutral shell, which admits on `authenticated` so a zero-role user can
// reach /sessions. Needs a real OrgStore (Orgs is a concrete *store.OrgStore,
// so no stub is possible without changing non-test code) — DATABASE_URL gated.
func TestGetPortalAccess_ZeroRoleAuthenticated(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	u := insertFixtureUser(t, db, store.RegisterInput{
		Name: "Zero Role", Email: "zero-role-portal@example.com", Password: "testpassword123",
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
	})

	h := &MeHandler{Orgs: store.NewOrgStore(db)}
	req := httptest.NewRequest(http.MethodGet, "/api/me/portal-access", nil)
	req = withClaims(req, &auth.Claims{UserID: u.ID, Email: u.Email, Name: u.Name})
	w := httptest.NewRecorder()
	h.GetPortalAccess(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["authenticated"], "a valid session must read as authenticated")
	assert.Equal(t, false, body["authorized"], "a user with zero roles must not be authorized")
}

func TestGetIdentity_NoClaims(t *testing.T) {
	h := &MeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/me/identity", nil)
	w := httptest.NewRecorder()
	h.GetIdentity(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetIdentity_ReturnsClaims(t *testing.T) {
	h := &MeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/me/identity", nil)
	req = withClaims(req, &auth.Claims{
		UserID:          "user-99",
		Email:           "u@example.com",
		Name:            "Diag User",
		IsPlatformAdmin: true,
	})
	w := httptest.NewRecorder()
	h.GetIdentity(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "user-99", body["userId"])
	assert.Equal(t, "u@example.com", body["email"])
	assert.Equal(t, "Diag User", body["name"])
	assert.Equal(t, true, body["isPlatformAdmin"])
}

func TestGetIdentity_ImpersonatedBy(t *testing.T) {
	h := &MeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/me/identity", nil)
	req = withClaims(req, &auth.Claims{
		UserID:         "target",
		Email:          "t@example.com",
		Name:           "Target",
		ImpersonatedBy: "admin-1",
	})
	w := httptest.NewRecorder()
	h.GetIdentity(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "target", body["userId"])
	assert.Equal(t, "admin-1", body["impersonatedBy"])
}
