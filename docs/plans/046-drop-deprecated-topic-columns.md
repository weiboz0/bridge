# Plan 046 — Drop Deprecated Topic Columns

## Status

- **Date:** 2026-04-27
- **Branch:** `feat/046-drop-deprecated-topic-columns` (chained off `feat/045-unit-picker`)
- **Depends on:** Plan 044 + 045 merged. The "one release safety net" gate is **explicitly waived** — Bridge is pre-production (no live student/teacher traffic), so manual smoke-testing through plans 044 + 045 is sufficient evidence the linked-Unit world is solid. Once Bridge enters pilot, the safety-net rule applies for future deprecations.
- **Goal:** Remove `topics.lesson_content` and `topics.starter_code` columns from the database, drop them from the Drizzle/Go schemas, retire the orphaned `LessonContent` helper code path, and replace the deprecation-rejection tests with strict-mode unknown-field rejection coverage.

## Problem

Plan 044 deprecated but did not drop `topics.lesson_content` (jsonb) and `topics.starter_code` (text). It kept them as a "one release safety net" — if migration 0017 (plan 032) had missed any rows, or if any read path was missed in plan 044, the data would still be there. Plan 044 also kept the `@deprecated` JSDoc and the strict-zod / Go reject paths.

That safety net has done its job. The columns are unread by user-facing code paths, unwritten, and the column drop is overdue. Leaving them in place creates ongoing confusion: every future schema reader has to mentally filter out the deprecated columns, and every test author has to remember the rejection contract.

## Goal

After plan 046:

- `topics.lesson_content` and `topics.starter_code` no longer exist in the database.
- Drizzle schema (`src/lib/db/schema.ts`) doesn't define them.
- Go `Topic` struct, `topicColumns`, `CreateTopicInput`, `UpdateTopicInput` no longer reference them.
- `platform/internal/store/sessions.go::GetSessionTopics` no longer SELECTs them; `SessionTopicWithDetails` no longer has the `LessonContent` / `StarterCode` fields; `platform/internal/handlers/sessions.go::teacherPageTopicRef` no longer has `LessonContent`; the mapping at `sessions.go:1235-1239` no longer copies it.
- `src/lib/session-topics.ts::getSessionTopics` SELECT no longer includes `lessonContent` / `starterCode`.
- The orphaned `LessonContent` helper code path (`src/lib/lesson-content.ts`, `src/components/lesson/lesson-renderer.tsx`, `src/components/lesson/lesson-editor.tsx`) is deleted — these were only used to render `topic.lesson_content`. Confirmed unused by Units (Units use the Tiptap-based `unit_documents.blocks` format).
- The TS `.strict()` zod schemas on the topic POST/PATCH routes stay in place; the field-specific deprecation rejection comments come out (since the field doesn't exist, the strict mode catches it generically).
- The Go topic create/update handlers stop checking for the specific deprecated field names and instead use `json.Decoder.DisallowUnknownFields()` on the request body to symmetrically reject unknown fields (mirroring TS strict mode). This is a tighter contract than today's "ignore unknown fields silently" Go default — chosen for parity with TS and to catch future stale clients.
- Tests: the 6 Go + 4 TS deprecation rejection tests are replaced by 4 unknown-field tests (TS POST + PATCH, Go CreateTopic + UpdateTopic) that use `lessonContent` as the canary field — same contract, just expressed as "unknown field" rather than "deprecated field."

## Non-Goals

- Schema work on Units, courses, classes, sessions, or anything not directly named above.
- Topic renaming ("Syllabus Area"/"Focus Area") — plan 047.
- Multi-Unit-per-Topic — explicitly out, schema is 1:1.
- Backfilling missed Topic→Unit conversions — migration 0017 already did this; pre-plan discovery in plan 044 confirmed dev DB has 0 topics with non-empty `lesson_content` and 14/15 with linked Units. We re-verify in Phase 0 below with a more thorough query.

## Pre-Plan Discovery (Phase 0 — must run before any code phase)

Run against dev (and staging if available; see release-gate note above for why production isn't gated). All three queries must return zero rows. The first two produce a row listing instead of just a count, so a single non-empty topic is impossible to overlook.

```sql
-- Q1: Any topics carrying non-empty lesson_content (linked OR unlinked).
-- Plan 044's discovery only counted; this lists rows so we cannot miss a stragglar.
SELECT t.id, t.title, length(t.lesson_content::text) AS payload_len,
       u.id IS NOT NULL AS has_linked_unit
FROM topics t
LEFT JOIN teaching_units u ON u.topic_id = t.id
WHERE t.lesson_content IS NOT NULL
  AND t.lesson_content::text NOT IN ('null', '{}', '');

-- Q2: Any topics carrying non-empty starter_code (linked OR unlinked).
SELECT t.id, t.title, length(t.starter_code) AS payload_len,
       u.id IS NOT NULL AS has_linked_unit
FROM topics t
LEFT JOIN teaching_units u ON u.topic_id = t.id
WHERE t.starter_code IS NOT NULL AND t.starter_code != '';

-- Q3: Schema-level dependencies on the columns (views, triggers, FKs, indexes).
-- pg_depend covers most catalog references; if anything is reported, address it
-- before dropping the column or the migration will fail.
SELECT DISTINCT n.nspname || '.' || c.relname AS dependent_object,
                d.deptype, a.attname AS dependent_column
FROM pg_depend d
JOIN pg_attribute a ON a.attrelid = d.refobjid AND a.attnum = d.refobjsubid
JOIN pg_class tc ON tc.oid = d.refobjid AND tc.relname = 'topics'
JOIN pg_class c ON c.oid = d.objid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE a.attname IN ('lesson_content', 'starter_code')
  AND d.deptype != 'a';  -- exclude trivial 'auto' dependencies
```

**Stop conditions:**
- Q1 returns ANY rows: a topic still has lesson_content. If `has_linked_unit = true`, the data is unread (the user-facing path uses the linked Unit) but would be silently dropped. Investigate whether this content needs to migrate to the linked Unit before dropping the column.
- Q2 returns ANY rows: same logic for starter_code.
- Q3 returns ANY rows: a view, trigger, FK, or index references one of the columns. Drop or rebuild the dependency before the column drop.

If all three return empty, proceed to Phase 1.

## Implementation Order

Strict order, code first then migration:

1. **Phase 0 (this section above):** Discovery queries.
2. **Phase 1: TS code cleanup.** Strip the columns from Drizzle schema and every TS read site.
3. **Phase 2: Go code cleanup.** Strip from store, handlers, struct fields, and tests.
4. **Phase 3: Strict-zod / DisallowUnknownFields tests.** Replace the deprecation tests with unknown-field rejection tests.
5. **Phase 4: Migration applied.** Run migration 0022 against dev; full Vitest + Go suite passes.
6. **Phase 5: Docs + TODO.**

This ordering means at every commit boundary the app boots and the tests pass. Phase 1 + 2 remove all reads of the columns BEFORE Phase 4 drops them — so even if a downstream environment's deploy applies the migration before the new code rolls out (which would be an operator mistake; see Risk Surface), nothing crashes because the new code already doesn't read the columns. The columns remain queryable until Phase 4 actually drops them.

## Phase 1: TypeScript code cleanup

**Files:**

- Modify: `src/lib/db/schema.ts` — delete the two `lessonContent` / `starterCode` columns from the `topics` pgTable (lines ~318-326), removing the `@deprecated` JSDoc with them.
- Modify: `src/lib/session-topics.ts::getSessionTopics` (lines 32-69) — drop `lessonContent: topics.lessonContent` and `starterCode: topics.starterCode` from the select projection. Also strip the comment block at lines 33-37 that refers to "the legacy lessonContent/starterCode fields."
- Modify: `src/lib/topics.ts` — delete the deprecated-fields comment block at lines 5-7 (the columns are gone, the comment serves no purpose).
- Modify: `src/app/api/courses/[id]/topics/route.ts` — keep `.strict()` on the createSchema (it's the unknown-field reject mechanism). Strip the comment at lines 8-11 referencing plan 044 phase 3 (no longer relevant once the field doesn't exist).
- Modify: `src/app/api/courses/[id]/topics/[topicId]/route.ts` — same: keep `.strict()`, strip the deprecation comment.
- Delete: `src/lib/lesson-content.ts` — orphaned helper module. **Before deleting**, run `git grep -l "from.*lesson-content\|import.*lesson-content"` and confirm zero hits outside the files in this delete list.
- Delete: `src/components/lesson/lesson-renderer.tsx` — orphaned component. **Before deleting**, run `git grep -l "lesson-renderer\|LessonRenderer"` and confirm zero hits in `src/app/`, `src/components/` (outside the file itself), `e2e/`.
- Delete: `src/components/lesson/lesson-editor.tsx` — orphaned component. **Before deleting**, run `git grep -l "lesson-editor\|LessonEditor"` and confirm zero hits in `src/app/`, `src/components/`, `e2e/`.
- Delete: `tests/unit/lesson-content.test.ts` — its target module is gone. (The file already had pre-existing TS errors per plan 044's verification report, so this also cleans up that lint debt.)

**Verification:** `node_modules/.bin/tsc --noEmit` clean for plan-046 surface; `node_modules/.bin/vitest run` passes (462 - 4 deleted tests later in Phase 3 + 1 deleted lesson-content.test.ts ≈ 457 + 4 new in Phase 3).

## Phase 2: Go code cleanup

**Files:**

- Modify: `platform/internal/store/topics.go`:
  - Remove `LessonContent` and `StarterCode` from `Topic`, `CreateTopicInput`, `UpdateTopicInput` structs.
  - Remove `lesson_content`, `starter_code` from the `topicColumns` constant.
  - Strip the `lessonContent` default-to-`{}` block in `CreateTopic` (was a guard for old callers; now the field doesn't exist).
  - Strip the SET-clause builders in `UpdateTopic` for the two columns.
- Modify: `platform/internal/store/sessions.go`:
  - `SessionTopicWithDetails` (lines 66-83): remove `LessonContent string` and `StarterCode *string` fields. Remove the comment block at lines 71-74.
  - `GetSessionTopics` (lines 568-606): drop `t.lesson_content` and `t.starter_code` from the SELECT, drop the corresponding `&t.LessonContent, &t.StarterCode` scan args.
- Modify: `platform/internal/handlers/sessions.go`:
  - `teacherPageTopicRef` (lines 1101-1111): remove `LessonContent string` field. Remove the comment block at lines 1104-1106.
  - The mapping at lines 1235-1239: drop `LessonContent: t.LessonContent`.
- Modify: `platform/internal/handlers/topics.go::CreateTopic` and `UpdateTopic`:
  - Remove the explicit-400 rejection blocks at lines 80-82 and 193-195 (the field-specific checks for `lessonContent` and `starterCode`).
  - Switch the request decoder from `json.NewDecoder(r.Body).Decode(&body)` (or whatever helper is used) to one that calls `dec.DisallowUnknownFields()` first. **If the project has a shared `decodeJSON` helper** (which `platform/internal/handlers/topics.go::LinkUnit` uses), audit that helper:
    - If it already uses DisallowUnknownFields, no change needed.
    - If it doesn't, add a new `decodeJSONStrict` helper with DisallowUnknownFields and use it for CreateTopic + UpdateTopic. Don't flip the global helper — that's a project-wide contract change beyond plan 046's scope.
- Modify: `platform/internal/store/courses.go::CloneCourse` — already cleaned in plan 044 phase 4 + post-impl. Verify nothing remains; no change expected.
- Modify: `platform/internal/store/topics_test.go` and `platform/internal/store/courses_test.go`:
  - Strip any test setup that inserts `lesson_content` / `starter_code` directly via SQL.
  - The plan 044 phase 4 clone regression test (asserts cloned topic has empty lesson_content) becomes moot since the column is gone — convert to "cloned topic has only title/description/sort_order columns set."
- Modify: `platform/internal/store/teaching_units_test.go` — search for `lesson_content` / `starter_code` references in test setup (Codex flagged this surface as "may reference"), strip if present.

**Verification:** `cd platform && DATABASE_URL=... go test ./... -count=1` passes. The deletion of `topics_deprecation_test.go` happens in Phase 3, so this phase's test count is "all tests minus the deleted lesson_content.test.ts" plus no additions.

## Phase 3: Replacement tests (strict mode, unknown-field rejection)

**Files:**

- Delete: `platform/internal/handlers/topics_deprecation_test.go` — its 6 tests asserted "POST/PATCH rejects lessonContent/starterCode." With the columns gone and the explicit reject blocks removed, this contract is now expressed differently (DisallowUnknownFields).
- Modify: `tests/integration/courses-api.test.ts` — delete the four "rejects POST/PATCH with deprecated lessonContent/starterCode" tests added in plan 044 phase 3 (lines ~199-257).
- Add: `platform/internal/handlers/topics_strict_decode_test.go` (new) — 2 tests:
  - `TestCreateTopic_RejectsUnknownField`: POST with `{"title":"X","lessonContent":{"blocks":[]}}` returns 400.
  - `TestUpdateTopic_RejectsUnknownField`: PATCH with `{"lessonContent":{"blocks":[]}}` returns 400.
  - Use `lessonContent` as the canary field name so future readers grepping for the deprecated name see the contract is enforced.
- Add to `tests/integration/courses-api.test.ts`: 2 new tests:
  - "POST topic with unknown lessonContent field is rejected (400)"
  - "PATCH topic with unknown lessonContent field is rejected (400)"

Net test delta: -10 (6 Go + 4 TS deprecation tests deleted) + 4 (2 Go + 2 TS unknown-field tests added) = -6 total.

**Verification:** `bun run test` and `cd platform && go test ./...` both pass.

## Phase 4: Migration applied

**Files:**

- Create: `drizzle/0022_drop_topic_lesson_content_and_starter_code.sql`

```sql
-- Plan 046: drop the deprecated topic columns one release after plan 044
-- locked the writes and removed all reads. Pre-plan Phase 0 discovery
-- confirmed no topic has non-empty lesson_content / starter_code, every
-- topic with teaching material has a linked teaching_unit (via topic_id),
-- and no view / trigger / index depends on these columns.
--
-- Plain DROP COLUMN (not IF EXISTS): if the column doesn't exist, that's
-- a state we want to be loud about — it means a previous run partially
-- succeeded or the schema drifted, both of which deserve manual review.
ALTER TABLE topics DROP COLUMN lesson_content;
ALTER TABLE topics DROP COLUMN starter_code;
```

**Why hand-written SQL** (not generated via drizzle-kit): per `TODO.md` line 10, the project's drizzle-kit migrate tooling has known issues, so migrations are applied via `psql -f`. README.md line 61-62 still advertises `bun run db:migrate` (drizzle-kit) as the migration command — that's a documentation mismatch flagged elsewhere (TODO.md), not something plan 046 fixes.

**Apply order (operator runbook):**
1. Merge plan 046's PR (which carries the code changes from Phases 1-3 and the migration file from Phase 4).
2. Deploy the new application code to dev/staging.
3. Run `psql $DATABASE_URL -f drizzle/0022_drop_topic_lesson_content_and_starter_code.sql` against dev. Verify with `\d topics` that the columns are gone.
4. Smoke-test the topic editor + student class detail + teacher session page in dev.
5. Repeat for any further environments.

The reason migration runs AFTER code deploy: the new code no longer SELECTs/scans these columns, so dropping them won't crash anything. If the migration ran first while old code was still running, every active session-topic query would crash on `column "lesson_content" does not exist`. Standard "code first, then drop" pattern.

**Verification:** Local dev DB column drop succeeds; full Vitest + Go suite passes against the post-drop schema.

## Phase 5: Docs + TODO

**Files:**

- Modify: `TODO.md` — remove the "Plan 046: Drop deprecated topic columns" entry.
- Modify: `docs/plans/044-topic-syllabus-refactor.md` — append a one-line note to the Post-Execution Report: "Plan 046 dropped the columns on YYYY-MM-DD."
- (Optional) `README.md` line 61-62 — leave the drizzle-kit reference alone; it's a separate cleanup tracked in TODO.md.

## Risk Surface

**Database safety:**
- Dropping columns is irreversible without a backup restore. Phase 0 discovery is the gate. If any of Q1, Q2, Q3 returns rows, **stop**.
- `DROP COLUMN` (without IF EXISTS) is intentional — see migration comment.
- Postgres `DROP COLUMN` is metadata-only (no table rewrite); fast and lock-light.

**Deploy ordering** (corrected per Codex CRITICAL):
- Plan ships code FIRST, migration LAST (Phases 1-3 strip reads, Phase 4 drops columns). The IMPLEMENTATION ORDER in this plan now reflects deploy mechanics, not the inverted "migration first" wording from the original draft.
- If a future operator runs the migration before deploying the new code, every session-topic SELECT crashes. Operator runbook in Phase 4 spells this out.

**Code orphans:**
- `lesson-content.ts`, `lesson-renderer.tsx`, `lesson-editor.tsx` deletion is reversible via git but each gets an explicit "grep before delete" gate in Phase 1.

**Test churn:**
- Net -6 tests, but the contract is preserved (and arguably tightened) by switching from "deprecated fields rejected" to "any unknown field rejected" in the strict-decode tests.

**Strict mode change on Go side:**
- The `DisallowUnknownFields` switch on Go topic endpoints is a tighter contract than today. Stale clients that send extra fields will newly receive 400 instead of silent strip. This is intentional — symmetric with TS `.strict()` — but it's a real API surface change. Document in the post-execution report so the Go client side knows.

## Out-of-Scope Acknowledgements

- Plan 047 — Topic rename + multi-Unit-per-Topic.
- README.md vs `bun run db:migrate` mismatch — separate cleanup.

## Codex Pre-Implementation Review

- **Date:** 2026-04-27
- **Verdict:** Plan needed substantive rewrite; this document IS the post-correction plan.

### Corrections applied

1. `[CRITICAL]` **Deploy order contradiction.** Original draft put "migration applied to dev" as Phase 1 but Risk Surface said "code first, then migration." → Implementation Order rewritten end-to-end so code phases (1, 2, 3) precede the migration (Phase 4). Plan now matches the deploy mechanics it advocates.
2. `[IMPORTANT]` **Discovery queries count-only.** → Phase 0 now lists rows for Q1 and Q2 (not just counts), and adds Q3 (pg_depend dependency check). Stop conditions explicit per query.
3. `[IMPORTANT]` **Strict mode inconsistency.** Original draft said "silently ignored" but TS schemas are `.strict()`. → Plan keeps `.strict()` on TS side, adds `DisallowUnknownFields` on Go side for symmetric contract. Replacement tests assert 400 on unknown field, not silent strip.
4. `[IMPORTANT]` **Replacement tests too thin.** → 4 replacement tests instead of 2: TS POST + PATCH, Go CreateTopic + UpdateTopic. Net -6 tests overall (delete 10 deprecation, add 4 strict-mode).
5. `[IMPORTANT]` **`src/lib/session-topics.ts` not in plan.** → Added to Phase 1 with line refs (32-69 SELECT projection).
6. `[IMPORTANT]` **Go cleanup misses `SessionTopicWithDetails`, `teacherPageTopicRef`, mapping at sessions.go:1235-1239, and the function name was wrong (`SelectTopicsForSession` → `GetSessionTopics`).** → All four corrections folded into Phase 2.
7. `[IMPORTANT]` **`DROP COLUMN IF EXISTS` masks bad baseline.** → Switched to plain `DROP COLUMN` with comment explaining why being loud about a missing column is the right default. Pre-drop pg_depend dependency query added to Phase 0 Q3.
8. `[IMPORTANT]` **One-release safety net unverifiable.** → Status section now explicitly waives the safety-net rule on the grounds that Bridge is pre-production. The rule re-engages once Bridge enters pilot.
9. `[MINOR]` **Grep-before-delete only mentioned for renderer/editor, not lesson-content.ts.** → All three files have explicit grep gates in Phase 1.
10. `[MINOR]` **Hand-written SQL vs README's `bun run db:migrate` mismatch.** → Phase 4 includes a one-paragraph note explaining the choice with reference to TODO.md.

---

## Post-Execution Report

**Date:** 2026-04-28
**Branch:** `feat/046-drop-deprecated-topic-columns` (chained off `feat/045-unit-picker`)
**Phase 0 discovery:** Q1, Q2, Q3 all returned 0 rows on dev — green light to drop.

### What shipped

- Migration `drizzle/0022_drop_topic_lesson_content_and_starter_code.sql` applied to dev + bridge_test. Plain `DROP COLUMN` (not `IF EXISTS`); `\d topics` confirms columns are gone.
- Drizzle schema (`src/lib/db/schema.ts`): `lessonContent` + `starterCode` removed from the `topics` pgTable.
- `src/lib/session-topics.ts::getSessionTopics` SELECT no longer projects the columns.
- `src/lib/topics.ts`: deprecated-fields comment removed.
- API route comments updated; `.strict()` zod stays as the unknown-field reject mechanism.
- Orphan helpers deleted (verified by grep beforehand): `src/lib/lesson-content.ts`, `src/components/lesson/lesson-renderer.tsx`, `src/components/lesson/lesson-editor.tsx`, `tests/unit/lesson-content.test.ts`. Empty `src/components/lesson/` directory removed.
- Go store (`platform/internal/store/topics.go`): `Topic`, `CreateTopicInput`, `UpdateTopicInput`, `topicColumns`, `scanTopic`, `CreateTopic`, `UpdateTopic`, `ListTopicsByCourse` all stripped of the column fields.
- Go store (`platform/internal/store/sessions.go`): `SessionTopicWithDetails` lost `LessonContent` / `StarterCode`; `GetSessionTopics` SQL + scan no longer reference the columns.
- Go store (`platform/internal/store/courses.go::CloneCourse`): comment updated.
- Go handler (`platform/internal/handlers/sessions.go`): `teacherPageTopicRef` lost `LessonContent`; mapping at `GetTeacherPage` no longer copies it.
- Go handler (`platform/internal/handlers/topics.go`): explicit-400 deprecation rejects removed; CreateTopic + UpdateTopic now use new `decodeJSONStrict` helper (`platform/internal/handlers/helpers.go`) which calls `dec.DisallowUnknownFields()` — symmetric with TS-side `.strict()`. Stale clients sending unknown fields get a clean 400 instead of silent strip.
- Test cleanup: `platform/internal/store/courses_test.go` clone test rewritten (no longer references dropped columns); `platform/internal/store/teaching_units_test.go` and `platform/internal/handlers/teaching_units_integration_test.go` setup INSERT statements no longer reference `lesson_content`.
- Replacement strict-mode tests (4 total): 2 in `platform/internal/handlers/topics_strict_decode_test.go` (CreateTopic + UpdateTopic reject unknown field), 2 in `tests/integration/courses-api.test.ts` (POST + PATCH reject unknown field). Old deprecation rejection tests (6 Go in `topics_deprecation_test.go`, deleted; 4 TS in `courses-api.test.ts`, deleted/replaced) gone.
- TODO.md entry removed; plan 044 post-exec report annotated with "Shipped 2026-04-28."

### Verification

- Phase 0 discovery: all three queries 0 rows.
- Migration: `\d topics` confirms `lesson_content` and `starter_code` no longer exist; only `id, course_id, title, description, sort_order, created_at, updated_at` remain.
- Vitest: **460 passed | 11 skipped** (was 462 pre-046; net -2 = 4 deprecation tests deleted + 2 strict-mode replacements added; the lesson-content.test.ts had been removed in plan 045).
- Go: all packages green post-drop. Strict-decode tests pass.

### Net test delta

- TS: -4 deprecation tests + 2 strict-mode tests = -2
- Go: -6 deprecation tests + 4 strict-mode tests (2 reject + 2 happy-path sanity) = -2
- Total: -4 net (the contract is preserved and arguably tightened — strict mode now rejects ANY unknown field, not just the two deprecated ones).

### Risk realized

- **None**. Phase 0 caught no stragglers, the migration was metadata-only, and the test suites pass against the post-drop schema.

### Codex post-impl review

- **Date:** 2026-04-28
- **Verdict:** NEEDS FIXES — 1 IMPORTANT. Fixed inline (commit `ad74034`).

#### Findings + resolutions

1. `[IMPORTANT]` **TS `TeacherPagePayload.courseTopics` still declared `lessonContent: string`** (`src/app/(portal)/teacher/sessions/[sessionId]/page.tsx:24-30`). The Go handler's `teacherPageTopicRef` no longer emits `lessonContent` (stripped in this plan's Phase 2), but the TS type still declared the field as required. At runtime it would always be `undefined`, which TS would not catch. → Removed `lessonContent: string` from the TS payload type. Also cleaned up adjacent stale "Plan 044 phase 2: lessonContent kept as deprecated" comments in `src/components/session/teacher/teacher-dashboard.tsx`, `src/components/parent/live-session-viewer.tsx`, and `src/app/(portal)/student/classes/[id]/page.tsx`.

#### Confirmations (PASS)

- Migration safety: plain `DROP COLUMN` (not `IF EXISTS`); pg_depend confirmed no view/trigger/index dependencies.
- Deploy order: plan correctly documents code-first, migration-last.
- Strict mode: TS `.strict()` zod intact; Go `decodeJSONStrict` calls `DisallowUnknownFields()`. Both `CreateTopic` and `UpdateTopic` use it.
- Orphan deletion: all four files deleted; grep confirms zero remaining `LessonRenderer` / `LessonEditor` / `lesson-content` runtime references outside historical plan docs.
- Test deletion + replacement: `topics_deprecation_test.go` gone; 4 deprecation TS tests removed; 4 strict-mode replacement tests added (2 reject + 2 happy-path Go in `topics_strict_decode_test.go`; 2 reject TS in `courses-api.test.ts`).
- Go store integrity: `topicColumns` and `scanTopic` aligned at 7 columns; `GetSessionTopics` SELECT/Scan column order matches.

No MINOR findings.

### Out-of-scope deferrals

- **Plan 045** still in progress (PR #74 open). No interaction with plan 046 — they're independent surfaces.
- **README.md vs `bun run db:migrate` mismatch** — separate cleanup, tracked in TODO.md.
- **Plan 047** — Topic rename + multi-Unit-per-Topic exploration.
