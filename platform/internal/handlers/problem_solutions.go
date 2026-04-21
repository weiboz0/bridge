package handlers

import (
	"context"
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

// ---------- access helpers (duplicated from ProblemHandler; Task 7 extracts) ----------

// solAuthorizedForScope mirrors ProblemHandler.authorizedForScope.
func (h *SolutionHandler) solAuthorizedForScope(ctx context.Context, c *auth.Claims, scope string, scopeID *string) bool {
	switch scope {
	case "platform":
		return c.IsPlatformAdmin
	case "org":
		if scopeID == nil {
			return false
		}
		roles, err := h.Orgs.GetUserRolesInOrg(ctx, *scopeID, c.UserID)
		if err != nil || len(roles) == 0 {
			return false
		}
		for _, m := range roles {
			if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
				return true
			}
		}
		return false
	case "personal":
		return scopeID != nil && *scopeID == c.UserID
	default:
		return false
	}
}

// canEditProblem mirrors ProblemHandler.authorizedForProblemEdit.
func (h *SolutionHandler) canEditProblem(ctx context.Context, c *auth.Claims, problemID string) bool {
	p, err := h.Problems.GetProblem(ctx, problemID)
	if err != nil || p == nil {
		return false
	}
	if h.solAuthorizedForScope(ctx, c, p.Scope, p.ScopeID) {
		return true
	}
	return p.Scope == "personal" && p.CreatedBy == c.UserID
}

// canViewTopic mirrors ProblemHandler.canViewTopic.
func (h *SolutionHandler) canViewTopic(ctx context.Context, topicID string, claims *auth.Claims) (bool, error) {
	if claims.IsPlatformAdmin {
		return true, nil
	}
	topic, err := h.Topics.GetTopic(ctx, topicID)
	if err != nil || topic == nil {
		return false, err
	}
	course, err := h.Courses.GetCourse(ctx, topic.CourseID)
	if err != nil || course == nil {
		return false, err
	}
	if course.CreatedBy == claims.UserID {
		return true, nil
	}
	hasAccess, err := h.Courses.UserHasAccessToCourse(ctx, course.ID, claims.UserID)
	if err != nil {
		return false, err
	}
	return hasAccess, nil
}

// canViewProblem mirrors ProblemHandler.canViewProblem. Returns (ok, problem).
func (h *SolutionHandler) canViewProblem(ctx context.Context, c *auth.Claims, problemID string) (bool, *store.Problem) {
	p, err := h.Problems.GetProblem(ctx, problemID)
	if err != nil || p == nil {
		return false, p
	}
	if c.IsPlatformAdmin {
		return true, p
	}
	// Drafts are editor-only.
	if p.Status == "draft" {
		ok := h.canEditProblem(ctx, c, p.ID)
		return ok, p
	}
	switch p.Scope {
	case "platform":
		return p.Status == "published" || p.Status == "archived", p
	case "org":
		if p.ScopeID == nil {
			break
		}
		roles, err := h.Orgs.GetUserRolesInOrg(ctx, *p.ScopeID, c.UserID)
		if err != nil {
			break
		}
		for _, m := range roles {
			if m.Status == "active" {
				return true, p
			}
		}
	case "personal":
		if p.ScopeID != nil && *p.ScopeID == c.UserID {
			return true, p
		}
	}
	// Attachment grant.
	topicIDs, err := h.TopicProblems.ListTopicsByProblem(ctx, p.ID)
	if err != nil {
		return false, p
	}
	for _, tid := range topicIDs {
		ok, err := h.canViewTopic(ctx, tid, c)
		if err == nil && ok {
			return true, p
		}
	}
	return false, p
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
	canView, p := h.canViewProblem(r.Context(), claims, id)
	if p == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !canView {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	includeDrafts := h.canEditProblem(r.Context(), claims, id)
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
	if !h.canEditProblem(r.Context(), claims, id) {
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
	if !h.canEditProblem(r.Context(), claims, id) {
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
	if !h.canEditProblem(r.Context(), claims, id) {
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
	if !h.canEditProblem(r.Context(), claims, id) {
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
