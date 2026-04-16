package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type ScheduleHandler struct {
	Schedules   *store.ScheduleStore
	Sessions    *store.SessionStore
	Orgs        *store.OrgStore
	Broadcaster *events.Broadcaster
}

func (h *ScheduleHandler) Routes(r chi.Router) {
	// Nested under classes
	r.Route("/api/classes/{classId}/schedule", func(r chi.Router) {
		r.Use(ValidateUUIDParam("classId"))
		r.Post("/", h.Create)
		r.Get("/", h.List)
		r.Get("/upcoming", h.ListUpcoming)
	})
	// Top-level for individual schedule operations
	r.Route("/api/schedule/{id}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Patch("/", h.Update)
		r.Delete("/", h.Cancel)
		r.Post("/start", h.Start)
	})
}

func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "classId")

	var body struct {
		Title          *string  `json:"title"`
		ScheduledStart string   `json:"scheduledStart"`
		ScheduledEnd   string   `json:"scheduledEnd"`
		TopicIDs       []string `json:"topicIds"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	start, err := time.Parse(time.RFC3339, body.ScheduledStart)
	if err != nil {
		writeError(w, http.StatusBadRequest, "scheduledStart must be RFC3339 format")
		return
	}
	end, err := time.Parse(time.RFC3339, body.ScheduledEnd)
	if err != nil {
		writeError(w, http.StatusBadRequest, "scheduledEnd must be RFC3339 format")
		return
	}
	if !end.After(start) {
		writeError(w, http.StatusBadRequest, "scheduledEnd must be after scheduledStart")
		return
	}

	sched, err := h.Schedules.CreateSchedule(r.Context(), store.CreateScheduleInput{
		ClassID:        classID,
		TeacherID:      claims.UserID,
		Title:          body.Title,
		ScheduledStart: start,
		ScheduledEnd:   end,
		TopicIDs:       body.TopicIDs,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create schedule")
		return
	}
	writeJSON(w, http.StatusCreated, sched)
}

func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "classId")
	schedules, err := h.Schedules.ListByClass(r.Context(), classID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, schedules)
}

func (h *ScheduleHandler) ListUpcoming(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "classId")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}

	schedules, err := h.Schedules.ListUpcoming(r.Context(), classID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, schedules)
}

func (h *ScheduleHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	scheduleID := chi.URLParam(r, "id")

	var body store.UpdateScheduleInput
	if !decodeJSON(w, r, &body) {
		return
	}

	updated, err := h.Schedules.UpdateSchedule(r.Context(), scheduleID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "Schedule not found or not in planned status")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *ScheduleHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	scheduleID := chi.URLParam(r, "id")
	cancelled, err := h.Schedules.CancelSchedule(r.Context(), scheduleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if cancelled == nil {
		writeError(w, http.StatusNotFound, "Schedule not found or not in planned status")
		return
	}
	writeJSON(w, http.StatusOK, cancelled)
}

func (h *ScheduleHandler) Start(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	scheduleID := chi.URLParam(r, "id")

	session, err := h.Schedules.StartScheduledSession(r.Context(), scheduleID, claims.UserID, h.Sessions)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, session)
}
