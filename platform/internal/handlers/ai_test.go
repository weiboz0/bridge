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

func TestChat_NoClaims(t *testing.T) {
	h := &AIHandler{}
	body, _ := json.Marshal(map[string]string{"sessionId": "s1", "message": "hi"})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Chat(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestChat_MissingSessionId(t *testing.T) {
	h := &AIHandler{}
	body, _ := json.Marshal(map[string]string{"message": "hi"})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.Chat(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChat_MissingMessage(t *testing.T) {
	h := &AIHandler{}
	body, _ := json.Marshal(map[string]string{"sessionId": "s1"})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.Chat(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChat_MessageTooLong(t *testing.T) {
	h := &AIHandler{}
	longMsg := string(make([]byte, 5001))
	body, _ := json.Marshal(map[string]string{"sessionId": "s1", "message": longMsg})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.Chat(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestToggle_NoClaims(t *testing.T) {
	h := &AIHandler{}
	body, _ := json.Marshal(map[string]any{"sessionId": "s1", "studentId": "st1", "enabled": true})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/toggle", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Toggle(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestToggle_MissingFields(t *testing.T) {
	h := &AIHandler{}
	body, _ := json.Marshal(map[string]any{"enabled": true})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/toggle", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.Toggle(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListInteractions_NoClaims(t *testing.T) {
	h := &AIHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/ai/interactions?sessionId=s1", nil)
	w := httptest.NewRecorder()
	h.ListInteractions(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListInteractions_MissingSessionId(t *testing.T) {
	h := &AIHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/ai/interactions", nil)
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.ListInteractions(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
