package skills

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/tools"
)

func TestCodeAnalyzer_GetSpec(t *testing.T) {
	analyzer := NewCodeAnalyzer()
	assert.Equal(t, "code_analyzer", analyzer.GetName())
}

func TestCodeAnalyzer_PythonMissingColon(t *testing.T) {
	analyzer := NewCodeAnalyzer()
	result, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"language": "python",
			"code":     "if x == 1\n    print('hi')",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)

	issues, ok := result.Payload["issues"].([]CodeIssue)
	require.True(t, ok)
	found := false
	for _, issue := range issues {
		if issue.Type == "error" && issue.Line == 1 {
			found = true
		}
	}
	assert.True(t, found, "should detect missing colon")
}

func TestCodeAnalyzer_PythonValid(t *testing.T) {
	analyzer := NewCodeAnalyzer()
	result, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"language": "python",
			"code":     "if x == 1:\n    print('hi')",
		},
	})
	require.NoError(t, err)
	issues := result.Payload["issues"].([]CodeIssue)
	assert.Empty(t, issues)
}

func TestCodeAnalyzer_JSVarUsage(t *testing.T) {
	analyzer := NewCodeAnalyzer()
	result, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"language": "javascript",
			"code":     "var x = 1;",
		},
	})
	require.NoError(t, err)
	issues := result.Payload["issues"].([]CodeIssue)
	assert.Len(t, issues, 1)
	assert.Equal(t, "style", issues[0].Type)
	assert.Contains(t, issues[0].Message, "let")
}

func TestCodeAnalyzer_CppMissingInclude(t *testing.T) {
	analyzer := NewCodeAnalyzer()
	result, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"language": "cpp",
			"code":     "int main() {\n    cout << \"hello\";\n}",
		},
	})
	require.NoError(t, err)
	issues := result.Payload["issues"].([]CodeIssue)
	found := false
	for _, issue := range issues {
		if issue.Type == "warning" && strings.Contains(issue.Message, "iostream") {
			found = true
		}
	}
	assert.True(t, found, "should detect missing iostream")
}

func TestCodeAnalyzer_Metrics(t *testing.T) {
	analyzer := NewCodeAnalyzer()
	result, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"language": "python",
			"code":     "x = 1\n\ny = 2\n",
		},
	})
	require.NoError(t, err)
	metrics := result.Payload["metrics"].(map[string]any)
	assert.Equal(t, 4, metrics["line_count"])
	assert.Equal(t, 2, metrics["non_empty_lines"])
}

func TestCodeAnalyzer_MissingInput(t *testing.T) {
	analyzer := NewCodeAnalyzer()
	result, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{"language": "python"},
	})
	assert.NoError(t, err)
	assert.Equal(t, "error", result.Status)
}
