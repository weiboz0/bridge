package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// TeacherProblemHandler serves teacher-side reads of student attempts.
// Used by the live-watch UI (plan 025b) and by the Hocuspocus auth
// callback (server-to-server check that a teacher may read an attempt).
type TeacherProblemHandler struct {
	Problems *store.ProblemStore
	Topics   *store.TopicStore
	Classes  *store.ClassStore
	Attempts *store.AttemptStore
}

func (h *TeacherProblemHandler) Routes(r chi.Router) {
	r.Route("/api/teacher/problems/{problemId}/students/{studentId}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("problemId"))
		r.Use(ValidateUUIDParam("studentId"))
		r.Get("/attempts", h.ListStudentAttempts)
	})
}

// canTeacherViewStudent — caller must be a class instructor in the
// problem's course AND the student must be a member of one such class.
func (h *TeacherProblemHandler) canTeacherViewStudent(
	r *http.Request,
	problem *store.Problem,
	studentID string,
	claims *auth.Claims,
) (bool, error) {
	if claims.IsPlatformAdmin {
		return true, nil
	}
	topic, err := h.Topics.GetTopic(r.Context(), problem.TopicID)
	if err != nil {
		return false, err
	}
	if topic == nil {
		return false, nil
	}
	return h.Classes.TeacherCanViewStudentInCourse(r.Context(), claims.UserID, studentID, topic.CourseID)
}

// ListStudentAttempts handles
// GET /api/teacher/problems/{problemId}/students/{studentId}/attempts.
func (h *TeacherProblemHandler) ListStudentAttempts(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	problemID := chi.URLParam(r, "problemId")
	studentID := chi.URLParam(r, "studentId")

	problem, err := h.Problems.GetProblem(r.Context(), problemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if problem == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	canView, err := h.canTeacherViewStudent(r, problem, studentID, claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !canView {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	list, err := h.Attempts.ListByUserAndProblem(r.Context(), problemID, studentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, list)
}
