package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the auth guard paths — every handler must return 401
// when no claims are present. Richer access-control tests that need a real
// DB live in problems_integration_test.go (separate file, DB-gated).

func TestListProblems_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/problems", nil)
	w := httptest.NewRecorder()
	h.ListProblems(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListProblemsByTopic_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/topics/abc/problems", nil)
	req = withChiParams(req, map[string]string{"topicId": "abc"})
	w := httptest.NewRecorder()
	h.ListProblemsByTopic(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateProblem_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	body, _ := json.Marshal(map[string]string{"title": "x", "scope": "personal"})
	req := httptest.NewRequest(http.MethodPost, "/api/problems", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPublishProblem_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/problems/abc/publish", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.PublishProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestArchiveProblem_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/problems/abc/archive", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.ArchiveProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUnarchiveProblem_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/problems/abc/unarchive", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.UnarchiveProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestForkProblem_NoClaims(t *testing.T) {
	h := &ProblemHandler{}
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/problems/abc/fork", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.ForkProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// Pure-function tests for cursor round-trip — exercised without a DB so they
// run as part of the default `go test ./...`.
func TestDecodeCursor_EmptyInput(t *testing.T) {
	ts, id, err := decodeCursor("")
	require.NoError(t, err)
	assert.Nil(t, ts)
	assert.Nil(t, id)
}

func TestEncodeDecodeCursor_RoundTrip(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 5, 678000000, time.UTC)
	id := "11111111-2222-3333-4444-555555555555"
	c := encodeCursor(when, id)
	assert.NotEmpty(t, c)
	gotTime, gotID, err := decodeCursor(c)
	require.NoError(t, err)
	require.NotNil(t, gotTime)
	require.NotNil(t, gotID)
	assert.True(t, gotTime.Equal(when), "round-tripped time should equal input")
	assert.Equal(t, id, *gotID)
}

func TestDecodeCursor_Malformed(t *testing.T) {
	_, _, err := decodeCursor("not-base64!!")
	assert.Error(t, err)
}

func TestDecodeCursor_MissingSeparator(t *testing.T) {
	// "nosep" base64-encoded, but no "|"
	raw := "bm9zZXA"
	_, _, err := decodeCursor(raw)
	assert.Error(t, err)
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
