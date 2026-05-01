# Design gaps surfaced by authoring a real course

> **Historical note (plan 049):** the seed file referenced below
> (`scripts/seed_python_101.sql`) was retired when plan 049 introduced
> the YAML-based authoring format at `content/python-101/`. Treat this
> file as a historical inventory of friction points the *original*
> SQL-based authoring exposed; some are now resolved by the importer
> (see `content/python-101/README.md`), others are still open.

*Context:* writing `scripts/seed_python_101.sql` (a 6-topic, 12-problem Python 101 unit) against the Problem / Attempt / Test workflow shipped in plans 024–026. The seed works end-to-end; the issues below are real friction points that will surface again when teachers author their own problems. Ordered roughly by impact.

---

## 1. `normalizeStdout` is exact-match-with-minor-tweaks only

**Current behavior** (`platform/internal/handlers/attempt_run_test_handler.go::normalizeStdout`):

```go
s = strings.ReplaceAll(s, "\r\n", "\n")
return strings.TrimRight(s, "\n")
```

CRLF → LF and strip trailing blank lines. Nothing else.

**Failures this produces:**

- `print("7.80")` vs expected `7.8` → fail (exact float format required)
- `print("Hello, World! ")` with a trailing space → fail against `"Hello, World!"`
- Problems where output order doesn't matter (e.g., "print the even numbers") can't be expressed — the student must match the exact sequence the teacher imagined
- `print(3/4)` in Python prints `0.75` but `1/3` prints `0.3333333333333333` — forcing students to use `f"{x:.2f}"` or similar just to get deterministic output

**Fix candidates (ascending cost):**

1. **Strip trailing whitespace per line** in addition to trailing newlines. One-line change; covers the most common student footgun.
2. **Per-problem comparison mode** — a `comparison` column on `problems` with values `exact` (current), `trimmed-lines`, `sorted-lines`, `numeric-tolerance`. Handler dispatches on this.
3. **Numeric-tolerance comparator** — when expected looks numeric, allow `|a - b| < epsilon`.

Without at least (1), teachers authoring problems spend most of their time wrestling with whitespace instead of writing good problems.

---

## 2. I/O-only test model is restrictive

Test cases are `stdin → expected_stdout`. The student writes a whole program that reads from `input()` and prints to stdout.

Most competitive-programming platforms use a **harness**: the student writes `def solve(...)` and the test runner wraps it with `print(solve(parse(stdin)))`. This lets tests compare *return values* rather than print formatting.

Without a harness, every problem gets cluttered with `list(map(int, input().split()))` and `print(result)`. For a K-12 "first course in Python" this is actually educational (students learn I/O), but for anything more advanced it becomes noise.

**Fix:** add `problems.test_harness` (text, nullable) — a template with `{{student_code}}` and `{{case_stdin}}` placeholders. Handler expands before sending to Piston. Hide the `input()` + `print(solve(...))` boilerplate; tests compare the harness's stdout.

This is a meaningful migration (touches the runner, the editor, the problem-authoring surface), but it's what makes the platform viable beyond intro-level.

---

## 3. No problem metadata — difficulty, tags, points

`problems` has `order` (sequence within topic) and that's it. No `difficulty`, no `tags`, no `points`.

A student looking at a topic has no signal about what's easy vs hard before clicking in. A teacher can't filter "show me all string problems." A grader later can't weight problems differently.

**Fix:** add `metadata jsonb` now, evolve the shape later. Cheap. Keeps the door open.

---

## 4. Seed is re-runnable as no-op but not as content update

The seed uses `ON CONFLICT (id) DO NOTHING`. If I edit a description in the SQL and re-apply, nothing changes — the database keeps the old text.

For a real authoring workflow, seeds need to be `ON CONFLICT ... DO UPDATE SET` on the mutable content fields (title, description, starter_code, sort_order, is_example, stdin, expected_stdout, …).

**Fix:** rewrite each `INSERT ... ON CONFLICT` clause with a per-table upsert column list. Tedious but mechanical. For now the workaround is "delete the course row first, FK cascades clear the rest."

---

## 5. No problem-authoring UI

Teachers create problems via raw SQL against dev DB, or will have to use a Postgres client. There's no:

- Admin form to create a problem
- Inline markdown preview
- "Run this starter code against all cases" dry-run check
- Test case editor with live expected-output auto-capture

Students have a polished 3-pane IDE; teachers have `psql`. This is fine for v1 bootstrap but the platform isn't self-serve until a teacher can author without help.

---

## 6. Pyodide output vs Piston output drift

The student's **Run** path uses Pyodide in the browser. The **Test** path uses Piston (real CPython) on the server. These can diverge:

- Pyodide uses WebAssembly floats; Piston uses native x86 floats. Most programs are identical, but edge cases exist.
- Pyodide has a subset of the standard library. A `random` seeded the same way might produce different sequences.
- Library availability differs — `numpy` works in Pyodide but needs explicit import in Piston.

For Python 101 problems none of this matters. For anything touching randomness or numerical libs, students can see green on Run and red on Test.

**Mitigation:** document which runtime drives the grading result (Piston) and recommend teachers author problems that don't rely on library-specific output.

---

## 7. Test case `name` field is partly decorative

The UI labels example cases as "Example 1", "Example 2" by ordinal, or uses the case's `name` if set. Hidden cases are always `Hidden #N` regardless of name. Teachers can author a descriptive name ("Negatives", "Empty input") but it doesn't render consistently.

**Fix:** always honor the authored `name` in the results card, fall back to ordinal only when empty. Minor UX polish.

---

## 8. No way to preview what a student sees

Authoring a problem means writing the description, then manually logging in as a student to see how it renders. If the markdown is broken, the description overflows, or the example doesn't render, there's no preview.

**Fix:** `/design/problem-student?problemId={id}` could render the full shell with a chosen problem — teachers use it to spot-check. Trivial to add.

---

## 9. `class_memberships` gate is all-or-nothing per course

Access to a course's problems is granted via `UserHasAccessToCourse` (any class membership in a class of that course). So if a student joins one class and the course has 20 problems, they see all 20.

In practice teachers often want "week-gated" content — this week's problem is unlocked, next week's isn't. Currently impossible without an `assignments`-style layer that the plan intentionally deferred.

**Fix:** the existing `assignments` table (from plan 012) can be wired to `problem_id` later to gate per-problem availability. Small follow-up plan.

---

## Takeaway

For Python 101 as written, every problem works and grades correctly with the seed as-is. But three issues (1 normalize, 2 harness, 5 authoring UI) would make or break the experience as soon as teachers move beyond intro material. Issue 1 is fixable in a one-line PR and should land soon.
