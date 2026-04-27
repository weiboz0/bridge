package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDB(t *testing.T) *sql.DB {
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

// createTestOrg is a helper that creates an org and registers cleanup.
func createTestOrg(t *testing.T, db *sql.DB, store *OrgStore, suffix string) *Org {
	t.Helper()
	ctx := context.Background()
	org, err := store.CreateOrg(ctx, CreateOrgInput{
		Name:         "Test Org " + suffix,
		Slug:         "test-org-" + suffix,
		Type:         "school",
		ContactEmail: suffix + "@example.com",
		ContactName:  "Admin " + suffix,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})
	return org
}

// createTestUser is a helper that creates a user and registers cleanup.
func createTestUser(t *testing.T, db *sql.DB, store *UserStore, suffix string) *RegisteredUser {
	t.Helper()
	ctx := context.Background()
	user, err := store.RegisterUser(ctx, RegisterInput{
		Name:     "User " + suffix,
		Email:    suffix + "@example.com",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", user.ID)
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})
	return user
}

// --- Org CRUD ---

func TestOrgStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)
	org := createTestOrg(t, db, store, t.Name())

	assert.Equal(t, "Test Org "+t.Name(), org.Name)
	assert.Equal(t, "pending", org.Status)

	fetched, err := store.GetOrg(context.Background(), org.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, org.ID, fetched.ID)
}

func TestOrgStore_GetOrg_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)

	org, err := store.GetOrg(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, org)
}

func TestOrgStore_GetOrgBySlug(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)
	org := createTestOrg(t, db, store, t.Name())

	fetched, err := store.GetOrgBySlug(context.Background(), "test-org-"+t.Name())
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, org.ID, fetched.ID)
}

func TestOrgStore_GetOrgBySlug_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)

	fetched, err := store.GetOrgBySlug(context.Background(), "nonexistent-slug-xyz")
	assert.NoError(t, err)
	assert.Nil(t, fetched)
}

func TestOrgStore_ListOrgs(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)

	orgs, err := store.ListOrgs(context.Background(), "")
	require.NoError(t, err)
	assert.NotNil(t, orgs)
}

func TestOrgStore_ListOrgs_WithStatusFilter(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)
	ctx := context.Background()
	org := createTestOrg(t, db, store, t.Name())

	// Org starts as pending
	orgs, err := store.ListOrgs(ctx, "pending")
	require.NoError(t, err)
	found := false
	for _, o := range orgs {
		if o.ID == org.ID {
			found = true
		}
	}
	assert.True(t, found, "should find org in pending list")

	// Should not appear in active list
	orgs, err = store.ListOrgs(ctx, "active")
	require.NoError(t, err)
	for _, o := range orgs {
		assert.NotEqual(t, org.ID, o.ID, "pending org should not appear in active list")
	}
}

func TestOrgStore_UpdateOrg(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)
	ctx := context.Background()
	org := createTestOrg(t, db, store, t.Name())

	newName := "Updated Name"
	newEmail := "updated@example.com"
	updated, err := store.UpdateOrg(ctx, org.ID, UpdateOrgInput{
		Name:         &newName,
		ContactEmail: &newEmail,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, "updated@example.com", updated.ContactEmail)
	// Unchanged fields should be preserved
	assert.Equal(t, org.ContactName, updated.ContactName)
}

func TestOrgStore_UpdateOrg_NoChanges(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)
	ctx := context.Background()
	org := createTestOrg(t, db, store, t.Name())

	// Empty input should return existing org unchanged
	updated, err := store.UpdateOrg(ctx, org.ID, UpdateOrgInput{})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, org.Name, updated.Name)
}

func TestOrgStore_UpdateOrg_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)

	newName := "X"
	updated, err := store.UpdateOrg(context.Background(), "00000000-0000-0000-0000-000000000000", UpdateOrgInput{Name: &newName})
	assert.NoError(t, err)
	assert.Nil(t, updated)
}

func TestOrgStore_UpdateOrgStatus(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)
	ctx := context.Background()
	org := createTestOrg(t, db, store, t.Name())

	// Activate school -- should set verifiedAt
	updated, err := store.UpdateOrgStatus(ctx, org.ID, "active")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "active", updated.Status)
	assert.NotNil(t, updated.VerifiedAt)
}

func TestOrgStore_UpdateOrgStatus_NonSchool(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)
	ctx := context.Background()

	org, err := store.CreateOrg(ctx, CreateOrgInput{
		Name: "Bootcamp", Slug: "bootcamp-" + t.Name(), Type: "bootcamp",
		ContactEmail: "bc@example.com", ContactName: "BC Admin",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	updated, err := store.UpdateOrgStatus(ctx, org.ID, "active")
	require.NoError(t, err)
	assert.Equal(t, "active", updated.Status)
	assert.Nil(t, updated.VerifiedAt, "non-school should not get verifiedAt")
}

func TestOrgStore_UpdateOrgStatus_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewOrgStore(db)

	updated, err := store.UpdateOrgStatus(context.Background(), "00000000-0000-0000-0000-000000000000", "active")
	assert.NoError(t, err)
	assert.Nil(t, updated)
}

// --- Membership operations ---

func TestOrgStore_AddOrgMember(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	m, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID:  org.ID,
		UserID: user.ID,
		Role:   "teacher",
		Status: "active",
	})
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, org.ID, m.OrgID)
	assert.Equal(t, user.ID, m.UserID)
	assert.Equal(t, "teacher", m.Role)
	assert.Equal(t, "active", m.Status)
}

func TestOrgStore_AddOrgMember_DefaultStatus(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	m, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID:  org.ID,
		UserID: user.ID,
		Role:   "student",
	})
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "active", m.Status, "default status should be active")
}

func TestOrgStore_AddOrgMember_Duplicate(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	_, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher",
	})
	require.NoError(t, err)

	// Same role again — ON CONFLICT DO NOTHING, returns nil
	dup, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher",
	})
	assert.NoError(t, err)
	assert.Nil(t, dup, "duplicate membership should return nil")
}

func TestOrgStore_AddOrgMember_MultipleRoles(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	m1, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher",
	})
	require.NoError(t, err)
	require.NotNil(t, m1)

	// Same user, different role — should succeed
	m2, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "org_admin",
	})
	require.NoError(t, err)
	require.NotNil(t, m2)
	assert.NotEqual(t, m1.ID, m2.ID)
}

func TestOrgStore_ListOrgMembers(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	_, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher",
	})
	require.NoError(t, err)

	members, err := orgStore.ListOrgMembers(ctx, org.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, user.ID, members[0].UserID)
	assert.Equal(t, user.Name, members[0].Name)
	assert.Equal(t, user.Email, members[0].Email)
	assert.Equal(t, "teacher", members[0].Role)
}

func TestOrgStore_ListOrgMembers_Empty(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())

	members, err := orgStore.ListOrgMembers(ctx, org.ID)
	require.NoError(t, err)
	assert.NotNil(t, members)
	assert.Len(t, members, 0)
}

func TestOrgStore_GetUserMemberships(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	_, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "student",
	})
	require.NoError(t, err)

	memberships, err := orgStore.GetUserMemberships(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, memberships, 1)
	assert.Equal(t, org.ID, memberships[0].OrgID)
	assert.Equal(t, org.Name, memberships[0].OrgName)
	assert.Equal(t, org.Slug, memberships[0].OrgSlug)
	assert.Equal(t, "student", memberships[0].Role)
}

// Plan 040 phase 6: a user with multiple roles in the same org (e.g.
// teacher + org_admin in Bridge Demo School) used to surface as one row
// per role pair, breaking React's `key={orgId}` rendering downstream.
// DISTINCT ON ensures one row per (orgId, role) — but a user with TWO
// distinct roles in the same org should still get TWO rows because the
// pair is unique. Lock both behaviors in tests.
// Plan 041 phase 1.2: ListOrgMembers DISTINCT ON (user_id, role) is a
// defensive guard. The schema's unique constraint
// org_memberships_org_user_role_idx physically prevents duplicate
// (org_id, user_id, role) rows, so the dedup is currently a no-op —
// but it documents intent and survives any future schema relaxation.
// What we CAN test: distinct roles for the same user surface as
// distinct rows (the pair is unique, no collapse).
func TestOrgStore_ListOrgMembers_DistinctRolesPreserved(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	_, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)
	_, err = orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "org_admin", Status: "active",
	})
	require.NoError(t, err)

	members, err := orgStore.ListOrgMembers(ctx, org.ID)
	require.NoError(t, err)
	roles := map[string]int{}
	for _, m := range members {
		if m.UserID == user.ID {
			roles[m.Role]++
		}
	}
	// Distinct roles → distinct rows.
	assert.Equal(t, 1, roles["teacher"])
	assert.Equal(t, 1, roles["org_admin"])
}

func TestOrgStore_GetUserMemberships_DistinctRolesPreserved(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	_, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher",
	})
	require.NoError(t, err)
	_, err = orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "org_admin",
	})
	require.NoError(t, err)

	memberships, err := orgStore.GetUserMemberships(ctx, user.ID)
	require.NoError(t, err)
	// Two distinct (orgId, role) pairs → two rows.
	require.Len(t, memberships, 2)
	roles := map[string]bool{memberships[0].Role: true, memberships[1].Role: true}
	assert.True(t, roles["teacher"])
	assert.True(t, roles["org_admin"])
}

func TestOrgStore_GetUserMemberships_Empty(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, userStore, t.Name())

	memberships, err := orgStore.GetUserMemberships(ctx, user.ID)
	require.NoError(t, err)
	assert.NotNil(t, memberships)
	assert.Len(t, memberships, 0)
}

func TestOrgStore_GetUserRolesInOrg(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	_, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)
	_, err = orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "org_admin", Status: "active",
	})
	require.NoError(t, err)

	roles, err := orgStore.GetUserRolesInOrg(ctx, org.ID, user.ID)
	require.NoError(t, err)
	assert.Len(t, roles, 2)
}

func TestOrgStore_GetUserRolesInOrg_OnlyActive(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	m, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)

	// Suspend the membership
	_, err = orgStore.UpdateMemberStatus(ctx, m.ID, "suspended")
	require.NoError(t, err)

	roles, err := orgStore.GetUserRolesInOrg(ctx, org.ID, user.ID)
	require.NoError(t, err)
	assert.Len(t, roles, 0, "suspended memberships should not be returned")
}

func TestOrgStore_GetUserRolesInOrg_NoMembership(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)

	roles, err := orgStore.GetUserRolesInOrg(context.Background(), "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Nil(t, roles)
}

func TestOrgStore_GetOrgMembership(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	m, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher",
	})
	require.NoError(t, err)

	fetched, err := orgStore.GetOrgMembership(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, m.ID, fetched.ID)
	assert.Equal(t, org.ID, fetched.OrgID)
	assert.Equal(t, user.ID, fetched.UserID)
}

func TestOrgStore_GetOrgMembership_NotFound(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)

	fetched, err := orgStore.GetOrgMembership(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, fetched)
}

func TestOrgStore_UpdateMemberStatus(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	m, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "student", Status: "pending",
	})
	require.NoError(t, err)
	assert.Equal(t, "pending", m.Status)

	updated, err := orgStore.UpdateMemberStatus(ctx, m.ID, "active")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "active", updated.Status)
	assert.Equal(t, m.ID, updated.ID)
}

func TestOrgStore_UpdateMemberStatus_NotFound(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)

	updated, err := orgStore.UpdateMemberStatus(context.Background(), "00000000-0000-0000-0000-000000000000", "active")
	assert.NoError(t, err)
	assert.Nil(t, updated)
}

func TestOrgStore_RemoveOrgMember(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)
	userStore := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgStore, t.Name())
	user := createTestUser(t, db, userStore, t.Name())

	m, err := orgStore.AddOrgMember(ctx, AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher",
	})
	require.NoError(t, err)

	removed, err := orgStore.RemoveOrgMember(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, removed)
	assert.Equal(t, m.ID, removed.ID)

	// Verify it's gone
	fetched, err := orgStore.GetOrgMembership(ctx, m.ID)
	assert.NoError(t, err)
	assert.Nil(t, fetched)
}

func TestOrgStore_RemoveOrgMember_NotFound(t *testing.T) {
	db := testDB(t)
	orgStore := NewOrgStore(db)

	removed, err := orgStore.RemoveOrgMember(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, removed)
}
