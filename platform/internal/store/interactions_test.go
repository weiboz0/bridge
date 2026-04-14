package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInteractionStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	interactions := NewInteractionStore(db)
	users := NewUserStore(db)
	classrooms := NewClassroomStore(db)
	sessions := NewSessionStore(db)
	ctx := context.Background()

	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")
	classroom := createTestClassroom(t, classrooms, teacher.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM ai_interactions WHERE session_id IN (SELECT id FROM live_sessions WHERE classroom_id = $1)", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: teacher.ID,
	})
	require.NoError(t, err)

	interaction, err := interactions.CreateInteraction(ctx, CreateInteractionInput{
		StudentID: student.ID, SessionID: session.ID, EnabledByTeacherID: teacher.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, interaction)
	assert.Equal(t, "[]", interaction.Messages)

	fetched, err := interactions.GetInteraction(ctx, interaction.ID)
	require.NoError(t, err)
	assert.Equal(t, interaction.ID, fetched.ID)
}

func TestInteractionStore_GetActiveInteraction(t *testing.T) {
	db := testDB(t)
	interactions := NewInteractionStore(db)
	users := NewUserStore(db)
	classrooms := NewClassroomStore(db)
	sessions := NewSessionStore(db)
	ctx := context.Background()

	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")
	classroom := createTestClassroom(t, classrooms, teacher.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM ai_interactions WHERE session_id IN (SELECT id FROM live_sessions WHERE classroom_id = $1)", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: teacher.ID,
	})

	// No interaction yet
	active, err := interactions.GetActiveInteraction(ctx, student.ID, session.ID)
	assert.NoError(t, err)
	assert.Nil(t, active)

	// Create one
	interactions.CreateInteraction(ctx, CreateInteractionInput{
		StudentID: student.ID, SessionID: session.ID, EnabledByTeacherID: teacher.ID,
	})

	active, err = interactions.GetActiveInteraction(ctx, student.ID, session.ID)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, student.ID, active.StudentID)
}

func TestInteractionStore_AppendMessage(t *testing.T) {
	db := testDB(t)
	interactions := NewInteractionStore(db)
	users := NewUserStore(db)
	classrooms := NewClassroomStore(db)
	sessions := NewSessionStore(db)
	ctx := context.Background()

	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")
	classroom := createTestClassroom(t, classrooms, teacher.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM ai_interactions WHERE session_id IN (SELECT id FROM live_sessions WHERE classroom_id = $1)", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: teacher.ID,
	})
	interaction, _ := interactions.CreateInteraction(ctx, CreateInteractionInput{
		StudentID: student.ID, SessionID: session.ID, EnabledByTeacherID: teacher.ID,
	})

	updated, err := interactions.AppendMessage(ctx, interaction.ID, ChatMessage{
		Role: "user", Content: "Hello!", Timestamp: "2026-04-13T00:00:00Z",
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Contains(t, updated.Messages, "Hello!")

	updated, err = interactions.AppendMessage(ctx, interaction.ID, ChatMessage{
		Role: "assistant", Content: "Hi there!", Timestamp: "2026-04-13T00:00:01Z",
	})
	require.NoError(t, err)
	assert.Contains(t, updated.Messages, "Hi there!")
}

func TestInteractionStore_ListBySession(t *testing.T) {
	db := testDB(t)
	interactions := NewInteractionStore(db)
	users := NewUserStore(db)
	classrooms := NewClassroomStore(db)
	sessions := NewSessionStore(db)
	ctx := context.Background()

	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")
	classroom := createTestClassroom(t, classrooms, teacher.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM ai_interactions WHERE session_id IN (SELECT id FROM live_sessions WHERE classroom_id = $1)", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: teacher.ID,
	})
	interactions.CreateInteraction(ctx, CreateInteractionInput{
		StudentID: student.ID, SessionID: session.ID, EnabledByTeacherID: teacher.ID,
	})

	list, err := interactions.ListInteractionsBySession(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestInteractionStore_DeleteInteraction(t *testing.T) {
	db := testDB(t)
	interactions := NewInteractionStore(db)
	users := NewUserStore(db)
	classrooms := NewClassroomStore(db)
	sessions := NewSessionStore(db)
	ctx := context.Background()

	teacher := createTestUser(t, db, users, t.Name()+"-teacher")
	student := createTestUser(t, db, users, t.Name()+"-student")
	classroom := createTestClassroom(t, classrooms, teacher.ID)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM ai_interactions WHERE session_id IN (SELECT id FROM live_sessions WHERE classroom_id = $1)", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE classroom_id = $1", classroom.ID)
		db.ExecContext(ctx, "DELETE FROM classrooms WHERE id = $1", classroom.ID)
	})

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{
		ClassroomID: classroom.ID, TeacherID: teacher.ID,
	})
	interactions.CreateInteraction(ctx, CreateInteractionInput{
		StudentID: student.ID, SessionID: session.ID, EnabledByTeacherID: teacher.ID,
	})

	// Verify it exists
	active, err := interactions.GetActiveInteraction(ctx, student.ID, session.ID)
	require.NoError(t, err)
	require.NotNil(t, active)

	// Delete
	err = interactions.DeleteInteraction(ctx, student.ID, session.ID)
	require.NoError(t, err)

	// Verify gone
	active, err = interactions.GetActiveInteraction(ctx, student.ID, session.ID)
	require.NoError(t, err)
	assert.Nil(t, active)
}
