package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSessionTest creates the full class chain needed for session tests.
func setupSessionTest(t *testing.T, db *sql.DB, suffix string) (classID, teacherID string) {
	t.Helper()
	ctx := context.Background()
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	courses := NewCourseStore(db)
	classes := NewClassStore(db)

	org := createTestOrg(t, db, orgs, suffix)
	teacher := createTestUser(t, db, users, suffix)
	course, err := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: teacher.ID, Title: "Session Course", GradeLevel: "K-5",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })

	class, err := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Session Class", CreatedBy: teacher.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id IN (SELECT id FROM sessions WHERE class_id = $1)", class.ID)
		db.ExecContext(ctx, "DELETE FROM sessions WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	return class.ID, teacher.ID
}

func TestSessionStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())

	session, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		ClassID:   strPtr(classID),
		TeacherID: teacherID,
		Title:     "Class session",
	})
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "live", session.Status)
	require.NotNil(t, session.ClassID)
	assert.Equal(t, classID, *session.ClassID)

	fetched, err := sessions.GetSession(context.Background(), session.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, session.ID, fetched.ID)
}

func TestSessionStore_CreateOrphanSession(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	teacher := createTestUser(t, db, users, t.Name()+"-teacher")

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		TeacherID: teacher.ID,
		Title:     "Office hours",
	})
	require.NoError(t, err)
	require.NotNil(t, session)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", session.ID)
	})

	assert.Nil(t, session.ClassID)
	assert.Equal(t, "Office hours", session.Title)

	fetched, err := sessions.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Nil(t, fetched.ClassID)
	assert.Equal(t, session.ID, fetched.ID)
	assert.Equal(t, "Office hours", fetched.Title)
}

func TestSessionStore_ListSessions_Filters(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name()+"-class")
	otherClassID, otherTeacherID := setupSessionTest(t, db, t.Name()+"-other-class")

	orphanOne, err := sessions.CreateSession(ctx, CreateSessionInput{
		TeacherID: teacherID,
		Title:     "Orphan one",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", orphanOne.ID)
	})

	classSession, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID:   strPtr(classID),
		TeacherID: teacherID,
		Title:     "Class session",
	})
	require.NoError(t, err)

	orphanTwo, err := sessions.CreateSession(ctx, CreateSessionInput{
		TeacherID: teacherID,
		Title:     "Orphan two",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", orphanTwo.ID)
	})

	otherTeacherSession, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID:   strPtr(otherClassID),
		TeacherID: otherTeacherID,
		Title:     "Other teacher",
	})
	require.NoError(t, err)

	_, err = sessions.EndSession(ctx, classSession.ID)
	require.NoError(t, err)

	base := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	_, err = db.ExecContext(ctx, "UPDATE sessions SET started_at = $1 WHERE id = $2", base.Add(1*time.Minute), orphanOne.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "UPDATE sessions SET started_at = $1 WHERE id = $2", base.Add(2*time.Minute), classSession.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "UPDATE sessions SET started_at = $1 WHERE id = $2", base.Add(3*time.Minute), orphanTwo.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "UPDATE sessions SET started_at = $1 WHERE id = $2", base.Add(4*time.Minute), otherTeacherSession.ID)
	require.NoError(t, err)

	teacherSessions, hasMore, err := sessions.ListSessions(ctx, ListSessionsFilter{
		TeacherID: teacherID,
	})
	require.NoError(t, err)
	assert.False(t, hasMore)
	require.Len(t, teacherSessions, 3)
	assert.Equal(t, []string{orphanTwo.ID, classSession.ID, orphanOne.ID}, []string{
		teacherSessions[0].ID,
		teacherSessions[1].ID,
		teacherSessions[2].ID,
	})

	classSessions, hasMore, err := sessions.ListSessions(ctx, ListSessionsFilter{
		ClassID: strPtr(classID),
	})
	require.NoError(t, err)
	assert.False(t, hasMore)
	require.Len(t, classSessions, 1)
	assert.Equal(t, classSession.ID, classSessions[0].ID)

	endedSessions, hasMore, err := sessions.ListSessions(ctx, ListSessionsFilter{
		Status: "ended",
	})
	require.NoError(t, err)
	assert.False(t, hasMore)
	require.Len(t, endedSessions, 1)
	assert.Equal(t, classSession.ID, endedSessions[0].ID)

	firstPage, hasMore, err := sessions.ListSessions(ctx, ListSessionsFilter{
		TeacherID: teacherID,
		Limit:     1,
	})
	require.NoError(t, err)
	require.True(t, hasMore)
	require.Len(t, firstPage, 1)
	assert.Equal(t, orphanTwo.ID, firstPage[0].ID)

	secondPage, hasMore, err := sessions.ListSessions(ctx, ListSessionsFilter{
		TeacherID:       teacherID,
		Limit:           1,
		CursorStartedAt: &firstPage[0].StartedAt,
		CursorID:        &firstPage[0].ID,
	})
	require.NoError(t, err)
	require.True(t, hasMore)
	require.Len(t, secondPage, 1)
	assert.Equal(t, classSession.ID, secondPage[0].ID)

}

func TestSessionStore_ListSessionsByClass_ExcludesOrphans(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())

	classSession, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID:   strPtr(classID),
		TeacherID: teacherID,
		Title:     "Class session",
	})
	require.NoError(t, err)

	orphanSession, err := sessions.CreateSession(ctx, CreateSessionInput{
		TeacherID: teacherID,
		Title:     "Orphan session",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", orphanSession.ID)
	})

	list, err := sessions.ListSessionsByClass(ctx, classID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, classSession.ID, list[0].ID)
}

func TestSessionStore_CreateAutoEnds(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	s1, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	require.NoError(t, err)

	s2, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	require.NoError(t, err)
	assert.NotEqual(t, s1.ID, s2.ID)

	ended, err := sessions.GetSession(ctx, s1.ID)
	require.NoError(t, err)
	assert.Equal(t, "ended", ended.Status)
	assert.NotNil(t, ended.EndedAt)
}

func TestSessionStore_EndSession(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())

	session, _ := sessions.CreateSession(context.Background(), CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})

	ended, err := sessions.EndSession(context.Background(), session.ID)
	require.NoError(t, err)
	assert.Equal(t, "ended", ended.Status)
	assert.NotNil(t, ended.EndedAt)
}

func TestSessionStore_JoinAndLeave(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})

	p, err := sessions.JoinSession(ctx, session.ID, student.ID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "present", p.Status)

	dup, err := sessions.JoinSession(ctx, session.ID, student.ID)
	assert.NoError(t, err)
	assert.Nil(t, dup)

	participants, err := sessions.GetSessionParticipants(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, participants, 1)

	left, err := sessions.LeaveSession(ctx, session.ID, student.ID)
	require.NoError(t, err)
	require.NotNil(t, left)
	assert.NotNil(t, left.LeftAt)
}

func TestSessionStore_UpdateParticipantStatus(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	sessions.JoinSession(ctx, session.ID, student.ID)

	updated, err := sessions.UpdateParticipantStatus(ctx, session.ID, student.ID, "needs_help")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "present", updated.Status)
}

func TestSessionStore_UpdateParticipantStatusRejectsUnknownStatus(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	sessions.JoinSession(ctx, session.ID, student.ID)

	updated, err := sessions.UpdateParticipantStatus(ctx, session.ID, student.ID, "idle")
	require.Error(t, err)
	assert.Nil(t, updated)
	assert.EqualError(t, err, `unsupported participant status "idle"`)
}

func TestSessionStore_GetActiveSession(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	active, err := sessions.GetActiveSession(ctx, classID)
	assert.NoError(t, err)
	assert.Nil(t, active)

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})

	active, err = sessions.GetActiveSession(ctx, classID)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, session.ID, active.ID)
}

func TestSessionStore_SessionTopics(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	courses := NewCourseStore(db)
	topics := NewTopicStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	classes := NewClassStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	user := createTestUser(t, db, users, t.Name())
	course, _ := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID, Title: "Topic Test", GradeLevel: "K-5",
	})
	topic, _ := topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "Variables"})
	class, _ := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Topic Class", CreatedBy: user.ID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_topics WHERE topic_id = $1", topic.ID)
		db.ExecContext(ctx, "DELETE FROM sessions WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID)
		db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
	})

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(class.ID), TeacherID: user.ID, Title: "Session"})

	link, err := sessions.LinkSessionTopic(ctx, session.ID, topic.ID)
	require.NoError(t, err)
	require.NotNil(t, link)

	dup, err := sessions.LinkSessionTopic(ctx, session.ID, topic.ID)
	assert.NoError(t, err)
	assert.Nil(t, dup)

	topicList, err := sessions.GetSessionTopics(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, topicList, 1)
	assert.Equal(t, "Variables", topicList[0].Title)

	err = sessions.UnlinkSessionTopic(ctx, session.ID, topic.ID)
	require.NoError(t, err)

	topicList, err = sessions.GetSessionTopics(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, topicList, 0)
}

func TestSessionStore_GetSessionTopics_Empty(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)

	topics, err := sessions.GetSessionTopics(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.NotNil(t, topics)
	assert.Len(t, topics, 0)
}

// --- Invite Token Tests ---

func TestSessionStore_GenerateAndRotateToken(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	require.NoError(t, err)
	assert.Nil(t, session.InviteToken, "new session should have nil invite token")

	// First rotation
	rotated, err := sessions.RotateInviteToken(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, rotated)
	require.NotNil(t, rotated.InviteToken)
	assert.Len(t, *rotated.InviteToken, 24)
	firstToken := *rotated.InviteToken

	// Second rotation — token changes
	rotated2, err := sessions.RotateInviteToken(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, rotated2.InviteToken)
	assert.Len(t, *rotated2.InviteToken, 24)
	assert.NotEqual(t, firstToken, *rotated2.InviteToken, "rotated token should differ from previous")

	// Old token should no longer resolve
	old, err := sessions.GetSessionByToken(ctx, firstToken)
	assert.NoError(t, err)
	assert.Nil(t, old, "old token should not resolve after rotation")
}

func TestSessionStore_GetSessionByToken(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	rotated, err := sessions.RotateInviteToken(ctx, session.ID)
	require.NoError(t, err)

	// Correct token returns session
	found, err := sessions.GetSessionByToken(ctx, *rotated.InviteToken)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, session.ID, found.ID)

	// Wrong token returns nil
	notFound, err := sessions.GetSessionByToken(ctx, "nonexistent_token_12345")
	assert.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestSessionStore_RevokeToken(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	rotated, _ := sessions.RotateInviteToken(ctx, session.ID)
	token := *rotated.InviteToken

	// Revoke
	revoked, err := sessions.RevokeInviteToken(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, revoked)
	assert.Nil(t, revoked.InviteToken)
	assert.Nil(t, revoked.InviteExpiresAt)

	// Token no longer resolves
	found, err := sessions.GetSessionByToken(ctx, token)
	assert.NoError(t, err)
	assert.Nil(t, found)
}

func TestSessionStore_SetInviteExpiry(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})

	// Set expiry to the future
	future := time.Now().Add(24 * time.Hour)
	updated, err := sessions.SetInviteExpiry(ctx, session.ID, &future)
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.InviteExpiresAt)
	assert.WithinDuration(t, future, *updated.InviteExpiresAt, time.Second)

	// Clear expiry (nil)
	cleared, err := sessions.SetInviteExpiry(ctx, session.ID, nil)
	require.NoError(t, err)
	require.NotNil(t, cleared)
	assert.Nil(t, cleared.InviteExpiresAt)
}

func TestSessionStore_JoinSessionByToken_HappyPath(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	rotated, _ := sessions.RotateInviteToken(ctx, session.ID)

	// Clean up participant on test end
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2", session.ID, student.ID)
	})

	p, err := sessions.JoinSessionByToken(ctx, session.ID, student.ID, *rotated.InviteToken)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, session.ID, p.SessionID)
	assert.Equal(t, student.ID, p.StudentID)
	assert.Equal(t, "present", p.Status)
}

func TestSessionStore_JoinSessionByToken_WrongToken(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	sessions.RotateInviteToken(ctx, session.ID)

	p, err := sessions.JoinSessionByToken(ctx, session.ID, student.ID, "wrong_token_value_12345")
	assert.Nil(t, p)
	assert.ErrorIs(t, err, ErrTokenNotFound)
}

func TestSessionStore_JoinSessionByToken_ExpiredToken(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	rotated, _ := sessions.RotateInviteToken(ctx, session.ID)

	// Set expiry to the past
	past := time.Now().Add(-1 * time.Hour)
	sessions.SetInviteExpiry(ctx, session.ID, &past)

	p, err := sessions.JoinSessionByToken(ctx, session.ID, student.ID, *rotated.InviteToken)
	assert.Nil(t, p)
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestSessionStore_JoinSessionByToken_EndedSession(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	rotated, _ := sessions.RotateInviteToken(ctx, session.ID)
	token := *rotated.InviteToken

	// End the session
	sessions.EndSession(ctx, session.ID)

	p, err := sessions.JoinSessionByToken(ctx, session.ID, student.ID, token)
	assert.Nil(t, p)
	assert.ErrorIs(t, err, ErrSessionEnded)
}

func TestSessionStore_JoinSessionByToken_AlreadyParticipant(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	rotated, _ := sessions.RotateInviteToken(ctx, session.ID)
	token := *rotated.InviteToken

	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2", session.ID, student.ID)
	})

	// First join
	p1, err := sessions.JoinSessionByToken(ctx, session.ID, student.ID, token)
	require.NoError(t, err)
	require.NotNil(t, p1)

	// Second join — idempotent, returns existing
	p2, err := sessions.JoinSessionByToken(ctx, session.ID, student.ID, token)
	require.NoError(t, err)
	require.NotNil(t, p2)
	assert.Equal(t, p1.SessionID, p2.SessionID)
	assert.Equal(t, p1.StudentID, p2.StudentID)
}

func TestSessionStore_CanAccessSession_Teacher(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})

	allowed, reason, err := sessions.CanAccessSession(ctx, session.ID, teacherID)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, "teacher", reason)
}

func TestSessionStore_CanAccessSession_ClassMember(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classes := NewClassStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())

	// Add a student as class member
	student := createTestUser(t, db, users, t.Name()+"-student")
	_, err := classes.AddClassMember(ctx, AddClassMemberInput{
		ClassID: classID, UserID: student.ID, Role: "student",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1 AND user_id = $2", classID, student.ID)
	})

	session, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	require.NoError(t, err)

	allowed, reason, err := sessions.CanAccessSession(ctx, session.ID, student.ID)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, "class_member", reason)
}

func TestSessionStore_CanAccessSession_TokenJoinedParticipant(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	outsider := createTestUser(t, db, users, t.Name()+"-outsider")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	rotated, _ := sessions.RotateInviteToken(ctx, session.ID)

	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2", session.ID, outsider.ID)
	})

	// Join via token first
	_, err := sessions.JoinSessionByToken(ctx, session.ID, outsider.ID, *rotated.InviteToken)
	require.NoError(t, err)

	// Now check access — should be "participant"
	allowed, reason, err := sessions.CanAccessSession(ctx, session.ID, outsider.ID)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, "participant", reason)
}

func TestSessionStore_CanAccessSession_EndedSession(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	sessions.EndSession(ctx, session.ID)

	allowed, reason, err := sessions.CanAccessSession(ctx, session.ID, teacherID)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, "ended", reason)
}

func TestSessionStore_CanAccessSession_NoAccess(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	outsider := createTestUser(t, db, users, t.Name()+"-outsider")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})

	allowed, reason, err := sessions.CanAccessSession(ctx, session.ID, outsider.ID)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, "no_access", reason)

	// Non-existent session
	allowed, reason, err = sessions.CanAccessSession(ctx, "00000000-0000-0000-0000-000000000000", outsider.ID)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, "not_found", reason)
}

// --- Direct-Add Participant Tests ---

func TestSessionStore_AddParticipant_NewUser(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	require.NoError(t, err)

	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2", session.ID, student.ID)
	})

	p, err := sessions.AddParticipant(ctx, session.ID, student.ID, teacherID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, session.ID, p.SessionID)
	assert.Equal(t, student.ID, p.StudentID)
	assert.Equal(t, "invited", p.Status)
	require.NotNil(t, p.InvitedBy)
	assert.Equal(t, teacherID, *p.InvitedBy)
	require.NotNil(t, p.InvitedAt)
	assert.Nil(t, p.JoinedAt, "invited participant should not have joined_at")
}

func TestSessionStore_AddParticipant_AlreadyPresent(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2", session.ID, student.ID)
	})

	// Join as present first
	joined, err := sessions.JoinSession(ctx, session.ID, student.ID)
	require.NoError(t, err)
	require.NotNil(t, joined)
	assert.Equal(t, "present", joined.Status)

	// AddParticipant should be a no-op, returning existing row
	p, err := sessions.AddParticipant(ctx, session.ID, student.ID, teacherID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "present", p.Status, "status should remain present")
	assert.Nil(t, p.InvitedBy, "invited_by should remain nil for self-joined participant")
}

func TestSessionStore_AddParticipant_ReInviteAfterLeft(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2", session.ID, student.ID)
	})

	// Join and leave
	sessions.JoinSession(ctx, session.ID, student.ID)
	sessions.LeaveSession(ctx, session.ID, student.ID)

	// Re-invite after leaving
	p, err := sessions.AddParticipant(ctx, session.ID, student.ID, teacherID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "invited", p.Status, "left participant should be re-invited")
	require.NotNil(t, p.InvitedBy)
	assert.Equal(t, teacherID, *p.InvitedBy)
	assert.Nil(t, p.LeftAt, "left_at should be cleared on re-invite")
}

func TestSessionStore_AddParticipantByEmail_HappyPath(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2", session.ID, student.ID)
	})

	email := t.Name() + "-student@example.com"
	p, err := sessions.AddParticipantByEmail(ctx, session.ID, email, teacherID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, student.ID, p.StudentID)
	assert.Equal(t, "invited", p.Status)
}

func TestSessionStore_AddParticipantByEmail_NotFound(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})

	p, err := sessions.AddParticipantByEmail(ctx, session.ID, "nonexistent@example.com", teacherID)
	assert.Nil(t, p)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUserNotFound))
}

func TestSessionStore_RemoveParticipant(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})

	// Add participant
	sessions.AddParticipant(ctx, session.ID, student.ID, teacherID)

	// Remove
	removed, err := sessions.RemoveParticipant(ctx, session.ID, student.ID)
	require.NoError(t, err)
	assert.True(t, removed, "should delete existing participant row")

	// Remove again — no-op
	removed, err = sessions.RemoveParticipant(ctx, session.ID, student.ID)
	require.NoError(t, err)
	assert.False(t, removed, "should return false when no row to delete")

	// Verify access is revoked
	allowed, reason, err := sessions.CanAccessSession(ctx, session.ID, student.ID)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, "no_access", reason)
}

func TestSessionStore_PromoteToPresent(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2", session.ID, student.ID)
	})

	// Add as invited
	sessions.AddParticipant(ctx, session.ID, student.ID, teacherID)

	// Promote
	p, err := sessions.PromoteToPresent(ctx, session.ID, student.ID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "present", p.Status)
	require.NotNil(t, p.JoinedAt, "joined_at should be set after promotion")
}

func TestSessionStore_PromoteToPresent_AlreadyPresent(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2", session.ID, student.ID)
	})

	// Join directly as present
	sessions.JoinSession(ctx, session.ID, student.ID)

	// Promote — should be no-op, returning existing row
	p, err := sessions.PromoteToPresent(ctx, session.ID, student.ID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "present", p.Status)
}

func TestSessionStore_CanAccessSession_InvitedParticipant(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1 AND user_id = $2", session.ID, student.ID)
	})

	// Add as invited (not yet present)
	sessions.AddParticipant(ctx, session.ID, student.ID, teacherID)

	// Should still have access
	allowed, reason, err := sessions.CanAccessSession(ctx, session.ID, student.ID)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, "participant", reason)
}

func TestSessionStore_CanAccessSession_RevokedParticipant(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	student := createTestUser(t, db, users, t.Name()+"-student")
	ctx := context.Background()

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session"})

	// Add then remove
	sessions.AddParticipant(ctx, session.ID, student.ID, teacherID)
	sessions.RemoveParticipant(ctx, session.ID, student.ID)

	// Should have no access
	allowed, reason, err := sessions.CanAccessSession(ctx, session.ID, student.ID)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, "no_access", reason)
}

// --- ListSessionsWithCounts Tests ---

func TestSessionStore_ListSessionsWithCounts_EmptyClass(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, _ := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	list, err := sessions.ListSessionsWithCounts(ctx, classID)
	require.NoError(t, err)
	require.NotNil(t, list)
	assert.Len(t, list, 0)
}

func TestSessionStore_ListSessionsWithCounts_ZeroParticipants(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	_, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID: strPtr(classID), TeacherID: teacherID, Title: "No participants",
	})
	require.NoError(t, err)

	list, err := sessions.ListSessionsWithCounts(ctx, classID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 0, list[0].ParticipantCount)
	assert.Equal(t, "No participants", list[0].Title)
}

func TestSessionStore_ListSessionsWithCounts_MultipleParticipants(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID: strPtr(classID), TeacherID: teacherID, Title: "With participants",
	})
	require.NoError(t, err)

	// Add 3 participants: student1 joins, student2 joins, student3 joins then leaves
	student1 := createTestUser(t, db, users, t.Name()+"-s1")
	student2 := createTestUser(t, db, users, t.Name()+"-s2")
	student3 := createTestUser(t, db, users, t.Name()+"-s3")

	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id = $1", session.ID)
	})

	_, err = sessions.JoinSession(ctx, session.ID, student1.ID)
	require.NoError(t, err)
	_, err = sessions.JoinSession(ctx, session.ID, student2.ID)
	require.NoError(t, err)
	_, err = sessions.JoinSession(ctx, session.ID, student3.ID)
	require.NoError(t, err)

	// Student3 leaves — but COUNT(*) still includes them
	_, err = sessions.LeaveSession(ctx, session.ID, student3.ID)
	require.NoError(t, err)

	list, err := sessions.ListSessionsWithCounts(ctx, classID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 3, list[0].ParticipantCount, "count should include all participant rows, even left ones")
}

func TestSessionStore_ListSessionsWithCounts_MultipleSessions_SortedDesc(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	users := NewUserStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	// Create first session (will be auto-ended by the second)
	s1, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID: strPtr(classID), TeacherID: teacherID, Title: "First",
	})
	require.NoError(t, err)

	// Force older started_at
	base := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	_, err = db.ExecContext(ctx, "UPDATE sessions SET started_at = $1 WHERE id = $2", base, s1.ID)
	require.NoError(t, err)

	// Create second session (auto-ends the first)
	s2, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID: strPtr(classID), TeacherID: teacherID, Title: "Second",
	})
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "UPDATE sessions SET started_at = $1 WHERE id = $2", base.Add(1*time.Hour), s2.ID)
	require.NoError(t, err)

	// Add a participant to s1 only
	student := createTestUser(t, db, users, t.Name()+"-s")
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM session_participants WHERE session_id IN ($1, $2)", s1.ID, s2.ID)
	})
	_, err = sessions.JoinSession(ctx, s1.ID, student.ID)
	require.NoError(t, err)

	list, err := sessions.ListSessionsWithCounts(ctx, classID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	// DESC order: s2 first, s1 second
	assert.Equal(t, s2.ID, list[0].ID)
	assert.Equal(t, s1.ID, list[1].ID)
	assert.Equal(t, 0, list[0].ParticipantCount)
	assert.Equal(t, 1, list[1].ParticipantCount)
}

// --- UpdateSession Tests ---

func TestSessionStore_UpdateSession_TitleOnly(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID: strPtr(classID), TeacherID: teacherID, Title: "Original", Settings: `{"foo":"bar"}`,
	})
	require.NoError(t, err)

	newTitle := "Updated Title"
	updated, err := sessions.UpdateSession(ctx, session.ID, UpdateSessionInput{
		Title: &newTitle,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Updated Title", updated.Title)
	assert.Contains(t, updated.Settings, "foo", "settings must remain unchanged")
}

func TestSessionStore_UpdateSession_SettingsOnly(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID: strPtr(classID), TeacherID: teacherID, Title: "Keep Title",
	})
	require.NoError(t, err)

	newSettings := `{"mode":"guided"}`
	updated, err := sessions.UpdateSession(ctx, session.ID, UpdateSessionInput{
		Settings: &newSettings,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Keep Title", updated.Title, "title must remain unchanged")
	assert.Contains(t, updated.Settings, "guided", "settings must contain the new value")
}

func TestSessionStore_UpdateSession_InviteExpiresAt_SetAndClear(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID: strPtr(classID), TeacherID: teacherID, Title: "Expiry test",
	})
	require.NoError(t, err)

	// Set invite_expires_at
	future := time.Now().Add(24 * time.Hour)
	updated, err := sessions.UpdateSession(ctx, session.ID, UpdateSessionInput{
		InviteExpiresAt: &future,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.InviteExpiresAt)
	assert.WithinDuration(t, future, *updated.InviteExpiresAt, time.Second)

	// Clear invite_expires_at using ClearInviteExpiry
	cleared, err := sessions.UpdateSession(ctx, session.ID, UpdateSessionInput{
		ClearInviteExpiry: true,
	})
	require.NoError(t, err)
	require.NotNil(t, cleared)
	assert.Nil(t, cleared.InviteExpiresAt, "invite_expires_at should be cleared to NULL")
}

func TestSessionStore_UpdateSession_NonExistent(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	ctx := context.Background()

	newTitle := "Ghost"
	result, err := sessions.UpdateSession(ctx, "00000000-0000-0000-0000-000000000000", UpdateSessionInput{
		Title: &newTitle,
	})
	assert.NoError(t, err)
	assert.Nil(t, result, "updating non-existent session should return nil")
}

func TestSessionStore_UpdateSession_NoFields(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID: strPtr(classID), TeacherID: teacherID, Title: "Untouched",
	})
	require.NoError(t, err)

	// No fields set at all — COALESCE($1, title) with nil $1 preserves title
	result, err := sessions.UpdateSession(ctx, session.ID, UpdateSessionInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Untouched", result.Title, "title must not change when no fields provided")
}

// --- ListSessionsByClass empty class test ---

func TestSessionStore_ListSessionsByClass_EmptyClass(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, _ := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	list, err := sessions.ListSessionsByClass(ctx, classID)
	require.NoError(t, err)
	require.NotNil(t, list)
	assert.Len(t, list, 0)
}

// --- RemoveParticipant non-existent test ---

func TestSessionStore_RemoveParticipant_NonExistent(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID: strPtr(classID), TeacherID: teacherID, Title: "Session",
	})
	require.NoError(t, err)

	// Remove a participant that was never added
	removed, err := sessions.RemoveParticipant(ctx, session.ID, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.False(t, removed, "should return false when no row to delete")
}

// --- RotateInviteToken non-existent test ---

func TestSessionStore_RotateInviteToken_NonExistent(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	ctx := context.Background()

	result, err := sessions.RotateInviteToken(ctx, "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, result, "rotating token on non-existent session should return nil")
}

// Plan 048 phase 1: when CreateSession is called with TopicIDs that
// don't exist in `topics`, the bulk insert into session_topics hits
// the FK constraint and the WHOLE transaction rolls back — no session
// row, no session_topics row. This test guards the atomicity contract
// at the store boundary (a handler-level failure-injection test would
// be unreliable since pre-inserts fire before CreateSession even
// runs).
func TestSessionStore_CreateSession_AtomicTopicSnapshot(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	const bogusTopicID = "00000000-0000-0000-0000-000000000000"
	const sentinelTitle = "Atomic Rollback Sentinel Title"

	session, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID:   strPtr(classID),
		TeacherID: teacherID,
		Title:     sentinelTitle,
		TopicIDs:  []string{bogusTopicID},
	})
	assert.Error(t, err, "bogus topic_id must violate the session_topics FK and fail CreateSession")
	assert.Nil(t, session)

	// Rollback signature 1: no session row with the sentinel title.
	var sessionCount int
	err = db.QueryRow(
		"SELECT count(*) FROM sessions WHERE title = $1", sentinelTitle,
	).Scan(&sessionCount)
	require.NoError(t, err)
	assert.Equal(t, 0, sessionCount, "session row must NOT exist after FK rollback")

	// Rollback signature 2: no session_topics row referencing the bogus topic.
	var stCount int
	err = db.QueryRow(
		"SELECT count(*) FROM session_topics WHERE topic_id = $1", bogusTopicID,
	).Scan(&stCount)
	require.NoError(t, err)
	assert.Equal(t, 0, stCount, "session_topics row must NOT exist after FK rollback")
}
