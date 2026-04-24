# 033b — Teaching Unit: Render Projection + Student View

**Goal:** Build the per-role render projection pipeline that filters blocks based on `(viewer_role, attempt_state)`, add the remaining block types that depend on projection (`solution-ref`, `test-case-ref`, `live-cue`, `assignment-variant`), and ship the student-facing unit view + editor preview toggle.

**Spec:** `docs/specs/012-teaching-units.md` — §Render projection, §Block types (remaining)

**Branch:** `feat/033b-projection`

**Depends On:** Plan 033a (lifecycle + revisions + base block palette)

**Unblocks:** Plan 034 (overlay reuse), Plan 036 (discovery — needs student view for library cards)

**Status:** Not started

---

## Scope

**In scope:**
- Render projection pipeline: pure function of `(block, viewer_role, attempt_state)` per spec 012 projection table
- New block types: `solution-ref` (reveal-after-attempt), `test-case-ref` (visibility override), `live-cue` (teacher-only, trigger-based), `assignment-variant` (gradable wrapper)
- Custom Tiptap nodes for the four new block types
- Student-facing unit page: `/student/units/{id}` — projected view (no teacher-notes, no live-cues, solutions gated by attempt state)
- Session → unit binding: `session_units` join table so sessions can reference teaching units (replaces/augments `session_topics`)
- Preview toggle in the editor: teacher sees student projection without leaving the edit page
- Go projection endpoint: `GET /api/units/{id}/projected?role=student&attemptState=...`

**Out of scope:**
- Overlay composition (plan 034 — projection runs after overlay merge)
- AI drafting (plan 035)
- Assignment grading flow integration (follow-up spec)

---

## Tasks

To be written when this plan is picked up for implementation.
