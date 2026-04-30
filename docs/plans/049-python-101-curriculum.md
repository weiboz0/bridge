# Plan 049 — Python 101: Library Content + Bridge-HQ Course

## Status

- **Date:** 2026-04-30
- **Branch:** `feat/049-python-101-curriculum`
- **Goal:** Build a **platform-scope library** of 12 teaching units and ~60 problems for introductory Python (Option B per user 2026-04-30), then create a Bridge-HQ-org-scoped Python 101 course as the canonical arrangement. Other orgs adopt via the existing `CloneCourse` flow. Replace the legacy seed at `scripts/seed_python_101.sql` (broken since plan 046 dropped `topics.lesson_content`).

## Sources

- Existing legacy seed: `scripts/seed_python_101.sql` (6 topics, 12 problems, ~50 test cases). Broken on current main.
- Existing problem-bank seed: `scripts/seed_problem_demo.sql`. Independent of Python 101 but wires demo problems to demo classes.
- Platform-scope content infrastructure: `teaching_units` (plan 044), `problems` + `test_cases` + `problem_solutions` (plan 028), Unit picker (plan 045), `topic_problems` M2M.
- Existing test runner: `platform/internal/sandbox/piston.go::PistonClient.ExecuteWithStdin` — the SERVER-SIDE grading path. Students "Run" code via Pyodide-in-browser (`src/lib/pyodide/`); students "Test" submissions go to the server which runs Piston/CPython.
- Pre-existing drift documentation: `docs/design-gaps-problem-workflow.md` already calls out the Pyodide↔Piston runtime drift.
- User decision 2026-04-30: **Option B — Unit-primary library**.

## Framing

The data model has two distinct visibility scopes for content:

| Entity | Has `scope` field? | This plan's choice |
|---|---|---|
| `teaching_units` | YES (`platform / org / personal`) | **`scope = 'platform'`** — library content visible to all |
| `problems` | YES (`platform / org / personal`) | **`scope = 'platform'`** — library content; `topic_problems` is M2M so problems can attach to many topics |
| `problem_solutions` | NO | Inherits from problem (FK cascade) |
| `test_cases` | NO | Inherits from problem |
| `courses` | **NO scope, requires `org_id` NOT NULL** | **Created in a new "Bridge HQ" org** (Codex CRITICAL #3) |
| `topics` | NO scope | Owned by the course → Bridge HQ org |

**There is no such thing as a platform-scope course in the current schema.** Courses are always org-scoped. To ship a "canonical Python 101 course" we need an organization to own it. The cleanest path is a new system organization called "Bridge HQ" (or similar) owned by a system user. Other orgs adopt the course via the existing `CloneCourse` flow.

**What about cross-org library reuse?** With the 1:1 unit↔topic invariant from plan 044, a teaching_unit linked to Bridge HQ's Python 101 topic cannot ALSO be linked to another org's topic. The plan 045 picker would surface the unit but show it as "Already linked." True cross-course unit reuse requires the unit fork/overlay mechanics (plan 033/034) which are NOT yet implemented (Codex IMPORTANT #5).

This plan ships:
- Platform-scope teaching_units that ARE discoverable in the picker but get claimed by Python 101's topics.
- Platform-scope problems that ARE freely reusable via the M2M `topic_problems` (a problem CAN appear in multiple topics across multiple courses).
- A Bridge HQ Python 101 course that claims those units.
- The `CloneCourse` flow for adoption (existing). The cloned course gets new topic rows but NO unit links (per plan 044 phase 4) — adopting teachers re-link via the picker, which finds the (claimed) library units. So adoption today means clone-and-author-your-own-units. Plan 050+ can build fork/overlay.

## Runtime: Piston for grading, NOT Pyodide

Codex CRITICAL #1: my prior draft falsely claimed "the importer runs reference solutions through the same executor the student-facing UI uses." Wrong. Two execution paths exist:

| Path | Where | Used by |
|---|---|---|
| **Pyodide** | Browser Web Worker (`src/lib/pyodide/use-pyodide.ts`) | Student "Run" button (instant feedback, no test cases) |
| **Piston (CPython)** | Server, called from the Go API (`platform/internal/sandbox/piston.go`) | Student "Test" submissions, attempt grading |

The importer's job is to verify reference solutions pass their test cases. The grading runtime is Piston. So:

- **The importer calls Piston.** Either via a thin HTTP endpoint that wraps `PistonClient.ExecuteWithStdin`, or via a Go-binary CLI invocation that the TS importer shells out to.
- **Authors must avoid Pyodide-only behaviors** in problems. Concretely: no `random` (or seed it), no clock-dependent output, no Pyodide-specific Python (no `js` module imports, no DOM access). Document the floating-point formatting drift if any tests print floats.
- **Phase 0 confirms** the importer can hit the Piston endpoint, with credentials, against the running Go service. If it can't (e.g., Piston isn't accessible from the importer's network), the plan adds a new authenticated import-time verification endpoint.

## Decisions to confirm before Phase 1 (Phase 0)

These steer everything downstream. User confirms each before authoring content.

1. **Grade band.** Recommend **6-8** primary, with notes on K-5 simplifications and 9-12 extensions. Default: 6-8.
2. **Total problem count.** Recommend **5-7 problems per focus area × 12 focus areas = 60-84 problems**. Default: 60.
3. **Authoring format.** **YAML** files in `content/python-101/`, with three subdirs: `units/`, `problems/`, plus `course.yaml`.
4. **AI-assisted authoring.** Default: **OUT of scope for this plan** (Codex IMPORTANT #6). The platform's `/api/units/{id}/draft-with-ai` endpoint requires an existing unit and only drafts blocks; there's no general "draft a problem" API. Authoring is hand-driven (humans, possibly assisted by Claude in chat session, then committed). Plan 050+ can build a proper authoring AI.
5. **Legacy seed disposition.** Default: **delete `scripts/seed_python_101.sql`** in Phase 6. Audit `seed_problem_demo.sql` separately — it may still be useful for problem-bank-only demos.
6. **Course visibility.** Default: `is_published = true` on the Bridge HQ-owned Python 101 course.
7. **Demo data integration.** Default: **the existing demo class** (`eve@demo.edu`'s class) gets re-pointed to clone the new Python 101 course (one CloneCourse call in the demo seed) so the demo experience walks through real curriculum. Bridge HQ's original Python 101 stays untouched as the canonical reference.
8. **Bridge HQ org + system user identity.** Recommend creating a new org `Bridge HQ` (slug `bridge-hq`) and a system user `system@bridge.platform` with `is_platform_admin = true`. Both seeded in Phase 0 as a one-time bootstrap. Their UUIDs become well-known constants the importer references.

## Library structure (proposed)

12 platform-scope teaching units; the Bridge HQ course's 12 topics each link to one of these in this order:

| # | Unit slug | Concepts | Sample problems |
|---|---|---|---|
| 1 | `print-and-comments` | `print()`, string literals, `#` comments | Print your name; print a multi-line poem; comment-out a buggy line |
| 2 | `variables-and-types` | `int`, `float`, `str`, `bool`, assignment, `type()` | Seconds in a day; F to C; format a greeting |
| 3 | `arithmetic-and-operators` | `+ - * / // % **`, precedence, casts | Circle area; tip calculator; modulo |
| 4 | `strings` | indexing, slicing, methods, f-strings | Count vowels; reverse a string; censor a word |
| 5 | `conditionals` | `if/elif/else`, comparisons, booleans | FizzBuzz; pass/fail grader; leap year |
| 6 | `loops` | `for ... in range()`, `while`, `break`, `continue` | Sum 1..N; multiplication table; first power of 2 above N |
| 7 | `lists` | `[]`, indexing, slicing, methods, `in`, list comp (intro) | Average a list; remove duplicates; running max |
| 8 | `dicts-and-sets` | `{}`, key/value, set ops | Word count; unique vowels; gradebook lookup |
| 9 | `functions` | `def`, parameters, return, `*args`, scope | `factorial(n)`; `is_prime(n)`; `palindrome(s)` |
| 10 | `files-and-exceptions` | `open()`, `try/except`, `with` | Count lines; CSV row sum; safe int parsing |
| 11 | `classes-and-objects` | `class`, `__init__`, methods, `__str__` | `Counter` class; `Rectangle` area; `Bank` deposit/withdraw |
| 12 | `capstone` | mixed concepts | Hangman (subset); shopping cart; tiny TODO CLI |

Each teaching_unit follows: **Big idea → Worked examples → Try it → Common pitfalls → Vocabulary**. The "Try it" section embeds 1-2 problems via `problem-ref` blocks.

## Importer DB-field mapping (Codex IMPORTANT #5)

Every required DB field, with the constant value the importer uses:

```
teaching_units:
  id           = uuidv5("python-101:units:" + slug, NS)
  scope        = 'platform'
  scope_id     = NULL
  status       = 'classroom_ready'
  material_type = 'notes'
  created_by   = SYSTEM_USER_ID    # set in Phase 0
  topic_id     = NULL initially; set later by LinkUnitToTopic

unit_documents:
  unit_id      = teaching_units.id
  blocks       = JSON serialization of YAML "blocks"

problems:
  id           = uuidv5("python-101:problems:" + slug, NS)
  scope        = 'platform'
  scope_id     = NULL
  status       = 'published'
  difficulty   = from YAML
  grade_level  = from YAML (or course default)
  tags         = from YAML
  created_by   = SYSTEM_USER_ID

problem_solutions:
  problem_id   = problems.id
  language     = 'python'
  code         = YAML solution
  is_published = true
  created_by   = SYSTEM_USER_ID

test_cases (table lives in drizzle/0008_problems.sql, not schema.ts):
  problem_id      = problems.id
  owner_id        = NULL (canonical)
  name            = from YAML
  stdin           = from YAML
  expected_stdout = from YAML  # exact string only — no patterns
  is_example      = from YAML  # true=visible, false=hidden
  order           = sort within problem

courses:
  id           = uuidv5("python-101:course", NS)
  org_id       = BRIDGE_HQ_ORG_ID
  created_by   = SYSTEM_USER_ID
  title        = "Python 101 — Introduction to Programming"
  grade_level  = '6-8'
  language     = 'python'
  is_published = true

topics:
  id           = uuidv5("python-101:topics:" + unitSlug, NS)
  course_id    = courses.id
  title        = unit title
  description  = unit description
  sort_order   = position in course.yaml topics list

topic_problems:
  topic_id     = topics.id
  problem_id   = problems.id
  sort_order   = position in unit's problemSlugs
  attached_by  = SYSTEM_USER_ID
```

## Test outputs: exact match only (Codex CRITICAL #2)

The platform's `test_cases` table only stores `expected_stdout` (string). The grader does normalized exact-string comparison. There is no regex matching today. **The plan drops `expectedStdoutPattern` entirely.** Authoring constraints:

- Every problem MUST have a deterministic single-correct-output. No "any non-empty line" semantics today.
- Where a "real" problem might allow multiple outputs (e.g., word-cloud ordering), the problem is rewritten to constrain the output (e.g., "print words in descending count, ties broken alphabetically").
- Authors should test their problems against this constraint during review.

## Phases

### Phase 0: Decisions + bootstrap + legacy audit + runtime check

**Files:**
- Create: `scripts/seed_bridge_hq.sql` — idempotent seed for the Bridge HQ org + system user. Hardcoded UUIDs so the importer can reference them as constants.
- Create: `docs/plans/049-python-101-legacy-extract.md` — extracted legacy problem titles + descriptions for re-use as drafts.

**Actions:**
1. User confirms the 8 decisions above.
2. Run `scripts/seed_bridge_hq.sql` against bridge dev DB. Confirm Bridge HQ org + system user exist.
3. **Runtime check:** confirm the Piston client is reachable from a TS importer context. Either:
   - The Go service exposes an authenticated `POST /api/admin/run-piston` (or similar) that wraps `PistonClient.ExecuteWithStdin`. The importer hits it with platform-admin credentials.
   - Or the importer shells out to `go run platform/cmd/run-piston ...` (a new tiny Go CLI binary).
   
   Pick one in Phase 0 and document. If the existing endpoints `/api/attempts/.../test` aren't usable for arbitrary code (they require an attempt context), build the new endpoint.
4. Audit `scripts/seed_python_101.sql` and extract the 12 problems (titles, descriptions, starter, solution, test cases) into the legacy-extract doc.
5. Audit `scripts/seed_problem_demo.sql` — does it overlap with Python 101 content? If yes, plan whether to keep, retire, or merge.

**Verification:** Decisions written into this plan under "Decisions confirmed." Bridge HQ seed runs idempotently. Piston reachable.

### Phase 1: Authoring format + Zod validator + dependencies

**Files:**
- Create: `content/python-101/README.md`
- Create: `scripts/python-101/schema.ts` — Zod schemas (no `expectedStdoutPattern`).
- Create: `scripts/python-101/validate.ts` — CLI walks the tree.
- Create: `tests/unit/python-101-schema.test.ts`.
- Modify: `package.json`:
  - Add direct deps: `uuid` (Codex IMPORTANT #10) and pin a YAML parser — recommend `yaml` (Codex MINOR #12). Pin both versions.
  - Add scripts: `content:python-101:validate`, `content:python-101:import`.

### Phase 2: Importer — transactional, with sandbox-verified solution check

**Files:**
- Create: `scripts/python-101/import.ts` — CLI that:
  1. Validates the YAML tree.
  2. For every problem: runs the reference solution through Piston with each test case's stdin; asserts the actual stdout (normalized like the grader does — see `attempt_run_test_handler.go:222-260`) matches the expected stdout. **A solution that doesn't pass its own tests fails the import.**
  3. Computes deterministic UUIDs from slugs (uuidv5).
  4. Inserts in **two passes inside ONE transaction**:
     - Pass 1: library content (teaching_units with topic_id=NULL, problems, problem_solutions, test_cases). UPSERT pattern: `INSERT ... ON CONFLICT (id) DO UPDATE SET ...` for canonical content rows.
     - Pass 2: course arrangement (courses, topics, topic_problems M2M). UPSERT same.
     - Pass 3: `LinkUnitToTopic` for each. `teaching_units` UPSERTs use `ON CONFLICT (topic_id) DO NOTHING` per the existing legacy pattern (Codex CRITICAL #4).
  5. **Post-insert verification step**: query the database and assert every Python 101 topic has a linked unit (mirrors plan 047's session-start guard). Fail loud if any topic ended up with `topic_id IS NULL` somewhere upstream.
  6. **Whole import is one transaction.** Either the entire library + course goes in, or nothing does. No partial state. No "library ok, course missing" gap.
- Create: `scripts/python-101/sandbox-runner.ts` — wraps Piston calls.
- Create: `tests/integration/python-101-import.test.ts` — happy path + failure modes (bad solution, malformed YAML, dangling problem slug, dangling unit slug).

**Failure modes the importer must catch:**
- Reference solution fails any of its test cases against Piston → import aborts (transaction rolls back).
- Topic ended up unlinked from a unit (post-insert verification) → import aborts.
- A unit's `problemSlugs` references a missing slug → import aborts before any insert.
- A `course.yaml` references a missing unit slug → import aborts before any insert.
- Re-run produces a NO-OP transaction (idempotent UUIDs + UPSERT semantics; LinkUnitToTopic is a no-op when same).

### Phase 3: ONE unit + its problems end-to-end (the canary + throughput checkpoint)

**Files:**
- Create: `content/python-101/units/print-and-comments.yaml`
- Create: `content/python-101/problems/<slug>.yaml` for each of the unit's 5-7 problems
- Create: `content/python-101/course.yaml` (initially with just topic 1)

**Actions:**
1. Author the unit + 5-7 problems by hand. **Time the authoring effort** (Codex MINOR #13 — the 30-50h estimate from the previous draft was unjustified). The actual time on this canary unit becomes the multiplier for re-estimating Phases 4-5.
2. Run the importer. All reference solutions pass against Piston.
3. Browser smoke A — library visibility (PRE-second-pass): comment out the second-pass `LinkUnitToTopic` calls temporarily, re-run; log in as a teacher in another org and verify the units appear in the picker as `classroom_ready` and unlinked. Restore the calls.
4. Browser smoke B — Python 101 itself: log in as the demo teacher, see the demo class wired to Python 101, navigate as a student, attempt a problem.

**Phase 3 is the integration AND throughput gate.** Re-estimate Phases 4-5 from the actual Phase 3 time. If too slow, defer Phases 4-5 to a separate plan.

### Phase 4: Author remaining 11 units + their problems

**Files:**
- Create: `content/python-101/units/<slug>.yaml` × 11 more
- Create: `content/python-101/problems/<slug>.yaml` × ~50 more
- Modify: `content/python-101/course.yaml` — add the remaining 11 topics

**Workflow per unit:** outline → author problems → author unit → validate → import dry-run → commit.

### Phase 5: Full-scale verification (NOT just spot-check — Codex IMPORTANT #8)

**Required by CLAUDE.md / docs/development-workflow.md:**
1. End-to-end import. All ~60 reference solutions pass via Piston.
2. **Full Vitest suite** passes.
3. **Full Go suite** passes.
4. **Playwright e2e** suite runs cleanly. Now that plan 048 unblocked auth.setup and the dev-overlay warning is silent, this is feasible.
5. Pedagogical review: each lesson's flow; "Try it" exercises the concept just taught; difficulty progression within each unit.
6. Picker discovery test: a different-org teacher sees Python 101 units as `classroom_ready` (and "Already linked"). Document the reuse caveat in the README.

**Verification artifact:** `docs/plans/049-python-101-qa-report.md` with the test outputs + sign-off.

### Phase 6: Ship — replace legacy seed, integrate with demo

**Files:**
- Delete: `scripts/seed_python_101.sql`.
- **Update doc references** to the legacy seed (Codex MINOR #11):
  - `docs/design-gaps-problem-workflow.md:3`
  - `docs/plans/027-legacy-classroom-cleanup.md:191`
  - `docs/plans/028-problem-bank.md` (multiple)
  - `docs/plans/032-teaching-unit-topic-migration.md:79`
  - `docs/specs/009-problem-bank.md:267`
  - Mark each as historical reference; add a forward note that plan 049 is the current Python 101 source of truth.
- Modify: the existing demo seed (`scripts/seed_problem_demo.sql` or wherever `eve@demo.edu`'s class lives — Phase 0 audit identified the actual file). Re-point the demo class's `course_id` to be a clone of the Bridge HQ Python 101 course. The clone happens at seed time via the `CloneCourse` flow (or a SQL equivalent that mirrors its semantics).
- Modify: `docs/setup.md` — reference the new authoring format and `bun content:python-101:import`.
- Modify: `TODO.md` — remove any Python-101-related entries.

**Actions:**
1. Audit who imports `seed_python_101.sql`: `git grep "seed_python_101"`. Already done in Phase 0; confirm zero runtime callers.
2. Run the importer against `bridge` and `bridge_test`. Confirm idempotence (no DB changes on re-run).
3. Browser smoke: log in as `eve@demo.edu`, see the cloned Python 101 class, walk a student through a problem.
4. Open PR.

## Implementation Order

Strict order. Each phase commits separately.

1. Phase 0 (decisions + Bridge HQ seed + Piston runtime confirmation + legacy audit).
2. Phase 1 (format + validator + npm deps).
3. Phase 2 (transactional importer with Piston check).
4. Phase 3 (canary unit; **measure authoring time** before committing to Phases 4-5 in this plan).
5. Phase 4 (11 more units + problems).
6. Phase 5 (full verification).
7. Phase 6 (ship).

## Risk Surface

- **Piston endpoint accessibility from importer.** Phase 0 confirms or builds. If we need a new admin endpoint, that's a small Go addition gated to platform-admin auth.
- **Pyodide ↔ Piston drift.** Documented in `docs/design-gaps-problem-workflow.md`. Authoring constraints in `content/python-101/README.md` will list known divergences (float printing, available stdlib).
- **Library reuse semantics.** The 1:1 unit↔topic invariant from plan 044 — once Bridge HQ Python 101 claims a unit, no other course can pick it. Documented as a known limitation. Plan 050+ can add fork/overlay.
- **Bridge HQ org bootstrap.** Phase 0 ships a one-time idempotent seed. If the seed already ran but the org was deleted, re-running the import would fail unless we add a "create org if missing" branch.
- **Importer transaction size.** Inserting 12 units + 60 problems + 60 solutions + ~250 test cases + 1 course + 12 topics + ~60 topic_problems in one transaction is fine for Postgres (sub-second). No batching needed.
- **CloneCourse for the demo wire-up.** The demo seed will need to call CloneCourse equivalent SQL (it's pure SQL today). The new wire-up materializes Python 101's content into the demo class's namespace once. If the demo seed is re-run, it should detect the clone exists and skip.
- **Effort estimate.** Original 30-50h was unjustified. Phase 3 is the throughput checkpoint — actual numbers there drive Phases 4-5 scope. The plan accepts that we may re-scope.

## Out of scope (explicit deferrals)

- **Plan 050 — `/register-org` form + parent_links + ended-session review surface.** The platform-feature deferrals from plans 047 and 048.
- **Plan 051 — Unit fork/overlay mechanics for true multi-course unit reuse** (the foundation isn't built yet, per Codex IMPORTANT #5).
- **Plan 052 — `expectedStdoutPattern` / regex test matching** (schema migration + grader change).
- **Plan 053 — Authoring AI** that drafts problem statements, solutions, tests from one-line prompts.
- **Plan 054 — Python 201** (intermediate Python).
- **JavaScript / Blockly variants of Python 101.**
- **AI tutor specifically tuned for Python 101.**

## Codex Review of This Plan

- **Date:** 2026-04-30
- **Verdict:** Plan needed substantive rewrite (4 CRITICAL + 6 IMPORTANT + 3 MINOR — Pyodide-vs-Piston runtime confusion, missing `expectedStdoutPattern` schema, missing `courses.scope`, fragile UPSERT semantics, plus several IMPORTANT issues including phantom AI endpoints and unspecified npm deps). This document IS the post-correction plan.

### Corrections applied

1. `[CRITICAL]` **Pyodide vs Piston runtime drift.** Original plan claimed "the importer uses the same executor the student-facing UI uses." Wrong — Pyodide is browser-only; Piston is the server grader. → Plan now uses Piston for verification; Pyodide↔Piston drift is documented as an authoring constraint (no random output, no Pyodide-only modules). Phase 0 confirms Piston endpoint accessibility.
2. `[CRITICAL]` **`expectedStdoutPattern` doesn't exist.** Schema has only `expected_stdout` (string), grader does exact match. → Field dropped from YAML schema. Authoring constraint: every problem must have a deterministic single-output answer. Pattern matching is filed as plan 052.
3. `[CRITICAL]` **No platform-scope courses.** `courses.org_id NOT NULL`, no scope field. → Course belongs to a new "Bridge HQ" org owned by a system user. Phase 0 ships an idempotent bootstrap seed. Reuse path is `CloneCourse`.
4. `[CRITICAL]` **Teaching-unit UPSERT vs `teaching_units_topic_id_uniq`.** UUID-keyed UPSERT could violate the topic_id partial-unique constraint. → Importer uses `ON CONFLICT (topic_id) DO NOTHING` for teaching_units (the legacy seed's pattern), and the whole import runs in one transaction so partial state is impossible.
5. `[IMPORTANT]` **YAML schema omitted required DB fields.** → New "Importer DB-field mapping" section enumerates every field + constant the importer uses, including `created_by = SYSTEM_USER_ID`, `attached_by`, `scope = 'platform'`, status defaults, and the test_cases columns from `drizzle/0008_problems.sql`.
6. `[IMPORTANT]` **`--ai-draft` referenced a non-existent API.** → Cut entirely. Authoring is hand-driven (Claude-in-chat is fine, but happens outside the importer). A real authoring AI is plan 053.
7. `[IMPORTANT]` **Wrong demo seed file name.** → Phase 6 names `seed_problem_demo.sql` (the actual file) and Phase 0 audits whether it overlaps Python 101 content.
8. `[IMPORTANT]` **Phase 5 was below verification policy.** → Phase 5 now requires full Vitest + Go + Playwright e2e per CLAUDE.md and `docs/development-workflow.md`.
9. `[IMPORTANT]` **Plan 047 guard could fire on partial import.** → Importer is one transaction; post-insert verification step asserts every topic has a linked unit before declaring success.
10. `[IMPORTANT]` **`uuid` npm dep undeclared.** → Phase 1 modifies `package.json` to add `uuid` and `yaml` (pinned versions).
11. `[MINOR]` **Legacy seed doc references.** → Phase 6 lists each doc to update.
12. `[MINOR]` **YAML parser unspecified.** → Phase 1 pins `yaml` package, parse policy disallows anchors/aliases.
13. `[MINOR]` **Effort estimate unjustified.** → Phase 3 is now an explicit throughput checkpoint that re-estimates Phases 4-5.
