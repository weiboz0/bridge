package handlers

import (
	"context"
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
			// Plan 045: detach the currently-linked teaching_unit.
			r.Delete("/link-unit", h.UnlinkUnit)
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

	// Strict decode: unknown fields are rejected with 400. Teaching
	// material is attached via POST /api/courses/{cid}/topics/{tid}/link-unit,
	// not the topic create body.
	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if !decodeJSONStrict(w, r, &body) {
		return
	}

	if body.Title == "" || len(body.Title) > 255 {
		writeError(w, http.StatusBadRequest, "title is required (max 255 chars)")
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

	// Strict decode: unknown fields are rejected with 400.
	var body store.UpdateTopicInput
	if !decodeJSONStrict(w, r, &body) {
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

// publishedPlatformUnitStatuses lists the statuses at which a
// platform-scope Unit is considered "live" library content. Plan 045
// widens the link/unlink gate so any teacher who can edit a course can
// attach a published platform Unit (previously only platform admins
// could). Drafts (`draft`, `in_review`) remain admin-only because they
// haven't passed editorial review.
var publishedPlatformUnitStatuses = map[string]bool{
	"classroom_ready": true,
	"coach_ready":     true,
	"archived":        true,
}

// canLinkUnitToCourse decides whether `claims` may attach `unit` to a
// topic in `course`. Replaces plan 044's Unit-edit-only gate with the
// plan 045 widened model:
//
//   - personal-scope: never linkable (read-side join filters them).
//   - org-scope: scope_id MUST match course.OrgID, AND caller must be
//     teacher/org_admin in that org (or a platform admin).
//   - platform-scope: linkable when the Unit is published (any course
//     teacher), or by platform admins regardless of status.
//
// Returns (allowed, internalErr). internalErr is non-nil only when an
// org-membership lookup fails — the handler should map that to 500.
func canLinkUnitToCourse(
	ctx context.Context,
	orgs *store.OrgStore,
	claims *auth.Claims,
	unit *store.TeachingUnit,
	course *store.Course,
) (bool, error) {
	if claims.IsPlatformAdmin || claims.ImpersonatedBy != "" {
		return true, nil
	}
	switch unit.Scope {
	case "personal":
		return false, nil
	case "platform":
		return publishedPlatformUnitStatuses[unit.Status], nil
	case "org":
		if unit.ScopeID == nil || *unit.ScopeID != course.OrgID {
			return false, nil
		}
		if orgs == nil {
			return false, nil
		}
		roles, err := orgs.GetUserRolesInOrg(ctx, *unit.ScopeID, claims.UserID)
		if err != nil {
			return false, err
		}
		for _, role := range roles {
			if role.Role == "teacher" || role.Role == "org_admin" {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, nil
	}
}

// LinkUnit handles POST /api/courses/{courseId}/topics/{topicId}/link-unit.
//
// Plan 044 phase 2 introduced the endpoint with a Unit-edit-only gate;
// plan 045 widened it (see canLinkUnitToCourse) so course teachers can
// attach published platform-scope library Units without admin help.
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

	allowed, err := canLinkUnitToCourse(r.Context(), h.Orgs, claims, unit, course)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "You cannot link this unit to this course")
		return
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

// UnlinkUnit handles DELETE /api/courses/{courseId}/topics/{topicId}/link-unit.
//
// Plan 045: detaches the teaching_unit currently linked to this topic
// (sets teaching_units.topic_id = NULL). Idempotent: returns 200 with
// an empty body when nothing is linked.
//
// Auth chain mirrors LinkUnit:
//  1. Claims must be present (401 otherwise).
//  2. Course must exist; caller must be creator or platform admin (403).
//  3. Topic must belong to that course (404 on mismatch — path
//     traversal guard).
//  4. Look up the currently-linked Unit; if none, 200 (idempotent).
//  5. Caller must satisfy canLinkUnitToCourse for the linked Unit
//     (same gate that authorized the link). 403 otherwise.
//  6. Clear topic_id; return 200.
func (h *TopicHandler) UnlinkUnit(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "courseId")
	topicID := chi.URLParam(r, "topicId")

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
		writeError(w, http.StatusForbidden, "Only the course creator can detach units from topics")
		return
	}

	topic, err := h.Topics.GetTopic(r.Context(), topicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if topic == nil || topic.CourseID != courseID {
		writeError(w, http.StatusNotFound, "Topic not found in this course")
		return
	}

	if h.TeachingUnits == nil {
		writeError(w, http.StatusInternalServerError, "Teaching units store unavailable")
		return
	}
	current, err := h.TeachingUnits.GetUnitByTopicID(r.Context(), topicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if current == nil {
		// Nothing linked — idempotent success.
		writeJSON(w, http.StatusOK, map[string]any{"unlinked": false})
		return
	}

	allowed, err := canLinkUnitToCourse(r.Context(), h.Orgs, claims, current, course)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "You cannot detach this unit")
		return
	}

	if err := h.TeachingUnits.UnlinkUnitFromTopic(r.Context(), current.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unlinked": true, "unitId": current.ID})
}
