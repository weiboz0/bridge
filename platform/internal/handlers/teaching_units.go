package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// TeachingUnitHandler serves teaching unit CRUD and document endpoints.
// Access is scope-based (platform / org / personal) per spec 012.
// Lifecycle transitions (publish/archive) and overlay/fork routes are
// excluded from this handler — they land in plans 033 and 034.
type TeachingUnitHandler struct {
	Units *store.TeachingUnitStore
	Orgs  *store.OrgStore
}

// validUnitScopes is the allowed set of scope values.
var validUnitScopes = map[string]bool{
	"platform": true,
	"org":      true,
	"personal": true,
}

// validUnitStatuses is the allowed set of status values for initial creation /
// manual updates in plan 031. Lifecycle transitions (draft → reviewed → ready)
// land in plan 033.
var validUnitStatuses = map[string]bool{
	"draft":           true,
	"reviewed":        true,
	"classroom_ready": true,
	"coach_ready":     true,
	"archived":        true,
}

// knownBlockTypes is the plan-031 allowlist for top-level block types in a
// unit document. It is kept as a package-level variable so tests can inspect
// it directly.
var knownBlockTypes = map[string]bool{
	"prose":          true,
	"problem-ref":    true,
	"paragraph":      true,
	"heading":        true,
	"bulletList":     true,
	"orderedList":    true,
	"listItem":       true,
	"codeBlock":      true,
	"blockquote":     true,
	"horizontalRule": true,
	"hardBreak":      true,
	"teacher-note":   true,
	"code-snippet":   true,
	"media-embed":    true,
}

const maxUnitTitleLen = 255

func (h *TeachingUnitHandler) Routes(r chi.Router) {
	r.Route("/api/units", func(r chi.Router) {
		r.Get("/", h.ListUnits)
		r.Post("/", h.CreateUnit)
	})
	// by-topic lookup must be registered BEFORE the /{id} wildcard route so Chi
	// does not attempt to parse "by-topic" as a UUID parameter.
	r.Route("/api/units/by-topic/{topicId}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("topicId"))
		r.Get("/", h.GetUnitByTopic)
	})
	r.Route("/api/units/{id}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Get("/", h.GetUnit)
		r.Patch("/", h.UpdateUnit)
		r.Delete("/", h.DeleteUnit)
		r.Get("/document", h.GetDocument)
		r.Put("/document", h.SaveDocument)
		r.Post("/transition", h.TransitionUnit)
		r.Get("/revisions", h.ListRevisions)
		r.Route("/revisions/{revisionId}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("revisionId"))
			r.Get("/", h.GetRevision)
		})
	})
}

// ---------- Access helpers ----------

// canViewUnit applies the Access table from spec 012 §Access, with the
// plan-031-specific narrowing: org students are denied access entirely
// until class-binding lands in plan 032.
//
// Platform scope: published/archived/coach_ready/classroom_ready → any auth;
// draft/reviewed → platform admin only.
// Org scope: teachers/org_admins see all statuses; students denied.
// Personal scope: owner only.
// Platform admin bypass applies everywhere.
func (h *TeachingUnitHandler) canViewUnit(ctx context.Context, c *auth.Claims, u *store.TeachingUnit) bool {
	if c.IsPlatformAdmin {
		return true
	}
	switch u.Scope {
	case "platform":
		return u.Status == "classroom_ready" || u.Status == "coach_ready" || u.Status == "archived"
	case "org":
		if u.ScopeID == nil {
			return false
		}
		roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *u.ScopeID, c.UserID)
		for _, m := range roles {
			if m.Status != "active" {
				continue
			}
			// Plan 031 only grants teachers and org_admins access. Students
			// are denied until plan 032 wires class/session binding.
			if m.Role == "org_admin" || m.Role == "teacher" {
				return true
			}
		}
		return false
	case "personal":
		return u.ScopeID != nil && *u.ScopeID == c.UserID
	}
	return false
}

// canEditUnit checks whether the caller may create, update, or delete a unit
// in the given scope. Platform requires platform admin; org requires an active
// org_admin or teacher; personal requires the caller to be the scope owner.
func (h *TeachingUnitHandler) canEditUnit(ctx context.Context, c *auth.Claims, scope string, scopeID *string) bool {
	if c.IsPlatformAdmin && scope == "platform" {
		return true
	}
	switch scope {
	case "platform":
		return c.IsPlatformAdmin
	case "org":
		if scopeID == nil {
			return false
		}
		roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *scopeID, c.UserID)
		for _, m := range roles {
			if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
				return true
			}
		}
		return false
	case "personal":
		return scopeID != nil && *scopeID == c.UserID
	}
	return false
}

// ---------- Document validation ----------

// validateBlockDocument enforces the spec-012 block-ID invariant and the
// plan-031 block-type allowlist. Returns a non-nil error with a descriptive
// message if the document fails validation.
func validateBlockDocument(raw json.RawMessage) error {
	// Decode just enough to inspect the envelope.
	var envelope struct {
		Type    string            `json:"type"`
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("document must be valid JSON")
	}
	if envelope.Type != "doc" {
		return fmt.Errorf("document envelope must have \"type\":\"doc\"")
	}

	// Block types that carry attrs.id (custom teaching-unit blocks).
	// Standard StarterKit structural blocks (paragraph, heading, etc.)
	// don't need IDs — they're just rich text.
	blockTypesRequiringID := map[string]bool{
		"prose":        true,
		"problem-ref":  true,
		"teacher-note": true,
		"code-snippet": true,
		"media-embed":  true,
	}

	// Walk top-level blocks.
	for i, rawBlock := range envelope.Content {
		var block struct {
			Type  string `json:"type"`
			Attrs struct {
				ID string `json:"id"`
			} `json:"attrs"`
		}
		if err := json.Unmarshal(rawBlock, &block); err != nil {
			return fmt.Errorf("block at index %d is not valid JSON", i)
		}
		if blockTypesRequiringID[block.Type] && block.Attrs.ID == "" {
			return fmt.Errorf("block at index %d (%s) is missing attrs.id", i, block.Type)
		}
		if !knownBlockTypes[block.Type] {
			return fmt.Errorf("block at index %d has unknown type %q (allowed: %s)",
				i, block.Type, joinBlockTypes())
		}
	}
	return nil
}

func joinBlockTypes() string {
	types := make([]string, 0, len(knownBlockTypes))
	for t := range knownBlockTypes {
		types = append(types, t)
	}
	return strings.Join(types, ", ")
}

// ---------- Handlers ----------

// ListUnits — GET /api/units?scope=&scopeId=
// Returns units the caller can view. Scope + scopeId filter to a specific
// bucket; omitting both returns all units visible to the caller (this plan
// returns units from the requested bucket only for simplicity).
func (h *TeachingUnitHandler) ListUnits(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	scope := r.URL.Query().Get("scope")
	scopeIDRaw := r.URL.Query().Get("scopeId")

	if scope != "" && !validUnitScopes[scope] {
		writeError(w, http.StatusBadRequest, "scope must be platform, org, or personal")
		return
	}

	// If no scope is provided, default to returning units across all accessible
	// buckets. For plan 031 simplicity, require at least a scope.
	if scope == "" {
		// Return an empty list with a hint rather than a 400 so clients can
		// call without a scope and learn they need to specify one.
		writeJSON(w, http.StatusOK, map[string]any{"items": []store.TeachingUnit{}})
		return
	}

	units, err := h.Units.ListUnitsForScope(r.Context(), scope, scopeIDRaw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Filter by view access.
	visible := make([]store.TeachingUnit, 0, len(units))
	for _, u := range units {
		u := u // capture
		if h.canViewUnit(r.Context(), claims, &u) {
			visible = append(visible, u)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": visible})
}

// CreateUnit — POST /api/units
func (h *TeachingUnitHandler) CreateUnit(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		Scope            string   `json:"scope"`
		ScopeID          *string  `json:"scopeId"`
		Title            string   `json:"title"`
		Slug             *string  `json:"slug"`
		Summary          string   `json:"summary"`
		GradeLevel       *string  `json:"gradeLevel"`
		SubjectTags      []string `json:"subjectTags"`
		StandardsTags    []string `json:"standardsTags"`
		EstimatedMinutes *int     `json:"estimatedMinutes"`
		Status           string   `json:"status"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if !validUnitScopes[body.Scope] {
		writeError(w, http.StatusBadRequest, "scope must be platform, org, or personal")
		return
	}
	if body.Title == "" || len(body.Title) > maxUnitTitleLen {
		writeError(w, http.StatusBadRequest, "title is required (max 255 chars)")
		return
	}
	if body.Status != "" && !validUnitStatuses[body.Status] {
		writeError(w, http.StatusBadRequest, "invalid status value")
		return
	}

	// Enforce scope / scopeId consistency.
	if body.Scope == "platform" && body.ScopeID != nil && *body.ScopeID != "" {
		writeError(w, http.StatusBadRequest, "platform-scope units must not have a scopeId")
		return
	}
	if body.Scope != "platform" && (body.ScopeID == nil || *body.ScopeID == "") {
		writeError(w, http.StatusBadRequest, "scopeId is required for org and personal scope")
		return
	}

	if !h.canEditUnit(r.Context(), claims, body.Scope, body.ScopeID) {
		writeError(w, http.StatusForbidden, "not authorized for scope")
		return
	}

	unit, err := h.Units.CreateUnit(r.Context(), store.CreateTeachingUnitInput{
		Scope:            body.Scope,
		ScopeID:          body.ScopeID,
		Title:            body.Title,
		Slug:             body.Slug,
		Summary:          body.Summary,
		GradeLevel:       body.GradeLevel,
		SubjectTags:      body.SubjectTags,
		StandardsTags:    body.StandardsTags,
		EstimatedMinutes: body.EstimatedMinutes,
		Status:           body.Status,
		CreatedBy:        claims.UserID,
	})
	if err != nil {
		if isConstraintError(err) {
			writeError(w, http.StatusConflict, "constraint violation: check scope/scopeId combination")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to create unit")
		return
	}
	writeJSON(w, http.StatusCreated, unit)
}

// GetUnit — GET /api/units/{id}
func (h *TeachingUnitHandler) GetUnit(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	unit, err := h.Units.GetUnit(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canViewUnit(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found") // don't leak existence
		return
	}
	writeJSON(w, http.StatusOK, unit)
}

// UpdateUnit — PATCH /api/units/{id}
func (h *TeachingUnitHandler) UpdateUnit(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	unitID := chi.URLParam(r, "id")

	// Load the row first to determine scope for authz and to return 404 vs 403.
	unit, err := h.Units.GetUnit(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	// If not found OR caller can't even view it → 404 (don't leak existence).
	if unit == nil || !h.canViewUnit(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditUnit(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to edit")
		return
	}

	var body store.UpdateTeachingUnitInput
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Title != nil && (*body.Title == "" || len(*body.Title) > maxUnitTitleLen) {
		writeError(w, http.StatusBadRequest, "title must be 1-255 chars")
		return
	}
	if body.Status != nil && *body.Status != "" && !validUnitStatuses[*body.Status] {
		writeError(w, http.StatusBadRequest, "invalid status value")
		return
	}

	updated, err := h.Units.UpdateUnit(r.Context(), unitID, body)
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

// DeleteUnit — DELETE /api/units/{id}
func (h *TeachingUnitHandler) DeleteUnit(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	unitID := chi.URLParam(r, "id")

	unit, err := h.Units.GetUnit(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewUnit(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditUnit(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to delete")
		return
	}

	deleted, err := h.Units.DeleteUnit(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if deleted == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetDocument — GET /api/units/{id}/document
func (h *TeachingUnitHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	unitID := chi.URLParam(r, "id")

	unit, err := h.Units.GetUnit(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewUnit(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	doc, err := h.Units.GetDocument(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// GetUnitByTopic — GET /api/units/by-topic/{topicId}
// Looks up the teaching unit linked to the given topic. Returns 404 if no unit
// is linked to that topic or if the caller cannot view the found unit.
func (h *TeachingUnitHandler) GetUnitByTopic(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	topicID := chi.URLParam(r, "topicId")

	unit, err := h.Units.GetUnitByTopicID(r.Context(), topicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canViewUnit(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found") // don't leak existence
		return
	}
	writeJSON(w, http.StatusOK, unit)
}

// SaveDocument — PUT /api/units/{id}/document
// Accepts a raw JSON body that must satisfy the spec-012 block-document shape.
func (h *TeachingUnitHandler) SaveDocument(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	unitID := chi.URLParam(r, "id")

	unit, err := h.Units.GetUnit(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewUnit(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditUnit(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to edit document")
		return
	}

	// Read the raw body so we can validate it before forwarding to the store.
	var raw json.RawMessage
	if !decodeJSON(w, r, &raw) {
		return
	}
	if err := validateBlockDocument(raw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	doc, err := h.Units.SaveDocument(r.Context(), unitID, raw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// TransitionUnit — POST /api/units/{id}/transition
// Transitions the unit to a new status per the spec-012 state machine.
// Body: { "status": "<target>" }
// Maps ErrInvalidTransition → 409 Conflict.
func (h *TeachingUnitHandler) TransitionUnit(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	unitID := chi.URLParam(r, "id")

	unit, err := h.Units.GetUnit(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewUnit(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditUnit(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to transition")
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if !validUnitStatuses[body.Status] {
		writeError(w, http.StatusBadRequest, "invalid target status")
		return
	}

	updated, err := h.Units.SetUnitStatus(r.Context(), unitID, body.Status, claims.UserID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "Not found")
		return
	case errors.Is(err, store.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "invalid status transition")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// ListRevisions — GET /api/units/{id}/revisions
func (h *TeachingUnitHandler) ListRevisions(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	unitID := chi.URLParam(r, "id")

	unit, err := h.Units.GetUnit(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewUnit(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	revisions, err := h.Units.ListRevisions(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": revisions})
}

// GetRevision — GET /api/units/{id}/revisions/{revisionId}
func (h *TeachingUnitHandler) GetRevision(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	unitID := chi.URLParam(r, "id")

	// Verify access to the parent unit first.
	unit, err := h.Units.GetUnit(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil || !h.canViewUnit(r.Context(), claims, unit) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	revisionID := chi.URLParam(r, "revisionId")
	rev, err := h.Units.GetRevision(r.Context(), revisionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if rev == nil || rev.UnitID != unitID {
		writeError(w, http.StatusNotFound, "Revision not found")
		return
	}
	writeJSON(w, http.StatusOK, rev)
}
