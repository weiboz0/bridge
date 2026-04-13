package llm

import "context"

// ToolSpec describes a tool definition sent to the LLM.
// Imported from internal/tools when needed, but duplicated here to avoid
// circular imports — the Backend interface needs it for ChatWithTools.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// Backend is the interface all LLM backends must implement.
type Backend interface {
	// Name returns the backend identifier (e.g. "openai", "anthropic").
	Name() string

	// Chat sends messages and returns the complete response.
	Chat(ctx context.Context, messages []Message, opts ...ChatOption) (*LLMResponse, error)

	// StreamChat sends messages and returns a channel of streaming chunks.
	StreamChat(ctx context.Context, messages []Message, opts ...ChatOption) (<-chan StreamChunk, error)

	// ChatWithTools sends messages with tool definitions and returns the response.
	ChatWithTools(ctx context.Context, messages []Message, tools []ToolSpec, opts ...ChatOption) (*LLMResponse, error)

	// SupportsTools reports whether this backend+model supports native function calling.
	SupportsTools() bool

	// ListModels returns known model names for this backend.
	ListModels(ctx context.Context) ([]string, error)
}

// chatOpts holds resolved options for a chat call.
type chatOpts struct {
	Tools     []map[string]any
	MaxTokens int
	ExtraBody map[string]any
}

// ChatOption is a functional option for Chat/StreamChat calls.
type ChatOption func(*chatOpts)

// WithTools adds tool definitions to a chat call.
func WithTools(tools []map[string]any) ChatOption {
	return func(o *chatOpts) {
		o.Tools = tools
	}
}

// WithMaxTokens overrides max tokens for a single call.
func WithMaxTokens(n int) ChatOption {
	return func(o *chatOpts) {
		o.MaxTokens = n
	}
}

// WithExtraBody merges extra parameters into the request body.
func WithExtraBody(extra map[string]any) ChatOption {
	return func(o *chatOpts) {
		if o.ExtraBody == nil {
			o.ExtraBody = make(map[string]any)
		}
		for k, v := range extra {
			o.ExtraBody[k] = v
		}
	}
}

// resolveOpts applies ChatOption functions and returns the resolved options.
func resolveOpts(opts []ChatOption) chatOpts {
	var o chatOpts
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
