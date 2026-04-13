package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportStore_CreateAndList(t *testing.T) {
	db := testDB(t)
	reports := NewReportStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	student := createTestUser(t, db, users, t.Name()+"-student")
	parent := createTestUser(t, db, users, t.Name()+"-parent")

	report, err := reports.CreateReport(ctx, CreateReportInput{
		StudentID:   student.ID,
		GeneratedBy: parent.ID,
		PeriodStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		Content:     "Student made great progress in Python fundamentals.",
		Summary:     `{"grade":"A","topics":["variables","loops"]}`,
	})
	require.NoError(t, err)
	require.NotNil(t, report)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM parent_reports WHERE id = $1", report.ID) })

	assert.Equal(t, student.ID, report.StudentID)
	assert.Equal(t, "Student made great progress in Python fundamentals.", report.Content)

	// Get by ID
	fetched, err := reports.GetReport(ctx, report.ID)
	require.NoError(t, err)
	assert.Equal(t, report.ID, fetched.ID)

	// List by student
	list, err := reports.ListReportsByStudent(ctx, student.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestReportStore_GetNotFound(t *testing.T) {
	db := testDB(t)
	reports := NewReportStore(db)

	r, err := reports.GetReport(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, r)
}

func TestReportStore_ListEmpty(t *testing.T) {
	db := testDB(t)
	reports := NewReportStore(db)

	list, err := reports.ListReportsByStudent(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Len(t, list, 0)
}
