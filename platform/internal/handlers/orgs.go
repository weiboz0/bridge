package handlers

import (
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// OrgHandler handles organization-related endpoints.
type OrgHandler struct {
	Orgs  *store.OrgStore
	Users *store.UserStore
}

// Routes registers org routes on the given router.
func (h *OrgHandler) Routes(r chi.Router) {
	r.Route("/api/orgs", func(r chi.Router) {
		r.Get("/", h.ListUserOrgs)
		r.Post("/", h.CreateOrg)
		r.Route("/{orgID}", func(r chi.Router) {
			r.Get("/", h.GetOrg)
			r.Patch("/", h.UpdateOrg)
			r.Route("/members", func(r chi.Router) {
				r.Get("/", h.ListMembers)
				r.Post("/", h.AddMember)
				r.Route("/{memberID}", func(r chi.Router) {
					r.Patch("/", h.UpdateMember)
					r.Delete("/", h.RemoveMember)
				})
			})
		})
	})
}

var slugRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

// CreateOrg handles POST /api/orgs
func (h *OrgHandler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		Name         string  `json:"name"`
		Slug         string  `json:"slug"`
		Type         string  `json:"type"`
		ContactEmail string  `json:"contactEmail"`
		ContactName  string  `json:"contactName"`
		Domain       *string `json:"domain,omitempty"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.Name == "" || len(body.Name) > 255 {
		writeError(w, http.StatusBadRequest, "Invalid input: name is required (max 255 chars)")
		return
	}
	if body.Slug == "" || len(body.Slug) > 255 || !slugRegex.MatchString(body.Slug) {
		writeError(w, http.StatusBadRequest, "Invalid input: slug must be lowercase alphanumeric with hyphens")
		return
	}
	validTypes := map[string]bool{"school": true, "tutoring_center": true, "bootcamp": true, "other": true}
	if !validTypes[body.Type] {
		writeError(w, http.StatusBadRequest, "Invalid input: type must be school, tutoring_center, bootcamp, or other")
		return
	}
	if body.ContactEmail == "" {
		writeError(w, http.StatusBadRequest, "Invalid input: contactEmail is required")
		return
	}
	if body.ContactName == "" || len(body.ContactName) > 255 {
		writeError(w, http.StatusBadRequest, "Invalid input: contactName is required (max 255 chars)")
		return
	}

	// Check slug uniqueness
	existing, err := h.Orgs.GetOrgBySlug(r.Context(), body.Slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "Organization with this slug already exists")
		return
	}

	org, err := h.Orgs.CreateOrg(r.Context(), store.CreateOrgInput{
		Name:         body.Name,
		Slug:         body.Slug,
		Type:         body.Type,
		ContactEmail: body.ContactEmail,
		ContactName:  body.ContactName,
		Domain:       body.Domain,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create organization")
		return
	}

	// Auto-assign creator as org_admin
	_, err = h.Orgs.AddOrgMember(r.Context(), store.AddMemberInput{
		OrgID:  org.ID,
		UserID: claims.UserID,
		Role:   "org_admin",
		Status: "active",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to assign admin role")
		return
	}

	writeJSON(w, http.StatusCreated, org)
}

// ListUserOrgs handles GET /api/orgs
func (h *OrgHandler) ListUserOrgs(w http.ResponseWriter, r *http.Request) {
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

// GetOrg handles GET /api/orgs/{orgID}
func (h *OrgHandler) GetOrg(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

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

	// Check membership (platform admins bypass)
	if !claims.IsPlatformAdmin {
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), orgID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if len(roles) == 0 {
			writeError(w, http.StatusForbidden, "Not a member")
			return
		}
	}

	writeJSON(w, http.StatusOK, org)
}

// UpdateOrg handles PATCH /api/orgs/{orgID}
func (h *OrgHandler) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	orgID := chi.URLParam(r, "orgID")

	// Only org_admin or platform admin
	if !claims.IsPlatformAdmin {
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), orgID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		isOrgAdmin := false
		for _, m := range roles {
			if m.Role == "org_admin" {
				isOrgAdmin = true
				break
			}
		}
		if !isOrgAdmin {
			writeError(w, http.StatusForbidden, "Only org admins can update")
			return
		}
	}

	var body struct {
		Name         *string `json:"name,omitempty"`
		ContactEmail *string `json:"contactEmail,omitempty"`
		ContactName  *string `json:"contactName,omitempty"`
		Domain       *string `json:"domain,omitempty"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.Name != nil && (*body.Name == "" || len(*body.Name) > 255) {
		writeError(w, http.StatusBadRequest, "Invalid input: name must be 1-255 chars")
		return
	}
	if body.ContactName != nil && (*body.ContactName == "" || len(*body.ContactName) > 255) {
		writeError(w, http.StatusBadRequest, "Invalid input: contactName must be 1-255 chars")
		return
	}

	updated, err := h.Orgs.UpdateOrg(r.Context(), orgID, store.UpdateOrgInput{
		Name:         body.Name,
		ContactEmail: body.ContactEmail,
		ContactName:  body.ContactName,
		Domain:       body.Domain,
	})
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

// ListMembers handles GET /api/orgs/{orgID}/members
func (h *OrgHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	orgID := chi.URLParam(r, "orgID")

	if !claims.IsPlatformAdmin {
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), orgID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if len(roles) == 0 {
			writeError(w, http.StatusForbidden, "Not a member")
			return
		}
	}

	members, err := h.Orgs.ListOrgMembers(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// AddMember handles POST /api/orgs/{orgID}/members
func (h *OrgHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	orgID := chi.URLParam(r, "orgID")

	if !claims.IsPlatformAdmin {
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), orgID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		isOrgAdmin := false
		for _, m := range roles {
			if m.Role == "org_admin" {
				isOrgAdmin = true
				break
			}
		}
		if !isOrgAdmin {
			writeError(w, http.StatusForbidden, "Only org admins can add members")
			return
		}
	}

	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.Email == "" {
		writeError(w, http.StatusBadRequest, "Invalid input: email is required")
		return
	}
	validRoles := map[string]bool{"org_admin": true, "teacher": true, "student": true, "parent": true}
	if !validRoles[body.Role] {
		writeError(w, http.StatusBadRequest, "Invalid input: role must be org_admin, teacher, student, or parent")
		return
	}

	user, err := h.Users.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	invitedBy := claims.UserID
	membership, err := h.Orgs.AddOrgMember(r.Context(), store.AddMemberInput{
		OrgID:     orgID,
		UserID:    user.ID,
		Role:      body.Role,
		Status:    "active",
		InvitedBy: &invitedBy,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if membership == nil {
		writeError(w, http.StatusConflict, "User already has this role in the organization")
		return
	}

	writeJSON(w, http.StatusCreated, membership)
}

// UpdateMember handles PATCH /api/orgs/{orgID}/members/{memberID}
func (h *OrgHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	orgID := chi.URLParam(r, "orgID")
	memberID := chi.URLParam(r, "memberID")

	if !claims.IsPlatformAdmin {
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), orgID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		isOrgAdmin := false
		for _, m := range roles {
			if m.Role == "org_admin" {
				isOrgAdmin = true
				break
			}
		}
		if !isOrgAdmin {
			writeError(w, http.StatusForbidden, "Only org admins can update members")
			return
		}
	}

	// Verify membership belongs to this org
	membership, err := h.Orgs.GetOrgMembership(r.Context(), memberID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if membership == nil || membership.OrgID != orgID {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	validStatuses := map[string]bool{"pending": true, "active": true, "suspended": true}
	if !validStatuses[body.Status] {
		writeError(w, http.StatusBadRequest, "Invalid status")
		return
	}

	updated, err := h.Orgs.UpdateMemberStatus(r.Context(), memberID, body.Status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// RemoveMember handles DELETE /api/orgs/{orgID}/members/{memberID}
func (h *OrgHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	orgID := chi.URLParam(r, "orgID")
	memberID := chi.URLParam(r, "memberID")

	if !claims.IsPlatformAdmin {
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), orgID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		isOrgAdmin := false
		for _, m := range roles {
			if m.Role == "org_admin" {
				isOrgAdmin = true
				break
			}
		}
		if !isOrgAdmin {
			writeError(w, http.StatusForbidden, "Only org admins can remove members")
			return
		}
	}

	membership, err := h.Orgs.GetOrgMembership(r.Context(), memberID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if membership == nil || membership.OrgID != orgID {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	removed, err := h.Orgs.RemoveOrgMember(r.Context(), memberID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if removed == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	writeJSON(w, http.StatusOK, removed)
}
