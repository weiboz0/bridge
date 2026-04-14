package skills

import (
	"context"

	"github.com/weiboz0/bridge/platform/internal/tools"
)

// CodeExecutor is the interface for sandboxed code execution.
// Implemented by sandbox.PistonClient in Phase D.
type CodeExecutor interface {
	Execute(ctx context.Context, language, code string) (*ExecuteResult, error)
	ExecuteWithStdin(ctx context.Context, language, code, stdin string) (*ExecuteResult, error)
}

// ExecuteResult represents the output of code execution.
type ExecuteResult struct {
	Language string
	Run      RunOutput
	Compile  *RunOutput // nil for interpreted languages
}

// RunOutput holds stdout/stderr/exit code from a run or compile step.
type RunOutput struct {
	Stdout string
	Stderr string
	Code   int
}

// CodeRunner executes code in a sandbox and returns stdout/stderr.
type CodeRunner struct {
	executor CodeExecutor
}

func NewCodeRunner(executor CodeExecutor) *CodeRunner {
	return &CodeRunner{executor: executor}
}

func (t *CodeRunner) GetName() string { return "code_runner" }

func (t *CodeRunner) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        "code_runner",
		Description: "Execute code in a sandboxed environment and return the output. Supports Python, JavaScript, C++, Java, Rust, and more.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"language": map[string]any{
					"type":        "string",
					"description": "Programming language (python, javascript, cpp, java, rust, etc.)",
				},
				"code": map[string]any{
					"type":        "string",
					"description": "The source code to execute",
				},
				"stdin": map[string]any{
					"type":        "string",
					"description": "Optional standard input to provide to the program",
				},
			},
			"required": []string{"language", "code"},
		},
	}
}

func (t *CodeRunner) Invoke(ctx context.Context, inv tools.ToolInvocation) (tools.ToolResult, error) {
	language, _ := inv.Payload["language"].(string)
	code, _ := inv.Payload["code"].(string)
	stdin, _ := inv.Payload["stdin"].(string)

	if language == "" || code == "" {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": "language and code are required"},
		}, nil
	}

	if t.executor == nil {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": "code execution not configured"},
		}, nil
	}

	var resp *ExecuteResult
	var err error
	if stdin != "" {
		resp, err = t.executor.ExecuteWithStdin(ctx, language, code, stdin)
	} else {
		resp, err = t.executor.Execute(ctx, language, code)
	}
	if err != nil {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": err.Error()},
		}, nil
	}

	payload := map[string]any{
		"stdout":    resp.Run.Stdout,
		"stderr":    resp.Run.Stderr,
		"exit_code": resp.Run.Code,
		"language":  resp.Language,
	}
	if resp.Compile != nil {
		payload["compile_stdout"] = resp.Compile.Stdout
		payload["compile_stderr"] = resp.Compile.Stderr
		payload["compile_exit_code"] = resp.Compile.Code
	}

	status := "ok"
	if resp.Run.Code != 0 {
		status = "error"
	}

	return tools.ToolResult{
		ToolName: t.GetName(),
		Status:   status,
		Payload:  payload,
	}, nil
}
