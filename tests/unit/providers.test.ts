import { describe, it, expect } from "vitest";
import { resolveProvider, getLLMConfig } from "@/lib/ai/providers";

describe("resolveProvider", () => {
  it("resolves canonical provider names", () => {
    expect(resolveProvider("openai").name).toBe("OpenAI");
    expect(resolveProvider("anthropic").name).toBe("Anthropic");
    expect(resolveProvider("ark").name).toBe("Ark (ByteDance)");
    expect(resolveProvider("aliyun").name).toBe("Aliyun DashScope");
    expect(resolveProvider("nvidia").name).toBe("NVIDIA NIM");
    expect(resolveProvider("openrouter").name).toBe("OpenRouter");
  });

  it("resolves aliases", () => {
    expect(resolveProvider("doubao").name).toBe("Ark (ByteDance)");
    expect(resolveProvider("volcengine").name).toBe("Ark (ByteDance)");
    expect(resolveProvider("dashscope").name).toBe("Aliyun DashScope");
    expect(resolveProvider("qwen").name).toBe("Aliyun DashScope");
    expect(resolveProvider("bailian").name).toBe("Aliyun DashScope");
    expect(resolveProvider("nim").name).toBe("NVIDIA NIM");
    expect(resolveProvider("claude").name).toBe("Anthropic");
  });

  it("is case-insensitive", () => {
    expect(resolveProvider("OpenAI").name).toBe("OpenAI");
    expect(resolveProvider("ANTHROPIC").name).toBe("Anthropic");
    expect(resolveProvider("Aliyun").name).toBe("Aliyun DashScope");
  });

  it("throws on unknown provider", () => {
    expect(() => resolveProvider("unknown")).toThrow('Unknown LLM backend: "unknown"');
  });

  it("returns correct type for OpenAI-compatible providers", () => {
    expect(resolveProvider("openai").type).toBe("openai-compatible");
    expect(resolveProvider("ark").type).toBe("openai-compatible");
    expect(resolveProvider("aliyun").type).toBe("openai-compatible");
    expect(resolveProvider("nvidia").type).toBe("openai-compatible");
    expect(resolveProvider("openrouter").type).toBe("openai-compatible");
  });

  it("returns anthropic type for anthropic", () => {
    expect(resolveProvider("anthropic").type).toBe("anthropic");
  });

  it("returns correct base URLs", () => {
    expect(resolveProvider("ark").baseUrl).toBe("https://ark.cn-beijing.volces.com/api/v3");
    expect(resolveProvider("aliyun").baseUrl).toBe("https://dashscope.aliyuncs.com/compatible-mode/v1");
    expect(resolveProvider("nvidia").baseUrl).toBe("https://integrate.api.nvidia.com/v1");
  });

  it("returns correct API key env var names", () => {
    expect(resolveProvider("openai").apiKeyEnv).toBe("OPENAI_API_KEY");
    expect(resolveProvider("anthropic").apiKeyEnv).toBe("ANTHROPIC_API_KEY");
    expect(resolveProvider("ark").apiKeyEnv).toBe("ARK_API_KEY");
    expect(resolveProvider("aliyun").apiKeyEnv).toBe("DASHSCOPE_API_KEY");
    expect(resolveProvider("nvidia").apiKeyEnv).toBe("NVIDIA_API_KEY");
  });

  it("returns default models per provider", () => {
    expect(resolveProvider("openai").defaultModel).toBe("gpt-4o-mini");
    expect(resolveProvider("ark").defaultModel).toBe("doubao-1-5-pro-32k-250115");
    expect(resolveProvider("aliyun").defaultModel).toBe("qwen-max");
    expect(resolveProvider("nvidia").defaultModel).toBe("meta/llama-3.3-70b-instruct");
  });
});

describe("getLLMConfig", () => {
  it("defaults to anthropic when LLM_BACKEND is not set", () => {
    delete process.env.LLM_BACKEND;
    delete process.env.LLM_MODEL;
    delete process.env.LLM_BASE_URL;
    const config = getLLMConfig();
    expect(config.provider.name).toBe("Anthropic");
    expect(config.model).toBe("claude-sonnet-4-20250514");
  });
});
