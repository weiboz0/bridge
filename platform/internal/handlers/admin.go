package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// AdminHandler handles admin-only endpoints.
type AdminHandler struct {
	Orgs  *store.OrgStore
	Users *store.UserStore
	DB    *sql.DB
}

// Routes registers admin routes (all require platform admin).
func (h *AdminHandler) Routes(r chi.Router) {
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(auth.RequireAdmin)

		r.Get("/orgs", h.ListAllOrgs)
		r.Patch("/orgs/{orgID}", h.UpdateOrgStatus)

		r.Post("/impersonate", h.StartImpersonate)
		r.Get("/impersonate/status", h.ImpersonateStatus)
		r.Delete("/impersonate", h.StopImpersonate)
	})
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
