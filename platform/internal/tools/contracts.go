// Package tools provides the tool registry and contracts for assistant tools.
package tools

import "context"

// ToolSpec is a JSON-serializable tool definition sent to the LLM.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// ToAnthropic returns the Anthropic-format tool definition.
func (s ToolSpec) ToAnthropic() map[string]any {
	return map[string]any{
		"name":         s.Name,
		"description":  s.Description,
		"input_schema": s.Parameters,
	}
}

// ToOpenAI returns the OpenAI-format tool definition.
func (s ToolSpec) ToOpenAI() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        s.Name,
			"description": s.Description,
			"parameters":  s.Parameters,
		},
	}
}

// ToolInvocation represents a structured assistant tool call.
type ToolInvocation struct {
	ToolName     string         `json:"tool_name"`
	TenantID     string         `json:"tenant_id"`
	UserID       string         `json:"user_id"`
	Payload      map[string]any `json:"payload"`
	SessionID    string         `json:"session_id"`
	MessageIndex int            `json:"message_index"`
	MessageHash  string         `json:"message_hash"`
}

// ToolResult is the structured result returned from a tool invocation.
type ToolResult struct {
	ToolName string         `json:"tool_name"`
	Status   string         `json:"status"` // "ok" or "error"
	Payload  map[string]any `json:"payload"`
}

// Tool is the interface that all assistant tools must implement.
type Tool interface {
	GetName() string
	GetSpec() ToolSpec
	Invoke(ctx context.Context, inv ToolInvocation) (ToolResult, error)
}
