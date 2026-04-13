package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestClassroom(t *testing.T, classroomStore *ClassroomStore, userID string) *Classroom {
	t.Helper()
	classroom, err := classroomStore.CreateClassroom(context.Background(), CreateClassroomInput{
		TeacherID: userID, Name: "Session Test Classroom",
		GradeLevel: "K-5", EditorMode: "python",
	})
	require.NoError(t, err)
	return classroom
}

func TestSessionStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classrooms := NewClassroomStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name())
	classroom := createTestClassroom(t, classrooms, user.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: user.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "active", session.Status)
	assert.Equal(t, classroom.ID, session.ClassroomID)

	fetched, err := sessions.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, session.ID, fetched.ID)
}

func TestSessionStore_CreateAutoEnds(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classrooms := NewClassroomStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name())
	classroom := createTestClassroom(t, classrooms, user.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	s1, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: user.ID,
	})
	require.NoError(t, err)

	// Creating second session should end the first
	s2, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: user.ID,
	})
	require.NoError(t, err)
	assert.NotEqual(t, s1.ID, s2.ID)

	// First should now be ended
	ended, err := sessions.GetSession(ctx, s1.ID)
	require.NoError(t, err)
	assert.Equal(t, "ended", ended.Status)
	assert.NotNil(t, ended.EndedAt)
}

func TestSessionStore_EndSession(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classrooms := NewClassroomStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name())
	classroom := createTestClassroom(t, classrooms, user.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: user.ID,
	})
	require.NoError(t, err)

	ended, err := sessions.EndSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "ended", ended.Status)
	assert.NotNil(t, ended.EndedAt)
}

func TestSessionStore_JoinAndLeave(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classrooms := NewClassroomStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")
	classroom := createTestClassroom(t, classrooms, teacher.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id IN (SELECT id FROM live_sessions WHERE classroom_id = $1)", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: teacher.ID,
	})
	require.NoError(t, err)

	// Join
	p, err := sessions.JoinSession(ctx, session.ID, student.ID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "active", p.Status)

	// Duplicate join returns nil
	dup, err := sessions.JoinSession(ctx, session.ID, student.ID)
	assert.NoError(t, err)
	assert.Nil(t, dup)

	// List participants
	participants, err := sessions.GetSessionParticipants(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, participants, 1)
	assert.Equal(t, student.ID, participants[0].StudentID)

	// Leave
	left, err := sessions.LeaveSession(ctx, session.ID, student.ID)
	require.NoError(t, err)
	require.NotNil(t, left)
	assert.NotNil(t, left.LeftAt)
}

func TestSessionStore_UpdateParticipantStatus(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classrooms := NewClassroomStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")
	classroom := createTestClassroom(t, classrooms, teacher.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id IN (SELECT id FROM live_sessions WHERE classroom_id = $1)", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: teacher.ID,
	})
	require.NoError(t, err)

	_, err = sessions.JoinSession(ctx, session.ID, student.ID)
	require.NoError(t, err)

	updated, err := sessions.UpdateParticipantStatus(ctx, session.ID, student.ID, "needs_help")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "needs_help", updated.Status)
}

func TestSessionStore_GetActiveSession(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classrooms := NewClassroomStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name())
	classroom := createTestClassroom(t, classrooms, user.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	// No active session initially
	active, err := sessions.GetActiveSession(ctx, classroom.ID)
	assert.NoError(t, err)
	assert.Nil(t, active)

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: user.ID,
	})
	require.NoError(t, err)

	active, err = sessions.GetActiveSession(ctx, classroom.ID)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, session.ID, active.ID)
}
