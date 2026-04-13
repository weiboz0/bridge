package tools

import (
	"context"
	"testing"

	"github.com/weiboz0/bridge/platform/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTool is a test tool implementation.
type mockTool struct {
	name string
	spec ToolSpec
}

func (m *mockTool) GetName() string  { return m.name }
func (m *mockTool) GetSpec() ToolSpec { return m.spec }
func (m *mockTool) Invoke(_ context.Context, inv ToolInvocation) (ToolResult, error) {
	return ToolResult{
		ToolName: m.name,
		Status:   "ok",
		Payload:  map[string]any{"echo": inv.Payload},
	}, nil
}

func newMockTool(name, desc string) *mockTool {
	return &mockTool{
		name: name,
		spec: ToolSpec{
			Name:        name,
			Description: desc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		},
	}
}

func TestRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	tool := newMockTool("search", "Search the web")
	reg.Register(tool)

	got, ok := reg.Get("search")
	require.True(t, ok)
	assert.Equal(t, "search", got.GetName())

	_, ok = reg.Get("nonexistent")
	assert.False(t, ok)
}

func TestToolSpecs(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("search", "Search"))
	reg.Register(newMockTool("calculate", "Calculate"))

	specs := reg.ToolSpecs()
	assert.Len(t, specs, 2)

	names := make(map[string]bool)
	for _, s := range specs {
		names[s.Name] = true
	}
	assert.True(t, names["search"])
	assert.True(t, names["calculate"])
}

func TestToolSpecsAnthropic(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("search", "Search the web"))

	specs := reg.ToolSpecsAnthropic()
	require.Len(t, specs, 1)
	assert.Equal(t, "search", specs[0]["name"])
	assert.NotNil(t, specs[0]["input_schema"])
}

func TestToolSpecsOpenAI(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("search", "Search the web"))

	specs := reg.ToolSpecsOpenAI()
	require.Len(t, specs, 1)
	assert.Equal(t, "function", specs[0]["type"])
	fn, ok := specs[0]["function"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "search", fn["name"])
}

func TestExecute(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("search", "Search the web"))

	tc := llm.ToolCall{
		ID:        "call_123",
		Name:      "search",
		Arguments: map[string]any{"query": "golang"},
	}

	result, err := reg.Execute(context.Background(), tc, "user1", "session1", 0, "hash1")
	require.NoError(t, err)
	assert.Equal(t, "search", result.ToolName)
	assert.Equal(t, "ok", result.Status)
	assert.NotNil(t, result.Payload["echo"])
}

func TestExecute_UnknownTool(t *testing.T) {
	reg := NewRegistry()

	tc := llm.ToolCall{
		ID:   "call_999",
		Name: "nonexistent",
	}

	result, err := reg.Execute(context.Background(), tc, "user1", "session1", 0, "")
	require.Error(t, err)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, err.Error(), "unknown tool")
}

func TestCopy(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("search", "Search"))
	reg.Register(newMockTool("calc", "Calculate"))

	cp := reg.Copy()

	// Copy has same tools.
	assert.ElementsMatch(t, reg.ListToolNames(), cp.ListToolNames())

	// Modifying copy doesn't affect original.
	cp.Register(newMockTool("new_tool", "New"))
	assert.Len(t, reg.ListToolNames(), 2)
	assert.Len(t, cp.ListToolNames(), 3)
}

func TestListToolNames(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("beta", "B"))
	reg.Register(newMockTool("alpha", "A"))

	names := reg.ListToolNames()
	assert.Equal(t, []string{"alpha", "beta"}, names) // sorted
}

func TestToolSpec_ToAnthropic(t *testing.T) {
	spec := ToolSpec{
		Name:        "search",
		Description: "Search the web",
		Parameters:  map[string]any{"type": "object"},
	}
	ant := spec.ToAnthropic()
	assert.Equal(t, "search", ant["name"])
	assert.Equal(t, "Search the web", ant["description"])
	assert.Equal(t, map[string]any{"type": "object"}, ant["input_schema"])
}

func TestToolSpec_ToOpenAI(t *testing.T) {
	spec := ToolSpec{
		Name:        "search",
		Description: "Search the web",
		Parameters:  map[string]any{"type": "object"},
	}
	oai := spec.ToOpenAI()
	assert.Equal(t, "function", oai["type"])
	fn, ok := oai["function"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "search", fn["name"])
	assert.Equal(t, "Search the web", fn["description"])
	assert.Equal(t, map[string]any{"type": "object"}, fn["parameters"])
}
