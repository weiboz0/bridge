package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/weiboz0/bridge/platform/internal/auth"
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
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
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
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
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
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
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
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
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
