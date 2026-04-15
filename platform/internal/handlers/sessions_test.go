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

func TestCreateSession_NoClaims(t *testing.T) {
	h := &SessionHandler{}
	body, _ := json.Marshal(map[string]string{"classId": "c1"})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateSession(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateSession_MissingClassroomId(t *testing.T) {
	h := &SessionHandler{}
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.CreateSession(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetSession_NoClaims(t *testing.T) {
	h := &SessionHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1", nil)
	req = withChiParams(req, map[string]string{"id": "s1"})
	w := httptest.NewRecorder()
	h.GetSession(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJoinSession_NoClaims(t *testing.T) {
	h := &SessionHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/s1/join", nil)
	req = withChiParams(req, map[string]string{"id": "s1"})
	w := httptest.NewRecorder()
	h.JoinSession(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLeaveSession_NoClaims(t *testing.T) {
	h := &SessionHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/s1/leave", nil)
	req = withChiParams(req, map[string]string{"id": "s1"})
	w := httptest.NewRecorder()
	h.LeaveSession(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetParticipants_NoClaims(t *testing.T) {
	h := &SessionHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1/participants", nil)
	req = withChiParams(req, map[string]string{"id": "s1"})
	w := httptest.NewRecorder()
	h.GetParticipants(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestToggleHelp_NoClaims(t *testing.T) {
	h := &SessionHandler{}
	body, _ := json.Marshal(map[string]any{"raised": true})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/s1/help-queue", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "s1"})
	w := httptest.NewRecorder()
	h.ToggleHelp(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestToggleBroadcast_NoClaims(t *testing.T) {
	h := &SessionHandler{}
	body, _ := json.Marshal(map[string]any{"active": true})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/s1/broadcast", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "s1"})
	w := httptest.NewRecorder()
	h.ToggleBroadcast(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
