# Plan 049 — Python 101: Platform-Scope Library + Curriculum Arrangement

## Status

- **Date:** 2026-04-30
- **Branch:** `feat/049-python-101-curriculum`
- **Goal:** Build a **platform-scope library** of 12 teaching units and ~60 problems for introductory Python, then arrange them as the Python 101 course. The library content is the primary product; the Python 101 course is one curated arrangement. Other teachers can clone the whole course OR pick problems into their own courses (via plan 045's picker). Replace the legacy seed at `scripts/seed_python_101.sql` (broken since plan 046 dropped `topics.lesson_content`).

## Sources

- Existing legacy seed: `scripts/seed_python_101.sql` (6 topics, 12 problems, ~50 test cases). Broken on current main.
- Platform-scope content infrastructure: `teaching_units` (plan 044), Unit picker (plan 045), problems + solutions + test_cases (plan 028), topics 1:1 linked to units (plan 044), `topic_problems` M2M.
- User decision 2026-04-30: **"Group B" = Unit-primary library framing.** Lessons live as platform-scope `teaching_units` that exist independently of the course, then get linked to Python 101's topics. Problems live as platform-scope `problems` that can be attached to multiple topics (`topic_problems` is many-to-many).

## Framing: Library first, course as arrangement

**Library entities (the primary product):**

- 12 `teaching_units`, scope=`platform`, status=`classroom_ready`. Each holds the lesson content for one Python concept area (Hello World → OOP). Initial state: `topic_id = NULL` (uncommitted to any course).
- ~60 `problems`, scope=`platform`, status=`published`. Each has starter code, a reference solution, example test cases (visible), and hidden test cases. Independent of any course.

**Course entity (the arrangement):**

- One `courses` row, scope semantics implicit (created_by = a system / platform-admin user).
- 12 `topics` rows, each with `sort_order` 1..12.
- Each topic gets linked to its corresponding library unit via `LinkUnitToTopic` (sets `teaching_units.topic_id = <topic.id>` — the 1:1 invariant from plan 044). After this step the units are "claimed" by Python 101's topics.
- Each topic also gets the relevant problems attached via `topic_problems` (M2M), so multiple courses CAN share problems freely (only the unit↔topic link is exclusive).

**Reuse model:**

- **Other teachers cloning Python 101:** they use the existing `CloneCourse` flow. The clone gets new topic rows but starts with NO linked units (per plan 044 phase 4 — `cloneCourse` no longer copies the unit link). The teacher then picks library units via plan 045's picker.
  - **But:** the original Python 101 has already claimed each library unit (1:1). So another teacher's pick of "Loops" finds the unit already linked to Python 101's "Loops" topic and shows "Already linked" in the picker.
  - **Implication for this plan:** true many-to-many unit reuse requires the existing fork/overlay mechanics (plan 033/034) which are out of scope here. For now, "reuse" of a library unit by another teacher means: clone the unit (creates a new Personal/Org-scope copy) and link the clone. We document this clearly in `content/python-101/README.md` and don't try to solve it in this plan.
- **Other teachers picking individual problems:** the M2M `topic_problems` allows a problem to be attached to MANY topics across many courses. So problems are freely reusable; units have the 1:1 caveat above.

**Why this framing matters:**

- The 12 lessons exist as standalone library content even before they're claimed by Python 101 — visible in the picker, discoverable, properly attributed.
- Plan 050+ can add unit fork/overlay support to make the units truly multi-course-reusable; the data model already has the column shape (`forked_from`, `unit_overlays` referenced by plan 033/034).
- Problems remain a separate primitive. A teacher building a different course can pull "FizzBuzz" or "is_prime" without taking the rest of Python 101.

## Decisions to confirm before Phase 1 (Phase 0)

These steer everything downstream. User confirms each before authoring content.

1. **Grade band.** Recommend **6-8** primary, with notes on K-5 simplifications and 9-12 extensions for each focus area. Default: 6-8.
2. **Total problem count.** Recommend **5-7 problems per focus area × 12 focus areas = 60-84 problems**. Default: 60.
3. **Authoring format.** **YAML** files in `content/python-101/`, with three subdirectories:
   - `units/<slug>.yaml` — one teaching unit per file (lesson content, references the problem slugs to attach).
   - `problems/<slug>.yaml` — one problem per file (starter, solution, test cases, metadata).
   - `course.yaml` — the Python 101 arrangement (course title, description, ordered list of unit slugs that become its 12 topics).
4. **AI-assisted authoring.** Two options. Default: **AI-drafted, human-reviewed** (`--ai-draft` flag on the importer that scaffolds problem statements / solutions / tests / lesson prose from a one-line concept, then a human edits before commit).
5. **Legacy seed disposition.** Default: **delete `scripts/seed_python_101.sql`** (broken since plan 046, fully superseded). Extract its 12 problem titles + descriptions into the new format as starting drafts.
6. **Course visibility.** Default: **published** at platform scope. Teachers can clone and adapt.
7. **Demo data integration.** Default: **attach automatically.** The demo seed gives `eve@demo.edu` a class wired to Python 101 out of the box.

## Library structure (proposed, subject to Phase 0 confirmation)

12 platform-scope teaching units, each with a slug. The Python 101 course's 12 topics each link to one of these units in this order:

| # | Unit slug | Concepts | Sample problems |
|---|---|---|---|
| 1 | `print-and-comments` | `print()`, string literals, `#` comments, the REPL mental model | Print your name; print a multi-line poem; comment-out a buggy line |
| 2 | `variables-and-types` | `int`, `float`, `str`, `bool`, assignment, `type()` | Seconds in a day; F to C; format a greeting |
| 3 | `arithmetic-and-operators` | `+ - * / // % **`, precedence, casts | Circle area; tip calculator; modulo |
| 4 | `strings` | indexing, slicing, `.upper()` / `.split()` / `.replace()`, f-strings | Count vowels; reverse a string; censor a word |
| 5 | `conditionals` | `if/elif/else`, comparison + boolean ops, truthy/falsy | FizzBuzz; pass/fail grader; leap year |
| 6 | `loops` | `for ... in range()`, `while`, `break`, `continue`, accumulator | Sum 1..N; multiplication table; first power of 2 above N |
| 7 | `lists` | `[]`, indexing, slicing, `.append()` / `.pop()` / `.sort()`, `in`, list comp (intro) | Average a list; remove duplicates; running max |
| 8 | `dicts-and-sets` | `{}`, key/value, `.keys()` / `.values()` / `.items()`, set ops | Word count; unique vowels; gradebook lookup |
| 9 | `functions` | `def`, parameters, return, `*args`, scope, docstrings | `factorial(n)`; `is_prime(n)`; `palindrome(s)` |
| 10 | `files-and-exceptions` | `open()`, `.read()`, `try/except`, `with` | Count lines; CSV row sum; safe int parsing |
| 11 | `classes-and-objects` | `class`, `__init__`, attributes, methods, `__str__` | `Counter` class; `Rectangle` area; `Bank` deposit/withdraw |
| 12 | `capstone` | mixed concepts | Hangman (subset); shopping cart; tiny TODO CLI |

Each teaching_unit follows a consistent template: **Big idea → Worked examples → Try it → Common pitfalls → Vocabulary**. The "Try it" section embeds 1-2 problems via `problem-ref` blocks (the existing Tiptap block type).

## Phases

### Phase 0: Decisions + scope freeze + legacy audit

**Files:** None (decisions get recorded in this plan file under "Decisions confirmed").

**Actions:**
1. User confirms the 7 decisions above.
2. Audit `scripts/seed_python_101.sql`. Extract the 12 existing problems (titles, descriptions, starters, test cases) into `docs/plans/049-python-101-legacy-extract.md` for re-use as starting drafts.
3. Confirm the platform Python sandbox is callable from a TS importer. Quick check: take one existing seed problem's reference solution and run it through the executor service path the student-facing UI uses. If the executor needs full Hocuspocus + Yjs runtime, the importer needs a different sandbox path — document the choice.

**Verification:** All decisions written into this plan. Legacy extract committed.

### Phase 1: Authoring format + Zod validator

**Files:**

- Create: `content/python-101/README.md` — explains the format, the unit-primary framing, the 1:1 unit↔topic caveat for reuse.
- Create: `scripts/python-101/schema.ts` — Zod schemas for `units/*.yaml`, `problems/*.yaml`, `course.yaml`.
- Create: `scripts/python-101/validate.ts` — CLI that walks the tree and validates each file. No DB access yet.
- Create: `tests/unit/python-101-schema.test.ts` — round-trip tests on the schema.
- Create: `package.json` script `content:python-101:validate`.

**Schema sketches:**

```yaml
# content/python-101/units/01-print-and-comments.yaml
slug: print-and-comments
title: Hello, World — print and comments
materialType: notes
gradeLevel: 6-8
description: |
  First contact with Python — printing text and writing comments.
problemSlugs:
  - print-your-name
  - print-multi-line-poem
  - comment-out-a-bug
blocks:
  - type: prose
    content: |
      ## Big idea
      Python runs your file top to bottom. Each line is a step…
  - type: code-snippet
    lang: python
    code: |
      print("Hello, world!")
  - type: problem-ref
    problemSlug: print-your-name  # rendered inline in the lesson
```

```yaml
# content/python-101/problems/print-your-name.yaml
slug: print-your-name
title: Print your name
difficulty: easy
gradeLevel: 6-8
tags: [print, intro]
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
```

```yaml
# content/python-101/course.yaml
title: Python 101 — Introduction to Programming
slug: python-101
description: |
  A 12-week intro to Python for grades 6-8…
gradeLevel: 6-8
language: python
isPublished: true
topics:
  - unitSlug: print-and-comments       # → topic 1, sortOrder 1
  - unitSlug: variables-and-types      # → topic 2
  # … 12 entries
```

The `expectedStdoutPattern` field needs schema support — see Phase 1 risk note below.

### Phase 2: Importer with sandbox-verified solution check

**Files:**

- Create: `scripts/python-101/import.ts` — CLI that:
  1. Validates the YAML tree via the Phase 1 schema.
  2. For every problem, runs its reference solution through the platform's existing Python sandbox and asserts ALL test cases pass against the solution. **A solution that doesn't pass its own test cases fails the import.**
  3. Computes deterministic UUIDs from slugs (`uuidv5("python-101:units:" + slug, NS)`, similar for problems / course / topics) so re-runs are idempotent.
  4. Inserts/upserts in this order:
     - **First pass — library content:** all `teaching_units` (with `topic_id = NULL`), all `problems`, all `problem_solutions`, all `test_cases`. Library content exists on its own.
     - **Second pass — course arrangement:** the `courses` row, the 12 `topics` rows, then `LinkUnitToTopic(unitId, topicId)` for each (sets `teaching_units.topic_id`), then `topic_problems` rows.
- Create: `scripts/python-101/sandbox-runner.ts` — wraps the platform executor. **MUST use the same execution path the student UI uses** (Pyodide via the existing executor service) to avoid runtime drift.
- Create: `tests/integration/python-101-import.test.ts` — happy path (1-unit, 1-problem fixture), failure paths (bad solution, malformed YAML, dangling problem slug, dangling unit slug in course.yaml).
- Modify: `package.json` — add `content:python-101:import` script.

**Failure modes the importer must catch:**
- Reference solution fails any of its test cases → import fails.
- A test case has `expectedStdout` AND `expectedStdoutPattern` → import fails.
- A unit's `problemSlugs` references a slug that doesn't exist in `problems/` → import fails.
- A `course.yaml` `topics[].unitSlug` references a slug that doesn't exist in `units/` → import fails.
- Re-run produces no DB changes (idempotent UUIDs + UPSERT).

**Library-first insert order matters:**
- Insert units WITH `topic_id = NULL` first. They become picker-discoverable immediately.
- THEN create the course + topics.
- THEN call `LinkUnitToTopic` for each — this sets `topic_id`, the 1:1 link.
- This order means an interrupted import leaves library content in place but the course unfinished — re-running picks up where it left off.

### Phase 3: ONE unit + its problems end-to-end (the canary)

**Files:**

- Create: `content/python-101/units/01-print-and-comments.yaml`
- Create: `content/python-101/problems/print-your-name.yaml`, `print-multi-line-poem.yaml`, `comment-out-a-bug.yaml` (plus 2-4 more problems for this unit)
- Create: `content/python-101/course.yaml` (initially with just topic 1 referencing the print-and-comments unit; we'll add the rest in Phase 4)

**Actions:**
1. Author the unit + its 5-7 problems (hand-written or AI-drafted per Phase 0 decision #4).
2. Run the importer. All reference solutions pass their test cases through the sandbox.
3. Browser smoke A — library visibility: log in as a teacher in a DIFFERENT org, open the picker on one of their topics, observe the print-and-comments unit appears with status=`classroom_ready`. (At this point the unit is still UNCLAIMED because the Python 101 course has been created with the topic-link → so the picker WILL show it as already linked. Test the other branch by reverting the LinkUnitToTopic call temporarily, OR test before running the second pass of the importer.)
4. Browser smoke B — Python 101 itself: log in as a platform admin, navigate to the Python 101 course, observe the topic with the linked unit, attach the course's class to the demo class, log in as a student, attempt a problem, observe correct grading.
5. Eat the dogfood: read the lesson; does it flow for a 6-8 grader? Are problems easier→harder?

**Phase 3 is the integration gate.** Surface any structural issues here before authoring 11 more units.

### Phase 4: Author remaining 11 units + their problems

**Files:**

- Create: `content/python-101/units/02-variables-and-types.yaml` through `12-capstone.yaml`
- Create: `content/python-101/problems/<slug>.yaml` for each of the remaining ~50 problems
- Modify: `content/python-101/course.yaml` — add the remaining 11 topic entries

**Workflow per unit:**
1. Outline 5-7 problems' titles + difficulty.
2. Author each problem YAML (description + starter + solution + test cases). Optional `--ai-draft` mode scaffolds.
3. Author the unit YAML (Big-idea / Worked-examples / Try-it / Pitfalls / Vocab template).
4. `bun content:python-101:validate` — must pass.
5. `bun content:python-101:import --dry-run` — solutions verified by sandbox.
6. Commit the unit + its problems together (one logical unit of work per commit).

**Pacing:** ~11 commits. Each is small and reviewable.

### Phase 5: Full-scale QA

**Actions:**
1. End-to-end import. All ~60 reference solutions pass their tests.
2. Spot-check from a student perspective: pick 2 random units, attempt every problem.
3. Pedagogical review: each lesson's flow; "Try it" actually exercises the concept just taught.
4. Difficulty progression check: easier→harder within each unit.
5. Fix any defects; re-run the importer.
6. **Picker discovery test:** log in as a different-org teacher; verify all 12 platform units appear in the picker for THEIR course (caveat: with `topic_id` already set by Python 101's claim, they show as "Already linked" — document this clearly in the QA report and in the README).

**Verification:** `docs/plans/049-python-101-qa-report.md` with findings + sign-off.

### Phase 6: Ship — replace legacy seed, integrate with demo

**Files:**

- Delete: `scripts/seed_python_101.sql` (broken since plan 046, fully superseded).
- Modify: demo data seed (whichever script wires the demo accounts) — attach Python 101 to the demo class.
- Modify: `docs/setup.md` — reference the new authoring format and `bun content:python-101:import` command.
- Modify: `TODO.md` — remove any Python-101-related entries.

**Actions:**
1. Audit who imports `seed_python_101.sql` (`git grep "seed_python_101"`); delete only if zero callers.
2. Run the importer against `bridge` and `bridge_test`. Confirm idempotence.
3. Browser smoke: log in as `eve@demo.edu`, see the Python 101 class, walk a student through a problem.
4. PR.

## Implementation Order

Strict order:

1. **Phase 0** — decisions + legacy audit, no code yet.
2. **Phase 1** — authoring format + validator.
3. **Phase 2** — importer + sandbox check. The CRITICAL infrastructure. Don't move past until the sandbox-verified gate works AND the library-first insert order is correct.
4. **Phase 3** — ONE unit + problems end-to-end. The integration gate.
5. **Phase 4** — 11 more units + problems.
6. **Phase 5** — QA pass.
7. **Phase 6** — ship + demo integration.

## Risk Surface

- **Sandbox runtime drift.** Reference solutions might pass in the importer's local Python sandbox but fail in the student-facing Pyodide runtime. Mitigate: the importer runs solutions through the SAME executor the student-facing UI uses. Verify in Phase 0.
- **Library reuse semantics.** The 1:1 unit↔topic invariant from plan 044 means once Python 101 claims a unit, no other course can pick it via the picker. Document the reuse caveat clearly. Plan 050+ can add fork/overlay support if/when product wants true many-to-many.
- **`expectedStdoutPattern` schema gap.** The platform's existing `test_cases.expected_stdout` is a string only. The plan's pattern-vs-exact approach lets a problem accept multiple correct outputs (useful for "prints any non-empty line"). Two options:
  - (a) Schema extension (add an `expected_stdout_pattern` column), out of scope here.
  - (b) Convert pattern tests to exact tests at import time using a generated reference output (run the solution, capture its actual stdout, store as exact). Loses some semantics but keeps schema unchanged. **Plan default: option (b).**
  - Document in the YAML format README so authors know patterns get materialized to exact strings.
- **AI-drafted content quality.** AI scaffolding may include subtle pedagogy bugs (off-by-one in test cases, unclear wording). The importer's "solution must pass its own tests" check catches functional bugs, not pedagogical ones. Mitigate: human review is mandatory; AI-assist is a scaffolder, not author.
- **Importer idempotence.** Deterministic UUIDs are the foundation. If a unit's slug changes after first import, the existing row is orphaned. Mitigate: importer warns on slug changes and requires `--allow-rename` flag.
- **Legacy seed deletion.** Phase 6 grep before delete. The legacy seed is broken since plan 046, but a developer-onboarding doc might still reference it.
- **Demo data integration timing.** The demo seed creates classes wired to specific course UUIDs. After deleting the legacy seed, the demo class needs re-pointing to the new course. If we don't update both in the same commit, the demo loads with a broken course reference.

## Out of scope (explicit deferrals)

- **Plan 050 — `/register-org` form + parent_links + ended-session review surface.** The infra-and-platform deferrals from plans 047 and 048.
- **Plan 051 — Unit fork/overlay mechanics for true multi-course unit reuse.** The data model has the column shape (`forked_from`, `unit_overlays`); the missing piece is the workflow + UI.
- **Plan 052 — Python 201** (loops + nested data structures + algorithms).
- **JavaScript / Blockly variants of Python 101.**
- **AI tutor specifically tuned for Python 101.** The platform's AI tutor is a general feature; this plan doesn't extend it.
- **A built-in IDE / notebook view for content authoring.** YAML-files-in-PRs is the workflow.

## Codex Review of This Plan

(Pending — dispatch `codex:codex-rescue` per CLAUDE.md plan review gate before any implementation begins.)
