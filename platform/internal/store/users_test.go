package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserStore_RegisterAndGetByEmail(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db)
	ctx := context.Background()

	email := "register-test-" + t.Name() + "@example.com"
	user, err := store.RegisterUser(ctx, RegisterInput{
		Name:     "Test User",
		Email:    email,
		Password: "securepassword123",
	})
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "Test User", user.Name)
	assert.Equal(t, email, user.Email)

	// Get by email
	fetched, err := store.GetUserByEmail(ctx, email)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, user.ID, fetched.ID)
	assert.Equal(t, "active", fetched.Status)

	// Get by ID
	fetchedByID, err := store.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedByID)
	assert.Equal(t, user.ID, fetchedByID.ID)
	assert.Equal(t, "active", fetchedByID.Status)

	// Cleanup
	_, _ = db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
}

func TestUserStore_ListUsers_FiltersAndEnrichedShape(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	orgs := NewOrgStore(db)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]

	orgA := createTestOrg(t, db, orgs, "users-filter-a-"+suffix)
	orgB := createTestOrg(t, db, orgs, "users-filter-b-"+suffix)
	multi := createTestUser(t, db, users, "users-filter-multi-"+suffix)
	platformAdmin := createTestUser(t, db, users, "users-filter-platform-"+suffix)
	unassigned := createTestUser(t, db, users, "users-filter-unassigned-"+suffix)
	parent := createTestUser(t, db, users, "users-filter-parent-"+suffix)
	orgAdmin := createTestUser(t, db, users, "users-filter-org-admin-"+suffix)

	now := time.Now().UTC()
	addTestMembership(t, db, multi.ID, orgA.ID, "teacher", now.Add(-2*time.Hour))
	addTestMembership(t, db, multi.ID, orgB.ID, "student", now.Add(-time.Hour))
	addTestMembership(t, db, platformAdmin.ID, orgA.ID, "teacher", now.Add(-time.Hour))
	addTestMembership(t, db, parent.ID, orgB.ID, "parent", now.Add(-time.Hour))
	addTestMembership(t, db, orgAdmin.ID, orgA.ID, "org_admin", now.Add(-time.Hour))
	_, err := db.ExecContext(ctx, "UPDATE users SET is_platform_admin = true WHERE id = $1", platformAdmin.ID)
	require.NoError(t, err)

	all, err := users.ListUsers(ctx, ListUsersFilter{})
	require.NoError(t, err)
	allByID := adminUsersByID(all)
	require.Contains(t, allByID, multi.ID)
	assert.Equal(t, "active", allByID[multi.ID].Status)
	require.NotNil(t, allByID[multi.ID].OrgRole)
	assert.Equal(t, "teacher", *allByID[multi.ID].OrgRole, "earliest active membership wins")
	require.NotNil(t, allByID[multi.ID].OrgID)
	assert.Equal(t, orgA.ID, *allByID[multi.ID].OrgID)
	require.NotNil(t, allByID[multi.ID].OrgName)
	assert.Equal(t, orgA.Name, *allByID[multi.ID].OrgName)
	assert.True(t, allByID[multi.ID].HasPassword)
	assert.Nil(t, allByID[unassigned.ID].OrgRole)

	teacher := "teacher"
	teacherRows, err := users.ListUsers(ctx, ListUsersFilter{Role: &teacher})
	require.NoError(t, err)
	teacherIDs := adminUserIDs(teacherRows)
	assert.Contains(t, teacherIDs, multi.ID)
	assert.Contains(t, teacherIDs, platformAdmin.ID)

	student := "student"
	studentRows, err := users.ListUsers(ctx, ListUsersFilter{Role: &student})
	require.NoError(t, err)
	assert.NotContains(t, adminUserIDs(studentRows), multi.ID, "later membership should not drive primary-role filter")

	parentRole := "parent"
	parentRows, err := users.ListUsers(ctx, ListUsersFilter{Role: &parentRole})
	require.NoError(t, err)
	assert.Contains(t, adminUserIDs(parentRows), parent.ID)

	orgAdminRole := "org_admin"
	orgAdminRows, err := users.ListUsers(ctx, ListUsersFilter{Role: &orgAdminRole})
	require.NoError(t, err)
	assert.Contains(t, adminUserIDs(orgAdminRows), orgAdmin.ID)

	platformRole := "platform_admin"
	platformRows, err := users.ListUsers(ctx, ListUsersFilter{Role: &platformRole})
	require.NoError(t, err)
	assert.Contains(t, adminUserIDs(platformRows), platformAdmin.ID)

	unassignedRole := "unassigned"
	unassignedRows, err := users.ListUsers(ctx, ListUsersFilter{Role: &unassignedRole})
	require.NoError(t, err)
	unassignedIDs := adminUserIDs(unassignedRows)
	assert.Contains(t, unassignedIDs, unassigned.ID)
	assert.NotContains(t, unassignedIDs, platformAdmin.ID)

	orgRows, err := users.ListUsers(ctx, ListUsersFilter{OrgID: &orgB.ID})
	require.NoError(t, err)
	orgIDs := adminUserIDs(orgRows)
	assert.Contains(t, orgIDs, parent.ID)
	assert.NotContains(t, orgIDs, multi.ID, "primary membership is orgA, not later orgB")

	combinedRows, err := users.ListUsers(ctx, ListUsersFilter{Role: &teacher, OrgID: &orgA.ID})
	require.NoError(t, err)
	combinedIDs := adminUserIDs(combinedRows)
	assert.Contains(t, combinedIDs, multi.ID)
	assert.NotContains(t, combinedIDs, parent.ID)

	platformCombinedRows, err := users.ListUsers(ctx, ListUsersFilter{Role: &platformRole, OrgID: &orgA.ID})
	require.NoError(t, err)
	assert.Contains(t, adminUserIDs(platformCombinedRows), platformAdmin.ID)
}

func TestUserStore_GetAdminUserByID(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	orgs := NewOrgStore(db)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	org := createTestOrg(t, db, orgs, "admin-user-"+suffix)
	user := createTestUser(t, db, users, "admin-user-"+suffix)
	addTestMembership(t, db, user.ID, org.ID, "org_admin", time.Now().UTC())
	_, err := db.ExecContext(ctx, "UPDATE users SET status = 'suspended' WHERE id = $1", user.ID)
	require.NoError(t, err)

	got, err := users.GetAdminUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, user.ID, got.ID)
	assert.Equal(t, "suspended", got.Status)
	assert.True(t, got.HasPassword)
	require.NotNil(t, got.OrgRole)
	assert.Equal(t, "org_admin", *got.OrgRole)
	require.NotNil(t, got.OrgID)
	assert.Equal(t, org.ID, *got.OrgID)
	require.NotNil(t, got.OrgName)
	assert.Equal(t, org.Name, *got.OrgName)

	missing, err := users.GetAdminUserByID(ctx, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestUserStore_UpdateStatus(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	ctx := context.Background()
	user := createTestUser(t, db, users, "update-status-"+uuid.NewString()[:8])

	require.NoError(t, users.UpdateStatus(ctx, user.ID, "suspended"))
	got, err := users.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "suspended", got.Status)

	require.NoError(t, users.UpdateStatus(ctx, user.ID, "active"))
	got, err = users.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "active", got.Status)
}

func TestUserStore_UpdatePlatformAdmin(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	ctx := context.Background()
	user := createTestUser(t, db, users, "update-platform-admin-"+uuid.NewString()[:8])

	require.NoError(t, users.UpdatePlatformAdmin(ctx, user.ID, true))
	got, err := users.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.IsPlatformAdmin)

	require.NoError(t, users.UpdatePlatformAdmin(ctx, user.ID, false))
	got, err = users.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.IsPlatformAdmin)
}

func addTestMembership(t *testing.T, db *sql.DB, userID, orgID, role string, createdAt time.Time) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO org_memberships (id, org_id, user_id, role, status, created_at)
		VALUES ($1, $2, $3, $4, 'active', $5)`,
		uuid.NewString(), orgID, userID, role, createdAt,
	)
	require.NoError(t, err)
}

func adminUsersByID(users []AdminUser) map[string]AdminUser {
	byID := make(map[string]AdminUser, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	return byID
}

func adminUserIDs(users []AdminUser) map[string]bool {
	ids := make(map[string]bool, len(users))
	for _, u := range users {
		ids[u.ID] = true
	}
	return ids
}

func TestUserStore_RegisterUser_DuplicateEmail(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db)
	ctx := context.Background()

	email := "dup-test-" + t.Name() + "@example.com"
	user, err := store.RegisterUser(ctx, RegisterInput{
		Name: "First User", Email: email, Password: "password123",
	})
	require.NoError(t, err)
	require.NotNil(t, user)

	// Second registration should fail (unique constraint on email)
	_, err = store.RegisterUser(ctx, RegisterInput{
		Name: "Second User", Email: email, Password: "password456",
	})
	assert.Error(t, err)

	// Cleanup
	_, _ = db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
}

func TestUserStore_GetUserByID_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db)
	ctx := context.Background()

	user, err := store.GetUserByID(ctx, "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, user)
}

func TestUserStore_GetUserByEmail_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db)
	ctx := context.Background()

	user, err := store.GetUserByEmail(ctx, "nonexistent@example.com")
	assert.NoError(t, err)
	assert.Nil(t, user)
}
