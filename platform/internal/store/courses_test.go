package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestCourseOrg(t *testing.T, db interface {
	ExecContext(ctx context.Context, query string, args ...any) (interface{ RowsAffected() (int64, error) }, error)
}, orgStore *OrgStore) *Org {
	t.Helper()
	// Use the shared createTestOrg helper via OrgStore
	ctx := context.Background()
	org, err := orgStore.CreateOrg(ctx, CreateOrgInput{
		Name: "Course Test Org", Slug: "course-test-" + t.Name(),
		Type: "school", ContactEmail: "course@example.com", ContactName: "Admin",
	})
	require.NoError(t, err)
	return org
}

func TestCourseStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())

	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID,
		Title: "Test Course", Description: "A test course",
		GradeLevel: "6-8", Language: "python",
	})
	require.NoError(t, err)
	require.NotNil(t, course)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	assert.Equal(t, "Test Course", course.Title)
	assert.Equal(t, "python", course.Language)
	assert.Equal(t, "6-8", course.GradeLevel)
	assert.False(t, course.IsPublished)

	fetched, err := courses.GetCourse(ctx, course.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, course.ID, fetched.ID)
}

func TestCourseStore_GetCourse_NotFound(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)

	c, err := courses.GetCourse(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, c)
}

func TestCourseStore_ListCoursesByOrg(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())

	c1, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Course 1", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", c1.ID) })

	c2, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Course 2", GradeLevel: "9-12",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", c2.ID) })

	list, err := courses.ListCoursesByOrg(ctx, org.ID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestCourseStore_ListCoursesByCreator(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())

	c, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "My Course", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", c.ID) })

	list, err := courses.ListCoursesByCreator(ctx, user.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 1)
}

func TestCourseStore_UpdateCourse(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())

	c, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Original", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", c.ID) })

	newTitle := "Updated Title"
	published := true
	updated, err := courses.UpdateCourse(ctx, c.ID, UpdateCourseInput{
		Title: &newTitle, IsPublished: &published,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Updated Title", updated.Title)
	assert.True(t, updated.IsPublished)
	assert.Equal(t, "K-5", updated.GradeLevel) // unchanged
}

func TestCourseStore_UpdateCourse_NotFound(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)

	title := "X"
	updated, err := courses.UpdateCourse(context.Background(), "00000000-0000-0000-0000-000000000000", UpdateCourseInput{Title: &title})
	assert.NoError(t, err)
	assert.Nil(t, updated)
}

func TestCourseStore_DeleteCourse(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())

	c, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "To Delete", GradeLevel: "6-8",
	})
	require.NoError(t, err)

	deleted, err := courses.DeleteCourse(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, c.ID, deleted.ID)

	// Verify gone
	gone, err := courses.GetCourse(ctx, c.ID)
	assert.NoError(t, err)
	assert.Nil(t, gone)
}

func TestCourseStore_DeleteCourse_NotFound(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)

	deleted, err := courses.DeleteCourse(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestCourseStore_CloneCourse(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())

	orig, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Original Course",
		Description: "Desc", GradeLevel: "9-12", Language: "javascript",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", orig.ID) })

	// Add a topic to the original WITH lesson_content + starter_code set
	// (the deprecated Plan 044 columns). Plan 044 phase 4 explicitly
	// stops the clone from carrying these forward; this test guards
	// against accidental regression.
	_, err = db.ExecContext(ctx,
		`INSERT INTO topics (id, course_id, title, sort_order, lesson_content, starter_code, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, 'Topic 1', 0, '{"blocks":[{"type":"p","value":"x"}]}'::jsonb, 'print(1)', now(), now())`, orig.ID)
	require.NoError(t, err)

	cloned, err := courses.CloneCourse(ctx, orig.ID, user.ID)
	require.NoError(t, err)
	require.NotNil(t, cloned)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", cloned.ID)
		db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", cloned.ID)
	})

	assert.Equal(t, "Original Course (Copy)", cloned.Title)
	assert.Equal(t, orig.OrgID, cloned.OrgID)
	assert.Equal(t, "javascript", cloned.Language)
	assert.False(t, cloned.IsPublished)
	assert.NotEqual(t, orig.ID, cloned.ID)

	// Verify topic was cloned
	var topicCount int
	err = db.QueryRow("SELECT count(*) FROM topics WHERE course_id = $1", cloned.ID).Scan(&topicCount)
	require.NoError(t, err)
	assert.Equal(t, 1, topicCount)

	// Plan 044 phase 4 regression: the cloned topic must NOT carry
	// lesson_content or starter_code forward. lesson_content defaults
	// to '{}' (empty jsonb object) and starter_code to NULL on insert.
	var lessonJSON []byte
	var starter sql.NullString
	err = db.QueryRow(
		`SELECT lesson_content, starter_code FROM topics WHERE course_id = $1`,
		cloned.ID,
	).Scan(&lessonJSON, &starter)
	require.NoError(t, err)
	assert.JSONEq(t, "{}", string(lessonJSON), "lesson_content should not be cloned")
	assert.False(t, starter.Valid, "starter_code should not be cloned (NULL)")
}

func TestCourseStore_CloneCourse_NotFound(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)

	cloned, err := courses.CloneCourse(context.Background(), "00000000-0000-0000-0000-000000000000", "user-1")
	assert.NoError(t, err)
	assert.Nil(t, cloned)
}

func TestCourseStore_UserHasAccessToCourse(t *testing.T) {
	db := testDB(t)
	courses := NewCourseStore(db)
	classes := NewClassStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	creator := createTestUser(t, db, users, t.Name()+"-creator")
	member := createTestUser(t, db, users, t.Name()+"-member")
	stranger := createTestUser(t, db, users, t.Name()+"-stranger")

	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: creator.ID, Title: "Access Test", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, err := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Access Class", Term: "Fall", CreatedBy: creator.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	_, err = classes.AddClassMember(ctx, AddClassMemberInput{
		ClassID: class.ID, UserID: member.ID, Role: "student",
	})
	require.NoError(t, err)

	memberHas, err := courses.UserHasAccessToCourse(ctx, course.ID, member.ID)
	require.NoError(t, err)
	assert.True(t, memberHas, "member of a class in the course should have access")

	strangerHas, err := courses.UserHasAccessToCourse(ctx, course.ID, stranger.ID)
	require.NoError(t, err)
	assert.False(t, strangerHas, "unrelated user should NOT have access")
}
