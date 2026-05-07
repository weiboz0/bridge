package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// OrgDashboardHandler serves org admin aggregation + read-only list endpoints.
type OrgDashboardHandler struct {
	Orgs    *store.OrgStore
	Courses *store.CourseStore
	Classes *store.ClassStore
	Stats   *store.StatsStore
}

func (h *OrgDashboardHandler) Routes(r chi.Router) {
	r.Get("/api/org/dashboard", h.Dashboard)
	r.Get("/api/org/teachers", h.ListTeachers)
	r.Get("/api/org/students", h.ListStudents)
	r.Get("/api/org/courses", h.ListCourses)
	r.Get("/api/org/classes", h.ListClasses)
}

// authorizeOrgAdmin resolves the target orgId (from the orgId query param,
// or the caller's first org_admin membership) and confirms the caller may
// act as an org admin for that org.
//
// Caller is allowed if any of:
//   - claims.IsPlatformAdmin
//   - claims.ImpersonatedBy != "" (admin-while-impersonating, per plan 039)
//   - the caller has an active `org_admin` membership in the target org.
//
// On failure, writes the appropriate HTTP error and returns ok=false.
// Used by the dashboard + every read-only org list endpoint.
func (h *OrgDashboardHandler) authorizeOrgAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return "", false
	}

	orgID := r.URL.Query().Get("orgId")
	if orgID == "" {
		memberships, err := h.Orgs.GetUserMemberships(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return "", false
		}
		for _, m := range memberships {
			if m.Role == "org_admin" && m.Status == "active" && m.OrgStatus == "active" {
				orgID = m.OrgID
				break
			}
		}
		if orgID == "" {
			writeError(w, http.StatusForbidden, "Not an org admin")
			return "", false
		}
	}

	ok, err := RequireOrgAuthority(r.Context(), h.Orgs, claims, orgID, OrgAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return "", false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "Not an org admin")
		return "", false
	}
	return orgID, true
}

// Dashboard handles GET /api/org/dashboard?orgId=...
func (h *OrgDashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.authorizeOrgAdmin(w, r)
	if !ok {
		return
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
		"courseCount":  stats.CourseCount,
		"classCount":   stats.ClassCount,
	})
}

type orgMemberSummary struct {
	MembershipID string `json:"membershipId"`
	UserID       string `json:"userId"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	JoinedAt     string `json:"joinedAt"`
}

// ListTeachers handles GET /api/org/teachers?orgId=...
func (h *OrgDashboardHandler) ListTeachers(w http.ResponseWriter, r *http.Request) {
	h.listMembersByRole(w, r, "teacher")
}

// ListStudents handles GET /api/org/students?orgId=...
func (h *OrgDashboardHandler) ListStudents(w http.ResponseWriter, r *http.Request) {
	h.listMembersByRole(w, r, "student")
}

func (h *OrgDashboardHandler) listMembersByRole(w http.ResponseWriter, r *http.Request, role string) {
	orgID, ok := h.authorizeOrgAdmin(w, r)
	if !ok {
		return
	}

	members, err := h.Orgs.ListOrgMembers(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	out := make([]orgMemberSummary, 0)
	for _, m := range members {
		if m.Role == role {
			out = append(out, orgMemberSummary{
				MembershipID: m.ID,
				UserID:       m.UserID,
				Name:         m.Name,
				Email:        m.Email,
				Role:         m.Role,
				Status:       m.Status,
				JoinedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type orgCourseSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	GradeLevel string `json:"gradeLevel"`
	Language   string `json:"language"`
	CreatedAt  string `json:"createdAt"`
}

// ListCourses handles GET /api/org/courses?orgId=...
func (h *OrgDashboardHandler) ListCourses(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.authorizeOrgAdmin(w, r)
	if !ok {
		return
	}

	courses, err := h.Courses.ListCoursesByOrg(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	out := make([]orgCourseSummary, 0, len(courses))
	for _, c := range courses {
		out = append(out, orgCourseSummary{
			ID:         c.ID,
			Title:      c.Title,
			GradeLevel: c.GradeLevel,
			Language:   c.Language,
			CreatedAt:  c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// ListClasses handles GET /api/org/classes?orgId=...
func (h *OrgDashboardHandler) ListClasses(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.authorizeOrgAdmin(w, r)
	if !ok {
		return
	}

	rows, err := h.Classes.ListClassesByOrgWithCounts(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
