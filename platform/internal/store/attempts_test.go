package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAttemptEnv(t *testing.T, suffix string) (*AttemptStore, *Problem, *RegisteredUser, *RegisteredUser) {
	t.Helper()
	db, problems, topic, owner := setupProblemEnv(t, suffix)
	users := NewUserStore(db)
	attempts := NewAttemptStore(db)

	other := createTestUser(t, db, users, suffix+"-other")

	ctx := context.Background()
	_ = topic // kept for parity with older fixture; no longer used directly
	scopeID := owner.ID
	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		Scope: "personal", ScopeID: &scopeID, CreatedBy: owner.ID,
		Title:       "Attempt Test " + suffix,
		Description: "desc",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", p.ID) })

	return attempts, p, owner, other
}

func TestAttemptStore_CreateAndGet(t *testing.T) {
	attempts, p, user, _ := setupAttemptEnv(t, t.Name())
	ctx := context.Background()

	a, err := attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: user.ID, Language: "python", PlainText: "print(1)",
	})
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, "Untitled", a.Title, "empty title defaults to Untitled")
	assert.Equal(t, "python", a.Language)
	assert.Equal(t, "print(1)", a.PlainText)

	got, err := attempts.GetAttempt(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, a.ID, got.ID)
}

func TestAttemptStore_CreateAttempt_HonorsExplicitTitle(t *testing.T) {
	attempts, p, user, _ := setupAttemptEnv(t, t.Name())
	ctx := context.Background()

	a, err := attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: user.ID, Title: "Hashmap idea", Language: "python",
	})
	require.NoError(t, err)
	assert.Equal(t, "Hashmap idea", a.Title)
}

func TestAttemptStore_Get_NotFound_ReturnsNil(t *testing.T) {
	attempts, _, _, _ := setupAttemptEnv(t, t.Name())
	got, err := attempts.GetAttempt(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestAttemptStore_ListByUserAndProblem_OrderedByUpdatedDesc(t *testing.T) {
	attempts, p, user, _ := setupAttemptEnv(t, t.Name())
	ctx := context.Background()

	a1, err := attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: user.ID, Language: "python", PlainText: "v1",
	})
	require.NoError(t, err)
	a2, err := attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: user.ID, Language: "python", PlainText: "v2",
	})
	require.NoError(t, err)
	a3, err := attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: user.ID, Language: "python", PlainText: "v3",
	})
	require.NoError(t, err)

	// Touch a2 so it becomes the most recent.
	time.Sleep(15 * time.Millisecond)
	newText := "v2 edited"
	_, err = attempts.UpdateAttempt(ctx, a2.ID, UpdateAttemptInput{PlainText: &newText})
	require.NoError(t, err)

	list, err := attempts.ListByUserAndProblem(ctx, p.ID, user.ID)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, a2.ID, list[0].ID, "touched attempt is most recent")
	assert.Equal(t, a3.ID, list[1].ID)
	assert.Equal(t, a1.ID, list[2].ID)
}

func TestAttemptStore_ListByUserAndProblem_FiltersByUser(t *testing.T) {
	attempts, p, owner, other := setupAttemptEnv(t, t.Name())
	ctx := context.Background()

	ownerA, err := attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: owner.ID, Language: "python",
	})
	require.NoError(t, err)
	_, err = attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: other.ID, Language: "python",
	})
	require.NoError(t, err)

	list, err := attempts.ListByUserAndProblem(ctx, p.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, ownerA.ID, list[0].ID)
}

func TestAttemptStore_ListByUserAndProblem_EmptyReturnsEmptySlice(t *testing.T) {
	attempts, p, user, _ := setupAttemptEnv(t, t.Name())
	list, err := attempts.ListByUserAndProblem(context.Background(), p.ID, user.ID)
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Len(t, list, 0)
}

func TestAttemptStore_UpdateAttempt_TouchesUpdatedAt(t *testing.T) {
	attempts, p, user, _ := setupAttemptEnv(t, t.Name())
	ctx := context.Background()

	a, err := attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: user.ID, Language: "python", PlainText: "v1",
	})
	require.NoError(t, err)

	time.Sleep(15 * time.Millisecond)
	newText := "v2"
	updated, err := attempts.UpdateAttempt(ctx, a.ID, UpdateAttemptInput{PlainText: &newText})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "v2", updated.PlainText)
	assert.True(t, updated.UpdatedAt.After(a.UpdatedAt), "updated_at must advance")
}

func TestAttemptStore_DeleteAttempt(t *testing.T) {
	attempts, p, user, _ := setupAttemptEnv(t, t.Name())
	ctx := context.Background()

	a, err := attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: user.ID, Language: "python",
	})
	require.NoError(t, err)

	deleted, err := attempts.DeleteAttempt(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)

	got, err := attempts.GetAttempt(ctx, a.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestAttemptStore_UpdateLastTestResult_RoundTrip(t *testing.T) {
	attempts, p, user, _ := setupAttemptEnv(t, t.Name())
	ctx := context.Background()

	a, err := attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: user.ID, Language: "python",
	})
	require.NoError(t, err)
	assert.Nil(t, a.LastTestResult, "fresh attempt has no test result")

	summary := json.RawMessage(`{"summary":{"passed":2,"failed":1,"skipped":0,"total":3}}`)
	err = attempts.UpdateLastTestResult(ctx, a.ID, summary)
	require.NoError(t, err)

	got, err := attempts.GetAttempt(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.LastTestResult)
	assert.JSONEq(t, string(summary), string(*got.LastTestResult))
}

func TestAttemptStore_UpdateLastTestResult_DoesNotBumpUpdatedAt(t *testing.T) {
	attempts, p, user, _ := setupAttemptEnv(t, t.Name())
	ctx := context.Background()

	a, err := attempts.CreateAttempt(ctx, CreateAttemptInput{
		ProblemID: p.ID, UserID: user.ID, Language: "python",
	})
	require.NoError(t, err)

	originalUpdated := a.UpdatedAt
	time.Sleep(15 * time.Millisecond)

	err = attempts.UpdateLastTestResult(ctx, a.ID, json.RawMessage(`{"x":1}`))
	require.NoError(t, err)

	got, err := attempts.GetAttempt(ctx, a.ID)
	require.NoError(t, err)
	assert.True(t, got.UpdatedAt.Equal(originalUpdated),
		"writing a test result must not bump updated_at — Test is not an edit")
}
