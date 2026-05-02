package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// UnitCollectionHandler serves collection CRUD and item management endpoints.
// Access follows the same scope-based pattern as teaching units.
type UnitCollectionHandler struct {
	Collections *store.UnitCollectionStore
	Orgs        *store.OrgStore
	// Plan 052 PR-C: needed for the candidate-unit visibility check
	// in AddItem (CanViewUnit). Other endpoints don't yet require it
	// — collections currently store unit IDs without re-checking
	// visibility on read. If that changes, plumb the same check
	// through ListItems.
	TeachingUnits *store.TeachingUnitStore
}

const maxCollectionTitleLen = 255

func (h *UnitCollectionHandler) Routes(r chi.Router) {
	r.Route("/api/collections", func(r chi.Router) {
		r.Get("/", h.ListCollections)
		r.Post("/", h.CreateCollection)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("id"))
			r.Get("/", h.GetCollection)
			r.Patch("/", h.UpdateCollection)
			r.Delete("/", h.DeleteCollection)
			r.Post("/items", h.AddItem)
			r.Get("/items", h.ListItems)
			r.Route("/items/{unitId}", func(r chi.Router) {
				r.Use(ValidateUUIDParam("unitId"))
				r.Delete("/", h.RemoveItem)
			})
		})
	})
}

// canViewCollection checks whether the caller may view a collection.
// Platform → any auth. Org → teachers/admins. Personal → owner.
// Platform admin bypass applies everywhere.
func (h *UnitCollectionHandler) canViewCollection(ctx context.Context, c *auth.Claims, col *store.UnitCollection) bool {
	if c.IsPlatformAdmin {
		return true
	}
	switch col.Scope {
	case "platform":
		return true
	case "org":
		if col.ScopeID == nil {
			return false
		}
		roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *col.ScopeID, c.UserID)
		for _, m := range roles {
			if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
				return true
			}
		}
		return false
	case "personal":
		return col.ScopeID != nil && *col.ScopeID == c.UserID
	}
	return false
}

// canEditCollection checks whether the caller may create/update/delete a
// collection in the given scope.
func (h *UnitCollectionHandler) canEditCollection(ctx context.Context, c *auth.Claims, scope string, scopeID *string) bool {
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

// ListCollections — GET /api/collections?scope=
func (h *UnitCollectionHandler) ListCollections(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	scope := r.URL.Query().Get("scope")
	if scope != "" && !validUnitScopes[scope] {
		writeError(w, http.StatusBadRequest, "scope must be platform, org, or personal")
		return
	}

	// Build viewer's org list for visibility.
	var viewerOrgs []string
	if !claims.IsPlatformAdmin {
		orgs, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		for _, m := range orgs {
			if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
				viewerOrgs = append(viewerOrgs, m.OrgID)
			}
		}
	}

	collections, err := h.Collections.ListCollectionsForViewer(
		r.Context(), claims.UserID, viewerOrgs, claims.IsPlatformAdmin, scope,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": collections})
}

// CreateCollection — POST /api/collections
func (h *UnitCollectionHandler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		Scope       string  `json:"scope"`
		ScopeID     *string `json:"scopeId"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if !validUnitScopes[body.Scope] {
		writeError(w, http.StatusBadRequest, "scope must be platform, org, or personal")
		return
	}
	if body.Title == "" || len(body.Title) > maxCollectionTitleLen {
		writeError(w, http.StatusBadRequest, "title is required (max 255 chars)")
		return
	}

	// Enforce scope / scopeId consistency.
	if body.Scope == "platform" && body.ScopeID != nil && *body.ScopeID != "" {
		writeError(w, http.StatusBadRequest, "platform-scope collections must not have a scopeId")
		return
	}
	if body.Scope != "platform" && (body.ScopeID == nil || *body.ScopeID == "") {
		writeError(w, http.StatusBadRequest, "scopeId is required for org and personal scope")
		return
	}

	if !h.canEditCollection(r.Context(), claims, body.Scope, body.ScopeID) {
		writeError(w, http.StatusForbidden, "not authorized for scope")
		return
	}

	col, err := h.Collections.CreateCollection(r.Context(), store.CreateCollectionInput{
		Scope:       body.Scope,
		ScopeID:     body.ScopeID,
		Title:       body.Title,
		Description: body.Description,
		CreatedBy:   claims.UserID,
	})
	if err != nil {
		if isConstraintError(err) {
			writeError(w, http.StatusConflict, "constraint violation: check scope/scopeId combination")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to create collection")
		return
	}
	writeJSON(w, http.StatusCreated, col)
}

// GetCollection — GET /api/collections/{id}
func (h *UnitCollectionHandler) GetCollection(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	col, err := h.Collections.GetCollection(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if col == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canViewCollection(r.Context(), claims, col) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	// Fetch items alongside the collection.
	items, err := h.Collections.ListItems(r.Context(), col.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"collection": col,
		"items":      items,
	})
}

// UpdateCollection — PATCH /api/collections/{id}
func (h *UnitCollectionHandler) UpdateCollection(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	colID := chi.URLParam(r, "id")

	col, err := h.Collections.GetCollection(r.Context(), colID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if col == nil || !h.canViewCollection(r.Context(), claims, col) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditCollection(r.Context(), claims, col.Scope, col.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to edit")
		return
	}

	var body store.UpdateCollectionInput
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Title != nil && (*body.Title == "" || len(*body.Title) > maxCollectionTitleLen) {
		writeError(w, http.StatusBadRequest, "title must be 1-255 chars")
		return
	}

	updated, err := h.Collections.UpdateCollection(r.Context(), colID, body)
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

// DeleteCollection — DELETE /api/collections/{id}
func (h *UnitCollectionHandler) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	colID := chi.URLParam(r, "id")

	col, err := h.Collections.GetCollection(r.Context(), colID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if col == nil || !h.canViewCollection(r.Context(), claims, col) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditCollection(r.Context(), claims, col.Scope, col.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to delete")
		return
	}

	deleted, err := h.Collections.DeleteCollection(r.Context(), colID)
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

// AddItem — POST /api/collections/{id}/items
func (h *UnitCollectionHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	colID := chi.URLParam(r, "id")

	col, err := h.Collections.GetCollection(r.Context(), colID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if col == nil || !h.canViewCollection(r.Context(), claims, col) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditCollection(r.Context(), claims, col.Scope, col.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to add items")
		return
	}

	var body struct {
		UnitID    string `json:"unitId"`
		SortOrder *int   `json:"sortOrder"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.UnitID == "" || !isValidUUID(body.UnitID) {
		writeError(w, http.StatusBadRequest, "unitId is required and must be a valid UUID")
		return
	}

	// Plan 052 PR-C: verify the candidate unit is visible to the
	// caller before attaching it. Without this check, anyone with
	// canEditCollection permission could attach cross-org, draft, or
	// personal units they have no right to see, leaking content via
	// the collection's ListItems / projection paths.
	//
	// Returns 404 (not 403) on missing/invisible unit so we don't
	// leak unit existence by ID — same shape as canViewUnit's
	// failure mode in teaching_units.go (the not-found branch in
	// the GetUnit-related read handlers).
	if h.TeachingUnits == nil {
		writeError(w, http.StatusInternalServerError, "Teaching units store unavailable")
		return
	}
	candidate, err := h.TeachingUnits.GetUnit(r.Context(), body.UnitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if candidate == nil || !CanViewUnit(r.Context(), h.Orgs, h.TeachingUnits, claims, candidate) {
		writeError(w, http.StatusNotFound, "Unit not found")
		return
	}

	sortOrder := 0
	if body.SortOrder != nil {
		sortOrder = *body.SortOrder
	}

	item, err := h.Collections.AddItem(r.Context(), colID, body.UnitID, sortOrder)
	if err != nil {
		if isConstraintError(err) {
			writeError(w, http.StatusBadRequest, "invalid unitId — unit does not exist")
			return
		}
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// ListItems — GET /api/collections/{id}/items
func (h *UnitCollectionHandler) ListItems(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	colID := chi.URLParam(r, "id")

	col, err := h.Collections.GetCollection(r.Context(), colID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if col == nil || !h.canViewCollection(r.Context(), claims, col) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	items, err := h.Collections.ListItems(r.Context(), colID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// RemoveItem — DELETE /api/collections/{id}/items/{unitId}
func (h *UnitCollectionHandler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	colID := chi.URLParam(r, "id")
	unitID := chi.URLParam(r, "unitId")

	col, err := h.Collections.GetCollection(r.Context(), colID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if col == nil || !h.canViewCollection(r.Context(), claims, col) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditCollection(r.Context(), claims, col.Scope, col.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to remove items")
		return
	}

	removed, err := h.Collections.RemoveItem(r.Context(), colID, unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !removed {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
