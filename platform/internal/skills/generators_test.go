package skills

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/llm"
	"github.com/weiboz0/bridge/platform/internal/tools"
)

// mockBackend implements llm.Backend for testing
type mockBackend struct {
	response *llm.LLMResponse
	err      error
}

func (m *mockBackend) Name() string { return "mock" }
func (m *mockBackend) Chat(ctx context.Context, messages []llm.Message, opts ...llm.ChatOption) (*llm.LLMResponse, error) {
	return m.response, m.err
}
func (m *mockBackend) StreamChat(ctx context.Context, messages []llm.Message, opts ...llm.ChatOption) (<-chan llm.StreamChunk, error) {
	return nil, nil
}
func (m *mockBackend) ChatWithTools(ctx context.Context, messages []llm.Message, toolSpecs []llm.ToolSpec, opts ...llm.ChatOption) (*llm.LLMResponse, error) {
	return m.response, m.err
}
func (m *mockBackend) SupportsTools() bool                            { return false }
func (m *mockBackend) ListModels(ctx context.Context) ([]string, error) { return nil, nil }

// --- ReportGenerator tests ---

func TestReportGenerator_GetSpec(t *testing.T) {
	gen := NewReportGenerator(nil)
	assert.Equal(t, "report_generator", gen.GetName())
	spec := gen.GetSpec()
	assert.Equal(t, "report_generator", spec.Name)
}

func TestReportGenerator_NilBackend(t *testing.T) {
	gen := NewReportGenerator(nil)
	result, err := gen.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"student_name": "Alice", "grade_level": "K-5"},
	})
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Payload["error"], "not configured")
}

func TestReportGenerator_MissingName(t *testing.T) {
	gen := NewReportGenerator(&mockBackend{})
	result, err := gen.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"grade_level": "6-8"},
	})
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Payload["error"], "student_name")
}

func TestReportGenerator_Success(t *testing.T) {
	backend := &mockBackend{
		response: &llm.LLMResponse{Content: "Alice has made excellent progress in Python."},
	}
	gen := NewReportGenerator(backend)
	result, err := gen.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"student_name":      "Alice",
			"grade_level":       "K-5",
			"sessions_attended": float64(10),
			"topics_covered":    []any{"variables", "loops"},
			"ai_interactions":   float64(5),
			"code_submissions":  float64(8),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	assert.Equal(t, "Alice has made excellent progress in Python.", result.Payload["report"])
	assert.Equal(t, "Alice", result.Payload["student_name"])
}

func TestReportGenerator_LLMError(t *testing.T) {
	backend := &mockBackend{err: assert.AnError}
	gen := NewReportGenerator(backend)
	result, err := gen.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"student_name": "Bob", "grade_level": "6-8"},
	})
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
}

// --- LessonGenerator tests ---

func TestLessonGenerator_GetSpec(t *testing.T) {
	gen := NewLessonGenerator(nil)
	assert.Equal(t, "lesson_generator", gen.GetName())
	spec := gen.GetSpec()
	assert.Equal(t, "lesson_generator", spec.Name)
}

func TestLessonGenerator_NilBackend(t *testing.T) {
	gen := NewLessonGenerator(nil)
	result, err := gen.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"topic": "loops", "language": "python", "grade_level": "6-8"},
	})
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Payload["error"], "not configured")
}

func TestLessonGenerator_MissingFields(t *testing.T) {
	gen := NewLessonGenerator(&mockBackend{})
	result, err := gen.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"topic": "loops"},
	})
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
}

func TestLessonGenerator_Success(t *testing.T) {
	backend := &mockBackend{
		response: &llm.LLMResponse{Content: "# For Loops\n\nA for loop repeats code..."},
	}
	gen := NewLessonGenerator(backend)
	result, err := gen.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"topic":       "for loops",
			"language":    "python",
			"grade_level": "6-8",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	assert.Contains(t, result.Payload["lesson"], "For Loops")
	assert.Equal(t, "for loops", result.Payload["topic"])
}

func TestLessonGenerator_LLMError(t *testing.T) {
	backend := &mockBackend{err: assert.AnError}
	gen := NewLessonGenerator(backend)
	result, err := gen.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"topic": "arrays", "language": "javascript", "grade_level": "9-12"},
	})
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
}
