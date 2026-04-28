package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type TopicHandler struct {
	Topics        *store.TopicStore
	Courses       *store.CourseStore
	Orgs          *store.OrgStore
	TeachingUnits *store.TeachingUnitStore // Plan 044: backs LinkUnit.
}

// Routes registers topic routes nested under /api/courses/{courseId}/topics
func (h *TopicHandler) Routes(r chi.Router) {
	r.Route("/api/courses/{courseId}/topics", func(r chi.Router) {
		r.Use(ValidateUUIDParam("courseId"))
		r.Post("/", h.CreateTopic)
		r.Get("/", h.ListTopics)
		r.Patch("/reorder", h.ReorderTopics)
		r.Route("/{topicId}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("topicId"))
			r.Get("/", h.GetTopic)
			r.Patch("/", h.UpdateTopic)
			r.Delete("/", h.DeleteTopic)
			// Plan 044 phase 2: link a teaching_unit to this topic.
			r.Post("/link-unit", h.LinkUnit)
		})
	})
}

// CreateTopic handles POST /api/courses/{courseId}/topics
func (h *TopicHandler) CreateTopic(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "courseId")

	// Verify course exists and check ownership
	course, err := h.Courses.GetCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if course == nil {
		writeError(w, http.StatusNotFound, "Course not found")
		return
	}
	if !claims.IsPlatformAdmin && course.CreatedBy != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the course creator can add topics")
		return
	}

	// Plan 044 phase 3: lessonContent and starterCode are no longer
	// accepted on topic create. Use POST /api/courses/{cid}/topics/
	// {tid}/link-unit to attach a teaching_unit instead.
	var body struct {
		Title         string  `json:"title"`
		Description   string  `json:"description"`
		LessonContent *string `json:"lessonContent,omitempty"`
		StarterCode   *string `json:"starterCode,omitempty"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.Title == "" || len(body.Title) > 255 {
		writeError(w, http.StatusBadRequest, "title is required (max 255 chars)")
		return
	}
	if body.LessonContent != nil || body.StarterCode != nil {
		writeError(w, http.StatusBadRequest,
			"lessonContent and starterCode are no longer accepted; link a teaching unit via POST /api/courses/{courseId}/topics/{topicId}/link-unit")
		return
	}

	topic, err := h.Topics.CreateTopic(r.Context(), store.CreateTopicInput{
		CourseID:    courseID,
		Title:       body.Title,
		Description: body.Description,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create topic")
		return
	}

	writeJSON(w, http.StatusCreated, topic)
}

// ListTopics handles GET /api/courses/{courseId}/topics
// Access: creator, platform admin, or a user with a class membership in the course.
func (h *TopicHandler) ListTopics(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "courseId")

	if !claims.IsPlatformAdmin {
		course, err := h.Courses.GetCourse(r.Context(), courseID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if course == nil {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		if course.CreatedBy != claims.UserID {
			hasAccess, err := h.Courses.UserHasAccessToCourse(r.Context(), courseID, claims.UserID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Database error")
				return
			}
			if !hasAccess {
				writeError(w, http.StatusForbidden, "Access denied")
				return
			}
		}
	}

	topics, err := h.Topics.ListTopicsByCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, topics)
}

// GetTopic handles GET /api/courses/{courseId}/topics/{topicId}
func (h *TopicHandler) GetTopic(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	topic, err := h.Topics.GetTopic(r.Context(), chi.URLParam(r, "topicId"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if topic == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, topic)
}

// UpdateTopic handles PATCH /api/courses/{courseId}/topics/{topicId}
func (h *TopicHandler) UpdateTopic(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "courseId")

	course, err := h.Courses.GetCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if course == nil {
		writeError(w, http.StatusNotFound, "Course not found")
		return
	}
	if !claims.IsPlatformAdmin && course.CreatedBy != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the course creator can update topics")
		return
	}

	// Plan 044 phase 3: lessonContent and starterCode are no longer
	// accepted on update. The store struct still carries the fields
	// (migration paths use them); the handler explicitly rejects them
	// to lock the API contract before clearing the values pre-store.
	var body store.UpdateTopicInput
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.LessonContent != nil || body.StarterCode != nil {
		writeError(w, http.StatusBadRequest,
			"lessonContent and starterCode are no longer accepted; link a teaching unit via POST /api/courses/{courseId}/topics/{topicId}/link-unit")
		return
	}

	if body.Title != nil && (*body.Title == "" || len(*body.Title) > 255) {
		writeError(w, http.StatusBadRequest, "title must be 1-255 chars")
		return
	}

	updated, err := h.Topics.UpdateTopic(r.Context(), chi.URLParam(r, "topicId"), body)
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

// DeleteTopic handles DELETE /api/courses/{courseId}/topics/{topicId}
func (h *TopicHandler) DeleteTopic(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "courseId")
	course, err := h.Courses.GetCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if course == nil {
		writeError(w, http.StatusNotFound, "Course not found")
		return
	}
	if !claims.IsPlatformAdmin && course.CreatedBy != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the course creator can delete topics")
		return
	}

	deleted, err := h.Topics.DeleteTopic(r.Context(), chi.URLParam(r, "topicId"))
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

// ReorderTopics handles PATCH /api/courses/{courseId}/topics/reorder
func (h *TopicHandler) ReorderTopics(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "courseId")
	course, err := h.Courses.GetCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if course == nil {
		writeError(w, http.StatusNotFound, "Course not found")
		return
	}
	if !claims.IsPlatformAdmin && course.CreatedBy != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the course creator can reorder topics")
		return
	}

	var body struct {
		TopicIDs []string `json:"topicIds"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.TopicIDs) == 0 {
		writeError(w, http.StatusBadRequest, "topicIds array is required")
		return
	}

	if err := h.Topics.ReorderTopics(r.Context(), courseID, body.TopicIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to reorder topics")
		return
	}

	// Return updated list
	topics, err := h.Topics.ListTopicsByCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, topics)
}

// LinkUnit handles POST /api/courses/{courseId}/topics/{topicId}/link-unit.
//
// Plan 044 phase 2: backs the teacher topic-edit page's primitive Unit
// attach UI. Sets teaching_units.topic_id = <topicId> for the given
// unit, after verifying:
//   - the caller can edit the parent course (creator or platform admin),
//   - the caller can edit the target unit (canEditUnit-equivalent: same
//     creator, or org-scope unit in an org where the caller is teacher
//     or org_admin).
//
// 1:1 invariant: if a different unit already claims this topic, returns
// 409 with a clear message rather than letting the unique-index
// violation surface as an opaque 500.
func (h *TopicHandler) LinkUnit(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "courseId")
	topicID := chi.URLParam(r, "topicId")

	// Course-edit gate (existing pattern from CreateTopic / UpdateTopic).
	course, err := h.Courses.GetCourse(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if course == nil {
		writeError(w, http.StatusNotFound, "Course not found")
		return
	}
	if !claims.IsPlatformAdmin && course.CreatedBy != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the course creator can attach units to topics")
		return
	}

	// Verify the topic actually belongs to this course (reject mismatched
	// path traversal: /courses/A/topics/B-from-course-C/link-unit).
	topic, err := h.Topics.GetTopic(r.Context(), topicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if topic == nil || topic.CourseID != courseID {
		writeError(w, http.StatusNotFound, "Topic not found in this course")
		return
	}

	var body struct {
		UnitID string `json:"unitId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.UnitID == "" {
		writeError(w, http.StatusBadRequest, "unitId is required")
		return
	}

	if h.TeachingUnits == nil {
		writeError(w, http.StatusInternalServerError, "Teaching units store unavailable")
		return
	}
	unit, err := h.TeachingUnits.GetUnit(r.Context(), body.UnitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil {
		writeError(w, http.StatusNotFound, "Unit not found")
		return
	}

	// Unit-edit gate: same shape as canEditUnit. Personal scope owners
	// are recognized by createdBy; org scope by teacher/org_admin
	// membership in the unit's org.
	if !claims.IsPlatformAdmin && claims.ImpersonatedBy == "" {
		canEdit := false
		switch unit.Scope {
		case "personal":
			canEdit = unit.ScopeID != nil && *unit.ScopeID == claims.UserID
		case "org":
			if unit.ScopeID != nil && h.Orgs != nil {
				roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), *unit.ScopeID, claims.UserID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "Database error")
					return
				}
				for _, role := range roles {
					if role.Role == "teacher" || role.Role == "org_admin" {
						canEdit = true
						break
					}
				}
			}
		case "platform":
			// Only platform admins can edit platform-scope; the outer
			// IsPlatformAdmin check would have allowed already.
		}
		if !canEdit {
			writeError(w, http.StatusForbidden, "Cannot edit this unit")
			return
		}
	}

	updated, err := h.TeachingUnits.LinkUnitToTopic(r.Context(), body.UnitID, topicID)
	if err != nil {
		if errors.Is(err, store.ErrTopicAlreadyLinked) {
			writeError(w, http.StatusConflict, "This topic is already linked to a different unit")
			return
		}
		if errors.Is(err, store.ErrUnitNotFound) {
			writeError(w, http.StatusNotFound, "Unit not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}
