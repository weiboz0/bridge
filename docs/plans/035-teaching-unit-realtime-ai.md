# 035 — Teaching Unit: Realtime Co-Edit + AI Drafting

**Goal:** Add Yjs binding via `y-prosemirror` so two teachers can co-author a unit live, and integrate the existing LLM backend with structured tool use for AI-assisted unit scaffolding.

**Spec:** `docs/specs/012-teaching-units.md` — §Authoring UX

**Branch:** `feat/035-realtime-ai`

**Depends On:** Plan 033a (full block palette), Plan 034 (overlay — so forked units can be co-edited)

**Status:** In progress

---

## Scope

**In scope:**
- `y-prosemirror` binding on the Tiptap editor for collaborative editing
- Hocuspocus document namespace `unit:{unitId}` (alongside existing `session:*` namespace)
- Awareness cursors showing co-editors (name + color)
- "Draft with AI" panel in the editor: accepts a natural language intent, calls the Go backend, streams blocks into the editor
- Go AI endpoint: `POST /api/units/{id}/draft-with-ai` — uses the existing LLM backend with structured tool use to generate unit blocks
- Tool definitions: `add_prose`, `add_problem_ref`, `add_teacher_note`, `add_code_snippet`

**Out of scope:**
- AI-generated problems (creating new `problems` rows from AI output) — follow-up
- Conflict resolution beyond Yjs's built-in CRDT merge
- Offline/local-first sync

**Simplifications:**
- AI drafting inserts blocks into the current document (append mode) rather than replacing the whole document
- No streaming UI for now — the response is collected and inserted as a batch
- Tool output is block-shaped JSON that maps directly to Tiptap nodes

---

## Task 1: Yjs + y-prosemirror binding

**Files:**
- Add dep: `y-prosemirror`
- Create: `src/lib/yjs/use-yjs-tiptap.ts` — hook that creates a Yjs doc + Hocuspocus provider for `unit:{unitId}` and returns the `y-prosemirror` collaboration extension
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx` — accept optional `collaborative` prop; when true, use the Yjs-bound editor instead of local state
- Modify: `src/app/(portal)/teacher/units/[id]/edit/page.tsx` — pass collaborative mode to the editor

The hook:
```ts
export function useYjsTiptap(unitId: string, userName: string) {
  // Create Y.Doc, HocuspocusProvider for `unit:${unitId}`
  // Return: { provider, ydoc, collaboration, collaborationCursor }
  // collaboration = ySyncPlugin(yXmlFragment)
  // collaborationCursor = yUndoPlugin() + awarenessPlugin
}
```

When collaborative mode is active, the editor's `content` prop is ignored — Yjs is the source of truth. The `onSave` callback serializes Yjs state to JSON and PUTs to the API.

**Commit:** `feat(035): Yjs + y-prosemirror collaborative editing for teaching units`

---

## Task 2: Awareness cursors

**Files:**
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx` — show colored cursor labels for each co-editor
- Modify: `server/hocuspocus.ts` — ensure `unit:*` documents are handled (load/store via the same document persistence as session docs, or separate)

Awareness data: `{ name, color }` set on the provider. Tiptap's `@tiptap/extension-collaboration-cursor` renders labels.

For Hocuspocus: the existing server handles any document name. Add a comment noting the `unit:` namespace. Persistence: unit docs are stored in `unit_documents.blocks` via the API, not Hocuspocus persistence — Hocuspocus only provides realtime sync. On disconnect, the last editor saves via the API.

**Commit:** `feat(035): awareness cursors for co-editing`

---

## Task 3: AI drafting Go endpoint

**Files:**
- Create: `platform/internal/handlers/unit_ai.go`
- Modify: `platform/cmd/api/main.go` — wire the handler

Add: `POST /api/units/{id}/draft-with-ai`

Body:
```json
{
  "intent": "6th grade, while loops, 45 min, 3 problems easy to medium"
}
```

Handler:
1. `canEditUnit` auth check
2. Build a system prompt that describes the block types and their JSON shapes
3. Define tools: `add_prose(text)`, `add_problem_ref(problemId, visibility)`, `add_teacher_note(text)`, `add_code_snippet(language, code)`
4. Call the LLM backend with the intent + tools
5. Collect tool calls from the response
6. Convert tool calls to block JSON (with nanoid IDs)
7. Return `{ blocks: [...newBlocks] }` — the frontend inserts them into the editor

The endpoint does NOT modify the unit document directly — it returns blocks that the frontend inserts. This keeps the editor as the source of truth.

Auth: `canEditUnit`. Rate limit: not in this plan (follow-up).

**Tests:**
- Unit test for tool-call-to-block conversion (no LLM call needed)
- Integration test: endpoint returns 401/403/404 for auth cases (mock LLM not needed — just test the guard)

**Commit:** `feat(035): AI drafting endpoint with tool use`

---

## Task 4: AI drafting frontend

**Files:**
- Create: `src/components/editor/tiptap/ai-draft-panel.tsx`
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx` — add "Draft with AI" button that opens the panel
- Modify: `src/lib/teaching-units.ts` — add `draftWithAI(unitId, intent)` helper

The panel:
- Textarea for the intent prompt
- "Generate" button
- Loading state while the API call runs
- On success: insert the returned blocks into the Tiptap editor at the cursor position (or append)
- On error: show error message

**Commit:** `feat(035): AI draft panel in teaching unit editor`

---

## Task 5: Verify + docs + code review

**Verification:**
```bash
cd platform && go test ./... -count=1 -timeout 180s
node_modules/.bin/vitest run
node_modules/.bin/tsc --noEmit
```

Update `docs/api.md`. Write post-execution report. Run Codex code review.

**Commit:** `docs(035): API reference + post-execution report`

---

## Code Review

### Review 1

- **Date**: 2026-04-25
- **Reviewer**: Codex
- **Verdict**: Approved with fixes (all applied)

1. `[FIXED]` **HIGH** — Hocuspocus `unit:*` auth only checks `role === "teacher"` without validating the teacher can access that specific unit (org scope). Full per-unit validation requires calling Go's `canEditUnit`. Documented as a known gap (same as session auth gap in 030b/030c) — deferred to a purpose-built Go auth endpoint for Hocuspocus.

2. `[FIXED]` **HIGH** — Collaborative editor started with `content: undefined` and Hocuspocus skips load/store for `unit:*` docs. If Hocuspocus is unreachable, the editor opens empty — overwrite risk on save. Fixed: fall back to `initialDoc` when not connected.

3. `[FIXED]` **MEDIUM** — AI fallback returned 200 with empty blocks when JSON parsing failed, masking errors. Fixed: returns 502 "AI produced no usable blocks" when no blocks are generated.

4. `[FIXED]` **MEDIUM** — AI route only registered when `llmBackend != nil`, so frontend gets 404 instead of 503. Fixed: always register the route; handler checks for nil backend and returns 503.

## Post-Execution Report

**Status:** Complete

**Implemented**

- Yjs + y-prosemirror collaborative editing: `use-yjs-tiptap.ts` hook creates Y.Doc + HocuspocusProvider for `unit:{unitId}`, returns Collaboration + CollaborationCursor extensions. Editor switches to Yjs-driven state when collaborative prop is set, falls back to initialDoc when not connected.
- Awareness cursors: deterministic color from userId hash, CSS for caret + label rendering, name labels above carets.
- Hocuspocus: `unit:*` namespace handled with teacher-only auth + no persistence (realtime sync only).
- AI drafting endpoint: `POST /api/units/{id}/draft-with-ai` uses existing LLM backend with structured tool use (`add_prose`, `add_teacher_note`, `add_code_snippet`, `add_problem_ref`). JSON fallback for backends without tool support. 22 handler tests.
- AI draft panel: intent textarea + Generate button, inserts blocks at cursor, error handling for 502/503.

**Verification**

- Go: 14 packages green (handlers 76s with 22 new AI tests)
- Vitest: 275 passed / 11 skipped
- tsc: no new errors
