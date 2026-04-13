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

func TestCreateCourse_NoClaims(t *testing.T) {
	h := &CourseHandler{}
	body, _ := json.Marshal(map[string]string{"title": "Test", "orgId": "org-1", "gradeLevel": "K-5"})
	req := httptest.NewRequest(http.MethodPost, "/api/courses", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateCourse(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateCourse_MissingTitle(t *testing.T) {
	h := &CourseHandler{}
	body, _ := json.Marshal(map[string]string{"orgId": "org-1", "gradeLevel": "K-5"})
	req := httptest.NewRequest(http.MethodPost, "/api/courses", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.CreateCourse(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateCourse_MissingOrgId(t *testing.T) {
	h := &CourseHandler{}
	body, _ := json.Marshal(map[string]string{"title": "Test", "gradeLevel": "K-5"})
	req := httptest.NewRequest(http.MethodPost, "/api/courses", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.CreateCourse(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateCourse_InvalidGradeLevel(t *testing.T) {
	h := &CourseHandler{}
	body, _ := json.Marshal(map[string]string{"title": "Test", "orgId": "org-1", "gradeLevel": "invalid"})
	req := httptest.NewRequest(http.MethodPost, "/api/courses", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.CreateCourse(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateCourse_InvalidLanguage(t *testing.T) {
	h := &CourseHandler{}
	body, _ := json.Marshal(map[string]string{"title": "Test", "orgId": "org-1", "gradeLevel": "K-5", "language": "ruby"})
	req := httptest.NewRequest(http.MethodPost, "/api/courses", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.CreateCourse(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListCourses_NoClaims(t *testing.T) {
	h := &CourseHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/courses?orgId=org-1", nil)
	w := httptest.NewRecorder()
	h.ListCourses(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListCourses_MissingOrgId(t *testing.T) {
	h := &CourseHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/courses", nil)
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.ListCourses(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetCourse_NoClaims(t *testing.T) {
	h := &CourseHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/courses/123", nil)
	req = withChiParams(req, map[string]string{"id": "123"})
	w := httptest.NewRecorder()
	h.GetCourse(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUpdateCourse_NoClaims(t *testing.T) {
	h := &CourseHandler{}
	body, _ := json.Marshal(map[string]string{"title": "Updated"})
	req := httptest.NewRequest(http.MethodPatch, "/api/courses/123", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "123"})
	w := httptest.NewRecorder()
	h.UpdateCourse(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDeleteCourse_NoClaims(t *testing.T) {
	h := &CourseHandler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/courses/123", nil)
	req = withChiParams(req, map[string]string{"id": "123"})
	w := httptest.NewRecorder()
	h.DeleteCourse(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCloneCourse_NoClaims(t *testing.T) {
	h := &CourseHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/courses/123/clone", nil)
	req = withChiParams(req, map[string]string{"id": "123"})
	w := httptest.NewRecorder()
	h.CloneCourse(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
