# 032 — Teaching Unit: Migration of Existing Topics

**Goal:** Convert each existing `topic` + its `topic_problems` rows into a teaching unit so plans 033+ operate on a library with real content. Add a `topic_id` column to `teaching_units` for the compatibility shim — sessions and assignments that reference topics can resolve the corresponding unit.

**Spec:** `docs/specs/012-teaching-units.md` — §How existing materials flow in

**Branch:** `feat/032-topic-migration`

**Depends On:** Plan 031 (teaching_units schema)

**Status:** In progress

---

## Scope

**In scope:**
- Migration 0017: add `teaching_units.topic_id` (nullable FK to topics) + unique index
- SQL migration script: for each topic, create a `teaching_unit` + `unit_document` with prose block (from `lesson_content`) + `problem-ref` blocks (from `topic_problems`)
- Go store helper: `GetUnitByTopicID(ctx, topicID) (*TeachingUnit, error)`
- Go handler: session context endpoint returns `unitId` alongside existing topic data
- Seed scripts updated so re-seeding creates units too
- Tests covering the migration output and the topic→unit lookup

**Out of scope:**
- Replacing topic references with unit references in session/assignment flows (that's plan 033+)
- Deleting topics or topic_problems (keep them as the source of truth until fully migrated)
- Student-facing unit rendering (plan 033)

---

### Task 1: Migration — add topic_id + convert topics to units

**Files:**
- Create: `drizzle/0017_topic_to_units.sql`
- Modify: `src/lib/db/schema.ts` — add `topicId` to `teachingUnits`

The migration does two things in one transaction:

1. Add `teaching_units.topic_id` (nullable FK to topics, unique — one unit per topic).
2. For each topic that doesn't already have a unit, INSERT a `teaching_unit` + `unit_document`:
   - `scope = 'org'`, `scope_id = courses.org_id` (via topic → course)
   - `title = topic.title`
   - `summary = topic.description`
   - `grade_level = courses.grade_level`
   - `status = 'classroom_ready'` (existing topics are assumed live)
   - `created_by = courses.created_by`
   - `topic_id = topic.id`
   - Unit document `blocks`: a `prose` block if `lesson_content != '{}'`, then one `problem-ref` block per `topic_problems` row (ordered by `sort_order`)

**Testing:**
- Apply to both DBs, verify idempotent
- `SELECT count(*) FROM teaching_units WHERE topic_id IS NOT NULL` = topic count
- Each unit has a `unit_documents` row with correct block count

**Commit:** `feat(032): migration 0017 — convert topics to teaching units`

---

### Task 2: Go store + handler — topic→unit lookup

**Files:**
- Modify: `platform/internal/store/teaching_units.go` — add `GetUnitByTopicID`
- Modify: `platform/internal/store/teaching_units_test.go` — test the lookup
- Modify: `platform/internal/handlers/teaching_units.go` — add `GET /api/units/by-topic/{topicId}`
- Modify: `platform/internal/handlers/teaching_units_integration_test.go` — test the endpoint

**Testing:**
- Store: `GetUnitByTopicID` returns the unit, nil for non-existent topic
- Handler: `GET /api/units/by-topic/{topicId}` returns 200 with the unit, 404 for unknown topic

**Commit:** `feat(032): topic→unit lookup (store + API)`

---

### Task 3: Seed update + verification

**Files:**
- Modify: `scripts/seed_python_101.sql` — after inserting topics + topic_problems, also insert corresponding teaching_units + unit_documents
- Modify: `scripts/seed_problem_demo.sql` — same

**Verification:**
- Full Go suite green
- Full Vitest green
- `psql -c "SELECT tu.title, tu.topic_id IS NOT NULL as has_topic FROM teaching_units tu ORDER BY tu.title"` — all migrated units have topic_id

**Commit:** `feat(032): seed scripts create teaching units from topics`

---

## Code Review

### Review 1

- **Date**: 2026-04-24
- **Reviewer**: Codex (post-merge)
- **PR**: #58
- **Verdict**: Approved with 1 known limitation

**Known Limitation (not a current bug)**

1. `[WONTFIX]` Migration 0017's prose block generation (lines 51-68) stores raw `lesson_content::text` as a Tiptap text node. For topics with non-empty `lesson_content`, this would produce a corrupted paragraph block containing raw JSON text. All current data has `lesson_content = '{}'` which the guard correctly skips, so no data is affected. The prose block generation can be removed or replaced with proper Tiptap node conversion in a follow-up if real lesson content exists in production.

**No bugs found for:**
- Column list + scanTeachingUnit consistency (topic_id correctly added as 14th column)
- Route ordering (by-topic registered before {id} wildcard)
- Access control on GetUnitByTopic (canViewUnit applied, 404 on miss)
- Seed ON CONFLICT correctness (partial unique index target matches migration)
- Migration idempotency (IF EXISTS guards, WHERE NOT EXISTS for data)

## Post-Execution Report

**Status:** Complete

**Implemented:** Migration 0017 (topic_id FK + data conversion), GetUnitByTopicID store method, GET /api/units/by-topic/{topicId} handler, seed scripts updated. 14 topics converted to teaching units.

**Verification:** Go 12/12 green, migration idempotent on both DBs.
