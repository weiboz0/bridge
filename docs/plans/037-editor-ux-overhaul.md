# 037 — Teaching Unit Editor: Notion-Like UX Overhaul

**Goal:** Close the UX gap between our Tiptap editor and a Notion-like experience. Add a floating toolbar, block manipulation (drag/move/delete), margin controls, expanded block types, AI selection tools, and keyboard shortcuts. The Tiptap Notion-like template and BlockNote are the baseline — we should exceed them with our teaching-specific blocks.

**Baseline references:**
- https://template.tiptap.dev/notion-like/Wb8BhbtRbR (minimum feature set)
- https://www.blocknotejs.org/ (block manipulation UX)

**Branch:** `feat/037-editor-ux`

**Depends On:** Plans 031-036 (teaching unit editor exists)

**Status:** Not started

---

## Current State (~25% feature-complete vs Notion)

**Have:** Slash menu (16 commands), 8 custom teaching blocks, AI drafting panel + inline prompt, collaborative editing (Yjs), basic StarterKit (bold/italic/strike via keyboard), ProseMirror CSS.

**Missing:** Toolbar, drag-and-drop, margin controls, tables, inline formatting UI, AI selection tools, keyboard shortcuts, and many block types.

---

## Phase 1: Toolbar + Inline Formatting (the most visible gap)

### Task 1.1: Bubble menu (floating toolbar on text selection)

**Extensions to activate:** `@tiptap/extension-bubble-menu` (already installed), `@tiptap/extension-underline` (installed), `@tiptap/extension-link` (installed).

**Extensions to install:** `@tiptap/extension-highlight`, `@tiptap/extension-text-style`, `@tiptap/extension-color`, `@tiptap/extension-subscript`, `@tiptap/extension-superscript`.

**Toolbar buttons (appear on text selection):**
- Bold (Cmd+B), Italic (Cmd+I), Strikethrough (Cmd+Shift+X), Underline (Cmd+U)
- Code (inline), Link (Cmd+K — shows URL input), Highlight (yellow bg)
- Text color picker (6-8 preset colors)
- Subscript, Superscript
- Clear formatting
- AI: "Rewrite", "Polish", "Simplify" (Phase 5)

**Files:**
- Create: `src/components/editor/tiptap/bubble-toolbar.tsx`
- Modify: `src/components/editor/tiptap/extensions.ts`
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx`

### Task 1.2: Fixed top toolbar (always visible, block-level controls)

A toolbar bar above the editor (not floating) with:
- Paragraph / H1 / H2 / H3 dropdown
- Bold / Italic / Underline / Strike / Code toggle buttons
- List (bullet / ordered / task) toggles
- Link button
- Text align (left / center / right) — install `@tiptap/extension-text-align`
- Undo / Redo buttons
- "+" insert menu (same items as slash menu, triggered as a dropdown)

**Files:**
- Create: `src/components/editor/tiptap/editor-toolbar.tsx`
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx`

---

## Phase 2: Block Manipulation (drag, move, delete)

### Task 2.1: Drag handle + block actions menu

Each block gets a left-margin grip handle (6-dot icon) that appears on hover. Clicking shows a block actions menu:
- Delete block
- Duplicate block
- Move up / Move down
- Turn into... (convert block type — paragraph → heading, etc.)
- Copy block
- Comment (stub for future)

**Implementation:** Custom Tiptap `NodeView` wrapper or a global decoration that attaches handles. Use `@tiptap/extension-dropcursor` (installed) for visual feedback during drag.

**Extensions to install:** None new — dropcursor is already installed.

**Files:**
- Create: `src/components/editor/tiptap/block-handle.tsx`
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx`

### Task 2.2: Plus button on left margin

A "+" button appears on the left margin of empty paragraphs (or between blocks on hover). Clicking opens the slash menu at that position. Same items as the `/` slash menu.

**Files:**
- Create: `src/components/editor/tiptap/margin-plus-button.tsx`

### Task 2.3: Keyboard shortcuts for block operations

- `Cmd+Shift+ArrowUp` — move block up
- `Cmd+Shift+ArrowDown` — move block down
- `Cmd+Shift+D` — duplicate block
- `Cmd+Shift+Delete` — delete block
- `Cmd+Enter` — save document
- `Tab` / `Shift+Tab` — indent / outdent in lists

**Files:**
- Create: `src/components/editor/tiptap/keyboard-shortcuts.ts` (Tiptap extension)

---

## Phase 3: Expanded Block Types

### Task 3.1: Table support

**Install:** `@tiptap/extension-table`, `@tiptap/extension-table-row`, `@tiptap/extension-table-cell`, `@tiptap/extension-table-header`.

Slash menu entry: `/table` — inserts a 3x3 table. Table toolbar (appears when cursor is in a table): add row/column, delete row/column, merge cells, toggle header row.

### Task 3.2: Task list (checklist)

**Install:** `@tiptap/extension-task-list`, `@tiptap/extension-task-item`.

Slash menu entry: `/todo` or `/checklist`. Renders as checkboxes.

### Task 3.3: Callout / info box

Custom Tiptap node: `callout` with `attrs: { id, type: "info"|"warning"|"tip"|"danger" }`. Rich text content inside. Colored left border + icon by type.

Slash menu entry: `/callout`.

### Task 3.4: Toggle / collapsible

Custom Tiptap node: `toggle` with `attrs: { id, summary }`. Expandable/collapsible content. Summary is always visible; content shows on click.

Slash menu entry: `/toggle`.

### Task 3.5: Image with upload

Enhance the existing `media-embed` to support drag-and-drop file upload + paste from clipboard. Upload to a server endpoint or S3. Currently media-embed only accepts URLs.

### Task 3.6: Bookmark / link preview

Custom Tiptap node: `bookmark` with `attrs: { id, url, title, description, image }`. Fetches Open Graph metadata from the URL and renders a rich preview card.

Slash menu entry: `/bookmark`.

### Task 3.7: Table of contents

Custom Tiptap node: `toc`. Auto-generates a linked list of headings in the document. Updates on heading changes.

Slash menu entry: `/toc`.

### Task 3.8: Columns layout

Custom Tiptap node: `columns` with `attrs: { id, count: 2|3 }`. Each column is a container with editable content. For side-by-side layouts.

Slash menu entry: `/columns`.

---

## Phase 4: UX Polish

### Task 4.1: Placeholder text

**Install:** `@tiptap/extension-placeholder` (not installed).

Show "Type / for commands, or start writing..." in empty documents. Per-block placeholders for custom nodes (e.g., "Enter teacher note..." inside teacher-note blocks).

### Task 4.2: Character count

**Install:** `@tiptap/extension-character-count`.

Show word count + character count in the editor footer. Useful for estimated reading time.

### Task 4.3: Typography

**Install:** `@tiptap/extension-typography`.

Smart quotes ("" instead of ""), em-dashes (—), ellipses (…), and other typographic niceties.

### Task 4.4: Keyboard shortcuts reference

A "?" button in the toolbar that opens a modal listing all keyboard shortcuts (Cmd+B, Cmd+I, Cmd+K, /, etc.).

### Task 4.5: Right-click context menu

Browser right-click on a block shows: Cut, Copy, Paste, Delete Block, Duplicate, Turn Into..., Add Comment (stub).

---

## Phase 5: AI Selection Tools

### Task 5.1: Selection-based AI commands

When text is selected and the bubble toolbar appears, add AI action buttons:
- **Rewrite** — rephrase the selected text (same meaning, different words)
- **Polish** — fix grammar, improve clarity
- **Simplify** — reduce reading level (for K-5 content)
- **Expand** — elaborate on the selected text
- **Summarize** — condense the selected text
- **Translate** — translate to another language (stub)

Each calls `POST /api/units/{id}/ai-transform` with `{ action, selectedText, context }` where context is the surrounding 500 chars.

### Task 5.2: Inline AI prompt at cursor (upgrade current implementation)

Current: the `/ai` slash command shows a prompt bar above the editor.

Upgrade: show the prompt input **inline at the cursor position** as a ProseMirror decoration. The generated content replaces the prompt decoration. Same flow as the Notion AI experience — type intent inline, content appears in-place.

### Task 5.3: AI-aware context

Include the full document content (or a summarized version) in the AI prompt so generated content is contextually relevant to the existing unit. Currently the AI has zero document context.

**Files:**
- Modify: `platform/internal/handlers/unit_ai.go` — add `/ai-transform` endpoint
- Modify: `src/components/editor/tiptap/bubble-toolbar.tsx` — add AI buttons
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx`

---

## Phase 6: Slash Menu Expansion

Update the slash menu with all new block types:

**AI:** AI Writer (existing)
**Text:** Heading 1-3, Paragraph, Bullet List, Numbered List, Task List, Blockquote, Code Block, Divider, Table, Columns, Toggle, Callout, Table of Contents, Bookmark
**Teaching:** Problem, Teacher Note, Code Snippet, Media, Solution, Live Cue, Assignment

Add icons (small SVGs or emoji fallback) instead of the current 2-letter badges.

---

## Implementation Order

| Phase | Effort | Impact | Priority |
|-------|--------|--------|----------|
| 1 (Toolbar + formatting) | Medium | Very high — most visible gap | P0 |
| 2 (Block manipulation) | Medium | High — core editing UX | P0 |
| 3 (Block types) | Large | Medium — feature richness | P1 |
| 4 (UX polish) | Small | Medium — feels professional | P1 |
| 5 (AI selection) | Medium | High — unique differentiator | P1 |
| 6 (Slash menu expansion) | Small | Low — follows from Phase 3 | P2 |

Phases 1-2 should ship first (P0). Phases 3-5 can be parallelized (P1). Phase 6 is a consequence of Phase 3.

---

## Code Review

Reviewers append findings here.

## Post-Execution Report

Populate after implementation.
