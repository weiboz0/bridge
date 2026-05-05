package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// TeacherParentLinksHandler serves the read-only parent-link view
// teachers need on their class-detail page (plan 070 phase 3).
//
// Routes:
//   GET /api/teacher/classes/{classID}/parent-links
//
// Auth: caller must have `roster` access to the class — instructor
// or TA in the class, or org_admin in the class's org, or platform
// admin. Same gate ListMembers uses (classes.go::ListMembers), so a
// teacher who can already see the roster gets the parent emails too.
//
// Plain class members (students, observers, guests) do NOT pass —
// the response includes parent emails which are PII the help-queue
// UI never needs.
type TeacherParentLinksHandler struct {
	Classes     *store.ClassStore
	Orgs        *store.OrgStore
	ParentLinks *store.ParentLinkStore
}

func (h *TeacherParentLinksHandler) Routes(r chi.Router) {
	r.Route("/api/teacher/classes/{classID}/parent-links", func(r chi.Router) {
		r.Use(ValidateUUIDParam("classID"))
		r.Get("/", h.ListByClass)
	})
}

// ListByClass handles GET /api/teacher/classes/{classID}/parent-links.
func (h *TeacherParentLinksHandler) ListByClass(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	classID := chi.URLParam(r, "classID")

	_, ok, err := RequireClassAuthority(r.Context(), h.Classes, h.Orgs, claims, classID, AccessRoster)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !ok {
		// Same convention as ListMembers — 404 to avoid leaking
		// existence to non-roster callers.
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	rows, err := h.ParentLinks.ListByClass(r.Context(), classID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
