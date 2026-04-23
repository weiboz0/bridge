package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

func TestCreateSchedule_NoClaims(t *testing.T) {
	h := &ScheduleHandler{}
	body, _ := json.Marshal(map[string]string{
		"scheduledStart": "2026-05-01T10:00:00Z",
		"scheduledEnd":   "2026-05-01T11:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/classes/c1/schedule", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"classId": "c1"})
	w := httptest.NewRecorder()
	h.Create(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateSchedule_InvalidDates(t *testing.T) {
	h := &ScheduleHandler{}
	body, _ := json.Marshal(map[string]string{
		"scheduledStart": "not-a-date",
		"scheduledEnd":   "2026-05-01T11:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/classes/c1/schedule", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"classId": "c1"})
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.Create(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateSchedule_EndBeforeStart(t *testing.T) {
	h := &ScheduleHandler{}
	body, _ := json.Marshal(map[string]string{
		"scheduledStart": "2026-05-01T11:00:00Z",
		"scheduledEnd":   "2026-05-01T10:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/classes/c1/schedule", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"classId": "c1"})
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.Create(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateSchedule_InvalidEndDate(t *testing.T) {
	h := &ScheduleHandler{}
	body, _ := json.Marshal(map[string]string{
		"scheduledStart": "2026-05-01T10:00:00Z",
		"scheduledEnd":   "not-a-date",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/classes/c1/schedule", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"classId": "c1"})
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.Create(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateSchedule_EqualStartEnd(t *testing.T) {
	h := &ScheduleHandler{}
	body, _ := json.Marshal(map[string]string{
		"scheduledStart": "2026-05-01T10:00:00Z",
		"scheduledEnd":   "2026-05-01T10:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/classes/c1/schedule", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"classId": "c1"})
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.Create(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListSchedule_NoClaims(t *testing.T) {
	h := &ScheduleHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/classes/c1/schedule", nil)
	req = withChiParams(req, map[string]string{"classId": "c1"})
	w := httptest.NewRecorder()
	h.List(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListUpcomingSchedule_NoClaims(t *testing.T) {
	h := &ScheduleHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/classes/c1/schedule/upcoming", nil)
	req = withChiParams(req, map[string]string{"classId": "c1"})
	w := httptest.NewRecorder()
	h.ListUpcoming(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUpdateSchedule_NoClaims(t *testing.T) {
	h := &ScheduleHandler{}
	body, _ := json.Marshal(map[string]string{"title": "new"})
	req := httptest.NewRequest(http.MethodPatch, "/api/schedule/s1", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "s1"})
	w := httptest.NewRecorder()
	h.Update(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCancelSchedule_NoClaims(t *testing.T) {
	h := &ScheduleHandler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/schedule/s1", nil)
	req = withChiParams(req, map[string]string{"id": "s1"})
	w := httptest.NewRecorder()
	h.Cancel(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestStartSchedule_NoClaims(t *testing.T) {
	h := &ScheduleHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/schedule/s1/start", nil)
	req = withChiParams(req, map[string]string{"id": "s1"})
	w := httptest.NewRecorder()
	h.Start(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestScheduleHandler_List_UsesSessionScheduledBackref(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	suffix := strings.ToLower(strings.ReplaceAll(t.Name(), "_", "-"))

	orgs := store.NewOrgStore(db)
	users := store.NewUserStore(db)
	courses := store.NewCourseStore(db)
	classes := store.NewClassStore(db)
	schedules := store.NewScheduleStore(db)

	org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name:         "Org " + suffix,
		Slug:         "org-" + suffix,
		Type:         "school",
		ContactEmail: suffix + "@example.com",
		ContactName:  "Admin",
	})
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "UPDATE organizations SET status = 'active' WHERE id = $1", org.ID)
	require.NoError(t, err)

	teacher, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "Teacher " + suffix,
		Email:    suffix + "-teacher@example.com",
		Password: "testpassword123",
	})
	require.NoError(t, err)

	course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID:      org.ID,
		CreatedBy:  teacher.ID,
		Title:      "Course " + suffix,
		GradeLevel: "K-5",
	})
	require.NoError(t, err)

	class, err := classes.CreateClass(ctx, store.CreateClassInput{
		CourseID:  course.ID,
		OrgID:     org.ID,
		Title:     "Class " + suffix,
		CreatedBy: teacher.ID,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM scheduled_sessions WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM sessions WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
		db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", teacher.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", teacher.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	sched, err := schedules.CreateSchedule(ctx, store.CreateScheduleInput{
		ClassID:        class.ID,
		TeacherID:      teacher.ID,
		ScheduledStart: time.Now().Add(time.Hour),
		ScheduledEnd:   time.Now().Add(2 * time.Hour),
	})
	require.NoError(t, err)

	session, err := schedules.StartScheduledSession(ctx, sched.ID, teacher.ID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `UPDATE scheduled_sessions SET live_session_id = NULL WHERE id = $1`, sched.ID)
	require.NoError(t, err)

	h := &ScheduleHandler{Schedules: schedules}
	req := httptest.NewRequest(http.MethodGet, "/api/classes/"+class.ID+"/schedule", nil)
	req = withChiParams(req, map[string]string{"classId": class.ID})
	req = withClaims(req, &auth.Claims{UserID: teacher.ID})
	w := httptest.NewRecorder()
	h.List(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var payload []store.ScheduledSession
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload, 1)
	require.NotNil(t, payload[0].LiveSessionID)
	assert.Equal(t, session.ID, *payload[0].LiveSessionID)
}
