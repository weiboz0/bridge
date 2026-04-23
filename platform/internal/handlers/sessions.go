package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type SessionHandler struct {
	Sessions    *store.SessionStore
	Schedules   *store.ScheduleStore
	Broadcaster *events.Broadcaster
}

func (h *SessionHandler) Routes(r chi.Router) {
	r.Route("/api/sessions", func(r chi.Router) {
		r.Post("/", h.CreateSession)
		r.Get("/by-class/{classId}", h.ListByClass)
		r.Get("/active/{classId}", h.GetActiveByClass)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("id"))
			r.Get("/", h.GetSession)
			r.Patch("/", h.PatchSession)
			r.Post("/end", h.EndSession)
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
			r.Post("/rotate-invite", h.RotateInviteToken)
			r.Delete("/invite", h.RevokeInviteToken)
		})
	})

	// Token-based session join — separate route tree but still inside RequireAuth.
	r.Post("/api/s/{token}/join", h.JoinSessionByToken)
}

// CreateSession handles POST /api/sessions
func (h *SessionHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		ClassID  string `json:"classId"`
		Settings string `json:"settings"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ClassID == "" {
		writeError(w, http.StatusBadRequest, "classId is required")
		return
	}

	session, err := h.Sessions.CreateSession(r.Context(), store.CreateSessionInput{
		ClassID:   body.ClassID,
		TeacherID: claims.UserID,
		Settings:    body.Settings,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

// ListByClass handles GET /api/sessions/by-class/{classId}
func (h *SessionHandler) ListByClass(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "classId")
	sessions, err := h.Sessions.ListSessionsWithCounts(r.Context(), classID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

// GetActiveByClass handles GET /api/sessions/active/{classId}
func (h *SessionHandler) GetActiveByClass(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "classId")
	session, err := h.Sessions.GetActiveSession(r.Context(), classID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	// Returns null if no active session
	writeJSON(w, http.StatusOK, session)
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

// EndSession handles POST /api/sessions/{id}/end
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

	// Complete any linked scheduled session
	if h.Schedules != nil {
		if err := h.Schedules.CompleteScheduledSession(r.Context(), sessionID); err != nil {
			slog.Warn("failed to complete scheduled session", "sessionId", sessionID, "error", err)
		}
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
	if session.Status != "live" {
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
		if p.HelpRequestedAt != nil {
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

// isSessionOwner checks that the caller is the session teacher or a platform admin.
// Returns the session and true if authorized, or nil and false (with error written) if not.
func (h *SessionHandler) isSessionOwner(w http.ResponseWriter, r *http.Request, sessionID string, claims *auth.Claims) (*store.LiveSession, bool) {
	session, err := h.Sessions.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil, false
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return nil, false
	}
	if session.TeacherID != claims.UserID && !claims.IsPlatformAdmin {
		writeError(w, http.StatusForbidden, "Only the session teacher can perform this action")
		return nil, false
	}
	return session, true
}

// PatchSession handles PATCH /api/sessions/{id} — update mutable fields (title, settings, invite_expires_at).
func (h *SessionHandler) PatchSession(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	if _, ok := h.isSessionOwner(w, r, sessionID, claims); !ok {
		return
	}

	// Use json.RawMessage so we can distinguish "absent" from "null" for inviteExpiresAt.
	var body struct {
		Title           *string          `json:"title"`
		Settings        *string          `json:"settings"`
		InviteExpiresAt json.RawMessage  `json:"inviteExpiresAt,omitempty"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	input := store.UpdateSessionInput{
		Title:    body.Title,
		Settings: body.Settings,
	}

	// Parse inviteExpiresAt: present string → set, JSON null → clear, absent → leave unchanged.
	if len(body.InviteExpiresAt) > 0 {
		if string(body.InviteExpiresAt) == "null" {
			input.ClearInviteExpiry = true
		} else {
			var ts string
			if err := json.Unmarshal(body.InviteExpiresAt, &ts); err != nil {
				writeError(w, http.StatusBadRequest, "Invalid inviteExpiresAt: must be RFC3339 string or null")
				return
			}
			t, err := time.Parse(time.RFC3339, ts)
			if err != nil {
				writeError(w, http.StatusBadRequest, "Invalid inviteExpiresAt: must be RFC3339")
				return
			}
			input.InviteExpiresAt = &t
		}
	}

	updated, err := h.Sessions.UpdateSession(r.Context(), sessionID, input)
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

// RotateInviteToken handles POST /api/sessions/{id}/rotate-invite.
func (h *SessionHandler) RotateInviteToken(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	if _, ok := h.isSessionOwner(w, r, sessionID, claims); !ok {
		return
	}

	updated, err := h.Sessions.RotateInviteToken(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to rotate invite token")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// RevokeInviteToken handles DELETE /api/sessions/{id}/invite.
func (h *SessionHandler) RevokeInviteToken(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	if _, ok := h.isSessionOwner(w, r, sessionID, claims); !ok {
		return
	}

	_, err := h.Sessions.RevokeInviteToken(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to revoke invite token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// JoinSessionByToken handles POST /api/s/{token}/join.
func (h *SessionHandler) JoinSessionByToken(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Token is required")
		return
	}

	session, err := h.Sessions.GetSessionByToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Invalid invite link")
		return
	}

	participant, err := h.Sessions.JoinSessionByToken(r.Context(), session.ID, claims.UserID, token)
	if err != nil {
		if errors.Is(err, store.ErrTokenNotFound) {
			writeError(w, http.StatusNotFound, "Invalid invite link")
			return
		}
		if errors.Is(err, store.ErrTokenExpired) {
			writeError(w, http.StatusGone, "Invite link has expired")
			return
		}
		if errors.Is(err, store.ErrSessionEnded) {
			writeError(w, http.StatusGone, "Session has ended")
			return
		}
		slog.Error("JoinSessionByToken failed", "error", err, "sessionId", session.ID)
		writeError(w, http.StatusInternalServerError, "Failed to join session")
		return
	}

	h.Broadcaster.Emit(session.ID, "student_joined", map[string]string{
		"studentId": claims.UserID, "name": claims.Name,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId":   session.ID,
		"classId":     session.ClassID,
		"participant": participant,
	})
}
