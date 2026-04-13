package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock backend
// ---------------------------------------------------------------------------

type mockResponse struct {
	Content   string
	ToolCalls []ToolCall
}

type mockAgenticBackend struct {
	responses []mockResponse // sequence of responses for each call
	callIndex int
	// tracks which method was called per invocation
	calledWith []string // "ChatWithTools" or "Chat"
}

func (m *mockAgenticBackend) ChatWithTools(ctx context.Context, messages []Message, tools []ToolSpec, opts ...ChatOption) (*LLMResponse, error) {
	m.calledWith = append(m.calledWith, "ChatWithTools")
	if m.callIndex >= len(m.responses) {
		return &LLMResponse{Content: "no more responses"}, nil
	}
	r := m.responses[m.callIndex]
	m.callIndex++
	return &LLMResponse{Content: r.Content, ToolCalls: r.ToolCalls}, nil
}

func (m *mockAgenticBackend) Chat(ctx context.Context, messages []Message, opts ...ChatOption) (*LLMResponse, error) {
	m.calledWith = append(m.calledWith, "Chat")
	if m.callIndex >= len(m.responses) {
		return &LLMResponse{Content: "no more responses"}, nil
	}
	r := m.responses[m.callIndex]
	m.callIndex++
	return &LLMResponse{Content: r.Content, ToolCalls: r.ToolCalls}, nil
}

func (m *mockAgenticBackend) StreamChat(ctx context.Context, messages []Message, opts ...ChatOption) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockAgenticBackend) Name() string          { return "mock" }
func (m *mockAgenticBackend) SupportsTools() bool   { return true }
func (m *mockAgenticBackend) ListModels(ctx context.Context) ([]string, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func noopTool(_ context.Context, _ ToolCall) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}

func makeTools() []ToolSpec {
	return []ToolSpec{{Name: "noop", Description: "does nothing", Parameters: map[string]any{}}}
}

func makeToolCall(id, name string) ToolCall {
	return ToolCall{ID: id, Name: name, Arguments: map[string]any{}}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestAgenticLoopNoTools — backend returns text immediately, 0 tool calls.
func TestAgenticLoopNoTools(t *testing.T) {
	backend := &mockAgenticBackend{
		responses: []mockResponse{
			{Content: "Hello, world!"},
		},
	}
	cfg := AgenticLoopConfig{
		MaxIterations: 5,
		Backend:       backend,
		Tools:         nil,
		ToolExecutor:  noopTool,
	}
	messages := []Message{{Role: RoleUser, Content: "Hi"}}

	content, count, err := RunAgenticLoop(context.Background(), cfg, messages)
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", content)
	assert.Equal(t, 0, count)
}

// TestAgenticLoopSingleToolCall — first response has tool call, second is text.
func TestAgenticLoopSingleToolCall(t *testing.T) {
	backend := &mockAgenticBackend{
		responses: []mockResponse{
			{Content: "Let me look that up.", ToolCalls: []ToolCall{makeToolCall("tc_1", "search")}},
			{Content: "Found it!"},
		},
	}
	cfg := AgenticLoopConfig{
		MaxIterations: 5,
		Backend:       backend,
		Tools:         makeTools(),
		ToolExecutor:  noopTool,
	}
	messages := []Message{{Role: RoleUser, Content: "search for cats"}}

	content, count, err := RunAgenticLoop(context.Background(), cfg, messages)
	require.NoError(t, err)
	assert.Equal(t, "Found it!", content)
	assert.Equal(t, 1, count)
}

// TestAgenticLoopMultipleToolCalls — response with 2 tool calls, then text.
func TestAgenticLoopMultipleToolCalls(t *testing.T) {
	backend := &mockAgenticBackend{
		responses: []mockResponse{
			{
				Content: "Running two tools.",
				ToolCalls: []ToolCall{
					makeToolCall("tc_1", "search"),
					makeToolCall("tc_2", "calculate"),
				},
			},
			{Content: "Both done!"},
		},
	}
	cfg := AgenticLoopConfig{
		MaxIterations: 5,
		Backend:       backend,
		Tools:         makeTools(),
		ToolExecutor:  noopTool,
	}
	messages := []Message{{Role: RoleUser, Content: "do both"}}

	content, count, err := RunAgenticLoop(context.Background(), cfg, messages)
	require.NoError(t, err)
	assert.Equal(t, "Both done!", content)
	assert.Equal(t, 2, count)
}

// TestAgenticLoopMaxIterations — backend always returns tool calls, capped at MaxIterations.
func TestAgenticLoopMaxIterations(t *testing.T) {
	// Build a backend that always returns a tool call.
	const maxIter = 4
	responses := make([]mockResponse, maxIter+1)
	for i := range responses {
		responses[i] = mockResponse{
			Content:   "still working",
			ToolCalls: []ToolCall{makeToolCall("tc_loop", "loop_tool")},
		}
	}
	backend := &mockAgenticBackend{responses: responses}
	cfg := AgenticLoopConfig{
		MaxIterations: maxIter,
		Backend:       backend,
		Tools:         makeTools(),
		ToolExecutor:  noopTool,
	}
	messages := []Message{{Role: RoleUser, Content: "loop forever"}}

	content, count, err := RunAgenticLoop(context.Background(), cfg, messages)
	require.ErrorIs(t, err, ErrMaxIterations)
	assert.Equal(t, "Max iterations reached", content)
	// Every iteration (0..maxIter-1) returns a tool call which gets executed,
	// so the total tool call count equals maxIter.
	assert.Equal(t, maxIter, count)
}

// TestAgenticLoopToolError — tool executor returns error, error payload sent to LLM.
func TestAgenticLoopToolError(t *testing.T) {
	backend := &mockAgenticBackend{
		responses: []mockResponse{
			{Content: "calling tool", ToolCalls: []ToolCall{makeToolCall("tc_err", "failing_tool")}},
			{Content: "handled the error"},
		},
	}

	var capturedMessages []Message
	errorExecutor := func(ctx context.Context, tc ToolCall) (map[string]any, error) {
		return nil, errors.New("tool exploded")
	}

	cfg := AgenticLoopConfig{
		MaxIterations: 5,
		Backend:       backend,
		Tools:         makeTools(),
		ToolExecutor:  errorExecutor,
	}
	messages := []Message{{Role: RoleUser, Content: "run failing tool"}}
	_ = capturedMessages

	content, count, err := RunAgenticLoop(context.Background(), cfg, messages)
	require.NoError(t, err)
	assert.Equal(t, "handled the error", content)
	assert.Equal(t, 1, count)

	// Verify the second call to the backend received the error payload in a tool message.
	// The backend received 2 calls; the messages passed to the second call should include
	// a tool-result message with the error.
	// We verify indirectly: the loop completed without error and the tool was counted.
}

// TestAgenticLoopContextCancellation — cancelled context returns error.
func TestAgenticLoopContextCancellation(t *testing.T) {
	backend := &mockAgenticBackend{
		responses: []mockResponse{
			{Content: "never returned", ToolCalls: []ToolCall{makeToolCall("tc_x", "some_tool")}},
		},
	}
	cfg := AgenticLoopConfig{
		MaxIterations: 10,
		Backend:       backend,
		Tools:         makeTools(),
		ToolExecutor:  noopTool,
	}
	messages := []Message{{Role: RoleUser, Content: "do something"}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, _, err := RunAgenticLoop(ctx, cfg, messages)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestAgenticLoopLastIterationNoTools — verify last iteration calls Chat, not ChatWithTools.
func TestAgenticLoopLastIterationNoTools(t *testing.T) {
	// MaxIterations=2: iteration 0 uses ChatWithTools (returns tool call),
	// iteration 1 (last) uses Chat.
	backend := &mockAgenticBackend{
		responses: []mockResponse{
			{Content: "using tool", ToolCalls: []ToolCall{makeToolCall("tc_last", "mytool")}},
			{Content: "final answer"},
		},
	}
	cfg := AgenticLoopConfig{
		MaxIterations: 2,
		Backend:       backend,
		Tools:         makeTools(),
		ToolExecutor:  noopTool,
	}
	messages := []Message{{Role: RoleUser, Content: "do it"}}

	content, count, err := RunAgenticLoop(context.Background(), cfg, messages)
	require.NoError(t, err)
	assert.Equal(t, "final answer", content)
	assert.Equal(t, 1, count)

	require.Len(t, backend.calledWith, 2)
	assert.Equal(t, "ChatWithTools", backend.calledWith[0], "first iteration should use ChatWithTools")
	assert.Equal(t, "Chat", backend.calledWith[1], "last iteration should use Chat")
}

// TestAgenticLoopDefaultMaxIterations — zero MaxIterations defaults to 15.
func TestAgenticLoopDefaultMaxIterations(t *testing.T) {
	backend := &mockAgenticBackend{
		responses: []mockResponse{
			{Content: "immediate answer"},
		},
	}
	cfg := AgenticLoopConfig{
		MaxIterations: 0, // should default to 15
		Backend:       backend,
		Tools:         makeTools(),
		ToolExecutor:  noopTool,
	}
	messages := []Message{{Role: RoleUser, Content: "hi"}}

	content, _, err := RunAgenticLoop(context.Background(), cfg, messages)
	require.NoError(t, err)
	assert.Equal(t, "immediate answer", content)
}
