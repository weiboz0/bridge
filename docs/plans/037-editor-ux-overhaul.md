# 037 — Teaching Unit Editor: Notion-Like UX Overhaul

**Goal:** Close the UX gap between our Tiptap editor and a Notion-like experience. Build a production-grade block editor that exceeds Notion for teaching-specific use cases. Not an MVP — full feature set.

**Baseline references:**
- https://template.tiptap.dev/notion-like/Wb8BhbtRbR (minimum feature set)
- https://www.blocknotejs.org/ (block manipulation UX patterns — borrow UX, don't switch codebase)

**Branch:** `feat/037-editor-ux`

**Depends On:** Plans 031-036 (teaching unit editor exists)

**Status:** Not started

---

## Architecture Decisions (from Codex review)

1. **Stay on Tiptap** — 8 custom teaching nodes + Yjs wiring make BlockNote migration too costly. Borrow BlockNote's UX patterns, implement in custom Tiptap React components.
2. **No paid Tiptap extensions** — drag context menu and AI extension require Tiptap Pro. Build custom equivalents using free extensions + custom React UI.
3. **Build custom AI layer** — don't depend on `@tiptap/extension-ai` (paid). Use our existing Go LLM backend with a custom selection-context API.
4. **Foundation before features** — fix collaboration edge cases and node ID stability before adding surface UX. Phase 0 handles this.

---

## Phase 0: Foundation (stability before features)

### Task 0.1: Collaboration stability audit

Verify that block operations (insert, delete, duplicate, reorder) produce consistent state under Yjs collaboration. Fix any node ID collisions or undo/redo issues. Add a block version marker in the doc JSON envelope for forward compat.

### Task 0.2: Block validation on load

Add client-side validation when loading a saved document: strip unknown block types gracefully (render as paragraph fallback), warn on missing `attrs.id`, repair broken content structure. Prevents corrupted documents from crashing the editor.

---

## Phase 1: Toolbar + Inline Formatting

### Task 1.1: Bubble menu (floating toolbar on text selection)

**Extensions to activate:** `@tiptap/extension-bubble-menu` (installed), `@tiptap/extension-underline` (installed), `@tiptap/extension-link` (installed).

**Extensions to install:** `@tiptap/extension-highlight`, `@tiptap/extension-text-style`, `@tiptap/extension-color`, `@tiptap/extension-subscript`, `@tiptap/extension-superscript`.

**Toolbar buttons (appear on text selection):**
- Bold (Cmd+B), Italic (Cmd+I), Strikethrough (Cmd+Shift+X), Underline (Cmd+U)
- Code (inline), Link (Cmd+K — shows URL input), Highlight (yellow bg)
- Text color picker (6-8 preset colors)
- Subscript, Superscript
- Clear formatting
- AI actions: Rewrite, Polish, Simplify, Expand, Summarize (Phase 5 implements backend, but reserve the button slots now)

**Accessibility:** Every button has ARIA label + tooltip showing the keyboard shortcut. Focus-visible ring on all interactive elements.

**Files:**
- Create: `src/components/editor/tiptap/bubble-toolbar.tsx`
- Modify: `src/components/editor/tiptap/extensions.ts`
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx`

### Task 1.2: Fixed top toolbar (always visible)

A toolbar bar above the editor with:
- Block type dropdown: Paragraph / H1 / H2 / H3
- Inline formatting toggles: Bold / Italic / Underline / Strike / Code / Link / Highlight / Color
- List toggles: Bullet / Ordered / Task list
- Text align: Left / Center / Right — install `@tiptap/extension-text-align`
- Insert menu: "+" dropdown with same items as slash menu
- Undo / Redo buttons
- Save button (existing, moved here)
- Collaborative status indicator (existing, moved here)

Each button shows active state when the current selection has that formatting applied.

**Files:**
- Create: `src/components/editor/tiptap/editor-toolbar.tsx`
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx`

---

## Phase 2: Block Manipulation

### Task 2.1: Block hover controls — drag handle + actions menu

On hover over any block, show a left-margin control strip:
- **Drag handle** (6-dot grip icon) — drag to reorder blocks. Use `@tiptap/extension-dropcursor` (installed) for visual drop feedback.
- **Actions menu** (click the handle or right-click) — dropdown with:
  - Delete block
  - Duplicate block
  - Move up / Move down
  - Turn into... (convert block type — paragraph ↔ heading ↔ blockquote ↔ list item)
  - Copy block
  - Add comment (stub for future)

**Implementation:** Custom React overlay that tracks the hovered block via `editor.view.domAtPos()`. Not a Tiptap NodeView wrapper (too invasive for existing nodes). Instead, a floating element positioned via the block's DOM rect.

**Files:**
- Create: `src/components/editor/tiptap/block-handle.tsx`
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx`

### Task 2.2: Plus button on left margin

A "+" button on the left margin that appears:
- On empty paragraphs (always visible)
- Between blocks on hover (appears in the gap)

Clicking opens the slash menu at that position.

**Files:**
- Create: `src/components/editor/tiptap/margin-plus-button.tsx`

### Task 2.3: Keyboard shortcuts for block operations

| Shortcut | Action |
|----------|--------|
| `Cmd+Shift+ArrowUp` | Move block up |
| `Cmd+Shift+ArrowDown` | Move block down |
| `Cmd+Shift+D` | Duplicate block |
| `Cmd+Shift+Delete` | Delete block |
| `Cmd+Enter` | Save document |
| `Cmd+K` | Insert/edit link |
| `Tab` / `Shift+Tab` | Indent / outdent in lists |
| `Cmd+Z` / `Cmd+Shift+Z` | Undo / Redo |

**Files:**
- Create: `src/components/editor/tiptap/keyboard-shortcuts.ts` (Tiptap extension)

---

## Phase 3: Expanded Block Types

### Task 3.1: Table

**Install:** `@tiptap/extension-table`, `@tiptap/extension-table-row`, `@tiptap/extension-table-cell`, `@tiptap/extension-table-header`.

Slash menu: `/table` — inserts 3x3. Table-specific toolbar (appears when cursor is in table): add/delete row/column, merge cells, toggle header row.

### Task 3.2: Task list (checklist)

**Install:** `@tiptap/extension-task-list`, `@tiptap/extension-task-item`.

Slash menu: `/todo` or `/checklist`. Renders as interactive checkboxes.

### Task 3.3: Callout / info box

Custom Tiptap node: `callout` with `attrs: { id, variant: "info"|"warning"|"tip"|"danger" }`. Rich text content. Colored left border + icon. Slash menu: `/callout`.

### Task 3.4: Toggle / collapsible

Custom Tiptap node: `toggle` with `attrs: { id, summary }`. Summary always visible; content expands on click. Slash menu: `/toggle`.

### Task 3.5: Image with upload

Extend `media-embed` to support:
- Drag-and-drop file upload
- Paste from clipboard
- Upload to server endpoint (need `POST /api/uploads` Go handler + file storage — S3 or local disk)
- Progress indicator
- Image resize handles

### Task 3.6: Bookmark / link preview

Custom Tiptap node: `bookmark` with `attrs: { id, url, title, description, image }`. Server-side Open Graph metadata fetch via `GET /api/unfurl?url=...`. Renders as a rich preview card. Slash menu: `/bookmark`.

### Task 3.7: Table of contents

Custom Tiptap node: `toc`. Auto-generates a linked list of headings. Updates reactively on heading changes. Slash menu: `/toc`.

### Task 3.8: Columns layout

Custom Tiptap node: `columns` with `attrs: { id, count: 2|3|4 }`. Each column is an editable container. Slash menu: `/columns`.

### Task 3.9: Math / KaTeX

Custom Tiptap node: `math-block` (block-level) and `math-inline` (inline). Renders via KaTeX. Input: LaTeX syntax. Slash menu: `/math`. Inline: `$...$` wrapper. Important for STEM teaching content.

### Task 3.10: Emoji picker

Inline emoji insertion. Triggered by `:` followed by a search term (`:smile:` etc.). Use a lightweight emoji dataset.

---

## Phase 4: UX Polish

### Task 4.1: Placeholder text

**Install:** `@tiptap/extension-placeholder`.

- Empty document: "Type / for commands, or start writing..."
- Empty teacher-note: "Write a note for teachers..."
- Empty live-cue: "Write an instruction cue..."
- Empty code-snippet: "Paste or type code here..."

### Task 4.2: Character / word count

**Install:** `@tiptap/extension-character-count`.

Footer bar: word count, character count, estimated reading time. Useful for pacing lesson content.

### Task 4.3: Typography

**Install:** `@tiptap/extension-typography`.

Smart quotes, em-dashes, ellipses, fraction characters.

### Task 4.4: Keyboard shortcuts reference

A "?" button in the toolbar opens a modal listing all keyboard shortcuts, grouped by category (formatting, block operations, navigation).

### Task 4.5: Right-click context menu

Custom context menu on block right-click: Cut, Copy, Paste, Duplicate, Delete, Turn Into..., Move Up/Down.

### Task 4.6: Teacher-facing help overlay

First-time UX: a brief overlay or tooltip tour highlighting the slash menu, toolbar, drag handles, and AI features. Dismissable, remembers dismissal in localStorage.

### Task 4.7: Minimal / presentation mode

A toggle that hides the toolbar, block handles, and all editing chrome — showing just the content in a clean read-like view. For teachers presenting to a class from the editor page.

---

## Phase 5: AI Selection Tools

### Task 5.1: Selection-based AI transform endpoint

**New endpoint:** `POST /api/units/{id}/ai-transform`

```json
{
  "action": "rewrite|polish|simplify|expand|summarize",
  "selectedText": "the selected content",
  "context": "surrounding 500 chars before + after selection",
  "documentSummary": "first 2000 chars of the full document"
}
```

Returns: `{ "result": "transformed text" }`.

The handler uses the existing LLM backend with action-specific system prompts. The full document context makes the transform aware of the unit's topic and tone.

### Task 5.2: AI buttons in bubble toolbar

When text is selected, the bubble toolbar includes AI action buttons:
- **Rewrite** — rephrase (same meaning, different words)
- **Polish** — fix grammar, improve clarity, maintain tone
- **Simplify** — reduce reading level (K-5 appropriate)
- **Expand** — elaborate with more detail
- **Summarize** — condense to key points
- **Translate** — translate to another language (dropdown: Spanish, Chinese, French, etc.)

On click: capture selection + context, call `/ai-transform`, replace selection with result. Show a "before/after" diff preview before committing the change.

### Task 5.3: Inline AI prompt at cursor (upgrade)

Current: `/ai` shows a prompt bar above the editor.

Upgrade: show the prompt as a ProseMirror decoration **at the cursor position** inside the document flow. Generated content appears in-place, replacing the decoration. The experience matches Notion's inline AI — type intent where you want the content to appear.

### Task 5.4: Document-aware AI context

Upgrade the existing `POST /api/units/{id}/draft-with-ai` to include:
- The unit's current document content (summarized to first 3000 chars)
- The unit's metadata (title, grade level, subject tags)
- The unit's lineage (if forked, what the parent's topic is)

This makes AI-generated content contextually relevant to the existing unit.

---

## Phase 6: Slash Menu Expansion

Update the slash menu with all new block types from Phase 3. Replace 2-letter text badges with small SVG icons (or Unicode symbols). Group into categories:

**AI:** AI Writer
**Basic blocks:** Paragraph, Heading 1-3, Bullet List, Numbered List, Task List, Blockquote, Code Block, Divider, Table, Columns
**Media:** Image, Bookmark, Embed
**Advanced:** Toggle, Callout, Table of Contents, Math, Emoji
**Teaching:** Problem, Teacher Note, Code Snippet, Solution, Live Cue, Assignment

---

## Implementation Order

| Phase | What | Effort | Notes |
|-------|------|--------|-------|
| 0 | Foundation | Small | Must ship first — stability |
| 1 | Toolbar + formatting | Medium | Most visible gap, highest teacher impact |
| 2 | Block manipulation | Medium | Core editing UX |
| 3 | Block types | Large | Can be incremental (ship table first, then others) |
| 4 | UX polish | Medium | Professional feel |
| 5 | AI selection | Medium | Unique differentiator |
| 6 | Slash menu expansion | Small | Follows from Phase 3 |

All phases ship incrementally — each task is independently mergeable.

---

## Codex Review Notes

- **No paid Tiptap extensions.** Drag context menu and `@tiptap/extension-ai` require Tiptap Pro. Build custom equivalents.
- **Don't switch to BlockNote.** Migration cost for 8 teaching nodes + Yjs too high. Borrow UX patterns only.
- **Media upload needs server infra** — `POST /api/uploads` + S3/local storage. Don't treat as a quick add.
- **Accessibility throughout** — ARIA labels, focus-visible, keyboard nav, contrast. Not a separate phase — baked into every task.
- **Block version marker** in doc JSON for forward compat when adding new block types.

---

## Code Review

Reviewers append findings here.

## Post-Execution Report

Populate after implementation.
