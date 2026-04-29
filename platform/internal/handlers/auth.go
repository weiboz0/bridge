package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/store"
)

// AuthHandler handles auth-related endpoints.
type AuthHandler struct {
	Users *store.UserStore
}

// PublicRoutes registers auth routes (no auth middleware required for register).
func (h *AuthHandler) PublicRoutes(r chi.Router) {
	r.Post("/api/auth/register", h.Register)
}

// validIntendedRoles mirrors the signupIntentEnum in src/lib/db/schema.ts
// (`["teacher", "student"]`). Empty / nil is also accepted and stored
// as NULL — the onboarding page falls back to a role-selector menu.
var validIntendedRoles = map[string]bool{
	"teacher": true,
	"student": true,
}

// Register handles POST /api/auth/register.
//
// Plan 047 phase 3: persists `intendedRole` into users.intended_role
// so the onboarding page can route teachers vs students correctly.
// Pre-047 the field was silently dropped, breaking the email/password
// signup→onboarding handoff.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string  `json:"name"`
		Email        string  `json:"email"`
		Password     string  `json:"password"`
		IntendedRole *string `json:"intendedRole,omitempty"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.Name == "" || len(body.Name) > 255 {
		writeError(w, http.StatusBadRequest, "Invalid input: name is required (max 255 chars)")
		return
	}
	if body.Email == "" {
		writeError(w, http.StatusBadRequest, "Invalid input: email is required")
		return
	}
	if len(body.Password) < 8 || len(body.Password) > 128 {
		writeError(w, http.StatusBadRequest, "Invalid input: password must be 8-128 chars")
		return
	}
	if body.IntendedRole != nil && *body.IntendedRole != "" && !validIntendedRoles[*body.IntendedRole] {
		writeError(w, http.StatusBadRequest, "Invalid input: intendedRole must be 'teacher' or 'student'")
		return
	}
	// Normalize empty-string to nil so the column ends up NULL.
	if body.IntendedRole != nil && *body.IntendedRole == "" {
		body.IntendedRole = nil
	}

	existing, err := h.Users.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "Email already registered")
		return
	}

	user, err := h.Users.RegisterUser(r.Context(), store.RegisterInput{
		Name:         body.Name,
		Email:        body.Email,
		Password:     body.Password,
		IntendedRole: body.IntendedRole,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Registration failed")
		return
	}

	writeJSON(w, http.StatusCreated, user)
}
