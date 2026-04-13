package tools

import (
	"context"
	"fmt"
	"sort"

	"github.com/weiboz0/bridge/platform/internal/llm"
)

// Registry holds registered assistant tools and dispatches calls.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.GetName()] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// ToolSpecs returns all registered tool specs.
func (r *Registry) ToolSpecs() []ToolSpec {
	specs := make([]ToolSpec, 0, len(r.tools))
	for _, t := range r.tools {
		specs = append(specs, t.GetSpec())
	}
	return specs
}

// ToolSpecsAnthropic returns all specs in Anthropic format.
func (r *Registry) ToolSpecsAnthropic() []map[string]any {
	var out []map[string]any
	for _, t := range r.tools {
		out = append(out, t.GetSpec().ToAnthropic())
	}
	return out
}

// ToolSpecsOpenAI returns all specs in OpenAI format.
func (r *Registry) ToolSpecsOpenAI() []map[string]any {
	var out []map[string]any
	for _, t := range r.tools {
		out = append(out, t.GetSpec().ToOpenAI())
	}
	return out
}

// Execute runs a tool call, looking it up by name in the registry.
func (r *Registry) Execute(ctx context.Context, tc llm.ToolCall, userID, sessionID string, messageIndex int, messageHash string) (ToolResult, error) {
	t, ok := r.tools[tc.Name]
	if !ok {
		return ToolResult{
			ToolName: tc.Name,
			Status:   "error",
			Payload:  map[string]any{"error": fmt.Sprintf("unknown tool: %s", tc.Name)},
		}, fmt.Errorf("unknown tool: %s", tc.Name)
	}

	inv := ToolInvocation{
		ToolName:     tc.Name,
		UserID:       userID,
		Payload:      tc.Arguments,
		SessionID:    sessionID,
		MessageIndex: messageIndex,
		MessageHash:  messageHash,
	}
	return t.Invoke(ctx, inv)
}

// Copy returns a shallow copy of the registry.
func (r *Registry) Copy() *Registry {
	cp := NewRegistry()
	for k, v := range r.tools {
		cp.tools[k] = v
	}
	return cp
}

// ListToolNames returns a sorted list of registered tool names.
func (r *Registry) ListToolNames() []string {
	names := make([]string, 0, len(r.tools))
	for k := range r.tools {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
