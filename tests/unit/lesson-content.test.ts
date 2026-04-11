import { describe, it, expect } from "vitest";
import {
  parseLessonContent,
  createEmptyLesson,
  addBlock,
  removeBlock,
  moveBlock,
  updateBlock,
  lessonContentSchema,
  type LessonContent,
} from "@/lib/lesson-content";

describe("parseLessonContent", () => {
  it("parses valid lesson content", () => {
    const raw = {
      blocks: [
        { type: "markdown", content: "# Hello" },
        { type: "code", language: "python", content: "print('hi')" },
        { type: "image", url: "https://example.com/img.png", alt: "diagram" },
      ],
    };
    const result = parseLessonContent(raw);
    expect(result.blocks).toHaveLength(3);
    expect(result.blocks[0].type).toBe("markdown");
    expect(result.blocks[1].type).toBe("code");
    expect(result.blocks[2].type).toBe("image");
  });

  it("returns empty blocks for null/undefined", () => {
    expect(parseLessonContent(null).blocks).toHaveLength(0);
    expect(parseLessonContent(undefined).blocks).toHaveLength(0);
  });

  it("returns empty blocks for invalid data", () => {
    expect(parseLessonContent({ blocks: [{ type: "invalid" }] }).blocks).toHaveLength(0);
    expect(parseLessonContent("string").blocks).toHaveLength(0);
  });
});

describe("block operations", () => {
  it("createEmptyLesson returns empty blocks", () => {
    const lesson = createEmptyLesson();
    expect(lesson.blocks).toHaveLength(0);
  });

  it("addBlock appends a block", () => {
    let lesson = createEmptyLesson();
    lesson = addBlock(lesson, { type: "markdown", content: "# Title" });
    expect(lesson.blocks).toHaveLength(1);
    lesson = addBlock(lesson, { type: "code", language: "python", content: "x = 1" });
    expect(lesson.blocks).toHaveLength(2);
  });

  it("removeBlock removes by index", () => {
    const lesson: LessonContent = {
      blocks: [
        { type: "markdown", content: "A" },
        { type: "markdown", content: "B" },
        { type: "markdown", content: "C" },
      ],
    };
    const result = removeBlock(lesson, 1);
    expect(result.blocks).toHaveLength(2);
    expect(result.blocks[0].content).toBe("A");
    expect(result.blocks[1].content).toBe("C");
  });

  it("moveBlock reorders blocks", () => {
    const lesson: LessonContent = {
      blocks: [
        { type: "markdown", content: "A" },
        { type: "markdown", content: "B" },
        { type: "markdown", content: "C" },
      ],
    };
    const result = moveBlock(lesson, 2, 0);
    expect(result.blocks[0].content).toBe("C");
    expect(result.blocks[1].content).toBe("A");
    expect(result.blocks[2].content).toBe("B");
  });

  it("updateBlock replaces a block at index", () => {
    const lesson: LessonContent = {
      blocks: [
        { type: "markdown", content: "old" },
      ],
    };
    const result = updateBlock(lesson, 0, { type: "markdown", content: "new" });
    expect(result.blocks[0].content).toBe("new");
  });
});

describe("lessonContentSchema", () => {
  it("validates correct content", () => {
    const result = lessonContentSchema.safeParse({
      blocks: [{ type: "markdown", content: "test" }],
    });
    expect(result.success).toBe(true);
  });

  it("rejects invalid block type", () => {
    const result = lessonContentSchema.safeParse({
      blocks: [{ type: "video", url: "test" }],
    });
    expect(result.success).toBe(false);
  });

  it("rejects missing required fields", () => {
    const result = lessonContentSchema.safeParse({
      blocks: [{ type: "code" }], // missing language and content
    });
    expect(result.success).toBe(false);
  });
});
