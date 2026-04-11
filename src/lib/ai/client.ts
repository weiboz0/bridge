import Anthropic from "@anthropic-ai/sdk";
import OpenAI from "openai";
import { getLLMConfig, type ProviderConfig } from "./providers";

let anthropicClient: Anthropic | null = null;
let openaiClient: OpenAI | null = null;
let cachedConfig: ReturnType<typeof getLLMConfig> | null = null;

function getConfig() {
  if (!cachedConfig) {
    cachedConfig = getLLMConfig();
  }
  return cachedConfig;
}

export function getAnthropicClient(): Anthropic {
  if (!anthropicClient) {
    const config = getConfig();
    anthropicClient = new Anthropic({
      apiKey: config.apiKey || process.env.ANTHROPIC_API_KEY,
    });
  }
  return anthropicClient;
}

export function getOpenAIClient(): OpenAI {
  if (!openaiClient) {
    const config = getConfig();
    openaiClient = new OpenAI({
      apiKey: config.apiKey,
      baseURL: config.baseUrl,
    });
  }
  return openaiClient;
}

export function isAnthropicBackend(): boolean {
  return getConfig().provider.type === "anthropic";
}

export function getModel(): string {
  return getConfig().model;
}

export function getProviderName(): string {
  return getConfig().provider.name;
}
