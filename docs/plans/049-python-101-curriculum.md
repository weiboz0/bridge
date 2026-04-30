# Plan 049 — Python 101: Full-Scale Curriculum

## Status

- **Date:** 2026-04-30
- **Branch:** `feat/049-python-101-curriculum`
- **Goal:** Build production-quality platform-scope Python 101 content — 12 focus areas, ~60 problems with reference solutions and test cases, teaching_units for each focus area, all wired through the existing platform-scope content infrastructure (plans 028 + 044-046). Replace the legacy seed at `scripts/seed_python_101.sql` (broken since plan 046 dropped `topics.lesson_content` it depended on).

## Sources

- Existing legacy seed: `scripts/seed_python_101.sql` (6 topics, 12 problems, ~50 test cases). Broken on current main — references the dropped `topics.lesson_content` column.
- Platform-scope content infrastructure: `teaching_units` (plan 044), Unit picker (plan 045), problems + solutions + test_cases (plan 028), topics→Units 1:1 link.
- User request 2026-04-30: "develop full fledged courses for Python 101 that consists of language topics, problems, solutions, test cases in full scale."

## Sources + scope

The data model is already in place. No new tables, no new columns. This plan is **content authoring + tooling**, not feature code. Specifically:

- `courses` — top-level container (platform-scope).
- `topics` (Focus Areas in user-visible copy per plan 048) — syllabus subdivisions of the course.
- `teaching_units` (1:1 linked to a topic via `topic_id`) — canonical teaching material as Tiptap blocks.
- `problems` — coding challenges, platform-scope, with starter_code map per language and difficulty/grade_level metadata.
- `problem_solutions` — reference solutions per language; verifies correctness and seeds the AI tutor.
- `test_cases` — stdin/expected_stdout, `is_example=true` for visible, false for hidden.
- `topic_problems` — many-to-many join (a topic can attach multiple problems).

What's NOT done: the actual content. The legacy seed's 6 topics × 2 problems each is more "demo" than "curriculum." We replace it with a 12-focus-area, 5-7-problems-per-area arc that's actually teachable in a class.

## Decisions to confirm before Phase 1 (Phase 0)

These steer everything downstream. User confirms each before authoring content.

1. **Grade band.** Recommend **6-8** primary, with notes on K-5 simplifications and 9-12 extensions for each focus area. K-5 typically uses Blockly, not Python text — if K-5 is required, scope expands significantly. Default: 6-8.
2. **Total problem count.** Recommend **5-7 problems per focus area × 12 focus areas = 60-84 problems**. Each focus area needs enough problems for differentiation (a fast student needs 2-3 stretch problems beyond the baseline). Default: 60.
3. **Authoring format.** Recommend **YAML files in `content/python-101/`**, one per focus area, with a top-level `course.yaml`. Reasons: human-readable, diff-friendly in PRs, supports Markdown for descriptions cleanly, parseable by both TS and Go importers. Alternative: JSON (more verbose) or direct SQL (loses validation). Default: YAML.
4. **AI-assisted authoring scope.** Two options:
   - **Option A — Hand-authored only.** Human writes every problem, solution, test case, and teaching_unit. Highest quality, slowest (estimated 80-120 hours for 60 problems with full teaching_units).
   - **Option B — AI-drafted, human-reviewed.** Use Claude / Codex / Codex's coding agent to draft problem statements, reference solutions, and test cases from a one-line prompt. Human reviews each, edits as needed. Estimated 30-50 hours total. Recommended for a pre-production curriculum.
   
   Default: **Option B**. The plan ships the authoring tool with an optional `--ai-draft` mode that calls the existing platform LLM backend; humans still own quality acceptance.
5. **Legacy seed disposition.** The current `scripts/seed_python_101.sql` is broken. Two options: (a) repair its `topics` insert and salvage its 12 problems; (b) delete it and start fresh with the new authoring format. Default: **(b) delete and start fresh**, but copy the 12 problem titles + descriptions into the new format as a starting point so we don't lose the work that's there.
6. **Course visibility.** Should the new Python 101 be `is_published=true` and visible to all teachers as a clone source, or `draft` until reviewed? Default: **published** at platform scope (the whole point is to ship a usable curriculum). Teachers can clone the course into their org and adapt.
7. **Demo data integration.** Should the demo seed (`eve@demo.edu` etc.) automatically attach the new Python 101 course to the demo class, or stay independent? Default: **attach** — the demo class becomes a working Python 101 class out of the box.

## Curriculum structure (proposed, subject to Phase 0 confirmation)

12 focus areas, sequenced for a typical 12-week intro course (1 focus area per week). Each focus area has a teaching_unit (lesson) + 5-7 problems.

| # | Focus Area | Concepts | Sample problems |
|---|---|---|---|
| 1 | Hello, World — print and comments | `print()`, string literals, `#` comments, the REPL mental model | Print your name; print a multi-line poem; comment-out a buggy line |
| 2 | Variables and basic types | `int`, `float`, `str`, `bool`, assignment, naming rules, `type()` | Calculate seconds in a day; convert F to C; format a greeting |
| 3 | Arithmetic and operators | `+ - * / // % **`, operator precedence, `int()`/`float()` casts | Compute area of a circle; tip calculator; modulo to find remainders |
| 4 | Strings | indexing, slicing, `len()`, `.upper()` / `.lower()` / `.strip()` / `.split()` / `.replace()`, f-strings | Count vowels; reverse a string; censor a word |
| 5 | Conditionals | `if/elif/else`, comparison ops, boolean ops, truthy/falsy | FizzBuzz; pass/fail grader; leap year |
| 6 | Loops | `for ... in range()`, `while`, `break`, `continue`, accumulator pattern | Sum 1..N; multiplication table; first power of 2 above N |
| 7 | Lists | `[]`, indexing, slicing, `.append()` / `.pop()` / `.sort()`, `len()`, `in`, list comprehensions (intro) | Average a list; remove duplicates; running max |
| 8 | Dictionaries and sets | `{}`, key/value, `.keys()` / `.values()` / `.items()`, set operations | Word count; unique vowels; gradebook lookup |
| 9 | Functions | `def`, parameters, return, `*args`, scope, docstrings | `factorial(n)`; `is_prime(n)`; `palindrome(s)` |
| 10 | File I/O and exceptions | `open()`, `.read()` / `.readline()`, `try/except`, `with` | Count lines in a file; CSV row sum; safe int parsing |
| 11 | Classes and objects (intro) | `class`, `__init__`, attributes, methods, `__str__` | `Counter` class; `Rectangle` area/perimeter; `Bank` deposit/withdraw |
| 12 | Capstone | mixed concepts | Hangman (subset); shopping cart; a tiny TODO CLI |

Each focus area's teaching_unit follows a consistent template: **Big idea → Worked examples → Try it → Common pitfalls → Vocabulary**. The "Try it" sub-section embeds 1-2 of the focus area's problems via `problem-ref` blocks (the existing Tiptap block type).

## Phases

### Phase 0: Decisions + scope freeze + legacy audit

**Files:** None.

**Actions:**
1. User confirms the 7 decisions above (grade band, problem count, format, AI-assist Y/N, legacy disposition, visibility, demo integration).
2. Audit `scripts/seed_python_101.sql` to extract the 12 problems' titles, descriptions, starter code, and test cases. Catalog into a side document (`docs/plans/049-python-101-curriculum-legacy-extract.md`) for re-use in Phase 4.
3. Confirm the platform Python sandbox can run the reference solutions we'll author. Quick check: pick one existing seed problem, run its reference solution through the executor, observe pass/fail.

**Verification:** Decisions written into this plan file under "Decisions confirmed." Legacy extract committed.

### Phase 1: Authoring format + Zod validator

**Files:**
- Create: `content/python-101/README.md` — explains the format, links the validator.
- Create: `scripts/python-101/schema.ts` — Zod schema for `course.yaml` and per-focus-area YAMLs.
- Create: `scripts/python-101/validate.ts` — CLI that walks the `content/python-101/` tree and validates each file. Exit 1 on any error. No DB access yet.
- Create: `tests/unit/python-101-schema.test.ts` — round-trip tests on the schema (valid example loads, invalid examples fail with clear messages).

**Schema sketch (YAML):**

```yaml
# content/python-101/course.yaml
title: Python 101 — Introduction to Programming
slug: python-101
description: |
  A 12-week introduction to Python for grades 6-8. Covers core
  language features through small problems with auto-graded test
  cases. Teachers can clone this course and adapt for their classes.
gradeLevel: 6-8
language: python
focusAreas:
  - 01-print-and-comments
  - 02-variables-and-types
  - ...
```

```yaml
# content/python-101/01-print-and-comments.yaml
title: Hello, World — print and comments
slug: print-and-comments
sortOrder: 1
description: |
  First contact with Python. We learn how to display text and how to
  leave notes for ourselves and other readers.
teachingUnit:
  title: Hello, World
  materialType: notes
  blocks:
    - type: prose
      content: |
        ## The big idea
        Python runs your file top to bottom...
    - type: code-snippet
      lang: python
      code: |
        print("Hello, world!")
problems:
  - slug: print-your-name
    title: Print your name
    difficulty: easy
    description: |
      Write a program that prints your name. Use the `print()` function.
    starterCode:
      python: |
        # Write your code below
    solution:
      python: |
        print("Ada Lovelace")
    testCases:
      examples:
        - name: prints "Ada Lovelace"
          stdin: ""
          expectedStdout: "Ada Lovelace\n"
      hidden:
        - name: prints any non-empty line
          stdin: ""
          expectedStdoutPattern: "^.+\n$"
          # OR expectedStdout: "..." for exact match
```

(The schema's full shape is in the Zod file. Pattern-vs-exact matches give problems with multiple correct outputs more flexibility.)

### Phase 2: Importer tooling + sandbox-verified solution check

**Files:**
- Create: `scripts/python-101/import.ts` — CLI that:
  1. Validates the YAML tree via the Phase 1 schema.
  2. For every problem, runs its reference solution through the existing Python sandbox and asserts ALL test cases pass against the solution. **A solution that doesn't pass its own test cases fails the import.** This is the key quality gate.
  3. Computes deterministic UUIDs from slugs (`uuidv5("python-101:" + slug, NAMESPACE_UUID)` with a fixed namespace) so re-runs are idempotent.
  4. Inserts/upserts platform-scope rows: course, topics, teaching_units (with topic_id 1:1 link), problems, problem_solutions, test_cases, topic_problems.
- Create: `scripts/python-101/sandbox-runner.ts` — wraps the existing executor service to run a Python solution against a list of test cases and return pass/fail per case.
- Create: `tests/integration/python-101-import.test.ts` — happy path (a tiny 1-focus-area, 1-problem fixture), failure paths (solution doesn't pass tests, malformed YAML, unknown topic ref).
- Modify: `package.json` — add `"content:python-101:validate"` and `"content:python-101:import"` scripts.

**Failure modes the importer must catch:**
- Reference solution fails any of its own test cases → import fails for that problem.
- A test case has `expectedStdout` AND `expectedStdoutPattern` → import fails (one or the other).
- A topic's `problems` reference a slug that doesn't exist → import fails.
- Re-run produces no DB changes (idempotent UUIDs + UPSERT).

### Phase 3: ONE focus area end-to-end (the canary)

**Files:**
- Create: `content/python-101/course.yaml`
- Create: `content/python-101/01-print-and-comments.yaml` — full content for focus area 1: 5-7 problems, each with starter, solution, examples, hidden tests; teaching_unit with prose blocks + 2-3 worked examples + "try it" problem-refs.

**Actions:**
1. Author focus area 1 by hand (or AI-drafted then human-edited per Phase 0 decision #4).
2. Run the importer. Observe all reference solutions pass their test cases.
3. Browser smoke: log in as platform admin, navigate to the new course, attach focus area 1 to a demo class, log in as student, attempt a problem, observe correct grading.
4. Eat the dogfood: check that the teaching_unit reads naturally for a 6-8 grader, that the problem progression makes sense (easiest → harder), that error messages are friendly.

**Verification gates the rest of the plan.** If Phase 3 surfaces a structural issue (schema needs a field, importer needs a new check, etc.), fix here before authoring 11 more focus areas.

### Phase 4: Author remaining 11 focus areas

**Files:**
- Create: `content/python-101/02-variables-and-types.yaml` through `12-capstone.yaml`.

**Workflow per focus area:**
1. Outline: pick the 5-7 problems' titles + difficulty.
2. Author: write description + starter + solution + test cases. Per the Phase 0 decision, optionally use the `--ai-draft` mode of the importer to scaffold, then human-review.
3. Author the teaching_unit: prose blocks following the Big-idea / Worked-examples / Try-it / Pitfalls / Vocab template.
4. Run `bun content:python-101:validate` — must pass.
5. Run `bun content:python-101:import --dry-run` — solutions verified by sandbox; must show no errors.
6. Commit the focus area's YAML.

**Pacing:** 1 focus area per commit (or per pair of commits — content + small importer fixes if needed). Total: 11 commits at this phase.

### Phase 5: Full-scale QA

**Actions:**
1. End-to-end import of the entire `content/python-101/` tree. All 60+ reference solutions pass through the sandbox.
2. Spot-check from a student's perspective: pick 2 focus areas at random, attempt every problem as a fresh student, time how long it takes.
3. Pedagogical review: skim each teaching_unit; does it flow? Does the "Try it" problem actually exercise the concept just taught?
4. Cross-check problem difficulty progression — within each focus area, easier problems should come first.
5. Address any defects surfaced. Re-run the importer.

**Verification:** A document at `docs/plans/049-python-101-qa-report.md` with the QA findings + sign-off.

### Phase 6: Ship — replace legacy seed, integrate with demo

**Files:**
- Delete: `scripts/seed_python_101.sql` (broken since plan 046, fully superseded).
- Modify: `scripts/seed_demo_data.sql` (or wherever the demo accounts wire up) — attach the new platform-scope Python 101 course to the demo class so `eve@demo.edu` and her students see a working class out of the box.
- Modify: `docs/setup.md` — update the "demo data" section to reference the new authoring format and importer command.
- Modify: `TODO.md` — remove any Python-101-related tech-debt entries.

**Actions:**
1. Run the importer against `bridge` (dev DB) and `bridge_test` (test DB). Confirm idempotence.
2. Mark the platform-scope course `is_published=true` (or whatever the publish marker is for `courses`).
3. Browser smoke: log in to the demo as `eve@demo.edu`, see the Python 101 class, walk a student through a problem.
4. Open PR.

## Implementation Order

Strict order:

1. Phase 0 (decisions + legacy audit) — no code yet.
2. Phase 1 (authoring format + validator) — pure data definition, fast.
3. Phase 2 (importer + sandbox check) — the CRITICAL infrastructure. Don't move past until the sandbox-verified gate works.
4. Phase 3 (one focus area) — the integration test. Don't proceed to Phase 4 until Phase 3's browser smoke passes.
5. Phase 4 (11 more focus areas) — the long content authoring stretch.
6. Phase 5 (QA pass).
7. Phase 6 (ship + demo integration).

Phases 0-3 should land in days. Phase 4 is the bulk of the calendar time. Phase 4 commits are individually small and reviewable.

## Risk Surface

- **Sandbox runtime drift.** Reference solutions might pass in the importer's local Python sandbox but fail in the student-facing Pyodide runtime. Mitigate: the importer runs solutions through the SAME executor the student-facing UI uses, not a local subprocess. Verify in Phase 0 that this path is feasible.
- **Content quality is subjective.** A 6-8 grader might find what we think is "easy" hard, or vice versa. Mitigate: Phase 5 includes a student-perspective walkthrough. Plan 050+ can iterate on specific problems.
- **AI-drafted content quality.** If we use the AI-assist mode (Phase 0 decision #4), drafts may include subtle bugs (off-by-one in test cases, unclear wording). The importer's "solution must pass its own tests" check catches functional bugs but not pedagogy bugs. Mitigate: human review is mandatory; the AI-assist is a scaffolder, not an author.
- **Schema rename collision.** The user-visible copy says "Focus Area" (plan 048) but the DB column is `topics`. The importer talks to the DB, so it uses `topics` everywhere internally. The course YAML uses `focusAreas` for user-visible naming consistency. Document this clearly in `content/python-101/README.md`.
- **Importer idempotence.** Deterministic UUIDs are the foundation. If we re-import after a topic's slug changes, the existing row is orphaned. Mitigate: the importer warns on slug changes and requires a `--allow-rename` flag.
- **Legacy seed deletion timing.** The legacy seed is broken since plan 046; deleting it is safe. But if any other test or seed script imports it, we'd break those. Phase 6 grep before delete.

## Out of scope (explicit deferrals)

- **Plan 050 — `/register-org` form + parent-child linking + ended-session review surface.** The infra-and-platform work continues there.
- **Plan 051 — Python 201 (loops + nested data structures + algorithms).** Once Python 101 is shipped and field-tested.
- **JavaScript / Blockly variants of Python 101.** A separate plan if/when product wants multi-language coverage.
- **AI tutor integration specifically tuned for Python 101.** The platform's AI tutor is a general feature; this plan doesn't extend it. Plan 052+ if specific tuning is needed.
- **A built-in IDE or notebook view for content authoring.** YAML-files-in-PRs is the authoring workflow for this plan.

## Codex Review of This Plan

(Pending — dispatch `codex:codex-rescue` per CLAUDE.md plan review gate before any implementation begins.)
