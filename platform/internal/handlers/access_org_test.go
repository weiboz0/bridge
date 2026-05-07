package handlers

// Plan 075 — integration tests for RequireOrgAuthority.
//
// The helper is a pure function (no HTTP fixtures needed). Each test
// creates its own user(s) + org + membership via store helpers, then
// calls RequireOrgAuthority directly and asserts (bool, error).

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// makeOrgUser creates a user + org and returns the OrgStore, the
// registered user, and the org ID. Cleanup removes the membership
// and org rows first, then the user + auth_provider rows.
//
// Pass role="" and status="" to skip adding a membership row (for
// non-member tests).
func makeOrgUser(t *testing.T, role, status string) (*store.OrgStore, *store.RegisteredUser, string) {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()

	users := store.NewUserStore(db)
	orgs := store.NewOrgStore(db)

	suffix := uuid.NewString()[:8]

	u, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "TestUser " + suffix,
		Email:    "orgtest-" + suffix + "@example.com",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", u.ID)
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
	})

	org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name: "TestOrg " + suffix,
		Slug: "testorg-" + suffix,
		Type: "school",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	if role != "" {
		_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
			OrgID:  org.ID,
			UserID: u.ID,
			Role:   role,
			Status: status,
		})
		require.NoError(t, err)
	}

	return orgs, u, org.ID
}

// ─── OrgRead ─────────────────────────────────────────────────────────────────

func TestRequireOrgAuthority_Read_ActiveMemberGrants(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "student", "active")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgRead)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRequireOrgAuthority_Read_NonMemberDenies(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "", "") // no membership row

	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgRead)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRequireOrgAuthority_Read_SuspendedMemberDenies(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "student", "suspended")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgRead)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRequireOrgAuthority_Read_ActiveOrgAdminGrants(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "org_admin", "active")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgRead)
	require.NoError(t, err)
	assert.True(t, ok)
}

// ─── OrgTeach ────────────────────────────────────────────────────────────────

func TestRequireOrgAuthority_Teach_ActiveTeacherGrants(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "teacher", "active")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgTeach)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRequireOrgAuthority_Teach_ActiveOrgAdminGrants(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "org_admin", "active")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgTeach)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRequireOrgAuthority_Teach_StudentDenies(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "student", "active")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgTeach)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRequireOrgAuthority_Teach_ParentDenies(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "parent", "active")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgTeach)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRequireOrgAuthority_Teach_SuspendedTeacherDenies(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "teacher", "suspended")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgTeach)
	require.NoError(t, err)
	assert.False(t, ok)
}

// ─── OrgAdmin ────────────────────────────────────────────────────────────────

func TestRequireOrgAuthority_Admin_ActiveOrgAdminGrants(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "org_admin", "active")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgAdmin)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRequireOrgAuthority_Admin_ActiveTeacherDenies(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "teacher", "active")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgAdmin)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRequireOrgAuthority_Admin_ActiveStudentDenies(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "student", "active")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgAdmin)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRequireOrgAuthority_Admin_SuspendedOrgAdminDenies(t *testing.T) {
	orgs, u, orgID := makeOrgUser(t, "org_admin", "suspended")
	claims := &auth.Claims{UserID: u.ID}

	ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, OrgAdmin)
	require.NoError(t, err)
	assert.False(t, ok)
}

// ─── Bypass paths ────────────────────────────────────────────────────────────

func TestRequireOrgAuthority_NilClaimsDenies(t *testing.T) {
	orgs, _, orgID := makeOrgUser(t, "org_admin", "active")

	ok, err := RequireOrgAuthority(context.Background(), orgs, nil, orgID, OrgAdmin)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRequireOrgAuthority_PlatformAdminBypass(t *testing.T) {
	// Platform admin with no membership row should pass every level.
	orgs, u, orgID := makeOrgUser(t, "", "") // no membership

	claims := &auth.Claims{UserID: u.ID, IsPlatformAdmin: true}

	for _, level := range []OrgAccessLevel{OrgRead, OrgTeach, OrgAdmin} {
		ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, level)
		require.NoError(t, err, "level=%s", level)
		assert.True(t, ok, "IsPlatformAdmin must bypass level=%s", level)
	}
}

func TestRequireOrgAuthority_ImpersonatorBypass(t *testing.T) {
	// Impersonator-of-admin (plan 039) with no membership row should pass
	// every level, including the most restrictive OrgAdmin.
	orgs, u, orgID := makeOrgUser(t, "", "") // no membership

	claims := &auth.Claims{UserID: u.ID, ImpersonatedBy: "some-admin-id"}

	for _, level := range []OrgAccessLevel{OrgRead, OrgTeach, OrgAdmin} {
		ok, err := RequireOrgAuthority(context.Background(), orgs, claims, orgID, level)
		require.NoError(t, err, "level=%s", level)
		assert.True(t, ok, "ImpersonatedBy must bypass level=%s", level)
	}
}

func TestRequireOrgAuthority_NilOrgsReturnsMisconfigured(t *testing.T) {
	// Passing orgs=nil must return ErrAccessHelperMisconfigured regardless of
	// valid claims — this surfaces a 500 instead of silently denying.
	claims := &auth.Claims{UserID: "any-user-id"}
	orgID := uuid.NewString()

	ok, err := RequireOrgAuthority(context.Background(), nil, claims, orgID, OrgRead)
	assert.False(t, ok)
	assert.True(t, errors.Is(err, ErrAccessHelperMisconfigured), "expected ErrAccessHelperMisconfigured, got %v", err)
}
