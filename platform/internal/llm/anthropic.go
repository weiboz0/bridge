package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var anthropicModels = []string{
	"claude-opus-4-6",
	"claude-sonnet-4-6",
	"claude-haiku-4-5-20251001",
	"claude-3-5-sonnet-20241022",
	"claude-3-5-haiku-20241022",
	"claude-3-opus-20240229",
}

const anthropicDefaultModel = "claude-sonnet-4-6"
const anthropicAPIVersion = "2023-06-01"
const anthropicDefaultURL = "https://api.anthropic.com"

// AnthropicBackend implements Backend for the Anthropic Messages API.
type AnthropicBackend struct {
	config       LLMConfig
	providerName string
	httpClient   *http.Client
}

// NewAnthropicBackend creates a new Anthropic backend.
func NewAnthropicBackend(cfg LLMConfig) *AnthropicBackend {
	return &AnthropicBackend{
		config:       cfg,
		providerName: "anthropic",
		httpClient:   &http.Client{Timeout: 5 * time.Minute},
	}
}

func (b *AnthropicBackend) Name() string { return b.providerName }

func (b *AnthropicBackend) model() string {
	if b.config.Model != "" {
		return b.config.Model
	}
	return anthropicDefaultModel
}

func (b *AnthropicBackend) baseURL() string {
	if b.config.BaseURL != "" {
		return strings.TrimRight(b.config.BaseURL, "/")
	}
	return anthropicDefaultURL
}

func (b *AnthropicBackend) SupportsTools() bool { return true }

func (b *AnthropicBackend) ListModels(_ context.Context) ([]string, error) {
	return anthropicModels, nil
}

// splitMessages separates system prompt from conversation and converts to
// Anthropic message format. System messages become the top-level system param;
// tool results become user messages with tool_result content blocks.
func (b *AnthropicBackend) splitMessages(messages []Message) (string, []map[string]any) {
	var system string
	var conversation []map[string]any

	for _, m := range messages {
		switch m.Role {
		case RoleSystem, RoleDirective:
			system = contentToString(m.Content)

		case RoleTool:
			block := map[string]any{
				"type":        "tool_result",
				"tool_use_id": m.ToolCallID,
				"content":     contentToString(m.Content),
			}
			// Merge into previous user message if it has content blocks
			if len(conversation) > 0 {
				last := conversation[len(conversation)-1]
				if last["role"] == "user" {
					if blocks, ok := last["content"].([]map[string]any); ok {
						last["content"] = append(blocks, block)
						continue
					}
				}
			}
			conversation = append(conversation, map[string]any{
				"role":    "user",
				"content": []map[string]any{block},
			})

		case RoleAssistant:
			// Check if content is already structured content blocks
			blocks := tryParseContentBlocks(m.Content)
			if blocks != nil {
				conversation = append(conversation, map[string]any{
					"role":    "assistant",
					"content": blocks,
				})
			} else {
				conversation = append(conversation, map[string]any{
					"role":    "assistant",
					"content": contentToString(m.Content),
				})
			}

		case RoleUser:
			// Pass content through — may be string or list of content blocks
			switch c := m.Content.(type) {
			case []ContentBlock:
				conversation = append(conversation, map[string]any{
					"role":    "user",
					"content": c,
				})
			default:
				conversation = append(conversation, map[string]any{
					"role":    "user",
					"content": contentToString(m.Content),
				})
			}

		default:
			// Skip status/command messages
		}
	}
	return system, conversation
}

// buildRequestBody constructs the JSON body for the Anthropic Messages API.
func (b *AnthropicBackend) buildRequestBody(messages []Message, stream bool, opts chatOpts) map[string]any {
	system, conversation := b.splitMessages(messages)

	maxTokens := b.config.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 65536
	}
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}

	body := map[string]any{
		"model":      b.model(),
		"max_tokens": maxTokens,
		"messages":   conversation,
	}
	if system != "" {
		body["system"] = system
	}
	if stream {
		body["stream"] = true
	}
	if b.config.Temperature != 0 {
		body["temperature"] = b.config.Temperature
	}
	if len(opts.Tools) > 0 {
		body["tools"] = opts.Tools
	}

	// Merge extra params
	for k, v := range b.config.ExtraParams {
		body[k] = v
	}
	for k, v := range opts.ExtraBody {
		body[k] = v
	}

	// Don't set temperature when thinking is enabled
	if _, hasThinking := body["thinking"]; hasThinking {
		delete(body, "temperature")
	}

	return body
}

// doRequest sends an HTTP request to the Anthropic API.
func (b *AnthropicBackend) doRequest(ctx context.Context, body map[string]any) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := b.baseURL() + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", b.config.APIKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	return resp, nil
}

// parseResponseBody converts an Anthropic API response to LLMResponse.
func parseAnthropicResponse(data map[string]any) *LLMResponse {
	var textParts []string
	var thinkingParts []string
	var toolCalls []ToolCall

	content, _ := data["content"].([]any)
	for _, block := range content {
		b, ok := block.(map[string]any)
		if !ok {
			continue
		}
		switch b["type"] {
		case "thinking":
			if t, ok := b["thinking"].(string); ok {
				thinkingParts = append(thinkingParts, t)
			}
		case "text":
			if t, ok := b["text"].(string); ok {
				textParts = append(textParts, t)
			}
		case "tool_use":
			id, _ := b["id"].(string)
			name, _ := b["name"].(string)
			args := make(map[string]any)
			if input, ok := b["input"].(map[string]any); ok {
				args = input
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:        id,
				Name:      name,
				Arguments: args,
			})
		}
	}

	var usage *UsageInfo
	if u, ok := data["usage"].(map[string]any); ok {
		prompt := intFromAny(u["input_tokens"])
		completion := intFromAny(u["output_tokens"])
		usage = &UsageInfo{
			PromptTokens:     prompt,
			CompletionTokens: completion,
			TotalTokens:      prompt + completion,
		}
	}

	model, _ := data["model"].(string)
	stopReason, _ := data["stop_reason"].(string)

	return &LLMResponse{
		Content:      strings.Join(textParts, ""),
		Model:        model,
		Usage:        usage,
		FinishReason: stopReason,
		ToolCalls:    toolCalls,
		Thinking:     strings.Join(thinkingParts, ""),
	}
}

// intFromAny extracts an int from a JSON number (which may be float64).
func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

// Chat sends a non-streaming chat request.
func (b *AnthropicBackend) Chat(ctx context.Context, messages []Message, opts ...ChatOption) (*LLMResponse, error) {
	o := resolveOpts(opts)
	body := b.buildRequestBody(messages, false, o)

	resp, err := b.doRequest(ctx, body)
	if err != nil {
		return nil, wrapError(b.providerName, err, b.model())
	}
	defer resp.Body.Close()

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return parseAnthropicResponse(data), nil
}

// StreamChat sends a streaming chat request and returns chunks via channel.
func (b *AnthropicBackend) StreamChat(ctx context.Context, messages []Message, opts ...ChatOption) (<-chan StreamChunk, error) {
	o := resolveOpts(opts)
	body := b.buildRequestBody(messages, true, o)

	resp, err := b.doRequest(ctx, body)
	if err != nil {
		return nil, wrapError(b.providerName, err, b.model())
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		stopReason := "end_turn"
		var totalUsage UsageInfo

		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			eventType, _ := event["type"].(string)
			switch eventType {
			case "content_block_delta":
				delta, _ := event["delta"].(map[string]any)
				deltaType, _ := delta["type"].(string)
				switch deltaType {
				case "thinking_delta":
					if t, ok := delta["thinking"].(string); ok && t != "" {
						ch <- StreamChunk{Delta: t, ChunkType: "thinking"}
					}
				case "text_delta":
					if t, ok := delta["text"].(string); ok && t != "" {
						ch <- StreamChunk{Delta: t, ChunkType: "text"}
					}
				}
			case "message_delta":
				if d, ok := event["delta"].(map[string]any); ok {
					if sr, ok := d["stop_reason"].(string); ok && sr != "" {
						stopReason = sr
					}
				}
				// Extract usage from message_delta event
				if u, ok := event["usage"].(map[string]any); ok {
					outTokens, _ := u["output_tokens"].(float64)
					if outTokens > 0 {
						totalUsage.CompletionTokens = int(outTokens)
						totalUsage.TotalTokens = totalUsage.PromptTokens + totalUsage.CompletionTokens
					}
				}
			case "message_start":
				if msg, ok := event["message"].(map[string]any); ok {
					if u, ok := msg["usage"].(map[string]any); ok {
						inputTokens, _ := u["input_tokens"].(float64)
						if inputTokens > 0 {
							totalUsage.PromptTokens = int(inputTokens)
						}
					}
				}
			}
		}
		ch <- StreamChunk{IsFinal: true, FinishReason: stopReason, Usage: &totalUsage}
	}()

	return ch, nil
}

// ChatWithTools sends a chat request with Anthropic-native tool definitions.
func (b *AnthropicBackend) ChatWithTools(ctx context.Context, messages []Message, tools []ToolSpec, opts ...ChatOption) (*LLMResponse, error) {
	var anthropicTools []map[string]any
	for _, t := range tools {
		anthropicTools = append(anthropicTools, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.Parameters,
		})
	}
	opts = append(opts, WithTools(anthropicTools))
	return b.Chat(ctx, messages, opts...)
}
