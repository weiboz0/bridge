package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// These tests exercise the auth guard paths — every handler must return 401
// when no claims are present. Richer access-control tests that need a real
// DB live in problems_integration_test.go (separate file, DB-gated).

func TestListProblems_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/topics/abc/problems", nil)
	req = withChiParams(req, map[string]string{"topicId": "abc"})
	w := httptest.NewRecorder()
	h.ListProblems(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateProblem_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	body, _ := json.Marshal(map[string]string{"title": "x", "language": "python"})
	req := httptest.NewRequest(http.MethodPost, "/api/topics/abc/problems", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"topicId": "abc"})
	w := httptest.NewRecorder()
	h.CreateProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetProblem_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/problems/abc", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.GetProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUpdateProblem_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	body, _ := json.Marshal(map[string]string{"title": "renamed"})
	req := httptest.NewRequest(http.MethodPatch, "/api/problems/abc", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.UpdateProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDeleteProblem_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/problems/abc", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.DeleteProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListTestCases_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/problems/abc/test-cases", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.ListTestCases(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateTestCase_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	body, _ := json.Marshal(map[string]string{"stdin": "1"})
	req := httptest.NewRequest(http.MethodPost, "/api/problems/abc/test-cases", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.CreateTestCase(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUpdateTestCase_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	body, _ := json.Marshal(map[string]string{"name": "renamed"})
	req := httptest.NewRequest(http.MethodPatch, "/api/test-cases/abc", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.UpdateTestCase(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDeleteTestCase_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/test-cases/abc", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.DeleteTestCase(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListAttempts_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/problems/abc/attempts", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.ListAttempts(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateAttempt_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	body, _ := json.Marshal(map[string]string{"plainText": "print(1)"})
	req := httptest.NewRequest(http.MethodPost, "/api/problems/abc/attempts", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.CreateAttempt(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetAttempt_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/attempts/abc", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.GetAttempt(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUpdateAttempt_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	body, _ := json.Marshal(map[string]string{"plainText": "v2"})
	req := httptest.NewRequest(http.MethodPatch, "/api/attempts/abc", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.UpdateAttempt(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDeleteAttempt_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/attempts/abc", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.DeleteAttempt(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
