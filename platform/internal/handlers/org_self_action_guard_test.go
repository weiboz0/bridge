package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 069 phase 4 (Codex post-impl Q3) — backend self-action guard
// tests for the org member endpoints.
//
// Without these guards an org_admin could PATCH or DELETE their own
// membership row directly via the API, locking themselves out of the
// org. The UI gates the option, but the UI is not the only entry
// point for the API.

func newSelfActionFixture(t *testing.T, suffix string) (*OrgHandler, *store.RegisteredUser, string, string) {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()

	users := store.NewUserStore(db)
	orgs := store.NewOrgStore(db)

	// Test admin user.
	admin, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "SAG Admin " + suffix,
		Email:    "sag-admin-" + suffix + "-" + uuid.NewString()[:8] + "@example.com",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", admin.ID)
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", admin.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", admin.ID)
	})

	// Org with admin as org_admin.
	org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name: "SAG Org " + suffix,
		Slug: "sag-org-" + suffix + "-" + uuid.NewString()[:8],
		Type: "school",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	membership, err := orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: org.ID, UserID: admin.ID, Role: "org_admin", Status: "active",
	})
	require.NoError(t, err)

	h := &OrgHandler{Orgs: orgs, Users: users}
	return h, admin, org.ID, membership.ID
}

func TestUpdateMember_SelfSuspendForbidden(t *testing.T) {
	h, admin, orgID, membershipID := newSelfActionFixture(t, t.Name())

	body, _ := json.Marshal(map[string]string{"status": "suspended"})
	req := httptest.NewRequest(http.MethodPatch,
		"/api/orgs/"+orgID+"/members/"+membershipID, bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{
		UserID:          admin.ID,
		Email:           admin.Email,
		Name:            admin.Name,
		IsPlatformAdmin: false,
	})
	req = withChiParams(req, map[string]string{
		"orgID":    orgID,
		"memberID": membershipID,
	})
	w := httptest.NewRecorder()
	h.UpdateMember(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "self-suspend must 403")
	assert.Contains(t, w.Body.String(), "suspend your own membership")
}

func TestUpdateMember_SelfActivateAllowed(t *testing.T) {
	// Self-action guard ONLY applies to "suspended". Setting one's
	// own status to active or pending is harmless — does not lock
	// the caller out — and should pass.
	h, admin, orgID, membershipID := newSelfActionFixture(t, t.Name())

	body, _ := json.Marshal(map[string]string{"status": "active"})
	req := httptest.NewRequest(http.MethodPatch,
		"/api/orgs/"+orgID+"/members/"+membershipID, bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{
		UserID:          admin.ID,
		Email:           admin.Email,
		Name:            admin.Name,
		IsPlatformAdmin: false,
	})
	req = withChiParams(req, map[string]string{
		"orgID":    orgID,
		"memberID": membershipID,
	})
	w := httptest.NewRecorder()
	h.UpdateMember(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "self-set-active must succeed (already-active no-op)")
}

func TestRemoveMember_SelfRemoveForbidden(t *testing.T) {
	h, admin, orgID, membershipID := newSelfActionFixture(t, t.Name())

	req := httptest.NewRequest(http.MethodDelete,
		"/api/orgs/"+orgID+"/members/"+membershipID, nil)
	req = withClaims(req, &auth.Claims{
		UserID:          admin.ID,
		Email:           admin.Email,
		Name:            admin.Name,
		IsPlatformAdmin: false,
	})
	req = withChiParams(req, map[string]string{
		"orgID":    orgID,
		"memberID": membershipID,
	})
	w := httptest.NewRecorder()
	h.RemoveMember(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "self-remove must 403")
	assert.Contains(t, w.Body.String(), "org transfer flow")
}

func TestRemoveMember_PlatformAdminBypass(t *testing.T) {
	// Platform admin self-removal is allowed because they don't
	// rely on the org_membership row for org access — they reach
	// every org via IsPlatformAdmin.
	h, admin, orgID, membershipID := newSelfActionFixture(t, t.Name())

	req := httptest.NewRequest(http.MethodDelete,
		"/api/orgs/"+orgID+"/members/"+membershipID, nil)
	req = withClaims(req, &auth.Claims{
		UserID:          admin.ID,
		Email:           admin.Email,
		Name:            admin.Name,
		IsPlatformAdmin: true,
	})
	req = withChiParams(req, map[string]string{
		"orgID":    orgID,
		"memberID": membershipID,
	})
	w := httptest.NewRecorder()
	h.RemoveMember(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "platform admin self-remove must succeed (bypass)")
}
