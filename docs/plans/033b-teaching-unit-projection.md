# 033b — Teaching Unit: Render Projection + Student View

**Goal:** Build the per-role render projection pipeline that filters blocks based on `(viewer_role, attempt_state)`, add the remaining block types, and ship the student-facing unit view + editor preview toggle.

**Spec:** `docs/specs/012-teaching-units.md` — §Render projection, §Block types (remaining)

**Branch:** `feat/033b-projection`

**Depends On:** Plan 033a (lifecycle + revisions + base block palette)

**Unblocks:** Plan 034 (overlay reuse), Plan 036 (discovery)

**Status:** In progress

---

## Scope

**In scope:**
- Go projection pipeline: pure function `ProjectBlocks(blocks, viewerRole, attemptStates) → filteredBlocks`
- Go projection endpoint: `GET /api/units/{id}/projected?role=student`
- 4 new block types: `solution-ref`, `test-case-ref`, `live-cue`, `assignment-variant`
- Server-side allowlist expansion
- Custom Tiptap nodes for the 4 new block types
- Student-facing unit page: `/student/units/{id}`
- Preview toggle in editor (teacher sees student projection)

**Out of scope:**
- Session → unit binding (`session_units` join table) — follow-up plan
- Overlay composition (plan 034 — projection runs after overlay merge)
- Assignment grading flow integration
- AI drafting (plan 035)

**Simplifications for this plan:**
- `assignment-variant`: always omitted for students (no session/assignment binding exists yet)
- `live-cue`: always omitted for students (no session context to determine trigger state)
- `solution-ref` with `reveal: "after-submit"`: treated as hidden for students (no attempt_state tracking wired yet — will be connected in a follow-up when session_units lands)
- `solution-ref` with `reveal: "always"`: shown to students

---

## Task 1: Go projection pipeline

**Files:**
- Create: `platform/internal/projection/project.go`
- Create: `platform/internal/projection/project_test.go`

Pure function, no DB access:

```go
package projection

type ViewerRole string
const (
    RoleStudent  ViewerRole = "student"
    RoleTeacher  ViewerRole = "teacher"
    RoleAdmin    ViewerRole = "platform_admin"
)

type AttemptState string
const (
    AttemptNotStarted AttemptState = "not_started"
    AttemptSubmitted  AttemptState = "submitted"
    AttemptPassed     AttemptState = "passed"
    AttemptFailed     AttemptState = "failed"
)

// ProjectBlocks filters a unit document's blocks for the given viewer.
// attemptStates maps problem-ref block IDs to their attempt state (for
// solution-ref reveal logic). Blocks not in the output are omitted entirely.
func ProjectBlocks(
    blocks []json.RawMessage,
    role ViewerRole,
    attemptStates map[string]AttemptState,
) []json.RawMessage
```

Per spec 012 projection table:
- `prose`, `code-snippet`, `media-embed`, `paragraph`, `heading` → always included
- `problem-ref` → included if `visibility: "always"` or role is teacher/admin
- `teacher-note` → included only for teacher/admin
- `live-cue` → included only for teacher/admin
- `solution-ref` → for students: included if `reveal: "always"`, or `reveal: "after-submit"` AND attempt state is submitted/passed/failed. For teacher/admin: always.
- `test-case-ref` → included for all (redaction of hidden cases happens at the API layer per spec 009, not in projection)
- `assignment-variant` → included only for teacher/admin (no assignment binding yet)

**Tests:** One test per block type × role combination. Verify teacher sees everything, student sees filtered output.

**Commit:** `feat(033b): projection pipeline — pure block filter by role + attempt state`

---

## Task 2: Go projection endpoint

**Files:**
- Modify: `platform/internal/handlers/teaching_units.go`
- Modify: `platform/internal/handlers/teaching_units_integration_test.go`

Add route:
```
GET /api/units/{id}/projected?role=student&attemptStates=b03:submitted,b05:not_started
```

Handler:
- `canViewUnit` auth check
- Fetch unit document
- Parse `role` query param (default: derive from caller's actual role)
- Parse `attemptStates` query param (comma-separated `blockId:state` pairs)
- Call `projection.ProjectBlocks`
- Return `{ "type": "doc", "content": [...filtered...] }`

Platform admins and teachers can request `?role=student` to preview.
Students always get `role=student` regardless of query param.

**Tests:**
- Teacher requesting `?role=student` → teacher-notes omitted
- Student requesting default → teacher-notes, live-cues omitted
- solution-ref with `reveal: "always"` → included for student
- solution-ref with `reveal: "after-submit"` + no attempt state → omitted for student
- Teacher sees everything (no filtering)

**Commit:** `feat(033b): projected unit endpoint`

---

## Task 3: Block allowlist + Tiptap nodes

**Files:**
- Modify: `platform/internal/handlers/teaching_units.go` — expand allowlist
- Create: `src/components/editor/tiptap/solution-ref-node.tsx`
- Create: `src/components/editor/tiptap/test-case-ref-node.tsx`
- Create: `src/components/editor/tiptap/live-cue-node.tsx`
- Create: `src/components/editor/tiptap/assignment-variant-node.tsx`
- Modify: `src/components/editor/tiptap/extensions.ts`

**solution-ref:**
- `attrs: { id, solutionId, reveal: "always"|"after-submit" }`
- Renders: fetches solution from API, shows title + language badge. If `reveal: "after-submit"`, shows a "Revealed after submission" label.

**test-case-ref:**
- `attrs: { id, testCaseId, problemRefId, showToStudent: boolean }`
- Renders: test case name + visibility indicator

**live-cue:**
- `attrs: { id, trigger: "before-problem"|"after-problem"|"manual", problemRefId?: string }`
- Rich text content (like teacher-note)
- Renders with a blue left border + "Live Cue" label + trigger info

**assignment-variant:**
- `attrs: { id, title, timeLimitMinutes?: number }`
- Container block with child content (like teacher-note)
- Renders with a purple border + "Assignment" label

Register all in extensions.ts + add slash commands `/solution`, `/testcase`, `/cue`, `/assignment`.

**Commit:** `feat(033b): Tiptap nodes — solution-ref, test-case-ref, live-cue, assignment-variant`

---

## Task 4: Student-facing unit page

**Files:**
- Create: `src/app/(portal)/student/units/[id]/page.tsx`
- Modify: `src/components/editor/tiptap/teaching-unit-viewer.tsx` — accept projected blocks

Student unit page:
- Fetch projected document: `GET /api/units/{id}/projected?role=student`
- Render with `TeachingUnitViewer`
- Show unit metadata (title, status badge)
- If unit not accessible → redirect to /student or show 404

**Commit:** `feat(033b): student-facing unit page with projected view`

---

## Task 5: Editor preview toggle

**Files:**
- Modify: `src/app/(portal)/teacher/units/[id]/edit/page.tsx`
- Modify: `src/components/editor/tiptap/teaching-unit-editor.tsx`

Add a "Preview as Student" toggle button in the editor header. When active:
- Fetch projected blocks from `GET /api/units/{id}/projected?role=student`
- Replace the editor with a `TeachingUnitViewer` showing the projected output
- Toggle back to edit mode restores the full editor

**Commit:** `feat(033b): editor preview toggle — teacher sees student projection`

---

## Task 6: Verify + docs

**Verification:**
```bash
cd platform && go test ./... -count=1 -timeout 180s
node_modules/.bin/vitest run
node_modules/.bin/tsc --noEmit
```

Update `docs/api.md` with projected endpoint documentation.
Write post-execution report.

**Commit:** `docs(033b): API reference + post-execution report`

---

## Code Review

### Review 1

- **Date**: 2026-04-24
- **Reviewer**: Codex
- **Verdict**: Approved with fixes

1. `[FIXED]` `attemptStates` parser rejected `in_progress` — a valid spec-012 state. Added `AttemptInProgress` constant and accepted it in the parser.

2. `[FIXED]` Preview toggle proceeded after failed auto-save, showing stale projected content. `handleSave` now re-throws after showing "Save failed" so the preview catch block fires.

3. `[WONTFIX]` Student unit page flattens 401 to "unavailable" instead of redirecting to login. The portal layout handles unauthenticated users at a higher level; the Go API enforces auth. Acceptable for MVP.

## Post-Execution Report

Populate after implementation.
