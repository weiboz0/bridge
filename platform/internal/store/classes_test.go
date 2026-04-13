package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Class Test Course", GradeLevel: "K-5", Language: "javascript",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, err := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Test Class", Term: "Fall 2026", CreatedBy: user.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, class)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	assert.Equal(t, "Test Class", class.Title)
	assert.Equal(t, "active", class.Status)
	assert.Len(t, class.JoinCode, 8)

	// Verify class fetches correctly
	fetched, err := classes.GetClass(ctx, class.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, class.ID, fetched.ID)

	// Verify new_classroom was created with course language
	classroom, err := classes.GetClassroom(ctx, class.ID)
	require.NoError(t, err)
	require.NotNil(t, classroom)
	assert.Equal(t, "javascript", classroom.EditorMode)

	// Verify creator was added as instructor
	members, err := classes.ListClassMembers(ctx, class.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, "instructor", members[0].Role)
}

func TestClassStore_ListClassesByOrg(t *testing.T) {
	db := testDB(t)
	classes := NewClassStore(db)
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
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, err := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Active Class", CreatedBy: user.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	list, err := classes.ListClassesByOrg(ctx, org.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 1)
}

func TestClassStore_ArchiveClass(t *testing.T) {
	db := testDB(t)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Archive Test", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, err := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "To Archive", CreatedBy: user.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	archived, err := classes.ArchiveClass(ctx, class.ID)
	require.NoError(t, err)
	require.NotNil(t, archived)
	assert.Equal(t, "archived", archived.Status)

	// Archived class should not appear in org list
	list, err := classes.ListClassesByOrg(ctx, org.ID)
	require.NoError(t, err)
	for _, c := range list {
		assert.NotEqual(t, class.ID, c.ID)
	}
}

func TestClassStore_JoinClassByCode(t *testing.T) {
	db := testDB(t)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	creator := createTestUser(t, db, users, t.Name()+"-creator")
	student := createTestUser(t, db, users, t.Name()+"-student")
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: creator.ID, Title: "Join Test", GradeLevel: "6-8",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, err := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Join Class", CreatedBy: creator.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	result, err := classes.JoinClassByCode(ctx, class.JoinCode, student.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, class.ID, result.Class.ID)
	assert.Equal(t, "student", result.Membership.Role)
}

func TestClassStore_JoinClassByCode_InvalidCode(t *testing.T) {
	db := testDB(t)
	classes := NewClassStore(db)

	result, err := classes.JoinClassByCode(context.Background(), "INVALID1", "user-1")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestClassStore_AddAndRemoveClassMember(t *testing.T) {
	db := testDB(t)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	creator := createTestUser(t, db, users, t.Name()+"-creator")
	member := createTestUser(t, db, users, t.Name()+"-member")
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: creator.ID, Title: "Member Test", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, err := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Member Class", CreatedBy: creator.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	// Add member
	m, err := classes.AddClassMember(ctx, AddClassMemberInput{
		ClassID: class.ID, UserID: member.ID, Role: "ta",
	})
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "ta", m.Role)

	// Update role
	updated, err := classes.UpdateClassMemberRole(ctx, m.ID, "instructor")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "instructor", updated.Role)

	// Remove
	removed, err := classes.RemoveClassMember(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, removed)

	// Verify gone
	gone, err := classes.GetClassMembership(ctx, m.ID)
	assert.NoError(t, err)
	assert.Nil(t, gone)
}
