# 003 — Monaco Editor Migration

**Date:** 2026-04-11
**Status:** Approved

## Overview

Migrate the code editor from CodeMirror 6 to Monaco Editor to gain advanced IDE features: IntelliSense (architecture-ready), diff view, find & replace, code folding, multi-cursor, bracket pair colorization, and minimap. The platform targets web browsers; mobile/Chromebook optimization is not a priority.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Editor library | Monaco via `@monaco-editor/react` | Advanced IDE features (IntelliSense, diff view) |
| Asset hosting | Self-hosted from `node_modules` | No CDN dependency, works behind firewalls |
| Student tiles | Shiki (read-only renderer) | Avoid 20+ Monaco instances on teacher dashboard |
| Migration strategy | Big bang replace | Clean abstraction layer, 6 consumers, not in production |
| Python autocomplete | Basic keywords now, LSP-ready architecture | Start simple, upgrade path without rework |
| Diff view | Monaco DiffEditor | Student before/after on run; teacher diffs student vs. starter |
| Theming | Light + dark themes following platform theme | Reactive via `useTheme()` hook |

## Architecture

### 1. Core Editor Component

**File:** `src/components/editor/code-editor.tsx`

Props interface preserved — consumers don't change:

```typescript
interface CodeEditorProps {
  initialCode?: string;
  onChange?: (code: string) => void;
  readOnly?: boolean;
  yText?: Y.Text | null;
  provider?: HocuspocusProvider | null;
}
```

Internals switch to `@monaco-editor/react`'s `<Editor>` component:

- Each instance gets a unique model URI (`inmemory://editor/{uuid}`) to avoid global model registry conflicts.
- Language: `python` (hardcoded for now; a `language` prop can be added later).
- `readOnly` maps directly to Monaco's `readOnly` option.
- `onChange` wires to Monaco's `onDidChangeModelContent` event.

**Enabled features:**
- Find & replace
- Code folding
- Multi-cursor editing
- Bracket pair colorization
- Minimap
- Hover hints (placeholder for future IntelliSense)
- Parameter hints (placeholder for future IntelliSense)

**Disabled features:**
- Command palette (confusing for students)
- Excessive context menu items (keep it focused)

### 2. Monaco Setup

**File:** `src/lib/monaco/setup.ts`

- Configures `@monaco-editor/loader` to load Monaco from `node_modules/monaco-editor` instead of CDN.
- Registers a `CompletionItemProvider` for Python with basic keyword/snippet completions.
- Clean provider interface so a future language server integration just swaps the provider implementation without touching the editor component.
- Imported once by the editor component.

### 3. Yjs Collaboration

**Hook:** `src/lib/yjs/use-yjs-provider.ts` — unchanged (editor-agnostic).

**Binding:** When `yText` + `provider` are provided, apply `y-monaco`'s `MonacoBinding` in Monaco's `onMount` callback:
- `MonacoBinding` takes the editor model, `yText`, and `provider.awareness`.
- Same pattern as the current `yCollab` call in CodeMirror.

**Non-collaborative mode:** Uses `initialCode` as `defaultValue` with standard `onChange`.

**Note:** In collaborative mode, `onChange` is still wired up (if provided) so consumers like the session page can track the current code for Python execution. The Yjs binding handles sync; `onChange` is a read-only tap on the content.

### 4. Student Tiles

**File:** `src/components/session/student-tile.tsx`

Replace direct CodeMirror usage with Shiki (`react-shiki`) for read-only syntax-highlighted display:
- Subscribe to `yText.observe` to get content updates from the Yjs document.
- Render via Shiki with Python highlighting.
- Eliminates the heaviest page — no more 20+ editor instances on the teacher dashboard.

### 5. Diff View

**File:** `src/components/editor/diff-viewer.tsx`

Uses Monaco's `DiffEditor` component.

**Props:**
```typescript
interface DiffViewerProps {
  original: string;    // left side
  modified: string;    // right side
  readOnly?: boolean;  // both sides read-only for teacher view
}
```

**Use cases:**
- **Student:** Sees before/after their own code when running. A snapshot is captured before each run, stored in component state (no persistence). After running, a "View Changes" toggle shows the inline diff (before-run vs. current).
- **Teacher:** Can diff a student's current code against the starter template. "View Diff" action on the selected student's tile opens the diff viewer with the student's current Yjs content vs. the starter code. Both sides read-only.

### 6. Theming

**File:** `src/lib/monaco/themes.ts`

Two Monaco themes defined:
- **Light theme:** Colors derived from the platform's existing OKLCH CSS variables (`:root`), converted to hex for Monaco's `defineTheme` API.
- **Dark theme:** Colors derived from the `.dark` CSS variables.

The active theme is driven by the `useTheme()` hook. The editor re-themes reactively when the user toggles. Default follows the platform default (light, once the platform default is switched).

### 7. Package Changes

**Remove:**
- `codemirror`
- `@codemirror/autocomplete`
- `@codemirror/commands`
- `@codemirror/lang-python`
- `@codemirror/language`
- `@codemirror/state`
- `@codemirror/view`
- `y-codemirror.next`

**Add:**
- `monaco-editor`
- `@monaco-editor/react`
- `y-monaco`
- `shiki` (or `react-shiki`)

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `src/components/editor/code-editor.tsx` | Rewrite | Swap CodeMirror internals for Monaco |
| `src/components/session/student-tile.tsx` | Rewrite | Replace CodeMirror with Shiki |
| `src/lib/monaco/setup.ts` | New | Monaco loader config + autocomplete provider |
| `src/lib/monaco/themes.ts` | New | Light/dark theme definitions |
| `src/components/editor/diff-viewer.tsx` | New | Diff view component |
| `package.json` | Modify | Swap dependencies |
| `src/app/dashboard/classrooms/[id]/editor/page.tsx` | Minimal | No changes expected (same props) |
| `src/app/dashboard/classrooms/[id]/session/[sessionId]/page.tsx` | Minimal | Add diff view toggle for student |
| `src/app/dashboard/classrooms/[id]/session/[sessionId]/dashboard/page.tsx` | Modify | Add diff view action for teacher |
| `src/components/session/broadcast-controls.tsx` | Minimal | No changes expected (same props) |

## Testing

- Unit tests for `CodeEditor` in both collaborative and standalone modes.
- Unit tests for `DiffViewer` with various original/modified inputs.
- Unit tests for student tile rendering with Shiki.
- Integration tests for Yjs binding with Monaco (MonacoBinding lifecycle).
- Visual verification: editor page, student session, teacher dashboard, broadcast mode.
- Theme toggle verification: both themes render correctly in all editor contexts.
