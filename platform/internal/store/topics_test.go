package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopicStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	topics := NewTopicStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Topic Test Course", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	topic, err := topics.CreateTopic(ctx, CreateTopicInput{
		CourseID: course.ID, Title: "Variables", Description: "Learn about vars",
	})
	require.NoError(t, err)
	require.NotNil(t, topic)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID) })

	assert.Equal(t, "Variables", topic.Title)
	assert.Equal(t, 0, topic.SortOrder)

	fetched, err := topics.GetTopic(ctx, topic.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, topic.ID, fetched.ID)
}

func TestTopicStore_AutoSortOrder(t *testing.T) {
	db := testDB(t)
	topics := NewTopicStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Sort Test", GradeLevel: "6-8",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", course.ID)
		db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
	})

	t1, err := topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "First"})
	require.NoError(t, err)
	assert.Equal(t, 0, t1.SortOrder)

	t2, err := topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "Second"})
	require.NoError(t, err)
	assert.Equal(t, 1, t2.SortOrder)
}

func TestTopicStore_ListTopicsByCourse(t *testing.T) {
	db := testDB(t)
	topics := NewTopicStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "List Test", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", course.ID)
		db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
	})

	_, err = topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "A"})
	require.NoError(t, err)
	_, err = topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "B"})
	require.NoError(t, err)

	list, err := topics.ListTopicsByCourse(ctx, course.ID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	assert.Equal(t, "A", list[0].Title)
	assert.Equal(t, "B", list[1].Title)
}

func TestTopicStore_UpdateTopic(t *testing.T) {
	db := testDB(t)
	topics := NewTopicStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Update Test", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", course.ID)
		db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
	})

	topic, err := topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "Original"})
	require.NoError(t, err)

	newTitle := "Updated"
	updated, err := topics.UpdateTopic(ctx, topic.ID, UpdateTopicInput{Title: &newTitle})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Updated", updated.Title)
}

func TestTopicStore_DeleteTopic(t *testing.T) {
	db := testDB(t)
	topics := NewTopicStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Delete Test", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	topic, err := topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "To Delete"})
	require.NoError(t, err)

	deleted, err := topics.DeleteTopic(ctx, topic.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)

	gone, err := topics.GetTopic(ctx, topic.ID)
	assert.NoError(t, err)
	assert.Nil(t, gone)
}

func TestTopicStore_ReorderTopics(t *testing.T) {
	db := testDB(t)
	topics := NewTopicStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Reorder Test", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", course.ID)
		db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
	})

	t1, err := topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "First"})
	require.NoError(t, err)
	t2, err := topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "Second"})
	require.NoError(t, err)

	// Reverse order
	err = topics.ReorderTopics(ctx, course.ID, []string{t2.ID, t1.ID})
	require.NoError(t, err)

	list, err := topics.ListTopicsByCourse(ctx, course.ID)
	require.NoError(t, err)
	assert.Equal(t, t2.ID, list[0].ID)
	assert.Equal(t, t1.ID, list[1].ID)
}

func TestTopicStore_GetTopic_NotFound(t *testing.T) {
	db := testDB(t)
	topics := NewTopicStore(db)

	topic, err := topics.GetTopic(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, topic)
}
