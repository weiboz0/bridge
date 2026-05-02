# Plan 061 — Widen `canViewUnit` for students with verified class/session/assignment binding (P1)

## Status

- **Date:** 2026-05-01
- **Severity:** P1 (Python 101 student flow is broken — students 403 when opening a teaching unit)
- **Origin:** `docs/reviews/009-deep-codebase-review-2026-04-30.md` §P1-6.

## Problem

`platform/internal/handlers/teaching_units.go:132-168`'s `canViewUnit` denies all org-scoped units to students. The comment at line 157 reads:

> "Students are denied until plan 032 wires class/session binding."

Plan 032 shipped. Plans 044 and 048 also shipped. The class/session binding exists. The comment is stale; the code is now wrong.

Two student-facing paths are broken because of this:

1. **`src/components/session/student/student-session.tsx:92-103`** renders links to `/student/units/{unitId}`.
2. **`src/app/(portal)/student/units/[id]/page.tsx:67-79`** calls `fetchUnit` and `fetchProjectedDocument` against the Go API. Both go through `canViewUnit` → both 403 for any logged-in student.

Concrete user impact: a Python 101 student in the demo class (`Python 101 · Period 3`) cannot open ANY of the 12 teaching units. The class is wired, the topics are linked to units, the unit_documents have prose + problem-refs — but the access layer says no.

Plan 049's `--wire-demo-class` flag worked around the *editing* side (cloned units in Bridge Demo School org so eve can edit), but it didn't fix the *student-read* side.

## Out of scope

- Designing the full unit-overlay / fork model — plan 051 placeholder, separate scope.
- Cross-org unit reuse semantics — also plan 051 territory.
- Widening any access path that isn't `canViewUnit` (Hocuspocus is plan 053; problem access is its own surface).

## Approach

Replace the blanket "students denied" rule with a verified-binding rule.

**Important — the rule lives in the shared `CanViewUnit` free function at `platform/internal/handlers/access.go:143`, NOT in `teaching_units.go::canViewUnit` (which is a thin wrapper).** Editing only the wrapper would miss the other consumer at `unit_collections.go:366`. The fix targets `access.go::CanViewUnit`; both callers inherit it.

A student passes `CanViewUnit(student, unit)` when ALL hold:
1. The unit is linked to a topic (`teaching_units.topic_id` is non-NULL).
2. That topic belongs to a course.
3. The student has a class membership row in a class where `classes.course_id` equals that course. **Existence-only check — `class_memberships` has no `status` column today, and existing `UserHasAccessToCourse` at `platform/internal/store/courses.go:88` doesn't filter on one either. Mirroring that convention.**
4. The unit's `status` is `classroom_ready` or `coach_ready` (or `archived` for completeness — read-only).

This mirrors the access shape `topic_problems` uses for problems. The rule is "you can view a unit if you're a student in a class that uses the course that owns the unit's topic."

For platform-scope units linked to a Bridge HQ course: a student in a class whose course is the Bridge HQ course passes (this unblocks the demo flow). Students in OTHER courses don't get access to platform-scope content via this rule unless they're also enrolled in a class using that course — which mirrors the existing `UserHasAccessToCourse` shape.

Edge cases:
- Unit is `draft` or `reviewed` → student denied. Only `classroom_ready`/`coach_ready`/`archived` are visible.
- Unit's `topic_id` is NULL (unattached library content) → student denied. Library content isn't student-visible until linked.
- Org admin viewing a student-scope unit: passes via existing org-admin rule (unchanged).

## Files

- Modify: `platform/internal/handlers/access.go::CanViewUnit` — replace the blanket-deny student branch with the verified-binding lookup. Add a new store method (see below) for the JOIN.
- Add: `platform/internal/store/teaching_units.go::IsStudentInTopicCourse(ctx, userID, topicID) (bool, error)` — single SQL query JOINing `topics → classes → class_memberships` so the access helper doesn't need to issue 3 separate lookups.
- Add: `platform/internal/handlers/teaching_units_integration_test.go` (or extend the existing) — auth matrix with student-of-this-class / student-of-other-class / unenrolled student / draft-unit / archived-class / unit-with-NULL-topic-id.
- Verify: existing teacher and org-admin paths still pass.
- **No TS integration test.** `/api/units/*` has no Next.js handler — `next.config.ts` proxies it directly to Go. The Go integration tests are the source of truth. (TS-integration would need a stub harness that doesn't exist; same finding as plan 056.)

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| The new query is a multi-JOIN and runs on every unit read | medium | Existing indexes cover the hot path: `class_memberships(user_id, class_id)` + `classes(course_id)` + `topics(course_id)` + `teaching_units(topic_id)`. Add `EXPLAIN` output to the test for visibility. |
| A class with no current sessions but a valid course_id → student gets access without an active session | low — this is the intended behavior; "course access" means "in a class that uses that course," sessions are orthogonal. |
| `GetProjectedDocument` (plan 062) renders empty for overlay children regardless of this fix | medium | This plan unblocks the access check; plan 062 fixes the rendering. They're complementary, can land in either order. |
| Cross-org platform-scope unit access broadens unexpectedly | low | The rule still requires class membership in a course that uses the unit's topic. Bridge HQ's Python 101 course is the only one today; future platform-scope content + other-org adoption is gated by the course-clone flow. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch on this plan + `teaching_units.go` + the schema relationships. The Codex pass should confirm:
- The JOIN path is correct end-to-end.
- The status filter matches the existing convention.
- No path is missed (e.g., session-based binding for ad-hoc sessions without class enrollment).

### Phase 1: implement + matrix tests + smoke

Single branch, two commits: helper + handler change, and the integration smoke. Run full suites.

### Phase 2: post-impl Codex review

## Codex Review of This Plan

### Pass 1 — 2026-05-02: BLOCKED → fixes folded in

Codex found 4 blockers:

1. **No `status` column on `class_memberships`.** Plan said "active class
   membership" — table has no status column. Existing
   `UserHasAccessToCourse` doesn't filter on one. **Fix:** drop "active"
   qualifier; existence is sufficient.

2. **Student-deny lives in `access.go::CanViewUnit`, not
   `teaching_units.go::canViewUnit`.** The handler method is a thin
   wrapper; editing only it would miss the `unit_collections.go:366`
   caller. **Fix:** target `access.go` so both callers inherit.

3. **Demo seed doesn't wire student memberships into Python 101.**
   Python 101 importer creates the class but doesn't add student
   class_memberships. **Fix:** Go integration tests build their own
   class+membership fixture rather than depending on seed wiring.

4. **No `/api/units` Next.js handler exists.** Pure Go-proxy
   rewrite. Plan's TS integration test would have nothing to import.
   **Fix:** drop the TS integration test; Go handler tests are the
   source of truth.

### Pass 2 — 2026-05-02: **CONCUR**

Codex confirmed:
- Existence-only rule matches `topic_problems` access pattern
  (`UserHasAccessToCourse` + `class_memberships.user_id` join, no
  status filter).
- `IsStudentInTopicCourse(ctx, userID, topicID) (bool, error)` shape
  is sufficient — unit row already carries `Status` and `TopicID`,
  so no class IDs need to be returned.

Plan ready for implementation.
