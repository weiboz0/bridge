package store

import (
	"context"
	"database/sql"
	"testing"

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
	assert.Equal(t, classID, session.ClassID)

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
