package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// AdminHandler handles admin-only endpoints.
type AdminHandler struct {
	Orgs        *store.OrgStore
	Users       *store.UserStore
	Stats       *store.StatsStore
	ParentLinks *store.ParentLinkStore
	DB          *sql.DB
}

// Routes registers admin routes (all require platform admin).
func (h *AdminHandler) Routes(r chi.Router) {
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(auth.RequireAdmin)

		r.Get("/stats", h.GetStats)
		r.Get("/users", h.ListAllUsers)
		r.Get("/orgs", h.ListAllOrgs)
		r.Patch("/orgs/{orgID}", h.UpdateOrgStatus)

		r.Post("/impersonate", h.StartImpersonate)
		r.Get("/impersonate/status", h.ImpersonateStatus)
		r.Delete("/impersonate", h.StopImpersonate)

		// Plan 064 — parent-link CRUD (platform admin only).
		r.Route("/parent-links", func(r chi.Router) {
			r.Get("/", h.ListParentLinks)
			r.Post("/", h.CreateParentLink)
			r.Route("/{linkID}", func(r chi.Router) {
				r.Use(ValidateUUIDParam("linkID"))
				r.Delete("/", h.RevokeParentLink)
			})
		})
	})
}

// GetStats handles GET /api/admin/stats
func (h *AdminHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Stats.GetAdminStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ListAllUsers handles GET /api/admin/users
func (h *AdminHandler) ListAllUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Users.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// ListAllOrgs handles GET /api/admin/orgs
func (h *AdminHandler) ListAllOrgs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	orgs, err := h.Orgs.ListOrgs(r.Context(), status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

// UpdateOrgStatus handles PATCH /api/admin/orgs/{orgID}
func (h *AdminHandler) UpdateOrgStatus(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")

	org, err := h.Orgs.GetOrg(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if org == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.Status != "active" && body.Status != "suspended" {
		writeError(w, http.StatusBadRequest, "Invalid input: status must be active or suspended")
		return
	}

	updated, err := h.Orgs.UpdateOrgStatus(r.Context(), orgID, body.Status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// StartImpersonate handles POST /api/admin/impersonate
func (h *AdminHandler) StartImpersonate(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		UserID string `json:"userId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.UserID == "" {
		writeError(w, http.StatusBadRequest, "Invalid input: userId is required")
		return
	}

	target, err := h.Users.GetUserByID(r.Context(), body.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	impData := auth.ImpersonationData{
		OriginalUserID: claims.UserID,
		TargetUserID:   target.ID,
		TargetName:     target.Name,
		TargetEmail:    target.Email,
	}
	jsonData, _ := json.Marshal(impData)

	http.SetCookie(w, &http.Cookie{
		Name:     "bridge-impersonate",
		Value:    url.QueryEscape(string(jsonData)),
		Path:     "/",
		MaxAge:   3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"impersonating": map[string]string{
			"id":    target.ID,
			"name":  target.Name,
			"email": target.Email,
		},
	})
}

// ImpersonateStatus handles GET /api/admin/impersonate/status
func (h *AdminHandler) ImpersonateStatus(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusOK, map[string]any{"impersonating": nil})
		return
	}

	if claims.ImpersonatedBy != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"impersonating": map[string]string{
				"targetUserId": claims.UserID,
				"targetName":   claims.Name,
				"targetEmail":  claims.Email,
			},
		})
		return
	}

	// Not impersonating -- check cookie directly
	cookie, err := r.Cookie("bridge-impersonate")
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusOK, map[string]any{"impersonating": nil})
		return
	}

	cookieVal := cookie.Value
	if decoded, decErr := url.QueryUnescape(cookieVal); decErr == nil {
		cookieVal = decoded
	}

	var impData auth.ImpersonationData
	if err := json.Unmarshal([]byte(cookieVal), &impData); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"impersonating": nil})
		return
	}

	if impData.OriginalUserID != claims.UserID {
		writeJSON(w, http.StatusOK, map[string]any{"impersonating": nil})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"impersonating": map[string]string{
			"targetUserId": impData.TargetUserID,
			"targetName":   impData.TargetName,
			"targetEmail":  impData.TargetEmail,
		},
	})
}

// StopImpersonate handles DELETE /api/admin/impersonate
func (h *AdminHandler) StopImpersonate(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "bridge-impersonate",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": true})
}

// --- Plan 064 — parent-link CRUD (platform admin only) ---

// ListParentLinks handles GET /api/admin/parent-links?parent={uid}|child={uid}.
// Exactly one of `parent` or `child` query params must be set.
// Returns ALL links for that user (active + revoked) ordered by
// created_at DESC.
func (h *AdminHandler) ListParentLinks(w http.ResponseWriter, r *http.Request) {
	if h.ParentLinks == nil {
		writeError(w, http.StatusInternalServerError, "ParentLinks store unavailable")
		return
	}
	parentID := r.URL.Query().Get("parent")
	childID := r.URL.Query().Get("child")
	if (parentID == "") == (childID == "") {
		writeError(w, http.StatusBadRequest, "exactly one of `parent` or `child` query params is required")
		return
	}
	var links []store.ParentLink
	var err error
	if parentID != "" {
		links, err = h.ParentLinks.ListByParent(r.Context(), parentID)
	} else {
		links, err = h.ParentLinks.ListByChild(r.Context(), childID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// CreateParentLink handles POST /api/admin/parent-links. Body:
//
//	{ "parentUserId": "...", "childUserId": "..." }
//
// Returns 201 + the created link, or 409 if an active link already
// exists for the pair, or 400 on validation errors (missing/equal
// IDs).
func (h *AdminHandler) CreateParentLink(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.ParentLinks == nil {
		writeError(w, http.StatusInternalServerError, "ParentLinks store unavailable")
		return
	}

	var body struct {
		ParentUserID string `json:"parentUserId"`
		ChildUserID  string `json:"childUserId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ParentUserID == "" || body.ChildUserID == "" {
		writeError(w, http.StatusBadRequest, "parentUserId and childUserId are required")
		return
	}
	if body.ParentUserID == body.ChildUserID {
		writeError(w, http.StatusBadRequest, "parent and child must be different users")
		return
	}

	link, err := h.ParentLinks.CreateLink(r.Context(), body.ParentUserID, body.ChildUserID, claims.UserID)
	if err != nil {
		if errors.Is(err, store.ErrParentLinkExists) {
			writeError(w, http.StatusConflict, "an active parent_link already exists for this pair")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to create parent_link")
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// RevokeParentLink handles DELETE /api/admin/parent-links/{linkID}.
// Soft-revoke: flips status to 'revoked' + sets revoked_at. The
// row stays for audit. Re-linking the same pair is supported via
// a fresh CreateLink call (partial-unique allows it).
func (h *AdminHandler) RevokeParentLink(w http.ResponseWriter, r *http.Request) {
	if h.ParentLinks == nil {
		writeError(w, http.StatusInternalServerError, "ParentLinks store unavailable")
		return
	}
	linkID := chi.URLParam(r, "linkID")
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
