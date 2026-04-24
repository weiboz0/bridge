# 033a — Teaching Unit: Lifecycle + Revisions + Block Palette

**Goal:** Add status transitions with `unit_revisions` snapshots on publish, and expand the block palette with `teacher-note`, `code-snippet`, and `media-embed` — the three block types that don't require render projection to be useful.

**Spec:** `docs/specs/012-teaching-units.md` — §Lifecycle, §Block types

**Branch:** `feat/033a-lifecycle-blocks`

**Depends On:** Plan 031 (schema), Plan 032 (real content)

**Unblocks:** Plan 033b (render projection), Plan 034 (overlay reuse — needs revisions for pinning)

**Status:** Not started

---

## Scope

**In scope:**
- Status transition endpoints: `POST /api/units/{id}/publish` (draft→reviewed→classroom_ready|coach_ready), `POST /api/units/{id}/archive`, `POST /api/units/{id}/unarchive`
- `unit_revisions` snapshot created on `classroom_ready` / `coach_ready` transitions (frozen copy of `unit_documents.blocks`)
- `GET /api/units/{id}/revisions` — list revision history
- Expand server-side `knownBlockTypes` allowlist with `teacher-note`, `code-snippet`, `media-embed`
- Custom Tiptap nodes for the three new block types
- Slash-command insertion palette in the editor (`/note`, `/code`, `/media`)
- Editor toolbar: status badge showing current status, publish button (for teachers with edit access)

**Out of scope (plan 033b):**
- `solution-ref`, `test-case-ref`, `live-cue`, `assignment-variant` block types (need projection)
- Render projection pipeline
- Student-facing unit view
- Preview toggle

---

## Tasks

To be written when this plan is picked up for implementation.
