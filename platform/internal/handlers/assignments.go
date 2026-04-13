package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type AssignmentHandler struct {
	Assignments *store.AssignmentStore
	Classes     *store.ClassStore
}

func (h *AssignmentHandler) Routes(r chi.Router) {
	r.Route("/api/assignments", func(r chi.Router) {
		r.Post("/", h.CreateAssignment)
		r.Get("/", h.ListAssignments)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetAssignment)
			r.Patch("/", h.UpdateAssignment)
			r.Delete("/", h.DeleteAssignment)
			r.Post("/submit", h.SubmitAssignment)
			r.Get("/submissions", h.ListSubmissions)
		})
	})
}

func (h *AssignmentHandler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		ClassID     string  `json:"classId"`
		TopicID     *string `json:"topicId"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		StarterCode *string `json:"starterCode"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ClassID == "" {
		writeError(w, http.StatusBadRequest, "classId is required")
		return
	}
	if body.Title == "" || len(body.Title) > 255 {
		writeError(w, http.StatusBadRequest, "title is required (max 255 chars)")
		return
	}

	assignment, err := h.Assignments.CreateAssignment(r.Context(), store.CreateAssignmentInput{
		ClassID:     body.ClassID,
		TopicID:     body.TopicID,
		Title:       body.Title,
		Description: body.Description,
		StarterCode: body.StarterCode,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create assignment")
		return
	}
	writeJSON(w, http.StatusCreated, assignment)
}

func (h *AssignmentHandler) ListAssignments(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := r.URL.Query().Get("classId")
	if classID == "" {
		writeError(w, http.StatusBadRequest, "classId query parameter is required")
		return
	}

	assignments, err := h.Assignments.ListAssignmentsByClass(r.Context(), classID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, assignments)
}

func (h *AssignmentHandler) GetAssignment(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	assignment, err := h.Assignments.GetAssignment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if assignment == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, assignment)
}

func (h *AssignmentHandler) UpdateAssignment(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body store.UpdateAssignmentInput
	if !decodeJSON(w, r, &body) {
		return
	}

	updated, err := h.Assignments.UpdateAssignment(r.Context(), chi.URLParam(r, "id"), body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *AssignmentHandler) DeleteAssignment(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	deleted, err := h.Assignments.DeleteAssignment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if deleted == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *AssignmentHandler) SubmitAssignment(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	assignmentID := chi.URLParam(r, "id")

	var body struct {
		DocumentID *string `json:"documentId"`
	}
	_ = decodeJSON(w, r, &body) // body is optional

	submission, err := h.Assignments.CreateSubmission(r.Context(), assignmentID, claims.UserID, body.DocumentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if submission == nil {
		writeError(w, http.StatusConflict, "Already submitted")
		return
	}
	writeJSON(w, http.StatusCreated, submission)
}

func (h *AssignmentHandler) ListSubmissions(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	submissions, err := h.Assignments.ListSubmissionsByAssignment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, submissions)
}

// SubmissionHandler for grading
type SubmissionHandler struct {
	Assignments *store.AssignmentStore
}

func (h *SubmissionHandler) Routes(r chi.Router) {
	r.Route("/api/submissions", func(r chi.Router) {
		r.Route("/{id}", func(r chi.Router) {
			r.Patch("/", h.GradeSubmission)
		})
	})
}

func (h *SubmissionHandler) GradeSubmission(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		Grade    float64 `json:"grade"`
		Feedback *string `json:"feedback"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Grade < 0 || body.Grade > 100 {
		writeError(w, http.StatusBadRequest, "grade must be 0-100")
		return
	}

	graded, err := h.Assignments.GradeSubmission(r.Context(), chi.URLParam(r, "id"), body.Grade, body.Feedback)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if graded == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, graded)
}
