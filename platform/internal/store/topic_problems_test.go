package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTopicProblemEnv reuses setupProblemEnv to get a full org/user/topic
// fixture and returns the raw *sql.DB alongside the three stores under test.
func setupTopicProblemEnv(t *testing.T, suffix string) (
	*sql.DB, *ProblemStore, *TopicProblemStore, string /*topicID*/, string /*orgID*/, string /*userID*/,
) {
	t.Helper()
	db, ps, topic, user := setupProblemEnv(t, suffix)

	ctx := context.Background()
	var orgID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT c.org_id FROM topics t JOIN courses c ON c.id = t.course_id WHERE t.id = $1`,
		topic.ID).Scan(&orgID))

	return db, ps, NewTopicProblemStore(db), topic.ID, orgID, user.ID
}

// TestTopicProblems_AttachDetachReorder covers the basic happy path:
// attach two problems, verify IsAttached, update sort order, detach one.
func TestTopicProblems_AttachDetachReorder(t *testing.T) {
	db, ps, tps, topicID, orgID, userID := setupTopicProblemEnv(t, t.Name())
	ctx := context.Background()

	p1 := mustCreateProblem(t, db, ps, "org", &orgID, "published", "P1 "+t.Name(), userID, nil)
	p2 := mustCreateProblem(t, db, ps, "org", &orgID, "published", "P2 "+t.Name(), userID, nil)

	// Attach both.
	a1, err := tps.Attach(ctx, topicID, p1.ID, 0, userID)
	require.NoError(t, err)
	require.NotNil(t, a1)
	assert.Equal(t, topicID, a1.TopicID)
	assert.Equal(t, p1.ID, a1.ProblemID)
	assert.Equal(t, 0, a1.SortOrder)
	assert.Equal(t, userID, a1.AttachedBy)

	a2, err := tps.Attach(ctx, topicID, p2.ID, 1, userID)
	require.NoError(t, err)
	require.NotNil(t, a2)

	// IsAttached.
	ok, err := tps.IsAttached(ctx, topicID, p1.ID)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = tps.IsAttached(ctx, topicID, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.False(t, ok)

	// SetSortOrder.
	updated, err := tps.SetSortOrder(ctx, topicID, p1.ID, 99)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, 99, updated.SortOrder)

	// Detach p1.
	removed, err := tps.Detach(ctx, topicID, p1.ID)
	require.NoError(t, err)
	assert.True(t, removed)

	// p1 no longer attached.
	ok, err = tps.IsAttached(ctx, topicID, p1.ID)
	require.NoError(t, err)
	assert.False(t, ok)

	// Detach same pair again: no row → false.
	removed, err = tps.Detach(ctx, topicID, p1.ID)
	require.NoError(t, err)
	assert.False(t, removed)
}

// TestTopicProblems_DuplicateAttachReturnsAlreadyAttached verifies that a
// second Attach for the same (topic, problem) pair returns ErrAlreadyAttached.
func TestTopicProblems_DuplicateAttachReturnsAlreadyAttached(t *testing.T) {
	db, ps, tps, topicID, orgID, userID := setupTopicProblemEnv(t, t.Name())
	ctx := context.Background()

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Dup "+t.Name(), userID, nil)

	// First attach succeeds.
	_, err := tps.Attach(ctx, topicID, p.ID, 0, userID)
	require.NoError(t, err)
	t.Cleanup(func() { tps.Detach(ctx, topicID, p.ID) })

	// Second attach returns ErrAlreadyAttached.
	_, err = tps.Attach(ctx, topicID, p.ID, 1, userID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyAttached)
}

// TestTopicProblems_ListTopicsByProblem verifies that attaching a problem to
// multiple topics is reflected in ListTopicsByProblem.
func TestTopicProblems_ListTopicsByProblem(t *testing.T) {
	db, ps, tps, topicID, orgID, userID := setupTopicProblemEnv(t, t.Name())
	ctx := context.Background()

	// Create a second topic under the same course.
	var courseID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT course_id FROM topics WHERE id = $1`, topicID).Scan(&courseID))

	topics := NewTopicStore(db)
	topic2, err := topics.CreateTopic(ctx, CreateTopicInput{CourseID: courseID, Title: "Topic2 " + t.Name()})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic2.ID) })

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Multi "+t.Name(), userID, nil)

	_, err = tps.Attach(ctx, topicID, p.ID, 0, userID)
	require.NoError(t, err)
	t.Cleanup(func() { tps.Detach(ctx, topicID, p.ID) })

	_, err = tps.Attach(ctx, topic2.ID, p.ID, 0, userID)
	require.NoError(t, err)
	t.Cleanup(func() { tps.Detach(ctx, topic2.ID, p.ID) })

	list, err := tps.ListTopicsByProblem(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)

	ids := map[string]bool{list[0]: true, list[1]: true}
	assert.True(t, ids[topicID])
	assert.True(t, ids[topic2.ID])
}

func TestTopicProblems_ListTopicsByProblem_Empty(t *testing.T) {
	db, ps, tps, _, orgID, userID := setupTopicProblemEnv(t, t.Name())

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "Unattached "+t.Name(), userID, nil)
	list, err := tps.ListTopicsByProblem(context.Background(), p.ID)
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Len(t, list, 0)
}

// TestTopicProblems_SetSortOrder_NotFound verifies that SetSortOrder on a
// non-existent pair returns nil without error.
func TestTopicProblems_SetSortOrder_NotFound(t *testing.T) {
	_, _, tps, _, _, _ := setupTopicProblemEnv(t, t.Name())
	got, err := tps.SetSortOrder(context.Background(),
		"00000000-0000-0000-0000-000000000000",
		"00000000-0000-0000-0000-000000000001",
		42)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestTopicProblems_CascadeOnTopicDelete verifies that deleting a topic removes
// its topic_problems rows via ON DELETE CASCADE.
func TestTopicProblems_CascadeOnTopicDelete(t *testing.T) {
	db, ps, tps, topicID, orgID, userID := setupTopicProblemEnv(t, t.Name())
	ctx := context.Background()

	// Create a second topic so we can delete the first without removing the course.
	var courseID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT course_id FROM topics WHERE id = $1`, topicID).Scan(&courseID))
	topics := NewTopicStore(db)
	extraTopic, err := topics.CreateTopic(ctx, CreateTopicInput{CourseID: courseID, Title: "Extra " + t.Name()})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", extraTopic.ID) })

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "CascTopic "+t.Name(), userID, nil)

	_, err = tps.Attach(ctx, topicID, p.ID, 0, userID)
	require.NoError(t, err)

	// Delete the topic — should cascade the attachment row.
	_, err = db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topicID)
	require.NoError(t, err)

	ok, err := tps.IsAttached(ctx, topicID, p.ID)
	require.NoError(t, err)
	assert.False(t, ok, "attachment must be gone after topic delete")
}

// TestTopicProblems_CascadeOnProblemDelete verifies that deleting a problem
// removes its topic_problems rows via ON DELETE CASCADE.
func TestTopicProblems_CascadeOnProblemDelete(t *testing.T) {
	db, ps, tps, topicID, orgID, userID := setupTopicProblemEnv(t, t.Name())
	ctx := context.Background()

	p := mustCreateProblem(t, db, ps, "org", &orgID, "published", "CascProb "+t.Name(), userID, nil)

	_, err := tps.Attach(ctx, topicID, p.ID, 0, userID)
	require.NoError(t, err)

	ok, err := tps.IsAttached(ctx, topicID, p.ID)
	require.NoError(t, err)
	assert.True(t, ok)

	// Delete the problem — should cascade the attachment row.
	_, err = ps.DeleteProblem(ctx, p.ID)
	require.NoError(t, err)

	ok, err = tps.IsAttached(ctx, topicID, p.ID)
	require.NoError(t, err)
	assert.False(t, ok, "attachment must be gone after problem delete")
}
