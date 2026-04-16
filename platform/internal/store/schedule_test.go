package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduleStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())

	title := "Intro to Loops"
	start := time.Now().Add(24 * time.Hour)
	end := start.Add(time.Hour)

	sched, err := schedules.CreateSchedule(ctx, CreateScheduleInput{
		ClassID:        classID,
		TeacherID:      teacherID,
		Title:          &title,
		ScheduledStart: start,
		ScheduledEnd:   end,
	})
	require.NoError(t, err)
	require.NotNil(t, sched)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE id = $1", sched.ID) })

	assert.Equal(t, "planned", sched.Status)
	assert.Equal(t, "Intro to Loops", *sched.Title)
	assert.Nil(t, sched.LiveSessionID)

	fetched, err := schedules.GetSchedule(ctx, sched.ID)
	require.NoError(t, err)
	assert.Equal(t, sched.ID, fetched.ID)
}

func TestScheduleStore_ListByClass(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())

	for i := 0; i < 3; i++ {
		start := time.Now().Add(time.Duration(i+1) * 24 * time.Hour)
		schedules.CreateSchedule(ctx, CreateScheduleInput{
			ClassID: classID, TeacherID: teacherID,
			ScheduledStart: start, ScheduledEnd: start.Add(time.Hour),
		})
	}
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE class_id = $1", classID) })

	list, err := schedules.ListByClass(ctx, classID)
	require.NoError(t, err)
	assert.Len(t, list, 3)
	// Should be ordered by scheduled_start ASC
	assert.True(t, list[0].ScheduledStart.Before(list[1].ScheduledStart))
}

func TestScheduleStore_ListUpcoming(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())

	// Create one past (should not appear) and one future
	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)
	schedules.CreateSchedule(ctx, CreateScheduleInput{
		ClassID: classID, TeacherID: teacherID,
		ScheduledStart: past, ScheduledEnd: past.Add(time.Hour),
	})
	schedules.CreateSchedule(ctx, CreateScheduleInput{
		ClassID: classID, TeacherID: teacherID,
		ScheduledStart: future, ScheduledEnd: future.Add(time.Hour),
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE class_id = $1", classID) })

	upcoming, err := schedules.ListUpcoming(ctx, classID, 10)
	require.NoError(t, err)
	assert.Len(t, upcoming, 1)
}

func TestScheduleStore_UpdateSchedule(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())
	start := time.Now().Add(24 * time.Hour)

	sched, _ := schedules.CreateSchedule(ctx, CreateScheduleInput{
		ClassID: classID, TeacherID: teacherID,
		ScheduledStart: start, ScheduledEnd: start.Add(time.Hour),
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE id = $1", sched.ID) })

	newTitle := "Updated Title"
	updated, err := schedules.UpdateSchedule(ctx, sched.ID, UpdateScheduleInput{
		Title: &newTitle,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Updated Title", *updated.Title)
}

func TestScheduleStore_CancelSchedule(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())
	start := time.Now().Add(24 * time.Hour)

	sched, _ := schedules.CreateSchedule(ctx, CreateScheduleInput{
		ClassID: classID, TeacherID: teacherID,
		ScheduledStart: start, ScheduledEnd: start.Add(time.Hour),
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE id = $1", sched.ID) })

	cancelled, err := schedules.CancelSchedule(ctx, sched.ID)
	require.NoError(t, err)
	require.NotNil(t, cancelled)
	assert.Equal(t, "cancelled", cancelled.Status)

	// Cancel again should return nil (already cancelled)
	again, err := schedules.CancelSchedule(ctx, sched.ID)
	assert.NoError(t, err)
	assert.Nil(t, again)
}

func TestScheduleStore_StartScheduledSession(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)
	topics := NewTopicStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	classes := NewClassStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	teacher := createTestUser(t, db, users, t.Name())
	course, _ := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: teacher.ID, Title: "Sched Course", GradeLevel: "K-5",
	})
	topic, _ := topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "Loops"})
	class, _ := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Sched Class", CreatedBy: teacher.ID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM session_topics WHERE topic_id = $1", topic.ID)
		db.ExecContext(ctx, "DELETE FROM live_sessions WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID)
		db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
	})

	start := time.Now().Add(time.Hour)
	sched, _ := schedules.CreateSchedule(ctx, CreateScheduleInput{
		ClassID: class.ID, TeacherID: teacher.ID,
		ScheduledStart: start, ScheduledEnd: start.Add(time.Hour),
		TopicIDs: []string{topic.ID},
	})

	// Start the scheduled session
	session, err := schedules.StartScheduledSession(ctx, sched.ID, teacher.ID)
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "active", session.Status)
	assert.Equal(t, class.ID, session.ClassID)

	// Schedule should now be in_progress
	updated, _ := schedules.GetSchedule(ctx, sched.ID)
	assert.Equal(t, "in_progress", updated.Status)
	assert.Equal(t, session.ID, *updated.LiveSessionID)

	// Topics should be linked
	linkedTopics, _ := NewSessionStore(db).GetSessionTopics(ctx, session.ID)
	assert.Len(t, linkedTopics, 1)
	assert.Equal(t, topic.ID, linkedTopics[0].TopicID)
}

func TestScheduleStore_GetSchedule_NotFound(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)

	s, err := schedules.GetSchedule(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, s)
}

func TestScheduleStore_CompleteScheduledSession(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())

	start := time.Now().Add(time.Hour)
	sched, err := schedules.CreateSchedule(ctx, CreateScheduleInput{
		ClassID: classID, TeacherID: teacherID,
		ScheduledStart: start, ScheduledEnd: start.Add(time.Hour),
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE id = $1", sched.ID) })

	// Start the scheduled session
	session, err := schedules.StartScheduledSession(ctx, sched.ID, teacherID)
	require.NoError(t, err)

	// Complete it
	err = schedules.CompleteScheduledSession(ctx, session.ID)
	require.NoError(t, err)

	// Verify status
	completed, err := schedules.GetSchedule(ctx, sched.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", completed.Status)
}

func TestScheduleStore_UpdateSchedule_NoOp(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())
	start := time.Now().Add(24 * time.Hour)

	sched, _ := schedules.CreateSchedule(ctx, CreateScheduleInput{
		ClassID: classID, TeacherID: teacherID,
		ScheduledStart: start, ScheduledEnd: start.Add(time.Hour),
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE id = $1", sched.ID) })

	// Update with empty input should return existing record
	result, err := schedules.UpdateSchedule(ctx, sched.ID, UpdateScheduleInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, sched.ID, result.ID)
}

func TestScheduleStore_CreateWithTopicIDs(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)
	topics := NewTopicStore(db)
	courses := NewCourseStore(db)
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	classes := NewClassStore(db)
	ctx := context.Background()

	org := createTestOrg(t, db, orgs, t.Name())
	teacher := createTestUser(t, db, users, t.Name())
	course, _ := courses.CreateCourse(ctx, CreateCourseInput{
		OrgID: org.ID, CreatedBy: teacher.ID, Title: "Topic Course", GradeLevel: "K-5",
	})
	topic1, _ := topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "Loops"})
	topic2, _ := topics.CreateTopic(ctx, CreateTopicInput{CourseID: course.ID, Title: "Variables"})
	class, _ := classes.CreateClass(ctx, CreateClassInput{
		CourseID: course.ID, OrgID: org.ID, Title: "Topic Class", CreatedBy: teacher.ID,
	})
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM new_classrooms WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", course.ID)
		db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
	})

	title := "Session with Topics"
	start := time.Now().Add(24 * time.Hour)
	sched, err := schedules.CreateSchedule(ctx, CreateScheduleInput{
		ClassID:        class.ID,
		TeacherID:      teacher.ID,
		Title:          &title,
		ScheduledStart: start,
		ScheduledEnd:   start.Add(time.Hour),
		TopicIDs:       []string{topic1.ID, topic2.ID},
	})
	require.NoError(t, err)
	require.NotNil(t, sched)
	assert.Len(t, sched.TopicIDs, 2)
	assert.Contains(t, sched.TopicIDs, topic1.ID)
	assert.Contains(t, sched.TopicIDs, topic2.ID)

	// Verify round-trip via Get
	fetched, err := schedules.GetSchedule(ctx, sched.ID)
	require.NoError(t, err)
	assert.Len(t, fetched.TopicIDs, 2)
}

func TestScheduleStore_UpdateSchedule_NonPlanned(t *testing.T) {
	db := testDB(t)
	schedules := NewScheduleStore(db)
	ctx := context.Background()

	classID, teacherID := setupSessionTest(t, db, t.Name())
	start := time.Now().Add(24 * time.Hour)

	sched, _ := schedules.CreateSchedule(ctx, CreateScheduleInput{
		ClassID: classID, TeacherID: teacherID,
		ScheduledStart: start, ScheduledEnd: start.Add(time.Hour),
	})
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE id = $1", sched.ID) })

	// Start the session (moves to in_progress)
	schedules.StartScheduledSession(ctx, sched.ID, teacherID)

	// Try updating — should return nil since it's no longer planned
	newTitle := "Should Not Work"
	updated, err := schedules.UpdateSchedule(ctx, sched.ID, UpdateScheduleInput{
		Title: &newTitle,
	})
	assert.NoError(t, err)
	assert.Nil(t, updated)
}
