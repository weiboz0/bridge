package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/llm"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// UnitAIHandler serves AI-assisted unit drafting endpoints.
type UnitAIHandler struct {
	Units   *store.TeachingUnitStore
	Orgs    *store.OrgStore
	Backend llm.Backend // may be nil if LLM not configured
}

func (h *UnitAIHandler) Routes(r chi.Router) {
	r.Route("/api/units/{id}/draft-with-ai", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Post("/", h.DraftWithAI)
	})
}

const maxIntentLen = 2000

// draftSystemPrompt is the system prompt sent to the LLM for unit drafting.
// It describes the available block types and their JSON shapes so the LLM
// can use the provided tools to generate a structured teaching unit.
const draftSystemPrompt = `You are a curriculum designer. Given a teacher's intent, generate a teaching unit composed of structured blocks.

Use the provided tools to build the unit. Call them in sequence to compose the lesson.

Available tools:
- add_prose: Add a prose paragraph to the unit.
- add_teacher_note: Add a private teacher note (visible only to teachers).
- add_code_snippet: Add a code example with syntax highlighting.
- add_problem_ref: Reference an existing problem by ID.

Guidelines:
- Start with a prose introduction explaining the lesson objectives.
- Use teacher notes for pedagogy tips, timing suggestions, and common misconceptions.
- Include code snippets to demonstrate concepts.
- Use problem references when the teacher mentions specific problem IDs.
- Structure the lesson logically: intro → concept explanation → examples → practice.
- Keep prose concise and grade-appropriate.
`

// draftToolDefs returns the tool specifications for the AI drafting endpoint.
func draftToolDefs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{
			Name:        "add_prose",
			Description: "Add a prose paragraph to the teaching unit.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "The paragraph text content (plain text or simple markdown).",
					},
				},
				"required": []string{"text"},
			},
		},
		{
			Name:        "add_teacher_note",
			Description: "Add a teacher-only note with pedagogy tips, timing, or facilitation guidance.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "The teacher note content.",
					},
				},
				"required": []string{"text"},
			},
		},
		{
			Name:        "add_code_snippet",
			Description: "Add a code snippet with syntax highlighting.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"language": map[string]any{
						"type":        "string",
						"description": "The programming language (e.g. python, javascript, java).",
					},
					"code": map[string]any{
						"type":        "string",
						"description": "The code content.",
					},
				},
				"required": []string{"language", "code"},
			},
		},
		{
			Name:        "add_problem_ref",
			Description: "Reference an existing problem by its UUID.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"problemId": map[string]any{
						"type":        "string",
						"description": "The UUID of the problem to reference.",
					},
					"visibility": map[string]any{
						"type":        "string",
						"description": "When to show the problem: always, after_attempt, or teacher_only.",
						"enum":        []string{"always", "after_attempt", "teacher_only"},
					},
				},
				"required": []string{"problemId", "visibility"},
			},
		},
	}
}

// generateBlockID produces a short random ID suitable for block attrs.id.
// Uses 10 random bytes → 20 hex chars, similar to nanoid output length.
func generateBlockID() string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		// Fallback: should never happen with crypto/rand.
		return "fallback-id"
	}
	return hex.EncodeToString(b)
}

// ToolCallToBlock converts a single LLM tool call into a Tiptap-compatible
// block JSON structure. Returns the block map and true on success, or nil and
// false if the tool name is unrecognized or arguments are invalid.
func ToolCallToBlock(tc llm.ToolCall) (map[string]any, bool) {
	switch tc.Name {
	case "add_prose":
		text, _ := tc.Arguments["text"].(string)
		if text == "" {
			return nil, false
		}
		return map[string]any{
			"type": "prose",
			"attrs": map[string]any{
				"id": generateBlockID(),
			},
			"content": []map[string]any{
				{
					"type": "paragraph",
					"content": []map[string]any{
						{"type": "text", "text": text},
					},
				},
			},
		}, true

	case "add_teacher_note":
		text, _ := tc.Arguments["text"].(string)
		if text == "" {
			return nil, false
		}
		return map[string]any{
			"type": "teacher-note",
			"attrs": map[string]any{
				"id": generateBlockID(),
			},
			"content": []map[string]any{
				{
					"type": "paragraph",
					"content": []map[string]any{
						{"type": "text", "text": text},
					},
				},
			},
		}, true

	case "add_code_snippet":
		language, _ := tc.Arguments["language"].(string)
		code, _ := tc.Arguments["code"].(string)
		if language == "" {
			language = "python"
		}
		return map[string]any{
			"type": "code-snippet",
			"attrs": map[string]any{
				"id":       generateBlockID(),
				"language": language,
				"code":     code,
			},
		}, true

	case "add_problem_ref":
		problemId, _ := tc.Arguments["problemId"].(string)
		visibility, _ := tc.Arguments["visibility"].(string)
		if problemId == "" {
			return nil, false
		}
		if visibility == "" {
			visibility = "always"
		}
		return map[string]any{
			"type": "problem-ref",
			"attrs": map[string]any{
				"id":              generateBlockID(),
				"problemId":       problemId,
				"pinnedRevision":  nil,
				"visibility":      visibility,
				"overrideStarter": nil,
			},
		}, true

	default:
		return nil, false
	}
}

// ToolCallsToBlocks converts a slice of tool calls into block JSON structures,
// skipping unrecognized or invalid tool calls.
func ToolCallsToBlocks(calls []llm.ToolCall) []map[string]any {
	blocks := make([]map[string]any, 0, len(calls))
	for _, tc := range calls {
		if block, ok := ToolCallToBlock(tc); ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// DraftWithAI handles POST /api/units/{id}/draft-with-ai.
// Accepts an intent string, calls the LLM with tool definitions, and returns
// the generated blocks for the frontend to insert into the editor.
func (h *UnitAIHandler) DraftWithAI(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if h.Backend == nil {
		writeError(w, http.StatusServiceUnavailable, "AI drafting not configured")
		return
	}

	unitID := chi.URLParam(r, "id")

	// Load unit and check edit access.
	unit, err := h.Units.GetUnit(r.Context(), unitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if !h.canEditUnit(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to edit this unit")
		return
	}

	// Parse request body.
	var body struct {
		Intent string `json:"intent"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Intent == "" {
		writeError(w, http.StatusBadRequest, "intent is required")
		return
	}
	if len(body.Intent) > maxIntentLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("intent is too long (max %d characters)", maxIntentLen))
		return
	}

	// Build messages.
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: draftSystemPrompt},
		{Role: llm.RoleUser, Content: body.Intent},
	}

	tools := draftToolDefs()

	// Call LLM with tools.
	var blocks []map[string]any

	if h.Backend.SupportsTools() {
		resp, err := h.Backend.ChatWithTools(r.Context(), messages, tools, llm.WithMaxTokens(4000))
		if err != nil {
			slog.Error("AI draft LLM call failed", "error", err, "unitId", unitID)
			writeError(w, http.StatusBadGateway, "AI backend error")
			return
		}

		blocks = ToolCallsToBlocks(resp.ToolCalls)

		// If the model returned no tool calls but returned text content,
		// try to parse the text as JSON blocks as a fallback.
		if len(blocks) == 0 && resp.Content != "" {
			blocks = tryParseJSONBlocks(resp.Content)
		}
	} else {
		// Fallback for backends that don't support tool use:
		// Ask the LLM to return JSON directly.
		fallbackPrompt := draftSystemPrompt + `

IMPORTANT: Since you cannot use tools, return your response as a JSON array of blocks.
Each block should be an object with a "tool" field (one of: add_prose, add_teacher_note, add_code_snippet, add_problem_ref) and an "args" field with the tool arguments.

Example:
[
  {"tool": "add_prose", "args": {"text": "Welcome to today's lesson on while loops."}},
  {"tool": "add_teacher_note", "args": {"text": "This lesson takes about 45 minutes."}},
  {"tool": "add_code_snippet", "args": {"language": "python", "code": "i = 0\nwhile i < 5:\n    print(i)\n    i += 1"}}
]

Return ONLY the JSON array, no other text.`

		fallbackMessages := []llm.Message{
			{Role: llm.RoleSystem, Content: fallbackPrompt},
			{Role: llm.RoleUser, Content: body.Intent},
		}

		resp, err := h.Backend.Chat(r.Context(), fallbackMessages, llm.WithMaxTokens(4000))
		if err != nil {
			slog.Error("AI draft LLM call failed (fallback)", "error", err, "unitId", unitID)
			writeError(w, http.StatusBadGateway, "AI backend error")
			return
		}

		blocks = tryParseJSONBlocks(resp.Content)
	}

	if len(blocks) == 0 {
		// Return an empty blocks array rather than an error — the LLM may
		// have produced no usable output, and the frontend can display a
		// "no blocks generated" message.
		slog.Warn("AI draft produced no blocks", "unitId", unitID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"blocks": blocks,
	})
}

// canEditUnit delegates to the same access logic as TeachingUnitHandler.
func (h *UnitAIHandler) canEditUnit(ctx context.Context, c *auth.Claims, scope string, scopeID *string) bool {
	if c.IsPlatformAdmin {
		return true
	}
	switch scope {
	case "platform":
		return c.IsPlatformAdmin
	case "org":
		if scopeID == nil {
			return false
		}
		roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *scopeID, c.UserID)
		for _, m := range roles {
			if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
				return true
			}
		}
		return false
	case "personal":
		return scopeID != nil && *scopeID == c.UserID
	}
	return false
}

// tryParseJSONBlocks attempts to parse LLM text output as a JSON array of
// tool-call-like objects and convert them to blocks. This is the fallback path
// when the backend doesn't support native tool use.
func tryParseJSONBlocks(content string) []map[string]any {
	// Try to extract JSON array from the content. The LLM might wrap it in
	// markdown code fences or add preamble text.
	raw := extractJSONArray(content)
	if raw == "" {
		return nil
	}

	var items []struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		slog.Warn("Failed to parse AI draft JSON fallback", "error", err)
		return nil
	}

	blocks := make([]map[string]any, 0, len(items))
	for _, item := range items {
		tc := llm.ToolCall{
			Name:      item.Tool,
			Arguments: item.Args,
		}
		if block, ok := ToolCallToBlock(tc); ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// extractJSONArray finds the first JSON array in the given string.
// Handles cases where the LLM wraps the array in markdown code fences.
func extractJSONArray(s string) string {
	// First try: find [ ... ] directly.
	start := -1
	for i, c := range s {
		if c == '[' {
			start = i
			break
		}
	}
	if start == -1 {
		return ""
	}

	// Find the matching closing bracket.
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}

	return ""
}
