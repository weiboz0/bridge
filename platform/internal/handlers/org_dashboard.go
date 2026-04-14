package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// OrgDashboardHandler serves org admin dashboard aggregation.
type OrgDashboardHandler struct {
	Orgs    *store.OrgStore
	Courses *store.CourseStore
	Classes *store.ClassStore
	Stats   *store.StatsStore
}

func (h *OrgDashboardHandler) Routes(r chi.Router) {
	r.Get("/api/org/dashboard", h.Dashboard)
}

// Dashboard handles GET /api/org/dashboard?orgId=...
func (h *OrgDashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	orgID := r.URL.Query().Get("orgId")
	if orgID == "" {
		// Find user's first org_admin membership
		memberships, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		for _, m := range memberships {
			if m.Role == "org_admin" && m.Status == "active" && m.OrgStatus == "active" {
				orgID = m.OrgID
				break
			}
		}
		if orgID == "" {
			writeError(w, http.StatusForbidden, "Not an org admin")
			return
		}
	}

	// Verify user is org_admin or platform admin
	if !claims.IsPlatformAdmin {
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), orgID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		isAdmin := false
		for _, m := range roles {
			if m.Role == "org_admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			writeError(w, http.StatusForbidden, "Not an org admin")
			return
		}
	}

	org, err := h.Orgs.GetOrg(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if org == nil {
		writeError(w, http.StatusNotFound, "Organization not found")
		return
	}

	stats, err := h.Stats.GetOrgDashboardStats(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"org":          org,
		"teacherCount": stats.TeacherCount,
		"studentCount": stats.StudentCount,
		"courseCount":   stats.CourseCount,
		"classCount":   stats.ClassCount,
	})
}
