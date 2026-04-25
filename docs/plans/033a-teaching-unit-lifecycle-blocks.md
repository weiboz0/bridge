# 033a — Teaching Unit: Lifecycle + Revisions + Block Palette

**Goal:** Add status transitions with `unit_revisions` snapshots on publish, and expand the block palette with `teacher-note`, `code-snippet`, and `media-embed` — the three block types that don't require render projection to be useful.

**Spec:** `docs/specs/012-teaching-units.md` — §Lifecycle, §Block types

**Branch:** `feat/033a-lifecycle-blocks`

**Depends On:** Plan 031 (schema), Plan 032 (real content)

**Unblocks:** Plan 033b (render projection), Plan 034 (overlay reuse — needs revisions for pinning)

**Status:** In progress

---

## Scope

**In scope:**
- Status transition endpoints with validation
- `unit_revisions` snapshot on `classroom_ready` / `coach_ready` transitions
- Revision list endpoint
- Three new block types: `teacher-note`, `code-snippet`, `media-embed`
- Server-side allowlist expansion
- Custom Tiptap nodes for the new blocks
- Slash-command insertion in the editor
- Status badge + publish button in the editor UI

**Out of scope (plan 033b):**
- `solution-ref`, `test-case-ref`, `live-cue`, `assignment-variant` blocks
- Render projection pipeline
- Student-facing unit view
- Preview toggle

---

## Task 1: Go store — lifecycle transitions + revisions

**Files:**
- Modify: `platform/internal/store/teaching_units.go`
- Modify: `platform/internal/store/teaching_units_test.go`

Add:

1. **`SetUnitStatus(ctx, unitID, newStatus, callerID string) (*TeachingUnit, error)`**
   - Valid transitions per spec 012:
     - `draft → reviewed`
     - `reviewed → classroom_ready`
     - `reviewed → coach_ready`
     - Any non-draft → `archived`
     - `archived → classroom_ready` (unarchive)
   - Invalid transitions return `ErrInvalidTransition` (reuse from problems.go)
   - On `classroom_ready` or `coach_ready`: also create a `unit_revisions` row (snapshot of current `unit_documents.blocks`)
   - Atomic: use a transaction for the status update + revision insert

2. **`UnitRevision` struct + `ListRevisions(ctx, unitID string) ([]UnitRevision, error)`**
   ```go
   type UnitRevision struct {
       ID        string          `json:"id"`
       UnitID    string          `json:"unitId"`
       Blocks    json.RawMessage `json:"blocks"`
       Reason    *string         `json:"reason"`
       CreatedBy string          `json:"createdBy"`
       CreatedAt time.Time       `json:"createdAt"`
   }
   ```
   Order by `created_at DESC`.

3. **`GetRevision(ctx, revisionID string) (*UnitRevision, error)`**

**Tests:**
- `SetUnitStatus` valid transitions: draft→reviewed→classroom_ready (creates revision), draft→reviewed→coach_ready (creates revision)
- `SetUnitStatus` invalid: draft→archived (rejected), draft→classroom_ready (skips reviewed), classroom_ready→reviewed (backwards)
- `SetUnitStatus` archive from any non-draft: reviewed→archived, classroom_ready→archived
- `SetUnitStatus` unarchive: archived→classroom_ready (creates revision)
- `SetUnitStatus` non-existent unit → `ErrInvalidTransition`
- Revision created: blocks match current unit_documents.blocks at time of transition
- `ListRevisions` returns revisions ordered by created_at DESC
- `ListRevisions` empty for a unit that was never published

**Commit:** `feat(033a): lifecycle transitions + unit_revisions snapshots`

---

## Task 2: Go handler — lifecycle + revision endpoints

**Files:**
- Modify: `platform/internal/handlers/teaching_units.go`
- Modify: `platform/internal/handlers/teaching_units_integration_test.go`

Add routes:
```
POST /api/units/{id}/transition   body: { "status": "reviewed" }
GET  /api/units/{id}/revisions
GET  /api/units/{id}/revisions/{revisionId}
```

The `/transition` endpoint:
- Auth: `canEditUnit` (same as PATCH)
- Validates the target status is a known value
- Calls `SetUnitStatus`
- Maps `ErrInvalidTransition` → 409
- Returns 200 with updated unit

Note: using `/transition` with a body rather than individual `/publish`, `/archive` endpoints — keeps the route surface small since there are 5+ possible target statuses.

**Tests:**
- Transition draft→reviewed → 200
- Transition reviewed→classroom_ready → 200 + revision created (verify via GET /revisions)
- Invalid transition draft→classroom_ready → 409
- Non-editor → 403
- Non-existent unit → 404
- GET /revisions returns list ordered by date
- GET /revisions/{id} returns specific revision with blocks

**Commit:** `feat(033a): unit lifecycle + revision endpoints`

---

## Task 3: Server-side block allowlist expansion

**Files:**
- Modify: `platform/internal/handlers/teaching_units.go`

Expand `knownBlockTypes` to include `teacher-note`, `code-snippet`, `media-embed`.

Add `teacher-note` and `code-snippet` to `blockTypesRequiringID` (they're custom blocks that need stable IDs for overlay semantics in plan 034).

`media-embed` also gets added to `blockTypesRequiringID`.

**Tests:**
- PUT /api/units/{id}/document with a `teacher-note` block → 200
- PUT /api/units/{id}/document with a `code-snippet` block → 200
- PUT /api/units/{id}/document with a `media-embed` block → 200
- PUT /api/units/{id}/document with unknown block type → 400

**Commit:** `feat(033a): expand block allowlist (teacher-note, code-snippet, media-embed)`

---

## Task 4: Tiptap nodes — teacher-note, code-snippet, media-embed

**Files:**
- Create: `src/components/editor/tiptap/teacher-note-node.tsx`
- Create: `src/components/editor/tiptap/code-snippet-node.tsx`
- Create: `src/components/editor/tiptap/media-embed-node.tsx`
- Modify: `src/components/editor/tiptap/extensions.ts`

Each node:
- `group: "block"`, `atom: true` (except teacher-note which has rich text content)
- Carries `attrs.id` (nanoid on creation)
- Custom `NodeView` via `ReactNodeViewRenderer`

**teacher-note:**
- Rich text content (not atom — teachers type inside it)
- Renders with a yellow/amber left border and "Teacher Note" label
- Content is ProseMirror-editable (headings, paragraphs, lists inside)
- `attrs: { id }`

**code-snippet:**
- `attrs: { id, language, code }`
- Renders as a syntax-highlighted code block (use existing `lowlight` if available, otherwise plain `<pre>`)
- Editing: click to open a simple code textarea + language selector
- Not executable — display only

**media-embed:**
- `attrs: { id, url, alt, type }` where type is `image | video | pdf | link`
- Renders: `<img>` for images, `<video>` for video, `<iframe>` for PDF, link card for others
- Editing: click to set/change URL + alt text

Register all three in `extensions.ts`.

**Tests (Vitest):**
- Each node renders without crashing
- Teacher-note content is editable (has content, not atom)

**Commit:** `feat(033a): Tiptap nodes — teacher-note, code-snippet, media-embed`

---

## Task 5: Editor UI — slash commands + status badge

**Files:**
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx`
- Modify: `src/app/(portal)/teacher/units/[id]/edit/page.tsx`

1. **Slash commands:** Add an input rule or suggestion plugin that responds to `/note`, `/code`, `/media`, `/problem` and inserts the corresponding block with a fresh nanoid. Keep it simple — Tiptap's `@tiptap/suggestion` or a basic `textInputRule` that replaces the slash command with the node.

2. **Status badge:** Show the unit's current status as a colored badge in the editor header bar (draft=gray, reviewed=blue, classroom_ready=green, coach_ready=purple, archived=red).

3. **Publish button:** If the user has edit access, show a "Publish" dropdown/button that lets them transition to the next valid status. Calls `POST /api/units/{id}/transition`. Refreshes the badge on success.

**Commit:** `feat(033a): editor slash commands + status badge + publish control`

---

## Task 6: Verify + docs + post-execution

**Files:**
- Modify: `docs/api.md` — document transition + revision endpoints
- Modify: `docs/plans/033a-teaching-unit-lifecycle-blocks.md` — post-execution report

**Verification:**
```bash
cd platform && DATABASE_URL=... go test ./... -count=1 -timeout 180s
DATABASE_URL=... node_modules/.bin/vitest run
node_modules/.bin/tsc --noEmit
```

**Commit:** `docs(033a): API reference + post-execution report`

---

## Code Review

Reviewers append findings here following `docs/code-review.md`.

## Post-Execution Report

Populate after implementation.
