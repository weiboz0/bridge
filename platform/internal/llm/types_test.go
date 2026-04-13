package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Message.ToDict
// ---------------------------------------------------------------------------

func TestMessageToDict(t *testing.T) {
	t.Run("string content", func(t *testing.T) {
		m := Message{Role: RoleUser, Content: "hello"}
		d := m.ToDict()
		assert.Equal(t, "user", d["role"])
		assert.Equal(t, "hello", d["content"])
		assert.NotContains(t, d, "name")
		assert.NotContains(t, d, "tool_call_id")
	})

	t.Run("with name", func(t *testing.T) {
		m := Message{Role: RoleAssistant, Content: "hi", Name: "bot"}
		d := m.ToDict()
		assert.Equal(t, "bot", d["name"])
	})

	t.Run("with tool call id", func(t *testing.T) {
		m := Message{Role: RoleTool, Content: "result", ToolCallID: "call_abc"}
		d := m.ToDict()
		assert.Equal(t, "call_abc", d["tool_call_id"])
	})

	t.Run("with name and tool call id", func(t *testing.T) {
		m := Message{Role: RoleTool, Content: "ok", Name: "weather", ToolCallID: "call_xyz"}
		d := m.ToDict()
		assert.Equal(t, "weather", d["name"])
		assert.Equal(t, "call_xyz", d["tool_call_id"])
	})

	t.Run("system role", func(t *testing.T) {
		m := Message{Role: RoleSystem, Content: "be helpful"}
		d := m.ToDict()
		assert.Equal(t, "system", d["role"])
		assert.Equal(t, "be helpful", d["content"])
	})

	t.Run("content block slice", func(t *testing.T) {
		blocks := []map[string]any{
			{"type": "text", "text": "hello"},
		}
		m := Message{Role: RoleAssistant, Content: blocks}
		d := m.ToDict()
		assert.Equal(t, blocks, d["content"])
	})
}

// ---------------------------------------------------------------------------
// LLMResponse.ToContentBlocks
// ---------------------------------------------------------------------------

func TestToContentBlocksText(t *testing.T) {
	r := &LLMResponse{Content: "Hello, world!"}
	blocks := r.ToContentBlocks()
	require.Len(t, blocks, 1)
	assert.Equal(t, "text", blocks[0]["type"])
	assert.Equal(t, "Hello, world!", blocks[0]["text"])
}

func TestToContentBlocksEmpty(t *testing.T) {
	r := &LLMResponse{}
	blocks := r.ToContentBlocks()
	assert.Empty(t, blocks)
}

func TestToContentBlocksWithThinking(t *testing.T) {
	r := &LLMResponse{
		Thinking: "Let me reason about this…",
		Content:  "The answer is 42.",
	}
	blocks := r.ToContentBlocks()
	require.Len(t, blocks, 2)

	// Thinking block comes first.
	assert.Equal(t, "thinking", blocks[0]["type"])
	assert.Equal(t, "Let me reason about this…", blocks[0]["thinking"])

	// Then the text block.
	assert.Equal(t, "text", blocks[1]["type"])
	assert.Equal(t, "The answer is 42.", blocks[1]["text"])
}

func TestToContentBlocksThinkingOnly(t *testing.T) {
	r := &LLMResponse{Thinking: "inner monologue"}
	blocks := r.ToContentBlocks()
	require.Len(t, blocks, 1)
	assert.Equal(t, "thinking", blocks[0]["type"])
}

func TestToContentBlocksWithToolCalls(t *testing.T) {
	r := &LLMResponse{
		Content: "I'll look that up.",
		ToolCalls: []ToolCall{
			{
				ID:        "call_1",
				Name:      "search",
				Arguments: map[string]any{"query": "Go testing"},
			},
			{
				ID:        "call_2",
				Name:      "calculator",
				Arguments: map[string]any{"expr": "2+2"},
			},
		},
	}
	blocks := r.ToContentBlocks()
	require.Len(t, blocks, 3)

	// Text block first.
	assert.Equal(t, "text", blocks[0]["type"])

	// Tool-use blocks follow.
	assert.Equal(t, "tool_use", blocks[1]["type"])
	assert.Equal(t, "call_1", blocks[1]["id"])
	assert.Equal(t, "search", blocks[1]["name"])
	assert.Equal(t, map[string]any{"query": "Go testing"}, blocks[1]["input"])

	assert.Equal(t, "tool_use", blocks[2]["type"])
	assert.Equal(t, "call_2", blocks[2]["id"])
	assert.Equal(t, "calculator", blocks[2]["name"])
}

func TestToContentBlocksToolCallsNoText(t *testing.T) {
	r := &LLMResponse{
		ToolCalls: []ToolCall{
			{ID: "call_3", Name: "noop", Arguments: map[string]any{}},
		},
	}
	blocks := r.ToContentBlocks()
	require.Len(t, blocks, 1)
	assert.Equal(t, "tool_use", blocks[0]["type"])
}

func TestToContentBlocksAllTypes(t *testing.T) {
	r := &LLMResponse{
		Thinking: "chain of thought",
		Content:  "final answer",
		ToolCalls: []ToolCall{
			{ID: "call_4", Name: "emit", Arguments: map[string]any{"msg": "done"}},
		},
	}
	blocks := r.ToContentBlocks()
	require.Len(t, blocks, 3)
	assert.Equal(t, "thinking", blocks[0]["type"])
	assert.Equal(t, "text", blocks[1]["type"])
	assert.Equal(t, "tool_use", blocks[2]["type"])
}

// ---------------------------------------------------------------------------
// contentToString
// ---------------------------------------------------------------------------

func TestContentToString(t *testing.T) {
	t.Run("string passthrough", func(t *testing.T) {
		assert.Equal(t, "hello", contentToString("hello"))
	})

	t.Run("empty string", func(t *testing.T) {
		assert.Equal(t, "", contentToString(""))
	})

	t.Run("slice of any marshals to JSON", func(t *testing.T) {
		input := []any{"a", "b", "c"}
		result := contentToString(input)
		assert.Equal(t, `["a","b","c"]`, result)
	})

	t.Run("map marshals to JSON", func(t *testing.T) {
		// Use a single-key map so the output is deterministic.
		input := map[string]any{"key": "value"}
		result := contentToString(input)
		assert.Equal(t, `{"key":"value"}`, result)
	})

	t.Run("int marshals to JSON", func(t *testing.T) {
		result := contentToString(42)
		assert.Equal(t, "42", result)
	})

	t.Run("bool marshals to JSON", func(t *testing.T) {
		assert.Equal(t, "true", contentToString(true))
		assert.Equal(t, "false", contentToString(false))
	})
}

// ---------------------------------------------------------------------------
// WithExtraBody ChatOption
// ---------------------------------------------------------------------------

func TestWithExtraBody(t *testing.T) {
	t.Run("sets values on empty opts", func(t *testing.T) {
		opt := WithExtraBody(map[string]any{"betas": []string{"x"}})
		var o chatOpts
		opt(&o)
		require.NotNil(t, o.ExtraBody)
		assert.Equal(t, []string{"x"}, o.ExtraBody["betas"])
	})

	t.Run("merges into existing extra body", func(t *testing.T) {
		var o chatOpts
		WithExtraBody(map[string]any{"a": 1})(&o)
		WithExtraBody(map[string]any{"b": 2})(&o)
		assert.Equal(t, 1, o.ExtraBody["a"])
		assert.Equal(t, 2, o.ExtraBody["b"])
	})

	t.Run("later value overwrites earlier for same key", func(t *testing.T) {
		var o chatOpts
		WithExtraBody(map[string]any{"key": "first"})(&o)
		WithExtraBody(map[string]any{"key": "second"})(&o)
		assert.Equal(t, "second", o.ExtraBody["key"])
	})
}

// ---------------------------------------------------------------------------
// resolveOpts
// ---------------------------------------------------------------------------

func TestResolveOpts(t *testing.T) {
	t.Run("empty opts returns zero value", func(t *testing.T) {
		o := resolveOpts(nil)
		assert.Nil(t, o.Tools)
		assert.Equal(t, 0, o.MaxTokens)
		assert.Nil(t, o.ExtraBody)
	})

	t.Run("WithMaxTokens is applied", func(t *testing.T) {
		o := resolveOpts([]ChatOption{WithMaxTokens(256)})
		assert.Equal(t, 256, o.MaxTokens)
	})

	t.Run("WithTools is applied", func(t *testing.T) {
		tools := []map[string]any{{"name": "tool1"}}
		o := resolveOpts([]ChatOption{WithTools(tools)})
		assert.Equal(t, tools, o.Tools)
	})

	t.Run("multiple options are all applied", func(t *testing.T) {
		tools := []map[string]any{{"name": "t"}}
		extra := map[string]any{"k": "v"}
		o := resolveOpts([]ChatOption{
			WithMaxTokens(512),
			WithTools(tools),
			WithExtraBody(extra),
		})
		assert.Equal(t, 512, o.MaxTokens)
		assert.Equal(t, tools, o.Tools)
		assert.Equal(t, "v", o.ExtraBody["k"])
	})

	t.Run("last WithMaxTokens wins", func(t *testing.T) {
		o := resolveOpts([]ChatOption{WithMaxTokens(100), WithMaxTokens(200)})
		assert.Equal(t, 200, o.MaxTokens)
	})
}
