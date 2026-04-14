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

func TestCreateClass_NoClaims(t *testing.T) {
	h := &ClassHandler{}
	body, _ := json.Marshal(map[string]string{"courseId": "c1", "orgId": "o1", "title": "Test"})
	req := httptest.NewRequest(http.MethodPost, "/api/classes", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateClass(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateClass_MissingCourseId(t *testing.T) {
	h := &ClassHandler{}
	body, _ := json.Marshal(map[string]string{"orgId": "o1", "title": "Test"})
	req := httptest.NewRequest(http.MethodPost, "/api/classes", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.CreateClass(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateClass_MissingTitle(t *testing.T) {
	h := &ClassHandler{}
	body, _ := json.Marshal(map[string]string{"courseId": "c1", "orgId": "o1"})
	req := httptest.NewRequest(http.MethodPost, "/api/classes", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.CreateClass(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListClasses_NoClaims(t *testing.T) {
	h := &ClassHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/classes?orgId=o1", nil)
	w := httptest.NewRecorder()
	h.ListClasses(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListClasses_MissingOrgId(t *testing.T) {
	h := &ClassHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/classes", nil)
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.ListClasses(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetClass_NoClaims(t *testing.T) {
	h := &ClassHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/classes/c1", nil)
	req = withChiParams(req, map[string]string{"id": "c1"})
	w := httptest.NewRecorder()
	h.GetClass(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJoinClass_NoClaims(t *testing.T) {
	h := &ClassHandler{}
	body, _ := json.Marshal(map[string]string{"joinCode": "ABCD1234"})
	req := httptest.NewRequest(http.MethodPost, "/api/classes/join", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.JoinClass(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJoinClass_MissingCode(t *testing.T) {
	h := &ClassHandler{}
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/classes/join", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.JoinClass(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAddClassMember_NoClaims(t *testing.T) {
	h := &ClassHandler{}
	body, _ := json.Marshal(map[string]string{"email": "test@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/classes/c1/members", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "c1"})
	w := httptest.NewRecorder()
	h.AddMember(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAddClassMember_MissingEmail(t *testing.T) {
	h := &ClassHandler{}
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/classes/c1/members", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "c1"})
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.AddMember(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
