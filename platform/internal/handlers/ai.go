package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/llm"
	"github.com/weiboz0/bridge/platform/internal/skills"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type AIHandler struct {
	Interactions *store.InteractionStore
	Sessions     *store.SessionStore
	Classes      *store.ClassStore
	Courses      *store.CourseStore
	Backend      llm.Backend
	Broadcaster  *events.Broadcaster
}

func (h *AIHandler) Routes(r chi.Router) {
	r.Route("/api/ai", func(r chi.Router) {
		r.Post("/chat", h.Chat)
		r.Post("/toggle", h.Toggle)
		r.Get("/interactions", h.ListInteractions)
	})
}

// Chat handles POST /api/ai/chat — streaming SSE chat with LLM
func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		SessionID string `json:"sessionId"`
		Message   string `json:"message"`
		Code      string `json:"code"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.SessionID == "" || body.Message == "" {
		writeError(w, http.StatusBadRequest, "sessionId and message are required")
		return
	}
	if len(body.Message) > 5000 {
		writeError(w, http.StatusBadRequest, "message is too long (max 5000 characters)")
		return
	}

	// Verify session exists and is active
	liveSession, err := h.Sessions.GetSession(r.Context(), body.SessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if liveSession == nil || liveSession.Status != "active" {
		writeError(w, http.StatusNotFound, "Session not found or ended")
		return
	}

	// Require AI to be enabled for this student (interaction created by Toggle)
	interaction, err := h.Interactions.GetActiveInteraction(r.Context(), claims.UserID, body.SessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if interaction == nil {
		writeError(w, http.StatusForbidden, "AI is not enabled for you in this session")
		return
	}

	// Save user message (atomic append)
	updated, err := h.Interactions.AppendMessage(r.Context(), interaction.ID, store.ChatMessage{
		Role:      "user",
		Content:   body.Message,
		Timestamp: time.Now().Format(time.RFC3339),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Build message history from the updated interaction
	var history []store.ChatMessage
	if err := json.Unmarshal([]byte(updated.Messages), &history); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to parse message history")
		return
	}

	// Get grade level from class → course chain
	gradeLevel := "6-8"
	cls, err := h.Classes.GetClass(r.Context(), liveSession.ClassID)
	if err == nil && cls != nil {
		if course, err := h.Courses.GetCourse(r.Context(), cls.CourseID); err == nil && course != nil {
			gradeLevel = course.GradeLevel
		}
	}

	lang := "python"
	if nc, err := h.Classes.GetClassroom(r.Context(), liveSession.ClassID); err == nil && nc != nil {
		lang = nc.EditorMode
	}
	systemPrompt := skills.BuildChatSystemPrompt(skills.GradeLevel(gradeLevel), body.Code, lang)

	// Build LLM messages
	llmMessages := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
	}
	for _, m := range history {
		role := llm.RoleUser
		if m.Role == "assistant" {
			role = llm.RoleAssistant
		}
		llmMessages = append(llmMessages, llm.Message{Role: role, Content: m.Content})
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Stream LLM response
	stream, err := h.Backend.StreamChat(r.Context(), llmMessages, llm.WithMaxTokens(500))
	if err != nil {
		data, _ := json.Marshal(map[string]string{"error": err.Error()})
		w.Write([]byte("data: " + string(data) + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
		return
	}

	var fullResponse strings.Builder
	guardrailTriggered := false
	for chunk := range stream {
		if chunk.IsFinal {
			break
		}
		if chunk.Delta != "" && chunk.ChunkType == "text" {
			fullResponse.WriteString(chunk.Delta)

			// Incremental guardrail check every 200 chars
			if fullResponse.Len() > 200 && skills.ContainsSolution(fullResponse.String()) {
				guardrailTriggered = true
				break
			}

			data, _ := json.Marshal(map[string]string{"text": chunk.Delta})
			w.Write([]byte("data: " + string(data) + "\n\n"))
			flusher.Flush()
		}
	}

	// Drain remaining chunks if guardrail triggered early
	if guardrailTriggered {
		go func() {
			for range stream {
			}
		}()
	}

	response := fullResponse.String()
	if guardrailTriggered || skills.ContainsSolution(response) {
		filtered := skills.FilterResponse(response)
		data, _ := json.Marshal(map[string]string{"replace": filtered})
		w.Write([]byte("data: " + string(data) + "\n\n"))
		flusher.Flush()
		response = filtered
	}

	// Save assistant message
	if _, err := h.Interactions.AppendMessage(r.Context(), interaction.ID, store.ChatMessage{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now().Format(time.RFC3339),
	}); err != nil {
		// Log but don't fail — response was already streamed
		_ = err
	}

	w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

// Toggle handles POST /api/ai/toggle — teacher enables/disables AI for a student
func (h *AIHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		SessionID string `json:"sessionId"`
		StudentID string `json:"studentId"`
		Enabled   bool   `json:"enabled"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.SessionID == "" || body.StudentID == "" {
		writeError(w, http.StatusBadRequest, "sessionId and studentId are required")
		return
	}

	// Verify caller is the session teacher
	liveSession, err := h.Sessions.GetSession(r.Context(), body.SessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if liveSession == nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}
	if !claims.IsPlatformAdmin && liveSession.TeacherID != claims.UserID {
		writeError(w, http.StatusForbidden, "Only the session teacher can toggle AI")
		return
	}

	if body.Enabled {
		existing, err := h.Interactions.GetActiveInteraction(r.Context(), body.StudentID, body.SessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if existing == nil {
			_, err = h.Interactions.CreateInteraction(r.Context(), store.CreateInteractionInput{
				StudentID:          body.StudentID,
				SessionID:          body.SessionID,
				EnabledByTeacherID: claims.UserID,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Database error")
				return
			}
		}
	} else {
		// Disable: delete the interaction so Chat handler rejects future messages
		if err := h.Interactions.DeleteInteraction(r.Context(), body.StudentID, body.SessionID); err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
	}

	h.Broadcaster.Emit(body.SessionID, "ai_toggled", map[string]any{
		"studentId": body.StudentID,
		"enabled":   body.Enabled,
		"teacherId": claims.UserID,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"studentId": body.StudentID,
		"enabled":   body.Enabled,
	})
}

// ListInteractions handles GET /api/ai/interactions?sessionId=...
func (h *AIHandler) ListInteractions(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "sessionId query parameter is required")
		return
	}

	// Verify caller is session teacher or platform admin
	if !claims.IsPlatformAdmin {
		liveSession, err := h.Sessions.GetSession(r.Context(), sessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if liveSession == nil {
			writeError(w, http.StatusNotFound, "Session not found")
			return
		}
		if liveSession.TeacherID != claims.UserID {
			writeError(w, http.StatusForbidden, "Only the session teacher can view interactions")
			return
		}
	}

	interactions, err := h.Interactions.ListInteractionsBySession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, interactions)
}
