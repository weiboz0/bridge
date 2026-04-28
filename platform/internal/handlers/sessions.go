package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type SessionHandler struct {
	Sessions    *store.SessionStore
	Schedules   *store.ScheduleStore
	Classes     *store.ClassStore
	Courses     *store.CourseStore
	Topics      *store.TopicStore
	Orgs        *store.OrgStore
	Broadcaster *events.Broadcaster
}

type sessionListResponse struct {
	Items      []store.LiveSession `json:"items"`
	NextCursor *string             `json:"nextCursor,omitempty"`
}

func (h *SessionHandler) Routes(r chi.Router) {
	r.Route("/api/sessions", func(r chi.Router) {
		r.Get("/", h.ListSessions)
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
			r.Post("/participants", h.AddParticipant)
			r.Route("/participants/{userId}", func(r chi.Router) {
				r.Use(ValidateUUIDParam("userId"))
				r.Delete("/", h.RemoveParticipant)
			})
			r.Post("/rotate-invite", h.RotateInviteToken)
			r.Delete("/invite", h.RevokeInviteToken)
			r.Get("/teacher-page", h.GetTeacherPage)
			r.Get("/student-page", h.GetStudentPage)
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
		Title    string  `json:"title"`
		ClassID  *string `json:"classId"`
		Settings string  `json:"settings"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	if body.ClassID == nil {
		if !claims.IsPlatformAdmin {
			ok, err := h.isTeacherOrOrgAdmin(r, claims.UserID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Database error")
				return
			}
			if !ok {
				writeError(w, http.StatusForbidden, "Must be teacher or platform admin")
				return
			}
		}
	} else {
		if _, ok := h.authorizeSessionCreateForClass(w, r, *body.ClassID, claims); !ok {
			return
		}
	}

	session, err := h.Sessions.CreateSession(r.Context(), store.CreateSessionInput{
		ClassID:   body.ClassID,
		TeacherID: claims.UserID,
		Title:     body.Title,
		Settings:  body.Settings,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func parseSessionListFilterFromQuery(r *http.Request) (store.ListSessionsFilter, error) {
	q := r.URL.Query()
	f := store.ListSessionsFilter{
		TeacherID: q.Get("teacherId"),
		Status:    q.Get("status"),
	}
	if classID := q.Get("classId"); classID != "" {
		f.ClassID = &classID
	}
	if limit := q.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil {
			return f, errors.New("limit must be an integer")
		}
		f.Limit = n
	}
	if cursor := q.Get("cursor"); cursor != "" {
		startedAt, id, err := decodeCursor(cursor)
		if err != nil {
			return f, err
		}
		f.CursorStartedAt = startedAt
		f.CursorID = id
	}
	return f, nil
}

// ListSessions handles GET /api/sessions?teacherId=&classId=&status=&limit=&cursor=
func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	filter, err := parseSessionListFilterFromQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid query: "+err.Error())
		return
	}

	if filter.TeacherID == "" {
		filter.TeacherID = claims.UserID
	} else if !claims.IsPlatformAdmin && filter.TeacherID != claims.UserID {
		writeError(w, http.StatusForbidden, "Cannot list another teacher's sessions")
		return
	}

	sessions, hasMore, err := h.Sessions.ListSessions(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	var nextCursor *string
	if hasMore && len(sessions) > 0 {
		last := sessions[len(sessions)-1]
		cursor := encodeCursor(last.StartedAt, last.ID)
		nextCursor = &cursor
	}

	writeJSON(w, http.StatusOK, sessionListResponse{
		Items:      sessions,
		NextCursor: nextCursor,
	})
}

// canAccessClass reports whether the caller may read class-scoped session
// data. Admin equivalence, class membership, or org_admin in the class's
// owning org. Returns 404 (not 403) when not authorized so callers don't
// leak class existence — same shape as ClassHandler.CanAccessClass.
//
// Plan 043 Phase 1 P0 (Codex correction #1): pre-043, ListByClass /
// GetActiveByClass / GetSessionTopics let any authenticated user
// enumerate sessions/topics by class ID. This helper backs the gate.
func (h *SessionHandler) canAccessClass(r *http.Request, classID string, claims *auth.Claims) (int, string) {
	if claims.IsPlatformAdmin || claims.ImpersonatedBy != "" {
		return 0, ""
	}
	if h.Classes == nil {
		return http.StatusInternalServerError, "Class store unavailable"
	}
	class, err := h.Classes.GetClass(r.Context(), classID)
	if err != nil {
		return http.StatusInternalServerError, "Database error"
	}
	if class == nil {
		return http.StatusNotFound, "Not found"
	}
	members, err := h.Classes.ListClassMembers(r.Context(), classID)
	if err != nil {
		return http.StatusInternalServerError, "Database error"
	}
	for _, m := range members {
		if m.UserID == claims.UserID {
			return 0, ""
		}
	}
	if h.Orgs != nil {
		roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), class.OrgID, claims.UserID)
		if err != nil {
			return http.StatusInternalServerError, "Database error"
		}
		for _, role := range roles {
			if role.Role == "org_admin" {
				return 0, ""
			}
		}
	}
	return http.StatusNotFound, "Not found"
}

// ListByClass handles GET /api/sessions/by-class/{classId}
func (h *SessionHandler) ListByClass(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	classID := chi.URLParam(r, "classId")
	if status, msg := h.canAccessClass(r, classID, claims); status != 0 {
		writeError(w, status, msg)
		return
	}
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
	if status, msg := h.canAccessClass(r, classID, claims); status != 0 {
		writeError(w, status, msg)
		return
	}
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

	// Platform admin bypasses access check.
	if !claims.IsPlatformAdmin {
		allowed, _, err := h.Sessions.CanAccessSession(r.Context(), sessionID, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if !allowed {
			// Return 404 to not leak existence.
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
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

// canJoinSession reports whether the caller may join this session.
//
// Plan 043 Phase 1 P0: pre-043, JoinSession added any authenticated caller
// as a participant. Now access requires one of:
//
//   - admin equivalence (IsPlatformAdmin || ImpersonatedBy != "")
//   - Class membership in the session's owning class
//   - A pre-existing session_participants row with status `invited` or
//     `present` (pre-invited via AddParticipant or token). Status `left`
//     does NOT grant re-entry — a kicked or left student must be re-invited
//   - For class-less sessions, only the session teacher (or admin) may join
//
// Returns (0, "") if authorized, or an http status + message to write
// otherwise. The same logic is reused by GetStudentPage so the page can
// load before the join POST runs.
func (h *SessionHandler) canJoinSession(r *http.Request, session *store.LiveSession, claims *auth.Claims) (int, string) {
	if claims.IsPlatformAdmin || claims.ImpersonatedBy != "" {
		return 0, ""
	}

	if session.ClassID == nil {
		if session.TeacherID == claims.UserID {
			return 0, ""
		}
		return http.StatusForbidden, "Not authorized"
	}

	if h.Classes != nil {
		members, err := h.Classes.ListClassMembers(r.Context(), *session.ClassID)
		if err != nil {
			return http.StatusInternalServerError, "Database error"
		}
		for _, m := range members {
			if m.UserID == claims.UserID {
				return 0, ""
			}
		}
	}

	if existing, err := h.Sessions.GetSessionParticipant(r.Context(), session.ID, claims.UserID); err != nil {
		return http.StatusInternalServerError, "Database error"
	} else if existing != nil && (existing.Status == "invited" || existing.Status == "present") {
		return 0, ""
	}

	return http.StatusForbidden, "Not authorized"
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

	if status, msg := h.canJoinSession(r, session, claims); status != 0 {
		writeError(w, status, msg)
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

	sessionID := chi.URLParam(r, "id")
	if _, ok := h.isSessionAuthority(w, r, sessionID, claims); !ok {
		return
	}

	participants, err := h.Sessions.GetSessionParticipants(r.Context(), sessionID)
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

	sessionID := chi.URLParam(r, "id")

	// Plan 043 Phase 1 P0: gate by class membership. Resolve the class
	// via the session, then defer to canAccessClass. Class-less sessions
	// (rare) only the teacher or admin may inspect.
	session, err := h.Sessions.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if session.ClassID != nil {
		if status, msg := h.canAccessClass(r, *session.ClassID, claims); status != 0 {
			writeError(w, status, msg)
			return
		}
	} else if !claims.IsPlatformAdmin && claims.ImpersonatedBy == "" && session.TeacherID != claims.UserID {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	topics, err := h.Sessions.GetSessionTopics(r.Context(), sessionID)
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

func (h *SessionHandler) isInstructor(r *http.Request, classID, userID string) (bool, error) {
	if h.Classes == nil {
		return false, errors.New("class store unavailable")
	}
	members, err := h.Classes.ListClassMembers(r.Context(), classID)
	if err != nil {
		return false, err
	}
	for _, m := range members {
		if m.UserID == userID && m.Role == "instructor" {
			return true, nil
		}
	}
	return false, nil
}

func (h *SessionHandler) isTeacherOrOrgAdmin(r *http.Request, userID string) (bool, error) {
	if h.Orgs == nil {
		return false, errors.New("org store unavailable")
	}
	memberships, err := h.Orgs.GetUserMemberships(r.Context(), userID)
	if err != nil {
		return false, err
	}
	for _, m := range memberships {
		if m.Status != "active" || m.OrgStatus != "active" {
			continue
		}
		if m.Role == "teacher" || m.Role == "org_admin" {
			return true, nil
		}
	}
	return false, nil
}

func (h *SessionHandler) authorizeSessionCreateForClass(w http.ResponseWriter, r *http.Request, classID string, claims *auth.Claims) (*store.Class, bool) {
	if h.Classes == nil {
		writeError(w, http.StatusInternalServerError, "Class store unavailable")
		return nil, false
	}

	class, err := h.Classes.GetClass(r.Context(), classID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil, false
	}
	if class == nil {
		writeError(w, http.StatusNotFound, "Class not found")
		return nil, false
	}
	if claims.IsPlatformAdmin {
		return class, true
	}

	isInstructor, err := h.isInstructor(r, classID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil, false
	}
	if isInstructor {
		return class, true
	}

	if h.Orgs == nil {
		writeError(w, http.StatusInternalServerError, "Org store unavailable")
		return nil, false
	}
	roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), class.OrgID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil, false
	}
	for _, role := range roles {
		if role.Role == "org_admin" {
			return class, true
		}
	}

	writeError(w, http.StatusForbidden, "Must be instructor or org admin for this class")
	return nil, false
}

// isSessionAuthority checks whether the caller has "authority" over a session:
// platform admin, session teacher, class instructor/ta, or org admin (if session is class-bound).
// It fetches the session and returns it; on failure it writes the error response.
func (h *SessionHandler) isSessionAuthority(w http.ResponseWriter, r *http.Request, sessionID string, claims *auth.Claims) (*store.LiveSession, bool) {
	session, err := h.Sessions.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil, false
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return nil, false
	}

	if claims.IsPlatformAdmin || session.TeacherID == claims.UserID {
		return session, true
	}

	// If the session belongs to a class, check class instructor/ta and org admin.
	if session.ClassID != nil && h.Classes != nil {
		// Check class instructor/ta
		members, err := h.Classes.ListClassMembers(r.Context(), *session.ClassID)
		if err == nil {
			for _, m := range members {
				if m.UserID == claims.UserID && (m.Role == "instructor" || m.Role == "ta") {
					return session, true
				}
			}
		}

		// Check org admin: get the class to find the org, then check org membership.
		if h.Orgs != nil {
			class, err := h.Classes.GetClass(r.Context(), *session.ClassID)
			if err == nil && class != nil {
				roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), class.OrgID, claims.UserID)
				if err == nil {
					for _, role := range roles {
						if role.Role == "org_admin" {
							return session, true
						}
					}
				}
			}
		}
	}

	writeError(w, http.StatusForbidden, "Not authorized")
	return nil, false
}

// AddParticipant handles POST /api/sessions/{id}/participants — add a participant directly.
func (h *SessionHandler) AddParticipant(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	if _, ok := h.isSessionAuthority(w, r, sessionID, claims); !ok {
		return
	}

	var body struct {
		UserID string `json:"userId"`
		Email  string `json:"email"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.UserID == "" && body.Email == "" {
		writeError(w, http.StatusBadRequest, "userId or email is required")
		return
	}

	var participant *store.SessionParticipant
	var err error
	if body.UserID != "" {
		participant, err = h.Sessions.AddParticipant(r.Context(), sessionID, body.UserID, claims.UserID)
	} else {
		participant, err = h.Sessions.AddParticipantByEmail(r.Context(), sessionID, body.Email, claims.UserID)
	}
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "User not found")
			return
		}
		slog.Error("AddParticipant failed", "error", err, "sessionId", sessionID)
		writeError(w, http.StatusInternalServerError, "Failed to add participant")
		return
	}

	writeJSON(w, http.StatusCreated, participant)
}

// RemoveParticipant handles DELETE /api/sessions/{id}/participants/{userId} — revoke a participant.
func (h *SessionHandler) RemoveParticipant(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := chi.URLParam(r, "id")
	if _, ok := h.isSessionAuthority(w, r, sessionID, claims); !ok {
		return
	}

	targetUserID := chi.URLParam(r, "userId")
	deleted, err := h.Sessions.RemoveParticipant(r.Context(), sessionID, targetUserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "Participant not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
		Title           *string         `json:"title"`
		Settings        *string         `json:"settings"`
		InviteExpiresAt json.RawMessage `json:"inviteExpiresAt,omitempty"`
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

// teacherPagePayload is the response shape for GET /api/sessions/{id}/teacher-page.
// Includes everything the teacher's session render needs in one trip — eliminates
// the previous Next.js multi-fetch + local ID comparison pattern that drifted
// when the auth identity boundary slipped (review 002 P0).
type teacherPagePayload struct {
	Session      *store.LiveSession    `json:"session"`
	ClassID      *string               `json:"classId"`
	ReturnPath   string                `json:"returnPath"`
	EditorMode   string                `json:"editorMode"`
	CourseTopics []teacherPageTopicRef `json:"courseTopics"`
}

type teacherPageTopicRef struct {
	TopicID       string `json:"topicId"`
	Title         string `json:"title"`
	LessonContent string `json:"lessonContent"`
}

// canActAsAdmin returns true when the caller is a platform admin or an admin
// who is currently impersonating another user. The middleware rewrites claims
// during impersonation and clears IsPlatformAdmin, so we have to check
// ImpersonatedBy explicitly to preserve admin-equivalent access.
func canActAsAdmin(claims *auth.Claims) bool {
	return claims.IsPlatformAdmin || claims.ImpersonatedBy != ""
}

// GetTeacherPage handles GET /api/sessions/{id}/teacher-page.
//
// Single source of truth for "is this user allowed to see the teacher
// dashboard for this session, and what should the page render?" Replaces
// the multi-fetch + Next-side ID comparison that the old teacher session
// page used.
//
// Authorization: claims must be the session teacher, a class instructor/ta
// (when the session is class-bound), an org admin for the class's org, OR a
// platform admin (including admin-while-impersonating).
func (h *SessionHandler) GetTeacherPage(w http.ResponseWriter, r *http.Request) {
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

	authorized := canActAsAdmin(claims) || session.TeacherID == claims.UserID
	if !authorized && session.ClassID != nil && h.Classes != nil {
		members, err := h.Classes.ListClassMembers(r.Context(), *session.ClassID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		for _, m := range members {
			if m.UserID == claims.UserID && (m.Role == "instructor" || m.Role == "ta") {
				authorized = true
				break
			}
		}
		if !authorized && h.Orgs != nil {
			class, err := h.Classes.GetClass(r.Context(), *session.ClassID)
			if err == nil && class != nil {
				roles, err := h.Orgs.GetUserRolesInOrg(r.Context(), class.OrgID, claims.UserID)
				if err == nil {
					for _, role := range roles {
						if role.Role == "org_admin" {
							authorized = true
							break
						}
					}
				}
			}
		}
	}
	if !authorized {
		writeError(w, http.StatusForbidden, "Not authorized to view this session as teacher")
		return
	}

	payload := teacherPagePayload{
		Session:      session,
		ClassID:      session.ClassID,
		EditorMode:   "python",
		CourseTopics: []teacherPageTopicRef{},
	}

	if session.ClassID == nil {
		payload.ReturnPath = "/teacher"
	} else {
		payload.ReturnPath = "/teacher/classes/" + *session.ClassID

		if h.Classes != nil {
			settings, err := h.Classes.GetClassSettings(r.Context(), *session.ClassID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Database error")
				return
			}
			if settings != nil && settings.EditorMode != "" {
				payload.EditorMode = settings.EditorMode
			}

			class, err := h.Classes.GetClass(r.Context(), *session.ClassID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Database error")
				return
			}
			if class != nil && h.Topics != nil {
				topics, err := h.Topics.ListTopicsByCourse(r.Context(), class.CourseID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "Database error")
					return
				}
				refs := make([]teacherPageTopicRef, 0, len(topics))
				for _, t := range topics {
					refs = append(refs, teacherPageTopicRef{
						TopicID:       t.ID,
						Title:         t.Title,
						LessonContent: t.LessonContent,
					})
				}
				payload.CourseTopics = refs
			}
		}
	}

	writeJSON(w, http.StatusOK, payload)
}

// studentPagePayload is the response shape for GET /api/sessions/{id}/student-page.
// Same single-trip philosophy as teacher-page: one auth decision, one render payload.
type studentPagePayload struct {
	Session    *store.LiveSession `json:"session"`
	ClassID    *string            `json:"classId"`
	ReturnPath string             `json:"returnPath"`
	EditorMode string             `json:"editorMode"`
}

// GetStudentPage handles GET /api/sessions/{id}/student-page.
//
// Authorization: claims must be enrolled in the session's class as a student
// or instructor/ta, OR be the session teacher, OR be a platform admin
// (including admin-while-impersonating). For class-less sessions, only the
// teacher and admins may view.
//
// Returns 404 when the session has ended ("not live") so a stale link does
// not render an empty workspace.
func (h *SessionHandler) GetStudentPage(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusNotFound, "Session has ended")
		return
	}

	authorized := canActAsAdmin(claims) || session.TeacherID == claims.UserID
	if !authorized && session.ClassID != nil && h.Classes != nil {
		members, err := h.Classes.ListClassMembers(r.Context(), *session.ClassID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		for _, m := range members {
			if m.UserID == claims.UserID {
				authorized = true
				break
			}
		}
	}
	// Plan 043 Codex correction #2: pre-invited non-class-members must be
	// able to load the page before POSTing /join. Without this gate, a
	// teacher-invited guest gets 403 from /student-page and can't even
	// reach the join button.
	if !authorized {
		if existing, err := h.Sessions.GetSessionParticipant(r.Context(), session.ID, claims.UserID); err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		} else if existing != nil && (existing.Status == "invited" || existing.Status == "present") {
			authorized = true
		}
	}
	if !authorized {
		writeError(w, http.StatusForbidden, "Not enrolled in this session's class")
		return
	}

	payload := studentPagePayload{
		Session:    session,
		ClassID:    session.ClassID,
		EditorMode: "python",
	}

	if session.ClassID == nil {
		payload.ReturnPath = "/student"
	} else {
		payload.ReturnPath = "/student/classes/" + *session.ClassID
		if h.Classes != nil {
			settings, err := h.Classes.GetClassSettings(r.Context(), *session.ClassID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Database error")
				return
			}
			if settings != nil && settings.EditorMode != "" {
				payload.EditorMode = settings.EditorMode
			}
		}
	}

	writeJSON(w, http.StatusOK, payload)
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
