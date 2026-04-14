package skills

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/tools"
)

func TestCodeRunner_NilExecutor(t *testing.T) {
	runner := NewCodeRunner(nil)
	result, err := runner.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"language": "python", "code": "print('hi')"},
	})
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Payload["error"], "not configured")
}

func TestCodeRunner_MissingInput(t *testing.T) {
	runner := NewCodeRunner(nil)
	result, err := runner.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"language": "python"},
	})
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
}

type mockExecutor struct {
	result *ExecuteResult
	err    error
}

func (m *mockExecutor) Execute(ctx context.Context, language, code string) (*ExecuteResult, error) {
	return m.result, m.err
}

func (m *mockExecutor) ExecuteWithStdin(ctx context.Context, language, code, stdin string) (*ExecuteResult, error) {
	return m.result, m.err
}

func TestCodeRunner_Success(t *testing.T) {
	executor := &mockExecutor{
		result: &ExecuteResult{
			Language: "python",
			Run:      RunOutput{Stdout: "hello\n", Code: 0},
		},
	}
	runner := NewCodeRunner(executor)
	result, err := runner.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"language": "python", "code": "print('hello')"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	assert.Equal(t, "hello\n", result.Payload["stdout"])
	assert.Equal(t, 0, result.Payload["exit_code"])
}

func TestCodeRunner_RuntimeError(t *testing.T) {
	executor := &mockExecutor{
		result: &ExecuteResult{
			Language: "python",
			Run:      RunOutput{Stderr: "NameError: name 'x' is not defined\n", Code: 1},
		},
	}
	runner := NewCodeRunner(executor)
	result, err := runner.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"language": "python", "code": "print(x)"},
	})
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
	assert.Equal(t, 1, result.Payload["exit_code"])
}

func TestCodeRunner_WithStdin(t *testing.T) {
	executor := &mockExecutor{
		result: &ExecuteResult{
			Language: "python",
			Run:      RunOutput{Stdout: "hello\n", Code: 0},
		},
	}
	runner := NewCodeRunner(executor)
	result, err := runner.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"language": "python", "code": "print(input())", "stdin": "hello"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
}

func TestCodeRunner_CompileError(t *testing.T) {
	executor := &mockExecutor{
		result: &ExecuteResult{
			Language: "cpp",
			Run:      RunOutput{Code: 0},
			Compile:  &RunOutput{Stderr: "error: expected ';'", Code: 1},
		},
	}
	runner := NewCodeRunner(executor)
	result, err := runner.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"language": "cpp", "code": "int main() {}"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status) // run succeeded even though compile had errors
	assert.Equal(t, "error: expected ';'", result.Payload["compile_stderr"])
}

func TestCodeRunner_GetSpec(t *testing.T) {
	runner := NewCodeRunner(nil)
	assert.Equal(t, "code_runner", runner.GetName())
	spec := runner.GetSpec()
	assert.Equal(t, "code_runner", spec.Name)
}
