# Live Editor Implementation Plan (Plan 2 of 4)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a browser-based code editor with Python execution so students can write and run Python code from within a classroom — no server-side execution required.

**Architecture:** CodeMirror 6 as the editor component, Pyodide (CPython compiled to WASM) for in-browser Python execution. The editor page lives under the classroom route. A Pyodide Web Worker handles execution off the main thread. Output (stdout/stderr) streams to a console panel below the editor.

**Tech Stack:** CodeMirror 6, @codemirror/lang-python, Pyodide 0.27+, Web Workers, React

**Spec:** `docs/specs/001-bridge-platform-design.md` (Code Execution section)

**Depends on:** Plan 1 (Foundation) — classroom pages, auth, layout

---

## File Structure

```
src/
├── app/
│   └── dashboard/
│       └── classrooms/
│           └── [id]/
│               ├── page.tsx                        # Existing — add "Open Editor" link
│               └── editor/
│                   └── page.tsx                    # Editor page (client component)
├── components/
│   ├── editor/
│   │   ├── code-editor.tsx                         # CodeMirror 6 wrapper
│   │   ├── output-panel.tsx                        # Console output display
│   │   └── run-button.tsx                          # Run/Stop button
│   └── ui/                                         # Existing shadcn components
├── lib/
│   └── pyodide/
│       └── use-pyodide.ts                          # React hook for Pyodide worker
└── workers/
    └── pyodide-worker.ts                           # Web Worker for Pyodide execution
tests/
└── unit/
    ├── output-panel.test.tsx                        # Output panel rendering tests
    └── use-pyodide.test.ts                         # Pyodide hook logic tests
```

---

## Task 1: Install Editor Dependencies

**Files:**
- Modify: `package.json`

- [ ] **Step 1: Install CodeMirror packages**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun add codemirror @codemirror/lang-python @codemirror/view @codemirror/state @codemirror/commands @codemirror/autocomplete @codemirror/language
```

- [ ] **Step 2: Verify installation**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build completes without errors.

- [ ] **Step 3: Commit**

```bash
git add package.json bun.lock
git commit -m "chore: install CodeMirror 6 dependencies"
```

---

## Task 2: CodeMirror Editor Component

**Files:**
- Create: `src/components/editor/code-editor.tsx`

- [ ] **Step 1: Create CodeMirror wrapper component**

Create `src/components/editor/code-editor.tsx`:

```tsx
"use client";

import { useRef, useEffect } from "react";
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { defaultKeymap, indentWithTab, history, historyKeymap } from "@codemirror/commands";
import { python } from "@codemirror/lang-python";
import { syntaxHighlighting, defaultHighlightStyle, bracketMatching, indentOnInput } from "@codemirror/language";
import { autocompletion, closeBrackets } from "@codemirror/autocomplete";

interface CodeEditorProps {
  initialCode?: string;
  onChange?: (code: string) => void;
  readOnly?: boolean;
}

export function CodeEditor({ initialCode = "", onChange, readOnly = false }: CodeEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const state = EditorState.create({
      doc: initialCode,
      extensions: [
        lineNumbers(),
        highlightActiveLine(),
        highlightActiveLineGutter(),
        history(),
        bracketMatching(),
        closeBrackets(),
        indentOnInput(),
        autocompletion(),
        python(),
        syntaxHighlighting(defaultHighlightStyle),
        keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
        EditorView.editable.of(!readOnly),
        EditorView.theme({
          "&": { height: "100%", fontSize: "14px" },
          ".cm-scroller": { overflow: "auto", fontFamily: "var(--font-geist-mono), monospace" },
          ".cm-content": { minHeight: "200px" },
        }),
        ...(onChange
          ? [
              EditorView.updateListener.of((update) => {
                if (update.docChanged) {
                  onChange(update.state.doc.toString());
                }
              }),
            ]
          : []),
      ],
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
  }, []);

  return (
    <div
      ref={containerRef}
      className="border rounded-lg overflow-hidden h-full"
    />
  );
}
```

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/components/editor/code-editor.tsx
git commit -m "feat: add CodeMirror 6 editor component

Python syntax highlighting, line numbers, bracket matching,
auto-completion, keyboard shortcuts, and onChange callback."
```

---

## Task 3: Output Panel Component

**Files:**
- Create: `src/components/editor/output-panel.tsx`
- Test: `tests/unit/output-panel.test.tsx`

- [ ] **Step 1: Write failing test for output panel**

Create `tests/unit/output-panel.test.tsx`:

```tsx
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { OutputPanel } from "@/components/editor/output-panel";

describe("OutputPanel", () => {
  it("renders empty state when no output", () => {
    render(<OutputPanel output={[]} running={false} />);
    expect(screen.getByTestId("output-panel")).toBeInTheDocument();
    expect(screen.getByTestId("output-panel")).toHaveTextContent("");
  });

  it("renders stdout lines", () => {
    const output = [
      { type: "stdout" as const, text: "Hello, world!" },
      { type: "stdout" as const, text: "Line 2" },
    ];
    render(<OutputPanel output={output} running={false} />);
    expect(screen.getByText("Hello, world!")).toBeInTheDocument();
    expect(screen.getByText("Line 2")).toBeInTheDocument();
  });

  it("renders stderr lines with error styling", () => {
    const output = [
      { type: "stderr" as const, text: "NameError: name 'x' is not defined" },
    ];
    render(<OutputPanel output={output} running={false} />);
    const errorLine = screen.getByText("NameError: name 'x' is not defined");
    expect(errorLine).toBeInTheDocument();
    expect(errorLine.className).toContain("text-red");
  });

  it("shows running indicator when running", () => {
    render(<OutputPanel output={[]} running={true} />);
    expect(screen.getByText("Running...")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/output-panel.test.tsx
```

Expected: FAIL — cannot resolve `@/components/editor/output-panel`.

- [ ] **Step 3: Update vitest config for React component tests**

Modify `vitest.config.ts` — change the `environment` for component tests. Replace the existing config:

```typescript
import { defineConfig } from "vitest/config";
import path from "path";

export default defineConfig({
  test: {
    globals: true,
    environment: "node",
    setupFiles: ["./tests/setup.ts"],
    include: ["tests/**/*.test.ts", "tests/**/*.test.tsx"],
    fileParallelism: false,
    environmentMatchGlobs: [
      ["tests/**/*.test.tsx", "jsdom"],
    ],
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
```

- [ ] **Step 4: Create a test setup file for jsdom tests**

Create `tests/setup-dom.ts`:

```typescript
import "@testing-library/jest-dom/vitest";
```

Update `vitest.config.ts` setupFiles to include both:

```typescript
    setupFiles: ["./tests/setup.ts", "./tests/setup-dom.ts"],
```

- [ ] **Step 5: Implement OutputPanel**

Create `src/components/editor/output-panel.tsx`:

```tsx
"use client";

export interface OutputLine {
  type: "stdout" | "stderr";
  text: string;
}

interface OutputPanelProps {
  output: OutputLine[];
  running: boolean;
}

export function OutputPanel({ output, running }: OutputPanelProps) {
  return (
    <div
      data-testid="output-panel"
      className="bg-zinc-950 text-zinc-100 font-mono text-sm p-3 rounded-lg overflow-auto h-full min-h-[120px]"
    >
      {running && (
        <div className="text-yellow-400 mb-1">Running...</div>
      )}
      {output.map((line, i) => (
        <div
          key={i}
          className={
            line.type === "stderr"
              ? "text-red-400 whitespace-pre-wrap"
              : "whitespace-pre-wrap"
          }
        >
          {line.text}
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 6: Run test to verify it passes**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/output-panel.test.tsx
```

Expected: All 4 tests pass.

- [ ] **Step 7: Commit**

```bash
git add src/components/editor/output-panel.tsx tests/unit/output-panel.test.tsx vitest.config.ts tests/setup-dom.ts
git commit -m "feat: add output panel component with stdout/stderr display

Dark terminal-style panel. Stderr shown in red. Running indicator.
Includes component tests with jsdom environment."
```

---

## Task 4: Pyodide Web Worker

**Files:**
- Create: `src/workers/pyodide-worker.ts`
- Create: `src/lib/pyodide/use-pyodide.ts`
- Test: `tests/unit/use-pyodide.test.ts`

- [ ] **Step 1: Create the Pyodide Web Worker**

Create `src/workers/pyodide-worker.ts`:

```typescript
/// <reference lib="webworker" />

declare const self: DedicatedWorkerGlobalScope;

interface RunMessage {
  type: "run";
  code: string;
  id: string;
}

interface InitMessage {
  type: "init";
}

type WorkerMessage = RunMessage | InitMessage;

let pyodide: any = null;

async function initPyodide() {
  if (pyodide) return;

  importScripts("https://cdn.jsdelivr.net/pyodide/v0.27.5/full/pyodide.js");

  pyodide = await (self as any).loadPyodide({
    stdout: (text: string) => {
      self.postMessage({ type: "stdout", text });
    },
    stderr: (text: string) => {
      self.postMessage({ type: "stderr", text });
    },
  });

  self.postMessage({ type: "ready" });
}

async function runCode(code: string, id: string) {
  if (!pyodide) {
    self.postMessage({ type: "stderr", text: "Pyodide not initialized" });
    self.postMessage({ type: "done", id, success: false });
    return;
  }

  try {
    await pyodide.runPythonAsync(code);
    self.postMessage({ type: "done", id, success: true });
  } catch (err: any) {
    self.postMessage({ type: "stderr", text: err.message });
    self.postMessage({ type: "done", id, success: false });
  }
}

self.onmessage = async (event: MessageEvent<WorkerMessage>) => {
  const { data } = event;

  switch (data.type) {
    case "init":
      await initPyodide();
      break;
    case "run":
      await runCode(data.code, data.id);
      break;
  }
};
```

- [ ] **Step 2: Create the usePyodide React hook**

Create `src/lib/pyodide/use-pyodide.ts`:

```typescript
"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import type { OutputLine } from "@/components/editor/output-panel";

interface UsePyodideReturn {
  ready: boolean;
  running: boolean;
  output: OutputLine[];
  runCode: (code: string) => void;
  clearOutput: () => void;
}

export function usePyodide(): UsePyodideReturn {
  const [ready, setReady] = useState(false);
  const [running, setRunning] = useState(false);
  const [output, setOutput] = useState<OutputLine[]>([]);
  const workerRef = useRef<Worker | null>(null);

  useEffect(() => {
    const worker = new Worker(
      new URL("@/workers/pyodide-worker.ts", import.meta.url),
      { type: "classic" }
    );

    worker.onmessage = (event) => {
      const { data } = event;

      switch (data.type) {
        case "ready":
          setReady(true);
          break;
        case "stdout":
          setOutput((prev) => [...prev, { type: "stdout", text: data.text }]);
          break;
        case "stderr":
          setOutput((prev) => [...prev, { type: "stderr", text: data.text }]);
          break;
        case "done":
          setRunning(false);
          break;
      }
    };

    worker.postMessage({ type: "init" });
    workerRef.current = worker;

    return () => {
      worker.terminate();
      workerRef.current = null;
    };
  }, []);

  const runCode = useCallback((code: string) => {
    if (!workerRef.current || !ready) return;
    setRunning(true);
    setOutput([]);
    const id = crypto.randomUUID();
    workerRef.current.postMessage({ type: "run", code, id });
  }, [ready]);

  const clearOutput = useCallback(() => {
    setOutput([]);
  }, []);

  return { ready, running, output, runCode, clearOutput };
}
```

- [ ] **Step 3: Write test for usePyodide hook logic**

Create `tests/unit/use-pyodide.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import type { OutputLine } from "@/components/editor/output-panel";

describe("OutputLine type", () => {
  it("accepts stdout type", () => {
    const line: OutputLine = { type: "stdout", text: "hello" };
    expect(line.type).toBe("stdout");
    expect(line.text).toBe("hello");
  });

  it("accepts stderr type", () => {
    const line: OutputLine = { type: "stderr", text: "error" };
    expect(line.type).toBe("stderr");
    expect(line.text).toBe("error");
  });
});

describe("worker message protocol", () => {
  it("init message has correct shape", () => {
    const msg = { type: "init" as const };
    expect(msg.type).toBe("init");
  });

  it("run message has correct shape", () => {
    const msg = { type: "run" as const, code: "print('hi')", id: "abc-123" };
    expect(msg.type).toBe("run");
    expect(msg.code).toBe("print('hi')");
    expect(msg.id).toBe("abc-123");
  });
});
```

- [ ] **Step 4: Run tests**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test tests/unit/use-pyodide.test.ts
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/workers/pyodide-worker.ts src/lib/pyodide/use-pyodide.ts tests/unit/use-pyodide.test.ts
git commit -m "feat: add Pyodide Web Worker and usePyodide hook

Python execution runs in a Web Worker off the main thread.
Streams stdout/stderr back to the React state via postMessage.
Worker loads Pyodide from CDN on init."
```

---

## Task 5: Run Button Component

**Files:**
- Create: `src/components/editor/run-button.tsx`

- [ ] **Step 1: Create RunButton component**

Create `src/components/editor/run-button.tsx`:

```tsx
"use client";

import { Button } from "@/components/ui/button";

interface RunButtonProps {
  onRun: () => void;
  running: boolean;
  ready: boolean;
}

export function RunButton({ onRun, running, ready }: RunButtonProps) {
  return (
    <Button
      onClick={onRun}
      disabled={running || !ready}
      size="sm"
      variant={running ? "outline" : "default"}
    >
      {!ready ? "Loading Python..." : running ? "Running..." : "Run"}
    </Button>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add src/components/editor/run-button.tsx
git commit -m "feat: add Run button component with loading/running states"
```

---

## Task 6: Editor Page

**Files:**
- Create: `src/app/dashboard/classrooms/[id]/editor/page.tsx`
- Modify: `src/app/dashboard/classrooms/[id]/page.tsx`

- [ ] **Step 1: Create the editor page**

Create `src/app/dashboard/classrooms/[id]/editor/page.tsx`:

```tsx
"use client";

import { useState } from "react";
import { CodeEditor } from "@/components/editor/code-editor";
import { OutputPanel } from "@/components/editor/output-panel";
import { RunButton } from "@/components/editor/run-button";
import { usePyodide } from "@/lib/pyodide/use-pyodide";
import { Button } from "@/components/ui/button";

const STARTER_CODE = `# Welcome to Bridge!
# Write your Python code here and click Run.

print("Hello, world!")
`;

export default function EditorPage() {
  const [code, setCode] = useState(STARTER_CODE);
  const { ready, running, output, runCode, clearOutput } = usePyodide();

  return (
    <div className="flex flex-col h-[calc(100vh-3.5rem)] gap-2 p-0">
      <div className="flex items-center justify-between px-4 pt-2">
        <h2 className="text-sm font-medium text-muted-foreground">Python Editor</h2>
        <div className="flex gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={clearOutput}
            disabled={running}
          >
            Clear
          </Button>
          <RunButton onRun={() => runCode(code)} running={running} ready={ready} />
        </div>
      </div>

      <div className="flex-1 min-h-0 px-4">
        <CodeEditor initialCode={STARTER_CODE} onChange={setCode} />
      </div>

      <div className="h-[200px] shrink-0 px-4 pb-4">
        <OutputPanel output={output} running={running} />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Add "Open Editor" link to classroom detail page**

Modify `src/app/dashboard/classrooms/[id]/page.tsx`. Add a link to the editor after the classroom header. Add this import at the top:

```tsx
import { buttonVariants } from "@/components/ui/button";
import Link from "next/link";
```

Then add after the closing `</div>` of the header section (after `{classroom.description && ...}`), before `{isTeacher && (`:

```tsx
      <div className="flex gap-2">
        <Link
          href={`/dashboard/classrooms/${id}/editor`}
          className={buttonVariants()}
        >
          Open Editor
        </Link>
      </div>
```

- [ ] **Step 3: Configure Next.js for Web Workers**

Modify `next.config.ts` to support the Web Worker import pattern. Read the current file, then replace its contents:

```typescript
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  webpack: (config) => {
    config.resolve.fallback = {
      ...config.resolve.fallback,
      fs: false,
      path: false,
      crypto: false,
    };
    return config;
  },
};

export default nextConfig;
```

- [ ] **Step 4: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build completes. The editor page shows as a dynamic route.

- [ ] **Step 5: Commit**

```bash
git add src/app/dashboard/classrooms/\[id\]/editor/page.tsx src/app/dashboard/classrooms/\[id\]/page.tsx next.config.ts
git commit -m "feat: add editor page with Python execution

CodeMirror 6 editor with Pyodide execution in a Web Worker.
Students can write and run Python code from the classroom.
Output streams to a terminal-style console panel."
```

---

## Task 7: Full Test Suite Verification and Build Check

**Files:**
- None (verification only)

- [ ] **Step 1: Run full test suite**

```bash
export PATH="$HOME/.bun/bin:$PATH"
DATABASE_URL="postgresql://work@127.0.0.1:5432/bridge_test" bun run test
```

Expected: All tests pass (previous 23 + new output panel + use-pyodide tests).

- [ ] **Step 2: Verify build**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run build
```

Expected: Build succeeds. Routes include `/dashboard/classrooms/[id]/editor`.

- [ ] **Step 3: Manual smoke test**

```bash
export PATH="$HOME/.bun/bin:$PATH"
bun run dev
```

1. Navigate to `http://localhost:3000`, log in
2. Open a classroom → click "Open Editor"
3. Verify CodeMirror editor loads with Python syntax highlighting
4. Wait for "Loading Python..." to become "Run"
5. Click "Run" — should see "Hello, world!" in the output panel
6. Edit code to have an error (e.g., `print(x)`) → Run → verify red error output

- [ ] **Step 4: Commit any fixes from smoke test**

If smoke testing reveals issues, fix and commit:

```bash
git add -A
git commit -m "fix: address issues found during editor smoke test"
```
