package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type TopicHandler struct {
	Topics  *store.TopicStore
	Courses *store.CourseStore
	Orgs    *store.OrgStore
}

// Routes registers topic routes nested under /api/courses/{courseId}/topics
func (h *TopicHandler) Routes(r chi.Router) {
	r.Route("/api/courses/{courseId}/topics", func(r chi.Router) {
		r.Post("/", h.CreateTopic)
		r.Get("/", h.ListTopics)
		r.Patch("/reorder", h.ReorderTopics)
		r.Route("/{topicId}", func(r chi.Router) {
			r.Get("/", h.GetTopic)
			r.Patch("/", h.UpdateTopic)
			r.Delete("/", h.DeleteTopic)
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

	var body struct {
		Title         string  `json:"title"`
		Description   string  `json:"description"`
		LessonContent string  `json:"lessonContent"`
		StarterCode   *string `json:"starterCode"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.Title == "" || len(body.Title) > 255 {
		writeError(w, http.StatusBadRequest, "title is required (max 255 chars)")
		return
	}

	topic, err := h.Topics.CreateTopic(r.Context(), store.CreateTopicInput{
		CourseID:      courseID,
		Title:         body.Title,
		Description:   body.Description,
		LessonContent: body.LessonContent,
		StarterCode:   body.StarterCode,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create topic")
		return
	}

	writeJSON(w, http.StatusCreated, topic)
}

// ListTopics handles GET /api/courses/{courseId}/topics
func (h *TopicHandler) ListTopics(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := chi.URLParam(r, "courseId")
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

	var body store.UpdateTopicInput
	if !decodeJSON(w, r, &body) {
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
