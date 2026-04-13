package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

var openaiModels = []string{
	"gpt-4o",
	"gpt-4o-mini",
	"gpt-4-turbo",
	"gpt-4",
	"gpt-3.5-turbo",
	"o1",
	"o1-mini",
	"o3-mini",
}

const openaiDefaultModel = "gpt-4o-mini"

// Models that don't support native function calling (reasoning-only).
var noToolsPatterns = []string{"deepseek-r1", "qwq"}

// thinkRE matches <think>...</think> blocks.
var thinkRE = regexp.MustCompile(`(?s)<think>(.*?)</think>`)

// OpenAIBackend implements Backend for OpenAI and OpenAI-compatible APIs.
type OpenAIBackend struct {
	config       LLMConfig
	client       *openai.Client
	providerName string
	defaultModel string
	models       []string
}

// NewOpenAIBackend creates a new OpenAI-compatible backend.
func NewOpenAIBackend(cfg LLMConfig, providerName, defaultModel string, models []string) *OpenAIBackend {
	ocfg := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		ocfg.BaseURL = cfg.BaseURL
	}
	return &OpenAIBackend{
		config:       cfg,
		client:       openai.NewClientWithConfig(ocfg),
		providerName: providerName,
		defaultModel: defaultModel,
		models:       models,
	}
}

func (b *OpenAIBackend) Name() string { return b.providerName }

func (b *OpenAIBackend) model() string {
	if b.config.Model != "" {
		return b.config.Model
	}
	return b.defaultModel
}

func (b *OpenAIBackend) SupportsTools() bool {
	m := strings.ToLower(b.model())
	for _, p := range noToolsPatterns {
		if strings.Contains(m, p) {
			return false
		}
	}
	return true
}

func (b *OpenAIBackend) ListModels(ctx context.Context) ([]string, error) {
	// Try the API first (OpenAI-compatible /v1/models endpoint)
	resp, err := b.client.ListModels(ctx)
	if err == nil && len(resp.Models) > 0 {
		models := make([]string, 0, len(resp.Models))
		for _, m := range resp.Models {
			models = append(models, m.ID)
		}
		return models, nil
	}
	// Fall back to hardcoded list
	return b.models, nil
}

// extractThinking splits <think>...</think> blocks from content.
// Returns (thinking, remaining_content).
func extractThinking(text string) (string, string) {
	var parts []string
	remaining := thinkRE.ReplaceAllStringFunc(text, func(match string) string {
		sub := thinkRE.FindStringSubmatch(match)
		if len(sub) > 1 {
			parts = append(parts, strings.TrimSpace(sub[1]))
		}
		return ""
	})
	return strings.Join(parts, "\n\n"), strings.TrimSpace(remaining)
}

// prepareMessages converts Message objects to OpenAI-format request messages.
func (b *OpenAIBackend) prepareMessages(messages []Message) []openai.ChatCompletionMessage {
	var out []openai.ChatCompletionMessage

	for _, m := range messages {
		switch m.Role {
		case RoleTool:
			out = append(out, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    contentToString(m.Content),
				ToolCallID: m.ToolCallID,
			})

		case RoleAssistant:
			blocks := tryParseContentBlocks(m.Content)
			if blocks != nil {
				// Convert Anthropic-style content blocks to OpenAI format.
				var textParts []string
				var toolCalls []openai.ToolCall
				for _, blk := range blocks {
					switch blk["type"] {
					case "text":
						if t, ok := blk["text"].(string); ok {
							textParts = append(textParts, t)
						}
					case "tool_use":
						argsBytes, _ := json.Marshal(blk["input"])
						id, _ := blk["id"].(string)
						name, _ := blk["name"].(string)
						toolCalls = append(toolCalls, openai.ToolCall{
							ID:   id,
							Type: openai.ToolTypeFunction,
							Function: openai.FunctionCall{
								Name:      name,
								Arguments: string(argsBytes),
							},
						})
					}
				}
				msg := openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: strings.Join(textParts, " "),
				}
				if len(toolCalls) > 0 {
					msg.ToolCalls = toolCalls
				}
				out = append(out, msg)
			} else {
				out = append(out, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: contentToString(m.Content),
				})
			}

		case RoleUser, RoleSystem, RoleDirective:
			// Handle multimodal content (list of content blocks with images).
			if blocks, ok := m.Content.([]ContentBlock); ok {
				var parts []openai.ChatMessagePart
				for _, blk := range blocks {
					switch blk["type"] {
					case "text":
						if t, ok := blk["text"].(string); ok {
							parts = append(parts, openai.ChatMessagePart{
								Type: openai.ChatMessagePartTypeText,
								Text: t,
							})
						}
					case "image":
						source, _ := blk["source"].(map[string]any)
						if source != nil {
							srcType, _ := source["type"].(string)
							if srcType == "base64" {
								mediaType, _ := source["media_type"].(string)
								if mediaType == "" {
									mediaType = "image/jpeg"
								}
								data, _ := source["data"].(string)
								parts = append(parts, openai.ChatMessagePart{
									Type: openai.ChatMessagePartTypeImageURL,
									ImageURL: &openai.ChatMessageImageURL{
										URL: fmt.Sprintf("data:%s;base64,%s", mediaType, data),
									},
								})
							} else if srcType == "url" {
								url, _ := source["url"].(string)
								parts = append(parts, openai.ChatMessagePart{
									Type: openai.ChatMessagePartTypeImageURL,
									ImageURL: &openai.ChatMessageImageURL{
										URL: url,
									},
								})
							}
						}
					}
				}
				role := openai.ChatMessageRoleUser
				if m.Role == RoleSystem || m.Role == RoleDirective {
					role = openai.ChatMessageRoleSystem
				}
				out = append(out, openai.ChatCompletionMessage{
					Role:         role,
					MultiContent: parts,
				})
			} else {
				role := openai.ChatMessageRoleUser
				if m.Role == RoleSystem || m.Role == RoleDirective {
					role = openai.ChatMessageRoleSystem
				}
				out = append(out, openai.ChatCompletionMessage{
					Role:    role,
					Content: contentToString(m.Content),
				})
			}

		default:
			// Skip status/command messages — they are display-only.
		}
	}
	return out
}

// Chat sends a non-streaming chat request.
func (b *OpenAIBackend) Chat(ctx context.Context, messages []Message, opts ...ChatOption) (*LLMResponse, error) {
	o := resolveOpts(opts)

	req := openai.ChatCompletionRequest{
		Model:       b.model(),
		Messages:    b.prepareMessages(messages),
		Temperature: float32(b.config.Temperature),
		Stream:      false,
	}
	if b.config.MaxTokens > 0 {
		req.MaxTokens = b.config.MaxTokens
	}
	if o.MaxTokens > 0 {
		req.MaxTokens = o.MaxTokens
	}
	if len(o.Tools) > 0 {
		req.Tools = convertToolDefs(o.Tools)
	}

	resp, err := b.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, wrapError(b.providerName, err, b.model())
	}
	return b.parseResponse(resp), nil
}

// StreamChat sends a streaming chat request and returns chunks via channel.
func (b *OpenAIBackend) StreamChat(ctx context.Context, messages []Message, opts ...ChatOption) (<-chan StreamChunk, error) {
	o := resolveOpts(opts)

	req := openai.ChatCompletionRequest{
		Model:       b.model(),
		Messages:    b.prepareMessages(messages),
		Temperature: float32(b.config.Temperature),
		Stream:      true,
	}
	if b.config.MaxTokens > 0 {
		req.MaxTokens = b.config.MaxTokens
	}
	if o.MaxTokens > 0 {
		req.MaxTokens = o.MaxTokens
	}
	if len(o.Tools) > 0 {
		req.Tools = convertToolDefs(o.Tools)
	}

	stream, err := b.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, wrapError(b.providerName, err, b.model())
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer stream.Close()

		inThinkTag := false
		var buf string

		for {
			chunk, err := stream.Recv()
			if err != nil {
				// Stream ended (io.EOF or error).
				if buf != "" {
					ct := "text"
					if inThinkTag {
						ct = "thinking"
					}
					ch <- StreamChunk{Delta: buf, ChunkType: ct}
				}
				ch <- StreamChunk{IsFinal: true, FinishReason: "stop"}
				return
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			delta := choice.Delta.Content
			isFinal := choice.FinishReason != ""

			if delta == "" && isFinal {
				if buf != "" {
					ct := "text"
					if inThinkTag {
						ct = "thinking"
					}
					ch <- StreamChunk{Delta: buf, ChunkType: ct}
					buf = ""
				}
				ch <- StreamChunk{IsFinal: true, FinishReason: string(choice.FinishReason)}
				return
			}

			// Parse <think>...</think> tags from streamed content.
			buf += delta
			buf, inThinkTag = flushStreamBuf(buf, inThinkTag, ch)

			if isFinal {
				if buf != "" {
					ct := "text"
					if inThinkTag {
						ct = "thinking"
					}
					ch <- StreamChunk{Delta: buf, ChunkType: ct}
					buf = ""
				}
				ch <- StreamChunk{IsFinal: true, FinishReason: string(choice.FinishReason)}
				return
			}
		}
	}()
	return ch, nil
}

// flushStreamBuf parses <think> tags from the buffer and emits chunks.
func flushStreamBuf(buf string, inThinkTag bool, ch chan<- StreamChunk) (string, bool) {
	for {
		if inThinkTag {
			idx := strings.Index(buf, "</think>")
			if idx == -1 {
				safe := len(buf) - 8
				if safe > 0 {
					ch <- StreamChunk{Delta: buf[:safe], ChunkType: "thinking"}
					buf = buf[safe:]
				}
				return buf, inThinkTag
			}
			if idx > 0 {
				ch <- StreamChunk{Delta: buf[:idx], ChunkType: "thinking"}
			}
			buf = buf[idx+8:]
			inThinkTag = false
		} else {
			idx := strings.Index(buf, "<think>")
			if idx == -1 {
				safe := len(buf) - 6
				if safe > 0 {
					ch <- StreamChunk{Delta: buf[:safe], ChunkType: "text"}
					buf = buf[safe:]
				}
				return buf, inThinkTag
			}
			if idx > 0 {
				ch <- StreamChunk{Delta: buf[:idx], ChunkType: "text"}
			}
			buf = buf[idx+7:]
			inThinkTag = true
		}
	}
}

// ChatWithTools sends a chat request with tool definitions.
func (b *OpenAIBackend) ChatWithTools(ctx context.Context, messages []Message, tools []ToolSpec, opts ...ChatOption) (*LLMResponse, error) {
	var oaiTools []map[string]any
	for _, t := range tools {
		oaiTools = append(oaiTools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}
	opts = append(opts, WithTools(oaiTools))
	return b.Chat(ctx, messages, opts...)
}

// parseResponse converts an OpenAI response to LLMResponse.
func (b *OpenAIBackend) parseResponse(resp openai.ChatCompletionResponse) *LLMResponse {
	if len(resp.Choices) == 0 {
		return &LLMResponse{Model: resp.Model}
	}

	choice := resp.Choices[0]
	var usage *UsageInfo
	if resp.Usage.TotalTokens > 0 {
		usage = &UsageInfo{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	var toolCalls []ToolCall
	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]any{}
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	rawContent := choice.Message.Content
	tagThinking, content := extractThinking(rawContent)

	return &LLMResponse{
		Content:      content,
		Model:        resp.Model,
		Usage:        usage,
		FinishReason: string(choice.FinishReason),
		ToolCalls:    toolCalls,
		Thinking:     tagThinking,
	}
}

// convertToolDefs converts generic tool definition maps to openai.Tool structs.
func convertToolDefs(defs []map[string]any) []openai.Tool {
	var tools []openai.Tool
	for _, d := range defs {
		fn, _ := d["function"].(map[string]any)
		if fn == nil {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)

		paramsJSON, _ := json.Marshal(params)
		var paramsDefinition json.RawMessage = paramsJSON

		tools = append(tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        name,
				Description: desc,
				Parameters:  paramsDefinition,
			},
		})
	}
	return tools
}

// tryParseContentBlocks checks if content is or contains Anthropic-style content blocks.
func tryParseContentBlocks(content any) []map[string]any {
	switch c := content.(type) {
	case []map[string]any:
		if len(c) > 0 {
			if _, ok := c[0]["type"]; ok {
				return c
			}
		}
	case string:
		var parsed []map[string]any
		if err := json.Unmarshal([]byte(c), &parsed); err == nil {
			if len(parsed) > 0 {
				if _, ok := parsed[0]["type"]; ok {
					return parsed
				}
			}
		}
	}
	return nil
}

// wrapError wraps provider errors with user-friendly context.
func wrapError(provider string, err error, model string) error {
	msg := err.Error()
	modelInfo := ""
	if model != "" {
		modelInfo = fmt.Sprintf(" (model: %s)", model)
	}

	switch {
	case strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized") ||
		strings.Contains(strings.ToLower(msg), "authentication"):
		return fmt.Errorf("%s%s authentication failed: check your API key", provider, modelInfo)
	case strings.Contains(msg, "429") || strings.Contains(strings.ToLower(msg), "rate limit"):
		return fmt.Errorf("%s%s rate limit exceeded: please wait and retry", provider, modelInfo)
	case strings.Contains(strings.ToLower(msg), "connection") || strings.Contains(strings.ToLower(msg), "connect"):
		return fmt.Errorf("cannot connect to %s API%s: check your internet connection", provider, modelInfo)
	default:
		return fmt.Errorf("%s%s error: %w", provider, modelInfo, err)
	}
}
