import { z } from "zod";

// --- Types ---

export interface MarkdownBlock {
  type: "markdown";
  content: string;
}

export interface CodeBlock {
  type: "code";
  language: string;
  content: string;
}

export interface ImageBlock {
  type: "image";
  url: string;
  alt: string;
}

export type ContentBlock = MarkdownBlock | CodeBlock | ImageBlock;

export interface LessonContent {
  blocks: ContentBlock[];
}

// --- Zod Schemas ---

export const markdownBlockSchema = z.object({
  type: z.literal("markdown"),
  content: z.string().min(1),
});

export const codeBlockSchema = z.object({
  type: z.literal("code"),
  language: z.string().min(1),
  content: z.string().min(1),
});

export const imageBlockSchema = z.object({
  type: z.literal("image"),
  url: z.string().url(),
  alt: z.string().min(1),
});

export const contentBlockSchema = z.discriminatedUnion("type", [
  markdownBlockSchema,
  codeBlockSchema,
  imageBlockSchema,
]);

export const lessonContentSchema = z.object({
  blocks: z.array(contentBlockSchema),
});

// --- Helpers ---

export function parseLessonContent(raw: unknown): LessonContent {
  if (!raw || typeof raw !== "object") {
    return { blocks: [] };
  }
  const result = lessonContentSchema.safeParse(raw);
  if (result.success) return result.data;
  return { blocks: [] };
}

export function createEmptyLesson(): LessonContent {
  return { blocks: [] };
}

export function addBlock(content: LessonContent, block: ContentBlock): LessonContent {
  return { blocks: [...content.blocks, block] };
}

export function removeBlock(content: LessonContent, index: number): LessonContent {
  return { blocks: content.blocks.filter((_, i) => i !== index) };
}

export function moveBlock(content: LessonContent, from: number, to: number): LessonContent {
  const blocks = [...content.blocks];
  const [moved] = blocks.splice(from, 1);
  blocks.splice(to, 0, moved);
  return { blocks };
}

export function updateBlock(
  content: LessonContent,
  index: number,
  block: ContentBlock
): LessonContent {
  const blocks = [...content.blocks];
  blocks[index] = block;
  return { blocks };
}
