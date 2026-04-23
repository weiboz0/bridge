package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// SolutionHandler serves CRUD + publish/unpublish for problem solutions.
// Solutions are nested under problems: /api/problems/{id}/solutions[/{solutionId}].
// Access control mirrors ProblemHandler: viewers may see published solutions;
// only editors (same logic as authorizedForProblemEdit) may see drafts or
// mutate any solution.
type SolutionHandler struct {
	Problems      *store.ProblemStore
	Solutions     *store.ProblemSolutionStore
	Orgs          *store.OrgStore
	TopicProblems *store.TopicProblemStore
	Topics        *store.TopicStore
	Courses       *store.CourseStore
}

func (h *SolutionHandler) Routes(r chi.Router) {
	r.Route("/api/problems/{id}/solutions", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Get("/", h.ListSolutions)
		r.Post("/", h.CreateSolution)
	})
	r.Route("/api/problems/{id}/solutions/{solutionId}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"), ValidateUUIDParam("solutionId"))
		r.Patch("/", h.UpdateSolution)
		r.Delete("/", h.DeleteSolution)
		r.Post("/publish", h.PublishSolution)
		r.Post("/unpublish", h.UnpublishSolution)
	})
}

// ---------- access helpers ----------

// accessDeps constructs a problemAccessDeps from the handler's fields.
func (h *SolutionHandler) accessDeps() problemAccessDeps {
	return problemAccessDeps{
		Problems:      h.Problems,
		TopicProblems: h.TopicProblems,
		Topics:        h.Topics,
		Courses:       h.Courses,
		Orgs:          h.Orgs,
	}
}

// ---------- handlers ----------

// ListSolutions — GET /api/problems/{id}/solutions
// Editors see all (drafts + published); viewers see published only.
func (h *SolutionHandler) ListSolutions(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	canView, p, cvStatus := canViewProblem(r.Context(), h.accessDeps(), claims, id)
	if cvStatus == http.StatusInternalServerError {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !canView {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	includeDrafts := authorizedForProblemEdit(r.Context(), h.accessDeps(), claims, id)
	list, err := h.Solutions.ListByProblem(r.Context(), id, includeDrafts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// CreateSolution — POST /api/problems/{id}/solutions
// Requires problem edit access.
func (h *SolutionHandler) CreateSolution(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	if !authorizedForProblemEdit(r.Context(), h.accessDeps(), claims, id) {
		writeError(w, http.StatusForbidden, "Not authorized to edit problem")
		return
	}

	var body struct {
		Language     string   `json:"language"`
		Title        *string  `json:"title"`
		Code         string   `json:"code"`
		Notes        *string  `json:"notes"`
		ApproachTags []string `json:"approachTags"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Language == "" {
		writeError(w, http.StatusBadRequest, "language is required")
		return
	}
	if body.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	sol, err := h.Solutions.CreateSolution(r.Context(), store.CreateSolutionInput{
		ProblemID:    id,
		Language:     body.Language,
		Title:        body.Title,
		Code:         body.Code,
		Notes:        body.Notes,
		ApproachTags: body.ApproachTags,
		CreatedBy:    claims.UserID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create solution")
		return
	}
	writeJSON(w, http.StatusCreated, sol)
}

// UpdateSolution — PATCH /api/problems/{id}/solutions/{solutionId}
// Requires problem edit access.
func (h *SolutionHandler) UpdateSolution(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	solutionID := chi.URLParam(r, "solutionId")
	if !authorizedForProblemEdit(r.Context(), h.accessDeps(), claims, id) {
		writeError(w, http.StatusForbidden, "Not authorized to edit problem")
		return
	}

	sol, err := h.Solutions.GetSolution(r.Context(), solutionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if sol == nil || sol.ProblemID != id {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	var body store.UpdateSolutionInput
	if !decodeJSON(w, r, &body) {
		return
	}
	updated, err := h.Solutions.UpdateSolution(r.Context(), solutionID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteSolution — DELETE /api/problems/{id}/solutions/{solutionId}
// Requires problem edit access. Returns 204 on success.
func (h *SolutionHandler) DeleteSolution(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	solutionID := chi.URLParam(r, "solutionId")
	if !authorizedForProblemEdit(r.Context(), h.accessDeps(), claims, id) {
		writeError(w, http.StatusForbidden, "Not authorized to edit problem")
		return
	}

	sol, err := h.Solutions.GetSolution(r.Context(), solutionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if sol == nil || sol.ProblemID != id {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	if _, err := h.Solutions.DeleteSolution(r.Context(), solutionID); err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PublishSolution — POST /api/problems/{id}/solutions/{solutionId}/publish
// Sets is_published = true. Idempotent (no 409 if already published).
func (h *SolutionHandler) PublishSolution(w http.ResponseWriter, r *http.Request) {
	h.setPublished(w, r, true)
}

// UnpublishSolution — POST /api/problems/{id}/solutions/{solutionId}/unpublish
// Sets is_published = false. Idempotent.
func (h *SolutionHandler) UnpublishSolution(w http.ResponseWriter, r *http.Request) {
	h.setPublished(w, r, false)
}

func (h *SolutionHandler) setPublished(w http.ResponseWriter, r *http.Request, published bool) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	solutionID := chi.URLParam(r, "solutionId")
	if !authorizedForProblemEdit(r.Context(), h.accessDeps(), claims, id) {
		writeError(w, http.StatusForbidden, "Not authorized to edit problem")
		return
	}

	sol, err := h.Solutions.GetSolution(r.Context(), solutionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if sol == nil || sol.ProblemID != id {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	updated, err := h.Solutions.SetPublished(r.Context(), solutionID, published)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
