package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// TeacherHandler serves teacher-specific aggregation endpoints.
type TeacherHandler struct {
	Courses *store.CourseStore
	Classes *store.ClassStore
	Orgs    *store.OrgStore
}

func (h *TeacherHandler) Routes(r chi.Router) {
	r.Route("/api/teacher", func(r chi.Router) {
		r.Get("/dashboard", h.Dashboard)
		r.Get("/courses", h.CoursesWithOrgs)
	})
}

// Dashboard handles GET /api/teacher/dashboard
// Returns: { courses[], classes[] }
func (h *TeacherHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courses, err := h.Courses.ListCoursesByCreator(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Get orgs where user is teacher/org_admin to list their classes
	memberships, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	var allClasses []store.Class
	seen := make(map[string]bool)
	for _, m := range memberships {
		if m.Status != "active" || m.OrgStatus != "active" {
			continue
		}
		if m.Role != "teacher" && m.Role != "org_admin" {
			continue
		}
		if seen[m.OrgID] {
			continue
		}
		seen[m.OrgID] = true
		classes, err := h.Classes.ListClassesByOrg(r.Context(), m.OrgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		allClasses = append(allClasses, classes...)
	}
	if allClasses == nil {
		allClasses = []store.Class{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"courses": courses,
		"classes": allClasses,
	})
}

// CoursesWithOrgs handles GET /api/teacher/courses
// Returns: { courses[], teacherOrgs[] }
func (h *TeacherHandler) CoursesWithOrgs(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courses, err := h.Courses.ListCoursesByCreator(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	memberships, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Filter to orgs where user is teacher or org_admin
	type teacherOrg struct {
		OrgID   string `json:"orgId"`
		OrgName string `json:"orgName"`
	}
	var teacherOrgs []teacherOrg
	seen := make(map[string]bool)
	for _, m := range memberships {
		if m.Status != "active" || m.OrgStatus != "active" {
			continue
		}
		if m.Role != "teacher" && m.Role != "org_admin" {
			continue
		}
		if seen[m.OrgID] {
			continue
		}
		seen[m.OrgID] = true
		teacherOrgs = append(teacherOrgs, teacherOrg{OrgID: m.OrgID, OrgName: m.OrgName})
	}
	if teacherOrgs == nil {
		teacherOrgs = []teacherOrg{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"courses":     courses,
		"teacherOrgs": teacherOrgs,
	})
}
