package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 070 phase 1 — org-admin parent-link CRUD.
//
// Routes (mounted under /api/orgs/{orgID}/parent-links by main.go):
//   GET    /                      → ListByOrg
//   POST   /                      → CreateLink
//   DELETE /{linkID}              → RevokeLink
//
// Authorization: caller must be a platform admin OR an active
// org_admin in {orgID}. Same gate the rest of the org route group
// uses (see orgs.go::UpdateOrg for the pattern). The CRUD verbs all
// require write power, so we don't admit teachers/students even on
// the read path — listing parent emails is operator-power.

type OrgParentLinksHandler struct {
	Orgs        *store.OrgStore
	ParentLinks *store.ParentLinkStore
	Users       *store.UserStore
}

// Routes mounts the org-scoped parent-link CRUD routes. Caller is
// responsible for placing this inside the auth-required group; the
// handler does its own org_admin gate inside each method.
//
// Path parameters are validated as UUIDs by ValidateUUIDParam — same
// pattern as orgs.go. Without this, a malformed orgID or linkID
// would reach SQL comparisons and surface as a 500 instead of a
// clean 400 (Codex post-impl Q8).
func (h *OrgParentLinksHandler) Routes(r chi.Router) {
	r.Route("/api/orgs/{orgID}/parent-links", func(r chi.Router) {
		r.Use(ValidateUUIDParam("orgID"))
		r.Get("/", h.ListByOrg)
		r.Post("/", h.CreateLink)
		r.Route("/{linkID}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("linkID"))
			r.Delete("/", h.RevokeLink)
		})
	})
}

// requireOrgAdmin verifies the caller is a platform admin OR an
// active org_admin in `orgID`. Returns true on success and writes a
// 401/403 response itself on failure (caller just `return`s).
func (h *OrgParentLinksHandler) requireOrgAdmin(w http.ResponseWriter, r *http.Request, orgID string) (*auth.Claims, bool) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return nil, false
	}
	if claims.IsPlatformAdmin {
		return claims, true
	}
	roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), orgID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil, false
	}
	for _, m := range roles {
		if m.Role == "org_admin" && m.Status == "active" {
			return claims, true
		}
	}
	writeError(w, http.StatusForbidden, "Only org admins can manage parent links")
	return nil, false
}

// ListByOrg handles GET /api/orgs/{orgID}/parent-links.
//
// Query params (all optional):
//
//	status  — "active" (default), "revoked", "all"
//	parent  — email-prefix (case-insensitive) for the parent
//	child   — exact child user_id
//	class   — class_id; restrict to children with active student
//	          membership in that class
//
// Returns one row per link with parent + child + (one) class
// embedded. Empty array if none match (never null).
func (h *OrgParentLinksHandler) ListByOrg(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if _, ok := h.requireOrgAdmin(w, r, orgID); !ok {
		return
	}
	q := r.URL.Query()
	rows, err := h.ParentLinks.ListByOrg(r.Context(), orgID, store.ListByOrgFilters{
		Status:      q.Get("status"),
		ParentEmail: q.Get("parent"),
		ChildUserID: q.Get("child"),
		ClassID:     q.Get("class"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// CreateLink handles POST /api/orgs/{orgID}/parent-links.
//
// Body: { "parentEmail": "...", "childUserId": "..." }
//
// Resolves parent by email (404 → "ask the parent to register
// first"); validates the child has an active student membership in
// {orgID}; creates the parent_link AND upserts an active
// org_memberships{role:'parent'} row so the parent gets `/parent`
// portal access (Decisions §3).
//
// Returns 201 + the created ParentLink, 404 if parent unknown, 403
// if child is not in the org, 409 if an active link already exists,
// 400 for missing/equal IDs.
func (h *OrgParentLinksHandler) CreateLink(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	claims, ok := h.requireOrgAdmin(w, r, orgID)
	if !ok {
		return
	}

	var body struct {
		ParentEmail string `json:"parentEmail"`
		ChildUserID string `json:"childUserId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ParentEmail == "" || body.ChildUserID == "" {
		writeError(w, http.StatusBadRequest, "parentEmail and childUserId are required")
		return
	}

	parent, err := h.Users.GetUserByEmail(r.Context(), body.ParentEmail)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if parent == nil {
		writeError(w, http.StatusNotFound, "No user with that email — ask the parent to register first")
		return
	}
	if parent.ID == body.ChildUserID {
		writeError(w, http.StatusBadRequest, "parent and child must be different users")
		return
	}

	// Cross-org guard: child must be an active student in this org.
	childInOrg, err := h.ParentLinks.ChildBelongsToOrg(r.Context(), body.ChildUserID, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !childInOrg {
		writeError(w, http.StatusForbidden, "Student is not in this organization")
		return
	}

	// Codex post-impl Q7 — link insert + membership upsert run in
	// one tx so a failure between them can't produce an orphaned
	// link with no membership (which would render as "child shown
	// but parent can't reach /parent" — confusing and hard to
	// reconcile). CreateLinkWithMembership rolls back on either
	// statement's failure.
	link, err := h.ParentLinks.CreateLinkWithMembership(
		r.Context(), parent.ID, body.ChildUserID, claims.UserID, orgID, "parent",
	)
	if err != nil {
		if errors.Is(err, store.ErrParentLinkExists) {
			writeError(w, http.StatusConflict, "An active parent link already exists for this pair")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to create parent link")
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// RevokeLink handles DELETE /api/orgs/{orgID}/parent-links/{linkID}.
//
// Validates the targeted link's child belongs to {orgID} so an
// admin can't revoke cross-org links. Soft-revoke; re-creating later
// is allowed.
//
// Decisions §3 — does NOT remove the parent's org_membership; the
// parent may have multiple children in the same org. The dashboard
// just shows "no children" if all links revoke.
func (h *OrgParentLinksHandler) RevokeLink(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if _, ok := h.requireOrgAdmin(w, r, orgID); !ok {
		return
	}
	linkID := chi.URLParam(r, "linkID")

	link, err := h.ParentLinks.GetLink(r.Context(), linkID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if link == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	childInOrg, err := h.ParentLinks.ChildBelongsToOrg(r.Context(), link.ChildUserID, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !childInOrg {
		// Don't leak existence: 404 rather than 403 so a cross-org
		// admin can't enumerate link IDs.
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	revoked, err := h.ParentLinks.RevokeLink(r.Context(), linkID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if revoked == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, revoked)
}
