package sandbox

import (
	"context"

	"github.com/weiboz0/bridge/platform/internal/skills"
)

// PistonExecutor adapts PistonClient to the skills.CodeExecutor interface.
type PistonExecutor struct {
	client *PistonClient
}

// NewPistonExecutor creates a PistonExecutor wrapping the given client.
func NewPistonExecutor(client *PistonClient) *PistonExecutor {
	return &PistonExecutor{client: client}
}

func (e *PistonExecutor) Execute(ctx context.Context, language, code string) (*skills.ExecuteResult, error) {
	resp, err := e.client.Execute(ctx, language, code)
	if err != nil {
		return nil, err
	}
	return convertPistonResponse(resp), nil
}

func (e *PistonExecutor) ExecuteWithStdin(ctx context.Context, language, code, stdin string) (*skills.ExecuteResult, error) {
	resp, err := e.client.ExecuteWithStdin(ctx, language, code, stdin)
	if err != nil {
		return nil, err
	}
	return convertPistonResponse(resp), nil
}

func convertPistonResponse(resp *PistonExecuteResponse) *skills.ExecuteResult {
	result := &skills.ExecuteResult{
		Language: resp.Language,
		Run: skills.RunOutput{
			Stdout: resp.Run.Stdout,
			Stderr: resp.Run.Stderr,
			Code:   resp.Run.Code,
		},
	}
	if resp.Compile != nil {
		result.Compile = &skills.RunOutput{
			Stdout: resp.Compile.Stdout,
			Stderr: resp.Compile.Stderr,
			Code:   resp.Compile.Code,
		}
	}
	return result
}
