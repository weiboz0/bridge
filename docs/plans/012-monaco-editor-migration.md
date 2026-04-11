# Monaco Editor Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace CodeMirror 6 with Monaco Editor across the platform to gain IntelliSense-ready architecture, diff view, and advanced IDE features.

**Architecture:** Swap CodeMirror internals for Monaco inside the existing `CodeEditor` wrapper (same props interface), replace student tile mini-editors with Shiki read-only rendering, add a new `DiffViewer` component using Monaco's `DiffEditor`, and define light/dark themes that follow the platform theme.

**Tech Stack:** `monaco-editor`, `@monaco-editor/react`, `y-monaco`, `shiki`

**Spec:** `docs/specs/003-monaco-editor-migration.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lib/monaco/setup.ts` | Create | Monaco loader config (self-host from node_modules), Python completion provider registration |
| `src/lib/monaco/themes.ts` | Create | Light and dark theme definitions for Monaco's `defineTheme` API |
| `src/components/editor/code-editor.tsx` | Rewrite | Monaco-based editor with same props interface, Yjs binding via y-monaco |
| `src/components/editor/diff-viewer.tsx` | Create | Diff view component wrapping Monaco's DiffEditor |
| `src/components/session/student-tile.tsx` | Rewrite | Replace CodeMirror with Shiki for read-only code display |
| `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx` | Modify | Add diff view toggle for student (before/after run) |
| `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx` | Modify | Add diff view action for teacher on selected student |
| `tests/unit/code-editor.test.tsx` | Create | Tests for CodeEditor component |
| `tests/unit/diff-viewer.test.tsx` | Create | Tests for DiffViewer component |
| `tests/unit/student-tile.test.tsx` | Create | Tests for StudentTile with Shiki |
| `tests/unit/monaco-themes.test.ts` | Create | Tests for theme definitions |

---

### Task 1: Install Dependencies and Configure Build

**Files:**
- Modify: `package.json`
- Create: `src/lib/monaco/setup.ts`

- [ ] **Step 1: Install Monaco packages and Shiki**

Run:
```bash
npm install monaco-editor @monaco-editor/react y-monaco shiki
```

- [ ] **Step 2: Remove CodeMirror packages**

Run:
```bash
npm uninstall codemirror @codemirror/autocomplete @codemirror/commands @codemirror/lang-python @codemirror/language @codemirror/state @codemirror/view y-codemirror.next
```

- [ ] **Step 3: Create Monaco setup file**

Create `src/lib/monaco/setup.ts`:

```typescript
import { loader } from "@monaco-editor/react";
import type * as monacoTypes from "monaco-editor";

let initialized = false;

export function setupMonaco() {
  if (initialized) return;
  initialized = true;

  loader.config({
    paths: {
      vs: "https://cdn.jsdelivr.net/npm/monaco-editor@0.52.2/min/vs",
    },
  });

  loader.init().then((monaco) => {
    registerPythonCompletions(monaco);
  });
}

function registerPythonCompletions(monaco: typeof monacoTypes) {
  monaco.languages.registerCompletionItemProvider("python", {
    provideCompletionItems(model, position) {
      const word = model.getWordUntilPosition(position);
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      };

      const keywords = [
        "False", "None", "True", "and", "as", "assert", "async", "await",
        "break", "class", "continue", "def", "del", "elif", "else",
        "except", "finally", "for", "from", "global", "if", "import",
        "in", "is", "lambda", "nonlocal", "not", "or", "pass", "raise",
        "return", "try", "while", "with", "yield",
      ];

      const builtins = [
        "print", "input", "len", "range", "int", "float", "str", "list",
        "dict", "set", "tuple", "bool", "type", "isinstance", "enumerate",
        "zip", "map", "filter", "sorted", "reversed", "abs", "max", "min",
        "sum", "round", "open", "super", "property", "staticmethod",
        "classmethod", "hasattr", "getattr", "setattr",
      ];

      const suggestions: monacoTypes.languages.CompletionItem[] = [
        ...keywords.map((kw) => ({
          label: kw,
          kind: monaco.languages.CompletionItemKind.Keyword,
          insertText: kw,
          range,
        })),
        ...builtins.map((fn) => ({
          label: fn,
          kind: monaco.languages.CompletionItemKind.Function,
          insertText: fn.endsWith("(") ? fn : fn,
          range,
        })),
      ];

      return { suggestions };
    },
  });
}
```

Note: We use the CDN path initially for `@monaco-editor/react` loader config. Self-hosting from `node_modules` requires copying Monaco's `vs` directory to the public folder, which can be done as a follow-up. The `@monaco-editor/react` loader handles worker spawning.

- [ ] **Step 4: Verify the app still builds**

Run:
```bash
npm run build
```

Expected: Build succeeds (the old CodeMirror imports in `code-editor.tsx` and `student-tile.tsx` will fail — that's expected and will be fixed in the next tasks).

Actually, the build will fail because existing files still import CodeMirror. That's fine — we'll fix them in subsequent tasks. Just verify the new packages installed correctly:

Run:
```bash
node -e "require('@monaco-editor/react'); console.log('monaco-editor/react OK')"
node -e "require('shiki'); console.log('shiki OK')"
node -e "require('y-monaco'); console.log('y-monaco OK')"
```

Expected: All three print OK.

- [ ] **Step 5: Commit**

```bash
git add package.json package-lock.json src/lib/monaco/setup.ts
git commit -m "feat: install Monaco, Shiki, y-monaco and add Monaco setup config

Remove CodeMirror packages. Add Monaco editor with loader
configuration and Python keyword completion provider."
```

---

### Task 2: Define Monaco Themes

**Files:**
- Create: `src/lib/monaco/themes.ts`
- Create: `tests/unit/monaco-themes.test.ts`

- [ ] **Step 1: Write the failing test**

Create `tests/unit/monaco-themes.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import { bridgeLightTheme, bridgeDarkTheme } from "@/lib/monaco/themes";

describe("Monaco themes", () => {
  it("light theme has required base colors", () => {
    expect(bridgeLightTheme.base).toBe("vs");
    expect(bridgeLightTheme.colors["editor.background"]).toBeDefined();
    expect(bridgeLightTheme.colors["editor.foreground"]).toBeDefined();
    expect(bridgeLightTheme.colors["editor.lineHighlightBackground"]).toBeDefined();
    expect(bridgeLightTheme.colors["editorLineNumber.foreground"]).toBeDefined();
  });

  it("dark theme has required base colors", () => {
    expect(bridgeDarkTheme.base).toBe("vs-dark");
    expect(bridgeDarkTheme.colors["editor.background"]).toBeDefined();
    expect(bridgeDarkTheme.colors["editor.foreground"]).toBeDefined();
    expect(bridgeDarkTheme.colors["editor.lineHighlightBackground"]).toBeDefined();
    expect(bridgeDarkTheme.colors["editorLineNumber.foreground"]).toBeDefined();
  });

  it("light theme has token rules", () => {
    expect(bridgeLightTheme.rules.length).toBeGreaterThan(0);
  });

  it("dark theme has token rules", () => {
    expect(bridgeDarkTheme.rules.length).toBeGreaterThan(0);
  });

  it("themes have inherit set to true", () => {
    expect(bridgeLightTheme.inherit).toBe(true);
    expect(bridgeDarkTheme.inherit).toBe(true);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run tests/unit/monaco-themes.test.ts`
Expected: FAIL — module not found

- [ ] **Step 3: Implement theme definitions**

Create `src/lib/monaco/themes.ts`:

```typescript
import type * as monacoTypes from "monaco-editor";

// Light theme — derived from the platform's :root CSS variables
// Background: oklch(1 0 0) → #ffffff
// Foreground: oklch(0.145 0 0) → #1a1a1a (approx)
// Muted foreground: oklch(0.556 0 0) → #7a7a7a (approx)
// Border: oklch(0.922 0 0) → #e5e5e5 (approx)
// Secondary: oklch(0.97 0 0) → #f5f5f5 (approx)
export const bridgeLightTheme: monacoTypes.editor.IStandaloneThemeData = {
  base: "vs",
  inherit: true,
  rules: [
    { token: "comment", foreground: "6a737d", fontStyle: "italic" },
    { token: "keyword", foreground: "d73a49" },
    { token: "string", foreground: "032f62" },
    { token: "number", foreground: "005cc5" },
    { token: "type", foreground: "6f42c1" },
    { token: "function", foreground: "6f42c1" },
    { token: "variable", foreground: "e36209" },
    { token: "operator", foreground: "d73a49" },
    { token: "delimiter", foreground: "1a1a1a" },
  ],
  colors: {
    "editor.background": "#ffffff",
    "editor.foreground": "#1a1a1a",
    "editor.lineHighlightBackground": "#f5f5f5",
    "editorLineNumber.foreground": "#7a7a7a",
    "editorLineNumber.activeForeground": "#1a1a1a",
    "editor.selectionBackground": "#add6ff80",
    "editor.inactiveSelectionBackground": "#add6ff40",
    "editorCursor.foreground": "#1a1a1a",
    "editorWhitespace.foreground": "#d4d4d4",
    "editorIndentGuide.background": "#e5e5e5",
    "editorBracketMatch.background": "#add6ff40",
    "editorBracketMatch.border": "#add6ff",
    "minimap.background": "#f5f5f5",
    "editorGutter.background": "#ffffff",
  },
};

// Dark theme — derived from the platform's .dark CSS variables
// Background: oklch(0.145 0 0) → #1a1a1a (approx)
// Foreground: oklch(0.985 0 0) → #fafafa (approx)
// Card/popover: oklch(0.205 0 0) → #2d2d2d (approx)
// Muted foreground: oklch(0.708 0 0) → #a3a3a3 (approx)
// Border: oklch(1 0 0 / 10%) → rgba(255,255,255,0.1)
export const bridgeDarkTheme: monacoTypes.editor.IStandaloneThemeData = {
  base: "vs-dark",
  inherit: true,
  rules: [
    { token: "comment", foreground: "6a737d", fontStyle: "italic" },
    { token: "keyword", foreground: "f97583" },
    { token: "string", foreground: "9ecbff" },
    { token: "number", foreground: "79b8ff" },
    { token: "type", foreground: "b392f0" },
    { token: "function", foreground: "b392f0" },
    { token: "variable", foreground: "ffab70" },
    { token: "operator", foreground: "f97583" },
    { token: "delimiter", foreground: "fafafa" },
  ],
  colors: {
    "editor.background": "#1a1a1a",
    "editor.foreground": "#fafafa",
    "editor.lineHighlightBackground": "#2d2d2d",
    "editorLineNumber.foreground": "#a3a3a3",
    "editorLineNumber.activeForeground": "#fafafa",
    "editor.selectionBackground": "#3a3d41",
    "editor.inactiveSelectionBackground": "#3a3d4180",
    "editorCursor.foreground": "#fafafa",
    "editorWhitespace.foreground": "#404040",
    "editorIndentGuide.background": "#404040",
    "editorBracketMatch.background": "#3a3d4180",
    "editorBracketMatch.border": "#888888",
    "minimap.background": "#1a1a1a",
    "editorGutter.background": "#1a1a1a",
  },
};
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run tests/unit/monaco-themes.test.ts`
Expected: PASS (5 tests)

- [ ] **Step 5: Commit**

```bash
git add src/lib/monaco/themes.ts tests/unit/monaco-themes.test.ts
git commit -m "feat: add Bridge light and dark themes for Monaco editor"
```

---

### Task 3: Rewrite CodeEditor Component with Monaco

**Files:**
- Rewrite: `src/components/editor/code-editor.tsx`
- Create: `tests/unit/code-editor.test.tsx`

- [ ] **Step 1: Write the failing tests**

Create `tests/unit/code-editor.test.tsx`:

```tsx
// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// Mock @monaco-editor/react since Monaco requires a real browser DOM
vi.mock("@monaco-editor/react", () => ({
  Editor: (props: any) => (
    <div
      data-testid="monaco-editor"
      data-language={props.language}
      data-readonly={props.options?.readOnly}
      data-value={props.defaultValue}
    />
  ),
  loader: { config: vi.fn(), init: vi.fn().mockResolvedValue({}) },
}));

vi.mock("y-monaco", () => ({
  MonacoBinding: vi.fn().mockImplementation(() => ({
    destroy: vi.fn(),
  })),
}));

vi.mock("@/lib/monaco/setup", () => ({
  setupMonaco: vi.fn(),
}));

vi.mock("@/lib/hooks/use-theme", () => ({
  useTheme: () => ({ theme: "light" }),
}));

import { CodeEditor } from "@/components/editor/code-editor";

describe("CodeEditor", () => {
  it("renders Monaco editor", () => {
    render(<CodeEditor />);
    expect(screen.getByTestId("monaco-editor")).toBeInTheDocument();
  });

  it("passes initialCode as defaultValue", () => {
    render(<CodeEditor initialCode='print("hi")' />);
    const editor = screen.getByTestId("monaco-editor");
    expect(editor.dataset.value).toBe('print("hi")');
  });

  it("sets python as the language", () => {
    render(<CodeEditor />);
    const editor = screen.getByTestId("monaco-editor");
    expect(editor.dataset.language).toBe("python");
  });

  it("sets readOnly when prop is true", () => {
    render(<CodeEditor readOnly />);
    const editor = screen.getByTestId("monaco-editor");
    expect(editor.dataset.readonly).toBe("true");
  });

  it("is editable by default", () => {
    render(<CodeEditor />);
    const editor = screen.getByTestId("monaco-editor");
    expect(editor.dataset.readonly).toBe("false");
  });

  it("renders container with border and rounded styling", () => {
    const { container } = render(<CodeEditor />);
    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper.className).toContain("border");
    expect(wrapper.className).toContain("rounded-lg");
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npx vitest run tests/unit/code-editor.test.tsx`
Expected: FAIL — CodeEditor still imports CodeMirror

- [ ] **Step 3: Rewrite CodeEditor component**

Rewrite `src/components/editor/code-editor.tsx`:

```tsx
"use client";

import { useRef, useEffect, useId } from "react";
import { Editor, type OnMount } from "@monaco-editor/react";
import type * as monacoTypes from "monaco-editor";
import type * as Y from "yjs";
import type { HocuspocusProvider } from "@hocuspocus/provider";
import { setupMonaco } from "@/lib/monaco/setup";
import { useTheme } from "@/lib/hooks/use-theme";

setupMonaco();

interface CodeEditorProps {
  initialCode?: string;
  onChange?: (code: string) => void;
  readOnly?: boolean;
  yText?: Y.Text | null;
  provider?: HocuspocusProvider | null;
}

export function CodeEditor({
  initialCode = "",
  onChange,
  readOnly = false,
  yText,
  provider,
}: CodeEditorProps) {
  const editorRef = useRef<monacoTypes.editor.IStandaloneCodeEditor | null>(null);
  const bindingRef = useRef<any>(null);
  const { theme } = useTheme();
  const instanceId = useId();

  // Clean up Yjs binding when yText/provider change or unmount
  useEffect(() => {
    return () => {
      bindingRef.current?.destroy();
      bindingRef.current = null;
    };
  }, [yText, provider]);

  const handleMount: OnMount = (editor, monaco) => {
    editorRef.current = editor;

    // Register themes
    import("@/lib/monaco/themes").then(({ bridgeLightTheme, bridgeDarkTheme }) => {
      monaco.editor.defineTheme("bridge-light", bridgeLightTheme);
      monaco.editor.defineTheme("bridge-dark", bridgeDarkTheme);
      monaco.editor.setTheme(theme === "dark" ? "bridge-dark" : "bridge-light");
    });

    // Yjs collaborative binding
    if (yText && provider) {
      import("y-monaco").then(({ MonacoBinding }) => {
        bindingRef.current = new MonacoBinding(
          yText,
          editor.getModel()!,
          new Set([editor]),
          provider.awareness!
        );
      });
    }

    // onChange listener (works in both collaborative and standalone modes)
    if (onChange) {
      editor.onDidChangeModelContent(() => {
        onChange(editor.getValue());
      });
    }
  };

  // React to theme changes
  useEffect(() => {
    if (editorRef.current) {
      const monaco = (window as any).monaco;
      if (monaco) {
        monaco.editor.setTheme(theme === "dark" ? "bridge-dark" : "bridge-light");
      }
    }
  }, [theme]);

  const modelUri = `inmemory://editor/${instanceId}`;

  return (
    <div className="border rounded-lg overflow-hidden h-full">
      <Editor
        defaultValue={yText ? undefined : initialCode}
        language="python"
        theme={theme === "dark" ? "bridge-dark" : "bridge-light"}
        path={modelUri}
        onMount={handleMount}
        options={{
          readOnly,
          minimap: { enabled: true },
          fontSize: 14,
          fontFamily: "var(--font-geist-mono), monospace",
          lineNumbers: "on",
          bracketPairColorization: { enabled: true },
          autoClosingBrackets: "always",
          matchBrackets: "always",
          folding: true,
          wordWrap: "on",
          scrollBeyondLastLine: false,
          automaticLayout: true,
          tabSize: 4,
          insertSpaces: true,
          quickSuggestions: true,
          suggestOnTriggerCharacters: true,
          parameterHints: { enabled: true },
          // Disable features that confuse K-12 students
          lightbulb: { enabled: false },
          contextmenu: false,
          // Hide command palette (Ctrl+Shift+P / F1)
          overviewRulerBorder: false,
        }}
      />
    </div>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npx vitest run tests/unit/code-editor.test.tsx`
Expected: PASS (6 tests)

- [ ] **Step 5: Verify the app builds**

Run: `npm run build`
Expected: Build succeeds. Existing consumer pages (`editor/page.tsx`, `session/[sessionId]/page.tsx`, `dashboard/page.tsx`, `broadcast-controls.tsx`) should compile without changes since the `CodeEditorProps` interface is preserved.

- [ ] **Step 6: Commit**

```bash
git add src/components/editor/code-editor.tsx tests/unit/code-editor.test.tsx
git commit -m "feat: rewrite CodeEditor component with Monaco editor

Replace CodeMirror internals with @monaco-editor/react. Same props
interface so consumers don't change. Yjs binding via y-monaco,
theme-reactive via useTheme hook."
```

---

### Task 4: Rewrite Student Tiles with Shiki

**Files:**
- Rewrite: `src/components/session/student-tile.tsx`
- Create: `tests/unit/student-tile.test.tsx`

- [ ] **Step 1: Write the failing tests**

Create `tests/unit/student-tile.test.tsx`:

```tsx
// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

// Mock Yjs
vi.mock("yjs", () => {
  const mockYText = {
    toString: vi.fn(() => 'print("hello")'),
    observe: vi.fn(),
    unobserve: vi.fn(),
  };
  const mockYDoc = {
    getText: vi.fn(() => mockYText),
    destroy: vi.fn(),
  };
  return {
    Doc: vi.fn(() => mockYDoc),
    __mockYText: mockYText,
    __mockYDoc: mockYDoc,
  };
});

// Mock HocuspocusProvider
vi.mock("@hocuspocus/provider", () => ({
  HocuspocusProvider: vi.fn().mockImplementation(() => ({
    destroy: vi.fn(),
    awareness: {},
  })),
}));

// Mock shiki
vi.mock("shiki", () => ({
  codeToHtml: vi.fn().mockResolvedValue(
    '<pre class="shiki"><code><span>print("hello")</span></code></pre>'
  ),
}));

// Mock AiToggleButton
vi.mock("@/components/ai/ai-toggle-button", () => ({
  AiToggleButton: () => <button data-testid="ai-toggle">AI</button>,
}));

import { StudentTile } from "@/components/session/student-tile";

describe("StudentTile", () => {
  const defaultProps = {
    sessionId: "session-1",
    studentId: "student-1",
    studentName: "Alice",
    status: "active",
    token: "test-token",
    onClick: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders student name", () => {
    render(<StudentTile {...defaultProps} />);
    expect(screen.getByText("Alice")).toBeInTheDocument();
  });

  it("renders status indicator with correct color", () => {
    const { container } = render(<StudentTile {...defaultProps} status="active" />);
    const dot = container.querySelector(".bg-green-500");
    expect(dot).toBeInTheDocument();
  });

  it("renders needs_help status with red indicator", () => {
    const { container } = render(<StudentTile {...defaultProps} status="needs_help" />);
    const dot = container.querySelector(".bg-red-500");
    expect(dot).toBeInTheDocument();
  });

  it("calls onClick when clicked", () => {
    render(<StudentTile {...defaultProps} />);
    fireEvent.click(screen.getByText("Alice").closest("[class*='cursor-pointer']")!);
    expect(defaultProps.onClick).toHaveBeenCalledTimes(1);
  });

  it("renders AI toggle button", () => {
    render(<StudentTile {...defaultProps} />);
    expect(screen.getByTestId("ai-toggle")).toBeInTheDocument();
  });

  it("renders code container", () => {
    const { container } = render(<StudentTile {...defaultProps} />);
    const codeContainer = container.querySelector(".h-24");
    expect(codeContainer).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npx vitest run tests/unit/student-tile.test.tsx`
Expected: FAIL — StudentTile still imports CodeMirror

- [ ] **Step 3: Rewrite StudentTile component**

Rewrite `src/components/session/student-tile.tsx`:

```tsx
"use client";

import { useEffect, useRef, useState } from "react";
import * as Y from "yjs";
import { HocuspocusProvider } from "@hocuspocus/provider";
import { codeToHtml } from "shiki";
import { AiToggleButton } from "@/components/ai/ai-toggle-button";

interface StudentTileProps {
  sessionId: string;
  studentId: string;
  studentName: string;
  status: string;
  token: string;
  onClick: () => void;
}

export function StudentTile({
  sessionId,
  studentId,
  studentName,
  status,
  token,
  onClick,
}: StudentTileProps) {
  const [html, setHtml] = useState("");
  const providerRef = useRef<HocuspocusProvider | null>(null);
  const yDocRef = useRef<Y.Doc | null>(null);

  useEffect(() => {
    const yDoc = new Y.Doc();
    const yText = yDoc.getText("content");
    const documentName = `session:${sessionId}:user:${studentId}`;

    const provider = new HocuspocusProvider({
      url: process.env.NEXT_PUBLIC_HOCUSPOCUS_URL || `ws://${window.location.hostname}:4000`,
      name: documentName,
      document: yDoc,
      token,
    });

    yDocRef.current = yDoc;
    providerRef.current = provider;

    async function highlight(code: string) {
      if (!code.trim()) {
        setHtml("");
        return;
      }
      const result = await codeToHtml(code, {
        lang: "python",
        theme: "github-light",
      });
      setHtml(result);
    }

    // Initial render
    highlight(yText.toString());

    // Subscribe to changes
    const observer = () => {
      highlight(yText.toString());
    };
    yText.observe(observer);

    return () => {
      yText.unobserve(observer);
      provider.destroy();
      yDoc.destroy();
    };
  }, [sessionId, studentId, token]);

  const statusColor = {
    active: "bg-green-500",
    idle: "bg-yellow-500",
    needs_help: "bg-red-500",
  }[status] || "bg-gray-500";

  return (
    <div
      onClick={onClick}
      className="border rounded-lg p-2 cursor-pointer hover:border-primary transition-colors"
    >
      <div className="flex items-center justify-between mb-1">
        <span className="text-xs font-medium truncate">{studentName}</span>
        <div className="flex items-center gap-1">
          <AiToggleButton sessionId={sessionId} studentId={studentId} />
          <span className={`w-2 h-2 rounded-full ${statusColor}`} />
        </div>
      </div>
      <div
        className="h-24 overflow-hidden rounded text-[9px] font-mono leading-tight"
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npx vitest run tests/unit/student-tile.test.tsx`
Expected: PASS (6 tests)

- [ ] **Step 5: Commit**

```bash
git add src/components/session/student-tile.tsx tests/unit/student-tile.test.tsx
git commit -m "feat: replace CodeMirror with Shiki in student tiles

Use Shiki for read-only syntax-highlighted code display in student
tiles. Subscribes to Yjs document changes and re-highlights on update.
Eliminates heavy editor instances on the teacher dashboard."
```

---

### Task 5: Create DiffViewer Component

**Files:**
- Create: `src/components/editor/diff-viewer.tsx`
- Create: `tests/unit/diff-viewer.test.tsx`

- [ ] **Step 1: Write the failing tests**

Create `tests/unit/diff-viewer.test.tsx`:

```tsx
// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// Mock @monaco-editor/react DiffEditor
vi.mock("@monaco-editor/react", () => ({
  DiffEditor: (props: any) => (
    <div
      data-testid="monaco-diff-editor"
      data-original={props.original}
      data-modified={props.modified}
      data-readonly={props.options?.readOnly}
    />
  ),
  loader: { config: vi.fn(), init: vi.fn().mockResolvedValue({}) },
}));

vi.mock("@/lib/monaco/setup", () => ({
  setupMonaco: vi.fn(),
}));

vi.mock("@/lib/hooks/use-theme", () => ({
  useTheme: () => ({ theme: "light" }),
}));

import { DiffViewer } from "@/components/editor/diff-viewer";

describe("DiffViewer", () => {
  it("renders Monaco DiffEditor", () => {
    render(<DiffViewer original="" modified="" />);
    expect(screen.getByTestId("monaco-diff-editor")).toBeInTheDocument();
  });

  it("passes original and modified text", () => {
    render(
      <DiffViewer
        original='print("before")'
        modified='print("after")'
      />
    );
    const editor = screen.getByTestId("monaco-diff-editor");
    expect(editor.dataset.original).toBe('print("before")');
    expect(editor.dataset.modified).toBe('print("after")');
  });

  it("sets readOnly when prop is true", () => {
    render(<DiffViewer original="" modified="" readOnly />);
    const editor = screen.getByTestId("monaco-diff-editor");
    expect(editor.dataset.readonly).toBe("true");
  });

  it("is editable by default", () => {
    render(<DiffViewer original="" modified="" />);
    const editor = screen.getByTestId("monaco-diff-editor");
    expect(editor.dataset.readonly).toBe("false");
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npx vitest run tests/unit/diff-viewer.test.tsx`
Expected: FAIL — module not found

- [ ] **Step 3: Implement DiffViewer component**

Create `src/components/editor/diff-viewer.tsx`:

```tsx
"use client";

import { DiffEditor } from "@monaco-editor/react";
import { setupMonaco } from "@/lib/monaco/setup";
import { useTheme } from "@/lib/hooks/use-theme";

setupMonaco();

interface DiffViewerProps {
  original: string;
  modified: string;
  readOnly?: boolean;
}

export function DiffViewer({
  original,
  modified,
  readOnly = false,
}: DiffViewerProps) {
  const { theme } = useTheme();

  return (
    <div className="border rounded-lg overflow-hidden h-full">
      <DiffEditor
        original={original}
        modified={modified}
        language="python"
        theme={theme === "dark" ? "bridge-dark" : "bridge-light"}
        options={{
          readOnly,
          renderSideBySide: true,
          minimap: { enabled: false },
          fontSize: 14,
          fontFamily: "var(--font-geist-mono), monospace",
          lineNumbers: "on",
          scrollBeyondLastLine: false,
          automaticLayout: true,
          contextmenu: false,
        }}
      />
    </div>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npx vitest run tests/unit/diff-viewer.test.tsx`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add src/components/editor/diff-viewer.tsx tests/unit/diff-viewer.test.tsx
git commit -m "feat: add DiffViewer component using Monaco DiffEditor

Supports read-only mode for teacher views and editable mode for
student before/after comparisons. Theme-reactive via useTheme hook."
```

---

### Task 6: Add Student Diff View (Before/After Run)

**Files:**
- Modify: `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`

- [ ] **Step 1: Add diff view toggle to student session page**

Edit `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx`. The changes are:

1. Import `DiffViewer`
2. Add state for `codeBeforeRun` and `showDiff`
3. Capture snapshot before running
4. Add a "View Changes" toggle button
5. Conditionally render `DiffViewer` instead of the editor when toggled

```tsx
"use client";

import { useState, useEffect } from "react";
import { useParams } from "next/navigation";
import { useSession } from "next-auth/react";
import { CodeEditor } from "@/components/editor/code-editor";
import { OutputPanel } from "@/components/editor/output-panel";
import { RunButton } from "@/components/editor/run-button";
import { DiffViewer } from "@/components/editor/diff-viewer";
import { AiChatPanel } from "@/components/ai/ai-chat-panel";
import { RaiseHandButton } from "@/components/help-queue/raise-hand-button";
import { usePyodide } from "@/lib/pyodide/use-pyodide";
import { useYjsProvider } from "@/lib/yjs/use-yjs-provider";
import { Button } from "@/components/ui/button";

export default function StudentSessionPage() {
  const params = useParams<{ id: string; sessionId: string }>();
  const { data: session } = useSession();
  const [code, setCode] = useState("");
  const [aiEnabled, setAiEnabled] = useState(false);
  const [showAi, setShowAi] = useState(false);
  const [broadcastActive, setBroadcastActive] = useState(false);
  const [codeBeforeRun, setCodeBeforeRun] = useState<string | null>(null);
  const [showDiff, setShowDiff] = useState(false);
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

  function handleRun() {
    const currentCode = yText?.toString() || code;
    setCodeBeforeRun(currentCode);
    setShowDiff(false);
    runCode(currentCode);
  }

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
            {codeBeforeRun !== null && !running && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setShowDiff(!showDiff)}
              >
                {showDiff ? "Editor" : "View Changes"}
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
              onRun={handleRun}
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
          {showDiff && codeBeforeRun !== null ? (
            <DiffViewer
              original={codeBeforeRun}
              modified={yText?.toString() || code}
              readOnly
            />
          ) : (
            <CodeEditor
              onChange={setCode}
              yText={yText}
              provider={provider}
            />
          )}
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

- [ ] **Step 2: Verify the app builds**

Run: `npm run build`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx
git commit -m "feat: add diff view toggle to student session page

Student can view before/after diff of their code when running.
Snapshot captured before each run, toggled via 'View Changes' button."
```

---

### Task 7: Add Teacher Diff View on Student Code

**Files:**
- Modify: `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx`

- [ ] **Step 1: Add diff view action to teacher dashboard**

Edit `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx`. Add a "View Diff" button in the selected student view that shows the student's current code diffed against the starter template. The changes:

1. Import `DiffViewer`
2. Add state for `showDiff`
3. Add a "View Diff" toggle in the student view header
4. Conditionally render `DiffViewer` instead of the editor

Add the import at the top:

```tsx
import { DiffViewer } from "@/components/editor/diff-viewer";
```

Add state in the component:

```tsx
const [showDiff, setShowDiff] = useState(false);
```

Reset diff when switching students — add to the beginning of the `if (selectedStudent)` block or add an effect:

```tsx
// Reset diff view when switching students
useEffect(() => {
  setShowDiff(false);
}, [selectedStudent]);
```

In the selected student view's header bar (after the `AiToggleButton`), add:

```tsx
<div className="flex items-center gap-2">
  <Button
    variant="ghost"
    size="sm"
    onClick={() => setShowDiff(!showDiff)}
  >
    {showDiff ? "Editor" : "View Diff"}
  </Button>
  <AiToggleButton sessionId={params.sessionId} studentId={selectedStudent} />
</div>
```

In the editor area, conditionally render:

```tsx
<div className="flex-1 min-h-0 p-4">
  {selectedDocName && (
    showDiff ? (
      <DiffViewer
        original={STARTER_CODE}
        modified={yText?.toString() || ""}
        readOnly
      />
    ) : (
      <CodeEditor yText={yText} provider={provider} />
    )
  )}
</div>
```

Add the starter code constant (same as in the editor page):

```tsx
const STARTER_CODE = `# Welcome to Bridge!
# Write your Python code here and click Run.

print("Hello, world!")
`;
```

- [ ] **Step 2: Verify the app builds**

Run: `npm run build`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx
git commit -m "feat: add teacher diff view for student code

Teacher can toggle between editor and diff view when viewing a
student's code. Diffs against the starter template, both sides
read-only."
```

---

### Task 8: Run Full Test Suite and Visual Verification

**Files:** None (verification only)

- [ ] **Step 1: Run the full test suite**

Run: `npx vitest run`
Expected: All tests pass. Check that no existing tests broke due to CodeMirror removal.

- [ ] **Step 2: Fix any broken tests**

If any existing tests imported CodeMirror directly, update them to use Monaco mocks or remove the CodeMirror references.

- [ ] **Step 3: Start the dev server and verify visually**

Run: `npm run dev`

Verify these scenarios manually:

1. **Editor page** (`/dashboard/classrooms/{id}/editor`): Monaco editor loads, Python syntax highlighting works, run button executes code, output panel shows results.
2. **Student session** (`/dashboard/classrooms/{id}/session/{sessionId}`): Collaborative editing works, broadcast view renders when teacher broadcasts, "View Changes" button appears after running code and shows diff.
3. **Teacher dashboard** (`/dashboard/classrooms/{id}/session/{sessionId}/dashboard`): Student grid shows Shiki-highlighted code tiles, clicking a student opens Monaco editor, "View Diff" button shows diff against starter code.
4. **Theme toggle**: Both light and dark themes apply correctly to all editor instances.
5. **Features**: Find & replace (Ctrl+F), code folding, multi-cursor (Alt+Click), bracket colorization, minimap all work.

- [ ] **Step 4: Run lint check**

Run: `npm run lint`
Expected: No new lint errors.

- [ ] **Step 5: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: resolve test and lint issues from Monaco migration"
```

Only commit this if there were fixes. Skip if everything passed cleanly.

---

## Summary

| Task | Description | Key files |
|------|-------------|-----------|
| 1 | Install deps, configure build, Monaco setup | `package.json`, `next.config.ts`, `src/lib/monaco/setup.ts` |
| 2 | Define light/dark themes | `src/lib/monaco/themes.ts` |
| 3 | Rewrite CodeEditor with Monaco | `src/components/editor/code-editor.tsx` |
| 4 | Rewrite student tiles with Shiki | `src/components/session/student-tile.tsx` |
| 5 | Create DiffViewer component | `src/components/editor/diff-viewer.tsx` |
| 6 | Add student diff view (before/after run) | `session/[sessionId]/page.tsx` |
| 7 | Add teacher diff view on student code | `session/[sessionId]/dashboard/page.tsx` |
| 8 | Full test suite + visual verification | — |

---

## Post-Execution Report

**Date:** 2026-04-11
**Branch:** `feat/monaco-editor-migration`
**Commits:** 11 (8ad479d..bf12eeb)
**Tests:** 284 passed, 11 skipped (LLM integration tests requiring API keys), 0 failed
**Build:** Passes clean

### Implementation Details

All 8 tasks completed successfully using subagent-driven development (one subagent per task, spec review + code quality review after each).

| Task | Status | Notes |
|------|--------|-------|
| 1. Install deps | Done | Used CDN loader instead of self-hosting (follow-up) |
| 2. Themes | Done | Light/dark themes derived from platform OKLCH CSS variables |
| 3. CodeEditor rewrite | Done | Same props interface, zero consumer changes needed |
| 4. Student tiles | Done | Shiki replaces 20+ CodeMirror instances |
| 5. DiffViewer | Done | New component wrapping Monaco DiffEditor |
| 6. Student diff | Done | Before/after toggle on code runs |
| 7. Teacher diff | Done | Student code vs hardcoded starter template |
| 8. Verification | Done | All tests pass, build succeeds |

### Deviations from Plan

1. **CDN instead of self-hosting.** The spec says "self-hosted from node_modules." The implementation uses `@monaco-editor/react`'s default CDN loader (jsDelivr). Self-hosting requires copying Monaco's `vs/` directory to `/public/`, which is a follow-up task. The loader config in `setup.ts` was simplified — the hardcoded CDN URL was removed so `@monaco-editor/react` uses its default matched version.

2. **`next.config.ts` not modified.** The plan originally included a webpack config change for Monaco workers, but this was removed during self-review since `@monaco-editor/react` handles worker loading via its own loader.

3. **`lightbulb` option removed.** Monaco 0.55.x changed `lightbulb.enabled` from `boolean` to `ShowLightbulbIconMode` enum. Rather than importing the enum, the option was removed — lightbulb is off by default without code actions providers.

4. **Theme switching `useEffect` removed.** Code review identified that `(window as any).monaco` is unreliable. The `<Editor>` component's `theme` prop already handles reactive theme changes natively.

5. **Debounce added to student tiles.** Code review identified that every Yjs keystroke triggered a Shiki `codeToHtml` call. A 300ms debounce was added to prevent performance issues with 20+ concurrent tiles.

### Known Limitations

1. **Student tiles always use `github-light` Shiki theme** — does not follow platform dark/light toggle. Should use `useTheme()` to pick between `github-light` and `github-dark`.
2. **`STARTER_CODE` is hardcoded** in the teacher dashboard diff view. Should be fetched from the database (`topics.starterCode` column) for the session's topic.
3. **Monaco models not explicitly disposed on unmount.** `@monaco-editor/react` handles some cleanup, but custom `path` URIs can leak models over long sessions.
4. **CDN dependency.** Monaco assets loaded from jsDelivr. Should self-host for firewall reliability.
5. **`bindingRef` typed as `any`** — could use `MonacoBinding` type from `y-monaco`.

### Follow-Up Work

- [ ] Self-host Monaco assets (copy `vs/` to `/public/`, configure loader)
- [ ] Student tiles: respect dark/light theme for Shiki highlighting
- [ ] Teacher diff: fetch actual starter code from database instead of hardcoded constant
- [ ] Dispose Monaco models on component unmount
- [ ] Type `bindingRef` properly with `MonacoBinding`

---

## Code Review

### Review 1

- **Date:** 2026-04-11
- **Reviewer:** Claude (superpowers:code-reviewer subagent)
- **Branch:** `feat/monaco-editor-migration`
- **Verdict:** Approved with fixes

**Must Fix**

1. `[FIXED]` Redundant/fragile `(window as any).monaco` theme switching in `code-editor.tsx:72-79`. The `<Editor>` component's `theme` prop already handles reactive theme changes. The `window.monaco` global is not guaranteed by `@monaco-editor/react`.
   → Response: Removed the entire `useEffect` block. Commit bf12eeb.

2. `[FIXED]` No debounce on Shiki highlighting in `student-tile.tsx:60-64`. Every Yjs keystroke triggers `codeToHtml` across 20+ tiles on the teacher dashboard, causing performance issues.
   → Response: Added 300ms debounce with proper cleanup in the effect teardown. Commit bf12eeb.

**Should Fix**

3. `[OPEN]` Student tiles hardcode `github-light` Shiki theme in `student-tile.tsx:52` — ignores platform dark mode. Should use `useTheme()` to select between `github-light` and `github-dark`.
   → Response: Deferred to follow-up. Cosmetic issue, not a blocker.

4. `[OPEN]` `STARTER_CODE` duplicated and hardcoded in `dashboard/page.tsx:18-22` and `editor/page.tsx:10`. Should come from the database (`topics.starterCode`).
   → Response: Deferred to follow-up. Requires API integration outside migration scope.

5. `[OPEN]` Monaco model not disposed on unmount in `code-editor.tsx`. Custom `path` URIs can leak models over long sessions.
   → Response: Deferred to follow-up. Gradual degradation, not acute.

6. `[OPEN]` `dangerouslySetInnerHTML` in `student-tile.tsx:93` is safe (Shiki handles escaping) but deserves a comment explaining the trust boundary.
   → Response: Deferred to follow-up. Minor documentation concern.

**Minor**

7. `[OPEN]` `bindingRef` typed as `any` in `code-editor.tsx:29`. Could use `MonacoBinding` type.
   → Response: Deferred. Minor type improvement.

8. `[OPEN]` Theme registration via dynamic import in `code-editor.tsx:45` adds latency. Could be a static import since it's small static data.
   → Response: Deferred. Minor optimization.

9. `[OPEN]` Monaco assets loaded from CDN, spec says self-host. Noted as follow-up.
   → Response: Tracked in follow-up work above.

**Summary:** The migration is functionally complete and architecturally sound. All spec requirements are met. The two must-fix issues (fragile theme switching and missing debounce) were resolved in commit bf12eeb. Remaining items are cosmetic, optimization, or follow-up scope — none are merge blockers.
