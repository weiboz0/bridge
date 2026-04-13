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
)

var geminiModels = []string{
	"gemini-2.0-flash",
	"gemini-2.0-flash-lite",
	"gemini-1.5-pro",
	"gemini-1.5-flash",
	"gemini-1.5-flash-8b",
}

const geminiDefaultModel = "gemini-2.0-flash"
const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// GeminiBackend implements Backend for the Google Gemini REST API.
type GeminiBackend struct {
	config       LLMConfig
	providerName string
	httpClient   *http.Client
}

// NewGeminiBackend creates a new Gemini backend.
func NewGeminiBackend(cfg LLMConfig) *GeminiBackend {
	return &GeminiBackend{
		config:       cfg,
		providerName: "gemini",
		httpClient:   &http.Client{},
	}
}

func (b *GeminiBackend) Name() string { return b.providerName }

func (b *GeminiBackend) model() string {
	if b.config.Model != "" {
		return b.config.Model
	}
	return geminiDefaultModel
}

func (b *GeminiBackend) baseURL() string {
	if b.config.BaseURL != "" {
		return strings.TrimRight(b.config.BaseURL, "/")
	}
	return geminiBaseURL
}

func (b *GeminiBackend) SupportsTools() bool { return true }

func (b *GeminiBackend) ListModels(_ context.Context) ([]string, error) {
	return geminiModels, nil
}

// prepareMessages converts internal Messages to Gemini format.
// Returns (systemInstruction, contents) where contents is the array of
// role/parts objects and systemInstruction is optional.
func (b *GeminiBackend) prepareMessages(messages []Message) (string, []map[string]any) {
	var system string
	var contents []map[string]any

	for _, m := range messages {
		switch m.Role {
		case RoleSystem, RoleDirective:
			system = contentToString(m.Content)
		case RoleUser:
			contents = append(contents, map[string]any{
				"role":  "user",
				"parts": b.contentToParts(m.Content),
			})
		case RoleAssistant:
			contents = append(contents, map[string]any{
				"role":  "model",
				"parts": b.contentToParts(m.Content),
			})
		case RoleTool:
			// Tool results as functionResponse parts
			var resultData any
			if err := json.Unmarshal([]byte(contentToString(m.Content)), &resultData); err != nil {
				resultData = map[string]any{"result": contentToString(m.Content)}
			}
			// Wrap non-object results in an object
			if _, ok := resultData.(map[string]any); !ok {
				resultData = map[string]any{"result": resultData}
			}
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{
						"functionResponse": map[string]any{
							"name":     m.Name,
							"response": resultData,
						},
					},
				},
			})
		default:
			// Skip status/command
		}
	}
	return system, contents
}

// contentToParts converts message content to Gemini parts array.
func (b *GeminiBackend) contentToParts(content any) []map[string]any {
	switch c := content.(type) {
	case string:
		return []map[string]any{{"text": c}}
	case []ContentBlock:
		var parts []map[string]any
		for _, block := range c {
			switch block["type"] {
			case "text":
				if t, ok := block["text"].(string); ok {
					parts = append(parts, map[string]any{"text": t})
				}
			case "tool_use":
				name, _ := block["name"].(string)
				args, _ := block["input"].(map[string]any)
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"name": name,
						"args": args,
					},
				})
			}
		}
		if len(parts) == 0 {
			parts = append(parts, map[string]any{"text": ""})
		}
		return parts
	default:
		return []map[string]any{{"text": contentToString(content)}}
	}
}

// buildRequestBody constructs the Gemini generateContent request body.
func (b *GeminiBackend) buildRequestBody(messages []Message, opts chatOpts) map[string]any {
	system, contents := b.prepareMessages(messages)

	body := map[string]any{
		"contents": contents,
	}

	if system != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": system}},
		}
	}

	// Generation config
	genConfig := map[string]any{}
	if b.config.Temperature != 0 {
		genConfig["temperature"] = b.config.Temperature
	}
	maxTokens := b.config.MaxTokens
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}
	if maxTokens > 0 {
		genConfig["maxOutputTokens"] = maxTokens
	}
	if len(genConfig) > 0 {
		body["generationConfig"] = genConfig
	}

	// Tools
	if len(opts.Tools) > 0 {
		body["tools"] = opts.Tools
	}

	return body
}

// doRequest sends an HTTP request to the Gemini API.
func (b *GeminiBackend) doRequest(ctx context.Context, endpoint string, body map[string]any) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	url := fmt.Sprintf("%s/models/%s:%s%skey=%s", b.baseURL(), b.model(), endpoint, sep, b.config.APIKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

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

// parseGeminiResponse converts a Gemini API response to LLMResponse.
func parseGeminiResponse(data map[string]any, model string) *LLMResponse {
	candidates, _ := data["candidates"].([]any)
	if len(candidates) == 0 {
		return &LLMResponse{Model: model}
	}

	candidate, _ := candidates[0].(map[string]any)
	content, _ := candidate["content"].(map[string]any)
	parts, _ := content["parts"].([]any)

	var textParts []string
	var toolCalls []ToolCall

	for _, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := part["text"].(string); ok {
			textParts = append(textParts, t)
		}
		if fc, ok := part["functionCall"].(map[string]any); ok {
			name, _ := fc["name"].(string)
			args := make(map[string]any)
			if a, ok := fc["args"].(map[string]any); ok {
				args = a
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:        name, // Gemini doesn't use IDs, use name
				Name:      name,
				Arguments: args,
			})
		}
	}

	finishReason, _ := candidate["finishReason"].(string)

	// Parse usage metadata
	var usage *UsageInfo
	if um, ok := data["usageMetadata"].(map[string]any); ok {
		prompt := intFromAny(um["promptTokenCount"])
		completion := intFromAny(um["candidatesTokenCount"])
		usage = &UsageInfo{
			PromptTokens:     prompt,
			CompletionTokens: completion,
			TotalTokens:      prompt + completion,
		}
	}

	return &LLMResponse{
		Content:      strings.Join(textParts, ""),
		Model:        model,
		Usage:        usage,
		FinishReason: finishReason,
		ToolCalls:    toolCalls,
	}
}

// Chat sends a non-streaming chat request.
func (b *GeminiBackend) Chat(ctx context.Context, messages []Message, opts ...ChatOption) (*LLMResponse, error) {
	o := resolveOpts(opts)
	body := b.buildRequestBody(messages, o)

	resp, err := b.doRequest(ctx, "generateContent", body)
	if err != nil {
		return nil, wrapError(b.providerName, err, b.model())
	}
	defer resp.Body.Close()

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return parseGeminiResponse(data, b.model()), nil
}

// StreamChat sends a streaming chat request and returns chunks via channel.
func (b *GeminiBackend) StreamChat(ctx context.Context, messages []Message, opts ...ChatOption) (<-chan StreamChunk, error) {
	o := resolveOpts(opts)
	body := b.buildRequestBody(messages, o)

	resp, err := b.doRequest(ctx, "streamGenerateContent?alt=sse", body)
	if err != nil {
		return nil, wrapError(b.providerName, err, b.model())
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			parsed := parseGeminiResponse(event, b.model())
			if parsed.Content != "" {
				ch <- StreamChunk{Delta: parsed.Content, ChunkType: "text"}
			}
		}
		ch <- StreamChunk{IsFinal: true, FinishReason: "stop"}
	}()

	return ch, nil
}

// ChatWithTools sends a chat request with Gemini-native tool definitions.
func (b *GeminiBackend) ChatWithTools(ctx context.Context, messages []Message, tools []ToolSpec, opts ...ChatOption) (*LLMResponse, error) {
	// Convert to Gemini tool format: tools array with functionDeclarations
	var declarations []map[string]any
	for _, t := range tools {
		declarations = append(declarations, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
		})
	}
	geminiTools := []map[string]any{
		{
			"functionDeclarations": declarations,
		},
	}
	opts = append(opts, WithTools(geminiTools))
	return b.Chat(ctx, messages, opts...)
}
