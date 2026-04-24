# 034 — Teaching Unit: Overlay Reuse

**Goal:** Implement the `unit_overlays` table and overlay render pipeline so teachers can inherit from platform or peer units while overriding specific blocks. Add fork/inherit UI in the library and breadcrumb lineage display.

**Spec:** `docs/specs/012-teaching-units.md` — §Overlay semantics, §unit_overlays data model

**Depends On:** Plan 033 (lifecycle + revisions needed for pinning)

**Status:** Not started

---

## Scope

- `unit_overlays` table: `child_unit_id` (PK) -> `parent_unit_id`, `parent_revision_id` (nullable = floating), `block_overrides` (JSONB with `hide`/`replace` actions)
- Overlay render algorithm: walk parent blocks, apply overrides, insert child-anchored blocks after their anchor, fallthrough for orphaned anchors
- Fork action: create a new unit with an overlay row pointing at the source
- Pin/float toggle: teacher controls whether to track parent's latest published revision or freeze at a specific one
- "New revision available" indicator when parent publishes and child is floating
- Breadcrumb lineage display: "Platform > Lincoln HS (forked 2026-03) > Your adaptation"
- Single-inheritance only (no multi-parent)

## Tasks

To be written when this plan is picked up for implementation.
