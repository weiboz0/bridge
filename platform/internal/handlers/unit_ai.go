package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

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
	r.Route("/api/units/{id}/ai-transform", func(r chi.Router) {
		r.Use(ValidateUUIDParam("id"))
		r.Post("/", h.AITransform)
	})
}

const maxIntentLen = 2000

// draftSystemPrompt is the system prompt sent to the LLM for unit drafting.
// It describes the available block types and their JSON shapes so the LLM
// can use the provided tools to generate a structured teaching unit.
// materialTypeGuidelines maps material_type to AI writing style guidance.
var materialTypeGuidelines = map[string]string{
	"notes": `Writing style: DETAILED NOTES
- Write full paragraphs with thorough explanations.
- Include examples, analogies, and context.
- Explain concepts step by step.
- Target reading level appropriate for the grade.
- Use teacher notes liberally for pedagogy tips.`,

	"slides": `Writing style: CONCISE SLIDES
- Use short, punchy bullet points — one idea per line.
- Maximum 3-5 bullet points per prose block.
- Headlines should be clear and scannable.
- Minimize paragraphs — prefer lists and key phrases.
- Teacher notes should contain the detailed explanation the teacher will say aloud.`,

	"worksheet": `Writing style: PRACTICE WORKSHEET
- Brief introductions only — focus on exercises and problems.
- Reference problems heavily — this is practice material.
- Include worked examples before independent practice.
- Group problems by difficulty (easy → medium → hard).
- Teacher notes should contain answer keys and common mistakes.`,

	"reference": `Writing style: REFERENCE / CHEAT SHEET
- Dense, lookup-friendly format.
- Use tables, lists, and code snippets extensively.
- No narrative flow — organize by topic/concept.
- Include syntax summaries, common patterns, and quick examples.
- Minimize prose — every word should earn its place.`,
}

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

	// Enrich the system prompt with material type guidance + document context.
	enrichedSystemPrompt := draftSystemPrompt
	if guide, ok := materialTypeGuidelines[unit.MaterialType]; ok {
		enrichedSystemPrompt += "\n\n" + guide
	}
	enrichedSystemPrompt += h.buildDraftContext(r.Context(), unitID, unit)

	// Build messages.
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: enrichedSystemPrompt},
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
		slog.Warn("AI draft produced no blocks", "unitId", unitID)
		writeError(w, http.StatusBadGateway, "AI produced no usable blocks — try rephrasing your intent")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"blocks": blocks,
	})
}

// ---------------------------------------------------------------------------
// AI Transform endpoint — selection-based text transformation
// ---------------------------------------------------------------------------

// Maximum lengths for AI transform request fields.
const (
	maxSelectedTextLen = 10000
	maxContextLen      = 12000
	maxDocSummaryLen   = 5000
)

// aiTransformPrompts maps action names to system prompts that instruct the LLM
// how to transform the selected text.
var aiTransformPrompts = map[string]string{
	"rewrite": `You are a writing assistant for a K-12 teaching unit editor.
The user has selected text in their document and wants it rewritten — same meaning, different words.
Rephrase the selected text while maintaining the original meaning, tone, and level of detail.
Do NOT add new information or change the intent.
Return ONLY the rewritten text, no explanations or preamble.`,

	"polish": `You are a writing assistant for a K-12 teaching unit editor.
The user has selected text in their document and wants it polished — fix grammar, improve clarity, maintain tone.
Correct any grammatical errors, improve sentence flow, and enhance clarity without changing the meaning.
Keep the same level of formality and writing style.
Return ONLY the polished text, no explanations or preamble.`,

	"simplify": `You are a writing assistant for a K-12 teaching unit editor.
The user has selected text in their document and wants it simplified — reduce reading level for younger students.
Rewrite the text using simpler vocabulary, shorter sentences, and clearer structure.
Target a reading level appropriate for grades K-5.
Maintain the core meaning and key information.
Return ONLY the simplified text, no explanations or preamble.`,

	"expand": `You are a writing assistant for a K-12 teaching unit editor.
The user has selected text in their document and wants it expanded — elaborate with more detail.
Add supporting details, examples, or explanations to make the text more comprehensive.
Maintain the same writing style and level of formality.
Do not introduce information that contradicts the original text.
Return ONLY the expanded text, no explanations or preamble.`,

	"summarize": `You are a writing assistant for a K-12 teaching unit editor.
The user has selected text in their document and wants it summarized — condense to key points.
Distill the text down to its most essential points.
Maintain accuracy and preserve the most important information.
Return ONLY the summarized text, no explanations or preamble.`,
}

// AITransform handles POST /api/units/{id}/ai-transform.
// Accepts a selected text passage, surrounding context, and an action to
// perform (rewrite, polish, simplify, expand, summarize). Returns the
// transformed text.
func (h *UnitAIHandler) AITransform(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if h.Backend == nil {
		writeError(w, http.StatusServiceUnavailable, "AI not configured")
		return
	}

	unitID := chi.URLParam(r, "id")

	// Load unit and check edit access.
	unit, err := h.Units.GetUnit(r.Context(), unitID)
	if err != nil {
		slog.Error("AI transform: failed to fetch unit", "error", err, "unitId", unitID)
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if unit == nil {
		writeError(w, http.StatusNotFound, "Unit not found")
		return
	}
	if !h.canEditUnit(r.Context(), claims, unit.Scope, unit.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to edit this unit")
		return
	}

	// Parse and validate request body.
	var body struct {
		Action          string `json:"action"`
		SelectedText    string `json:"selectedText"`
		Context         string `json:"context"`
		DocumentSummary string `json:"documentSummary"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	systemPrompt, validAction := aiTransformPrompts[body.Action]
	if !validAction {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid action %q — must be one of: rewrite, polish, simplify, expand, summarize", body.Action))
		return
	}

	if body.SelectedText == "" {
		writeError(w, http.StatusBadRequest, "selectedText is required")
		return
	}
	if len(body.SelectedText) > maxSelectedTextLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("selectedText is too long (max %d characters)", maxSelectedTextLen))
		return
	}
	if len(body.Context) > maxContextLen {
		body.Context = body.Context[:maxContextLen]
	}
	if len(body.DocumentSummary) > maxDocSummaryLen {
		body.DocumentSummary = body.DocumentSummary[:maxDocSummaryLen]
	}

	// Build the user message with context
	userMessage := fmt.Sprintf("Selected text to %s:\n\n%s", body.Action, body.SelectedText)
	if body.Context != "" {
		userMessage += fmt.Sprintf("\n\nSurrounding context:\n%s", body.Context)
	}
	if body.DocumentSummary != "" {
		userMessage += fmt.Sprintf("\n\nDocument summary (for tone/topic awareness):\n%s", body.DocumentSummary)
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
		{Role: llm.RoleUser, Content: userMessage},
	}

	resp, err := h.Backend.Chat(r.Context(), messages, llm.WithMaxTokens(4000))
	if err != nil {
		slog.Error("AI transform LLM call failed", "error", err, "unitId", unitID, "action", body.Action)
		writeError(w, http.StatusBadGateway, "AI backend error")
		return
	}

	result := resp.Content
	if result == "" {
		slog.Warn("AI transform returned empty content", "unitId", unitID, "action", body.Action)
		writeError(w, http.StatusBadGateway, "AI returned empty result — try again")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"result": result,
	})
}

// buildDraftContext fetches the unit's current document and metadata to enrich
// the AI drafting system prompt with document-aware context.
func (h *UnitAIHandler) buildDraftContext(ctx context.Context, unitID string, unit *store.TeachingUnit) string {
	var parts []string

	// Unit metadata
	meta := fmt.Sprintf("\n\nUnit metadata:\n- Title: %s", unit.Title)
	if unit.GradeLevel != nil && *unit.GradeLevel != "" {
		meta += fmt.Sprintf("\n- Grade level: %s", *unit.GradeLevel)
	}
	if len(unit.SubjectTags) > 0 {
		meta += fmt.Sprintf("\n- Subject tags: %s", strings.Join(unit.SubjectTags, ", "))
	}
	if unit.Summary != "" {
		meta += fmt.Sprintf("\n- Summary: %s", unit.Summary)
	}
	parts = append(parts, meta)

	// Current document content (first 3000 chars)
	doc, err := h.Units.GetDocument(ctx, unitID)
	if err != nil {
		slog.Warn("buildDraftContext: failed to fetch document", "error", err, "unitId", unitID)
	} else if doc != nil && len(doc.Blocks) > 0 {
		// Extract text content from the block JSON for context.
		// Use the raw JSON truncated to 3000 chars as a rough summary.
		docStr := string(doc.Blocks)
		if len(docStr) > 3000 {
			docStr = docStr[:3000] + "..."
		}
		parts = append(parts, fmt.Sprintf("\n\nExisting document content (for context — generate content that fits with the existing material):\n%s", docStr))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "")
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
