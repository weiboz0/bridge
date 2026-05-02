# Plan 062 — `GetProjectedDocument` returns composed overlay-merged content (P1)

## Status

- **Date:** 2026-05-01
- **Severity:** P1 (forked units render empty or stale to students)
- **Origin:** `docs/reviews/009-deep-codebase-review-2026-04-30.md` §P1-7.

## Problem

Two sibling endpoints, two different code paths, one is wrong:

- `GetComposedDocument` at `platform/internal/handlers/teaching_units.go:1271-1305` correctly merges parent + child + overlay rows to produce the composed document the editor renders.
- `GetProjectedDocument` at `platform/internal/handlers/teaching_units.go:833-861` calls `GetDocument` directly, which returns ONLY the raw child document.

`/projected` is the endpoint the **student** unit page (`src/app/(portal)/student/units/[id]/page.tsx`) uses. So:

- A teacher forks a unit (creates a child overlay with selective block overrides on a parent).
- The teacher edits the overlay; the parent stays canonical.
- A student opens the forked unit via the student page.
- The page calls `/projected`, which returns just the child's raw blocks. If the child has no blocks (overlay-only), the student sees empty content. If the child has stale blocks from before the fork, the student sees stale content.

Combined with plan 061 (which fixes the access denial for students), this is the second half of the broken Python 101 student render path: even after the access check passes, what they see is wrong for any forked unit.

## Out of scope

- Changing the overlay/composition model itself — `unit_overlays` tables and `GetComposedDocument` are correct.
- Editor-side rendering — the bug is on the API, not the client.
- Performance optimization for the composition step — same query the composed endpoint already runs.

## Approach

Make `GetProjectedDocument` use the same composition path as `GetComposedDocument` for overlay children, then apply role filtering after.

Change shape:

```go
// Before:
doc := h.Units.GetDocument(ctx, unitID)  // raw child only
return projectForRole(doc, claims)

// After:
doc := h.Units.GetComposedDocument(ctx, unitID)  // overlay-merged
return projectForRole(doc, claims)
```

Where `projectForRole` is the existing role-filter logic that hides teacher-only blocks from students. The composition step happens BEFORE the role filter so role-filtered blocks still survive composition correctly.

For non-overlay units (no `unit_overlays` row exists), `GetComposedDocument` falls back to the raw document — so the change is a strict superset of the current behavior.

## Files

- Modify: `platform/internal/handlers/teaching_units.go::GetProjectedDocument` (lines 833-861) — call `GetComposedDocument` instead of `GetDocument`.
- Maybe modify: `platform/internal/store/teaching_units.go` — if the existing `GetComposedDocument` store method isn't directly callable from the handler context, expose it. Verify before assuming.
- Add: `platform/internal/handlers/teaching_units_test.go` cases — forked unit with empty child and a populated parent → `/projected` returns the parent's blocks. Forked unit with selective overlay overrides → `/projected` returns the merged blocks.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Composition adds latency to `/projected` | low | Same query `/composed` already pays. The teacher editor is fine with that latency; students should be too. If profiling shows it's a hot path, cache. |
| Role filtering on composed (rather than raw) blocks reveals teacher-only blocks that exist in the parent but not the child overlay | medium | The role filter runs on the composed doc; teacher-only blocks should already be filtered by `projectForRole`. Add a regression test: parent with a teacher-only block + student-facing child overlay → student call to `/projected` does NOT include the teacher-only block. |
| Empty-child edge case where `GetComposedDocument` returns nil | low | Fall back to the empty-doc shape `{type: "doc", content: []}` so the student page renders an empty unit gracefully (matching how teachers see an empty unit). |

## Phases

### Phase 0: pre-impl Codex review

Confirm the composition function signature, the role-filter flow, and that the composed-then-filtered ordering is the right shape (vs filter-then-compose). Capture verdict.

### Phase 1: implement + tests

Single commit if the composed function is already exposed; otherwise two (expose, then wire).

### Phase 2: post-impl Codex review

## Codex Review of This Plan

### Pass 1 — 2026-05-02: **CONCUR**

Codex confirmed: bug verified at `teaching_units.go:813` (GetDocument
call) vs `:1272` (composed). `GetComposedDocument` already exposed
with signature `(ctx, unitID) (json.RawMessage, error)`.
`projection.ProjectBlocks` operates on a block slice independent of
storage state — compose-then-filter is semantically correct. Non-
overlay fallback in `GetComposedDocument` confirmed (returns raw
doc when no overlay row exists). Existing `unitFixture` supports
forks; new tests go alongside existing projected tests around
`teaching_units_integration_test.go:1260`. Teacher editor uses raw
`/document` so it's unaffected — only `/projected` (student-facing)
is in scope.
