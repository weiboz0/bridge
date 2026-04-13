package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/llm"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type AIHandler struct {
	Interactions *store.InteractionStore
	Sessions     *store.SessionStore
	Classrooms   *store.ClassroomStore
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

// --- System prompts by grade level ---

const baseRules = `You are a patient coding tutor helping a student learn to program.

RULES:
- Ask guiding questions to help the student think through the problem
- Point to where the issue might be (e.g., "look at line 5"), but don't give the answer
- Never provide complete function implementations or full solutions
- If the student asks you to write the code for them, redirect them to think about the approach
- Celebrate small wins and encourage persistence
- Keep responses concise (2-4 sentences unless explaining a concept)`

var gradePrompts = map[string]string{
	"K-5": baseRules + `

GRADE LEVEL: Elementary (K-5)
- Use simple vocabulary and short sentences
- Use analogies from everyday life (building blocks, recipes, treasure maps)
- Be extra encouraging and patient
- Focus on visual thinking: "What do you see happening when you run this?"
- Reference block concepts if using Blockly: "Which purple block did you use?"`,

	"6-8": baseRules + `

GRADE LEVEL: Middle School (6-8)
- Explain concepts clearly but don't over-simplify
- Reference specific line numbers: "Take a look at line 7 — what value does x have there?"
- Use analogies when helpful but can be more technical
- Encourage reading error messages: "What does the error message tell you?"
- Help build debugging habits: "What did you expect to happen vs what actually happened?"`,

	"9-12": baseRules + `

GRADE LEVEL: High School (9-12)
- Use proper technical terminology
- Reference documentation and best practices
- Discuss trade-offs when relevant: "This works, but what happens if the list is empty?"
- Encourage independent problem-solving: "How would you test that this works?"
- Help develop computational thinking and code organization skills`,
}

func getSystemPrompt(gradeLevel string) string {
	if p, ok := gradePrompts[gradeLevel]; ok {
		return p
	}
	return gradePrompts["6-8"]
}

// --- Guardrails ---

var solutionPatterns = []*regexp.Regexp{
	regexp.MustCompile("(?s)```python\\n.{200,}```"),
	regexp.MustCompile("(?s)def\\s+\\w+\\s*\\([^)]*\\):.{100,}"),
	regexp.MustCompile("(?s)class\\s+\\w+.{150,}"),
	regexp.MustCompile("(?i)here(?:'s| is) the (?:complete |full )?(?:solution|answer|code)"),
	regexp.MustCompile("(?i)just (?:copy|paste|use) this"),
}

func containsSolution(text string) bool {
	for _, p := range solutionPatterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

func filterResponse(text string) string {
	if containsSolution(text) {
		return "I was about to give you too much! Let me try again with a hint instead.\n\nWhat part of the problem are you finding most confusing? Let's break it down together."
	}
	return text
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

	// Get or create interaction
	interaction, err := h.Interactions.GetActiveInteraction(r.Context(), claims.UserID, body.SessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if interaction == nil {
		interaction, err = h.Interactions.CreateInteraction(r.Context(), store.CreateInteractionInput{
			StudentID:          claims.UserID,
			SessionID:          body.SessionID,
			EnabledByTeacherID: liveSession.TeacherID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
	}

	// Save user message
	_, err = h.Interactions.AppendMessage(r.Context(), interaction.ID, store.ChatMessage{
		Role:      "user",
		Content:   body.Message,
		Timestamp: time.Now().Format(time.RFC3339),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Build message history
	var history []store.ChatMessage
	json.Unmarshal([]byte(interaction.Messages), &history)
	history = append(history, store.ChatMessage{Role: "user", Content: body.Message})

	// Get classroom for grade level
	classroom, err := h.Classrooms.GetClassroom(r.Context(), liveSession.ClassroomID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	gradeLevel := "6-8"
	if classroom != nil {
		gradeLevel = classroom.GradeLevel
	}

	systemPrompt := getSystemPrompt(gradeLevel)
	if body.Code != "" {
		systemPrompt += "\n\nThe student's current code:\n```python\n" + body.Code + "\n```"
	}

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
	for chunk := range stream {
		if chunk.Delta != "" {
			fullResponse.WriteString(chunk.Delta)
			data, _ := json.Marshal(map[string]string{"text": chunk.Delta})
			w.Write([]byte("data: " + string(data) + "\n\n"))
			flusher.Flush()
		}
	}

	// Apply guardrails
	response := fullResponse.String()
	filtered := filterResponse(response)
	if filtered != response {
		data, _ := json.Marshal(map[string]string{"replace": filtered})
		w.Write([]byte("data: " + string(data) + "\n\n"))
		flusher.Flush()
		response = filtered
	}

	// Save assistant message
	h.Interactions.AppendMessage(r.Context(), interaction.ID, store.ChatMessage{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now().Format(time.RFC3339),
	})

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

	interactions, err := h.Interactions.ListInteractionsBySession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, interactions)
}
