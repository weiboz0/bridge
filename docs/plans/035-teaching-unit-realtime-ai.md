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

Reviewers append findings here following `docs/code-review.md`.

## Post-Execution Report

Populate after implementation.
