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

func TestIsValidClassMemberRole(t *testing.T) {
	assert.True(t, IsValidClassMemberRole("instructor"))
	assert.True(t, IsValidClassMemberRole("ta"))
	assert.True(t, IsValidClassMemberRole("student"))
	assert.True(t, IsValidClassMemberRole("observer"))
	assert.True(t, IsValidClassMemberRole("guest"))
	assert.True(t, IsValidClassMemberRole("parent"))
	assert.False(t, IsValidClassMemberRole("admin"))
	assert.False(t, IsValidClassMemberRole("superuser"))
	assert.False(t, IsValidClassMemberRole(""))
}

func TestClassStore_ListClassesByUser(t *testing.T) {
	db := testDB(t)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")
	course, _ := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: teacher.ID, Title: "User Classes", GradeLevel: "K-5",
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, _ := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "User Class", CreatedBy: teacher.ID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	// Teacher is auto-added as instructor
	teacherClasses, err := classes.ListClassesByUser(ctx, teacher.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(teacherClasses), 1)
	found := false
	for _, c := range teacherClasses {
		if c.ID == class.ID {
			assert.Equal(t, "instructor", c.MemberRole)
			found = true
		}
	}
	assert.True(t, found, "teacher should see their class")

	// Add student
	classes.AddClassMember(ctx, AddClassMemberInput{
		ClassID: class.ID, UserID: student.ID, Role: "student",
	})

	studentClasses, err := classes.ListClassesByUser(ctx, student.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(studentClasses), 1)
	for _, c := range studentClasses {
		if c.ID == class.ID {
			assert.Equal(t, "student", c.MemberRole)
		}
	}

	// User with no classes
	noClasses, err := classes.ListClassesByUser(ctx, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Len(t, noClasses, 0)
}

func TestClassStore_TeacherCanViewStudentInCourse(t *testing.T) {
	db := testDB(t)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")
	stranger := createTestUser(t, db, users, t.Name()+"-stranger")

	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: teacher.ID,
		Title: "TC View Course", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	otherCourse, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: teacher.ID,
		Title: "Other Course", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", otherCourse.ID) })

	class, err := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "TC Class", CreatedBy: teacher.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	// teacher is instructor, student is student in this class
	_, err = classes.AddClassMember(ctx, AddClassMemberInput{
		ClassID: class.ID, UserID: teacher.ID, Role: "instructor",
	})
	require.NoError(t, err)
	_, err = classes.AddClassMember(ctx, AddClassMemberInput{
		ClassID: class.ID, UserID: student.ID, Role: "student",
	})
	require.NoError(t, err)

	// Happy path
	canView, err := classes.TeacherCanViewStudentInCourse(ctx, teacher.ID, student.ID, course.ID)
	require.NoError(t, err)
	assert.True(t, canView, "teacher of a class should see the student in that class")

	// Wrong course
	wrongCourse, err := classes.TeacherCanViewStudentInCourse(ctx, teacher.ID, student.ID, otherCourse.ID)
	require.NoError(t, err)
	assert.False(t, wrongCourse, "teacher does not get access in a different course")

	// Stranger as 'teacher'
	notTeacher, err := classes.TeacherCanViewStudentInCourse(ctx, stranger.ID, student.ID, course.ID)
	require.NoError(t, err)
	assert.False(t, notTeacher, "non-instructor cannot view")

	// Student is not in this teacher's class
	notStudent, err := classes.TeacherCanViewStudentInCourse(ctx, teacher.ID, stranger.ID, course.ID)
	require.NoError(t, err)
	assert.False(t, notStudent, "student must be a member of one of the teacher's classes")
}
