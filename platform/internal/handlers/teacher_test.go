package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTeacherDashboard_NoClaims(t *testing.T) {
	h := &TeacherHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/teacher/dashboard", nil)
	w := httptest.NewRecorder()
	h.Dashboard(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTeacherCourses_NoClaims(t *testing.T) {
	h := &TeacherHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/teacher/courses", nil)
	w := httptest.NewRecorder()
	h.CoursesWithOrgs(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
