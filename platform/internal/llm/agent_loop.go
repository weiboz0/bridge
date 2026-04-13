package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// ErrMaxIterations is returned when the agentic loop exhausts its iteration budget
// without the LLM producing a final text-only response.
var ErrMaxIterations = fmt.Errorf("agentic loop: max iterations reached")

// ToolExecutor is called for each tool invocation during the agentic loop.
type ToolExecutor func(ctx context.Context, tc ToolCall) (map[string]any, error)

// AgenticLoopConfig configures the agentic loop.
type AgenticLoopConfig struct {
	MaxIterations int          // default 15
	Backend       Backend
	Tools         []ToolSpec
	ToolExecutor  ToolExecutor
}

// RunAgenticLoop executes a multi-turn LLM conversation with tool calling.
// Returns (finalContent, toolCallCount, error).
func RunAgenticLoop(ctx context.Context, cfg AgenticLoopConfig, messages []Message) (string, int, error) {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 15
	}

	toolCallCount := 0

	for i := 0; i < cfg.MaxIterations; i++ {
		select {
		case <-ctx.Done():
			return "", toolCallCount, ctx.Err()
		default:
		}

		var resp *LLMResponse
		var err error

		// Last iteration: disable tools to force text response.
		if i < cfg.MaxIterations-1 && len(cfg.Tools) > 0 {
			resp, err = cfg.Backend.ChatWithTools(ctx, messages, cfg.Tools)
		} else {
			resp, err = cfg.Backend.Chat(ctx, messages)
		}
		if err != nil {
			return "", toolCallCount, fmt.Errorf("llm call (iteration %d): %w", i, err)
		}

		// No tool calls — return text content.
		if len(resp.ToolCalls) == 0 {
			return resp.Content, toolCallCount, nil
		}

		// Build assistant message with content blocks (text + tool_use).
		blocks := resp.ToContentBlocks()
		blocksJSON, _ := json.Marshal(blocks)
		messages = append(messages, Message{
			Role:    RoleAssistant,
			Content: string(blocksJSON),
		})

		// Execute each tool call.
		for _, tc := range resp.ToolCalls {
			slog.Debug("executing tool", "name", tc.Name, "id", tc.ID, "iteration", i)

			result, execErr := cfg.ToolExecutor(ctx, tc)
			toolCallCount++

			var payload map[string]any
			if execErr != nil {
				payload = map[string]any{"error": execErr.Error()}
			} else {
				payload = result
			}

			resultJSON, _ := json.Marshal(payload)
			messages = append(messages, Message{
				Role:       RoleTool,
				Content:    string(resultJSON),
				ToolCallID: tc.ID,
			})
		}
	}

	return "Max iterations reached", toolCallCount, ErrMaxIterations
}
