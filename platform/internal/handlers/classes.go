package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type ClassHandler struct {
	Classes *store.ClassStore
	Orgs    *store.OrgStore
	Users   *store.UserStore
}

func (h *ClassHandler) Routes(r chi.Router) {
	r.Route("/api/classes", func(r chi.Router) {
		r.Post("/", h.CreateClass)
		r.Get("/", h.ListClasses)
		r.Get("/mine", h.ListMyClasses)
		r.Post("/join", h.JoinClass)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("id"))
			r.Get("/", h.GetClass)
			r.Patch("/", h.ArchiveClass)
			r.Route("/members", func(r chi.Router) {
				r.Post("/", h.AddMember)
				r.Get("/", h.ListMembers)
				r.Route("/{memberId}", func(r chi.Router) {
					r.Use(ValidateUUIDParam("memberId"))
					r.Patch("/", h.UpdateMemberRole)
					r.Delete("/", h.RemoveMember)
				})
			})
		})
	})
}

// CreateClass handles POST /api/classes
func (h *ClassHandler) CreateClass(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		CourseID string `json:"courseId"`
		OrgID    string `json:"orgId"`
		Title    string `json:"title"`
		Term     string `json:"term"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.CourseID == "" {
		writeError(w, http.StatusBadRequest, "courseId is required")
		return
	}
	if body.OrgID == "" {
		writeError(w, http.StatusBadRequest, "orgId is required")
		return
	}
	if body.Title == "" || len(body.Title) > 255 {
		writeError(w, http.StatusBadRequest, "title is required (max 255 chars)")
		return
	}

	// Auth: teacher or org_admin in org, or platform admin
	if !claims.IsPlatformAdmin {
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), body.OrgID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		hasRole := false
		for _, m := range roles {
			if m.Role == "teacher" || m.Role == "org_admin" {
				hasRole = true
				break
			}
		}
		if !hasRole {
			writeError(w, http.StatusForbidden, "Must be teacher or org admin")
			return
		}
	}

	class, err := h.Classes.CreateClass(r.Context(), store.CreateClassInput{
		CourseID:  body.CourseID,
		OrgID:    body.OrgID,
		Title:    body.Title,
		Term:     body.Term,
		CreatedBy: claims.UserID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create class")
		return
	}

	writeJSON(w, http.StatusCreated, class)
}

// ListClasses handles GET /api/classes?orgId=...
func (h *ClassHandler) ListClasses(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	orgID := r.URL.Query().Get("orgId")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "orgId query parameter is required")
		return
	}

	if !claims.IsPlatformAdmin {
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), orgID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if len(roles) == 0 {
			writeError(w, http.StatusForbidden, "Not a member of this organization")
			return
		}
	}

	classes, err := h.Classes.ListClassesByOrg(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, classes)
}

// ListMyClasses handles GET /api/classes/mine — user's own classes with role
func (h *ClassHandler) ListMyClasses(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classes, err := h.Classes.ListClassesByUser(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, classes)
}

// CanAccessClass reports whether the caller may read this class.
//
// Plan 043 P0: pre-039, GetClass returned class metadata to any authenticated
// user. Callers now must satisfy one of:
//
//   - claims.IsPlatformAdmin || claims.ImpersonatedBy != "" (admin equivalent
//     per plan 039 correction #4 — admin-while-impersonating retains read
//     access).
//   - Class membership (any role: instructor, ta, student).
//   - Active org_admin membership in the class's owning org.
//
// Returns the resolved class on success (so callers don't double-fetch). On
// not-found OR not-authorized, returns (nil, false). Callers should write
// `404 Not found` for both — leaking class existence to non-members is the
// bug we're fixing.
func (h *ClassHandler) CanAccessClass(r *http.Request, classID string, claims *auth.Claims) (*store.Class, bool, error) {
	// Plan 052 PR-A: thin wrapper around the free `RequireClassAuthority`
	// helper so the new helper is the single source of truth for
	// class-access decisions (callable from non-ClassHandler types
	// like ScheduleHandler / AssignmentHandler in PR-B).
	return RequireClassAuthority(r.Context(), h.Classes, h.Orgs, claims, classID, AccessRead)
}

// GetClass handles GET /api/classes/{id}
func (h *ClassHandler) GetClass(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	class, ok, err := h.CanAccessClass(r, chi.URLParam(r, "id"), claims)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !ok {
		// 404 for both not-found and not-authorized — don't leak existence.
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, class)
}

// ArchiveClass handles PATCH /api/classes/{id}
//
// Plan 052: requires `mutate` authority (instructor / org_admin /
// platform admin). Previously checked only `claims != nil`, allowing
// any authenticated user to archive any class by UUID.
func (h *ClassHandler) ArchiveClass(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "id")
	_, ok, err := RequireClassAuthority(r.Context(), h.Classes, h.Orgs, claims, classID, AccessMutate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !ok {
		// 404 on deny, matching the class-subsystem precedent at
		// `classes.go:218-225`. Don't leak class existence.
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	archived, err := h.Classes.ArchiveClass(r.Context(), classID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if archived == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, archived)
}

// JoinClass handles POST /api/classes/join
func (h *ClassHandler) JoinClass(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		JoinCode string `json:"joinCode"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.JoinCode == "" {
		writeError(w, http.StatusBadRequest, "joinCode is required")
		return
	}

	result, err := h.Classes.JoinClassByCode(r.Context(), body.JoinCode, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if result == nil {
		writeError(w, http.StatusNotFound, "Invalid or inactive join code")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// AddMember handles POST /api/classes/{id}/members
//
// Plan 052: requires `mutate` authority. Previously any authenticated
// user could inject any user with any role into any class.
//
// Body parsing runs before auth so malformed payloads return 400
// without surfacing class existence to malformed callers; the auth
// gate then runs before any store mutation.
func (h *ClassHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "id")

	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if body.Role != "" && !store.IsValidClassMemberRole(body.Role) {
		writeError(w, http.StatusBadRequest, "invalid role")
		return
	}

	_, ok, err := RequireClassAuthority(r.Context(), h.Classes, h.Orgs, claims, classID, AccessMutate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "Not found")
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

	role := body.Role
	if role == "" {
		role = "student"
	}

	membership, err := h.Classes.AddClassMember(r.Context(), store.AddClassMemberInput{
		ClassID: classID, UserID: user.ID, Role: role,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if membership == nil {
		writeError(w, http.StatusConflict, "User is already a member of this class")
		return
	}
	writeJSON(w, http.StatusCreated, membership)
}

// ListMembers handles GET /api/classes/{id}/members
//
// Plan 052: requires `roster` authority (instructor / TA / org_admin
// / platform admin). The roster includes student email + name PII
// (`store/classes.go:45-52`), so plain class members do NOT pass.
// Help-queue UX uses session_participants, not class members, so
// students don't legitimately need this view.
func (h *ClassHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "id")

	_, ok, err := RequireClassAuthority(r.Context(), h.Classes, h.Orgs, claims, classID, AccessRoster)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	members, err := h.Classes.ListClassMembers(r.Context(), classID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// UpdateMemberRole handles PATCH /api/classes/{id}/members/{memberId}
//
// Plan 052: requires `mutate` authority.
func (h *ClassHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "id")
	memberID := chi.URLParam(r, "memberId")

	_, ok, err := RequireClassAuthority(r.Context(), h.Classes, h.Orgs, claims, classID, AccessMutate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	membership, err := h.Classes.GetClassMembership(r.Context(), memberID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if membership == nil || membership.ClassID != classID {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if !store.IsValidClassMemberRole(body.Role) {
		writeError(w, http.StatusBadRequest, "invalid role")
		return
	}

	updated, err := h.Classes.UpdateClassMemberRole(r.Context(), memberID, body.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// RemoveMember handles DELETE /api/classes/{id}/members/{memberId}
//
// Plan 052: requires `mutate` authority.
func (h *ClassHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "id")
	memberID := chi.URLParam(r, "memberId")

	_, ok, err := RequireClassAuthority(r.Context(), h.Classes, h.Orgs, claims, classID, AccessMutate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	membership, err := h.Classes.GetClassMembership(r.Context(), memberID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if membership == nil || membership.ClassID != classID {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	removed, err := h.Classes.RemoveClassMember(r.Context(), memberID)
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
