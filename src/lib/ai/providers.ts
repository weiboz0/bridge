/**
 * LLM Provider Registry
 *
 * Mirrors the magicburg factory pattern: all OpenAI-compatible providers
 * use the same OpenAI SDK client with different base URLs and API keys.
 * Anthropic uses its own SDK for native features.
 */

export interface ProviderConfig {
  type: "openai-compatible" | "anthropic";
  name: string;
  baseUrl: string;
  apiKeyEnv: string;
  defaultModel: string;
}

// --- Provider Definitions ---

const PROVIDERS: Record<string, ProviderConfig> = {
  openai: {
    type: "openai-compatible",
    name: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    apiKeyEnv: "OPENAI_API_KEY",
    defaultModel: "gpt-4o-mini",
  },
  anthropic: {
    type: "anthropic",
    name: "Anthropic",
    baseUrl: "https://api.anthropic.com",
    apiKeyEnv: "ANTHROPIC_API_KEY",
    defaultModel: "claude-sonnet-4-20250514",
  },
  ark: {
    type: "openai-compatible",
    name: "Ark (ByteDance)",
    baseUrl: "https://ark.cn-beijing.volces.com/api/v3",
    apiKeyEnv: "ARK_API_KEY",
    defaultModel: "doubao-1-5-pro-32k-250115",
  },
  aliyun: {
    type: "openai-compatible",
    name: "Aliyun DashScope",
    baseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    apiKeyEnv: "DASHSCOPE_API_KEY",
    defaultModel: "qwen-max",
  },
  nvidia: {
    type: "openai-compatible",
    name: "NVIDIA NIM",
    baseUrl: "https://integrate.api.nvidia.com/v1",
    apiKeyEnv: "NVIDIA_API_KEY",
    defaultModel: "meta/llama-3.3-70b-instruct",
  },
  openrouter: {
    type: "openai-compatible",
    name: "OpenRouter",
    baseUrl: "https://openrouter.ai/api/v1",
    apiKeyEnv: "OPENROUTER_API_KEY",
    defaultModel: "anthropic/claude-sonnet-4",
  },
};

// Aliases for convenience (same as magicburg)
const ALIASES: Record<string, string> = {
  openai: "openai",
  anthropic: "anthropic",
  claude: "anthropic",
  ark: "ark",
  doubao: "ark",
  volcengine: "ark",
  aliyun: "aliyun",
  dashscope: "aliyun",
  qwen: "aliyun",
  bailian: "aliyun",
  nvidia: "nvidia",
  nim: "nvidia",
  openrouter: "openrouter",
};

export function resolveProvider(backend: string): ProviderConfig {
  const key = ALIASES[backend.toLowerCase()];
  if (!key || !PROVIDERS[key]) {
    throw new Error(
      `Unknown LLM backend: "${backend}". Supported: ${Object.keys(ALIASES).join(", ")}`
    );
  }
  return PROVIDERS[key];
}

export function getProviderApiKey(provider: ProviderConfig): string {
  return process.env[provider.apiKeyEnv] || "";
}

/**
 * Resolve the active LLM configuration from environment variables.
 *
 * Environment variables:
 *   LLM_BACKEND  — provider name (default: "anthropic")
 *   LLM_MODEL    — model override (default: provider's default)
 *   LLM_BASE_URL — base URL override (default: provider's default)
 */
export function getLLMConfig(): {
  provider: ProviderConfig;
  model: string;
  baseUrl: string;
  apiKey: string;
} {
  const backend = process.env.LLM_BACKEND || "anthropic";
  const provider = resolveProvider(backend);
  const model = process.env.LLM_MODEL || provider.defaultModel;
  const baseUrl = process.env.LLM_BASE_URL || provider.baseUrl;
  const apiKey = getProviderApiKey(provider);

  return { provider, model, baseUrl, apiKey };
}
