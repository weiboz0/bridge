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
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM ai_interactions WHERE session_id = $1", session.ID)
	})

	interaction, err := interactions.CreateInteraction(ctx, CreateInteractionInput{
		StudentID: student.ID, SessionID: session.ID, EnabledByTeacherID: teacherID,
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
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM ai_interactions WHERE session_id = $1", session.ID)
	})

	active, err := interactions.GetActiveInteraction(ctx, student.ID, session.ID)
	assert.NoError(t, err)
	assert.Nil(t, active)

	interactions.CreateInteraction(ctx, CreateInteractionInput{
		StudentID: student.ID, SessionID: session.ID, EnabledByTeacherID: teacherID,
	})

	active, err = interactions.GetActiveInteraction(ctx, student.ID, session.ID)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, student.ID, active.StudentID)
}

func TestInteractionStore_AppendMessage(t *testing.T) {
	db := testDB(t)
	interactions := NewInteractionStore(db)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
	interaction, _ := interactions.CreateInteraction(ctx, CreateInteractionInput{
		StudentID: student.ID, SessionID: session.ID, EnabledByTeacherID: teacherID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM ai_interactions WHERE session_id = $1", session.ID)
	})

	updated, err := interactions.AppendMessage(ctx, interaction.ID, ChatMessage{
		Role: "user", Content: "Hello!", Timestamp: "2026-04-13T00:00:00Z",
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Contains(t, updated.Messages, "Hello!")
}

func TestInteractionStore_ListBySession(t *testing.T) {
	db := testDB(t)
	interactions := NewInteractionStore(db)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
	interactions.CreateInteraction(ctx, CreateInteractionInput{
		StudentID: student.ID, SessionID: session.ID, EnabledByTeacherID: teacherID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM ai_interactions WHERE session_id = $1", session.ID)
	})

	list, err := interactions.ListInteractionsBySession(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestInteractionStore_DeleteInteraction(t *testing.T) {
	db := testDB(t)
	interactions := NewInteractionStore(db)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
	interactions.CreateInteraction(ctx, CreateInteractionInput{
		StudentID: student.ID, SessionID: session.ID, EnabledByTeacherID: teacherID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM ai_interactions WHERE session_id = $1", session.ID)
	})

	err := interactions.DeleteInteraction(ctx, student.ID, session.ID)
	require.NoError(t, err)

	active, _ := interactions.GetActiveInteraction(ctx, student.ID, session.ID)
	assert.Nil(t, active)
}
