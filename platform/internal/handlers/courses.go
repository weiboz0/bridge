package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type CourseHandler struct {
	Courses *store.CourseStore
	Orgs    *store.OrgStore
}

func (h *CourseHandler) Routes(r chi.Router) {
	r.Route("/api/courses", func(r chi.Router) {
		r.Post("/", h.CreateCourse)
		r.Get("/", h.ListCourses)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetCourse)
			r.Patch("/", h.UpdateCourse)
			r.Delete("/", h.DeleteCourse)
			r.Post("/clone", h.CloneCourse)
		})
	})
}

var validGradeLevels = map[string]bool{"K-5": true, "6-8": true, "9-12": true}
var validLanguages = map[string]bool{"python": true, "javascript": true, "blockly": true}

// CreateCourse handles POST /api/courses
func (h *CourseHandler) CreateCourse(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		OrgID       string `json:"orgId"`
		Title       string `json:"title"`
		Description string `json:"description"`
		GradeLevel  string `json:"gradeLevel"`
		Language    string `json:"language"`
	}
	if !decodeJSON(w, r, &body) {
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
	if !validGradeLevels[body.GradeLevel] {
		writeError(w, http.StatusBadRequest, "gradeLevel must be K-5, 6-8, or 9-12")
		return
	}
	if body.Language != "" && !validLanguages[body.Language] {
		writeError(w, http.StatusBadRequest, "language must be python, javascript, or blockly")
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

	course, err := h.Courses.CreateCourse(r.Context(), store.CreateCourseInput{
		OrgID:       body.OrgID,
		CreatedBy:   claims.UserID,
		Title:       body.Title,
		Description: body.Description,
		GradeLevel:  body.GradeLevel,
		Language:    body.Language,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create course")
		return
	}

	writeJSON(w, http.StatusCreated, course)
}

// ListCourses handles GET /api/courses?orgId=...
func (h *CourseHandler) ListCourses(w http.ResponseWriter, r *http.Request) {
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

	// Auth: any member of org, or platform admin
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

	courses, err := h.Courses.ListCoursesByOrg(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, courses)
}

// GetCourse handles GET /api/courses/{id}
func (h *CourseHandler) GetCourse(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	course, err := h.Courses.GetCourse(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if course == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, course)
}

// UpdateCourse handles PATCH /api/courses/{id}
func (h *CourseHandler) UpdateCourse(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "id")

	// Get course to check ownership
	course, err := h.Courses.GetCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if course == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	// Auth: creator or platform admin
	if !claims.IsPlatformAdmin && course.CreatedBy != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the course creator can update")
		return
	}

	var body store.UpdateCourseInput
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.Title != nil && (*body.Title == "" || len(*body.Title) > 255) {
		writeError(w, http.StatusBadRequest, "title must be 1-255 chars")
		return
	}
	if body.GradeLevel != nil && !validGradeLevels[*body.GradeLevel] {
		writeError(w, http.StatusBadRequest, "gradeLevel must be K-5, 6-8, or 9-12")
		return
	}
	if body.Language != nil && !validLanguages[*body.Language] {
		writeError(w, http.StatusBadRequest, "language must be python, javascript, or blockly")
		return
	}

	updated, err := h.Courses.UpdateCourse(r.Context(), courseID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteCourse handles DELETE /api/courses/{id}
func (h *CourseHandler) DeleteCourse(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "id")

	course, err := h.Courses.GetCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if course == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	if !claims.IsPlatformAdmin && course.CreatedBy != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the course creator can delete")
		return
	}

	deleted, err := h.Courses.DeleteCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if deleted == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

// CloneCourse handles POST /api/courses/{id}/clone
func (h *CourseHandler) CloneCourse(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "id")
	cloned, err := h.Courses.CloneCourse(r.Context(), courseID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to clone course")
		return
	}
	if cloned == nil {
		writeError(w, http.StatusNotFound, "Course not found")
		return
	}
	writeJSON(w, http.StatusCreated, cloned)
}
