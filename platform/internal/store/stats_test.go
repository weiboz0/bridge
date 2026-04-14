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
