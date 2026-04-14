package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// MeHandler serves user-specific aggregation endpoints.
type MeHandler struct {
	Orgs    *store.OrgStore
	Courses *store.CourseStore
	Classes *store.ClassStore
}

// OptionalAuthRoutes registers /api/me endpoints that work with or without auth.
// These use optional auth — the middleware validates the token if present but
// doesn't reject requests without one.
func (h *MeHandler) OptionalAuthRoutes(r chi.Router) {
	r.Get("/api/me/roles", h.GetRoles)
	r.Get("/api/me/portal-access", h.GetPortalAccess)
}

// Routes registers /api/me endpoints that require auth.
func (h *MeHandler) Routes(r chi.Router) {
	r.Get("/api/me/memberships", h.GetMemberships)
}

type userRole struct {
	Role    string `json:"role"`
	OrgID   string `json:"orgId,omitempty"`
	OrgName string `json:"orgName,omitempty"`
}

var rolePriority = []string{"admin", "org_admin", "teacher", "student", "parent"}

func portalPath(role string) string {
	paths := map[string]string{
		"admin": "/admin", "org_admin": "/org", "teacher": "/teacher",
		"student": "/student", "parent": "/parent",
	}
	if p, ok := paths[role]; ok {
		return p
	}
	return "/onboarding"
}

func buildRoles(isPlatformAdmin bool, memberships []store.UserMembershipWithOrg) []userRole {
	var roles []userRole
	if isPlatformAdmin {
		roles = append(roles, userRole{Role: "admin"})
	}
	seen := make(map[string]bool)
	for _, m := range memberships {
		if m.Status != "active" || m.OrgStatus != "active" {
			continue
		}
		key := m.Role + ":" + m.OrgID
		if seen[key] {
			continue
		}
		seen[key] = true
		roles = append(roles, userRole{Role: m.Role, OrgID: m.OrgID, OrgName: m.OrgName})
	}
	return roles
}

func primaryRole(roles []userRole) *userRole {
	if len(roles) == 0 {
		return nil
	}
	for _, p := range rolePriority {
		for i := range roles {
			if roles[i].Role == p {
				return &roles[i]
			}
		}
	}
	return &roles[0]
}

// GetRoles handles GET /api/me/roles
// Returns: { authenticated, primaryPortalPath, roles[] }
func (h *MeHandler) GetRoles(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated":     false,
			"primaryPortalPath": "/login",
		})
		return
	}

	memberships, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	roles := buildRoles(claims.IsPlatformAdmin, memberships)
	primary := primaryRole(roles)
	path := "/onboarding"
	if primary != nil {
		path = portalPath(primary.Role)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":     true,
		"primaryPortalPath": path,
		"roles":             roles,
	})
}

// GetPortalAccess handles GET /api/me/portal-access
// Returns: { authorized, userName, roles[], currentRole }
func (h *MeHandler) GetPortalAccess(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusOK, map[string]any{"authorized": false})
		return
	}

	memberships, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	roles := buildRoles(claims.IsPlatformAdmin, memberships)
	primary := primaryRole(roles)

	var currentRole *userRole
	if primary != nil {
		currentRole = primary
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"authorized":  len(roles) > 0,
		"userName":    claims.Name,
		"roles":       roles,
		"currentRole": currentRole,
	})
}

// GetMemberships handles GET /api/me/memberships
func (h *MeHandler) GetMemberships(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	memberships, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, memberships)
}
