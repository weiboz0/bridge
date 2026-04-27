package store

import (
	"context"
	"database/sql"
)

// StatsStore provides aggregation queries for dashboard stats.
type StatsStore struct {
	db *sql.DB
}

func NewStatsStore(db *sql.DB) *StatsStore {
	return &StatsStore{db: db}
}

type AdminStats struct {
	PendingOrgs int `json:"pendingOrgs"`
	ActiveOrgs  int `json:"activeOrgs"`
	TotalUsers  int `json:"totalUsers"`
}

func (s *StatsStore) GetAdminStats(ctx context.Context) (*AdminStats, error) {
	var stats AdminStats
	err := s.db.QueryRowContext(ctx,
		`SELECT
			COALESCE((SELECT count(*) FROM organizations WHERE status = 'pending'), 0),
			COALESCE((SELECT count(*) FROM organizations WHERE status = 'active'), 0),
			COALESCE((SELECT count(*) FROM users), 0)`,
	).Scan(&stats.PendingOrgs, &stats.ActiveOrgs, &stats.TotalUsers)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

type OrgDashboardStats struct {
	TeacherCount int `json:"teacherCount"`
	StudentCount int `json:"studentCount"`
	CourseCount  int `json:"courseCount"`
	ClassCount   int `json:"classCount"`
}

func (s *StatsStore) GetOrgDashboardStats(ctx context.Context, orgID string) (*OrgDashboardStats, error) {
	// Count distinct users per role so the headline number matches the
	// row count the /org/teachers and /org/students list pages render.
	// Plain count(*) inflates the headline whenever a single user has
	// duplicate (user_id, role, status='active') rows in the same org —
	// rare but possible after an add/remove/re-add cycle.
	var stats OrgDashboardStats
	err := s.db.QueryRowContext(ctx,
		`SELECT
			COALESCE((SELECT count(DISTINCT user_id) FROM org_memberships WHERE org_id = $1 AND role = 'teacher' AND status = 'active'), 0),
			COALESCE((SELECT count(DISTINCT user_id) FROM org_memberships WHERE org_id = $1 AND role = 'student' AND status = 'active'), 0),
			COALESCE((SELECT count(*) FROM courses WHERE org_id = $1), 0),
			COALESCE((SELECT count(*) FROM classes WHERE org_id = $1 AND status = 'active'), 0)`,
		orgID,
	).Scan(&stats.TeacherCount, &stats.StudentCount, &stats.CourseCount, &stats.ClassCount)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}
