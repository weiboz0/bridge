/**
 * Real LLM provider integration tests.
 *
 * These tests hit actual API endpoints. They are skipped when the
 * corresponding API key is not set in the environment.
 *
 * Run with: DATABASE_URL=... bun run test tests/llm/
 *
 * Set keys in .env:
 *   ANTHROPIC_API_KEY, OPENAI_API_KEY, DASHSCOPE_API_KEY,
 *   OPENROUTER_API_KEY, GEMINI_API_KEY
 */
import { describe, it, expect } from "vitest";
import Anthropic from "@anthropic-ai/sdk";
import OpenAI from "openai";
import { resolveProvider, getProviderApiKey } from "@/lib/ai/providers";

const TEST_PROMPT = "Reply with exactly one word: hello";
const TIMEOUT = 30000;

// --- Anthropic ---

describe.skipIf(!process.env.ANTHROPIC_API_KEY)("Anthropic (Claude)", () => {
  it("sends a message and gets a response", async () => {
    const provider = resolveProvider("anthropic");
    const client = new Anthropic({ apiKey: process.env.ANTHROPIC_API_KEY });

    const response = await client.messages.create({
      model: provider.defaultModel,
      max_tokens: 50,
      messages: [{ role: "user", content: TEST_PROMPT }],
    });

    expect(response.content).toBeDefined();
    expect(response.content.length).toBeGreaterThan(0);
    const text = response.content[0].type === "text" ? response.content[0].text : "";
    expect(text.toLowerCase()).toContain("hello");
    console.log(`  Anthropic (${provider.defaultModel}): "${text.trim()}"`);
  }, TIMEOUT);

  it("streams a response", async () => {
    const client = new Anthropic({ apiKey: process.env.ANTHROPIC_API_KEY });

    const stream = await client.messages.create({
      model: "claude-sonnet-4-20250514",
      max_tokens: 50,
      messages: [{ role: "user", content: TEST_PROMPT }],
      stream: true,
    });

    let fullText = "";
    for await (const event of stream) {
      if (event.type === "content_block_delta" && event.delta.type === "text_delta") {
        fullText += event.delta.text;
      }
    }

    expect(fullText.length).toBeGreaterThan(0);
    console.log(`  Anthropic stream: "${fullText.trim()}"`);
  }, TIMEOUT);
});

// --- OpenAI ---

describe.skipIf(!process.env.OPENAI_API_KEY)("OpenAI", () => {
  it("sends a chat completion and gets a response", async () => {
    const provider = resolveProvider("openai");
    const client = new OpenAI({ apiKey: process.env.OPENAI_API_KEY });

    const response = await client.chat.completions.create({
      model: provider.defaultModel,
      max_tokens: 50,
      messages: [
        { role: "system", content: "You are a helpful assistant." },
        { role: "user", content: TEST_PROMPT },
      ],
    });

    const text = response.choices[0]?.message?.content || "";
    expect(text.length).toBeGreaterThan(0);
    expect(text.toLowerCase()).toContain("hello");
    console.log(`  OpenAI (${provider.defaultModel}): "${text.trim()}"`);
  }, TIMEOUT);

  it("streams a response", async () => {
    const client = new OpenAI({ apiKey: process.env.OPENAI_API_KEY });

    const stream = await client.chat.completions.create({
      model: "gpt-4o-mini",
      max_tokens: 50,
      messages: [{ role: "user", content: TEST_PROMPT }],
      stream: true,
    });

    let fullText = "";
    for await (const chunk of stream) {
      const text = chunk.choices[0]?.delta?.content;
      if (text) fullText += text;
    }

    expect(fullText.length).toBeGreaterThan(0);
    console.log(`  OpenAI stream: "${fullText.trim()}"`);
  }, TIMEOUT);
});

// --- DashScope (Aliyun/Qwen) ---

describe.skipIf(!process.env.DASHSCOPE_API_KEY)("DashScope (Aliyun/Qwen)", () => {
  it("sends a chat completion and gets a response", async () => {
    const provider = resolveProvider("aliyun");
    const client = new OpenAI({
      apiKey: process.env.DASHSCOPE_API_KEY,
      baseURL: process.env.LLM_BASE_URL || provider.baseUrl,
    });

    const response = await client.chat.completions.create({
      model: process.env.LLM_MODEL || provider.defaultModel,
      max_tokens: 50,
      messages: [
        { role: "system", content: "You are a helpful assistant. Reply concisely." },
        { role: "user", content: TEST_PROMPT },
      ],
    });

    const text = response.choices[0]?.message?.content || "";
    expect(text.length).toBeGreaterThan(0);
    console.log(`  DashScope (${process.env.LLM_MODEL || provider.defaultModel}): "${text.trim()}"`);
  }, TIMEOUT);

  it("streams a response", async () => {
    const provider = resolveProvider("aliyun");
    const client = new OpenAI({
      apiKey: process.env.DASHSCOPE_API_KEY,
      baseURL: process.env.LLM_BASE_URL || provider.baseUrl,
    });

    const stream = await client.chat.completions.create({
      model: process.env.LLM_MODEL || provider.defaultModel,
      max_tokens: 50,
      messages: [{ role: "user", content: TEST_PROMPT }],
      stream: true,
    });

    let fullText = "";
    for await (const chunk of stream) {
      const text = chunk.choices[0]?.delta?.content;
      if (text) fullText += text;
    }

    expect(fullText.length).toBeGreaterThan(0);
    console.log(`  DashScope stream: "${fullText.trim()}"`);
  }, TIMEOUT);
});

// --- OpenRouter ---

describe.skipIf(!process.env.OPENROUTER_API_KEY)("OpenRouter", () => {
  it("sends a chat completion and gets a response", async () => {
    const provider = resolveProvider("openrouter");
    const client = new OpenAI({
      apiKey: process.env.OPENROUTER_API_KEY,
      baseURL: provider.baseUrl,
    });

    const response = await client.chat.completions.create({
      model: provider.defaultModel,
      max_tokens: 50,
      messages: [
        { role: "system", content: "You are a helpful assistant. Reply concisely." },
        { role: "user", content: TEST_PROMPT },
      ],
    });

    const text = response.choices[0]?.message?.content || "";
    expect(text.length).toBeGreaterThan(0);
    console.log(`  OpenRouter (${provider.defaultModel}): "${text.trim()}"`);
  }, TIMEOUT);
});

// --- Gemini ---

describe.skipIf(!process.env.GEMINI_API_KEY)("Google Gemini", () => {
  it("sends a chat completion and gets a response", async () => {
    const provider = resolveProvider("gemini");
    const client = new OpenAI({
      apiKey: process.env.GEMINI_API_KEY,
      baseURL: provider.baseUrl,
    });

    const response = await client.chat.completions.create({
      model: provider.defaultModel,
      max_tokens: 50,
      messages: [
        { role: "system", content: "You are a helpful assistant. Reply concisely." },
        { role: "user", content: TEST_PROMPT },
      ],
    });

    const text = response.choices[0]?.message?.content || "";
    expect(text.length).toBeGreaterThan(0);
    console.log(`  Gemini (${provider.defaultModel}): "${text.trim()}"`);
  }, TIMEOUT);

  it("streams a response", async () => {
    const provider = resolveProvider("gemini");
    const client = new OpenAI({
      apiKey: process.env.GEMINI_API_KEY,
      baseURL: provider.baseUrl,
    });

    const stream = await client.chat.completions.create({
      model: provider.defaultModel,
      max_tokens: 50,
      messages: [{ role: "user", content: TEST_PROMPT }],
      stream: true,
    });

    let fullText = "";
    for await (const chunk of stream) {
      const text = chunk.choices[0]?.delta?.content;
      if (text) fullText += text;
    }

    expect(fullText.length).toBeGreaterThan(0);
    console.log(`  Gemini stream: "${fullText.trim()}"`);
  }, TIMEOUT);
});
