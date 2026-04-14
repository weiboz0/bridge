package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssignmentStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	assignments := NewAssignmentStore(db)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, _ := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Assign Course", GradeLevel: "K-5",
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, _ := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Assign Class", CreatedBy: user.ID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	a, err := assignments.CreateAssignment(ctx, CreateAssignmentInput{
		ClassID: class.ID, Title: "Homework 1", Description: "Do the thing",
	})
	require.NoError(t, err)
	require.NotNil(t, a)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM assignments WHERE id = $1", a.ID) })

	assert.Equal(t, "Homework 1", a.Title)

	fetched, err := assignments.GetAssignment(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, a.ID, fetched.ID)
}

func TestAssignmentStore_ListByClass(t *testing.T) {
	db := testDB(t)
	assignments := NewAssignmentStore(db)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, _ := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "List Course", GradeLevel: "K-5",
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, _ := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "List Class", CreatedBy: user.ID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM assignments WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	assignments.CreateAssignment(ctx, CreateAssignmentInput{ClassID: class.ID, Title: "HW 1"})
	assignments.CreateAssignment(ctx, CreateAssignmentInput{ClassID: class.ID, Title: "HW 2"})

	list, err := assignments.ListAssignmentsByClass(ctx, class.ID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestAssignmentStore_DeleteAssignment(t *testing.T) {
	db := testDB(t)
	assignments := NewAssignmentStore(db)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, _ := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Del Course", GradeLevel: "K-5",
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, _ := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Del Class", CreatedBy: user.ID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	a, _ := assignments.CreateAssignment(ctx, CreateAssignmentInput{ClassID: class.ID, Title: "To Delete"})
	deleted, err := assignments.DeleteAssignment(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)

	gone, _ := assignments.GetAssignment(ctx, a.ID)
	assert.Nil(t, gone)
}

func TestAssignmentStore_SubmissionAndGrade(t *testing.T) {
	db := testDB(t)
	assignments := NewAssignmentStore(db)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")
	course, _ := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: teacher.ID, Title: "Sub Course", GradeLevel: "6-8",
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, _ := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Sub Class", CreatedBy: teacher.ID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM submissions WHERE assignment_id IN (SELECT id FROM assignments WHERE class_id = $1)", class.ID)
		db.ExecContext(ctx, "DELETE FROM assignments WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	a, _ := assignments.CreateAssignment(ctx, CreateAssignmentInput{ClassID: class.ID, Title: "Submit HW"})

	// Submit
	sub, err := assignments.CreateSubmission(ctx, a.ID, student.ID, nil)
	require.NoError(t, err)
	require.NotNil(t, sub)
	assert.Equal(t, student.ID, sub.StudentID)

	// Duplicate submit returns nil
	dup, err := assignments.CreateSubmission(ctx, a.ID, student.ID, nil)
	assert.NoError(t, err)
	assert.Nil(t, dup)

	// List submissions
	subs, err := assignments.ListSubmissionsByAssignment(ctx, a.ID)
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, student.Name, subs[0].StudentName)

	// Grade
	feedback := "Good work!"
	graded, err := assignments.GradeSubmission(ctx, sub.ID, 95.0, &feedback)
	require.NoError(t, err)
	require.NotNil(t, graded)
	assert.Equal(t, 95.0, *graded.Grade)
	assert.Equal(t, "Good work!", *graded.Feedback)

	// GetSubmission
	fetched, err := assignments.GetSubmission(ctx, sub.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, sub.ID, fetched.ID)
	assert.Equal(t, 95.0, *fetched.Grade)
}

func TestAssignmentStore_GetSubmission_NotFound(t *testing.T) {
	db := testDB(t)
	assignments := NewAssignmentStore(db)

	s, err := assignments.GetSubmission(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, s)
}

func TestAssignmentStore_UpdateAssignment(t *testing.T) {
	db := testDB(t)
	assignments := NewAssignmentStore(db)
	classes := NewClassStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, _ := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Update Course", GradeLevel: "K-5",
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, _ := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Update Class", CreatedBy: user.ID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM assignments WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	a, _ := assignments.CreateAssignment(ctx, CreateAssignmentInput{
		ClassID: class.ID, Title: "Original HW", Description: "Do stuff",
	})

	newTitle := "Updated HW"
	newDesc := "Do more stuff"
	updated, err := assignments.UpdateAssignment(ctx, a.ID, UpdateAssignmentInput{
		Title:       &newTitle,
		Description: &newDesc,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Updated HW", updated.Title)
	assert.Equal(t, "Do more stuff", updated.Description)
}

func TestAssignmentStore_UpdateAssignment_NotFound(t *testing.T) {
	db := testDB(t)
	assignments := NewAssignmentStore(db)

	title := "X"
	updated, err := assignments.UpdateAssignment(context.Background(), "00000000-0000-0000-0000-000000000000", UpdateAssignmentInput{Title: &title})
	assert.NoError(t, err)
	assert.Nil(t, updated)
}
