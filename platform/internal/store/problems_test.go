package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupProblemEnv creates an org + user + course + topic, wires a ProblemStore,
// and registers cleanup. All subsequent test rows are freed either directly or
// by FK cascade when the fixture cleans up.
func setupProblemEnv(t *testing.T, suffix string) (*sql.DB, *ProblemStore, *Topic, *RegisteredUser) {
	t.Helper()
	db := testDB(t)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	courses := NewCourseStore(db)
	topics := NewTopicStore(db)
	problems := NewProblemStore(db)

	org := createTestOrg(t, db, orgs, suffix)
	user := createTestUser(t, db, users, suffix)

	ctx := context.Background()
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID,
		Title: "Problem Test Course " + suffix, GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	topic, err := topics.CreateTopic(ctx, CreateTopicInput{
		CourseID: course.ID, Title: "Arrays",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID) })

	return db, problems, topic, user
}

func TestProblemStore_CreateAndGet(t *testing.T) {
	db, problems, topic, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()

	starter := "def solve(): pass"
	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		TopicID: topic.ID, CreatedBy: user.ID,
		Title: "Two Sum", Description: "Find two numbers that sum to target.",
		Language: "python", StarterCode: starter,
	})
	require.NoError(t, err)
	require.NotNil(t, p)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", p.ID) })

	assert.Equal(t, "Two Sum", p.Title)
	assert.Equal(t, "python", p.Language)
	assert.Equal(t, 0, p.Order)
	require.NotNil(t, p.StarterCode)
	assert.Equal(t, starter, *p.StarterCode)

	got, err := problems.GetProblem(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, p.ID, got.ID)
}

func TestProblemStore_Get_NotFound_ReturnsNil(t *testing.T) {
	_, problems, _, _ := setupProblemEnv(t, t.Name())
	got, err := problems.GetProblem(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestProblemStore_CreateProblem_EmptyStarterStoresNull(t *testing.T) {
	_, problems, topic, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		TopicID: topic.ID, CreatedBy: user.ID,
		Title: "NoStarter", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { p := p; _ = p /* cleaned by topic cascade */ })
	assert.Nil(t, p.StarterCode, "empty starter_code should persist as NULL")
}

func TestProblemStore_ListProblemsByTopic_OrderedByOrderThenCreatedAt(t *testing.T) {
	_, problems, topic, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()

	// Insert three problems with order 2, 0, 1 (out-of-creation-order).
	p2, err := problems.CreateProblem(ctx, CreateProblemInput{
		TopicID: topic.ID, CreatedBy: user.ID, Title: "Third", Language: "python", Order: 2,
	})
	require.NoError(t, err)
	p0, err := problems.CreateProblem(ctx, CreateProblemInput{
		TopicID: topic.ID, CreatedBy: user.ID, Title: "First", Language: "python", Order: 0,
	})
	require.NoError(t, err)
	p1, err := problems.CreateProblem(ctx, CreateProblemInput{
		TopicID: topic.ID, CreatedBy: user.ID, Title: "Second", Language: "python", Order: 1,
	})
	require.NoError(t, err)

	list, err := problems.ListProblemsByTopic(ctx, topic.ID)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, p0.ID, list[0].ID)
	assert.Equal(t, p1.ID, list[1].ID)
	assert.Equal(t, p2.ID, list[2].ID)
}

func TestProblemStore_ListProblemsByTopic_EmptyReturnsEmptySlice(t *testing.T) {
	_, problems, topic, _ := setupProblemEnv(t, t.Name())
	list, err := problems.ListProblemsByTopic(context.Background(), topic.ID)
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Len(t, list, 0)
}

func TestProblemStore_UpdateProblem_PartialFields(t *testing.T) {
	_, problems, topic, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		TopicID: topic.ID, CreatedBy: user.ID,
		Title: "Original", Description: "original-desc", Language: "python",
	})
	require.NoError(t, err)

	newTitle := "Renamed"
	updated, err := problems.UpdateProblem(ctx, p.ID, UpdateProblemInput{Title: &newTitle})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Renamed", updated.Title)
	assert.Equal(t, "original-desc", updated.Description, "description should be unchanged")
	assert.Equal(t, "python", updated.Language, "language should be unchanged")
	assert.True(t, updated.UpdatedAt.After(p.UpdatedAt) || updated.UpdatedAt.Equal(p.UpdatedAt))
}

func TestProblemStore_UpdateProblem_ClearStarterCode(t *testing.T) {
	_, problems, topic, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		TopicID: topic.ID, CreatedBy: user.ID,
		Title: "Has Starter", Language: "python", StarterCode: "print('x')",
	})
	require.NoError(t, err)

	empty := ""
	updated, err := problems.UpdateProblem(ctx, p.ID, UpdateProblemInput{StarterCode: &empty})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Nil(t, updated.StarterCode, "empty pointer should clear starter_code to NULL")
}

func TestProblemStore_DeleteProblem(t *testing.T) {
	_, problems, topic, user := setupProblemEnv(t, t.Name())
	ctx := context.Background()

	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		TopicID: topic.ID, CreatedBy: user.ID,
		Title: "Doomed", Language: "python",
	})
	require.NoError(t, err)

	deleted, err := problems.DeleteProblem(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, p.ID, deleted.ID)

	got, err := problems.GetProblem(ctx, p.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}
