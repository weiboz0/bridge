package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSolutionEnv reuses setupProblemEnv to get an org/user/topic and returns
// a *sql.DB, ProblemStore, ProblemSolutionStore, the org ID, and the user ID.
func setupSolutionEnv(t *testing.T, suffix string) (*sql.DB, *ProblemStore, *ProblemSolutionStore, string /*orgID*/, string /*userID*/) {
	t.Helper()
	db, ps, topic, user := setupProblemEnv(t, suffix)

	ctx := context.Background()
	var orgID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT c.org_id FROM topics t JOIN courses c ON c.id = t.course_id WHERE t.id = $1`,
		topic.ID).Scan(&orgID))

	return db, ps, NewProblemSolutionStore(db), orgID, user.ID
}

// mustCreateSolution creates a solution for the given problem and registers cleanup.
func mustCreateSolution(t *testing.T, s *ProblemSolutionStore, in CreateSolutionInput) *ProblemSolution {
	t.Helper()
	sol, err := s.CreateSolution(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, sol)
	t.Cleanup(func() { s.DeleteSolution(context.Background(), sol.ID) })
	return sol
}

// --- CRUD ---

func TestSolutions_CreateAndGet(t *testing.T) {
	db, ps, s, orgID, userID := setupSolutionEnv(t, t.Name())
	ctx := context.Background()

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Two Sum "+t.Name(), userID, nil)

	title := "Brute Force"
	notes := "O(n^2) but easy to read"
	sol, err := s.CreateSolution(ctx, CreateSolutionInput{
		ProblemID:    p.ID,
		Language:     "python",
		Title:        &title,
		Code:         "def solve(): pass",
		Notes:        &notes,
		ApproachTags: []string{"brute-force"},
		CreatedBy:    userID,
	})
	require.NoError(t, err)
	require.NotNil(t, sol)
	t.Cleanup(func() { s.DeleteSolution(ctx, sol.ID) })

	assert.Equal(t, p.ID, sol.ProblemID)
	assert.Equal(t, "python", sol.Language)
	require.NotNil(t, sol.Title)
	assert.Equal(t, title, *sol.Title)
	assert.Equal(t, "def solve(): pass", sol.Code)
	require.NotNil(t, sol.Notes)
	assert.Equal(t, notes, *sol.Notes)
	assert.Equal(t, []string{"brute-force"}, sol.ApproachTags)
	assert.False(t, sol.IsPublished, "new solutions default to draft")
	assert.Equal(t, userID, sol.CreatedBy)

	got, err := s.GetSolution(ctx, sol.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, sol.ID, got.ID)
	assert.Equal(t, []string{"brute-force"}, got.ApproachTags)
}

func TestSolutions_GetNotFound_ReturnsNil(t *testing.T) {
	_, _, s, _, _ := setupSolutionEnv(t, t.Name())
	got, err := s.GetSolution(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSolutions_NilApproachTagsCoercedToEmpty(t *testing.T) {
	db, ps, s, orgID, userID := setupSolutionEnv(t, t.Name())

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "NilTags "+t.Name(), userID, nil)
	sol := mustCreateSolution(t, s, CreateSolutionInput{
		ProblemID: p.ID, Language: "python", Code: "pass", CreatedBy: userID,
		// ApproachTags intentionally nil
	})
	assert.Equal(t, []string{}, sol.ApproachTags)
}

// --- Update ---

func TestSolutions_UpdatePartialFields(t *testing.T) {
	db, ps, s, orgID, userID := setupSolutionEnv(t, t.Name())

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Update "+t.Name(), userID, nil)
	sol := mustCreateSolution(t, s, CreateSolutionInput{
		ProblemID: p.ID, Language: "python", Code: "original", CreatedBy: userID,
	})

	newCode := "updated"
	updated, err := s.UpdateSolution(context.Background(), sol.ID, UpdateSolutionInput{Code: &newCode})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "updated", updated.Code)
	assert.Nil(t, updated.Title, "title should remain nil/unchanged")
}

func TestSolutions_UpdateApproachTags(t *testing.T) {
	db, ps, s, orgID, userID := setupSolutionEnv(t, t.Name())

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Tags "+t.Name(), userID, nil)
	sol := mustCreateSolution(t, s, CreateSolutionInput{
		ProblemID:    p.ID,
		Language:     "python",
		Code:         "pass",
		CreatedBy:    userID,
		ApproachTags: []string{"brute-force"},
	})

	updated, err := s.UpdateSolution(context.Background(), sol.ID, UpdateSolutionInput{
		ApproachTags: []string{"dynamic-programming", "memoization"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"dynamic-programming", "memoization"}, updated.ApproachTags)

	// Clear tags with empty slice.
	cleared, err := s.UpdateSolution(context.Background(), sol.ID, UpdateSolutionInput{
		ApproachTags: []string{},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{}, cleared.ApproachTags)
}

func TestSolutions_UpdateNoFields_IsNoop(t *testing.T) {
	db, ps, s, orgID, userID := setupSolutionEnv(t, t.Name())

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Noop "+t.Name(), userID, nil)
	sol := mustCreateSolution(t, s, CreateSolutionInput{
		ProblemID: p.ID, Language: "go", Code: "package main", CreatedBy: userID,
	})

	same, err := s.UpdateSolution(context.Background(), sol.ID, UpdateSolutionInput{})
	require.NoError(t, err)
	require.NotNil(t, same)
	assert.Equal(t, sol.ID, same.ID)
	assert.Equal(t, "package main", same.Code)
}

// --- SetPublished ---

func TestSolutions_SetPublished_IdempotentRoundTrip(t *testing.T) {
	db, ps, s, orgID, userID := setupSolutionEnv(t, t.Name())

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Pub "+t.Name(), userID, nil)
	sol := mustCreateSolution(t, s, CreateSolutionInput{
		ProblemID: p.ID, Language: "python", Code: "pass", CreatedBy: userID,
	})
	assert.False(t, sol.IsPublished)

	ctx := context.Background()

	// publish
	pub, err := s.SetPublished(ctx, sol.ID, true)
	require.NoError(t, err)
	require.NotNil(t, pub)
	assert.True(t, pub.IsPublished)

	// publish again (idempotent — no error)
	pub2, err := s.SetPublished(ctx, sol.ID, true)
	require.NoError(t, err)
	require.NotNil(t, pub2)
	assert.True(t, pub2.IsPublished)

	// unpublish
	draft, err := s.SetPublished(ctx, sol.ID, false)
	require.NoError(t, err)
	require.NotNil(t, draft)
	assert.False(t, draft.IsPublished)
}

// --- ListByProblem ---

func TestSolutions_ListByProblem_DraftVsPublishedFilter(t *testing.T) {
	db, ps, s, orgID, userID := setupSolutionEnv(t, t.Name())
	ctx := context.Background()

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "List "+t.Name(), userID, nil)

	sol1 := mustCreateSolution(t, s, CreateSolutionInput{
		ProblemID: p.ID, Language: "python", Code: "sol1", CreatedBy: userID,
	})
	_ = mustCreateSolution(t, s, CreateSolutionInput{
		ProblemID: p.ID, Language: "go", Code: "sol2", CreatedBy: userID,
	})

	// Publish sol1, leave sol2 as draft.
	_, err := s.SetPublished(ctx, sol1.ID, true)
	require.NoError(t, err)

	// includeDrafts=true: both
	all, err := s.ListByProblem(ctx, p.ID, true)
	require.NoError(t, err)
	require.Len(t, all, 2)

	// includeDrafts=false: only published
	published, err := s.ListByProblem(ctx, p.ID, false)
	require.NoError(t, err)
	require.Len(t, published, 1)
	assert.Equal(t, sol1.ID, published[0].ID)
}

func TestSolutions_ListByProblem_EmptyReturnsEmptySlice(t *testing.T) {
	db, ps, s, orgID, userID := setupSolutionEnv(t, t.Name())

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Empty "+t.Name(), userID, nil)
	list, err := s.ListByProblem(context.Background(), p.ID, true)
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Len(t, list, 0)
}

// --- Delete ---

func TestSolutions_Delete_ReturnsDeletedRow(t *testing.T) {
	db, ps, s, orgID, userID := setupSolutionEnv(t, t.Name())
	ctx := context.Background()

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Delete "+t.Name(), userID, nil)
	sol := mustCreateSolution(t, s, CreateSolutionInput{
		ProblemID: p.ID, Language: "python", Code: "pass", CreatedBy: userID,
	})

	deleted, err := s.DeleteSolution(ctx, sol.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, sol.ID, deleted.ID)

	got, err := s.GetSolution(ctx, sol.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSolutions_Delete_NotFound_ReturnsNil(t *testing.T) {
	_, _, s, _, _ := setupSolutionEnv(t, t.Name())
	got, err := s.DeleteSolution(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// --- Cascade ---

func TestSolutions_CascadeDeleteWithProblem(t *testing.T) {
	db, ps, s, orgID, userID := setupSolutionEnv(t, t.Name())
	ctx := context.Background()

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Cascade "+t.Name(), userID, nil)

	sol, err := s.CreateSolution(ctx, CreateSolutionInput{
		ProblemID: p.ID, Language: "python", Code: "print('hi')", CreatedBy: userID,
	})
	require.NoError(t, err)
	require.NotNil(t, sol)

	// Delete the parent problem (cascade should take the solution with it).
	_, err = ps.DeleteProblem(ctx, p.ID)
	require.NoError(t, err)

	// Solution must be gone (ON DELETE CASCADE).
	got, err := s.GetSolution(ctx, sol.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}
