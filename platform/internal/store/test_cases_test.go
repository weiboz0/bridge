package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

// setupTestCaseEnv creates a full org/user/course/topic/problem chain plus a
// second user ("other"), registers cleanup, and returns everything the tests need.
func setupTestCaseEnv(t *testing.T, suffix string) (
	tc *TestCaseStore,
	problem *Problem,
	owner, other *RegisteredUser,
) {
	t.Helper()
	db, problems, topic, user := setupProblemEnv(t, suffix)
	users := NewUserStore(db)
	tc = NewTestCaseStore(db)

	other = createTestUser(t, db, users, suffix+"-other")

	ctx := context.Background()
	p, err := problems.CreateProblem(ctx, CreateProblemInput{
		TopicID: topic.ID, CreatedBy: user.ID,
		Title: "TC Problem " + suffix, Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM problems WHERE id = $1", p.ID) })

	return tc, p, user, other
}

func TestTestCaseStore_CreateCanonicalAndGet(t *testing.T) {
	tc, p, _, _ := setupTestCaseEnv(t, t.Name())
	ctx := context.Background()

	c, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, Name: "Example 1",
		Stdin:          "1 2",
		ExpectedStdout: strPtr("3"),
		IsExample:      true,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Nil(t, c.OwnerID, "canonical case has no owner")
	assert.True(t, c.IsExample)
	require.NotNil(t, c.ExpectedStdout)
	assert.Equal(t, "3", *c.ExpectedStdout)

	got, err := tc.GetTestCase(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, c.ID, got.ID)
}

func TestTestCaseStore_CreatePrivate(t *testing.T) {
	tc, p, owner, _ := setupTestCaseEnv(t, t.Name())
	ctx := context.Background()

	c, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, OwnerID: &owner.ID,
		Name:      "my edge case",
		Stdin:     "-1 0",
		IsExample: false,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	require.NotNil(t, c.OwnerID)
	assert.Equal(t, owner.ID, *c.OwnerID)
	assert.Nil(t, c.ExpectedStdout, "no expected_stdout provided stores NULL")
}

func TestTestCaseStore_ListForViewer_MixesCanonicalAndOwnerPrivate(t *testing.T) {
	tc, p, owner, other := setupTestCaseEnv(t, t.Name())
	ctx := context.Background()

	canonicalExample, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, Name: "Ex1", Stdin: "1", ExpectedStdout: strPtr("1"), IsExample: true,
	})
	require.NoError(t, err)
	canonicalHidden, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, Name: "Hidden", Stdin: "x", ExpectedStdout: strPtr("y"), IsExample: false,
	})
	require.NoError(t, err)
	ownerPriv, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, OwnerID: &owner.ID, Name: "owner priv", Stdin: "a",
	})
	require.NoError(t, err)
	otherPriv, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, OwnerID: &other.ID, Name: "other priv", Stdin: "b",
	})
	require.NoError(t, err)

	list, err := tc.ListForViewer(ctx, p.ID, owner.ID)
	require.NoError(t, err)

	seen := map[string]bool{}
	for _, c := range list {
		seen[c.ID] = true
	}
	assert.True(t, seen[canonicalExample.ID], "canonical example visible to owner-viewer")
	assert.True(t, seen[canonicalHidden.ID], "canonical hidden also returned (handler layer redacts)")
	assert.True(t, seen[ownerPriv.ID], "owner's private case visible to owner")
	assert.False(t, seen[otherPriv.ID], "other user's private case hidden from owner")
	assert.Equal(t, 3, len(list))
}

func TestTestCaseStore_ListForViewer_CanonicalFirst(t *testing.T) {
	tc, p, owner, _ := setupTestCaseEnv(t, t.Name())
	ctx := context.Background()

	// Private first so that order-tie-breaker can't accidentally produce the
	// expected ordering.
	priv, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, OwnerID: &owner.ID, Name: "priv", Stdin: "a", Order: 0,
	})
	require.NoError(t, err)
	canon, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, Name: "canon", Stdin: "b", Order: 0,
	})
	require.NoError(t, err)

	list, err := tc.ListForViewer(ctx, p.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, canon.ID, list[0].ID, "canonical (owner_id IS NULL) sorts before private")
	assert.Equal(t, priv.ID, list[1].ID)
}

func TestTestCaseStore_ListCanonical_ExcludesPrivate(t *testing.T) {
	tc, p, owner, _ := setupTestCaseEnv(t, t.Name())
	ctx := context.Background()

	canon, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, Name: "canon", Stdin: "1",
	})
	require.NoError(t, err)
	_, err = tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, OwnerID: &owner.ID, Name: "priv", Stdin: "2",
	})
	require.NoError(t, err)

	list, err := tc.ListCanonical(context.Background(), p.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, canon.ID, list[0].ID)
	assert.Nil(t, list[0].OwnerID)
}

func TestTestCaseStore_UpdateTestCase_ClearExpectedStdout(t *testing.T) {
	tc, p, _, _ := setupTestCaseEnv(t, t.Name())
	ctx := context.Background()

	c, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, Stdin: "1", ExpectedStdout: strPtr("2"),
	})
	require.NoError(t, err)
	require.NotNil(t, c.ExpectedStdout)

	empty := ""
	updated, err := tc.UpdateTestCase(ctx, c.ID, UpdateTestCaseInput{ExpectedStdout: &empty})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Nil(t, updated.ExpectedStdout, "ptr(\"\") clears expected_stdout")
}

func TestTestCaseStore_UpdateTestCase_ToggleIsExample(t *testing.T) {
	tc, p, _, _ := setupTestCaseEnv(t, t.Name())
	ctx := context.Background()

	c, err := tc.CreateTestCase(ctx, CreateTestCaseInput{
		ProblemID: p.ID, Stdin: "1", IsExample: false,
	})
	require.NoError(t, err)
	assert.False(t, c.IsExample)

	updated, err := tc.UpdateTestCase(ctx, c.ID, UpdateTestCaseInput{IsExample: boolPtr(true)})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.True(t, updated.IsExample)
}

func TestTestCaseStore_DeleteTestCase(t *testing.T) {
	tc, p, _, _ := setupTestCaseEnv(t, t.Name())
	ctx := context.Background()

	c, err := tc.CreateTestCase(ctx, CreateTestCaseInput{ProblemID: p.ID, Stdin: "x"})
	require.NoError(t, err)

	deleted, err := tc.DeleteTestCase(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)

	got, err := tc.GetTestCase(ctx, c.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}
