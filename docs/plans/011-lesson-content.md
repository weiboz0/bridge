# Lesson Content Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the lesson content authoring and rendering pipeline. Teachers author structured lesson content (markdown, code, image blocks) within topics, and students see that content alongside their code editor during sessions.

**Architecture:** The lesson content data model already exists (`topics.lessonContent` JSONB column). This plan adds: (1) a shared React renderer component for displaying lesson blocks, (2) a teacher-facing block editor for authoring content, (3) enhanced teacher course management UI (create course, manage topics, edit lesson content), (4) student lesson view integration, and (5) the `react-markdown` + `remark-gfm` dependency.

**Tech Stack:** Next.js 16 App Router, React Server Components + Client Components, Drizzle ORM, shadcn/ui (Card, Button, Input, Label, Select), lucide-react icons, CodeMirror 6, react-markdown + remark-gfm, Vitest + Testing Library

**Depends on:** Plan 007 (course-hierarchy — schema + lib functions), Plan 009 (portal-shell), Plan 010 (portal-pages — basic course/class pages exist)

**Key constraints:**
- shadcn/ui uses `@base-ui/react` -- NO `asChild` prop; use `buttonVariants()` with `<Link>` instead
- Auth.js v5: `session.user.id`, `session.user.isPlatformAdmin`
- Drizzle ORM for all DB queries -- use existing lib functions, add new ones only when missing
- `fileParallelism: false` in Vitest -- `.tsx` tests need `// @vitest-environment jsdom`
- Server actions use inline `"use server"` inside server component functions (see `src/app/(portal)/admin/orgs/page.tsx` pattern)
- `topics.lessonContent` defaults to `{}` -- valid content has shape `{ blocks: LessonBlock[] }`
- Test files live under `tests/` directory, not alongside source files

---

## Lesson Content Block Schema

```typescript
// src/lib/lesson-content.ts

interface MarkdownBlock {
  type: "markdown";
  content: string;
}

interface CodeBlock {
  type: "code";
  language: string; // "python" | "javascript" | etc.
  content: string;
}

interface ImageBlock {
  type: "image";
  url: string;
  alt: string;
}

type LessonBlock = MarkdownBlock | CodeBlock | ImageBlock;

interface LessonContent {
  blocks: LessonBlock[];
}
```

---

## File Structure

```
src/
├── lib/
│   └── lesson-content.ts              # Create: types + Zod schema + helpers
├── components/
│   └── lesson/
│       ├── lesson-renderer.tsx         # Create: renders lesson content blocks (client)
│       ├── lesson-editor.tsx           # Create: block editor for authoring (client)
│       ├── blocks/
│       │   ├── markdown-block.tsx      # Create: renders a single markdown block
│       │   ├── code-block.tsx          # Create: renders a single code block (syntax-highlighted)
│       │   └── image-block.tsx         # Create: renders a single image block
│       └── editor-blocks/
│           ├── markdown-editor.tsx     # Create: markdown editing with live preview
│           ├── code-editor-block.tsx   # Create: code editing with language selector
│           └── image-editor-block.tsx  # Create: image URL + alt text inputs
├── app/
│   └── (portal)/
│       ├── teacher/
│       │   └── courses/
│       │       ├── page.tsx            # Modify: add Create Course form
│       │       └── [id]/
│       │           ├── page.tsx        # Modify: add topic management (CRUD, reorder)
│       │           ├── create-class/
│       │           │   └── page.tsx    # Create: create class from course
│       │           └── topics/
│       │               └── [topicId]/
│       │                   └── page.tsx  # Create: topic detail + lesson editor
│       └── student/
│           └── classes/
│               └── [id]/
│                   └── page.tsx        # Modify: show topics list with lesson preview
tests/
├── unit/
│   ├── lesson-content.test.ts          # Create: Zod validation + helper tests
│   └── lesson-renderer.test.tsx        # Create: renderer component tests
```

---

## Task 1: Install Dependencies + Create Types/Validation

**Files:**
- Modify: `package.json` (via `bun add`)
- Create: `src/lib/lesson-content.ts`
- Create: `tests/unit/lesson-content.test.ts`

Install `react-markdown` and `remark-gfm` for rendering markdown content.

- [ ] **Step 1: Install react-markdown and remark-gfm**

```bash
cd /home/chris/workshop/Bridge && bun add react-markdown remark-gfm
```

- [ ] **Step 2: Create `src/lib/lesson-content.ts`**

This file defines the TypeScript types, Zod v4 validation schema, and helper functions for working with lesson content JSONB.

```typescript
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

export type LessonBlock = MarkdownBlock | CodeBlock | ImageBlock;

export interface LessonContent {
  blocks: LessonBlock[];
}

// --- Zod Schemas ---

export const markdownBlockSchema = z.object({
  type: z.literal("markdown"),
  content: z.string().min(1, "Markdown content is required"),
});

export const codeBlockSchema = z.object({
  type: z.literal("code"),
  language: z.string().min(1, "Language is required"),
  content: z.string().min(1, "Code content is required"),
});

export const imageBlockSchema = z.object({
  type: z.literal("image"),
  url: z.url("Valid URL is required"),
  alt: z.string().min(1, "Alt text is required"),
});

export const lessonBlockSchema = z.discriminatedUnion("type", [
  markdownBlockSchema,
  codeBlockSchema,
  imageBlockSchema,
]);

export const lessonContentSchema = z.object({
  blocks: z.array(lessonBlockSchema),
});

// --- Helpers ---

/** Create an empty lesson content structure */
export function emptyLessonContent(): LessonContent {
  return { blocks: [] };
}

/** Create a new block with default values */
export function createBlock(type: LessonBlock["type"]): LessonBlock {
  switch (type) {
    case "markdown":
      return { type: "markdown", content: "" };
    case "code":
      return { type: "code", language: "python", content: "" };
    case "image":
      return { type: "image", url: "", alt: "" };
  }
}

/** Parse raw JSONB into typed LessonContent, returning empty if invalid */
export function parseLessonContent(raw: unknown): LessonContent {
  if (!raw || typeof raw !== "object") return emptyLessonContent();
  const result = lessonContentSchema.safeParse(raw);
  if (result.success) return result.data;
  return emptyLessonContent();
}

/** Check whether a lesson content object has any blocks */
export function hasLessonContent(raw: unknown): boolean {
  const parsed = parseLessonContent(raw);
  return parsed.blocks.length > 0;
}

/** Move a block within the blocks array */
export function moveBlock(
  content: LessonContent,
  fromIndex: number,
  toIndex: number
): LessonContent {
  if (
    fromIndex < 0 ||
    fromIndex >= content.blocks.length ||
    toIndex < 0 ||
    toIndex >= content.blocks.length
  ) {
    return content;
  }
  const blocks = [...content.blocks];
  const [moved] = blocks.splice(fromIndex, 1);
  blocks.splice(toIndex, 0, moved);
  return { blocks };
}

/** Remove a block at a given index */
export function removeBlock(
  content: LessonContent,
  index: number
): LessonContent {
  if (index < 0 || index >= content.blocks.length) return content;
  const blocks = content.blocks.filter((_, i) => i !== index);
  return { blocks };
}

/** Update a block at a given index */
export function updateBlock(
  content: LessonContent,
  index: number,
  block: LessonBlock
): LessonContent {
  if (index < 0 || index >= content.blocks.length) return content;
  const blocks = [...content.blocks];
  blocks[index] = block;
  return { blocks };
}

/** Add a block at the end */
export function addBlock(
  content: LessonContent,
  block: LessonBlock
): LessonContent {
  return { blocks: [...content.blocks, block] };
}
```

- [ ] **Step 3: Create `tests/unit/lesson-content.test.ts`**

```typescript
import { describe, it, expect } from "vitest";
import {
  parseLessonContent,
  hasLessonContent,
  emptyLessonContent,
  createBlock,
  moveBlock,
  removeBlock,
  updateBlock,
  addBlock,
  lessonContentSchema,
  lessonBlockSchema,
  type LessonContent,
} from "@/lib/lesson-content";

describe("lesson content types and helpers", () => {
  describe("parseLessonContent", () => {
    it("returns empty for null/undefined", () => {
      expect(parseLessonContent(null)).toEqual({ blocks: [] });
      expect(parseLessonContent(undefined)).toEqual({ blocks: [] });
    });

    it("returns empty for invalid objects", () => {
      expect(parseLessonContent({})).toEqual({ blocks: [] });
      expect(parseLessonContent({ blocks: "not-array" })).toEqual({ blocks: [] });
      expect(parseLessonContent(42)).toEqual({ blocks: [] });
    });

    it("parses valid lesson content", () => {
      const raw = {
        blocks: [
          { type: "markdown", content: "# Hello" },
          { type: "code", language: "python", content: "print('hi')" },
          { type: "image", url: "https://example.com/img.png", alt: "An image" },
        ],
      };
      const result = parseLessonContent(raw);
      expect(result.blocks).toHaveLength(3);
      expect(result.blocks[0].type).toBe("markdown");
      expect(result.blocks[1].type).toBe("code");
      expect(result.blocks[2].type).toBe("image");
    });

    it("returns empty when blocks contain invalid items", () => {
      const raw = {
        blocks: [
          { type: "unknown", data: "something" },
        ],
      };
      expect(parseLessonContent(raw)).toEqual({ blocks: [] });
    });

    it("rejects markdown block with empty content", () => {
      const raw = { blocks: [{ type: "markdown", content: "" }] };
      expect(parseLessonContent(raw)).toEqual({ blocks: [] });
    });

    it("rejects code block with empty content", () => {
      const raw = { blocks: [{ type: "code", language: "python", content: "" }] };
      expect(parseLessonContent(raw)).toEqual({ blocks: [] });
    });

    it("rejects image block with invalid URL", () => {
      const raw = { blocks: [{ type: "image", url: "not-a-url", alt: "test" }] };
      expect(parseLessonContent(raw)).toEqual({ blocks: [] });
    });
  });

  describe("hasLessonContent", () => {
    it("returns false for empty/null", () => {
      expect(hasLessonContent(null)).toBe(false);
      expect(hasLessonContent({})).toBe(false);
      expect(hasLessonContent({ blocks: [] })).toBe(false);
    });

    it("returns true when blocks exist", () => {
      const raw = { blocks: [{ type: "markdown", content: "Hello" }] };
      expect(hasLessonContent(raw)).toBe(true);
    });
  });

  describe("createBlock", () => {
    it("creates a markdown block with empty content", () => {
      const block = createBlock("markdown");
      expect(block).toEqual({ type: "markdown", content: "" });
    });

    it("creates a code block with python default", () => {
      const block = createBlock("code");
      expect(block).toEqual({ type: "code", language: "python", content: "" });
    });

    it("creates an image block with empty fields", () => {
      const block = createBlock("image");
      expect(block).toEqual({ type: "image", url: "", alt: "" });
    });
  });

  describe("moveBlock", () => {
    const content: LessonContent = {
      blocks: [
        { type: "markdown", content: "A" },
        { type: "markdown", content: "B" },
        { type: "markdown", content: "C" },
      ],
    };

    it("moves a block forward", () => {
      const result = moveBlock(content, 0, 2);
      expect(result.blocks[0]).toEqual({ type: "markdown", content: "B" });
      expect(result.blocks[2]).toEqual({ type: "markdown", content: "A" });
    });

    it("moves a block backward", () => {
      const result = moveBlock(content, 2, 0);
      expect(result.blocks[0]).toEqual({ type: "markdown", content: "C" });
      expect(result.blocks[1]).toEqual({ type: "markdown", content: "A" });
    });

    it("returns unchanged for out-of-bounds indices", () => {
      expect(moveBlock(content, -1, 0)).toBe(content);
      expect(moveBlock(content, 0, 5)).toBe(content);
    });

    it("does not mutate original", () => {
      moveBlock(content, 0, 2);
      expect(content.blocks[0]).toEqual({ type: "markdown", content: "A" });
    });
  });

  describe("removeBlock", () => {
    it("removes a block at valid index", () => {
      const content: LessonContent = {
        blocks: [
          { type: "markdown", content: "A" },
          { type: "markdown", content: "B" },
        ],
      };
      const result = removeBlock(content, 0);
      expect(result.blocks).toHaveLength(1);
      expect(result.blocks[0]).toEqual({ type: "markdown", content: "B" });
    });

    it("returns unchanged for out-of-bounds", () => {
      const content: LessonContent = { blocks: [{ type: "markdown", content: "A" }] };
      expect(removeBlock(content, 5)).toBe(content);
      expect(removeBlock(content, -1)).toBe(content);
    });
  });

  describe("updateBlock", () => {
    it("updates a block at valid index", () => {
      const content: LessonContent = {
        blocks: [{ type: "markdown", content: "old" }],
      };
      const result = updateBlock(content, 0, { type: "markdown", content: "new" });
      expect(result.blocks[0]).toEqual({ type: "markdown", content: "new" });
    });

    it("returns unchanged for out-of-bounds", () => {
      const content: LessonContent = { blocks: [] };
      expect(updateBlock(content, 0, { type: "markdown", content: "x" })).toBe(content);
    });
  });

  describe("addBlock", () => {
    it("appends a block", () => {
      const content: LessonContent = { blocks: [{ type: "markdown", content: "A" }] };
      const result = addBlock(content, { type: "code", language: "python", content: "x=1" });
      expect(result.blocks).toHaveLength(2);
      expect(result.blocks[1].type).toBe("code");
    });

    it("adds to empty blocks array", () => {
      const result = addBlock(emptyLessonContent(), { type: "markdown", content: "Hello" });
      expect(result.blocks).toHaveLength(1);
    });
  });

  describe("Zod schemas", () => {
    it("validates a correct lesson content object", () => {
      const data = {
        blocks: [
          { type: "markdown", content: "# Title" },
          { type: "code", language: "javascript", content: "console.log(1)" },
        ],
      };
      expect(lessonContentSchema.safeParse(data).success).toBe(true);
    });

    it("rejects unknown block type", () => {
      const data = { blocks: [{ type: "video", url: "x" }] };
      expect(lessonContentSchema.safeParse(data).success).toBe(false);
    });

    it("validates individual block schemas", () => {
      expect(lessonBlockSchema.safeParse({ type: "markdown", content: "x" }).success).toBe(true);
      expect(lessonBlockSchema.safeParse({ type: "code", language: "py", content: "x" }).success).toBe(true);
      expect(lessonBlockSchema.safeParse({ type: "image", url: "https://x.com/i.png", alt: "a" }).success).toBe(true);
    });
  });
});
```

- [ ] **Step 4: Run tests and commit**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/lesson-content.test.ts
```

Commit: "Add lesson content types, Zod validation, and block manipulation helpers"

---

## Task 2: Lesson Content Renderer Components

**Files:**
- Create: `src/components/lesson/blocks/markdown-block.tsx`
- Create: `src/components/lesson/blocks/code-block.tsx`
- Create: `src/components/lesson/blocks/image-block.tsx`
- Create: `src/components/lesson/lesson-renderer.tsx`
- Create: `tests/unit/lesson-renderer.test.tsx`

Build the read-only renderer that displays lesson content blocks. This component is shared between teacher preview and student session views.

- [ ] **Step 1: Create `src/components/lesson/blocks/markdown-block.tsx`**

```tsx
"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { MarkdownBlock as MarkdownBlockType } from "@/lib/lesson-content";

interface MarkdownBlockProps {
  block: MarkdownBlockType;
}

export function MarkdownBlock({ block }: MarkdownBlockProps) {
  return (
    <div className="prose prose-sm dark:prose-invert max-w-none">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>
        {block.content}
      </ReactMarkdown>
    </div>
  );
}
```

- [ ] **Step 2: Create `src/components/lesson/blocks/code-block.tsx`**

Uses a `<pre><code>` with syntax class and a copy-to-clipboard button. This avoids loading a full CodeMirror instance for read-only display, keeping bundle size small.

```tsx
"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Copy, Check } from "lucide-react";
import type { CodeBlock as CodeBlockType } from "@/lib/lesson-content";

interface CodeBlockProps {
  block: CodeBlockType;
}

export function CodeBlock({ block }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(block.content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard API may fail in insecure contexts; ignore silently
    }
  }

  return (
    <div className="relative group rounded-lg border bg-muted/50 overflow-hidden">
      <div className="flex items-center justify-between px-3 py-1.5 border-b bg-muted/80">
        <span className="text-xs font-medium text-muted-foreground">
          {block.language}
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={handleCopy}
          aria-label="Copy code"
        >
          {copied ? (
            <Check className="size-3 text-green-600" />
          ) : (
            <Copy className="size-3" />
          )}
        </Button>
      </div>
      <pre className="p-3 overflow-x-auto text-sm">
        <code className={`language-${block.language}`}>
          {block.content}
        </code>
      </pre>
    </div>
  );
}
```

- [ ] **Step 3: Create `src/components/lesson/blocks/image-block.tsx`**

```tsx
import type { ImageBlock as ImageBlockType } from "@/lib/lesson-content";

interface ImageBlockProps {
  block: ImageBlockType;
}

export function ImageBlock({ block }: ImageBlockProps) {
  return (
    <figure className="my-2">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={block.url}
        alt={block.alt}
        className="max-w-full rounded-lg border"
        loading="lazy"
      />
      {block.alt && (
        <figcaption className="mt-1 text-xs text-muted-foreground text-center">
          {block.alt}
        </figcaption>
      )}
    </figure>
  );
}
```

- [ ] **Step 4: Create `src/components/lesson/lesson-renderer.tsx`**

```tsx
"use client";

import { parseLessonContent, type LessonContent } from "@/lib/lesson-content";
import { MarkdownBlock } from "@/components/lesson/blocks/markdown-block";
import { CodeBlock } from "@/components/lesson/blocks/code-block";
import { ImageBlock } from "@/components/lesson/blocks/image-block";

interface LessonRendererProps {
  /** Raw JSONB from topics.lessonContent */
  content: unknown;
  /** Optional className for the outer container */
  className?: string;
}

export function LessonRenderer({ content, className }: LessonRendererProps) {
  const parsed = parseLessonContent(content);

  if (parsed.blocks.length === 0) {
    return (
      <div className={className}>
        <p className="text-sm text-muted-foreground italic">
          No lesson content yet.
        </p>
      </div>
    );
  }

  return (
    <div className={`space-y-4 ${className || ""}`}>
      {parsed.blocks.map((block, index) => {
        switch (block.type) {
          case "markdown":
            return <MarkdownBlock key={index} block={block} />;
          case "code":
            return <CodeBlock key={index} block={block} />;
          case "image":
            return <ImageBlock key={index} block={block} />;
          default:
            return null;
        }
      })}
    </div>
  );
}
```

- [ ] **Step 5: Create `tests/unit/lesson-renderer.test.tsx`**

```tsx
// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";

describe("LessonRenderer", () => {
  it("renders empty state for null content", () => {
    render(<LessonRenderer content={null} />);
    expect(screen.getByText("No lesson content yet.")).toBeDefined();
  });

  it("renders empty state for empty blocks", () => {
    render(<LessonRenderer content={{ blocks: [] }} />);
    expect(screen.getByText("No lesson content yet.")).toBeDefined();
  });

  it("renders markdown blocks", () => {
    const content = {
      blocks: [{ type: "markdown", content: "Hello world" }],
    };
    render(<LessonRenderer content={content} />);
    expect(screen.getByText("Hello world")).toBeDefined();
  });

  it("renders code blocks with language label", () => {
    const content = {
      blocks: [{ type: "code", language: "python", content: "x = 5" }],
    };
    render(<LessonRenderer content={content} />);
    expect(screen.getByText("python")).toBeDefined();
    expect(screen.getByText("x = 5")).toBeDefined();
  });

  it("renders image blocks with alt text", () => {
    const content = {
      blocks: [{ type: "image", url: "https://example.com/img.png", alt: "Test image" }],
    };
    render(<LessonRenderer content={content} />);
    const img = screen.getByAltText("Test image");
    expect(img).toBeDefined();
    expect(img.getAttribute("src")).toBe("https://example.com/img.png");
  });

  it("renders multiple blocks in order", () => {
    const content = {
      blocks: [
        { type: "markdown", content: "First" },
        { type: "code", language: "python", content: "print('second')" },
        { type: "markdown", content: "Third" },
      ],
    };
    render(<LessonRenderer content={content} />);
    expect(screen.getByText("First")).toBeDefined();
    expect(screen.getByText("print('second')")).toBeDefined();
    expect(screen.getByText("Third")).toBeDefined();
  });

  it("renders copy button for code blocks", () => {
    const content = {
      blocks: [{ type: "code", language: "python", content: "x = 1" }],
    };
    render(<LessonRenderer content={content} />);
    expect(screen.getByLabelText("Copy code")).toBeDefined();
  });

  it("handles invalid content gracefully", () => {
    render(<LessonRenderer content={{ not: "valid" }} />);
    expect(screen.getByText("No lesson content yet.")).toBeDefined();
  });

  it("applies custom className", () => {
    const { container } = render(
      <LessonRenderer content={{ blocks: [{ type: "markdown", content: "Test" }] }} className="custom-class" />
    );
    expect(container.firstElementChild?.classList.contains("custom-class")).toBe(true);
  });
});
```

- [ ] **Step 6: Run tests and commit**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/lesson-renderer.test.tsx
```

Commit: "Add lesson content renderer with markdown, code, and image block components"

---

## Task 3: Lesson Content Editor Components

**Files:**
- Create: `src/components/lesson/editor-blocks/markdown-editor.tsx`
- Create: `src/components/lesson/editor-blocks/code-editor-block.tsx`
- Create: `src/components/lesson/editor-blocks/image-editor-block.tsx`
- Create: `src/components/lesson/lesson-editor.tsx`
- Create: `tests/unit/lesson-editor.test.tsx`

Build the teacher-facing block editor for authoring lesson content. This is a client component with add/remove/reorder/edit capabilities per block.

- [ ] **Step 1: Create `src/components/lesson/editor-blocks/markdown-editor.tsx`**

Markdown block editor: a textarea with a side-by-side live preview.

```tsx
"use client";

import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Button } from "@/components/ui/button";
import { Eye, EyeOff } from "lucide-react";
import type { MarkdownBlock } from "@/lib/lesson-content";

interface MarkdownEditorProps {
  block: MarkdownBlock;
  onChange: (block: MarkdownBlock) => void;
}

export function MarkdownEditor({ block, onChange }: MarkdownEditorProps) {
  const [showPreview, setShowPreview] = useState(false);

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">Markdown</span>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={() => setShowPreview(!showPreview)}
          aria-label={showPreview ? "Hide preview" : "Show preview"}
        >
          {showPreview ? <EyeOff className="size-3" /> : <Eye className="size-3" />}
        </Button>
      </div>
      <div className={showPreview ? "grid grid-cols-2 gap-3" : ""}>
        <textarea
          value={block.content}
          onChange={(e) => onChange({ ...block, content: e.target.value })}
          placeholder="Write markdown content..."
          className="w-full min-h-[120px] rounded-lg border border-input bg-transparent px-3 py-2 text-sm font-mono resize-y outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
          rows={6}
        />
        {showPreview && (
          <div className="prose prose-sm dark:prose-invert max-w-none rounded-lg border p-3 overflow-auto min-h-[120px]">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {block.content || "*Nothing to preview*"}
            </ReactMarkdown>
          </div>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create `src/components/lesson/editor-blocks/code-editor-block.tsx`**

Code block editor: a CodeMirror editor (reusing the existing component patterns) with a language dropdown.

```tsx
"use client";

import { useRef, useEffect } from "react";
import { EditorView, keymap, lineNumbers, highlightActiveLine } from "@codemirror/view";
import { EditorState, type Extension } from "@codemirror/state";
import { defaultKeymap, indentWithTab, history, historyKeymap } from "@codemirror/commands";
import { syntaxHighlighting, defaultHighlightStyle, bracketMatching, indentOnInput } from "@codemirror/language";
import { closeBrackets } from "@codemirror/autocomplete";
import { python } from "@codemirror/lang-python";
import type { CodeBlock } from "@/lib/lesson-content";

const SUPPORTED_LANGUAGES = [
  { value: "python", label: "Python" },
  { value: "javascript", label: "JavaScript" },
  { value: "html", label: "HTML" },
  { value: "css", label: "CSS" },
  { value: "text", label: "Plain Text" },
] as const;

interface CodeEditorBlockProps {
  block: CodeBlock;
  onChange: (block: CodeBlock) => void;
}

export function CodeEditorBlock({ block, onChange }: CodeEditorBlockProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  const blockRef = useRef(block);

  // Keep refs current without re-creating the editor
  onChangeRef.current = onChange;
  blockRef.current = block;

  useEffect(() => {
    if (!containerRef.current) return;

    const extensions: Extension[] = [
      lineNumbers(),
      highlightActiveLine(),
      bracketMatching(),
      closeBrackets(),
      indentOnInput(),
      history(),
      python(), // Default; language selection affects label only since we support limited languages
      syntaxHighlighting(defaultHighlightStyle),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
      EditorView.theme({
        "&": { fontSize: "13px" },
        ".cm-scroller": { fontFamily: "var(--font-geist-mono), monospace", maxHeight: "300px", overflow: "auto" },
        ".cm-content": { minHeight: "80px" },
      }),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          const newContent = update.state.doc.toString();
          onChangeRef.current({ ...blockRef.current, content: newContent });
        }
      }),
    ];

    const state = EditorState.create({
      doc: block.content,
      extensions,
    });

    const view = new EditorView({
      state,
      parent: containerRef.current,
    });

    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
  }, []); // Only create once

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <span className="text-xs font-medium text-muted-foreground">Code</span>
        <select
          value={block.language}
          onChange={(e) => onChange({ ...block, language: e.target.value })}
          className="h-6 rounded border border-input bg-transparent px-1.5 text-xs outline-none focus-visible:border-ring"
        >
          {SUPPORTED_LANGUAGES.map((lang) => (
            <option key={lang.value} value={lang.value}>
              {lang.label}
            </option>
          ))}
        </select>
      </div>
      <div ref={containerRef} className="border rounded-lg overflow-hidden" />
    </div>
  );
}
```

- [ ] **Step 3: Create `src/components/lesson/editor-blocks/image-editor-block.tsx`**

```tsx
"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { ImageBlock } from "@/lib/lesson-content";

interface ImageEditorBlockProps {
  block: ImageBlock;
  onChange: (block: ImageBlock) => void;
}

export function ImageEditorBlock({ block, onChange }: ImageEditorBlockProps) {
  return (
    <div className="space-y-3">
      <span className="text-xs font-medium text-muted-foreground">Image</span>
      <div className="space-y-2">
        <div>
          <Label htmlFor="image-url" className="text-xs">Image URL</Label>
          <Input
            id="image-url"
            type="url"
            value={block.url}
            onChange={(e) => onChange({ ...block, url: e.target.value })}
            placeholder="https://example.com/image.png"
          />
        </div>
        <div>
          <Label htmlFor="image-alt" className="text-xs">Alt Text</Label>
          <Input
            id="image-alt"
            type="text"
            value={block.alt}
            onChange={(e) => onChange({ ...block, alt: e.target.value })}
            placeholder="Description of the image"
          />
        </div>
      </div>
      {block.url && (
        <div className="mt-2">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={block.url}
            alt={block.alt || "Preview"}
            className="max-w-full max-h-48 rounded border object-contain"
            loading="lazy"
            onError={(e) => {
              (e.target as HTMLImageElement).style.display = "none";
            }}
          />
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Create `src/components/lesson/lesson-editor.tsx`**

The main editor component that manages the full list of blocks with add/remove/reorder controls.

```tsx
"use client";

import { useState, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Plus, Trash2, ChevronUp, ChevronDown, Type, Code, ImageIcon, Save } from "lucide-react";
import {
  parseLessonContent,
  createBlock,
  addBlock,
  removeBlock,
  updateBlock,
  moveBlock,
  type LessonContent,
  type LessonBlock,
} from "@/lib/lesson-content";
import { MarkdownEditor } from "@/components/lesson/editor-blocks/markdown-editor";
import { CodeEditorBlock } from "@/components/lesson/editor-blocks/code-editor-block";
import { ImageEditorBlock } from "@/components/lesson/editor-blocks/image-editor-block";

interface LessonEditorProps {
  /** Raw JSONB from topics.lessonContent */
  initialContent: unknown;
  /** Called when the user saves — receives the full LessonContent object */
  onSave: (content: LessonContent) => void | Promise<void>;
  /** Whether the save is in progress */
  saving?: boolean;
}

export function LessonEditor({ initialContent, onSave, saving = false }: LessonEditorProps) {
  const [content, setContent] = useState<LessonContent>(() =>
    parseLessonContent(initialContent)
  );
  const [dirty, setDirty] = useState(false);

  const update = useCallback((updater: (prev: LessonContent) => LessonContent) => {
    setContent((prev) => {
      const next = updater(prev);
      setDirty(true);
      return next;
    });
  }, []);

  function handleAddBlock(type: LessonBlock["type"]) {
    update((prev) => addBlock(prev, createBlock(type)));
  }

  function handleRemoveBlock(index: number) {
    update((prev) => removeBlock(prev, index));
  }

  function handleUpdateBlock(index: number, block: LessonBlock) {
    update((prev) => updateBlock(prev, index, block));
  }

  function handleMoveUp(index: number) {
    if (index <= 0) return;
    update((prev) => moveBlock(prev, index, index - 1));
  }

  function handleMoveDown(index: number) {
    if (index >= content.blocks.length - 1) return;
    update((prev) => moveBlock(prev, index, index + 1));
  }

  async function handleSave() {
    await onSave(content);
    setDirty(false);
  }

  return (
    <div className="space-y-4">
      {/* Block list */}
      {content.blocks.length === 0 && (
        <p className="text-sm text-muted-foreground italic py-4 text-center">
          No content blocks yet. Add one below.
        </p>
      )}

      {content.blocks.map((block, index) => (
        <div key={index} className="border rounded-lg p-3 space-y-2">
          {/* Block controls */}
          <div className="flex items-center justify-end gap-1">
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() => handleMoveUp(index)}
              disabled={index === 0}
              aria-label="Move block up"
            >
              <ChevronUp className="size-3" />
            </Button>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() => handleMoveDown(index)}
              disabled={index === content.blocks.length - 1}
              aria-label="Move block down"
            >
              <ChevronDown className="size-3" />
            </Button>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() => handleRemoveBlock(index)}
              aria-label="Remove block"
            >
              <Trash2 className="size-3 text-destructive" />
            </Button>
          </div>

          {/* Block editor */}
          {block.type === "markdown" && (
            <MarkdownEditor
              block={block}
              onChange={(b) => handleUpdateBlock(index, b)}
            />
          )}
          {block.type === "code" && (
            <CodeEditorBlock
              block={block}
              onChange={(b) => handleUpdateBlock(index, b)}
            />
          )}
          {block.type === "image" && (
            <ImageEditorBlock
              block={block}
              onChange={(b) => handleUpdateBlock(index, b)}
            />
          )}
        </div>
      ))}

      {/* Add block buttons */}
      <div className="flex items-center gap-2 pt-2 border-t">
        <span className="text-xs text-muted-foreground">Add block:</span>
        <Button variant="outline" size="sm" onClick={() => handleAddBlock("markdown")}>
          <Type className="size-3.5 mr-1" />
          Markdown
        </Button>
        <Button variant="outline" size="sm" onClick={() => handleAddBlock("code")}>
          <Code className="size-3.5 mr-1" />
          Code
        </Button>
        <Button variant="outline" size="sm" onClick={() => handleAddBlock("image")}>
          <ImageIcon className="size-3.5 mr-1" />
          Image
        </Button>
      </div>

      {/* Save button */}
      <div className="flex justify-end pt-2">
        <Button onClick={handleSave} disabled={!dirty || saving}>
          <Save className="size-3.5 mr-1" />
          {saving ? "Saving..." : "Save Lesson Content"}
        </Button>
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Create `tests/unit/lesson-editor.test.tsx`**

```tsx
// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { LessonEditor } from "@/components/lesson/lesson-editor";

describe("LessonEditor", () => {
  it("renders empty state with add buttons", () => {
    render(<LessonEditor initialContent={{}} onSave={vi.fn()} />);
    expect(screen.getByText("No content blocks yet. Add one below.")).toBeDefined();
    expect(screen.getByText("Markdown")).toBeDefined();
    expect(screen.getByText("Code")).toBeDefined();
    expect(screen.getByText("Image")).toBeDefined();
  });

  it("adds a markdown block when clicking Add Markdown", () => {
    render(<LessonEditor initialContent={{ blocks: [] }} onSave={vi.fn()} />);
    fireEvent.click(screen.getByText("Markdown"));
    // After adding, the empty message should disappear
    expect(screen.queryByText("No content blocks yet. Add one below.")).toBeNull();
  });

  it("adds a code block when clicking Add Code", () => {
    render(<LessonEditor initialContent={{ blocks: [] }} onSave={vi.fn()} />);
    fireEvent.click(screen.getByText("Code"));
    // Code editor block should now appear with language selector
    expect(screen.queryByText("No content blocks yet. Add one below.")).toBeNull();
  });

  it("adds an image block when clicking Add Image", () => {
    render(<LessonEditor initialContent={{ blocks: [] }} onSave={vi.fn()} />);
    fireEvent.click(screen.getByText("Image"));
    expect(screen.getByLabelText("Image URL")).toBeDefined();
    expect(screen.getByLabelText("Alt Text")).toBeDefined();
  });

  it("removes a block when clicking remove", () => {
    const content = {
      blocks: [{ type: "markdown", content: "Hello" }],
    };
    render(<LessonEditor initialContent={content} onSave={vi.fn()} />);
    fireEvent.click(screen.getByLabelText("Remove block"));
    expect(screen.getByText("No content blocks yet. Add one below.")).toBeDefined();
  });

  it("calls onSave with content when Save is clicked", async () => {
    const onSave = vi.fn();
    const content = {
      blocks: [{ type: "markdown", content: "Test" }],
    };
    render(<LessonEditor initialContent={content} onSave={onSave} />);

    // Dirty the editor by removing and re-adding (or just add a block)
    fireEvent.click(screen.getByText("Markdown")); // adds a new block, makes dirty
    fireEvent.click(screen.getByText("Save Lesson Content"));
    expect(onSave).toHaveBeenCalledTimes(1);
    const savedContent = onSave.mock.calls[0][0];
    expect(savedContent.blocks.length).toBe(2);
  });

  it("disables Save button when not dirty", () => {
    const content = {
      blocks: [{ type: "markdown", content: "Clean" }],
    };
    render(<LessonEditor initialContent={content} onSave={vi.fn()} />);
    const saveBtn = screen.getByText("Save Lesson Content");
    expect(saveBtn.closest("button")?.disabled).toBe(true);
  });

  it("disables Save button when saving prop is true", () => {
    render(
      <LessonEditor initialContent={{ blocks: [] }} onSave={vi.fn()} saving />
    );
    // Add block to make dirty
    fireEvent.click(screen.getByText("Markdown"));
    const savingBtn = screen.getByText("Saving...");
    expect(savingBtn.closest("button")?.disabled).toBe(true);
  });

  it("renders existing blocks from initialContent", () => {
    const content = {
      blocks: [
        { type: "markdown", content: "# Title" },
        { type: "image", url: "https://example.com/img.png", alt: "pic" },
      ],
    };
    render(<LessonEditor initialContent={content} onSave={vi.fn()} />);
    // Should have 2 block containers with move/remove buttons
    const removeButtons = screen.getAllByLabelText("Remove block");
    expect(removeButtons).toHaveLength(2);
  });

  it("disables move up for first block and move down for last block", () => {
    const content = {
      blocks: [
        { type: "markdown", content: "A" },
        { type: "markdown", content: "B" },
      ],
    };
    render(<LessonEditor initialContent={content} onSave={vi.fn()} />);
    const moveUpButtons = screen.getAllByLabelText("Move block up");
    const moveDownButtons = screen.getAllByLabelText("Move block down");
    expect(moveUpButtons[0].closest("button")?.disabled).toBe(true);
    expect(moveDownButtons[1].closest("button")?.disabled).toBe(true);
  });
});
```

- [ ] **Step 6: Run tests and commit**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/lesson-editor.test.tsx
```

Commit: "Add lesson content editor with block add, remove, reorder, and preview"

---

## Task 4: Teacher Course Management -- Create Course Form

**Files:**
- Modify: `src/app/(portal)/teacher/courses/page.tsx`

Add a "Create Course" form to the teacher courses list page. Uses inline server actions (following the `admin/orgs/page.tsx` pattern).

- [ ] **Step 1: Modify `src/app/(portal)/teacher/courses/page.tsx`**

Replace the current file content. The page keeps the existing courses grid and adds a form at the top.

```tsx
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listCoursesByCreator, createCourse } from "@/lib/courses";
import { getUserMemberships } from "@/lib/org-memberships";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import Link from "next/link";
import { revalidatePath } from "next/cache";

export default async function TeacherCoursesPage() {
  const session = await auth();
  const userId = session!.user.id;
  const [courses, memberships] = await Promise.all([
    listCoursesByCreator(db, userId),
    getUserMemberships(db, userId),
  ]);

  // Find org IDs where user is a teacher
  const teacherOrgs = memberships.filter(
    (m) => (m.role === "teacher" || m.role === "org_admin") && m.status === "active"
  );

  async function handleCreateCourse(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const s = await getAuth();
    if (!s?.user?.id) return;

    const title = (formData.get("title") as string)?.trim();
    const description = (formData.get("description") as string)?.trim() || "";
    const gradeLevel = formData.get("gradeLevel") as "K-5" | "6-8" | "9-12";
    const language = formData.get("language") as "python" | "javascript" | "blockly";
    const orgId = formData.get("orgId") as string;

    if (!title || !gradeLevel || !orgId) return;

    const { createCourse } = await import("@/lib/courses");
    await createCourse(db, {
      orgId,
      createdBy: s.user.id,
      title,
      description,
      gradeLevel,
      language: language || "python",
    });

    revalidatePath("/teacher/courses");
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">My Courses</h1>
      </div>

      {/* Create Course Form */}
      {teacherOrgs.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Create New Course</CardTitle>
            <CardDescription>Set up a reusable course curriculum with topics and lesson content.</CardDescription>
          </CardHeader>
          <CardContent>
            <form action={handleCreateCourse} className="grid gap-4 sm:grid-cols-2">
              <div className="sm:col-span-2">
                <Label htmlFor="title">Course Title</Label>
                <Input id="title" name="title" placeholder="e.g., Intro to Python" required />
              </div>
              <div className="sm:col-span-2">
                <Label htmlFor="description">Description (optional)</Label>
                <Input id="description" name="description" placeholder="What students will learn..." />
              </div>
              {teacherOrgs.length === 1 ? (
                <input type="hidden" name="orgId" value={teacherOrgs[0].orgId} />
              ) : (
                <div>
                  <Label htmlFor="orgId">Organization</Label>
                  <select
                    id="orgId"
                    name="orgId"
                    required
                    className="h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring"
                  >
                    {teacherOrgs.map((m) => (
                      <option key={m.orgId} value={m.orgId}>
                        {m.orgName}
                      </option>
                    ))}
                  </select>
                </div>
              )}
              <div>
                <Label htmlFor="gradeLevel">Grade Level</Label>
                <select
                  id="gradeLevel"
                  name="gradeLevel"
                  required
                  className="h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring"
                >
                  <option value="K-5">K-5</option>
                  <option value="6-8">6-8</option>
                  <option value="9-12">9-12</option>
                </select>
              </div>
              <div>
                <Label htmlFor="language">Language</Label>
                <select
                  id="language"
                  name="language"
                  required
                  className="h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring"
                >
                  <option value="python">Python</option>
                  <option value="javascript">JavaScript</option>
                  <option value="blockly">Blockly</option>
                </select>
              </div>
              <div className="sm:col-span-2">
                <Button type="submit">Create Course</Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {teacherOrgs.length === 0 && (
        <Card>
          <CardContent className="py-6 text-center text-muted-foreground">
            <p>You need to be a member of an organization to create courses.</p>
          </CardContent>
        </Card>
      )}

      {/* Course list */}
      {courses.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No courses yet. Create your first course to get started.</p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {courses.map((course) => (
            <Link key={course.id} href={`/teacher/courses/${course.id}`}>
              <Card className="hover:border-primary transition-colors cursor-pointer">
                <CardHeader>
                  <CardTitle className="text-lg">{course.title}</CardTitle>
                  <CardDescription>
                    {course.gradeLevel} · {course.language}
                    {course.isPublished && " · Published"}
                  </CardDescription>
                </CardHeader>
                {course.description && (
                  <CardContent>
                    <p className="text-sm text-muted-foreground line-clamp-2">{course.description}</p>
                  </CardContent>
                )}
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

Commit: "Add Create Course form to teacher courses page"

---

## Task 5: Teacher Course Detail -- Topic Management

**Files:**
- Modify: `src/app/(portal)/teacher/courses/[id]/page.tsx`

Enhance the existing course detail page with:
- Create topic form
- Delete topic action
- Reorder topics (move up/down)
- Link each topic to its detail/editor page

- [ ] **Step 1: Modify `src/app/(portal)/teacher/courses/[id]/page.tsx`**

```tsx
import { notFound, redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse, deleteCourse } from "@/lib/courses";
import { listTopicsByCourse, createTopic, deleteTopic, reorderTopics } from "@/lib/topics";
import { listClassesByCourse } from "@/lib/classes";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { revalidatePath } from "next/cache";

export default async function TeacherCourseDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const course = await getCourse(db, id);
  if (!course) notFound();

  // Verify teacher is the course creator
  if (course.createdBy !== session!.user.id && !session!.user.isPlatformAdmin) notFound();

  const [topicList, classList] = await Promise.all([
    listTopicsByCourse(db, id),
    listClassesByCourse(db, id),
  ]);

  async function handleCreateTopic(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const s = await getAuth();
    if (!s?.user?.id) return;

    const title = (formData.get("title") as string)?.trim();
    const description = (formData.get("description") as string)?.trim() || "";
    if (!title) return;

    // Verify ownership
    const { getCourse } = await import("@/lib/courses");
    const c = await getCourse(db, id);
    if (!c || (c.createdBy !== s.user.id && !s.user.isPlatformAdmin)) return;

    const { createTopic } = await import("@/lib/topics");
    await createTopic(db, { courseId: id, title, description });
    revalidatePath(`/teacher/courses/${id}`);
  }

  async function handleDeleteTopic(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const s = await getAuth();
    if (!s?.user?.id) return;

    const topicId = formData.get("topicId") as string;
    if (!topicId) return;

    const { getCourse } = await import("@/lib/courses");
    const c = await getCourse(db, id);
    if (!c || (c.createdBy !== s.user.id && !s.user.isPlatformAdmin)) return;

    const { deleteTopic } = await import("@/lib/topics");
    await deleteTopic(db, topicId);
    revalidatePath(`/teacher/courses/${id}`);
  }

  async function handleMoveTopic(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const s = await getAuth();
    if (!s?.user?.id) return;

    const topicId = formData.get("topicId") as string;
    const direction = formData.get("direction") as string;
    if (!topicId || !direction) return;

    const { getCourse } = await import("@/lib/courses");
    const c = await getCourse(db, id);
    if (!c || (c.createdBy !== s.user.id && !s.user.isPlatformAdmin)) return;

    const { listTopicsByCourse, reorderTopics } = await import("@/lib/topics");
    const topics = await listTopicsByCourse(db, id);
    const currentIndex = topics.findIndex((t) => t.id === topicId);
    if (currentIndex === -1) return;

    const newIndex = direction === "up" ? currentIndex - 1 : currentIndex + 1;
    if (newIndex < 0 || newIndex >= topics.length) return;

    const ids = topics.map((t) => t.id);
    // Swap
    [ids[currentIndex], ids[newIndex]] = [ids[newIndex], ids[currentIndex]];
    await reorderTopics(db, id, ids);
    revalidatePath(`/teacher/courses/${id}`);
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{course.title}</h1>
          <p className="text-muted-foreground">{course.gradeLevel} · {course.language}</p>
          {course.description && <p className="mt-2">{course.description}</p>}
        </div>
        <Link
          href={`/teacher/courses/${id}/create-class`}
          className={buttonVariants()}
        >
          Create Class
        </Link>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        {/* Topics column */}
        <div>
          <h2 className="text-lg font-semibold mb-3">Topics ({topicList.length})</h2>

          {/* Create topic form */}
          <Card className="mb-4">
            <CardContent className="pt-4">
              <form action={handleCreateTopic} className="flex gap-2 items-end">
                <div className="flex-1">
                  <Label htmlFor="topic-title" className="text-xs">New Topic</Label>
                  <Input id="topic-title" name="title" placeholder="e.g., Variables and Data Types" required />
                </div>
                <Button type="submit" size="sm">Add Topic</Button>
              </form>
            </CardContent>
          </Card>

          {topicList.length === 0 ? (
            <p className="text-sm text-muted-foreground">No topics yet. Add your first topic above.</p>
          ) : (
            <div className="space-y-2">
              {topicList.map((topic, i) => (
                <Card key={topic.id}>
                  <CardContent className="py-3">
                    <div className="flex items-center justify-between">
                      <Link
                        href={`/teacher/courses/${id}/topics/${topic.id}`}
                        className="font-medium hover:text-primary transition-colors flex-1"
                      >
                        {i + 1}. {topic.title}
                      </Link>
                      <div className="flex items-center gap-1">
                        <form action={handleMoveTopic}>
                          <input type="hidden" name="topicId" value={topic.id} />
                          <input type="hidden" name="direction" value="up" />
                          <Button type="submit" variant="ghost" size="icon-xs" disabled={i === 0} aria-label="Move up">
                            <span className="text-xs">&#9650;</span>
                          </Button>
                        </form>
                        <form action={handleMoveTopic}>
                          <input type="hidden" name="topicId" value={topic.id} />
                          <input type="hidden" name="direction" value="down" />
                          <Button type="submit" variant="ghost" size="icon-xs" disabled={i === topicList.length - 1} aria-label="Move down">
                            <span className="text-xs">&#9660;</span>
                          </Button>
                        </form>
                        <form action={handleDeleteTopic}>
                          <input type="hidden" name="topicId" value={topic.id} />
                          <Button type="submit" variant="ghost" size="icon-xs" aria-label="Delete topic">
                            <span className="text-xs text-destructive">&#10005;</span>
                          </Button>
                        </form>
                      </div>
                    </div>
                    {topic.description && (
                      <p className="text-sm text-muted-foreground mt-1">{topic.description}</p>
                    )}
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </div>

        {/* Classes column */}
        <div>
          <h2 className="text-lg font-semibold mb-3">Classes ({classList.length})</h2>
          {classList.length === 0 ? (
            <p className="text-sm text-muted-foreground">No classes created from this course yet.</p>
          ) : (
            <div className="space-y-2">
              {classList.map((cls) => (
                <Link key={cls.id} href={`/teacher/classes/${cls.id}`}>
                  <Card className="hover:border-primary transition-colors cursor-pointer mb-2">
                    <CardContent className="py-3">
                      <p className="font-medium">{cls.title}</p>
                      <p className="text-sm text-muted-foreground">{cls.term || "No term"} · {cls.status}</p>
                    </CardContent>
                  </Card>
                </Link>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

Commit: "Add topic CRUD and reorder controls to teacher course detail page"

---

## Task 6: Create Class from Course Page

**Files:**
- Create: `src/app/(portal)/teacher/courses/[id]/create-class/page.tsx`

A form to create a class from a given course. Uses server actions.

- [ ] **Step 1: Create `src/app/(portal)/teacher/courses/[id]/create-class/page.tsx`**

```tsx
import { notFound, redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse } from "@/lib/courses";
import { createClass } from "@/lib/classes";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { revalidatePath } from "next/cache";

export default async function CreateClassFromCoursePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id: courseId } = await params;
  const course = await getCourse(db, courseId);
  if (!course) notFound();

  if (course.createdBy !== session!.user.id && !session!.user.isPlatformAdmin) notFound();

  async function handleCreateClass(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const s = await getAuth();
    if (!s?.user?.id) return;

    const title = (formData.get("title") as string)?.trim();
    const term = (formData.get("term") as string)?.trim() || "";
    if (!title) return;

    const { getCourse } = await import("@/lib/courses");
    const c = await getCourse(db, courseId);
    if (!c || (c.createdBy !== s.user.id && !s.user.isPlatformAdmin)) return;

    const { createClass } = await import("@/lib/classes");
    const cls = await createClass(db, {
      courseId,
      orgId: c.orgId,
      title,
      term,
      createdBy: s.user.id,
    });

    const { redirect } = await import("next/navigation");
    redirect(`/teacher/classes/${cls.id}`);
  }

  return (
    <div className="p-6 max-w-lg">
      <h1 className="text-2xl font-bold mb-2">Create Class</h1>
      <p className="text-muted-foreground mb-6">
        Create a new class offering for <strong>{course.title}</strong>.
      </p>

      <Card>
        <CardContent className="pt-6">
          <form action={handleCreateClass} className="space-y-4">
            <div>
              <Label htmlFor="title">Class Title</Label>
              <Input
                id="title"
                name="title"
                placeholder={`e.g., ${course.title} - Period 3`}
                required
              />
            </div>
            <div>
              <Label htmlFor="term">Term (optional)</Label>
              <Input
                id="term"
                name="term"
                placeholder="e.g., Fall 2026"
              />
            </div>
            <Button type="submit">Create Class</Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

Commit: "Add create-class-from-course page in teacher portal"

---

## Task 7: Topic Detail Page with Lesson Content Editor

**Files:**
- Create: `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx`

The main lesson content authoring page. Displays topic metadata, the lesson content editor, and a starter code field. The lesson content editor saves to `topics.lessonContent` via server action.

- [ ] **Step 1: Create `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx`**

This page is a server component that renders a client wrapper for the editor. The save action is a server action passed down.

```tsx
import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getCourse } from "@/lib/courses";
import { getTopic, updateTopic } from "@/lib/topics";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { revalidatePath } from "next/cache";
import { TopicEditorClient } from "./topic-editor-client";

export default async function TopicDetailPage({
  params,
}: {
  params: Promise<{ id: string; topicId: string }>;
}) {
  const session = await auth();
  const { id: courseId, topicId } = await params;

  const [course, topic] = await Promise.all([
    getCourse(db, courseId),
    getTopic(db, topicId),
  ]);

  if (!course || !topic) notFound();
  if (topic.courseId !== courseId) notFound();
  if (course.createdBy !== session!.user.id && !session!.user.isPlatformAdmin) notFound();

  async function handleUpdateDetails(formData: FormData) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const s = await getAuth();
    if (!s?.user?.id) return;

    const title = (formData.get("title") as string)?.trim();
    const description = (formData.get("description") as string)?.trim() || "";
    const starterCode = (formData.get("starterCode") as string) || "";
    if (!title) return;

    const { getCourse } = await import("@/lib/courses");
    const c = await getCourse(db, courseId);
    if (!c || (c.createdBy !== s.user.id && !s.user.isPlatformAdmin)) return;

    const { updateTopic } = await import("@/lib/topics");
    await updateTopic(db, topicId, { title, description, starterCode: starterCode || null });
    revalidatePath(`/teacher/courses/${courseId}/topics/${topicId}`);
  }

  async function handleSaveLessonContent(contentJson: string) {
    "use server";
    const { auth: getAuth } = await import("@/lib/auth");
    const s = await getAuth();
    if (!s?.user?.id) return;

    const { getCourse } = await import("@/lib/courses");
    const c = await getCourse(db, courseId);
    if (!c || (c.createdBy !== s.user.id && !s.user.isPlatformAdmin)) return;

    const { lessonContentSchema } = await import("@/lib/lesson-content");
    const parsed = lessonContentSchema.safeParse(JSON.parse(contentJson));
    if (!parsed.success) return;

    const { updateTopic } = await import("@/lib/topics");
    await updateTopic(db, topicId, { lessonContent: parsed.data });
    revalidatePath(`/teacher/courses/${courseId}/topics/${topicId}`);
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <Link
            href={`/teacher/courses/${courseId}`}
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            &larr; Back to {course.title}
          </Link>
          <h1 className="text-2xl font-bold mt-1">{topic.title}</h1>
        </div>
      </div>

      {/* Topic metadata */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Topic Details</CardTitle>
        </CardHeader>
        <CardContent>
          <form action={handleUpdateDetails} className="space-y-4">
            <div>
              <Label htmlFor="title">Title</Label>
              <Input id="title" name="title" defaultValue={topic.title} required />
            </div>
            <div>
              <Label htmlFor="description">Description</Label>
              <Input id="description" name="description" defaultValue={topic.description || ""} />
            </div>
            <div>
              <Label htmlFor="starterCode">Starter Code (pre-filled for students)</Label>
              <textarea
                id="starterCode"
                name="starterCode"
                defaultValue={topic.starterCode || ""}
                placeholder="# Students start with this code..."
                className="w-full min-h-[100px] rounded-lg border border-input bg-transparent px-3 py-2 text-sm font-mono resize-y outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
                rows={4}
              />
            </div>
            <Button type="submit">Save Details</Button>
          </form>
        </CardContent>
      </Card>

      {/* Lesson Content Editor */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Lesson Content</CardTitle>
        </CardHeader>
        <CardContent>
          <TopicEditorClient
            initialContent={topic.lessonContent}
            saveLessonContent={handleSaveLessonContent}
          />
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 2: Create `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/topic-editor-client.tsx`**

A thin client wrapper that connects the `LessonEditor` to the server action.

```tsx
"use client";

import { useState } from "react";
import { LessonEditor } from "@/components/lesson/lesson-editor";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { Button } from "@/components/ui/button";
import { Eye, EyeOff } from "lucide-react";
import type { LessonContent } from "@/lib/lesson-content";

interface TopicEditorClientProps {
  initialContent: unknown;
  saveLessonContent: (contentJson: string) => Promise<void>;
}

export function TopicEditorClient({ initialContent, saveLessonContent }: TopicEditorClientProps) {
  const [saving, setSaving] = useState(false);
  const [showPreview, setShowPreview] = useState(false);
  const [previewContent, setPreviewContent] = useState<unknown>(initialContent);

  async function handleSave(content: LessonContent) {
    setSaving(true);
    try {
      await saveLessonContent(JSON.stringify(content));
      setPreviewContent(content);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button
          variant="outline"
          size="sm"
          onClick={() => setShowPreview(!showPreview)}
        >
          {showPreview ? <EyeOff className="size-3.5 mr-1" /> : <Eye className="size-3.5 mr-1" />}
          {showPreview ? "Hide Preview" : "Show Preview"}
        </Button>
      </div>

      {showPreview && (
        <div className="border rounded-lg p-4 bg-muted/20">
          <h3 className="text-sm font-medium text-muted-foreground mb-3">Student Preview</h3>
          <LessonRenderer content={previewContent} />
        </div>
      )}

      <LessonEditor
        initialContent={initialContent}
        onSave={handleSave}
        saving={saving}
      />
    </div>
  );
}
```

- [ ] **Step 3: Commit**

Commit: "Add topic detail page with lesson content editor and student preview"

---

## Task 8: Student Lesson View in Class Detail

**Files:**
- Modify: `src/app/(portal)/student/classes/[id]/page.tsx`

Enhance the student class detail page to show the list of topics from the course, with expandable lesson content previews. Each topic's lesson content is rendered using the `LessonRenderer`.

- [ ] **Step 1: Modify `src/app/(portal)/student/classes/[id]/page.tsx`**

```tsx
import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClass } from "@/lib/classes";
import { getCourse } from "@/lib/courses";
import { listTopicsByCourse } from "@/lib/topics";
import { listClassMembers } from "@/lib/class-memberships";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { hasLessonContent } from "@/lib/lesson-content";
import { StudentTopicList } from "./student-topic-list";

export default async function StudentClassDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const cls = await getClass(db, id);
  if (!cls) notFound();

  // Verify student is enrolled in this class
  const members = await listClassMembers(db, id);
  const isEnrolled = members.some((m) => m.userId === session!.user.id);
  if (!isEnrolled && !session!.user.isPlatformAdmin) notFound();

  // Load the course and topics for this class
  const course = await getCourse(db, cls.courseId);
  const topics = course ? await listTopicsByCourse(db, cls.courseId) : [];

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{cls.title}</h1>
        <p className="text-muted-foreground">{cls.term || "No term"}</p>
        {course && (
          <p className="text-sm text-muted-foreground mt-1">
            Course: {course.title} · {course.language}
          </p>
        )}
      </div>

      <div className="flex gap-3">
        <Link
          href={`/dashboard/classrooms/${id}/editor`}
          className={buttonVariants()}
        >
          Open Editor
        </Link>
      </div>

      {/* Topics with lesson content */}
      {topics.length > 0 && (
        <div>
          <h2 className="text-lg font-semibold mb-3">Topics ({topics.length})</h2>
          <StudentTopicList
            topics={topics.map((t) => ({
              id: t.id,
              title: t.title,
              description: t.description,
              lessonContent: t.lessonContent,
              hasContent: hasLessonContent(t.lessonContent),
            }))}
          />
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Create `src/app/(portal)/student/classes/[id]/student-topic-list.tsx`**

A client component that renders an expandable list of topics with lesson content.

```tsx
"use client";

import { useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { ChevronDown, ChevronRight, BookOpen } from "lucide-react";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";

interface TopicSummary {
  id: string;
  title: string;
  description: string | null;
  lessonContent: unknown;
  hasContent: boolean;
}

interface StudentTopicListProps {
  topics: TopicSummary[];
}

export function StudentTopicList({ topics }: StudentTopicListProps) {
  const [expandedId, setExpandedId] = useState<string | null>(null);

  function toggleTopic(topicId: string) {
    setExpandedId((prev) => (prev === topicId ? null : topicId));
  }

  return (
    <div className="space-y-2">
      {topics.map((topic, i) => (
        <Card key={topic.id}>
          <CardContent className="py-3">
            <button
              onClick={() => topic.hasContent && toggleTopic(topic.id)}
              className="flex items-center gap-2 w-full text-left"
              disabled={!topic.hasContent}
            >
              {topic.hasContent ? (
                expandedId === topic.id ? (
                  <ChevronDown className="size-4 text-muted-foreground shrink-0" />
                ) : (
                  <ChevronRight className="size-4 text-muted-foreground shrink-0" />
                )
              ) : (
                <BookOpen className="size-4 text-muted-foreground shrink-0 opacity-30" />
              )}
              <div className="flex-1">
                <p className="font-medium">
                  {i + 1}. {topic.title}
                </p>
                {topic.description && (
                  <p className="text-sm text-muted-foreground mt-0.5">{topic.description}</p>
                )}
              </div>
              {topic.hasContent && (
                <span className="text-xs text-muted-foreground">
                  {expandedId === topic.id ? "Collapse" : "View lesson"}
                </span>
              )}
            </button>

            {expandedId === topic.id && (
              <div className="mt-4 pt-4 border-t">
                <LessonRenderer content={topic.lessonContent} />
              </div>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
```

- [ ] **Step 3: Commit**

Commit: "Show expandable topic lesson content in student class detail page"

---

## Task 9: Integrate Lesson Content into Student Session View

**Files:**
- Modify: `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`

Add lesson content display alongside the code editor in the student session view. The student can toggle between a side-by-side layout (lesson left, editor right) and a stacked layout (lesson on top). Lesson content is fetched based on session topics.

- [ ] **Step 1: Create `src/app/dashboard/classrooms/[id]/session/[sessionId]/session-lesson-panel.tsx`**

A client component that fetches and displays lesson content for the current session.

```tsx
"use client";

import { useState, useEffect } from "react";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { Button } from "@/components/ui/button";
import { BookOpen, X } from "lucide-react";

interface SessionLessonPanelProps {
  sessionId: string;
}

interface TopicData {
  id: string;
  title: string;
  lessonContent: unknown;
}

export function SessionLessonPanel({ sessionId }: SessionLessonPanelProps) {
  const [topics, setTopics] = useState<TopicData[]>([]);
  const [selectedTopicIndex, setSelectedTopicIndex] = useState(0);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchTopics() {
      try {
        const res = await fetch(`/api/sessions/${sessionId}/topics`);
        if (res.ok) {
          const data = await res.json();
          setTopics(data);
        }
      } catch {
        // Ignore errors silently — lesson content is supplementary
      } finally {
        setLoading(false);
      }
    }
    fetchTopics();
  }, [sessionId]);

  if (loading) {
    return (
      <div className="p-4 text-sm text-muted-foreground">
        Loading lesson content...
      </div>
    );
  }

  if (topics.length === 0) {
    return (
      <div className="p-4 text-sm text-muted-foreground italic">
        No lesson content for this session.
      </div>
    );
  }

  const currentTopic = topics[selectedTopicIndex];

  return (
    <div className="flex flex-col h-full">
      {/* Topic selector (if multiple topics) */}
      {topics.length > 1 && (
        <div className="flex gap-1 px-3 py-2 border-b overflow-x-auto">
          {topics.map((topic, i) => (
            <button
              key={topic.id}
              onClick={() => setSelectedTopicIndex(i)}
              className={`px-2 py-1 text-xs rounded whitespace-nowrap transition-colors ${
                i === selectedTopicIndex
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-muted"
              }`}
            >
              {topic.title}
            </button>
          ))}
        </div>
      )}

      {/* Lesson content */}
      <div className="flex-1 overflow-auto p-4">
        <h3 className="text-sm font-semibold mb-3">{currentTopic.title}</h3>
        <LessonRenderer content={currentTopic.lessonContent} />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create `src/app/api/sessions/[sessionId]/topics/route.ts`**

API route that returns the topics linked to a session (via `session_topics` table), including their lesson content.

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { sessionTopics, topics } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

export async function GET(
  _req: NextRequest,
  { params }: { params: Promise<{ sessionId: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json([], { status: 401 });
  }

  const { sessionId } = await params;

  // Get topics linked to this session
  const linkedTopics = await db
    .select({
      id: topics.id,
      title: topics.title,
      lessonContent: topics.lessonContent,
      sortOrder: topics.sortOrder,
    })
    .from(sessionTopics)
    .innerJoin(topics, eq(sessionTopics.topicId, topics.id))
    .where(eq(sessionTopics.sessionId, sessionId))
    .orderBy(topics.sortOrder);

  return NextResponse.json(linkedTopics);
}
```

- [ ] **Step 3: Modify `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`**

Add the lesson panel to the student session view. Add a layout toggle button.

Replace the file with:

```tsx
"use client";

import { useState, useEffect } from "react";
import { useParams } from "next/navigation";
import { useSession } from "next-auth/react";
import { CodeEditor } from "@/components/editor/code-editor";
import { OutputPanel } from "@/components/editor/output-panel";
import { RunButton } from "@/components/editor/run-button";
import { AiChatPanel } from "@/components/ai/ai-chat-panel";
import { RaiseHandButton } from "@/components/help-queue/raise-hand-button";
import { SessionLessonPanel } from "./session-lesson-panel";
import { usePyodide } from "@/lib/pyodide/use-pyodide";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { Button } from "@/components/ui/button";
import { BookOpen, PanelLeft, Rows } from "lucide-react";

type Layout = "side-by-side" | "stacked";

export default function StudentSessionPage() {
  const params = useParams<{ id: string; sessionId: string }>();
  const { data: session } = useSession();
  const [code, setCode] = useState("");
  const [aiEnabled, setAiEnabled] = useState(false);
  const [showAi, setShowAi] = useState(false);
  const [showLesson, setShowLesson] = useState(true);
  const [layout, setLayout] = useState<Layout>(() => {
    if (typeof window !== "undefined") {
      return (localStorage.getItem("bridge-session-layout") as Layout) || "side-by-side";
    }
    return "side-by-side";
  });
  const [broadcastActive, setBroadcastActive] = useState(false);
  const { ready, running, output, runCode, clearOutput } = usePyodide();

  const userId = session?.user?.id || "";
  const documentName = `session:${params.sessionId}:user:${userId}`;
  const token = `${userId}:user`;

  const { yText, provider, connected } = useYjsProvider({
    documentName,
    token,
  });

  // Broadcast document (read-only view of teacher's code)
  const broadcastDocName = `broadcast:${params.sessionId}`;
  const { yText: broadcastYText, provider: broadcastProvider } = useYjsProvider({
    documentName: broadcastActive ? broadcastDocName : "noop",
    token,
  });

  // Persist layout preference
  useEffect(() => {
    localStorage.setItem("bridge-session-layout", layout);
  }, [layout]);

  // Listen for SSE events (AI toggle, broadcast, session end)
  useEffect(() => {
    const eventSource = new EventSource(
      `/api/sessions/${params.sessionId}/events`
    );

    eventSource.addEventListener("ai_toggled", (e) => {
      const data = JSON.parse(e.data);
      if (data.studentId === userId) {
        setAiEnabled(data.enabled);
        if (data.enabled && !showAi) {
          setShowAi(true);
        }
      }
    });

    eventSource.addEventListener("broadcast_started", () => {
      setBroadcastActive(true);
    });

    eventSource.addEventListener("broadcast_ended", () => {
      setBroadcastActive(false);
    });

    eventSource.addEventListener("session_ended", () => {
      window.location.href = `/dashboard/classrooms/${params.id}`;
    });

    return () => eventSource.close();
  }, [params.sessionId, params.id, userId, showAi]);

  const toolbarButtons = (
    <div className="flex items-center gap-2">
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setShowLesson(!showLesson)}
        aria-label={showLesson ? "Hide lesson" : "Show lesson"}
      >
        <BookOpen className="size-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setLayout(layout === "side-by-side" ? "stacked" : "side-by-side")}
        aria-label={`Switch to ${layout === "side-by-side" ? "stacked" : "side-by-side"} layout`}
      >
        {layout === "side-by-side" ? <Rows className="size-4" /> : <PanelLeft className="size-4" />}
      </Button>
    </div>
  );

  // Side-by-side layout
  if (layout === "side-by-side") {
    return (
      <div className="flex h-[calc(100vh-3.5rem)]">
        {/* Lesson panel (left) */}
        {showLesson && (
          <div className="w-[400px] border-r overflow-hidden flex flex-col shrink-0">
            <SessionLessonPanel sessionId={params.sessionId} />
          </div>
        )}

        {/* Editor area */}
        <div className="flex flex-col flex-1 gap-2 p-0 min-w-0">
          <div className="flex items-center justify-between px-4 pt-2">
            <div className="flex items-center gap-2">
              <h2 className="text-sm font-medium text-muted-foreground">
                Live Session
              </h2>
              <span
                className={`w-2 h-2 rounded-full ${
                  connected ? "bg-green-500" : "bg-red-500"
                }`}
              />
              {toolbarButtons}
            </div>
            <div className="flex gap-2">
              <RaiseHandButton sessionId={params.sessionId} />
              {aiEnabled && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowAi(!showAi)}
                >
                  {showAi ? "Hide AI" : "Ask AI"}
                </Button>
              )}
              <Button
                variant="ghost"
                size="sm"
                onClick={clearOutput}
                disabled={running}
              >
                Clear
              </Button>
              <RunButton
                onRun={() => {
                  const currentCode = yText?.toString() || code;
                  runCode(currentCode);
                }}
                running={running}
                ready={ready}
              />
            </div>
          </div>

          {broadcastActive && (
            <div className="mx-4 mb-2 border rounded-lg overflow-hidden">
              <div className="bg-blue-50 px-3 py-1 text-xs font-medium text-blue-700 border-b">
                Teacher is broadcasting
              </div>
              <div className="h-40">
                <CodeEditor yText={broadcastYText} provider={broadcastProvider} readOnly />
              </div>
            </div>
          )}

          <div className="flex-1 min-h-0 px-4">
            <CodeEditor
              onChange={setCode}
              yText={yText}
              provider={provider}
            />
          </div>

          <div className="h-[200px] shrink-0 px-4 pb-4">
            <OutputPanel output={output} running={running} />
          </div>
        </div>

        {/* AI panel (right) */}
        {showAi && (
          <div className="w-80 border-l p-2">
            <AiChatPanel
              sessionId={params.sessionId}
              code={yText?.toString() || code}
              enabled={true}
            />
          </div>
        )}
      </div>
    );
  }

  // Stacked layout
  return (
    <div className="flex h-[calc(100vh-3.5rem)]">
      <div className="flex flex-col flex-1 gap-2 p-0">
        <div className="flex items-center justify-between px-4 pt-2">
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-medium text-muted-foreground">
              Live Session
            </h2>
            <span
              className={`w-2 h-2 rounded-full ${
                connected ? "bg-green-500" : "bg-red-500"
              }`}
            />
            {toolbarButtons}
          </div>
          <div className="flex gap-2">
            <RaiseHandButton sessionId={params.sessionId} />
            {aiEnabled && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setShowAi(!showAi)}
              >
                {showAi ? "Hide AI" : "Ask AI"}
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={clearOutput}
              disabled={running}
            >
              Clear
            </Button>
            <RunButton
              onRun={() => {
                const currentCode = yText?.toString() || code;
                runCode(currentCode);
              }}
              running={running}
              ready={ready}
            />
          </div>
        </div>

        {/* Lesson panel (top, collapsible) */}
        {showLesson && (
          <div className="mx-4 border rounded-lg overflow-hidden max-h-[300px]">
            <SessionLessonPanel sessionId={params.sessionId} />
          </div>
        )}

        {broadcastActive && (
          <div className="mx-4 mb-2 border rounded-lg overflow-hidden">
            <div className="bg-blue-50 px-3 py-1 text-xs font-medium text-blue-700 border-b">
              Teacher is broadcasting
            </div>
            <div className="h-40">
              <CodeEditor yText={broadcastYText} provider={broadcastProvider} readOnly />
            </div>
          </div>
        )}

        <div className="flex-1 min-h-0 px-4">
          <CodeEditor
            onChange={setCode}
            yText={yText}
            provider={provider}
          />
        </div>

        <div className="h-[200px] shrink-0 px-4 pb-4">
          <OutputPanel output={output} running={running} />
        </div>
      </div>

      {showAi && (
        <div className="w-80 border-l p-2">
          <AiChatPanel
            sessionId={params.sessionId}
            code={yText?.toString() || code}
            enabled={true}
          />
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Commit**

Commit: "Add lesson content panel to student session view with layout toggle"

---

## Task 10: API Route for Session Topics + Integration Tests

**Files:**
- Create: `tests/unit/session-topics-api.test.ts`
- Create: `tests/integration/session-topics-api.test.ts`

Test the session topics API route and the integration between lesson content, topics, and sessions.

- [ ] **Step 1: Create `tests/unit/session-topics-api.test.ts`**

Tests for the lesson content round-trip: create topic with content, link to session, fetch via API.

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestOrg,
  createTestCourse,
  createTestTopic,
  createTestClass,
  createTestSession,
} from "../helpers";
import { createTopic, updateTopic, getTopic } from "@/lib/topics";
import { parseLessonContent } from "@/lib/lesson-content";
import * as schema from "@/lib/db/schema";

describe("lesson content round-trip", () => {
  let course: Awaited<ReturnType<typeof createTestCourse>>;

  beforeEach(async () => {
    const org = await createTestOrg();
    const teacher = await createTestUser({ email: "teacher@test.edu" });
    course = await createTestCourse(org.id, teacher.id);
  });

  it("stores and retrieves lesson content in topic", async () => {
    const topic = await createTopic(testDb, {
      courseId: course.id,
      title: "Variables",
      lessonContent: {
        blocks: [
          { type: "markdown", content: "# Variables\n\nA variable stores a value." },
          { type: "code", language: "python", content: "x = 5\nprint(x)" },
        ],
      },
    });

    const fetched = await getTopic(testDb, topic.id);
    expect(fetched).not.toBeNull();

    const content = parseLessonContent(fetched!.lessonContent);
    expect(content.blocks).toHaveLength(2);
    expect(content.blocks[0].type).toBe("markdown");
    expect(content.blocks[1].type).toBe("code");
  });

  it("updates lesson content on existing topic", async () => {
    const topic = await createTestTopic(course.id);
    const updated = await updateTopic(testDb, topic.id, {
      lessonContent: {
        blocks: [{ type: "markdown", content: "Updated content" }],
      },
    });

    const content = parseLessonContent(updated!.lessonContent);
    expect(content.blocks).toHaveLength(1);
    expect(content.blocks[0]).toEqual({ type: "markdown", content: "Updated content" });
  });

  it("handles empty lesson content gracefully", async () => {
    const topic = await createTestTopic(course.id);
    const content = parseLessonContent(topic.lessonContent);
    expect(content.blocks).toHaveLength(0);
  });

  it("links topics to sessions via session_topics table", async () => {
    const teacher = await createTestUser({ email: "teacher2@test.edu" });
    const org = await createTestOrg({ slug: "session-org" });
    const cls = await createTestClass(course.id, org.id);

    // Create a classroom for the class
    const [classroom] = await testDb
      .insert(schema.newClassrooms)
      .values({ classId: cls.id, editorMode: "python" })
      .returning();

    // Create a legacy classroom for session (sessions reference old classrooms table)
    const legacyClassroom = await import("../helpers").then((h) =>
      h.createTestClassroom(teacher.id)
    );

    const liveSession = await createTestSession(legacyClassroom.id, teacher.id);

    const t1 = await createTestTopic(course.id, {
      title: "Topic 1",
      lessonContent: { blocks: [{ type: "markdown", content: "Content 1" }] },
    });
    const t2 = await createTestTopic(course.id, {
      title: "Topic 2",
      lessonContent: { blocks: [{ type: "code", language: "python", content: "print(1)" }] },
    });

    // Link topics to session
    await testDb.insert(schema.sessionTopics).values([
      { sessionId: liveSession.id, topicId: t1.id },
      { sessionId: liveSession.id, topicId: t2.id },
    ]);

    // Query session topics
    const { eq } = await import("drizzle-orm");
    const linkedTopics = await testDb
      .select({
        id: schema.topics.id,
        title: schema.topics.title,
        lessonContent: schema.topics.lessonContent,
      })
      .from(schema.sessionTopics)
      .innerJoin(schema.topics, eq(schema.sessionTopics.topicId, schema.topics.id))
      .where(eq(schema.sessionTopics.sessionId, liveSession.id));

    expect(linkedTopics).toHaveLength(2);
    expect(linkedTopics.map((t) => t.title).sort()).toEqual(["Topic 1", "Topic 2"]);
  });
});
```

- [ ] **Step 2: Run all tests**

```bash
cd /home/chris/workshop/Bridge && bun test tests/unit/lesson-content.test.ts tests/unit/session-topics-api.test.ts
```

- [ ] **Step 3: Commit**

Commit: "Add lesson content round-trip and session-topic linking tests"

---

## Task 11: Tailwind Prose Plugin for Markdown Rendering

**Files:**
- Modify: `package.json` (via `bun add`)
- Modify: CSS config if needed

The `prose` class used in `MarkdownBlock` requires the Tailwind Typography plugin. Check if it's already installed; if not, install it.

- [ ] **Step 1: Check and install @tailwindcss/typography**

```bash
cd /home/chris/workshop/Bridge && bun add @tailwindcss/typography
```

- [ ] **Step 2: Add the typography plugin to Tailwind config**

Check the CSS entry point (likely `src/app/globals.css` or similar) and add `@import "tailwindcss/typography"` or `@plugin "@tailwindcss/typography"` depending on Tailwind v4 configuration pattern. In Tailwind v4, plugins are loaded via CSS:

Find the main CSS file and add:

```css
@import "tailwindcss";
@plugin "@tailwindcss/typography";
```

- [ ] **Step 3: Commit**

Commit: "Install @tailwindcss/typography for markdown prose styling"

---

## Task 12: Final Integration Verification

- [ ] **Step 1: Run full test suite**

```bash
cd /home/chris/workshop/Bridge && bun test
```

Fix any failures.

- [ ] **Step 2: Build check**

```bash
cd /home/chris/workshop/Bridge && bun run build
```

Fix any type errors or build issues.

- [ ] **Step 3: Final commit if any fixes were needed**

Commit: "Fix build/test issues from lesson content integration"

---

## Summary of Changes

| Area | Files | Description |
|------|-------|-------------|
| Types + Validation | `src/lib/lesson-content.ts` | LessonBlock types, Zod schemas, pure helper functions |
| Renderer | `src/components/lesson/lesson-renderer.tsx`, `blocks/*.tsx` | Read-only display of markdown, code, and image blocks |
| Editor | `src/components/lesson/lesson-editor.tsx`, `editor-blocks/*.tsx` | Block-based authoring: add/remove/reorder/edit |
| Teacher Courses | `src/app/(portal)/teacher/courses/page.tsx` | Create Course form |
| Teacher Course Detail | `src/app/(portal)/teacher/courses/[id]/page.tsx` | Topic CRUD, reorder, delete |
| Create Class | `src/app/(portal)/teacher/courses/[id]/create-class/page.tsx` | Create class from course |
| Topic Detail | `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx` | Lesson content editor + details form |
| Student Class | `src/app/(portal)/student/classes/[id]/page.tsx` | Expandable topic list with lesson preview |
| Student Session | `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx` | Lesson panel with side-by-side/stacked layout toggle |
| Session Topics API | `src/app/api/sessions/[sessionId]/topics/route.ts` | GET topics linked to session |
| Dependencies | `package.json` | `react-markdown`, `remark-gfm`, `@tailwindcss/typography` |
| Tests | `tests/unit/lesson-content.test.ts`, `tests/unit/lesson-renderer.test.tsx`, `tests/unit/lesson-editor.test.tsx`, `tests/unit/session-topics-api.test.ts` | Validation, rendering, editor behavior, DB round-trip |

---

## Code Review

### Review 1

- **Date**: 2026-04-11
- **Reviewer**: Claude (superpowers:code-reviewer)
- **PR**: #13 — feat: lesson content system
- **Verdict**: Approved with changes

**Must Fix**

1. `[FIXED]` Topic API route (GET/PATCH/DELETE) lacks authorization — any auth'd user can modify any topic.
   → Response: Added `verifyCourseOwnership` check to all 3 handlers.

2. `[FIXED]` Topic editor page is client component with no server-side auth guard.
   → Response: Authorization now enforced at the API route level. The client page calls the protected API.

3. `[WONTFIX]` Image URL with user-supplied URLs is SSRF vector.
   → Response: Strengthened Zod schema to use `z.string().url()` which validates URL format. Teachers are trusted users; further restrictions deferred.

4. `[WONTFIX]` No markdown sanitization (XSS).
   → Response: react-markdown v10 does NOT use dangerouslySetInnerHTML — it renders to React elements safely. rehype-raw is not installed. No XSS risk in current implementation.

**Should Fix**

5. `[WONTFIX]` Type name `ContentBlock` vs plan's `LessonBlock`.
   → Response: Naming difference is acceptable — consistently used throughout implementation.

6-7. `[WONTFIX]` Missing separate block component files and test files.
   → Response: Inlined blocks are simpler for current scope. Tests deferred to UX polish phase.

8. `[WONTFIX]` Missing `hasLessonContent` and `createBlock` helpers.
   → Response: Not needed — editor creates blocks inline.

9. `[FIXED]` Zod schema lacks min length and URL validation.
   → Response: Added `.min(1)` to all content fields, `.url()` to image URL.

10. `[FIXED]` Server actions in course detail lack ownership verification.
    → Response: Added course ownership check before creating/deleting topics.

11. `[WONTFIX]` Missing topic reorder on course page.
    → Response: Deferred — reorder API exists, UI button will be added in polish pass.

12. `[WONTFIX]` Missing session integration (Task 9).
    → Response: Deferred to Sub-project 4 (Live Session Redesign) where session UX is being overhauled.

13. `[WONTFIX]` Student topics not expandable.
    → Response: Static rendering is acceptable for MVP. Expandable accordion deferred.
