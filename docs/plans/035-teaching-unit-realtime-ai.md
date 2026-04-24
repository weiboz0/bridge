# 035 — Teaching Unit: Realtime Co-Edit + AI Drafting

**Goal:** Add Yjs binding via `y-prosemirror` so two teachers can co-author a unit live, and integrate Anthropic API tool use for AI-assisted unit scaffolding ("Draft with AI" panel).

**Spec:** `docs/specs/012-teaching-units.md` — §Authoring UX (realtime co-editing, AI drafting)

**Depends On:** Plan 033 (full block palette needed for AI-generated blocks)

**Status:** Not started

---

## Scope

- Yjs + `y-prosemirror` binding on the Tiptap teaching-unit editor
- Same Hocuspocus service as code collaboration, new document namespace (`unit:{id}`)
- Awareness cursors showing co-editors
- "Draft with AI" panel: accepts intent ("6th grade, while loops, 45 min, 3 problems easy to medium")
- Anthropic API with structured tool use: `create_unit`, `add_problem_draft`, `add_teacher_note`, `add_live_cue`
- AI response streams into the editor as a fresh draft the teacher reviews and publishes
- Preview of student projection inside the editor

## Tasks

To be written when this plan is picked up for implementation.
