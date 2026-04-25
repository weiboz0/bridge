package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/llm"
)

// --- Auth guard tests (no LLM or DB needed) ---

func TestDraftWithAI_NoClaims(t *testing.T) {
	h := &UnitAIHandler{}
	body, _ := json.Marshal(map[string]string{"intent": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/units/00000000-0000-0000-0000-000000000001/draft-with-ai", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.DraftWithAI(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDraftWithAI_NoBackend(t *testing.T) {
	h := &UnitAIHandler{Backend: nil}
	body, _ := json.Marshal(map[string]string{"intent": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/units/00000000-0000-0000-0000-000000000001/draft-with-ai", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.DraftWithAI(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Contains(t, resp["error"], "not configured")
}

// Note: TestDraftWithAI_EmptyIntent and TestDraftWithAI_IntentTooLong require
// a unit store (GetUnit is called before body validation). These are covered
// at integration test level. The tool-call conversion tests below cover the
// core business logic without DB dependencies.

// --- Tool call to block conversion tests ---

func TestToolCallToBlock_Prose(t *testing.T) {
	tc := llm.ToolCall{
		ID:   "call-1",
		Name: "add_prose",
		Arguments: map[string]any{
			"text": "Welcome to today's lesson.",
		},
	}
	block, ok := ToolCallToBlock(tc)
	require.True(t, ok)
	assert.Equal(t, "prose", block["type"])

	attrs, ok := block["attrs"].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, attrs["id"], "block should have a generated ID")

	content, ok := block["content"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	assert.Equal(t, "paragraph", content[0]["type"])

	paraContent, ok := content[0]["content"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, paraContent, 1)
	assert.Equal(t, "text", paraContent[0]["type"])
	assert.Equal(t, "Welcome to today's lesson.", paraContent[0]["text"])
}

func TestToolCallToBlock_ProseEmptyText(t *testing.T) {
	tc := llm.ToolCall{
		Name:      "add_prose",
		Arguments: map[string]any{"text": ""},
	}
	_, ok := ToolCallToBlock(tc)
	assert.False(t, ok, "empty text should be rejected")
}

func TestToolCallToBlock_TeacherNote(t *testing.T) {
	tc := llm.ToolCall{
		Name: "add_teacher_note",
		Arguments: map[string]any{
			"text": "Spend 10 minutes on this section.",
		},
	}
	block, ok := ToolCallToBlock(tc)
	require.True(t, ok)
	assert.Equal(t, "teacher-note", block["type"])

	attrs := block["attrs"].(map[string]any)
	assert.NotEmpty(t, attrs["id"])

	content := block["content"].([]map[string]any)
	require.Len(t, content, 1)
	paraContent := content[0]["content"].([]map[string]any)
	assert.Equal(t, "Spend 10 minutes on this section.", paraContent[0]["text"])
}

func TestToolCallToBlock_TeacherNoteEmptyText(t *testing.T) {
	tc := llm.ToolCall{
		Name:      "add_teacher_note",
		Arguments: map[string]any{"text": ""},
	}
	_, ok := ToolCallToBlock(tc)
	assert.False(t, ok)
}

func TestToolCallToBlock_CodeSnippet(t *testing.T) {
	tc := llm.ToolCall{
		Name: "add_code_snippet",
		Arguments: map[string]any{
			"language": "python",
			"code":     "for i in range(5):\n    print(i)",
		},
	}
	block, ok := ToolCallToBlock(tc)
	require.True(t, ok)
	assert.Equal(t, "code-snippet", block["type"])

	attrs := block["attrs"].(map[string]any)
	assert.NotEmpty(t, attrs["id"])
	assert.Equal(t, "python", attrs["language"])
	assert.Equal(t, "for i in range(5):\n    print(i)", attrs["code"])
}

func TestToolCallToBlock_CodeSnippetDefaultLanguage(t *testing.T) {
	tc := llm.ToolCall{
		Name: "add_code_snippet",
		Arguments: map[string]any{
			"code": "console.log('hi')",
		},
	}
	block, ok := ToolCallToBlock(tc)
	require.True(t, ok)
	attrs := block["attrs"].(map[string]any)
	assert.Equal(t, "python", attrs["language"], "should default to python")
}

func TestToolCallToBlock_ProblemRef(t *testing.T) {
	tc := llm.ToolCall{
		Name: "add_problem_ref",
		Arguments: map[string]any{
			"problemId":  "00000000-0000-0000-0000-000000000099",
			"visibility": "after_attempt",
		},
	}
	block, ok := ToolCallToBlock(tc)
	require.True(t, ok)
	assert.Equal(t, "problem-ref", block["type"])

	attrs := block["attrs"].(map[string]any)
	assert.NotEmpty(t, attrs["id"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000099", attrs["problemId"])
	assert.Equal(t, "after_attempt", attrs["visibility"])
	assert.Nil(t, attrs["pinnedRevision"])
	assert.Nil(t, attrs["overrideStarter"])
}

func TestToolCallToBlock_ProblemRefEmptyId(t *testing.T) {
	tc := llm.ToolCall{
		Name:      "add_problem_ref",
		Arguments: map[string]any{"problemId": "", "visibility": "always"},
	}
	_, ok := ToolCallToBlock(tc)
	assert.False(t, ok)
}

func TestToolCallToBlock_ProblemRefDefaultVisibility(t *testing.T) {
	tc := llm.ToolCall{
		Name:      "add_problem_ref",
		Arguments: map[string]any{"problemId": "abc-123"},
	}
	block, ok := ToolCallToBlock(tc)
	require.True(t, ok)
	attrs := block["attrs"].(map[string]any)
	assert.Equal(t, "always", attrs["visibility"])
}

func TestToolCallToBlock_UnknownTool(t *testing.T) {
	tc := llm.ToolCall{
		Name:      "unknown_tool",
		Arguments: map[string]any{"foo": "bar"},
	}
	_, ok := ToolCallToBlock(tc)
	assert.False(t, ok, "unknown tool should be rejected")
}

func TestToolCallsToBlocks_Mixed(t *testing.T) {
	calls := []llm.ToolCall{
		{Name: "add_prose", Arguments: map[string]any{"text": "Intro"}},
		{Name: "unknown", Arguments: map[string]any{}},                                                 // should be skipped
		{Name: "add_code_snippet", Arguments: map[string]any{"language": "js", "code": "let x = 1;"}}, // valid
		{Name: "add_prose", Arguments: map[string]any{"text": ""}},                                     // should be skipped (empty)
		{Name: "add_teacher_note", Arguments: map[string]any{"text": "Check understanding"}},
	}
	blocks := ToolCallsToBlocks(calls)
	require.Len(t, blocks, 3)
	assert.Equal(t, "prose", blocks[0]["type"])
	assert.Equal(t, "code-snippet", blocks[1]["type"])
	assert.Equal(t, "teacher-note", blocks[2]["type"])
}

func TestToolCallsToBlocks_Empty(t *testing.T) {
	blocks := ToolCallsToBlocks(nil)
	assert.Empty(t, blocks)
	assert.NotNil(t, blocks, "should return empty slice, not nil")
}

// --- JSON fallback parsing tests ---

func TestTryParseJSONBlocks_Valid(t *testing.T) {
	input := `[
		{"tool": "add_prose", "args": {"text": "Hello"}},
		{"tool": "add_code_snippet", "args": {"language": "python", "code": "print(1)"}}
	]`
	blocks := tryParseJSONBlocks(input)
	require.Len(t, blocks, 2)
	assert.Equal(t, "prose", blocks[0]["type"])
	assert.Equal(t, "code-snippet", blocks[1]["type"])
}

func TestTryParseJSONBlocks_WithCodeFence(t *testing.T) {
	input := "Here is the output:\n```json\n" +
		`[{"tool": "add_prose", "args": {"text": "Hello"}}]` +
		"\n```"
	blocks := tryParseJSONBlocks(input)
	require.Len(t, blocks, 1)
	assert.Equal(t, "prose", blocks[0]["type"])
}

func TestTryParseJSONBlocks_InvalidJSON(t *testing.T) {
	blocks := tryParseJSONBlocks("not json at all")
	assert.Nil(t, blocks)
}

func TestTryParseJSONBlocks_EmptyArray(t *testing.T) {
	blocks := tryParseJSONBlocks("[]")
	assert.Empty(t, blocks)
}

func TestExtractJSONArray_NestedBrackets(t *testing.T) {
	input := `Some text [{"a": [1,2]}, {"b": 3}] more text`
	result := extractJSONArray(input)
	assert.Equal(t, `[{"a": [1,2]}, {"b": 3}]`, result)
}

func TestExtractJSONArray_NoArray(t *testing.T) {
	result := extractJSONArray("no arrays here")
	assert.Equal(t, "", result)
}

func TestExtractJSONArray_UnmatchedBracket(t *testing.T) {
	result := extractJSONArray("[1, 2, 3")
	assert.Equal(t, "", result, "unmatched bracket should return empty")
}

func TestGenerateBlockID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateBlockID()
		assert.Len(t, id, 20, "ID should be 20 hex chars")
		assert.False(t, ids[id], "ID should be unique")
		ids[id] = true
	}
}

// --- Stub backend for tests that need a non-nil backend ---

type stubBackend struct{}

func (s *stubBackend) Name() string { return "stub" }

func (s *stubBackend) Chat(_ context.Context, _ []llm.Message, _ ...llm.ChatOption) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Content: "stub response"}, nil
}

func (s *stubBackend) StreamChat(_ context.Context, _ []llm.Message, _ ...llm.ChatOption) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (s *stubBackend) ChatWithTools(_ context.Context, _ []llm.Message, _ []llm.ToolSpec, _ ...llm.ChatOption) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{ToolCalls: []llm.ToolCall{
		{ID: "call-1", Name: "add_prose", Arguments: map[string]any{"text": "Hello from stub"}},
	}}, nil
}

func (s *stubBackend) SupportsTools() bool                             { return true }
func (s *stubBackend) ListModels(_ context.Context) ([]string, error) { return nil, nil }
