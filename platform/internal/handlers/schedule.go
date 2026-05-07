package handlers

import (
	"net/http"
	"strconv"
	"strings"
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
	Classes     *store.ClassStore
	Broadcaster *events.Broadcaster
}

func (h *ScheduleHandler) Routes(r chi.Router) {
	r.Route("/api/classes/{classId}/schedule", func(r chi.Router) {
		r.Use(ValidateUUIDParam("classId"))
		r.Post("/", h.Create)
		r.Get("/", h.List)
		r.Get("/upcoming", h.ListUpcoming)
	})
	r.Route("/api/schedule/{id}", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Patch("/", h.Update)
		r.Delete("/", h.Cancel)
		r.Post("/start", h.Start)
	})
}

// checkScheduleOwner verifies the caller owns the schedule (or is platform admin).
func (h *ScheduleHandler) checkScheduleOwner(w http.ResponseWriter, r *http.Request, scheduleID string, claims *auth.Claims) *store.ScheduledSession {
	sched, err := h.Schedules.GetSchedule(r.Context(), scheduleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil
	}
	if sched == nil {
		writeError(w, http.StatusNotFound, "Schedule not found")
		return nil
	}
	if !claims.IsPlatformAdmin && sched.TeacherID != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the schedule owner can modify this")
		return nil
	}
	return sched
}

func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "classId")

	// Auth: verify user is teacher/org_admin in the class's org, or platform admin
	// TODO(plan-075-followup): class-or-org fallback, migrate to RequireClassOrOrgAccess
	if !claims.IsPlatformAdmin {
		cls, err := h.Classes.GetClass(r.Context(), classID)
		if err != nil || cls == nil {
			writeError(w, http.StatusNotFound, "Class not found")
			return
		}
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), cls.OrgID, claims.UserID)
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

	// Validate topicIDs are UUIDs
	for _, id := range body.TopicIDs {
		if !isValidUUID(id) {
			writeError(w, http.StatusBadRequest, "Invalid UUID in topicIds: "+id)
			return
		}
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

// List returns the scheduled sessions for a class.
//
// Plan 052 PR-B: requires AccessRead — caller must be a class member,
// instructor, org_admin of the class's org, or platform admin.
// Previously checked only `claims != nil`, allowing any authenticated
// user to enumerate scheduled sessions for any class. Schedule
// subsystem returns 403 on deny per `schedule.go:85-87` precedent.
func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "classId")

	_, ok, err := RequireClassAuthority(r.Context(), h.Classes, h.Orgs, claims, classID, AccessRead)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !ok {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	schedules, err := h.Schedules.ListByClass(r.Context(), classID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, schedules)
}

// ListUpcoming returns the next-N upcoming scheduled sessions for a class.
//
// Plan 052 PR-B: requires AccessRead. Same auth rule as List.
func (h *ScheduleHandler) ListUpcoming(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "classId")

	_, ok, err := RequireClassAuthority(r.Context(), h.Classes, h.Orgs, claims, classID, AccessRead)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !ok {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

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
	if h.checkScheduleOwner(w, r, scheduleID, claims) == nil {
		return
	}

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
	if h.checkScheduleOwner(w, r, scheduleID, claims) == nil {
		return
	}

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
	if h.checkScheduleOwner(w, r, scheduleID, claims) == nil {
		return
	}

	session, err := h.Schedules.StartScheduledSession(r.Context(), scheduleID, claims.UserID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not in planned") {
			writeError(w, http.StatusNotFound, "Schedule not found or not in planned status")
		} else {
			writeError(w, http.StatusInternalServerError, "Failed to start session")
		}
		return
	}

	writeJSON(w, http.StatusCreated, session)
}
