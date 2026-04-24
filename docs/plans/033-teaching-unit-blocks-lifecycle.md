# 033 — Teaching Unit: Full Block Palette + Lifecycle + Projection

**Goal:** Add all remaining block types (`teacher-note`, `live-cue`, `solution-ref`, `test-case-ref`, `assignment-variant`, `media-embed`, `code-snippet`), implement status transitions with `unit_revisions` snapshot creation, and build the render projection pipeline with per-role filtering and attempt-state resolution.

**Spec:** `docs/specs/012-teaching-units.md` — §Block types, §Lifecycle, §Render projection

**Depends On:** Plan 031, Plan 032 (for real content to test against)

**Status:** Not started

---

## Scope

- Custom Tiptap nodes for each new block type with appropriate rendering
- Server-side block type allowlist expanded
- Status transition endpoints: `draft -> reviewed -> classroom_ready | coach_ready`, archive/unarchive
- `unit_revisions` snapshot created on `classroom_ready` / `coach_ready` transitions
- Render projection pipeline: pure function of `(block, viewer_role, attempt_state)` that filters blocks per the spec's projection table
- Student-facing unit view (projected, no teacher-notes or live-cues)
- Preview toggle in the editor (teacher sees student projection without leaving)

## Tasks

To be written when this plan is picked up for implementation.
