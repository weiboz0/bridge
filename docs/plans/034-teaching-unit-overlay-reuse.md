# 034 — Teaching Unit: Overlay Reuse

**Goal:** Implement the `unit_overlays` table, the overlay composition algorithm, fork action, pin/float toggle, and breadcrumb lineage display so teachers can inherit from platform or peer units while overriding specific blocks.

**Spec:** `docs/specs/012-teaching-units.md` — §Overlay semantics, §unit_overlays data model, §Block document schema (attrs.parentId)

**Branch:** `feat/034-overlay-reuse`

**Depends On:** Plan 033a (lifecycle + revisions for pinning), Plan 033b (projection — runs after overlay merge)

**Status:** In progress

---

## Scope

**In scope:**
- Migration 0018: `unit_overlays` table
- Overlay composition algorithm (pure Go function — merges parent + child + overrides)
- Store: fork unit (creates child + overlay row), get/update overlay, get composed document
- Handler: `POST /api/units/{id}/fork`, `GET /api/units/{id}/composed`, `PATCH /api/units/{id}/overlay`
- Frontend: fork button on unit view, lineage breadcrumb, pin/float toggle
- Composed document feeds into the existing projection pipeline

**Out of scope:**
- Block-level override editing UI (hide/replace individual parent blocks) — follow-up; teachers can edit `block_overrides` via the API for now
- Multi-parent inheritance
- `unit_collections` (plan 036)

---

## Task 1: Migration — unit_overlays table

**Files:**
- Create: `drizzle/0018_unit_overlays.sql`
- Modify: `src/lib/db/schema.ts` — add `unitOverlays` table

```sql
BEGIN;

CREATE TABLE IF NOT EXISTS unit_overlays (
  child_unit_id      uuid PRIMARY KEY REFERENCES teaching_units(id) ON DELETE CASCADE,
  parent_unit_id     uuid NOT NULL REFERENCES teaching_units(id) ON DELETE CASCADE,
  parent_revision_id uuid REFERENCES unit_revisions(id) ON DELETE SET NULL,
  block_overrides    jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS unit_overlays_parent_idx ON unit_overlays(parent_unit_id);

COMMIT;
```

**Commit:** `feat(034): migration 0018 — unit_overlays table`

---

## Task 2: Overlay composition algorithm

**Files:**
- Create: `platform/internal/overlay/compose.go`
- Create: `platform/internal/overlay/compose_test.go`

Pure function, no DB access:

```go
package overlay

type BlockOverride struct {
    Action string          `json:"action"` // "hide" or "replace"
    Block  json.RawMessage `json:"block"`  // only for "replace"
}

// ComposeDocument merges parent blocks with child blocks and overrides.
// Returns the composed block list ready for projection.
func ComposeDocument(
    parentBlocks []json.RawMessage,
    childBlocks  []json.RawMessage,
    overrides    map[string]BlockOverride,
) []json.RawMessage
```

Algorithm per spec 012 §Overlay semantics:
1. Walk parent blocks in order
2. For each parent block P: check `overrides[P.attrs.id]`
   - `hide` → skip
   - `replace` → emit the override block instead
   - no override → emit P
3. After emitting P (or its replacement), emit any child blocks where `attrs.parentId == P.attrs.id`
4. After all parent blocks, emit child blocks where `attrs.parentId == null` (appended to end)
5. Orphaned child blocks (parentId references a deleted parent block) fall through to the end

**Tests:**
- No overrides, no child blocks → output == parent blocks
- Hide one block → that block omitted
- Replace one block → replacement emitted
- Child block anchored after parent block → correct position
- Child block with parentId=null → appended at end
- Orphaned child block (parentId doesn't exist in parent) → falls through to end
- Empty parent + child blocks → empty output
- Mixed: hide + replace + anchored children + null-anchored children

**Commit:** `feat(034): overlay composition — merge parent + child + overrides`

---

## Task 3: Store + handler — fork, overlay CRUD, composed document

**Files:**
- Modify: `platform/internal/store/teaching_units.go` — add overlay methods
- Modify: `platform/internal/store/teaching_units_test.go`
- Modify: `platform/internal/handlers/teaching_units.go` — add endpoints
- Modify: `platform/internal/handlers/teaching_units_integration_test.go`

**Store methods:**

1. `ForkUnit(ctx, sourceID string, target ForkTarget) (*TeachingUnit, error)` — creates a new unit + overlay row + empty child document in one transaction. Target has Scope, ScopeID, Title, CallerID.

2. `GetOverlay(ctx, childUnitID string) (*UnitOverlay, error)` — returns the overlay row, nil if unit has no parent.

3. `UpdateOverlay(ctx, childUnitID string, input UpdateOverlayInput) (*UnitOverlay, error)` — update parent_revision_id (pin/float) and/or block_overrides.

4. `GetComposedDocument(ctx, unitID string) (*ComposedDocument, error)` — if unit has an overlay, load parent revision blocks + child blocks + overrides, call `overlay.ComposeDocument`, return result. If no overlay, return the unit's own document blocks directly.

5. `GetLineage(ctx, unitID string) ([]LineageEntry, error)` — walk the overlay chain upward (child → parent → grandparent) and return the lineage as a breadcrumb list.

**Handler endpoints:**

```
POST   /api/units/{id}/fork           body: { scope, scopeId?, title? }
GET    /api/units/{id}/overlay        returns overlay row or 404
PATCH  /api/units/{id}/overlay        body: { parentRevisionId?, blockOverrides? }
GET    /api/units/{id}/composed       returns composed document (overlay-merged + ready for projection)
GET    /api/units/{id}/lineage        returns breadcrumb chain
```

Auth: fork requires view access to source + create access in target scope. Overlay CRUD requires edit access to the child unit.

**Tests:**
- Fork creates child + overlay + empty document
- Fork with custom title
- Forked unit's composed doc == parent's blocks (no overrides yet)
- Add override (hide) → composed doc omits that block
- Add override (replace) → composed doc has replacement
- Pin to specific revision → composed doc uses pinned blocks, not latest
- Float (null revision) → composed doc follows parent's latest published revision
- Lineage: child → parent → grandparent chain
- Non-overlaid unit → composed doc == own document

**Commit:** `feat(034): fork + overlay CRUD + composed document`

---

## Task 4: Frontend — fork button, lineage, pin/float

**Files:**
- Modify: `src/app/(portal)/teacher/units/[id]/page.tsx` — add fork button
- Modify: `src/app/(portal)/teacher/units/[id]/edit/page.tsx` — show lineage breadcrumb + pin/float toggle
- Modify: `src/lib/teaching-units.ts` — add fork/overlay/lineage helpers

1. **Fork button:** On the unit view page, "Fork to My Org" / "Fork to Personal" button. Calls `POST /api/units/{id}/fork`. On success, redirects to the new unit's edit page.

2. **Lineage breadcrumb:** In the edit page header, show "Platform > Org Name > Your Adaptation" chain from `GET /api/units/{id}/lineage`. Each ancestor is a link to its view page.

3. **Pin/float toggle:** If the unit has an overlay, show a toggle: "Following latest" (floating) vs "Pinned to revision X". Switching calls `PATCH /api/units/{id}/overlay`.

**Commit:** `feat(034): fork UI + lineage breadcrumb + pin/float toggle`

---

## Task 5: Verify + docs + code review

**Verification:**
```bash
cd platform && go test ./... -count=1 -timeout 180s
node_modules/.bin/vitest run
node_modules/.bin/tsc --noEmit
```

Update `docs/api.md`. Write post-execution report. Run Codex code review.

**Commit:** `docs(034): API reference + post-execution report`

---

## Code Review

Reviewers append findings here following `docs/code-review.md`.

## Post-Execution Report

Populate after implementation.
