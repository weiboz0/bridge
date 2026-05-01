# Plan 054 — Schema drift cleanup (P1)

## Status

- **Date:** 2026-05-01 (re-scoped from "Drop stale `documents.classroom_id`" to absorb every shipped-code-vs-schema drift finding from the deep review)
- **Severity:** P1 (each item is silent until something triggers it: `bun run db:generate`, a teacher creating a unit, a `drizzle-kit push`)
- **Origin:** Reviews `008-...:25-31`, `009-...:49-55` (documents.classroom_id), plus `009-2026-04-30.md` §P1-8 (`createUnit` drops `materialType`), §P1-12 (Drizzle `teaching_units` missing 3 indexes).

## Problem

Three independent code-vs-schema drifts. Each is small but each silently breaks something:

### 1. Drizzle schema declares `documents.classroom_id` after migration 0012 dropped it

Migration `drizzle/0012_drop_legacy_classrooms.sql:47-49` runs `ALTER TABLE documents DROP COLUMN IF EXISTS classroom_id` and drops the index. The Drizzle schema at `src/lib/db/schema.ts:405-427` still declares the field and index. `platform/internal/store/documents.go:98-101` resolves nav via `LEFT JOIN sessions` instead — the Go code is correct.

Failure modes:
- `bun run db:generate` regenerates the column on the next change to `documents`.
- Any Drizzle query like `db.select({ classroomId: documents.classroomId })` against a migrated DB returns column-doesn't-exist.
- Type-narrowing gives consumers a phantom column in their intellisense.

### 2. Drizzle `teaching_units` missing three live indexes (009-2026-04-30 §P1-12)

The SQL migrations created three indexes that the Drizzle declaration omits:

| Live index | Created by | In Drizzle schema? |
|---|---|---|
| `teaching_units_scope_slug_uniq` (partial unique on `(scope, scope_id, slug) WHERE slug IS NOT NULL`) | `drizzle/0016_teaching_units.sql:38-39` | ❌ |
| `teaching_units_topic_id_uniq` (partial unique on `(topic_id) WHERE topic_id IS NOT NULL`) | `drizzle/0017_topic_to_units.sql:15-16` | ❌ |
| `teaching_units_search_idx` (GIN on `search_vector`) | `drizzle/0019_discovery.sql:35-37` | ❌ |

`platform/internal/handlers/teaching_units.go:324` relies on `topic_id_uniq` to detect duplicate topic links — the 1:1 invariant from plan 044. A `drizzle-kit push` against the migrated DB would silently drop all three. The 1:1 invariant disappears; concurrent picker actions can race; slug uniqueness is gone.

### 3. `createUnit` silently drops `materialType` from the POST body (009-2026-04-30 §P1-8)

`src/lib/teaching-units.ts:63-82` defines `CreateUnitInput` with `materialType?: "notes" | "slides" | "worksheet" | "reference"` but the function's body does NOT include `materialType` in the `JSON.stringify` payload sent to `POST /api/units`. Created units always receive the backend default (`notes`) regardless of what the caller passes. The Unit creation form's material-type picker is decorative.

## Out of scope

- Auditing every schema column for drift — too broad for this plan. The three findings above are the documented ones; a broader drift checker is a future plan if/when needed.
- Plan 058's `scheduled_sessions` parity (already shipped).
- Drizzle's broken `db:migrate` for migrations beyond 0002 — different problem, tracked in `TODO.md`.

## Approach

Three independent fixes; one commit each on the same branch.

### Drift 1: drop `documents.classroom_id` from schema

- Delete `classroomId` field and `documents_classroom_idx` from `src/lib/db/schema.ts`.
- `grep -rn "classroomId\b\|classroom_id\b" src/ tests/` — update or delete each hit.
- Update the JSDoc on `documents` to remove the `@deprecated` notice for the column.

### Drift 2: declare the three missing `teaching_units` indexes in Drizzle

- Add `uniqueIndex("teaching_units_scope_slug_uniq").on(...).where(sql\`slug IS NOT NULL\`)` for the partial unique on `(scope, scope_id, slug)`.
- Add `uniqueIndex("teaching_units_topic_id_uniq").on(...).where(sql\`topic_id IS NOT NULL\`)` for the 1:1 invariant.
- Add the GIN index on `search_vector` (Drizzle: `index("teaching_units_search_idx").using("gin", ...)`). Confirm syntax against a Drizzle GIN example elsewhere in the codebase.
- Verify the live indexes' exact column lists via `psql \d teaching_units` and pin them.

### Drift 3: include `materialType` in `createUnit` payload

- Single-line addition: `body: JSON.stringify({ ..., materialType: data.materialType ?? "notes" })`.
- Add: `tests/unit/teaching-units-client.test.ts` — assert the request body includes `materialType` when set.

## Files

**Drift 1:**
- Modify: `src/lib/db/schema.ts` — drop `classroomId` field and `documents_classroom_idx`.
- Maybe modify: any test/fixture that imports the deprecated field.

**Drift 2:**
- Modify: `src/lib/db/schema.ts::teachingUnits` — add the three missing indexes.

**Drift 3:**
- Modify: `src/lib/teaching-units.ts:63-82` — include `materialType` in payload.
- Add: `tests/unit/teaching-units-client.test.ts` — request-body assertion.

**Regression test (covers all three):**
- Add: `tests/integration/schema-drift.test.ts` — for each known drifted table, query the live DB and assert (a) the column set matches the Drizzle declaration, (b) every index in the migrations exists in the live DB. Models the same shape as the plan-058 `schema-scheduled-sessions.test.ts`.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| A test or fixture still imports `documents.classroomId` | medium | grep step before deletion. |
| Drizzle's GIN index DSL is unfamiliar | medium | Consult the Drizzle docs + an existing GIN example in the codebase before authoring; verify with `\d teaching_units` after a no-op `bun run db:generate` against the migrated DB. |
| `materialType` change affects the create-unit form's UX | low | The form already has the picker; the backend already accepts the field; this just re-connects them. Smoke-test by creating a worksheet unit and confirming the row's `material_type` column is `worksheet`. |
| `bun run db:generate` emits an unexpected migration after the schema changes | medium | Run it; if it emits anything against `documents` or `teaching_units`, that's evidence of additional drift to triage and fix in this plan. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch on this plan + the grep results for both `classroomId` and `materialType`. Capture verdict.

### Phase 1: three drift-fix commits + regression test

- Commit 1: drift 1 (drop `documents.classroom_id`).
- Commit 2: drift 2 (add `teaching_units` indexes).
- Commit 3: drift 3 (`createUnit` materialType).
- Commit 4: regression test (schema-drift.test.ts).
- Run `bun run test` + `cd platform && go test ./...` after each.
- Self-review.

### Phase 2: post-impl Codex review

Dispatch on the diff before merge. Resolve findings.

## Codex Review of This Plan

(Filled in after Phase 0.)
