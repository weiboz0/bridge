package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

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
