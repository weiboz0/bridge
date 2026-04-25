package llm

import (
	"fmt"
	"strings"
)

// Backend aliases — map user-friendly names to canonical backend keys.
var backendAliases = map[string]string{
	"claude":     "anthropic",
	"gpt":        "openai",
	"google":     "gemini",
	"local":      "ollama",
	"openai":     "openai",
	"anthropic":  "anthropic",
	"gemini":     "gemini",
	"ollama":     "ollama",
	"openrouter": "openrouter",
	"or":         "openrouter",
	// Ark (ByteDance Volcengine) — OpenAI-compatible
	"ark":        "ark",
	"doubao":     "ark",
	"volcengine": "ark",
	// NVIDIA NIM — OpenAI-compatible
	"nvidia": "nvidia",
	"nim":    "nvidia",
	// Aliyun Bailian (百炼) — OpenAI-compatible, DashScope API
	"aliyun":    "aliyun",
	"bailian":   "aliyun",
	"dashscope": "aliyun",
	"qwen":      "aliyun",
	// DeepSeek — OpenAI-compatible
	"deepseek": "deepseek",
	"ds":       "deepseek",
}

// ── Ark (ByteDance Volcengine) ──────────────────────────────────────────────

const arkBaseURL = "https://ark.cn-beijing.volces.com/api/v3"
const arkDefaultModel = "doubao-1-5-pro-32k-250115"

var arkModels = []string{
	"doubao-seed-1-6-250615",
	"doubao-1-5-pro-32k-250115",
	"doubao-1-5-lite-32k-250115",
	"doubao-1-5-pro-256k-250115",
	"doubao-1-5-vision-32k-250115",
	"doubao-1-5-thinking-pro-250415",
	"doubao-pro-32k",
	"doubao-pro-128k",
	"doubao-pro-256k",
	"doubao-pro-4k",
	"doubao-lite-32k",
	"doubao-lite-128k",
	"doubao-lite-4k",
	"deepseek-v3-250324",
	"deepseek-r1-250120",
}

// ── NVIDIA NIM — OpenAI-compatible ──────────────────────────────────────────

const nvidiaBaseURL = "https://integrate.api.nvidia.com/v1"
const nvidiaDefaultModel = "meta/llama-3.3-70b-instruct"

var nvidiaModels = []string{
	"meta/llama-3.3-70b-instruct",
	"meta/llama-3.1-405b-instruct",
	"meta/llama-3.1-70b-instruct",
	"meta/llama-3.1-8b-instruct",
	"mistralai/mistral-large-2-instruct",
	"mistralai/mistral-small-24b-instruct-2501",
	"deepseek-ai/deepseek-r1",
	"deepseek-ai/deepseek-r1-distill-llama-70b",
	"qwen/qwen2.5-72b-instruct",
	"qwen/qwq-32b",
	"google/gemma-2-27b-it",
	"nvidia/llama-3.1-nemotron-70b-instruct",
}

// ── DeepSeek — OpenAI-compatible ───────────────────────────────────────────

const deepseekBaseURL = "https://api.deepseek.com"
const deepseekDefaultModel = "deepseek-v4-flash"

var deepseekModels = []string{
	"deepseek-v4-flash",
	"deepseek-v4-pro",
}

// ── OpenRouter — routes to 300+ models via OpenAI-compatible API ────────────

const openrouterBaseURL = "https://openrouter.ai/api/v1"
const openrouterDefaultModel = "meta-llama/llama-3.3-70b-instruct:free"

var openrouterModels = []string{
	"meta-llama/llama-3.3-70b-instruct:free",
	"google/gemma-3-27b-it:free",
	"mistralai/mistral-small-3.1-24b-instruct:free",
	"anthropic/claude-sonnet-4-6",
	"openai/gpt-4o",
	"openai/gpt-4o-mini",
	"google/gemini-2.0-flash-001",
}

// ── Aliyun Bailian / DashScope (Qwen series) ───────────────────────────────

const dashscopeBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
const dashscopeDefaultModel = "qwen-max"

var dashscopeModels = []string{
	"qwen3-235b-a22b",
	"qwen3-30b-a3b",
	"qwen3-14b",
	"qwen3-8b",
	"qwen-max",
	"qwen-max-latest",
	"qwen-plus",
	"qwen-plus-latest",
	"qwen-turbo",
	"qwen-turbo-latest",
	"qwen-long",
	"qwq-32b",
	"qwen-vl-max",
	"qwen-vl-plus",
}

// CreateBackend instantiates the appropriate LLM backend from config.
func CreateBackend(cfg LLMConfig) (Backend, error) {
	key, ok := backendAliases[strings.ToLower(cfg.Backend)]
	if !ok {
		// Collect unique canonical names for the error message.
		seen := make(map[string]bool)
		for _, v := range backendAliases {
			seen[v] = true
		}
		var names []string
		for n := range seen {
			names = append(names, n)
		}
		return nil, fmt.Errorf("unknown backend %q; supported: %s", cfg.Backend, strings.Join(names, ", "))
	}

	switch key {
	case "openai":
		return NewOpenAIBackend(cfg, "openai", openaiDefaultModel, openaiModels), nil

	case "ark":
		if cfg.BaseURL == "" {
			cfg.BaseURL = arkBaseURL
		}
		return NewOpenAIBackend(cfg, "ark", arkDefaultModel, arkModels), nil

	case "nvidia":
		if cfg.BaseURL == "" {
			cfg.BaseURL = nvidiaBaseURL
		}
		return NewOpenAIBackend(cfg, "nvidia", nvidiaDefaultModel, nvidiaModels), nil

	case "aliyun":
		if cfg.BaseURL == "" {
			cfg.BaseURL = dashscopeBaseURL
		}
		return NewOpenAIBackend(cfg, "aliyun", dashscopeDefaultModel, dashscopeModels), nil

	case "deepseek":
		if cfg.BaseURL == "" {
			cfg.BaseURL = deepseekBaseURL
		}
		return NewOpenAIBackend(cfg, "deepseek", deepseekDefaultModel, deepseekModels), nil

	case "anthropic":
		return NewAnthropicBackend(cfg), nil

	case "gemini":
		return NewGeminiBackend(cfg), nil

	case "ollama":
		return NewOllamaBackend(cfg), nil

	case "openrouter":
		if cfg.BaseURL == "" {
			cfg.BaseURL = openrouterBaseURL
		}
		return NewOpenAIBackend(cfg, "openrouter", openrouterDefaultModel, openrouterModels), nil

	default:
		return nil, fmt.Errorf("backend %q not yet implemented", key)
	}
}
