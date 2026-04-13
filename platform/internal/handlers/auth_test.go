package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegister_MissingName(t *testing.T) {
	h := &AuthHandler{}
	body, _ := json.Marshal(map[string]string{
		"email": "test@example.com", "password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRegister_ShortPassword(t *testing.T) {
	h := &AuthHandler{}
	body, _ := json.Marshal(map[string]string{
		"name": "Test", "email": "test@example.com", "password": "short",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRegister_MissingEmail(t *testing.T) {
	h := &AuthHandler{}
	body, _ := json.Marshal(map[string]string{
		"name": "Test", "password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRegister_InvalidJSON(t *testing.T) {
	h := &AuthHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
