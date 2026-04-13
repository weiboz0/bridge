package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassroomStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	classrooms := NewClassroomStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name())

	cr, err := classrooms.CreateClassroom(ctx, CreateClassroomInput{
		TeacherID: user.ID, Name: "Test Classroom",
		GradeLevel: "6-8", EditorMode: "python",
	})
	require.NoError(t, err)
	require.NotNil(t, cr)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM classroom_members WHERE classroom_id = $1", cr.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", cr.ID)
	})

	assert.Equal(t, "Test Classroom", cr.Name)
	assert.Equal(t, "python", cr.EditorMode)
	assert.Len(t, cr.JoinCode, 8)

	fetched, err := classrooms.GetClassroom(ctx, cr.ID)
	require.NoError(t, err)
	assert.Equal(t, cr.ID, fetched.ID)
}

func TestClassroomStore_ListClassrooms(t *testing.T) {
	db := testDB(t)
	classrooms := NewClassroomStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name())

	cr, err := classrooms.CreateClassroom(ctx, CreateClassroomInput{
		TeacherID: user.ID, Name: "List Classroom",
		GradeLevel: "K-5", EditorMode: "blockly",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", cr.ID)
	})

	list, err := classrooms.ListClassrooms(ctx, user.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 1)
}

func TestClassroomStore_JoinByCode(t *testing.T) {
	db := testDB(t)
	classrooms := NewClassroomStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")

	cr, err := classrooms.CreateClassroom(ctx, CreateClassroomInput{
		TeacherID: teacher.ID, Name: "Join Classroom",
		GradeLevel: "9-12", EditorMode: "javascript",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM classroom_members WHERE classroom_id = $1", cr.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", cr.ID)
	})

	// Find by join code
	found, err := classrooms.GetClassroomByJoinCode(ctx, cr.JoinCode)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, cr.ID, found.ID)

	// Join
	m, err := classrooms.JoinClassroom(ctx, cr.ID, student.ID)
	require.NoError(t, err)
	require.NotNil(t, m)

	// Duplicate join returns nil
	dup, err := classrooms.JoinClassroom(ctx, cr.ID, student.ID)
	assert.NoError(t, err)
	assert.Nil(t, dup)

	// List members
	members, err := classrooms.GetClassroomMembers(ctx, cr.ID)
	require.NoError(t, err)
	assert.Len(t, members, 1)
	assert.Equal(t, student.ID, members[0].UserID)
}

func TestClassroomStore_GetByJoinCode_NotFound(t *testing.T) {
	db := testDB(t)
	classrooms := NewClassroomStore(db)

	found, err := classrooms.GetClassroomByJoinCode(context.Background(), "NOTFOUND")
	assert.NoError(t, err)
	assert.Nil(t, found)
}
