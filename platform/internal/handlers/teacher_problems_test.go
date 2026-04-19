package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListStudentAttempts_NoClaims(t *testing.T) {
	h := &TeacherProblemHandler{}
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/teacher/problems/abc/students/xyz/attempts",
		nil,
	)
	req = withChiParams(req, map[string]string{"problemId": "abc", "studentId": "xyz"})
	w := httptest.NewRecorder()
	h.ListStudentAttempts(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
