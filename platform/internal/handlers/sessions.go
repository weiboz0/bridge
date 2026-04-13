package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type SessionHandler struct {
	Sessions    *store.SessionStore
	Classrooms  *store.ClassroomStore
	Broadcaster *events.Broadcaster
}

func (h *SessionHandler) Routes(r chi.Router) {
	r.Route("/api/sessions", func(r chi.Router) {
		r.Post("/", h.CreateSession)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("id"))
			r.Get("/", h.GetSession)
			r.Patch("/", h.EndSession)
			r.Post("/join", h.JoinSession)
			r.Post("/leave", h.LeaveSession)
			r.Get("/participants", h.GetParticipants)
			r.Get("/events", h.SessionEvents)
			r.Get("/help-queue", h.GetHelpQueue)
			r.Post("/help-queue", h.ToggleHelp)
			r.Post("/broadcast", h.ToggleBroadcast)
			r.Get("/topics", h.GetSessionTopics)
			r.Post("/topics", h.LinkSessionTopic)
			r.Delete("/topics", h.UnlinkSessionTopic)
		})
	})
}

// CreateSession handles POST /api/sessions
func (h *SessionHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		ClassroomID string `json:"classroomId"`
		Settings    string `json:"settings"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ClassroomID == "" {
		writeError(w, http.StatusBadRequest, "classroomId is required")
		return
	}

	// Verify user is the classroom teacher
	classroom, err := h.Classrooms.GetClassroom(r.Context(), body.ClassroomID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if classroom == nil {
		writeError(w, http.StatusNotFound, "Classroom not found")
		return
	}
	if !claims.IsPlatformAdmin && classroom.TeacherID != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the classroom teacher can start a session")
		return
	}

	session, err := h.Sessions.CreateSession(r.Context(), store.CreateSessionInput{
		ClassroomID: body.ClassroomID,
		TeacherID:   claims.UserID,
		Settings:    body.Settings,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

// GetSession handles GET /api/sessions/{id}
func (h *SessionHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	session, err := h.Sessions.GetSession(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// EndSession handles PATCH /api/sessions/{id}
func (h *SessionHandler) EndSession(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, err := h.Sessions.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !claims.IsPlatformAdmin && session.TeacherID != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the session teacher can end the session")
		return
	}

	ended, err := h.Sessions.EndSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	h.Broadcaster.Emit(sessionID, "session_ended", nil)
	writeJSON(w, http.StatusOK, ended)
}

// JoinSession handles POST /api/sessions/{id}/join
func (h *SessionHandler) JoinSession(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, err := h.Sessions.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if session.Status != "active" {
		writeError(w, http.StatusBadRequest, "Session has ended")
		return
	}

	participant, err := h.Sessions.JoinSession(r.Context(), sessionID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if participant != nil {
		h.Broadcaster.Emit(sessionID, "student_joined", map[string]string{
			"studentId": claims.UserID, "name": claims.Name,
		})
	}

	writeJSON(w, http.StatusOK, participant)
}

// LeaveSession handles POST /api/sessions/{id}/leave
func (h *SessionHandler) LeaveSession(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	participant, err := h.Sessions.LeaveSession(r.Context(), sessionID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if participant == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	h.Broadcaster.Emit(sessionID, "student_left", map[string]string{
		"studentId": claims.UserID,
	})
	writeJSON(w, http.StatusOK, participant)
}

// GetParticipants handles GET /api/sessions/{id}/participants
func (h *SessionHandler) GetParticipants(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	participants, err := h.Sessions.GetSessionParticipants(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, participants)
}

// SessionEvents handles GET /api/sessions/{id}/events (SSE)
func (h *SessionHandler) SessionEvents(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Send initial connected event
	events.WriteSSE(w, "connected", "{}")
	flusher.Flush()

	// Subscribe to events
	unsub := h.Broadcaster.Subscribe(sessionID, func(event string, data interface{}) {
		dataJSON, _ := json.Marshal(data)
		events.WriteSSE(w, event, string(dataJSON))
		flusher.Flush()
	})
	defer unsub()

	// Block until client disconnects
	<-r.Context().Done()
}

// GetHelpQueue handles GET /api/sessions/{id}/help-queue
func (h *SessionHandler) GetHelpQueue(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	participants, err := h.Sessions.GetSessionParticipants(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	var needsHelp []store.ParticipantWithUser
	for _, p := range participants {
		if p.Status == "needs_help" {
			needsHelp = append(needsHelp, p)
		}
	}
	if needsHelp == nil {
		needsHelp = []store.ParticipantWithUser{}
	}
	writeJSON(w, http.StatusOK, needsHelp)
}

// ToggleHelp handles POST /api/sessions/{id}/help-queue
func (h *SessionHandler) ToggleHelp(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")

	var body struct {
		Raised bool `json:"raised"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	status := "active"
	if body.Raised {
		status = "needs_help"
	}

	participant, err := h.Sessions.UpdateParticipantStatus(r.Context(), sessionID, claims.UserID, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if participant == nil {
		writeError(w, http.StatusNotFound, "Not a participant")
		return
	}

	eventType := "hand_lowered"
	if body.Raised {
		eventType = "hand_raised"
	}
	h.Broadcaster.Emit(sessionID, eventType, map[string]string{
		"studentId": claims.UserID, "name": claims.Name,
	})

	writeJSON(w, http.StatusOK, participant)
}

// ToggleBroadcast handles POST /api/sessions/{id}/broadcast
func (h *SessionHandler) ToggleBroadcast(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, err := h.Sessions.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !claims.IsPlatformAdmin && session.TeacherID != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the session teacher can toggle broadcast")
		return
	}

	var body struct {
		Active bool `json:"active"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	eventType := "broadcast_ended"
	if body.Active {
		eventType = "broadcast_started"
	}
	h.Broadcaster.Emit(sessionID, eventType, nil)

	writeJSON(w, http.StatusOK, map[string]bool{"active": body.Active})
}

// GetSessionTopics handles GET /api/sessions/{id}/topics
func (h *SessionHandler) GetSessionTopics(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	topics, err := h.Sessions.GetSessionTopics(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, topics)
}

// LinkSessionTopic handles POST /api/sessions/{id}/topics
func (h *SessionHandler) LinkSessionTopic(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, err := h.Sessions.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}
	if !claims.IsPlatformAdmin && session.TeacherID != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the session teacher can manage topics")
		return
	}

	var body struct {
		TopicID string `json:"topicId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.TopicID == "" {
		writeError(w, http.StatusBadRequest, "topicId is required")
		return
	}

	link, err := h.Sessions.LinkSessionTopic(r.Context(), sessionID, body.TopicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if link == nil {
		writeError(w, http.StatusConflict, "Topic already linked")
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// UnlinkSessionTopic handles DELETE /api/sessions/{id}/topics
func (h *SessionHandler) UnlinkSessionTopic(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, err := h.Sessions.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}
	if !claims.IsPlatformAdmin && session.TeacherID != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the session teacher can manage topics")
		return
	}

	var body struct {
		TopicID string `json:"topicId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.TopicID == "" {
		writeError(w, http.StatusBadRequest, "topicId is required")
		return
	}

	if err := h.Sessions.UnlinkSessionTopic(r.Context(), sessionID, body.TopicID); err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

