package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsStore_GetAdminStats(t *testing.T) {
	db := testDB(t)
	stats := NewStatsStore(db)

	result, err := stats.GetAdminStats(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	// Should return non-negative counts
	assert.GreaterOrEqual(t, result.TotalUsers, 0)
	assert.GreaterOrEqual(t, result.ActiveOrgs, 0)
	assert.GreaterOrEqual(t, result.PendingOrgs, 0)
}

func TestStatsStore_GetOrgDashboardStats(t *testing.T) {
	db := testDB(t)
	stats := NewStatsStore(db)
	orgs := NewOrgStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())

	result, err := stats.GetOrgDashboardStats(ctx, org.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.TeacherCount)
	assert.Equal(t, 0, result.StudentCount)
	assert.Equal(t, 0, result.CourseCount)
	assert.Equal(t, 0, result.ClassCount)
}

// Plan 041 phase 1.2: counts use COUNT(DISTINCT user_id) defensively.
// The schema's unique constraint on (org_id, user_id, role) prevents
// the duplicate scenario from arising, so we can't exercise it in a
// test — but the COUNT(DISTINCT ...) shape stays as documentation of
// intent and a guard against future schema relaxation. What we CAN
// verify: counts are unaffected by other-org memberships of the same
// users.
func TestStatsStore_GetOrgDashboardStats_OtherOrgMembersIgnored(t *testing.T) {
	db := testDB(t)
	stats := NewStatsStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	orgA := createTestOrg(t, db, orgs, t.Name()+"-a")
	orgB := createTestOrg(t, db, orgs, t.Name()+"-b")
	user := createTestUser(t, db, users, t.Name())

	// User is a teacher in BOTH orgs. orgA's headline must count 1.
	_, err := orgs.AddOrgMember(ctx, AddMemberInput{
		OrgID: orgA.ID, UserID: user.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)
	_, err = orgs.AddOrgMember(ctx, AddMemberInput{
		OrgID: orgB.ID, UserID: user.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)

	result, err := stats.GetOrgDashboardStats(ctx, orgA.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, result.TeacherCount)
}
