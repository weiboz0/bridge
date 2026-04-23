package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// TopicProblemHandler manages the topic_problems join table — attaching,
// detaching, and reordering problems within a topic. The GET list endpoint
// lives on ProblemHandler.ListProblemsByTopic to keep all problem-listing
// logic in one place.
type TopicProblemHandler struct {
	Problems      *store.ProblemStore
	TopicProblems *store.TopicProblemStore
	Topics        *store.TopicStore
	Courses       *store.CourseStore
	Orgs          *store.OrgStore
}

// accessDeps constructs a problemAccessDeps from the handler's fields.
func (h *TopicProblemHandler) accessDeps() problemAccessDeps {
	return problemAccessDeps{
		Problems:      h.Problems,
		TopicProblems: h.TopicProblems,
		Topics:        h.Topics,
		Courses:       h.Courses,
		Orgs:          h.Orgs,
	}
}

func (h *TopicProblemHandler) Routes(r chi.Router, listHandler func(http.ResponseWriter, *http.Request)) {
	r.Route("/api/topics/{topicId}/problems", func(r chi.Router) {
		r.Use(ValidateUUIDParam("topicId"))
		r.Get("/", listHandler)
		r.Post("/", h.AttachProblem)
	})
	r.Route("/api/topics/{topicId}/problems/{problemId}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("topicId"), ValidateUUIDParam("problemId"))
		r.Delete("/", h.DetachProblem)
		r.Patch("/", h.ReorderProblem)
	})
}

// isTopicEditor returns true when the caller is a teacher or org_admin in the
// org that owns the topic's course, or a platform admin.
func (h *TopicProblemHandler) isTopicEditor(r *http.Request, topicID string, claims *auth.Claims) (bool, int) {
	if claims.IsPlatformAdmin {
		return true, 0
	}
	topic, err := h.Topics.GetTopic(r.Context(), topicID)
	if err != nil {
		return false, http.StatusInternalServerError
	}
	if topic == nil {
		return false, http.StatusNotFound
	}
	course, err := h.Courses.GetCourse(r.Context(), topic.CourseID)
	if err != nil {
		return false, http.StatusInternalServerError
	}
	if course == nil {
		return false, http.StatusNotFound
	}
	roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), course.OrgID, claims.UserID)
	if err != nil {
		return false, http.StatusInternalServerError
	}
	for _, m := range roles {
		if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
			return true, 0
		}
	}
	return false, http.StatusForbidden
}

// AttachProblem — POST /api/topics/{topicId}/problems
// Body: { problemId: string, sortOrder?: int }
// Requires: caller is a teacher/admin in the topic's org, and the problem is
// published and visible to the caller.
func (h *TopicProblemHandler) AttachProblem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	topicID := chi.URLParam(r, "topicId")

	ok, status := h.isTopicEditor(r, topicID, claims)
	if !ok {
		switch status {
		case http.StatusNotFound:
			writeError(w, http.StatusNotFound, "Not found")
		case http.StatusInternalServerError:
			writeError(w, http.StatusInternalServerError, "Database error")
		default:
			writeError(w, http.StatusForbidden, "Forbidden")
		}
		return
	}

	var body struct {
		ProblemID string `json:"problemId"`
		SortOrder *int   `json:"sortOrder"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ProblemID == "" {
		writeError(w, http.StatusBadRequest, "problemId is required")
		return
	}

	// Problem must be published and visible to the caller.
	p, err := h.Problems.GetProblem(r.Context(), body.ProblemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if p.Status != "published" {
		writeError(w, http.StatusConflict, "problem is not published")
		return
	}
	if !canViewProblemRow(r.Context(), h.accessDeps(), claims, p) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	order := 0
	if body.SortOrder != nil {
		order = *body.SortOrder
	}
	att, err := h.TopicProblems.Attach(r.Context(), topicID, body.ProblemID, order, claims.UserID)
	switch {
	case errors.Is(err, store.ErrAlreadyAttached):
		writeError(w, http.StatusConflict, "already attached")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "Database error")
	default:
		writeJSON(w, http.StatusCreated, att)
	}
}

// DetachProblem — DELETE /api/topics/{topicId}/problems/{problemId}
// Requires: caller is a teacher/admin in the topic's org.
func (h *TopicProblemHandler) DetachProblem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	topicID := chi.URLParam(r, "topicId")
	problemID := chi.URLParam(r, "problemId")

	ok, status := h.isTopicEditor(r, topicID, claims)
	if !ok {
		switch status {
		case http.StatusNotFound:
			writeError(w, http.StatusNotFound, "Not found")
		case http.StatusInternalServerError:
			writeError(w, http.StatusInternalServerError, "Database error")
		default:
			writeError(w, http.StatusForbidden, "Forbidden")
		}
		return
	}

	deleted, err := h.TopicProblems.Detach(r.Context(), topicID, problemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReorderProblem — PATCH /api/topics/{topicId}/problems/{problemId}
// Body: { sortOrder: int }
// Requires: caller is a teacher/admin in the topic's org.
func (h *TopicProblemHandler) ReorderProblem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	topicID := chi.URLParam(r, "topicId")
	problemID := chi.URLParam(r, "problemId")

	ok, status := h.isTopicEditor(r, topicID, claims)
	if !ok {
		switch status {
		case http.StatusNotFound:
			writeError(w, http.StatusNotFound, "Not found")
		case http.StatusInternalServerError:
			writeError(w, http.StatusInternalServerError, "Database error")
		default:
			writeError(w, http.StatusForbidden, "Forbidden")
		}
		return
	}

	var body struct {
		SortOrder int `json:"sortOrder"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	att, err := h.TopicProblems.SetSortOrder(r.Context(), topicID, problemID, body.SortOrder)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if att == nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	writeJSON(w, http.StatusOK, att)
}
