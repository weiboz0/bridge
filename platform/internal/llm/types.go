// Package llm provides the LLM abstraction layer: types, backend interface, and implementations.
package llm

import (
	"encoding/json"
	"fmt"
)

// MessageRole identifies the role of a message in a conversation.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
	RoleDirective MessageRole = "directive" // system directs LLM to act
	RoleStatus    MessageRole = "status"    // informational status update (display only)
	RoleCommand   MessageRole = "command"   // slash command output (display only)
)

// ContentBlock represents a typed content block (text, image, tool_use, etc.).
type ContentBlock = map[string]any

// Message represents a single message in a conversation.
type Message struct {
	Role       MessageRole
	Content    any    // string or []ContentBlock
	Name       string // optional sender name
	ToolCallID string // for tool-result messages
}

// Text extracts plain text from Content. If Content is a string it is returned
// directly; if it is a slice of content blocks, all "text" blocks are concatenated.
func (m Message) Text() string {
	switch c := m.Content.(type) {
	case string:
		return c
	case []map[string]any:
		var out string
		for _, b := range c {
			if b["type"] == "text" {
				if t, ok := b["text"].(string); ok {
					out += t
				}
			}
		}
		return out
	default:
		return fmt.Sprintf("%v", c)
	}
}

// ToDict returns a map representation suitable for JSON serialization.
func (m Message) ToDict() map[string]any {
	d := map[string]any{
		"role":    string(m.Role),
		"content": m.Content,
	}
	if m.Name != "" {
		d["name"] = m.Name
	}
	if m.ToolCallID != "" {
		d["tool_call_id"] = m.ToolCallID
	}
	return d
}

// LLMConfig holds configuration for creating an LLM backend.
type LLMConfig struct {
	Backend     string         `json:"backend" toml:"backend"`
	Model       string         `json:"model" toml:"model"`
	APIKey      string         `json:"api_key" toml:"api_key"`
	BaseURL     string         `json:"base_url" toml:"base_url"`
	Temperature float64        `json:"temperature" toml:"temperature"`
	MaxTokens   int            `json:"max_tokens" toml:"max_tokens"`
	Streaming   bool           `json:"streaming" toml:"streaming"`
	ExtraParams map[string]any `json:"extra_params" toml:"extra_params"`
}

// StreamChunk represents one piece of a streaming LLM response.
type StreamChunk struct {
	Delta        string     `json:"delta"`
	IsFinal      bool       `json:"is_final"`
	FinishReason string     `json:"finish_reason,omitempty"`
	ChunkType    string     `json:"chunk_type"` // "text", "thinking", "tool_call"
	Usage        *UsageInfo `json:"usage,omitempty"`
}

// UsageInfo reports token consumption for a request.
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ToolCall represents a tool/function call emitted by the LLM.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// LLMResponse is the complete response from an LLM chat call.
type LLMResponse struct {
	Content      string     `json:"content"`
	Model        string     `json:"model"`
	Usage        *UsageInfo `json:"usage,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Thinking     string     `json:"thinking,omitempty"`
}

// ToContentBlocks builds assistant content blocks for message history.
// Returns a list of content blocks: thinking, text, tool_use.
func (r *LLMResponse) ToContentBlocks() []map[string]any {
	var blocks []map[string]any
	if r.Thinking != "" {
		blocks = append(blocks, map[string]any{
			"type":     "thinking",
			"thinking": r.Thinking,
		})
	}
	if r.Content != "" {
		blocks = append(blocks, map[string]any{
			"type": "text",
			"text": r.Content,
		})
	}
	for _, tc := range r.ToolCalls {
		blocks = append(blocks, map[string]any{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Name,
			"input": tc.Arguments,
		})
	}
	return blocks
}

// contentToString converts any content value to a string for OpenAI tool results.
func contentToString(content any) string {
	switch c := content.(type) {
	case string:
		return c
	default:
		b, err := json.Marshal(c)
		if err != nil {
			return fmt.Sprintf("%v", c)
		}
		return string(b)
	}
}
