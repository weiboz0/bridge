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
		ClassID: classID, TeacherID: teacherID,
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

func TestSessionStore_CreateAutoEnds(t *testing.T) {
	db := testDB(t)
	sessions := NewSessionStore(db)
	classID, teacherID := setupSessionTest(t, db, t.Name())
	ctx := context.Background()

	s1, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
	require.NoError(t, err)

	s2, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(context.Background(), CreateSessionInput{ClassID: classID, TeacherID: teacherID})

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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})

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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})

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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: class.ID, TeacherID: user.ID})

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

	session, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})

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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})

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

	session, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})

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

	session, err := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})

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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})

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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})
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

	session, _ := sessions.CreateSession(ctx, CreateSessionInput{ClassID: classID, TeacherID: teacherID})

	// Add then remove
	sessions.AddParticipant(ctx, session.ID, student.ID, teacherID)
	sessions.RemoveParticipant(ctx, session.ID, student.ID)

	// Should have no access
	allowed, reason, err := sessions.CanAccessSession(ctx, session.ID, student.ID)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, "no_access", reason)
}
