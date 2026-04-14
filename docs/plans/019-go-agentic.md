# Plan 019: Go Agentic — AI Integration, Agentic Loop, Piston, Reports

> **NOTE:** This plan requires review and approval before execution. Do not execute until the user explicitly approves.

## Overview

Phase 3 of the Go backend migration (spec `docs/specs/004-go-backend-migration.md`). Migrates all AI/LLM functionality from Next.js to Go: the LLM abstraction layer, agentic loop with tool calling, Bridge-specific AI tools (code runner, code analyzer, tutor, report generator, lesson generator), streaming SSE chat, and Piston-based code execution for compiled languages.

**Source reference:** `/home/chris/workshop/magicburg-go/platform/internal/` — proven Go patterns for LLM, tools, events, and agentic loop. Copied and adapted for Bridge.

**What moves:**
- `src/lib/ai/client.ts` → `platform/internal/llm/` (full LLM layer)
- `src/lib/ai/guardrails.ts` → `platform/internal/skills/tutor.go` (guardrail logic)
- `src/lib/ai/system-prompts.ts` → `platform/internal/skills/tutor.go` (prompt templates)
- `src/lib/ai/report-prompts.ts` → `platform/internal/skills/report_generator.go`
- `src/lib/ai/interactions.ts` → `platform/internal/store/interactions.go`
- `src/app/api/ai/chat/route.ts` → `platform/internal/handlers/ai.go`
- `src/app/api/ai/toggle/route.ts` → `platform/internal/handlers/ai.go`
- `src/app/api/ai/interactions/route.ts` → `platform/internal/handlers/ai.go`
- `src/app/api/parent/children/[id]/reports/route.ts` → `platform/internal/handlers/parent.go`
- `src/lib/parent-reports.ts` → `platform/internal/skills/report_generator.go`

**What's new:**
- Piston integration for server-side code execution (C++, Java, Rust)
- Code analyzer tool (AI analyzes student code patterns)
- Lesson generator tool (AIGC lesson content)
- Full agentic loop with multi-turn tool calling

---

## Prerequisites

- Go project foundation exists (`platform/cmd/api/main.go`, Chi router, auth middleware, DB connection, config) — assumed from Phase 1
- Phase 2 routes (sessions, documents, classrooms) are migrated — handlers referenced by AI endpoints exist
- PostgreSQL tables `ai_interactions` and `parent_reports` already exist (created by Drizzle migrations)

---

## Task 1: Copy LLM Layer from Magicburg

Copy the entire `internal/llm/` package from magicburg. Change the module import path from `github.com/weiboz0/bridge/platform` to `github.com/weiboz0/bridge/platform`.

### Files to create

All files under `platform/internal/llm/`:

#### 1a. `platform/internal/llm/types.go`

Copy verbatim from `/home/chris/workshop/magicburg-go/platform/internal/llm/types.go`. Only change: the `package` declaration stays `llm` (no import path change needed within the package).

```go
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
	RoleDirective MessageRole = "directive"
	RoleStatus    MessageRole = "status"
	RoleCommand   MessageRole = "command"
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

// Text extracts plain text from Content.
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

// contentToString converts any content value to a string.
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
```

#### 1b. `platform/internal/llm/backend.go`

Copy verbatim from magicburg. Contains `Backend` interface, `ChatOption` functional options, `resolveOpts`.

```go
package llm

import "context"

// ToolSpec describes a tool definition sent to the LLM.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Backend is the interface all LLM backends must implement.
type Backend interface {
	Name() string
	Chat(ctx context.Context, messages []Message, opts ...ChatOption) (*LLMResponse, error)
	StreamChat(ctx context.Context, messages []Message, opts ...ChatOption) (<-chan StreamChunk, error)
	ChatWithTools(ctx context.Context, messages []Message, tools []ToolSpec, opts ...ChatOption) (*LLMResponse, error)
	SupportsTools() bool
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

func WithTools(tools []map[string]any) ChatOption {
	return func(o *chatOpts) { o.Tools = tools }
}

func WithMaxTokens(n int) ChatOption {
	return func(o *chatOpts) { o.MaxTokens = n }
}

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

func resolveOpts(opts []ChatOption) chatOpts {
	var o chatOpts
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
```

#### 1c. `platform/internal/llm/openai.go`

Copy verbatim from magicburg (`/home/chris/workshop/magicburg-go/platform/internal/llm/openai.go`). All 506 lines. No changes needed — the file has no project-specific imports.

#### 1d. `platform/internal/llm/anthropic.go`

Copy verbatim from magicburg (`/home/chris/workshop/magicburg-go/platform/internal/llm/anthropic.go`). All 381 lines. No changes needed.

#### 1e. `platform/internal/llm/gemini.go`

Copy verbatim from magicburg (`/home/chris/workshop/magicburg-go/platform/internal/llm/gemini.go`). All 351 lines. No changes needed.

#### 1f. `platform/internal/llm/ollama.go`

Copy verbatim from magicburg (`/home/chris/workshop/magicburg-go/platform/internal/llm/ollama.go`). All 135 lines. No changes needed.

#### 1g. `platform/internal/llm/factory.go`

Copy verbatim from magicburg (`/home/chris/workshop/magicburg-go/platform/internal/llm/factory.go`). All 170 lines. No changes needed — the `CreateBackend` function, provider aliases, and model lists are project-agnostic.

#### 1h. `platform/internal/llm/agent_loop.go`

Copy verbatim from magicburg (`/home/chris/workshop/magicburg-go/platform/internal/llm/agent_loop.go`). All 91 lines. No changes needed.

### go.mod additions

Add to `platform/go.mod`:
```
github.com/sashabaranov/go-openai v1.41.2
```

---

## Task 2: Copy Tool Registry from Magicburg

Copy `internal/tools/` package. Change import path from `github.com/MagicBurg/magicburg/platform/internal/llm` to `github.com/bridge-edu/bridge/platform/internal/llm`.

### Files to create

#### 2a. `platform/internal/tools/contracts.go`

```go
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
```

#### 2b. `platform/internal/tools/registry.go`

```go
package tools

import (
	"context"
	"fmt"
	"sort"

	"github.com/bridge-edu/bridge/platform/internal/llm"
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

// ToolSpecs returns all registered tool specs (in llm.ToolSpec format for the agentic loop).
func (r *Registry) ToolSpecs() []llm.ToolSpec {
	specs := make([]llm.ToolSpec, 0, len(r.tools))
	for _, t := range r.tools {
		s := t.GetSpec()
		specs = append(specs, llm.ToolSpec{
			Name:        s.Name,
			Description: s.Description,
			Parameters:  s.Parameters,
		})
	}
	return specs
}

// Execute runs a tool call, looking it up by name in the registry.
func (r *Registry) Execute(ctx context.Context, tc llm.ToolCall, userID, sessionID string) (ToolResult, error) {
	t, ok := r.tools[tc.Name]
	if !ok {
		return ToolResult{
			ToolName: tc.Name,
			Status:   "error",
			Payload:  map[string]any{"error": fmt.Sprintf("unknown tool: %s", tc.Name)},
		}, fmt.Errorf("unknown tool: %s", tc.Name)
	}

	inv := ToolInvocation{
		ToolName:  tc.Name,
		UserID:    userID,
		Payload:   tc.Arguments,
		SessionID: sessionID,
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
```

---

## Task 3: Copy Events Broadcaster

Copy `internal/events/broadcaster.go` from magicburg, adapted for Bridge (no `agent_runs` table, simpler parent resolution).

### File to create

#### 3a. `platform/internal/events/broadcaster.go`

```go
// Package events provides per-session SSE event distribution using Go channels.
package events

import "sync"

// Event represents an SSE event to be broadcast to subscribers.
type Event struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// SessionBroadcaster distributes events to per-session subscriber channels.
type SessionBroadcaster struct {
	mu       sync.RWMutex
	channels map[string][]chan Event // session_id -> subscriber channels
	closed   bool
}

// NewBroadcaster creates a new SessionBroadcaster.
func NewBroadcaster() *SessionBroadcaster {
	return &SessionBroadcaster{
		channels: make(map[string][]chan Event),
	}
}

// Subscribe returns a buffered channel that receives events for the given session.
func (b *SessionBroadcaster) Subscribe(sessionID string) chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 100)
	b.channels[sessionID] = append(b.channels[sessionID], ch)
	return ch
}

// Unsubscribe removes a subscriber channel for the given session and closes it.
func (b *SessionBroadcaster) Unsubscribe(sessionID string, ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.channels[sessionID]
	for i, sub := range subs {
		if sub == ch {
			b.channels[sessionID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}
	if len(b.channels[sessionID]) == 0 {
		delete(b.channels, sessionID)
	}
}

// Broadcast sends an event to all subscribers of the given session.
func (b *SessionBroadcaster) Broadcast(sessionID, eventType string, data map[string]any) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return
	}
	defer b.mu.RUnlock()

	evt := Event{Type: eventType, Data: data}
	for _, ch := range b.channels[sessionID] {
		select {
		case ch <- evt:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- evt:
			default:
			}
		}
	}
}

// HasSubscribers returns true if the given session has at least one subscriber.
func (b *SessionBroadcaster) HasSubscribers(sessionID string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.channels[sessionID]) > 0
}

// Shutdown closes all subscriber channels.
func (b *SessionBroadcaster) Shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	for sessionID, subs := range b.channels {
		for _, ch := range subs {
			close(ch)
		}
		delete(b.channels, sessionID)
	}
}
```

---

## Task 4: Piston Sandbox Client

New file. Wraps the Piston HTTP API for sandboxed code execution.

### File to create

#### 4a. `platform/internal/sandbox/piston.go`

```go
// Package sandbox provides sandboxed code execution via the Piston API.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PistonConfig holds configuration for the Piston code execution service.
type PistonConfig struct {
	BaseURL          string        `json:"base_url" toml:"base_url"`     // default: http://piston:2000
	CompileTimeout   int           `json:"compile_timeout" toml:"compile_timeout"` // ms, default: 10000
	RunTimeout       int           `json:"run_timeout" toml:"run_timeout"`         // ms, default: 5000
	CompileMemLimit  int64         `json:"compile_mem_limit" toml:"compile_mem_limit"` // bytes, default: 256MB
	RunMemLimit      int64         `json:"run_mem_limit" toml:"run_mem_limit"`         // bytes, default: 256MB
	MaxOutputSize    int           `json:"max_output_size" toml:"max_output_size"`     // bytes, default: 65536
	MaxConcurrent    int           `json:"max_concurrent" toml:"max_concurrent"`       // default: 10
	HTTPTimeout      time.Duration `json:"-" toml:"-"`                                 // default: 30s
}

// DefaultPistonConfig returns configuration with sensible defaults.
func DefaultPistonConfig() PistonConfig {
	return PistonConfig{
		BaseURL:         "http://piston:2000",
		CompileTimeout:  10000,
		RunTimeout:      5000,
		CompileMemLimit: 256_000_000,
		RunMemLimit:     256_000_000,
		MaxOutputSize:   65536,
		MaxConcurrent:   10,
		HTTPTimeout:     30 * time.Second,
	}
}

// LanguageVersion maps language names to their Piston version strings.
var LanguageVersion = map[string]string{
	"python":     "3.10.0",
	"javascript": "18.15.0",
	"typescript": "5.0.3",
	"cpp":        "10.2.0",
	"c":          "10.2.0",
	"java":       "15.0.2",
	"rust":       "1.68.2",
	"go":         "1.16.2",
	"csharp":     "6.12.0",
	"ruby":       "3.0.1",
	"php":        "8.2.3",
}

// ExecuteRequest is the request body for Piston's /api/v2/execute endpoint.
type ExecuteRequest struct {
	Language        string         `json:"language"`
	Version         string         `json:"version"`
	Files           []ExecuteFile  `json:"files"`
	Stdin           string         `json:"stdin,omitempty"`
	Args            []string       `json:"args,omitempty"`
	CompileTimeout  int            `json:"compile_timeout,omitempty"`
	RunTimeout      int            `json:"run_timeout,omitempty"`
	CompileMemLimit int64          `json:"compile_memory_limit,omitempty"`
	RunMemLimit     int64          `json:"run_memory_limit,omitempty"`
}

// ExecuteFile represents a source file for Piston execution.
type ExecuteFile struct {
	Name    string `json:"name,omitempty"`
	Content string `json:"content"`
}

// ExecuteResponse is the response from Piston's /api/v2/execute endpoint.
type ExecuteResponse struct {
	Language string        `json:"language"`
	Version  string        `json:"version"`
	Run      ExecuteOutput `json:"run"`
	Compile  *ExecuteOutput `json:"compile,omitempty"`
}

// ExecuteOutput represents stdout/stderr/exit code from a run or compile step.
type ExecuteOutput struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Code   int    `json:"code"`
	Signal *string `json:"signal,omitempty"`
	Output string `json:"output"` // stdout + stderr combined
}

// Client wraps the Piston HTTP API.
type Client struct {
	config     PistonConfig
	httpClient *http.Client
	sem        chan struct{} // concurrency semaphore
}

// NewClient creates a new Piston client.
func NewClient(cfg PistonConfig) *Client {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 10
	}
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		config:     cfg,
		httpClient: &http.Client{Timeout: timeout},
		sem:        make(chan struct{}, cfg.MaxConcurrent),
	}
}

// Execute runs code in the Piston sandbox.
func (c *Client) Execute(ctx context.Context, language, code string) (*ExecuteResponse, error) {
	return c.ExecuteWithStdin(ctx, language, code, "")
}

// ExecuteWithStdin runs code with stdin input.
func (c *Client) ExecuteWithStdin(ctx context.Context, language, code, stdin string) (*ExecuteResponse, error) {
	version, ok := LanguageVersion[language]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", language)
	}

	req := ExecuteRequest{
		Language:        language,
		Version:         version,
		Files:           []ExecuteFile{{Content: code}},
		Stdin:           stdin,
		CompileTimeout:  c.config.CompileTimeout,
		RunTimeout:      c.config.RunTimeout,
		CompileMemLimit: c.config.CompileMemLimit,
		RunMemLimit:     c.config.RunMemLimit,
	}

	return c.executeRequest(ctx, req)
}

// ExecuteWithTests runs student code then appends test code and runs again.
func (c *Client) ExecuteWithTests(ctx context.Context, language, studentCode, testCode string) (*ExecuteResponse, error) {
	combined := studentCode + "\n\n" + testCode
	return c.Execute(ctx, language, combined)
}

// executeRequest sends the execution request to Piston.
func (c *Client) executeRequest(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	// Acquire concurrency slot
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.config.BaseURL + "/api/v2/execute"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("piston request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("piston HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	var result ExecuteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode piston response: %w", err)
	}

	// Truncate output if it exceeds the limit
	if c.config.MaxOutputSize > 0 {
		result.Run.Stdout = truncate(result.Run.Stdout, c.config.MaxOutputSize)
		result.Run.Stderr = truncate(result.Run.Stderr, c.config.MaxOutputSize)
		result.Run.Output = truncate(result.Run.Output, c.config.MaxOutputSize)
	}

	return &result, nil
}

// ListRuntimes returns available language runtimes from Piston.
func (c *Client) ListRuntimes(ctx context.Context) ([]RuntimeInfo, error) {
	url := c.config.BaseURL + "/api/v2/runtimes"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("piston runtimes request failed: %w", err)
	}
	defer resp.Body.Close()

	var runtimes []RuntimeInfo
	if err := json.NewDecoder(resp.Body).Decode(&runtimes); err != nil {
		return nil, fmt.Errorf("decode runtimes: %w", err)
	}
	return runtimes, nil
}

// RuntimeInfo describes an available Piston runtime.
type RuntimeInfo struct {
	Language string   `json:"language"`
	Version  string   `json:"version"`
	Aliases  []string `json:"aliases"`
}

// truncate shortens a string to maxLen bytes, appending "... [truncated]" if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... [truncated]"
}
```

---

## Task 5: Bridge-Specific AI Tools (Skills)

Five tools that implement the `tools.Tool` interface. Each is registered in the tool registry and can be invoked by the agentic loop.

### Files to create

#### 5a. `platform/internal/skills/code_runner.go`

Wraps Piston. Called by the agentic loop when the AI wants to execute student code or validate AI-generated code.

```go
// Package skills contains Bridge-specific AI tools.
package skills

import (
	"context"
	"fmt"

	"github.com/bridge-edu/bridge/platform/internal/sandbox"
	"github.com/bridge-edu/bridge/platform/internal/tools"
)

// CodeRunner executes code in Piston and returns stdout/stderr.
type CodeRunner struct {
	piston *sandbox.Client
}

// NewCodeRunner creates a CodeRunner tool.
func NewCodeRunner(piston *sandbox.Client) *CodeRunner {
	return &CodeRunner{piston: piston}
}

func (t *CodeRunner) GetName() string { return "code_runner" }

func (t *CodeRunner) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        "code_runner",
		Description: "Execute code in a sandboxed environment and return the output. Use this to run student code, test solutions, or validate AI-generated code. Supports Python, JavaScript, C++, Java, Rust, and more.",
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
		}, fmt.Errorf("missing language or code")
	}

	var resp *sandbox.ExecuteResponse
	var err error
	if stdin != "" {
		resp, err = t.piston.ExecuteWithStdin(ctx, language, code, stdin)
	} else {
		resp, err = t.piston.Execute(ctx, language, code)
	}
	if err != nil {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": err.Error()},
		}, nil // Return nil error so agentic loop continues with tool error in context
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
```

#### 5b. `platform/internal/skills/code_analyzer.go`

AI tool that analyzes student code patterns (not via LLM itself but by static analysis of common issues).

```go
package skills

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/bridge-edu/bridge/platform/internal/tools"
)

// CodeAnalyzer performs static analysis on student code to identify patterns,
// common mistakes, and style issues. Returns structured feedback for the AI tutor.
type CodeAnalyzer struct{}

// NewCodeAnalyzer creates a CodeAnalyzer tool.
func NewCodeAnalyzer() *CodeAnalyzer {
	return &CodeAnalyzer{}
}

func (t *CodeAnalyzer) GetName() string { return "code_analyzer" }

func (t *CodeAnalyzer) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        "code_analyzer",
		Description: "Analyze student code for common patterns, mistakes, and style issues. Returns structured feedback about the code without running it. Use this before giving hints to understand what the student has written.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"language": map[string]any{
					"type":        "string",
					"description": "Programming language of the code",
				},
				"code": map[string]any{
					"type":        "string",
					"description": "The student's source code to analyze",
				},
			},
			"required": []string{"language", "code"},
		},
	}
}

func (t *CodeAnalyzer) Invoke(ctx context.Context, inv tools.ToolInvocation) (tools.ToolResult, error) {
	language, _ := inv.Payload["language"].(string)
	code, _ := inv.Payload["code"].(string)

	if language == "" || code == "" {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": "language and code are required"},
		}, fmt.Errorf("missing language or code")
	}

	issues := analyzeCode(language, code)
	metrics := codeMetrics(code)

	return tools.ToolResult{
		ToolName: t.GetName(),
		Status:   "ok",
		Payload: map[string]any{
			"language":   language,
			"issues":     issues,
			"metrics":    metrics,
			"line_count": metrics["line_count"],
		},
	}, nil
}

type codeIssue struct {
	Type    string `json:"type"`    // "error", "warning", "style"
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}

// analyzeCode checks for common beginner mistakes per language.
func analyzeCode(language, code string) []codeIssue {
	var issues []codeIssue
	lines := strings.Split(code, "\n")

	switch language {
	case "python":
		issues = append(issues, analyzePython(lines)...)
	case "javascript", "typescript":
		issues = append(issues, analyzeJS(lines)...)
	case "cpp", "c":
		issues = append(issues, analyzeCpp(lines)...)
	case "java":
		issues = append(issues, analyzeJava(lines)...)
	}

	// Universal checks
	for i, line := range lines {
		if len(strings.TrimSpace(line)) > 120 {
			issues = append(issues, codeIssue{
				Type:    "style",
				Message: "Line is very long (>120 chars). Consider breaking it up.",
				Line:    i + 1,
			})
		}
	}

	return issues
}

func analyzePython(lines []string) []codeIssue {
	var issues []codeIssue
	tabMixed := false

	for i, line := range lines {
		// Mixed tabs and spaces
		if !tabMixed && strings.HasPrefix(line, "\t") {
			for _, other := range lines {
				if strings.HasPrefix(other, "    ") {
					issues = append(issues, codeIssue{
						Type:    "warning",
						Message: "Mixed tabs and spaces for indentation",
						Line:    i + 1,
					})
					tabMixed = true
					break
				}
			}
		}
		// Common mistake: = instead of == in if
		if matched, _ := regexp.MatchString(`^\s*if\s+.*[^=!<>]=[^=]`, line); matched {
			issues = append(issues, codeIssue{
				Type:    "warning",
				Message: "Possible assignment in if-condition (did you mean ==?)",
				Line:    i + 1,
			})
		}
		// Missing colon after def/if/for/while/class
		trimmed := strings.TrimSpace(line)
		for _, kw := range []string{"def ", "if ", "for ", "while ", "class ", "elif ", "else"} {
			if strings.HasPrefix(trimmed, kw) && !strings.HasSuffix(trimmed, ":") && !strings.HasSuffix(trimmed, ":\\") {
				issues = append(issues, codeIssue{
					Type:    "error",
					Message: fmt.Sprintf("Missing colon after '%s' statement", strings.TrimSpace(kw)),
					Line:    i + 1,
				})
				break
			}
		}
	}
	return issues
}

func analyzeJS(lines []string) []codeIssue {
	var issues []codeIssue
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// var usage
		if strings.HasPrefix(trimmed, "var ") {
			issues = append(issues, codeIssue{
				Type:    "style",
				Message: "Consider using 'let' or 'const' instead of 'var'",
				Line:    i + 1,
			})
		}
		// == instead of ===
		if matched, _ := regexp.MatchString(`[^=!]={2}[^=]`, line); matched {
			issues = append(issues, codeIssue{
				Type:    "warning",
				Message: "Using == instead of === (loose equality). Consider strict equality.",
				Line:    i + 1,
			})
		}
	}
	return issues
}

func analyzeCpp(lines []string) []codeIssue {
	var issues []codeIssue
	hasIncludeIOStream := false
	hasMain := false
	for _, line := range lines {
		if strings.Contains(line, "#include <iostream>") || strings.Contains(line, "#include<iostream>") {
			hasIncludeIOStream = true
		}
		if strings.Contains(line, "int main") {
			hasMain = true
		}
	}
	if !hasMain {
		issues = append(issues, codeIssue{
			Type:    "error",
			Message: "No main() function found — C++ programs need int main()",
		})
	}
	if !hasIncludeIOStream {
		for _, line := range lines {
			if strings.Contains(line, "cout") || strings.Contains(line, "cin") {
				issues = append(issues, codeIssue{
					Type:    "error",
					Message: "Using cout/cin without #include <iostream>",
				})
				break
			}
		}
	}
	return issues
}

func analyzeJava(lines []string) []codeIssue {
	var issues []codeIssue
	hasPublicClass := false
	for _, line := range lines {
		if strings.Contains(line, "public class") {
			hasPublicClass = true
		}
	}
	if !hasPublicClass {
		issues = append(issues, codeIssue{
			Type:    "warning",
			Message: "No public class found — Java files typically need a public class",
		})
	}
	return issues
}

// codeMetrics computes basic metrics about the code.
func codeMetrics(code string) map[string]any {
	lines := strings.Split(code, "\n")
	nonEmpty := 0
	commentLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		nonEmpty++
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			commentLines++
		}
	}
	return map[string]any{
		"line_count":    len(lines),
		"non_empty":     nonEmpty,
		"comment_lines": commentLines,
	}
}
```

#### 5c. `platform/internal/skills/tutor.go`

Socratic guardrails and grade-level system prompts. Migrates `src/lib/ai/guardrails.ts` and `src/lib/ai/system-prompts.ts`.

```go
package skills

import (
	"context"
	"fmt"
	"regexp"

	"github.com/bridge-edu/bridge/platform/internal/tools"
)

// GradeLevel represents the student's grade band.
type GradeLevel string

const (
	GradeK5  GradeLevel = "K-5"
	Grade68  GradeLevel = "6-8"
	Grade912 GradeLevel = "9-12"
)

// --- Guardrails (migrated from src/lib/ai/guardrails.ts) ---

var solutionPatterns = []*regexp.Regexp{
	regexp.MustCompile("(?s)```python\\n[\\s\\S]{200,}```"),        // Long code blocks (>200 chars)
	regexp.MustCompile(`(?s)def\s+\w+\s*\([^)]*\):[\s\S]{100,}`),  // Complete function definitions
	regexp.MustCompile(`(?s)class\s+\w+[\s\S]{150,}`),             // Complete class definitions
	regexp.MustCompile(`(?i)here(?:'s| is) the (?:complete |full )?(?:solution|answer|code)`),
	regexp.MustCompile(`(?i)just (?:copy|paste|use) this`),
}

// ContainsSolution checks if the LLM response gives away too much code.
func ContainsSolution(text string) bool {
	for _, p := range solutionPatterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

// FilterResponse replaces full-solution responses with a Socratic redirect.
func FilterResponse(text string) string {
	if ContainsSolution(text) {
		return "I was about to give you too much! Let me try again with a hint instead.\n\nWhat part of the problem are you finding most confusing? Let's break it down together."
	}
	return text
}

// --- System Prompts (migrated from src/lib/ai/system-prompts.ts) ---

const baseRules = `You are a patient coding tutor helping a student learn to program.

RULES:
- Ask guiding questions to help the student think through the problem
- Point to where the issue might be (e.g., "look at line 5"), but don't give the answer
- Never provide complete function implementations or full solutions
- If the student asks you to write the code for them, redirect them to think about the approach
- Celebrate small wins and encourage persistence
- Keep responses concise (2-4 sentences unless explaining a concept)`

var gradePrompts = map[GradeLevel]string{
	GradeK5: baseRules + `

GRADE LEVEL: Elementary (K-5)
- Use simple vocabulary and short sentences
- Use analogies from everyday life (building blocks, recipes, treasure maps)
- Be extra encouraging and patient
- Focus on visual thinking: "What do you see happening when you run this?"
- Reference block concepts if using Blockly: "Which purple block did you use?"`,

	Grade68: baseRules + `

GRADE LEVEL: Middle School (6-8)
- Explain concepts clearly but don't over-simplify
- Reference specific line numbers: "Take a look at line 7 — what value does x have there?"
- Use analogies when helpful but can be more technical
- Encourage reading error messages: "What does the error message tell you?"
- Help build debugging habits: "What did you expect to happen vs what actually happened?"`,

	Grade912: baseRules + `

GRADE LEVEL: High School (9-12)
- Use proper technical terminology
- Reference documentation and best practices
- Discuss trade-offs when relevant: "This works, but what happens if the list is empty?"
- Encourage independent problem-solving: "How would you test that this works?"
- Help develop computational thinking and code organization skills`,
}

// GetSystemPrompt returns the system prompt for the given grade level.
func GetSystemPrompt(grade GradeLevel) string {
	if prompt, ok := gradePrompts[grade]; ok {
		return prompt
	}
	return gradePrompts[Grade68] // default
}

// --- Tutor Tool (for agentic loop) ---

// Tutor is an AI tool that provides Socratic tutoring responses.
// It applies guardrails and grade-level prompts to generate pedagogically
// appropriate responses. Used within the agentic loop when the AI decides
// to formulate a tutoring response.
type Tutor struct{}

// NewTutor creates a Tutor tool.
func NewTutor() *Tutor { return &Tutor{} }

func (t *Tutor) GetName() string { return "tutor" }

func (t *Tutor) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        "tutor",
		Description: "Get the appropriate tutoring system prompt and guardrail rules for a student based on their grade level. Use this to understand how to respond to the student appropriately.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"grade_level": map[string]any{
					"type":        "string",
					"enum":        []string{"K-5", "6-8", "9-12"},
					"description": "The student's grade level band",
				},
				"proposed_response": map[string]any{
					"type":        "string",
					"description": "Optional: check if a proposed response violates guardrails",
				},
			},
			"required": []string{"grade_level"},
		},
	}
}

func (t *Tutor) Invoke(ctx context.Context, inv tools.ToolInvocation) (tools.ToolResult, error) {
	gradeLevelStr, _ := inv.Payload["grade_level"].(string)
	proposedResponse, _ := inv.Payload["proposed_response"].(string)

	gradeLevel := GradeLevel(gradeLevelStr)
	systemPrompt := GetSystemPrompt(gradeLevel)

	result := map[string]any{
		"system_prompt": systemPrompt,
		"grade_level":   string(gradeLevel),
	}

	if proposedResponse != "" {
		filtered := FilterResponse(proposedResponse)
		result["guardrail_triggered"] = filtered != proposedResponse
		if filtered != proposedResponse {
			result["filtered_response"] = filtered
			result["reason"] = "Response contained a complete solution. Socratic method requires guiding, not giving answers."
		}
	}

	return tools.ToolResult{
		ToolName: t.GetName(),
		Status:   "ok",
		Payload:  result,
	}, nil
}

// BuildChatSystemPrompt constructs the full system prompt for the AI chat handler.
// Includes grade-level rules and optionally the student's current code context.
func BuildChatSystemPrompt(gradeLevel GradeLevel, code string) string {
	prompt := GetSystemPrompt(gradeLevel)
	if code != "" {
		prompt += fmt.Sprintf("\n\nThe student's current code:\n```\n%s\n```", code)
	}
	return prompt
}
```

#### 5d. `platform/internal/skills/report_generator.go`

Migrates `src/lib/parent-reports.ts` and `src/lib/ai/report-prompts.ts`.

```go
package skills

import (
	"context"
	"fmt"
	"strings"

	"github.com/bridge-edu/bridge/platform/internal/llm"
	"github.com/bridge-edu/bridge/platform/internal/tools"
)

// ReportGenerator generates parent-facing progress reports using LLM.
type ReportGenerator struct {
	backend llm.Backend
}

// NewReportGenerator creates a ReportGenerator tool.
func NewReportGenerator(backend llm.Backend) *ReportGenerator {
	return &ReportGenerator{backend: backend}
}

func (t *ReportGenerator) GetName() string { return "report_generator" }

func (t *ReportGenerator) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        "report_generator",
		Description: "Generate a parent-facing progress report for a student. Summarizes attendance, topics covered, assignment grades, and AI tutor usage for a given time period.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"student_name": map[string]any{
					"type":        "string",
					"description": "The student's first name",
				},
				"period_start": map[string]any{
					"type":        "string",
					"description": "Start of the reporting period (e.g., '2026-01-01')",
				},
				"period_end": map[string]any{
					"type":        "string",
					"description": "End of the reporting period (e.g., '2026-01-07')",
				},
				"sessions_attended": map[string]any{
					"type":        "integer",
					"description": "Number of sessions the student attended",
				},
				"sessions_total": map[string]any{
					"type":        "integer",
					"description": "Total sessions available in the period",
				},
				"topics_covered": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "List of topic titles covered",
				},
				"assignment_grades": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title": map[string]any{"type": "string"},
							"grade": map[string]any{"type": "number"},
						},
					},
					"description": "List of assignment titles and grades (0-100 or null)",
				},
				"ai_interaction_count": map[string]any{
					"type":        "integer",
					"description": "Number of AI tutor conversations",
				},
				"annotation_count": map[string]any{
					"type":        "integer",
					"description": "Number of teacher code annotations",
				},
			},
			"required": []string{"student_name", "period_start", "period_end", "sessions_attended", "sessions_total"},
		},
	}
}

func (t *ReportGenerator) Invoke(ctx context.Context, inv tools.ToolInvocation) (tools.ToolResult, error) {
	studentName, _ := inv.Payload["student_name"].(string)
	periodStart, _ := inv.Payload["period_start"].(string)
	periodEnd, _ := inv.Payload["period_end"].(string)
	sessionsAttended := intFromPayload(inv.Payload, "sessions_attended")
	sessionsTotal := intFromPayload(inv.Payload, "sessions_total")
	topicsCovered := stringsFromPayload(inv.Payload, "topics_covered")
	aiInteractionCount := intFromPayload(inv.Payload, "ai_interaction_count")
	annotationCount := intFromPayload(inv.Payload, "annotation_count")

	// Build assignment grades string
	var gradeLines []string
	if grades, ok := inv.Payload["assignment_grades"].([]any); ok {
		for _, g := range grades {
			if gm, ok := g.(map[string]any); ok {
				title, _ := gm["title"].(string)
				grade, hasGrade := gm["grade"].(float64)
				if hasGrade {
					gradeLines = append(gradeLines, fmt.Sprintf("- %s: %.0f/100", title, grade))
				} else {
					gradeLines = append(gradeLines, fmt.Sprintf("- %s: not graded", title))
				}
			}
		}
	}
	gradeList := strings.Join(gradeLines, "\n")
	if gradeList == "" {
		gradeList = "No assignments yet"
	}

	topicsStr := "None recorded"
	if len(topicsCovered) > 0 {
		topicsStr = strings.Join(topicsCovered, ", ")
	}

	userPrompt := fmt.Sprintf(`Generate a progress report for %s.

Period: %s to %s

Attendance: %d of %d sessions

Topics covered: %s

Assignments:
%s

AI tutor interactions: %d conversations
Teacher annotations: %d comments on code`,
		studentName, periodStart, periodEnd,
		sessionsAttended, sessionsTotal,
		topicsStr, gradeList,
		aiInteractionCount, annotationCount)

	systemPrompt := getReportSystemPrompt()

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
		{Role: llm.RoleUser, Content: userPrompt},
	}

	resp, err := t.backend.Chat(ctx, messages, llm.WithMaxTokens(500))
	if err != nil {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": fmt.Sprintf("LLM call failed: %s", err.Error())},
		}, nil
	}

	return tools.ToolResult{
		ToolName: t.GetName(),
		Status:   "ok",
		Payload: map[string]any{
			"report_content": resp.Content,
			"model":          resp.Model,
		},
	}, nil
}

// getReportSystemPrompt returns the system prompt for parent report generation.
// Migrated from src/lib/ai/report-prompts.ts.
func getReportSystemPrompt() string {
	return `You are a helpful education assistant generating a weekly progress report for a parent.

Write in a warm, encouraging, parent-friendly tone. Use simple language.
Structure the report with these sections:
1. **Summary** — 2-3 sentence overview
2. **Attendance** — sessions attended, any missed
3. **Progress** — what topics were covered, what skills improved
4. **Areas for Growth** — gentle suggestions, not criticism
5. **Teacher's Notes** — if annotations or feedback available

Keep it concise — under 300 words. Use the student's first name.`
}

// --- Helper functions for payload extraction ---

func intFromPayload(p map[string]any, key string) int {
	switch v := p[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func stringsFromPayload(p map[string]any, key string) []string {
	arr, ok := p[key].([]any)
	if !ok {
		return nil
	}
	var result []string
	for _, v := range arr {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
```

#### 5e. `platform/internal/skills/lesson_generator.go`

AIGC lesson content generation tool.

```go
package skills

import (
	"context"
	"fmt"

	"github.com/bridge-edu/bridge/platform/internal/llm"
	"github.com/bridge-edu/bridge/platform/internal/tools"
)

// LessonGenerator generates lesson content using LLM.
type LessonGenerator struct {
	backend llm.Backend
}

// NewLessonGenerator creates a LessonGenerator tool.
func NewLessonGenerator(backend llm.Backend) *LessonGenerator {
	return &LessonGenerator{backend: backend}
}

func (t *LessonGenerator) GetName() string { return "lesson_generator" }

func (t *LessonGenerator) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        "lesson_generator",
		Description: "Generate lesson content for a coding topic. Creates an explanation, examples, and practice exercises appropriate for the student's grade level.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic": map[string]any{
					"type":        "string",
					"description": "The topic to create a lesson for (e.g., 'for loops', 'recursion', 'arrays')",
				},
				"language": map[string]any{
					"type":        "string",
					"description": "Programming language for code examples (python, javascript, cpp, etc.)",
				},
				"grade_level": map[string]any{
					"type":        "string",
					"enum":        []string{"K-5", "6-8", "9-12"},
					"description": "The student's grade level band",
				},
				"prerequisites": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Topics the student has already learned",
				},
			},
			"required": []string{"topic", "language", "grade_level"},
		},
	}
}

func (t *LessonGenerator) Invoke(ctx context.Context, inv tools.ToolInvocation) (tools.ToolResult, error) {
	topic, _ := inv.Payload["topic"].(string)
	language, _ := inv.Payload["language"].(string)
	gradeLevelStr, _ := inv.Payload["grade_level"].(string)
	prerequisites := stringsFromPayload(inv.Payload, "prerequisites")

	if topic == "" || language == "" {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": "topic and language are required"},
		}, fmt.Errorf("missing topic or language")
	}

	prereqStr := "None"
	if len(prerequisites) > 0 {
		prereqStr = fmt.Sprintf("%v", prerequisites)
	}

	systemPrompt := fmt.Sprintf(`You are an expert coding teacher creating lesson content for %s students.

Create a structured lesson with:
1. **Concept Explanation** — Clear explanation appropriate for the grade level
2. **Example Code** — 2-3 working code examples in %s, progressing in complexity
3. **Key Points** — 3-5 bullet points summarizing what to remember
4. **Practice Exercises** — 2-3 exercises for the student to try, with hints but NOT solutions

Format all code in fenced code blocks with the language tag.
Keep the tone encouraging and age-appropriate.`, gradeLevelStr, language)

	userPrompt := fmt.Sprintf(`Create a lesson on "%s" in %s.

Grade level: %s
Prerequisites the student knows: %s

Generate the complete lesson content.`, topic, language, gradeLevelStr, prereqStr)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
		{Role: llm.RoleUser, Content: userPrompt},
	}

	resp, err := t.backend.Chat(ctx, messages, llm.WithMaxTokens(2000))
	if err != nil {
		return tools.ToolResult{
			ToolName: t.GetName(),
			Status:   "error",
			Payload:  map[string]any{"error": fmt.Sprintf("LLM call failed: %s", err.Error())},
		}, nil
	}

	return tools.ToolResult{
		ToolName: t.GetName(),
		Status:   "ok",
		Payload: map[string]any{
			"lesson_content": resp.Content,
			"topic":          topic,
			"language":       language,
			"grade_level":    gradeLevelStr,
			"model":          resp.Model,
		},
	}, nil
}
```

---

## Task 6: Interaction Store (DB Layer)

Migrates `src/lib/ai/interactions.ts` to Go.

### File to create

#### 6a. `platform/internal/store/interactions.go`

```go
package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InteractionMessage represents a single message in an AI interaction.
type InteractionMessage struct {
	Role      string `json:"role"`      // "user" or "assistant"
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// Interaction represents a row in the ai_interactions table.
type Interaction struct {
	ID                 string               `json:"id"`
	StudentID          string               `json:"studentId"`
	SessionID          string               `json:"sessionId"`
	EnabledByTeacherID string               `json:"enabledByTeacherId"`
	Messages           []InteractionMessage `json:"messages"`
	CreatedAt          time.Time            `json:"createdAt"`
}

// InteractionStore provides DB operations for ai_interactions.
type InteractionStore struct {
	pool *pgxpool.Pool
}

// NewInteractionStore creates a new InteractionStore.
func NewInteractionStore(pool *pgxpool.Pool) *InteractionStore {
	return &InteractionStore{pool: pool}
}

// Create inserts a new AI interaction.
func (s *InteractionStore) Create(ctx context.Context, studentID, sessionID, teacherID string) (*Interaction, error) {
	var i Interaction
	var messagesJSON []byte

	err := s.pool.QueryRow(ctx,
		`INSERT INTO ai_interactions (student_id, session_id, enabled_by_teacher_id, messages)
		 VALUES ($1, $2, $3, '[]'::jsonb)
		 RETURNING id, student_id, session_id, enabled_by_teacher_id, messages, created_at`,
		studentID, sessionID, teacherID,
	).Scan(&i.ID, &i.StudentID, &i.SessionID, &i.EnabledByTeacherID, &messagesJSON, &i.CreatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(messagesJSON, &i.Messages)
	return &i, nil
}

// GetActive returns the active interaction for a student in a session.
func (s *InteractionStore) GetActive(ctx context.Context, studentID, sessionID string) (*Interaction, error) {
	var i Interaction
	var messagesJSON []byte

	err := s.pool.QueryRow(ctx,
		`SELECT id, student_id, session_id, enabled_by_teacher_id, messages, created_at
		 FROM ai_interactions
		 WHERE student_id = $1 AND session_id = $2
		 LIMIT 1`,
		studentID, sessionID,
	).Scan(&i.ID, &i.StudentID, &i.SessionID, &i.EnabledByTeacherID, &messagesJSON, &i.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(messagesJSON, &i.Messages)
	return &i, nil
}

// AppendMessage adds a message to an interaction's messages JSONB array.
func (s *InteractionStore) AppendMessage(ctx context.Context, interactionID string, msg InteractionMessage) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE ai_interactions
		 SET messages = messages || $1::jsonb
		 WHERE id = $2`,
		string(msgJSON), interactionID,
	)
	return err
}

// ListBySession returns all interactions for a session.
func (s *InteractionStore) ListBySession(ctx context.Context, sessionID string) ([]Interaction, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, student_id, session_id, enabled_by_teacher_id, messages, created_at
		 FROM ai_interactions
		 WHERE session_id = $1
		 ORDER BY created_at`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var interactions []Interaction
	for rows.Next() {
		var i Interaction
		var messagesJSON []byte
		if err := rows.Scan(&i.ID, &i.StudentID, &i.SessionID, &i.EnabledByTeacherID, &messagesJSON, &i.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(messagesJSON, &i.Messages)
		interactions = append(interactions, i)
	}
	return interactions, rows.Err()
}
```

---

## Task 7: Report Store (DB Layer)

Migrates the DB operations from `src/lib/parent-reports.ts`.

### File to create

#### 7a. `platform/internal/store/reports.go`

```go
package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReportSummary holds the structured summary data for a parent report.
type ReportSummary struct {
	SessionsAttended int      `json:"sessionsAttended"`
	SessionsTotal    int      `json:"sessionsTotal"`
	TopicsCovered    []string `json:"topicsCovered"`
	AIInteractions   int      `json:"aiInteractions"`
}

// ParentReport represents a row in the parent_reports table.
type ParentReport struct {
	ID          string        `json:"id"`
	StudentID   string        `json:"studentId"`
	GeneratedBy string        `json:"generatedBy"`
	PeriodStart time.Time     `json:"periodStart"`
	PeriodEnd   time.Time     `json:"periodEnd"`
	Content     string        `json:"content"`
	Summary     ReportSummary `json:"summary"`
	CreatedAt   time.Time     `json:"createdAt"`
}

// ReportStore provides DB operations for parent_reports.
type ReportStore struct {
	pool *pgxpool.Pool
}

// NewReportStore creates a new ReportStore.
func NewReportStore(pool *pgxpool.Pool) *ReportStore {
	return &ReportStore{pool: pool}
}

// Create inserts a new parent report.
func (s *ReportStore) Create(ctx context.Context, report ParentReport) (*ParentReport, error) {
	summaryJSON, err := json.Marshal(report.Summary)
	if err != nil {
		return nil, err
	}

	var r ParentReport
	var summaryBytes []byte

	err = s.pool.QueryRow(ctx,
		`INSERT INTO parent_reports (student_id, generated_by, period_start, period_end, content, summary)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb)
		 RETURNING id, student_id, generated_by, period_start, period_end, content, summary, created_at`,
		report.StudentID, report.GeneratedBy, report.PeriodStart, report.PeriodEnd,
		report.Content, string(summaryJSON),
	).Scan(&r.ID, &r.StudentID, &r.GeneratedBy, &r.PeriodStart, &r.PeriodEnd,
		&r.Content, &summaryBytes, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(summaryBytes, &r.Summary)
	return &r, nil
}

// ListByStudent returns all reports for a student, newest first.
func (s *ReportStore) ListByStudent(ctx context.Context, studentID string) ([]ParentReport, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, student_id, generated_by, period_start, period_end, content, summary, created_at
		 FROM parent_reports
		 WHERE student_id = $1
		 ORDER BY created_at DESC`,
		studentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []ParentReport
	for rows.Next() {
		var r ParentReport
		var summaryBytes []byte
		if err := rows.Scan(&r.ID, &r.StudentID, &r.GeneratedBy, &r.PeriodStart, &r.PeriodEnd,
			&r.Content, &summaryBytes, &r.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(summaryBytes, &r.Summary)
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

// GetByID returns a single report by its ID.
func (s *ReportStore) GetByID(ctx context.Context, reportID string) (*ParentReport, error) {
	var r ParentReport
	var summaryBytes []byte

	err := s.pool.QueryRow(ctx,
		`SELECT id, student_id, generated_by, period_start, period_end, content, summary, created_at
		 FROM parent_reports
		 WHERE id = $1`,
		reportID,
	).Scan(&r.ID, &r.StudentID, &r.GeneratedBy, &r.PeriodStart, &r.PeriodEnd,
		&r.Content, &summaryBytes, &r.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(summaryBytes, &r.Summary)
	return &r, nil
}

// GatherReportData queries the database for all data needed to generate a report.
// Returns the data as a map suitable for passing to the ReportGenerator tool.
func (s *ReportStore) GatherReportData(ctx context.Context, studentID string, periodStart, periodEnd time.Time) (map[string]any, error) {
	// Count sessions attended by the student in the period
	var sessionsAttended int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT sp.session_id)
		 FROM session_participants sp
		 JOIN live_sessions ls ON sp.session_id = ls.id
		 WHERE sp.student_id = $1
		   AND ls.started_at >= $2
		   AND ls.started_at <= $3`,
		studentID, periodStart, periodEnd,
	).Scan(&sessionsAttended)
	if err != nil {
		return nil, err
	}

	// Count total sessions in the period
	var sessionsTotal int
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM live_sessions
		 WHERE started_at >= $1 AND started_at <= $2`,
		periodStart, periodEnd,
	).Scan(&sessionsTotal)
	if err != nil {
		return nil, err
	}

	// Get topics covered
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT t.title
		 FROM session_topics st
		 JOIN topics t ON st.topic_id = t.id
		 JOIN session_participants sp ON st.session_id = sp.session_id
		 WHERE sp.student_id = $1
		   AND st.session_id IN (
		     SELECT ls.id FROM live_sessions ls
		     WHERE ls.started_at >= $2 AND ls.started_at <= $3
		   )`,
		studentID, periodStart, periodEnd,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return nil, err
		}
		topics = append(topics, title)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Count AI interactions
	var aiCount int
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM ai_interactions WHERE student_id = $1`,
		studentID,
	).Scan(&aiCount)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"sessions_attended":    sessionsAttended,
		"sessions_total":       sessionsTotal,
		"topics_covered":       topics,
		"ai_interaction_count": aiCount,
		"annotation_count":     0, // Annotations are per-document, not per-student
	}, nil
}
```

---

## Task 8: AI Chat Handler (Streaming SSE)

Migrates `src/app/api/ai/chat/route.ts`, `src/app/api/ai/toggle/route.ts`, and `src/app/api/ai/interactions/route.ts`.

### File to create

#### 8a. `platform/internal/handlers/ai.go`

```go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bridge-edu/bridge/platform/internal/events"
	"github.com/bridge-edu/bridge/platform/internal/llm"
	"github.com/bridge-edu/bridge/platform/internal/skills"
	"github.com/bridge-edu/bridge/platform/internal/store"
)

// AIHandler handles AI chat, toggle, and interaction endpoints.
type AIHandler struct {
	backend      llm.Backend
	interactions *store.InteractionStore
	sessions     *store.SessionStore    // assumed from Phase 2
	classrooms   *store.ClassroomStore  // assumed from Phase 2
	broadcaster  *events.SessionBroadcaster
}

// NewAIHandler creates a new AIHandler.
func NewAIHandler(
	backend llm.Backend,
	interactions *store.InteractionStore,
	sessions *store.SessionStore,
	classrooms *store.ClassroomStore,
	broadcaster *events.SessionBroadcaster,
) *AIHandler {
	return &AIHandler{
		backend:      backend,
		interactions: interactions,
		sessions:     sessions,
		classrooms:   classrooms,
		broadcaster:  broadcaster,
	}
}

// Routes registers AI routes on the router.
func (h *AIHandler) Routes(r chi.Router) {
	r.Post("/api/ai/chat", h.Chat)
	r.Post("/api/ai/toggle", h.Toggle)
	r.Get("/api/ai/interactions", h.ListInteractions)
}

// --- POST /api/ai/chat ---

type chatRequest struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	Code      string `json:"code"`
}

// Chat handles streaming AI chat requests.
// Migrated from src/app/api/ai/chat/route.ts.
func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r) // from auth middleware context
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.Message == "" {
		http.Error(w, "Missing sessionId or message", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Validate session is active
	liveSession, err := h.sessions.GetByID(ctx, req.SessionID)
	if err != nil || liveSession == nil || liveSession.Status != "active" {
		http.Error(w, "Session not found or ended", http.StatusNotFound)
		return
	}

	// Get classroom for grade level
	classroom, err := h.classrooms.GetByID(ctx, liveSession.ClassroomID)
	if err != nil || classroom == nil {
		http.Error(w, "Classroom not found", http.StatusNotFound)
		return
	}

	// Get or create interaction
	interaction, err := h.interactions.GetActive(ctx, userID, req.SessionID)
	if err != nil {
		slog.Error("get active interaction", "err", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if interaction == nil {
		interaction, err = h.interactions.Create(ctx, userID, req.SessionID, liveSession.TeacherID)
		if err != nil {
			slog.Error("create interaction", "err", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	}

	// Save user message
	err = h.interactions.AppendMessage(ctx, interaction.ID, store.InteractionMessage{
		Role:      "user",
		Content:   req.Message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		slog.Error("append user message", "err", err)
	}

	// Build conversation history
	var messages []llm.Message
	gradeLevel := skills.GradeLevel(classroom.GradeLevel)
	systemPrompt := skills.BuildChatSystemPrompt(gradeLevel, req.Code)
	messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: systemPrompt})

	for _, m := range interaction.Messages {
		role := llm.RoleUser
		if m.Role == "assistant" {
			role = llm.RoleAssistant
		}
		messages = append(messages, llm.Message{Role: role, Content: m.Content})
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: req.Message})

	// Set up SSE response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Stream the response
	streamCh, err := h.backend.StreamChat(ctx, messages, llm.WithMaxTokens(500))
	if err != nil {
		sseWrite(w, flusher, map[string]any{"error": err.Error()})
		sseWrite(w, flusher, "[DONE]")
		return
	}

	var fullResponse string
	for chunk := range streamCh {
		if chunk.IsFinal {
			break
		}
		if chunk.Delta != "" && chunk.ChunkType == "text" {
			fullResponse += chunk.Delta
			sseWrite(w, flusher, map[string]any{"text": chunk.Delta})
		}
	}

	// Apply guardrails
	filtered := skills.FilterResponse(fullResponse)
	if filtered != fullResponse {
		sseWrite(w, flusher, map[string]any{"replace": filtered})
		fullResponse = filtered
	}

	// Save assistant message
	err = h.interactions.AppendMessage(ctx, interaction.ID, store.InteractionMessage{
		Role:      "assistant",
		Content:   fullResponse,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		slog.Error("append assistant message", "err", err)
	}

	sseWrite(w, flusher, "[DONE]")
}

// --- POST /api/ai/toggle ---

type toggleRequest struct {
	SessionID string `json:"sessionId"`
	StudentID string `json:"studentId"`
	Enabled   bool   `json:"enabled"`
}

// Toggle enables/disables AI for a student in a session.
// Migrated from src/app/api/ai/toggle/route.ts.
func (h *AIHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req toggleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.StudentID == "" {
		http.Error(w, "Missing sessionId or studentId", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Verify teacher owns the session
	liveSession, err := h.sessions.GetByID(ctx, req.SessionID)
	if err != nil || liveSession == nil || liveSession.TeacherID != userID {
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
	}

	if req.Enabled {
		existing, _ := h.interactions.GetActive(ctx, req.StudentID, req.SessionID)
		if existing == nil {
			_, err := h.interactions.Create(ctx, req.StudentID, req.SessionID, userID)
			if err != nil {
				slog.Error("create interaction on toggle", "err", err)
				http.Error(w, "Internal error", http.StatusInternalServerError)
				return
			}
		}
	}

	// Broadcast SSE event
	h.broadcaster.Broadcast(req.SessionID, "ai_toggled", map[string]any{
		"studentId": req.StudentID,
		"enabled":   req.Enabled,
		"teacherId": userID,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"studentId": req.StudentID,
		"enabled":   req.Enabled,
	})
}

// --- GET /api/ai/interactions ---

// ListInteractions returns all AI interactions for a session.
// Migrated from src/app/api/ai/interactions/route.ts.
func (h *AIHandler) ListInteractions(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "sessionId required", http.StatusBadRequest)
		return
	}

	interactions, err := h.interactions.ListBySession(r.Context(), sessionID)
	if err != nil {
		slog.Error("list interactions", "err", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(interactions)
}

// --- SSE helpers ---

// sseWrite writes a single SSE data line and flushes.
func sseWrite(w http.ResponseWriter, flusher http.Flusher, data any) {
	switch v := data.(type) {
	case string:
		fmt.Fprintf(w, "data: %s\n\n", v)
	default:
		jsonBytes, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", string(jsonBytes))
	}
	flusher.Flush()
}

// getUserID extracts the user ID from the request context (set by auth middleware).
func getUserID(r *http.Request) string {
	if id, ok := r.Context().Value(contextKeyUserID).(string); ok {
		return id
	}
	return ""
}

// contextKeyUserID is the context key for the authenticated user ID.
type contextKey string

const contextKeyUserID contextKey = "userID"

// ContextWithUserID returns a new context with the user ID set.
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, contextKeyUserID, userID)
}
```

---

## Task 9: Parent Report Handler

Migrates `src/app/api/parent/children/[id]/reports/route.ts`.

### File to create

#### 9a. `platform/internal/handlers/parent.go`

```go
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bridge-edu/bridge/platform/internal/llm"
	"github.com/bridge-edu/bridge/platform/internal/skills"
	"github.com/bridge-edu/bridge/platform/internal/store"
	"github.com/bridge-edu/bridge/platform/internal/tools"
)

// ParentHandler handles parent portal API endpoints.
type ParentHandler struct {
	backend     llm.Backend
	reports     *store.ReportStore
	parentLinks *store.ParentLinkStore // assumed from Phase 2
	users       *store.UserStore       // assumed from Phase 2
}

// NewParentHandler creates a new ParentHandler.
func NewParentHandler(
	backend llm.Backend,
	reports *store.ReportStore,
	parentLinks *store.ParentLinkStore,
	users *store.UserStore,
) *ParentHandler {
	return &ParentHandler{
		backend:     backend,
		reports:     reports,
		parentLinks: parentLinks,
		users:       users,
	}
}

// Routes registers parent routes on the router.
func (h *ParentHandler) Routes(r chi.Router) {
	r.Get("/api/parent/children/{childId}/reports", h.ListReports)
	r.Post("/api/parent/children/{childId}/reports", h.GenerateReport)
}

// ListReports returns all reports for a child.
func (h *ParentHandler) ListReports(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	childID := chi.URLParam(r, "childId")

	// Verify parent-child link
	children, err := h.parentLinks.GetLinkedChildren(r.Context(), userID)
	if err != nil {
		slog.Error("get linked children", "err", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if !containsChild(children, childID) {
		http.Error(w, "Not linked to this child", http.StatusForbidden)
		return
	}

	reports, err := h.reports.ListByStudent(r.Context(), childID)
	if err != nil {
		slog.Error("list reports", "err", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reports)
}

// GenerateReport generates a new progress report for a child.
func (h *ParentHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	childID := chi.URLParam(r, "childId")

	ctx := r.Context()

	// Verify parent-child link
	children, err := h.parentLinks.GetLinkedChildren(ctx, userID)
	if err != nil {
		slog.Error("get linked children", "err", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if !containsChild(children, childID) {
		http.Error(w, "Not linked to this child", http.StatusForbidden)
		return
	}

	// Get child's name
	child, err := h.users.GetByID(ctx, childID)
	if err != nil || child == nil {
		http.Error(w, "Child not found", http.StatusNotFound)
		return
	}

	// Gather data for last 7 days
	now := time.Now().UTC()
	weekAgo := now.Add(-7 * 24 * time.Hour)

	reportData, err := h.reports.GatherReportData(ctx, childID, weekAgo, now)
	if err != nil {
		slog.Error("gather report data", "err", err)
		http.Error(w, "Failed to gather report data", http.StatusInternalServerError)
		return
	}

	// Use ReportGenerator tool to generate the report content
	generator := skills.NewReportGenerator(h.backend)
	payload := map[string]any{
		"student_name":         child.Name,
		"period_start":         weekAgo.Format("2006-01-02"),
		"period_end":           now.Format("2006-01-02"),
		"sessions_attended":    reportData["sessions_attended"],
		"sessions_total":       reportData["sessions_total"],
		"topics_covered":       reportData["topics_covered"],
		"ai_interaction_count": reportData["ai_interaction_count"],
		"annotation_count":     reportData["annotation_count"],
	}

	result, err := generator.Invoke(ctx, tools.ToolInvocation{
		ToolName: "report_generator",
		UserID:   userID,
		Payload:  payload,
	})
	if err != nil || result.Status == "error" {
		errMsg := "LLM generation failed"
		if err != nil {
			errMsg = err.Error()
		} else if e, ok := result.Payload["error"].(string); ok {
			errMsg = e
		}
		slog.Error("report generation", "err", errMsg)
		http.Error(w, "Failed to generate report", http.StatusInternalServerError)
		return
	}

	content, _ := result.Payload["report_content"].(string)

	// Build summary
	topicsCovered, _ := reportData["topics_covered"].([]string)
	sessionsAttended, _ := reportData["sessions_attended"].(int)
	sessionsTotal, _ := reportData["sessions_total"].(int)
	aiCount, _ := reportData["ai_interaction_count"].(int)

	// Save to DB
	report, err := h.reports.Create(ctx, store.ParentReport{
		StudentID:   childID,
		GeneratedBy: userID,
		PeriodStart: weekAgo,
		PeriodEnd:   now,
		Content:     content,
		Summary: store.ReportSummary{
			SessionsAttended: sessionsAttended,
			SessionsTotal:    sessionsTotal,
			TopicsCovered:    topicsCovered,
			AIInteractions:   aiCount,
		},
	})
	if err != nil {
		slog.Error("save report", "err", err)
		http.Error(w, "Failed to save report", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(report)
}

// containsChild checks if a child ID is in the linked children list.
func containsChild(children []store.LinkedChild, childID string) bool {
	for _, c := range children {
		if c.UserID == childID {
			return true
		}
	}
	return false
}
```

---

## Task 10: Tool Registry Wiring

Wire all Bridge tools into a registry, create the bridge-specific `ToolExecutor` for the agentic loop.

### File to create

#### 10a. `platform/internal/skills/registry.go`

```go
package skills

import (
	"context"
	"fmt"

	"github.com/bridge-edu/bridge/platform/internal/llm"
	"github.com/bridge-edu/bridge/platform/internal/sandbox"
	"github.com/bridge-edu/bridge/platform/internal/tools"
)

// NewBridgeRegistry creates a tool registry with all Bridge-specific tools registered.
func NewBridgeRegistry(backend llm.Backend, piston *sandbox.Client) *tools.Registry {
	reg := tools.NewRegistry()

	reg.Register(NewCodeRunner(piston))
	reg.Register(NewCodeAnalyzer())
	reg.Register(NewTutor())
	reg.Register(NewReportGenerator(backend))
	reg.Register(NewLessonGenerator(backend))

	return reg
}

// BridgeToolExecutor returns a ToolExecutor function for use with the agentic loop.
// It wraps the registry's Execute method, converting between the agentic loop's
// ToolCall/map format and the registry's ToolInvocation/ToolResult format.
func BridgeToolExecutor(registry *tools.Registry, userID, sessionID string) llm.ToolExecutor {
	return func(ctx context.Context, tc llm.ToolCall) (map[string]any, error) {
		result, err := registry.Execute(ctx, tc, userID, sessionID)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("tool execution failed: %s", err.Error())}, nil
		}
		return result.Payload, nil
	}
}
```

---

## Task 11: Piston Docker Setup

### File to create

#### 11a. `docker-compose.yml` (add Piston service)

Add the Piston service to the project's Docker Compose file (creating it if it doesn't exist, or adding the service if it does):

```yaml
services:
  piston:
    image: ghcr.io/engineer-man/piston:latest
    container_name: bridge-piston
    restart: unless-stopped
    ports:
      - "2000:2000"
    tmpfs:
      - /piston/jobs:exec,size=256M
    environment:
      - PISTON_RUN_TIMEOUT=5000
      - PISTON_COMPILE_TIMEOUT=10000
      - PISTON_OUTPUT_MAX_SIZE=65536
    volumes:
      - piston-packages:/piston/packages

volumes:
  piston-packages:
```

#### 11b. `scripts/piston-install-languages.sh`

Script to install language runtimes into Piston.

```bash
#!/usr/bin/env bash
# Install language runtimes into the Piston container.
# Run after 'docker compose up -d piston'.

set -euo pipefail

PISTON_URL="${PISTON_URL:-http://localhost:2000}"

install_pkg() {
  local lang=$1 version=$2
  echo "Installing $lang $version..."
  curl -s -X POST "$PISTON_URL/api/v2/packages" \
    -H "Content-Type: application/json" \
    -d "{\"language\": \"$lang\", \"version\": \"$version\"}" \
    | jq -r '.language + " " + .version + ": " + (.installed // "done")'
}

# Core languages (already supported via browser, now also server-side)
install_pkg python    3.10.0
install_pkg javascript 18.15.0
install_pkg typescript 5.0.3

# New compiled languages (Piston-only, not available in browser)
install_pkg cpp   10.2.0
install_pkg java  15.0.2
install_pkg rust  1.68.2

# Optional extras
install_pkg go    1.16.2
install_pkg csharp 6.12.0
install_pkg ruby  3.0.1

echo "All languages installed."
```

---

## Task 12: Configuration

### File to create / modify

#### 12a. `platform/config.toml` (add LLM and Piston sections)

Add to the existing `config.toml` (or create if it doesn't exist):

```toml
[llm]
backend = "anthropic"       # or "openai", "gemini", "ark", "aliyun", etc.
# model = ""                # override default model per provider
# base_url = ""             # override base URL
temperature = 0.7
max_tokens = 500
streaming = true

[piston]
base_url = "http://piston:2000"
compile_timeout = 10000
run_timeout = 5000
compile_mem_limit = 256000000
run_mem_limit = 256000000
max_output_size = 65536
max_concurrent = 10
```

#### 12b. `platform/internal/config/config.go` (extend)

Add to the existing config struct (assumed from Phase 1):

```go
// Add these fields to the existing Config struct:

type LLMConfig struct {
	Backend     string  `toml:"backend"`
	Model       string  `toml:"model"`
	BaseURL     string  `toml:"base_url"`
	Temperature float64 `toml:"temperature"`
	MaxTokens   int     `toml:"max_tokens"`
	Streaming   bool    `toml:"streaming"`
}

type PistonConfig struct {
	BaseURL         string `toml:"base_url"`
	CompileTimeout  int    `toml:"compile_timeout"`
	RunTimeout      int    `toml:"run_timeout"`
	CompileMemLimit int64  `toml:"compile_mem_limit"`
	RunMemLimit     int64  `toml:"run_mem_limit"`
	MaxOutputSize   int    `toml:"max_output_size"`
	MaxConcurrent   int    `toml:"max_concurrent"`
}

// In the main Config struct, add:
// LLM    LLMConfig    `toml:"llm"`
// Piston PistonConfig `toml:"piston"`
```

The LLM API key is read from environment variables (e.g., `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) matching the provider, NOT from the config file — following the "never hardcode secrets" rule from CLAUDE.md.

---

## Task 13: Server Startup Wiring

### File to modify

#### 13a. `platform/cmd/api/main.go` (extend)

Add initialization of the LLM backend, Piston client, tool registry, and new handlers to the existing `main.go` (assumed from Phase 1):

```go
// In the main() function, after existing initialization:

// --- LLM Backend ---
apiKeyEnvMap := map[string]string{
    "anthropic":  "ANTHROPIC_API_KEY",
    "openai":     "OPENAI_API_KEY",
    "gemini":     "GEMINI_API_KEY",
    "ark":        "ARK_API_KEY",
    "aliyun":     "DASHSCOPE_API_KEY",
    "nvidia":     "NVIDIA_API_KEY",
    "openrouter": "OPENROUTER_API_KEY",
}
llmCfg := llm.LLMConfig{
    Backend:     cfg.LLM.Backend,
    Model:       cfg.LLM.Model,
    BaseURL:     cfg.LLM.BaseURL,
    Temperature: cfg.LLM.Temperature,
    MaxTokens:   cfg.LLM.MaxTokens,
    Streaming:   cfg.LLM.Streaming,
}
// Resolve API key from environment
resolved, _ := llm.CreateBackend(llm.LLMConfig{Backend: llmCfg.Backend})
if envKey, ok := apiKeyEnvMap[resolved.Name()]; ok {
    llmCfg.APIKey = os.Getenv(envKey)
}
backend, err := llm.CreateBackend(llmCfg)
if err != nil {
    slog.Error("failed to create LLM backend", "err", err)
    os.Exit(1)
}
slog.Info("LLM backend ready", "provider", backend.Name())

// --- Piston Client ---
pistonCfg := sandbox.PistonConfig{
    BaseURL:         cfg.Piston.BaseURL,
    CompileTimeout:  cfg.Piston.CompileTimeout,
    RunTimeout:      cfg.Piston.RunTimeout,
    CompileMemLimit: cfg.Piston.CompileMemLimit,
    RunMemLimit:     cfg.Piston.RunMemLimit,
    MaxOutputSize:   cfg.Piston.MaxOutputSize,
    MaxConcurrent:   cfg.Piston.MaxConcurrent,
}
if pistonCfg.BaseURL == "" {
    pistonCfg = sandbox.DefaultPistonConfig()
}
pistonClient := sandbox.NewClient(pistonCfg)

// --- Tool Registry ---
toolRegistry := skills.NewBridgeRegistry(backend, pistonClient)
slog.Info("tool registry ready", "tools", toolRegistry.ListToolNames())

// --- Event Broadcaster ---
broadcaster := events.NewBroadcaster()

// --- Stores ---
interactionStore := store.NewInteractionStore(pool)
reportStore := store.NewReportStore(pool)

// --- Handlers ---
aiHandler := handlers.NewAIHandler(backend, interactionStore, sessionStore, classroomStore, broadcaster)
aiHandler.Routes(r)

parentHandler := handlers.NewParentHandler(backend, reportStore, parentLinkStore, userStore)
parentHandler.Routes(r)
```

---

## Task 14: Tests

### Files to create

#### 14a. `platform/tests/unit/llm/agent_loop_test.go`

Tests for the agentic loop.

```go
package llm_test

import (
	"context"
	"testing"

	"github.com/bridge-edu/bridge/platform/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBackend is a test double for llm.Backend.
type mockBackend struct {
	chatResponses         []*llm.LLMResponse
	chatWithToolsResponses []*llm.LLMResponse
	callIndex             int
}

func (m *mockBackend) Name() string { return "mock" }

func (m *mockBackend) Chat(ctx context.Context, messages []llm.Message, opts ...llm.ChatOption) (*llm.LLMResponse, error) {
	if m.callIndex < len(m.chatResponses) {
		resp := m.chatResponses[m.callIndex]
		m.callIndex++
		return resp, nil
	}
	return &llm.LLMResponse{Content: "fallback"}, nil
}

func (m *mockBackend) StreamChat(ctx context.Context, messages []llm.Message, opts ...llm.ChatOption) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockBackend) ChatWithTools(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec, opts ...llm.ChatOption) (*llm.LLMResponse, error) {
	if m.callIndex < len(m.chatWithToolsResponses) {
		resp := m.chatWithToolsResponses[m.callIndex]
		m.callIndex++
		return resp, nil
	}
	return &llm.LLMResponse{Content: "done"}, nil
}

func (m *mockBackend) SupportsTools() bool                           { return true }
func (m *mockBackend) ListModels(ctx context.Context) ([]string, error) { return nil, nil }

func TestAgenticLoop_NoTools_ReturnsDirectly(t *testing.T) {
	backend := &mockBackend{
		chatWithToolsResponses: []*llm.LLMResponse{
			{Content: "Hello, I can help you with that!"},
		},
	}

	cfg := llm.AgenticLoopConfig{
		MaxIterations: 5,
		Backend:       backend,
		Tools:         []llm.ToolSpec{{Name: "test", Description: "test"}},
		ToolExecutor: func(ctx context.Context, tc llm.ToolCall) (map[string]any, error) {
			return nil, nil
		},
	}

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Hello"},
	}

	content, toolCalls, err := llm.RunAgenticLoop(context.Background(), cfg, messages)
	require.NoError(t, err)
	assert.Equal(t, "Hello, I can help you with that!", content)
	assert.Equal(t, 0, toolCalls)
}

func TestAgenticLoop_WithToolCall(t *testing.T) {
	backend := &mockBackend{
		chatWithToolsResponses: []*llm.LLMResponse{
			// First call: LLM wants to use a tool
			{
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "code_runner", Arguments: map[string]any{
						"language": "python",
						"code":     "print('hi')",
					}},
				},
			},
			// Second call: LLM responds with text after seeing tool result
			{Content: "The code printed 'hi' successfully."},
		},
	}

	toolExecuted := false
	cfg := llm.AgenticLoopConfig{
		MaxIterations: 5,
		Backend:       backend,
		Tools:         []llm.ToolSpec{{Name: "code_runner", Description: "run code"}},
		ToolExecutor: func(ctx context.Context, tc llm.ToolCall) (map[string]any, error) {
			toolExecuted = true
			assert.Equal(t, "code_runner", tc.Name)
			return map[string]any{"stdout": "hi\n", "exit_code": 0}, nil
		},
	}

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Run print('hi')"},
	}

	content, toolCallCount, err := llm.RunAgenticLoop(context.Background(), cfg, messages)
	require.NoError(t, err)
	assert.True(t, toolExecuted)
	assert.Equal(t, 1, toolCallCount)
	assert.Equal(t, "The code printed 'hi' successfully.", content)
}

func TestAgenticLoop_MaxIterations(t *testing.T) {
	// Backend always returns tool calls, never text
	backend := &mockBackend{}
	for i := 0; i < 20; i++ {
		backend.chatWithToolsResponses = append(backend.chatWithToolsResponses, &llm.LLMResponse{
			ToolCalls: []llm.ToolCall{{ID: "call", Name: "test", Arguments: map[string]any{}}},
		})
	}
	// The last call (Chat without tools) returns text
	backend.chatResponses = []*llm.LLMResponse{{Content: "forced final"}}

	cfg := llm.AgenticLoopConfig{
		MaxIterations: 3,
		Backend:       backend,
		Tools:         []llm.ToolSpec{{Name: "test", Description: "test"}},
		ToolExecutor: func(ctx context.Context, tc llm.ToolCall) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		},
	}

	content, _, err := llm.RunAgenticLoop(context.Background(), cfg, []llm.Message{
		{Role: llm.RoleUser, Content: "loop forever"},
	})
	// On the last iteration, tools are disabled, so Chat is called, returning "forced final"
	// The mock's callIndex is shared, so we need to handle the sequence:
	// Iteration 0: ChatWithTools -> tool call (index 0)
	// Iteration 1: ChatWithTools -> tool call (index 1)
	// Iteration 2 (last): Chat -> "forced final" (chatResponses index 0)
	require.NoError(t, err)
	assert.Equal(t, "forced final", content)
}

func TestAgenticLoop_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	backend := &mockBackend{
		chatWithToolsResponses: []*llm.LLMResponse{
			{Content: "should not reach"},
		},
	}

	cfg := llm.AgenticLoopConfig{
		MaxIterations: 5,
		Backend:       backend,
		Tools:         []llm.ToolSpec{},
		ToolExecutor:  func(ctx context.Context, tc llm.ToolCall) (map[string]any, error) { return nil, nil },
	}

	_, _, err := llm.RunAgenticLoop(ctx, cfg, []llm.Message{{Role: llm.RoleUser, Content: "hi"}})
	assert.ErrorIs(t, err, context.Canceled)
}
```

#### 14b. `platform/tests/unit/tools/registry_test.go`

Tests for the tool registry.

```go
package tools_test

import (
	"context"
	"testing"

	"github.com/bridge-edu/bridge/platform/internal/llm"
	"github.com/bridge-edu/bridge/platform/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testTool is a minimal Tool implementation for testing.
type testTool struct {
	name string
}

func (t *testTool) GetName() string { return t.name }
func (t *testTool) GetSpec() tools.ToolSpec {
	return tools.ToolSpec{
		Name:        t.name,
		Description: "test tool",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
	}
}
func (t *testTool) Invoke(ctx context.Context, inv tools.ToolInvocation) (tools.ToolResult, error) {
	return tools.ToolResult{
		ToolName: t.name,
		Status:   "ok",
		Payload:  map[string]any{"echo": inv.Payload},
	}, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &testTool{name: "echo"}
	reg.Register(tool)

	got, ok := reg.Get("echo")
	require.True(t, ok)
	assert.Equal(t, "echo", got.GetName())

	_, ok = reg.Get("nonexistent")
	assert.False(t, ok)
}

func TestRegistry_ToolSpecs(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&testTool{name: "alpha"})
	reg.Register(&testTool{name: "beta"})

	specs := reg.ToolSpecs()
	assert.Len(t, specs, 2)
}

func TestRegistry_Execute(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&testTool{name: "echo"})

	result, err := reg.Execute(context.Background(), llm.ToolCall{
		Name:      "echo",
		Arguments: map[string]any{"msg": "hello"},
	}, "user-1", "session-1")
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	assert.Equal(t, map[string]any{"msg": "hello"}, result.Payload["echo"])
}

func TestRegistry_ExecuteUnknownTool(t *testing.T) {
	reg := tools.NewRegistry()

	_, err := reg.Execute(context.Background(), llm.ToolCall{
		Name: "nonexistent",
	}, "user-1", "session-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}

func TestRegistry_ListToolNames(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&testTool{name: "charlie"})
	reg.Register(&testTool{name: "alpha"})
	reg.Register(&testTool{name: "bravo"})

	names := reg.ListToolNames()
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, names) // sorted
}

func TestRegistry_Copy(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&testTool{name: "original"})

	cp := reg.Copy()
	cp.Register(&testTool{name: "added"})

	_, ok := reg.Get("added")
	assert.False(t, ok, "adding to copy should not affect original")

	_, ok = cp.Get("original")
	assert.True(t, ok, "copy should have original tools")
}
```

#### 14c. `platform/tests/unit/skills/guardrails_test.go`

Tests for guardrails and system prompts.

```go
package skills_test

import (
	"testing"

	"github.com/bridge-edu/bridge/platform/internal/skills"
	"github.com/stretchr/testify/assert"
)

func TestContainsSolution_LongCodeBlock(t *testing.T) {
	// >200 char code block should be flagged
	longCode := "```python\n" + string(make([]byte, 250)) + "\n```"
	assert.True(t, skills.ContainsSolution(longCode))
}

func TestContainsSolution_ShortCodeBlock(t *testing.T) {
	shortCode := "```python\nprint('hello')\n```"
	assert.False(t, skills.ContainsSolution(shortCode))
}

func TestContainsSolution_CompleteFunctionDef(t *testing.T) {
	code := "def solve(n):\n    result = []\n    for i in range(n):\n        result.append(i * 2)\n    return result\n\n# This is a complete solution with over 100 chars of implementation"
	assert.True(t, skills.ContainsSolution(code))
}

func TestContainsSolution_HereIsTheSolution(t *testing.T) {
	text := "Here is the complete solution for your problem."
	assert.True(t, skills.ContainsSolution(text))
}

func TestContainsSolution_JustCopyThis(t *testing.T) {
	text := "Just copy this into your editor and you're done!"
	assert.True(t, skills.ContainsSolution(text))
}

func TestContainsSolution_SafeHint(t *testing.T) {
	text := "Think about what happens when x is 0. What does your loop do in that case?"
	assert.False(t, skills.ContainsSolution(text))
}

func TestFilterResponse_TriggersGuardrail(t *testing.T) {
	bad := "Here is the complete solution:\ndef solve(): pass"
	filtered := skills.FilterResponse(bad)
	assert.NotEqual(t, bad, filtered)
	assert.Contains(t, filtered, "hint instead")
}

func TestFilterResponse_PassesThrough(t *testing.T) {
	good := "Try looking at line 5. What value does x have there?"
	assert.Equal(t, good, skills.FilterResponse(good))
}

func TestGetSystemPrompt_ValidGrades(t *testing.T) {
	for _, grade := range []skills.GradeLevel{skills.GradeK5, skills.Grade68, skills.Grade912} {
		prompt := skills.GetSystemPrompt(grade)
		assert.Contains(t, prompt, "coding tutor")
		assert.NotEmpty(t, prompt)
	}
}

func TestGetSystemPrompt_K5HasSimpleVocab(t *testing.T) {
	prompt := skills.GetSystemPrompt(skills.GradeK5)
	assert.Contains(t, prompt, "simple vocabulary")
	assert.Contains(t, prompt, "Elementary")
}

func TestGetSystemPrompt_912HasTechnical(t *testing.T) {
	prompt := skills.GetSystemPrompt(skills.Grade912)
	assert.Contains(t, prompt, "technical terminology")
	assert.Contains(t, prompt, "High School")
}

func TestGetSystemPrompt_InvalidGradeDefaultsTo68(t *testing.T) {
	prompt := skills.GetSystemPrompt("invalid")
	expected := skills.GetSystemPrompt(skills.Grade68)
	assert.Equal(t, expected, prompt)
}

func TestBuildChatSystemPrompt_WithCode(t *testing.T) {
	prompt := skills.BuildChatSystemPrompt(skills.Grade68, "x = 5\nprint(x)")
	assert.Contains(t, prompt, "coding tutor")
	assert.Contains(t, prompt, "x = 5")
}

func TestBuildChatSystemPrompt_WithoutCode(t *testing.T) {
	prompt := skills.BuildChatSystemPrompt(skills.Grade68, "")
	assert.Contains(t, prompt, "coding tutor")
	assert.NotContains(t, prompt, "student's current code")
}
```

#### 14d. `platform/tests/unit/skills/code_analyzer_test.go`

Tests for the code analyzer.

```go
package skills_test

import (
	"context"
	"testing"

	"github.com/bridge-edu/bridge/platform/internal/skills"
	"github.com/bridge-edu/bridge/platform/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodeAnalyzer_PythonMissingColon(t *testing.T) {
	analyzer := skills.NewCodeAnalyzer()
	result, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"language": "python",
			"code":     "def hello()\n    print('hi')",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)

	issues, ok := result.Payload["issues"].([]skills.CodeIssue)
	// The type will be unexported — check via the map instead
	_ = ok
	_ = issues

	// Verify the payload has issues
	issuesRaw := result.Payload["issues"]
	assert.NotNil(t, issuesRaw)
}

func TestCodeAnalyzer_JSVarUsage(t *testing.T) {
	analyzer := skills.NewCodeAnalyzer()
	result, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"language": "javascript",
			"code":     "var x = 5;\nconsole.log(x);",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
}

func TestCodeAnalyzer_CppNoMain(t *testing.T) {
	analyzer := skills.NewCodeAnalyzer()
	result, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"language": "cpp",
			"code":     "#include <iostream>\nstd::cout << \"hello\";",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
}

func TestCodeAnalyzer_MissingFields(t *testing.T) {
	analyzer := skills.NewCodeAnalyzer()
	_, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{},
	})
	assert.Error(t, err)
}

func TestCodeAnalyzer_Metrics(t *testing.T) {
	analyzer := skills.NewCodeAnalyzer()
	result, err := analyzer.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"language": "python",
			"code":     "# comment\nx = 5\n\nprint(x)",
		},
	})
	require.NoError(t, err)

	metrics, ok := result.Payload["metrics"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 4, metrics["line_count"])
}
```

#### 14e. `platform/tests/unit/sandbox/piston_test.go`

Tests for the Piston client (unit tests with HTTP mock).

```go
package sandbox_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bridge-edu/bridge/platform/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPiston(handler http.HandlerFunc) (*sandbox.Client, *httptest.Server) {
	server := httptest.NewServer(handler)
	cfg := sandbox.DefaultPistonConfig()
	cfg.BaseURL = server.URL
	return sandbox.NewClient(cfg), server
}

func TestPiston_ExecutePython(t *testing.T) {
	client, server := newTestPiston(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/execute", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var req sandbox.ExecuteRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "python", req.Language)
		assert.Equal(t, "3.10.0", req.Version)

		json.NewEncoder(w).Encode(sandbox.ExecuteResponse{
			Language: "python",
			Version:  "3.10.0",
			Run: sandbox.ExecuteOutput{
				Stdout: "hello\n",
				Stderr: "",
				Code:   0,
				Output: "hello\n",
			},
		})
	})
	defer server.Close()

	resp, err := client.Execute(context.Background(), "python", "print('hello')")
	require.NoError(t, err)
	assert.Equal(t, "hello\n", resp.Run.Stdout)
	assert.Equal(t, 0, resp.Run.Code)
}

func TestPiston_ExecuteWithStdin(t *testing.T) {
	client, server := newTestPiston(func(w http.ResponseWriter, r *http.Request) {
		var req sandbox.ExecuteRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "test input", req.Stdin)

		json.NewEncoder(w).Encode(sandbox.ExecuteResponse{
			Language: "python",
			Run:      sandbox.ExecuteOutput{Stdout: "test input\n", Code: 0},
		})
	})
	defer server.Close()

	resp, err := client.ExecuteWithStdin(context.Background(), "python", "x = input()\nprint(x)", "test input")
	require.NoError(t, err)
	assert.Equal(t, "test input\n", resp.Run.Stdout)
}

func TestPiston_UnsupportedLanguage(t *testing.T) {
	cfg := sandbox.DefaultPistonConfig()
	client := sandbox.NewClient(cfg)

	_, err := client.Execute(context.Background(), "brainfuck", "++++++++++")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported language")
}

func TestPiston_HTTPError(t *testing.T) {
	client, server := newTestPiston(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	})
	defer server.Close()

	_, err := client.Execute(context.Background(), "python", "print('hi')")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "piston HTTP 500")
}

func TestPiston_CompilationError(t *testing.T) {
	client, server := newTestPiston(func(w http.ResponseWriter, r *http.Request) {
		stderr := "error: expected ';' before '}' token"
		json.NewEncoder(w).Encode(sandbox.ExecuteResponse{
			Language: "cpp",
			Compile: &sandbox.ExecuteOutput{
				Stdout: "",
				Stderr: stderr,
				Code:   1,
			},
			Run: sandbox.ExecuteOutput{Code: 1, Stderr: stderr},
		})
	})
	defer server.Close()

	resp, err := client.Execute(context.Background(), "cpp", "int main() { return 0 }")
	require.NoError(t, err) // HTTP request succeeded
	assert.Equal(t, 1, resp.Compile.Code)
	assert.Contains(t, resp.Compile.Stderr, "expected ';'")
}

func TestPiston_OutputTruncation(t *testing.T) {
	longOutput := make([]byte, 100000)
	for i := range longOutput {
		longOutput[i] = 'x'
	}

	client, server := newTestPiston(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sandbox.ExecuteResponse{
			Language: "python",
			Run: sandbox.ExecuteOutput{
				Stdout: string(longOutput),
				Code:   0,
			},
		})
	})
	defer server.Close()

	resp, err := client.Execute(context.Background(), "python", "print('x'*100000)")
	require.NoError(t, err)
	assert.Less(t, len(resp.Run.Stdout), 100000)
	assert.Contains(t, resp.Run.Stdout, "[truncated]")
}

func TestPiston_ListRuntimes(t *testing.T) {
	client, server := newTestPiston(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/runtimes" {
			json.NewEncoder(w).Encode([]sandbox.RuntimeInfo{
				{Language: "python", Version: "3.10.0", Aliases: []string{"py"}},
				{Language: "cpp", Version: "10.2.0", Aliases: []string{"c++"}},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	runtimes, err := client.ListRuntimes(context.Background())
	require.NoError(t, err)
	assert.Len(t, runtimes, 2)
	assert.Equal(t, "python", runtimes[0].Language)
}

func TestPiston_ContextCancellation(t *testing.T) {
	client, server := newTestPiston(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response — the context should be cancelled before we respond
		<-r.Context().Done()
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Execute(ctx, "python", "print('hi')")
	assert.Error(t, err)
}
```

#### 14f. `platform/tests/unit/events/broadcaster_test.go`

Tests for the SSE broadcaster.

```go
package events_test

import (
	"testing"
	"time"

	"github.com/bridge-edu/bridge/platform/internal/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcaster_SubscribeAndBroadcast(t *testing.T) {
	b := events.NewBroadcaster()
	ch := b.Subscribe("session-1")

	b.Broadcast("session-1", "test_event", map[string]any{"key": "value"})

	select {
	case evt := <-ch:
		assert.Equal(t, "test_event", evt.Type)
		assert.Equal(t, "value", evt.Data["key"])
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	b.Unsubscribe("session-1", ch)
}

func TestBroadcaster_NoSubscribers(t *testing.T) {
	b := events.NewBroadcaster()
	// Should not panic
	b.Broadcast("session-1", "test", map[string]any{})
}

func TestBroadcaster_MultipleSubscribers(t *testing.T) {
	b := events.NewBroadcaster()
	ch1 := b.Subscribe("session-1")
	ch2 := b.Subscribe("session-1")

	b.Broadcast("session-1", "test", map[string]any{"n": 1})

	evt1 := <-ch1
	evt2 := <-ch2
	assert.Equal(t, "test", evt1.Type)
	assert.Equal(t, "test", evt2.Type)

	b.Unsubscribe("session-1", ch1)
	b.Unsubscribe("session-1", ch2)
}

func TestBroadcaster_IsolatedSessions(t *testing.T) {
	b := events.NewBroadcaster()
	ch1 := b.Subscribe("session-1")
	ch2 := b.Subscribe("session-2")

	b.Broadcast("session-1", "event1", map[string]any{})

	select {
	case <-ch1:
		// Expected
	case <-time.After(time.Second):
		t.Fatal("session-1 should receive event")
	}

	select {
	case <-ch2:
		t.Fatal("session-2 should NOT receive session-1 event")
	case <-time.After(100 * time.Millisecond):
		// Expected
	}

	b.Unsubscribe("session-1", ch1)
	b.Unsubscribe("session-2", ch2)
}

func TestBroadcaster_HasSubscribers(t *testing.T) {
	b := events.NewBroadcaster()
	assert.False(t, b.HasSubscribers("session-1"))

	ch := b.Subscribe("session-1")
	assert.True(t, b.HasSubscribers("session-1"))

	b.Unsubscribe("session-1", ch)
	assert.False(t, b.HasSubscribers("session-1"))
}

func TestBroadcaster_Shutdown(t *testing.T) {
	b := events.NewBroadcaster()
	ch := b.Subscribe("session-1")

	b.Shutdown()

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok)

	// Broadcast after shutdown should not panic
	b.Broadcast("session-1", "test", map[string]any{})
}

func TestBroadcaster_UnsubscribeCleansUp(t *testing.T) {
	b := events.NewBroadcaster()
	ch := b.Subscribe("session-1")
	require.True(t, b.HasSubscribers("session-1"))

	b.Unsubscribe("session-1", ch)
	assert.False(t, b.HasSubscribers("session-1"))
}
```

#### 14g. `platform/tests/unit/skills/tutor_tool_test.go`

Tests for the tutor tool invocation.

```go
package skills_test

import (
	"context"
	"testing"

	"github.com/bridge-edu/bridge/platform/internal/skills"
	"github.com/bridge-edu/bridge/platform/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTutor_GetSpec(t *testing.T) {
	tutor := skills.NewTutor()
	spec := tutor.GetSpec()
	assert.Equal(t, "tutor", spec.Name)
	assert.NotEmpty(t, spec.Description)
	assert.NotNil(t, spec.Parameters)
}

func TestTutor_InvokeReturnsPrompt(t *testing.T) {
	tutor := skills.NewTutor()
	result, err := tutor.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"grade_level": "K-5",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	prompt, ok := result.Payload["system_prompt"].(string)
	require.True(t, ok)
	assert.Contains(t, prompt, "Elementary")
}

func TestTutor_GuardrailCheckClean(t *testing.T) {
	tutor := skills.NewTutor()
	result, err := tutor.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"grade_level":       "6-8",
			"proposed_response": "Look at line 5. What do you think x equals there?",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, false, result.Payload["guardrail_triggered"])
}

func TestTutor_GuardrailCheckTriggered(t *testing.T) {
	tutor := skills.NewTutor()
	result, err := tutor.Invoke(context.Background(), tools.ToolInvocation{
		Payload: map[string]any{
			"grade_level":       "9-12",
			"proposed_response": "Here is the complete solution for your problem: def solve()...",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, true, result.Payload["guardrail_triggered"])
	assert.NotEmpty(t, result.Payload["filtered_response"])
	assert.NotEmpty(t, result.Payload["reason"])
}
```

#### 14h. `platform/tests/contract/ai_chat_test.go`

Contract test comparing Go AI chat endpoint with Next.js. This validates response format compatibility.

```go
package contract_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests compare Go backend responses with Next.js responses.
// They require both servers to be running:
//   NEXTJS_URL=http://localhost:3003  GO_URL=http://localhost:8001
//
// Run with: go test ./tests/contract/ -run TestContract -v

func getURLs(t *testing.T) (string, string) {
	nextjs := os.Getenv("NEXTJS_URL")
	goURL := os.Getenv("GO_URL")
	if nextjs == "" || goURL == "" {
		t.Skip("NEXTJS_URL and GO_URL must be set for contract tests")
	}
	return nextjs, goURL
}

func TestContract_AIToggle(t *testing.T) {
	nextjs, goURL := getURLs(t)
	authToken := os.Getenv("TEST_AUTH_TOKEN")
	if authToken == "" {
		t.Skip("TEST_AUTH_TOKEN required")
	}

	body := map[string]any{
		"sessionId": os.Getenv("TEST_SESSION_ID"),
		"studentId": os.Getenv("TEST_STUDENT_ID"),
		"enabled":   true,
	}
	bodyJSON, _ := json.Marshal(body)

	// Request to Next.js
	reqNext, _ := http.NewRequest("POST", nextjs+"/api/ai/toggle", bytes.NewReader(bodyJSON))
	reqNext.Header.Set("Content-Type", "application/json")
	reqNext.Header.Set("Authorization", "Bearer "+authToken)

	// Request to Go
	reqGo, _ := http.NewRequest("POST", goURL+"/api/ai/toggle", bytes.NewReader(bodyJSON))
	reqGo.Header.Set("Content-Type", "application/json")
	reqGo.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{Timeout: 10 * time.Second}
	respNext, err := client.Do(reqNext)
	require.NoError(t, err)
	defer respNext.Body.Close()

	respGo, err := client.Do(reqGo)
	require.NoError(t, err)
	defer respGo.Body.Close()

	// Compare status codes
	assert.Equal(t, respNext.StatusCode, respGo.StatusCode,
		"Status codes should match: Next.js=%d Go=%d", respNext.StatusCode, respGo.StatusCode)

	// Compare response shape
	var nextBody, goBody map[string]any
	json.NewDecoder(respNext.Body).Decode(&nextBody)
	json.NewDecoder(respGo.Body).Decode(&goBody)

	// Both should have studentId and enabled fields
	assert.Equal(t, nextBody["studentId"], goBody["studentId"])
	assert.Equal(t, nextBody["enabled"], goBody["enabled"])
}

func TestContract_AIInteractions(t *testing.T) {
	nextjs, goURL := getURLs(t)
	authToken := os.Getenv("TEST_AUTH_TOKEN")
	sessionID := os.Getenv("TEST_SESSION_ID")
	if authToken == "" || sessionID == "" {
		t.Skip("TEST_AUTH_TOKEN and TEST_SESSION_ID required")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	reqNext, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/ai/interactions?sessionId=%s", nextjs, sessionID), nil)
	reqNext.Header.Set("Authorization", "Bearer "+authToken)

	reqGo, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/ai/interactions?sessionId=%s", goURL, sessionID), nil)
	reqGo.Header.Set("Authorization", "Bearer "+authToken)

	respNext, _ := client.Do(reqNext)
	respGo, _ := client.Do(reqGo)
	if respNext != nil {
		defer respNext.Body.Close()
	}
	if respGo != nil {
		defer respGo.Body.Close()
	}

	if respNext != nil && respGo != nil {
		assert.Equal(t, respNext.StatusCode, respGo.StatusCode)
	}
}

func TestContract_AIChatSSEFormat(t *testing.T) {
	_, goURL := getURLs(t)
	authToken := os.Getenv("TEST_AUTH_TOKEN")
	sessionID := os.Getenv("TEST_SESSION_ID")
	if authToken == "" || sessionID == "" {
		t.Skip("TEST_AUTH_TOKEN and TEST_SESSION_ID required")
	}

	body := map[string]any{
		"sessionId": sessionID,
		"message":   "What is a variable?",
	}
	bodyJSON, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", goURL+"/api/ai/chat", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// Read SSE events and validate format
	scanner := bufio.NewScanner(resp.Body)
	var events []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, strings.TrimPrefix(line, "data: "))
		}
	}

	require.NotEmpty(t, events, "should receive at least one SSE event")

	// Last event should be [DONE]
	assert.Equal(t, "[DONE]", events[len(events)-1])

	// Earlier events should be valid JSON with "text" or "replace" keys
	for _, evt := range events[:len(events)-1] {
		var parsed map[string]any
		err := json.Unmarshal([]byte(evt), &parsed)
		require.NoError(t, err, "SSE event should be valid JSON: %s", evt)
		_, hasText := parsed["text"]
		_, hasReplace := parsed["replace"]
		_, hasError := parsed["error"]
		assert.True(t, hasText || hasReplace || hasError,
			"SSE event should have 'text', 'replace', or 'error' key: %s", evt)
	}
}
```

---

## Task 15: Export `codeIssue` Type for Tests

The `codeIssue` type in `code_analyzer.go` is lowercase (unexported). Either export it or change the test to not depend on the concrete type.

### Modification

In `platform/internal/skills/code_analyzer.go`, change `codeIssue` to `CodeIssue`:

```go
// Change:
// type codeIssue struct {
// To:
type CodeIssue struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}
```

Update all references within the file (`codeIssue` -> `CodeIssue`), and update function return types: `analyzeCode` returns `[]CodeIssue`, etc.

---

## Execution Order

1. **Task 1** — Copy LLM layer (types, backend, openai, anthropic, gemini, ollama, factory, agent_loop)
2. **Task 2** — Copy tool registry (contracts, registry)
3. **Task 3** — Copy events broadcaster
4. **Task 4** — Create Piston sandbox client
5. **Task 5** — Create Bridge-specific skills (code_runner, code_analyzer, tutor, report_generator, lesson_generator)
6. **Task 6** — Create interaction store
7. **Task 7** — Create report store
8. **Task 8** — Create AI chat handler
9. **Task 9** — Create parent report handler
10. **Task 10** — Wire tool registry
11. **Task 11** — Piston Docker setup
12. **Task 12** — Configuration additions
13. **Task 13** — Server startup wiring
14. **Task 14** — All tests
15. **Task 15** — Export `CodeIssue` type

Tasks 1-4 are independent and can be parallelized. Tasks 5-7 depend on 1-4. Tasks 8-10 depend on 5-7. Tasks 11-12 are independent of code tasks. Task 14 runs after everything else.

---

## Files Summary

### New files (30 total)

| # | File | Source |
|---|------|--------|
| 1 | `platform/internal/llm/types.go` | Copy from magicburg |
| 2 | `platform/internal/llm/backend.go` | Copy from magicburg |
| 3 | `platform/internal/llm/openai.go` | Copy from magicburg |
| 4 | `platform/internal/llm/anthropic.go` | Copy from magicburg |
| 5 | `platform/internal/llm/gemini.go` | Copy from magicburg |
| 6 | `platform/internal/llm/ollama.go` | Copy from magicburg |
| 7 | `platform/internal/llm/factory.go` | Copy from magicburg |
| 8 | `platform/internal/llm/agent_loop.go` | Copy from magicburg |
| 9 | `platform/internal/tools/contracts.go` | Copy from magicburg (path fix) |
| 10 | `platform/internal/tools/registry.go` | Copy from magicburg (path fix) |
| 11 | `platform/internal/events/broadcaster.go` | Adapted from magicburg |
| 12 | `platform/internal/sandbox/piston.go` | New |
| 13 | `platform/internal/skills/code_runner.go` | New |
| 14 | `platform/internal/skills/code_analyzer.go` | New |
| 15 | `platform/internal/skills/tutor.go` | New (migrates guardrails.ts + system-prompts.ts) |
| 16 | `platform/internal/skills/report_generator.go` | New (migrates parent-reports.ts + report-prompts.ts) |
| 17 | `platform/internal/skills/lesson_generator.go` | New |
| 18 | `platform/internal/skills/registry.go` | New |
| 19 | `platform/internal/store/interactions.go` | New (migrates interactions.ts) |
| 20 | `platform/internal/store/reports.go` | New (migrates parent-reports.ts DB ops) |
| 21 | `platform/internal/handlers/ai.go` | New (migrates 3 Next.js routes) |
| 22 | `platform/internal/handlers/parent.go` | New (migrates parent report route) |
| 23 | `docker-compose.yml` | New / extend |
| 24 | `scripts/piston-install-languages.sh` | New |
| 25 | `platform/tests/unit/llm/agent_loop_test.go` | New |
| 26 | `platform/tests/unit/tools/registry_test.go` | New |
| 27 | `platform/tests/unit/skills/guardrails_test.go` | New |
| 28 | `platform/tests/unit/skills/code_analyzer_test.go` | New |
| 29 | `platform/tests/unit/skills/tutor_tool_test.go` | New |
| 30 | `platform/tests/unit/sandbox/piston_test.go` | New |
| 31 | `platform/tests/unit/events/broadcaster_test.go` | New |
| 32 | `platform/tests/contract/ai_chat_test.go` | New |

### Modified files

| File | Change |
|------|--------|
| `platform/go.mod` | Add `go-openai` dependency |
| `platform/config.toml` | Add `[llm]` and `[piston]` sections |
| `platform/internal/config/config.go` | Add LLM and Piston config structs |
| `platform/cmd/api/main.go` | Wire LLM, Piston, tools, handlers |

---

## Verification Checklist

- [ ] `go build ./...` compiles cleanly
- [ ] `go test ./tests/unit/...` passes all unit tests
- [ ] `go vet ./...` reports no issues
- [ ] Piston container starts and responds to health check
- [ ] `scripts/piston-install-languages.sh` installs Python, JS, C++, Java, Rust
- [ ] `POST /api/ai/chat` streams SSE with `text` chunks and `[DONE]`
- [ ] Guardrails filter triggers on full-solution responses
- [ ] `POST /api/ai/toggle` broadcasts SSE event
- [ ] `GET /api/ai/interactions` returns interaction list
- [ ] `POST /api/parent/children/{id}/reports` generates and saves a report
- [ ] `GET /api/parent/children/{id}/reports` lists saved reports
- [ ] Contract tests pass (when both servers are running)

---

## Phase A — Post-Execution Report

**Branch:** `feat/019a-llm-foundation`
**PR:** #23
**Executed:** 2026-04-13

### What was done

- Synced `llm/` (8 files) and `tools/` (2 files) from magicburg-go
- Updated import paths to `github.com/weiboz0/bridge/platform`
- Added `sashabaranov/go-openai` dependency
- Updated config to resolve API keys from provider-specific env vars
- 53 new tests (LLM types + agentic loop + factory + tool registry + config)

### Code Review — Phase A

#### Review 1

- **Date**: 2026-04-13
- **Reviewer**: Claude (superpowers:code-reviewer)
- **PR**: #23
- **Verdict**: Changes requested (2 critical, 6 important)

**Must Fix**

1. `[FIXED]` Gemini tool result messages have empty function name. `platform/internal/llm/agent_loop.go:82`
   → Set `Name: tc.Name` on tool result messages. Commit 7a622d3.

2. `[FIXED]` `LLMConfig.APIKey` has `json:"api_key"` — key leakage risk. `platform/internal/llm/types.go:73`
   → Changed to `json:"-" toml:"-"`. Commit 7a622d3.

**Should Fix**

3. `[FIXED]` No HTTP client timeouts on Anthropic and Gemini. → Added 5-min timeout. Commit 7a622d3.

4. `[FIXED]` `ToolInvocation.TenantID` magicburg artifact. → Removed. Commit 7a622d3.

5. `[WONTFIX]` Duplicate `ToolSpec` type in llm and tools packages. → Intentional, avoids circular imports.

6. `[WONTFIX]` `Message.Text()` only handles `[]map[string]any`. → Works in practice; fix when needed.

7. `[FIXED]` Missing factory and config tests. → Added 18 tests. Commit 7a622d3.

**Nice to Have**

8. `[FIXED]` Anthropic StreamChat doesn't surface usage info for cost tracking.
   → Fixed: Extract input_tokens from message_start, output_tokens from message_delta. Final StreamChunk includes Usage.

9. `[FIXED]` Gemini uses tool name as ToolCall ID — could collide for duplicate tool calls.
   → Fixed: Generate synthetic unique IDs with `fmt.Sprintf("%s_%d", name, index)`.

---

## Phase B — Post-Execution Report

**Branch:** `feat/019b-stores-chat-handler`
**PR:** #25
**Executed:** 2026-04-13

### What was done

- InteractionStore: create, get, get active, list by session, atomic append message
- ReportStore: create, get, list by student
- AI chat handler: streaming SSE with incremental guardrails, grade-level prompts
- AI toggle handler: teacher enables/disables AI per student
- AI interactions list: scoped to session teacher
- Parent report handler: list + create (LLM generation deferred to Phase C)
- Server wiring: shared broadcaster, LLM backend from config, conditional AI routes

### Code Review — Phase B

#### Review 1

- **Date**: 2026-04-13
- **Reviewer**: Claude (superpowers:code-reviewer)
- **PR**: #25
- **Verdict**: Changes requested (2 critical, 6 important)

**Must Fix**

1. `[FIXED]` Race condition in AppendMessage — read-modify-write on JSONB.
   → Fixed: Use atomic `messages || jsonb_build_array($1::jsonb)`. Commit bcc5efb.

2. `[FIXED]` Guardrails applied post-streaming — client already received solution.
   → Fixed: Incremental guardrail checks during streaming, early termination on detection. Commit bcc5efb.

**Should Fix**

3. `[FIXED]` Chat handler auto-creates interactions — bypasses teacher toggle.
   → Fixed: Require existing interaction, return 403 if not enabled. Commit bcc5efb.

4. `[FIXED]` ListInteractions has no authorization scoping.
   → Fixed: Verify caller is session teacher or platform admin. Commit bcc5efb.

5. `[WONTFIX]` ParentHandler has no parent-child link verification.
   → Deferred: parent_links table doesn't exist yet. Added TODO comment.

6. `[FIXED]` Stale history — used pre-append interaction for LLM context.
   → Fixed: Use returned interaction from atomic AppendMessage. Commit bcc5efb.

7. `[FIXED]` json.Unmarshal errors silently ignored.
   → Fixed: Return error on parse failure. Commit bcc5efb.

8. `[FIXED]` Error from saving assistant message silently ignored.
   → Fixed: Capture error (logged, don't fail streamed response). Commit bcc5efb.

**Nice to Have**

9. `[FIXED]` No ChunkType/IsFinal filtering in stream loop.
   → Fixed: Filter on `chunk.ChunkType == "text"` and break on `chunk.IsFinal`. Commit bcc5efb.

10. `[FIXED]` No message length validation on user input.
   → Fixed: Added 5000 char limit on chat messages. Commit f381b51.

11. `[FIXED]` Code language hardcoded to python.
   → Fixed: Use classroom.EditorMode. Commit bcc5efb.

12. `[WONTFIX]` Missing handler-level tests for AI/parent endpoints.
   → Deferred: store integration tests cover core logic; will add handler tests in test pass.

13. `[WONTFIX]` Missing GatherReportData for LLM report generation.
   → Deferred to Phase C (AI Skills).

14. `[FIXED]` Toggle handler disable is cosmetic — no database state change.
   → Fixed: Disable now deletes interaction record; Chat handler returns 403. Commit f381b51.

---

## Phase C — Post-Execution Report

**Branch:** `feat/019c-ai-skills`
**PR:** #26
**Executed:** 2026-04-13

### What was done

- 5 AI skills: tutor, code_analyzer, code_runner, report_generator, lesson_generator
- Skills registry (`NewBridgeRegistry`) for centralized tool registration
- Refactored AI handler to use `skills.*` instead of inline guardrails/prompts
- 22 tests for tutor + code analyzer + code runner

### Code Review — Phase C

#### Review 1

- **Date**: 2026-04-13
- **Reviewer**: Claude (superpowers:code-reviewer)
- **PR**: #26
- **Verdict**: Changes requested (0 critical, 4 important)

**Should Fix**

1. `[WONTFIX]` Report Generator simplified from plan (missing period_start/end, assignment_grades, etc.).
   → Intentional: full parameters depend on GatherReportData which is deferred. Will enrich when data pipeline exists.

2. `[WONTFIX]` Lesson Generator simplified from plan (missing prerequisites parameter).
   → Intentional: will add when curriculum sequencing is implemented.

3. `[FIXED]` Missing `skills/registry.go` — no centralized skill registration.
   → Added `NewBridgeRegistry(executor, backend)`. Commit pending.

4. `[FIXED]` Inconsistent error returns — validation failures returned Go errors in some skills but nil in others.
   → Aligned: all tool-level failures return nil Go error with status "error" in ToolResult.

**Nice to Have**

5. `[OPEN]` Guardrail patterns only detect Python code blocks — should expand to JS/C++/Java fences.

6. `[OPEN]` No tests for ReportGenerator or LessonGenerator (need mock LLM backend).

7. `[OPEN]` `TestCodeRunner_NilExecutor` is in `code_analyzer_test.go` — should be its own file.
