package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const ollamaDefaultBaseURL = "http://localhost:11434/v1"
const ollamaDefaultModel = "llama3.1"

var ollamaFallbackModels = []string{
	"llama3.1",
	"llama3.2",
	"mistral",
	"codellama",
	"phi3",
	"gemma2",
}

// OllamaBackend wraps OpenAIBackend for Ollama's OpenAI-compatible API,
// with a custom ListModels that calls Ollama's native /api/tags endpoint.
type OllamaBackend struct {
	*OpenAIBackend
	ollamaBaseURL string // Ollama native API base (without /v1)
}

// NewOllamaBackend creates an Ollama backend backed by the OpenAI-compatible API.
func NewOllamaBackend(cfg LLMConfig) *OllamaBackend {
	// Determine the Ollama base URL for the native API
	ollamaBase := "http://localhost:11434"
	if cfg.BaseURL != "" {
		ollamaBase = cfg.BaseURL
	}

	// OpenAI-compatible endpoint is at /v1
	openaiURL := ollamaBase + "/v1"
	if cfg.BaseURL != "" {
		// If user set a custom base URL, use it as-is for OpenAI compat
		openaiURL = cfg.BaseURL
		// Strip /v1 suffix for native API calls
		if len(cfg.BaseURL) > 3 && cfg.BaseURL[len(cfg.BaseURL)-3:] == "/v1" {
			ollamaBase = cfg.BaseURL[:len(cfg.BaseURL)-3]
		}
	}

	cfgCopy := cfg
	cfgCopy.BaseURL = openaiURL
	// Ollama doesn't need an API key but the OpenAI client may require one
	if cfgCopy.APIKey == "" {
		cfgCopy.APIKey = "ollama"
	}

	return &OllamaBackend{
		OpenAIBackend: NewOpenAIBackend(cfgCopy, "ollama", ollamaDefaultModel, ollamaFallbackModels),
		ollamaBaseURL: ollamaBase,
	}
}

// ListModels calls Ollama's native /api/tags endpoint to list installed models.
func (b *OllamaBackend) ListModels(ctx context.Context) ([]string, error) {
	url := b.ollamaBaseURL + "/api/tags"
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ollamaFallbackModels, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return ollamaFallbackModels, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ollamaFallbackModels, nil
	}

	var data struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ollamaFallbackModels, nil
	}

	if len(data.Models) == 0 {
		return ollamaFallbackModels, nil
	}

	var names []string
	for _, m := range data.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// SupportsTools checks if the Ollama model supports tool calling.
// Most Ollama models don't support native function calling.
func (b *OllamaBackend) SupportsTools() bool {
	return false
}

// ChatWithTools for Ollama — since most models don't support tools natively,
// fall back to including tool descriptions in the prompt.
func (b *OllamaBackend) ChatWithTools(ctx context.Context, messages []Message, tools []ToolSpec, opts ...ChatOption) (*LLMResponse, error) {
	if len(tools) == 0 {
		return b.Chat(ctx, messages, opts...)
	}

	// Build a text description of tools and prepend to messages
	var toolDesc string
	for _, t := range tools {
		paramsJSON, _ := json.MarshalIndent(t.Parameters, "  ", "  ")
		toolDesc += fmt.Sprintf("- %s: %s\n  Parameters: %s\n", t.Name, t.Description, string(paramsJSON))
	}

	toolPrompt := Message{
		Role: RoleSystem,
		Content: fmt.Sprintf(
			"You have access to the following tools. To use a tool, respond with a JSON object "+
				"in this format: {\"tool\": \"tool_name\", \"arguments\": {...}}\n\nTools:\n%s", toolDesc),
	}

	augmented := make([]Message, 0, len(messages)+1)
	augmented = append(augmented, toolPrompt)
	augmented = append(augmented, messages...)

	return b.Chat(ctx, augmented, opts...)
}
