# 044 — Topic → Syllabus Area: Read-Path + UI Cutover

**Goal:** Stop reading `topic.lessonContent` and `topic.starterCode` everywhere; route every teaching-material display through the Unit linked via `teachingUnits.topicId`. Migration 0017 already moved historical Topic content into Units — this plan is the UI/API cutover, not a data migration.

**Sources:**
- `docs/reviews/003-teacher-portal-review-2026-04-26.md` P1: "Topic editor still treats topics as teaching material"
- `docs/reviews/004-student-portal-review-2026-04-26.md` P1: "Student surfaces still use Topic lesson content instead of Units"
- Plan 043 deferral.

**Branch:** `feat/044-topic-syllabus-refactor`

**Status:** Draft — awaiting approval (post-Codex-review revision)

---

## Pre-Plan Discovery

A planning-time check showed the migration is already done:

```
SELECT COUNT(*) FROM topics;                            -- 15
SELECT COUNT(*) FROM topics WHERE lesson_content::text != '{}'
                                AND lesson_content::text != 'null'
                                AND length(lesson_content::text) > 2;  -- 0
SELECT COUNT(*) FROM teaching_units WHERE topic_id IS NOT NULL;        -- 14
```

Migration `0017_topic_to_units.sql` (plan 032) already converted every Topic with non-empty `lesson_content` (and the linked `topic_problems` rows) into a `teaching_unit` + `unit_documents` blocks tree. The `teaching_units.topic_id` column is unique (`teaching_units_topic_id_uniq`), so the schema is **1:1** — each Topic has zero or one linked Unit, never multiple.

What survived plan 032: the *read paths* (LessonEditor on the teacher topic-edit page; LessonRenderer on student class detail and live-session pages) still try to render `topic.lessonContent`. With the data already moved, those reads return empty content. Plan 044 cuts those read paths to the linked Unit instead, then locks the API contract.

---

## Schema Reality Check (1:1)

The schema enforces **at most one Unit per Topic** via `CREATE UNIQUE INDEX … teaching_units_topic_id_uniq ON teaching_units(topic_id) WHERE topic_id IS NOT NULL` (`drizzle/0017_topic_to_units.sql:15`). Plan 044 embraces this: every read path renders zero or one Unit per Topic, with an empty state when none is linked.

If the product later wants Unit reuse (one Unit referenced by multiple Topics), that's a future schema change with its own plan — likely a junction table — and out of scope here.

---

## Scope

### In scope

- **Server read paths** stop returning `topic.lessonContent` and start returning the linked Unit's identity.
- **All UI consumers** of Topic content (teacher topic-edit, student class detail, live-session student delivery, parent live-session viewer, teacher session dashboard) switch to reading the linked Unit.
- **Server write paths** stop accepting `lessonContent` and `starterCode` on Topic POST and PATCH.
- **Topic-edit page** gains a primitive Unit-ID input so new topics aren't stranded without an attach UI (full picker deferred to plan 045).
- **`topic.lessonContent` and `topic.starterCode` columns stay** for one release as a safety net; plan 046 drops them after this is in production.

### Out of scope (explicit deferrals)

- **Plan 045 — Unit-attach picker UI.** A real "search and attach a Unit" picker on the topic-edit page. Plan 044 ships a primitive ID-input fallback so the cutover doesn't strand new topics; the polished picker is its own UX work.
- **Plan 046 — Drop deprecated columns.** Once 044 is in production for one release with no read-path consumers, drop `topic.lessonContent` + `topic.starterCode` + the migration trail.
- **Topic rename** to "Syllabus Area" / "Focus Area" — naming decision spans many copy strings.
- **Plan 047 — My Work navigability** (still queued from review 004 P2, pushed back one number).
- **Multi-Unit-per-Topic** — schema is 1:1; this plan does not change that.

---

## Implementation Strategy: Additive → Remove

Pre-impl Codex flagged a phase-ordering bug in the original draft (locking writes before removing the writer is a regression window). The corrected pattern: **add the new shape first, then remove the old**.

1. **Add** `unitId` (and a few denormalized fields) to topic-related read responses, keeping `lessonContent` in place.
2. **Switch** UIs to read the new shape.
3. **Remove** `lessonContent` from read responses; reject it on writes.

Each phase ships a working state.

---

## Phase 1: Read Paths (additive)

### Task 1.1: Topic GET shape

**Files:**
- `src/lib/session-topics.ts` — keep `lessonContent` field for compatibility AND add `unitId`, `unitTitle`, `unitMaterialType` from a left-join against `teachingUnits` (where `teachingUnits.topicId = topics.id`). The query stays a single statement, no N+1.
- `src/app/api/courses/[id]/topics/[topicId]/route.ts` — same shape addition on GET.
- Any other module returning topic rows: `grep -rn "topics\." src/lib/ src/app/api`.

### Task 1.2: `/api/sessions/{id}/topics` shape

**File:** `platform/internal/store/sessions.go::GetSessionTopics` (and the handler around it). Add `unit_id`, `unit_title`, `unit_material_type` to the SELECT via the same LEFT JOIN pattern; preserve `lesson_content` for now.

### Task 1.3: Org-scope safety on the join

Codex correction #3: the join must not surface a Unit whose `scope_id` doesn't match the topic's parent course's `org_id`. Add `AND (teaching_units.scope = 'platform' OR teaching_units.scope_id = courses.org_id)` to every Unit-by-topic join. (Platform-scope Units are visible everywhere.) Without this, a misaligned `topic_id` could leak a Unit cross-org.

### Task 1.4: Tests

- Update existing topic GET tests (Vitest integration) to assert the new fields.
- Go integration test for `GetSessionTopics` covering: topic with linked Unit, topic without linked Unit, cross-org leakage attempt (planted Unit with mismatched scope_id should NOT appear).

---

## Phase 2: UI Cutover

### Task 2.1: Teacher topic-edit page

**File:** `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx`.

- Strip the `LessonEditor` import and state.
- Form keeps title, description, sort_order. PATCH body NO LONGER includes `lessonContent`.
- Render the linked Unit when present: title, materialType badge, link to `/teacher/units/<unitId>/edit`.
- Codex correction #4: include a **primitive Unit-ID input** so new topics aren't stranded:
  - Text input labeled "Linked Unit ID" with a "Link" button.
  - Submit calls a new endpoint `POST /api/topics/{topicId}/link-unit` that updates `teaching_units.topic_id`.
  - Validates the Unit exists, the caller can edit it (existing teaching-unit auth), and the Unit isn't already linked to another Topic (1:1 invariant — error if conflict).
  - Empty state when no Unit linked: "No teaching unit linked. Paste a Unit ID above, or create one in the Units library and copy its ID."

### Task 2.2: `link-unit` endpoint

**File:** `platform/internal/handlers/topics.go` (or wherever topic handlers live). New `POST /api/topics/{topicId}/link-unit` body `{ unitId: string }`. Auth: caller must be able to edit the Topic's parent course (existing course-edit gate). Checks the Unit exists, caller can edit the Unit (existing teaching-unit auth), and no other Topic owns it (the unique-index constraint will reject it; we surface a clean error message).

### Task 2.3: Student class detail page

**File:** `src/app/(portal)/student/classes/[id]/page.tsx`.

Drop `LessonRenderer` + `parseLessonContent`. For each topic card, render the linked Unit as a small card with a link to `/student/units/<unitId>`. Empty state: "No material yet for this topic."

### Task 2.4: Student live-session

**File:** `src/components/session/student/student-session.tsx`.

Same shift — drop `LessonRenderer`, render Unit titles/cards. The unit gets opened in a side panel via the existing student unit viewer (or a new tab — pick whichever is simpler at impl time).

### Task 2.5: Parent live-session viewer

Codex correction #10b: `src/components/parent/live-session-viewer.tsx` reads `lessonContent` via `/api/sessions/{id}/topics`. Same UI fix — render Unit references instead.

### Task 2.6: Teacher dashboard topic rendering

Codex correction #7: the teacher dashboard's `courseTopics` rendering must handle "topic has no linked Unit AND has no legacy content" with a per-topic empty state, not just an all-topics-empty banner.

**File:** `src/components/session/teacher/teacher-dashboard.tsx`.

### Task 2.7: Tests

Vitest tests for each rewired UI surface. Mock the topic data with linked-Unit, no-linked-Unit, and (for the legacy-content guard window) populated-lessonContent cases. Assert correct rendering of each.

---

## Phase 3: Write-Path Lock

### Task 3.1: Topic POST + PATCH zod schemas

**Files:**
- `src/app/api/courses/[id]/topics/route.ts` (POST) — Codex correction #10a. Drop `lessonContent` and `starterCode` from the input schema. Reject 400 if either is present. (NB: route still accepts the fields if we use `.passthrough()` — switch to strict mode if it isn't already.)
- `src/app/api/courses/[id]/topics/[topicId]/route.ts` (PATCH) — same.

### Task 3.2: Topic GET stops returning deprecated fields

After Phase 2 ships and consumers read the new shape, drop `lessonContent` and `starterCode` from the GET response. Phase 1 was additive; this is the removal.

### Task 3.3: Tests

Update topic API integration tests: POST/PATCH with the deprecated fields → 400. GET response → no `lessonContent` / `starterCode`.

---

## Phase 4: Schema Deprecation Annotations

### Task 4.1: Drizzle schema comments

**File:** `src/lib/db/schema.ts:308-321` (the `topics` table).

Annotate `lessonContent` and `starterCode` with `// @deprecated — content lives in linked teaching_units (teachingUnits.topicId). Plan 046 drops these columns.`

### Task 4.2: CLAUDE.md / docs

Spot-check `docs/`, `CLAUDE.md`, and any AI workflow files for `topic.lessonContent` references; remove or update.

---

## Implementation Order

| Phase | Tasks | Why |
|-------|-------|-----|
| 1 | 1.1 → 1.2 → 1.3 → 1.4 | Additive read-path changes — no UI consequences yet, tests confirm new shape arrives. |
| 2 | 2.1 → 2.2 → 2.3 → 2.4 → 2.5 → 2.6 → 2.7 | UI cutover. After this, no UI reads `lessonContent`. |
| 3 | 3.1 → 3.2 → 3.3 | Write-path lock + GET shape removal. Safe now that no UI reads the deprecated fields. |
| 4 | 4.1 → 4.2 | Annotations. |

One PR. ~10-12 commits.

**Extraction gate after Phase 1:** if the additive read-path work surfaces unexpected schema gaps (e.g., a topic with non-empty `lessonContent` that 0017 missed; a course without `org_id`), pause and resolve before Phase 2 ships.

---

## Verification per Phase

- **Phase 1**: API responses now include `unitId` etc. when a Unit is linked. Tests cover linked / unlinked / cross-org-attempt scenarios.
- **Phase 2**: open every rewired surface in dev:
  - Teacher topic-edit: LessonEditor gone; linked-Unit display + ID-input work.
  - Student class detail: linked Units render; empty state appears for unlinked topics.
  - Live session (student): same.
  - Parent live-session viewer: same.
  - Teacher dashboard: per-topic empty state shows correctly.
- **Phase 3**: POST/PATCH with `lessonContent` returns 400; GET response no longer includes the deprecated fields.
- **Phase 4**: schema deprecation annotations present.
- **Whole plan**: full Vitest + Go suite green; manual smoke test of teacher and student flows.

---

## Out-of-Scope Acknowledgements

Recapping deferrals:

- **Plan 045 — Unit-attach picker UI.**
- **Plan 046 — Drop `topic.lessonContent` + `topic.starterCode` columns.**
- **Plan 047 — My Work navigability** (was 045 in earlier plans; renumbered).
- **Topic rename** ("Syllabus Area" / "Focus Area").
- **Multi-Unit-per-Topic** — schema is 1:1.

---

## Codex Review of This Plan

- **Date:** 2026-04-27
- **Reviewer:** Codex (pre-implementation, via `codex:rescue` against the original draft)
- **Verdict:** 3 CRITICAL + 7 IMPORTANT corrections forced a substantive rewrite. This document IS the post-correction plan.

### Corrections applied

1. `[CRITICAL]` **Phase order inverted (#2 in Codex's findings).** Original draft locked writes (Phase 2) before removing the writer (Phase 3) — would break the existing teacher edit page until Phase 3 lands. → New phase order is **additive read paths → UI cutover → write-path lock**, so each phase ships a working state.
2. `[CRITICAL]` **1:1 schema invariant ignored (#5).** Original draft talked about "list of attached Units" — but `teaching_units.topic_id` is unique. Each Topic has zero or one linked Unit. → Plan now embraces the 1:1, with empty state when no Unit is linked. Multi-Unit-per-Topic is explicitly out of scope.
3. `[CRITICAL]` **Two missing cut surfaces (#10).** Topic POST endpoint also accepts `lessonContent`/`starterCode`; parent live-session viewer also reads `topic.lessonContent`. → Both added to scope (Tasks 3.1 and 2.5).
4. `[IMPORTANT]` **Migration is already done.** Pre-plan discovery confirmed migration `0017_topic_to_units.sql` (plan 032) already converted every Topic with non-empty content. → Plan 044's original "Phase 1: Migration script" is dropped. The new Phase 1 is additive read-path work.
5. `[IMPORTANT]` **Idempotency on `topic_id` unique index (#1).** Moot since the migration is no longer in this plan.
6. `[IMPORTANT]` **Cross-org leak risk in `ListUnitsByTopicIDs` (#3).** → Task 1.3 adds an explicit org-scope filter to the join.
7. `[IMPORTANT]` **Phase 3 regression for new topics (#4).** Without an attach UI, a fresh topic has no path to get a Unit linked. → Task 2.1 adds a primitive Unit-ID text input + `POST /api/topics/{topicId}/link-unit` endpoint (Task 2.2). The polished picker is plan 045.
8. `[IMPORTANT]` **`topic_problems` already converted (#6).** Migration 0017's `DO` block already added `problem-ref` blocks for each `topic_problems` row when it created the Unit. → No additional `topic_problems` work in scope; Phase 1 read paths surface them via the Unit.
9. `[IMPORTANT]` **Empty-state copy inconsistent (#3 second part).** → Standardized: "No material yet" for student-side, "No teaching unit linked. Paste a Unit ID above…" for teacher-side. Different roles, different actionable hints — but both clearly per-topic.
10. `[IMPORTANT]` **Per-topic empty state on teacher dashboard (#7).** → Added explicit Task 2.6.
11. `[IMPORTANT]` **`/api/sessions/{id}/topics` rewire (#8).** → Added Task 1.2.
12. `[IMPORTANT]` **One PR too risky (#9).** → Mitigated by phase-level extraction gate after Phase 1, plus the additive→remove pattern means each phase ships a working state. If any phase grows unexpectedly, it splits to 044a/044b.
---

## Post-Execution Report

**Date:** 2026-04-27
**Branch:** `feat/044-topic-syllabus-refactor`
**Commits (5 + 1 fix):**
- `ed1cc24` docs: plan 044 — Topic → Syllabus Area taxonomy refactor
- `2bb7b09` feat(044): additive read paths surface linked Unit (phase 1)
- `92b998c` feat(044): UI cutover — read linked Units instead of topic.lessonContent (phase 2)
- `d2d59b8` feat(044): write-path lock — reject deprecated topic fields (phase 3)
- `18e38aa` Plan 044 phase 4: deprecate lessonContent/starterCode
- `f612b69` fix(044): post-impl review fixes — cross-org link gate, race-safe link, Go clone

### Verification

- Vitest: **462 passed | 11 skipped** (full suite, post-fixes)
- Go: all packages green (`internal/handlers` 99.9s, `internal/store` 31.9s)
- Type-check: clean for plan-044 surface (5 pre-existing unrelated TS errors in `units/new/page.tsx`, `admin/user-actions.tsx`, `lesson-content.test.ts`, `identity-assert.test.ts`, `auth-register.test.ts`, `annotations.test.ts` are documented as pre-existing in earlier reviews)

### Codex post-impl review findings (all fixed in `f612b69`)

- `[IMPORTANT]` `LinkUnit` did not enforce cross-org reachability — Units from any org where the caller had teacher rights would link successfully, but the read-side guard would silently filter them out for students. Fix: added `unit.Scope == "platform" || (unit.Scope == "org" && *unit.ScopeID == course.OrgID)` check in `topics.go::LinkUnit` after the unit fetch. Personal-scope and wrong-org Units now return 403.
- `[IMPORTANT]` `LinkUnitToTopic` had a TOCTOU race — concurrent linkers would both pass `GetUnitByTopicID` and one would hit the unique-index violation as an opaque 500. Fix: added `isUniqueViolationOn` helper that detects 23505 on `teaching_units_topic_id_uniq` (across both lib/pq and pgx/pgconn driver shapes) and returns `ErrTopicAlreadyLinked`.
- `[IMPORTANT]` Go `CloneCourse` SQL still copied `lesson_content` and `starter_code`. The TS path (`src/lib/courses.ts`) was cleaned up in phase 4 but the Go path was missed. Fix: removed both columns from the clone INSERT-SELECT in `courses.go::CloneCourse`. Existing clone test extended to assert lesson_content defaults to `{}` and starter_code is NULL.
- `[MINOR]` link-unit handler tests were missing wrong-org and wrong-owner cases. Added `TestLinkUnit_WrongOrgUnit_Forbidden` and `TestLinkUnit_WrongOwnerPersonalUnit_Forbidden` (both pass against live DB).

### What ships

- Read paths everywhere read the linked teaching_unit via the 1:1 join. Cross-org leak guard in place at every JOIN site (4 sites: `src/lib/session-topics.ts` × 2 helpers, `platform/internal/store/sessions.go::GetSessionTopics`, `platform/internal/store/teaching_units.go::ListUnitsByTopicIDs`).
- Teacher topic editor uses a primitive paste-Unit-ID picker + new `POST /api/courses/{cid}/topics/{tid}/link-unit` endpoint with full course-edit + Unit-edit + cross-org gates. Errors map cleanly: 400 (missing/invalid id), 403 (not creator OR wrong-org Unit OR cannot edit unit), 404 (unknown course/topic/unit), 409 (1:1 conflict). Idempotent for same-unit-same-topic.
- Strict zod on TS Topic POST/PATCH + explicit-400 in Go handlers reject any body that includes `lessonContent` or `starterCode`. Old client code that still sent these will get a clean error instead of silently writing into a deprecated column.
- Schema columns retain `@deprecated` JSDoc pointing at plan 046 (column drop). One-release safety net.
- Course clone (TS + Go) no longer copies the deprecated columns. Cloned topics start empty; teachers attach Units via the topic editor.

### Out-of-scope deferrals (carry forward unchanged)

- **Plan 045** — proper Unit-attach picker UI (search by title, filter by org/grade/material type) replacing the paste-ID input.
- **Plan 046** — drop `topic.lesson_content` and `topic.starter_code` columns + the JSON content-block code path. (Shipped 2026-04-28.)
- **Plan 047** — My Work navigability (topic rename, multi-Unit-per-Topic remain explicitly out of scope).
