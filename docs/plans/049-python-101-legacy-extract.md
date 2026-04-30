# Plan 049 — Legacy Python 101 extract (audit notes)

Source: `scripts/seed_python_101.sql` (795 lines, broken since plan 046).

## Why broken

- `topics.lesson_content` column no longer exists (verified against live schema). Inserts at lines 49–57 fail.
- `teaching_units` rows worked at write time but their UPSERT predicate (`ON CONFLICT (topic_id) WHERE topic_id IS NOT NULL DO NOTHING`) is fragile if a unit pre-exists with a different UUID — Codex pass-2 risk.

The seed is **not loaded by any runtime path** — `git grep "seed_python_101"` returns only `docs/`. Safe to retire after Python 101 importer lands.

## Re-use scope

Salvage as **starting drafts only.** Authors will rewrite descriptions for clarity, add lesson notes (Pyodide↔Piston drift constraints, no-emoji policy, etc.), and treat all UUIDs as throwaway — the new format uses fresh uuidv4 `id:` fields per Phase 1.

The 12 problems below should each map to **one Python 101 unit** in the new structure (1 unit ≈ 1 topic ≈ 1 problem-cluster). Plan 049 targets ~12 units total, and the legacy 12-problem set lines up surprisingly well with that goal — though several units will need 1–2 additional problems for depth.

## Topic → unit mapping (proposed)

| Legacy topic | Proposed unit slug | Problems carried over | Notes |
|---|---|---|---|
| 1. Hello, World | `unit-01-hello-world` | hello-world, three-lines | Add a "Print with quotes/escapes" problem |
| 2. Variables & Input | `unit-02-variables-input` | greet-by-name, name-then-age | Drop emoji from hidden case (Piston Python image's stdout encoding may differ from Pyodide) |
| 3. Numbers & Arithmetic | `unit-03-arithmetic` | sum-two-numbers, area-rectangle | Add a divmod / floor-divide problem |
| 4. Conditionals | `unit-04-conditionals` | even-or-odd, pass-or-fail | Add a 3-way branch (positive / negative / zero) |
| 5. Loops | `unit-05-loops` | count-to-n, fizzbuzz-short | Add `sum 1..N` (introduces accumulator pattern before lists) |
| 6. Lists | `unit-06-lists` | sum-of-list, max-in-list | Add a "find first occurrence" problem |

That's 6 units / 12 carried problems / ~6 new problems = 18 problems and 6 units. Plan 049 targets 12 units, so phases 4 will need to plan **6 more units** beyond the legacy salvage:

- `unit-07-strings` (string indexing, `len`, slicing, simple methods)
- `unit-08-functions` (def, return, parameters)
- `unit-09-strings-loops` (count vowels, reverse a string the long way)
- `unit-10-dictionaries` (key-value lookup, count occurrences)
- `unit-11-files-or-multilinput` (read N lines, process)
- `unit-12-recap-mini-project` (combines several earlier ideas)

(These six are placeholders — phase 4 brainstorming will refine.)

## Per-problem extract

Reference solutions in the legacy seed all pass against CPython 3.10 (no Pyodide-only features used), so they're a safe baseline for Piston verification.

### 1.1 Hello, World

- Difficulty: easy. Tags: output, print.
- Description: "Print the exact text `Hello, World!`. Use the `print` function. No input."
- Starter: `print("Hello, World!")\n`
- Solution: `print("Hello, World!")\n`
- Test cases: 1 example (empty stdin → `Hello, World!`), 1 hidden (same).

### 1.2 Three Lines

- Difficulty: easy. Tags: output, print.
- Description: "Print three lines exactly: `line 1`, `line 2`, `line 3` (each on its own line)."
- Starter: `# Print each line on its own line.\n`
- Solution: 3× `print` calls.
- Test cases: 1 example.

### 2.1 Greet by Name

- Difficulty: easy. Tags: input, variables, strings.
- Description: "Read a single name from input and print `Hello, {name}!`."
- Starter / solution: `name = input(); print(f"Hello, {name}!")`.
- Test cases: 2 examples (Ada, Grace Hopper), 2 hidden (Ada 💡 — *flagged for emoji review*, single-char X).

### 2.2 Name Then Age

- Difficulty: easy. Tags: input, variables, strings.
- Inputs over two lines, output: `{name} is {age} years old.`
- Test cases: 1 example (Ada/23), 2 hidden (Grace/85, Baby/0).

### 3.1 Sum Two Numbers

- Difficulty: easy. Tags: arithmetic, integers.
- Read two ints, print sum.
- Test cases: 2 examples (3+4, 10−3), 3 hidden (negatives, zero, big).

### 3.2 Area of a Rectangle

- Difficulty: easy. Tags: arithmetic, integers.
- Read w, h ints, print area.
- Test cases: 1 example (3×5), 2 hidden (square, one dim).

### 4.1 Even or Odd

- Difficulty: easy. Tags: conditionals, modulo.
- Read int, print `Even` or `Odd`.
- Test cases: 2 examples (4, 7), 2 hidden (0 → Even, −5 → Odd).

### 4.2 Pass or Fail

- Difficulty: easy. Tags: conditionals.
- Read score 0–100, print Pass (≥60) or Fail.
- Test cases: 2 examples (75, 45), 3 hidden (60, 0, 100).

### 5.1 Count to N

- Difficulty: easy. Tags: loops, range.
- Read N, print 1..N each on its own line.
- Test cases: 1 example (4), 2 hidden (1, 10).

### 5.2 FizzBuzz (short)

- Difficulty: medium. Tags: loops, conditionals, modulo.
- Standard FizzBuzz to N.
- Test cases: 2 examples (5, 15), 2 hidden (3, 1).

### 6.1 Sum of a List

- Difficulty: easy. Tags: lists, builtins.
- Read space-separated ints, print sum.
- Test cases: 1 example, 3 hidden (singleton, negatives sum to 0, all zeros).

### 6.2 Max in a List

- Difficulty: medium. Tags: lists, loops.
- Read space-separated ints, print max (without `max`).
- Test cases: 1 example, 3 hidden (all-negative, singleton, duplicates).

## Authoring carry-overs (drift / safety)

Two specific changes to make when authoring v2:

1. **Emoji in `2.1 Greet by Name` hidden case.** Piston Python images on different host kernels can normalize stdout encoding differently. Replace with an ASCII multi-word edge case (e.g., `"Doctor Strange"`). If we want to test unicode handling, do it in a dedicated unit later, not in unit 2.

2. **`Pass or Fail` description** says "0–100" but doesn't enforce. The current canonical solution doesn't validate. Either add validation language ("input is always 0–100 inclusive") or add a hidden out-of-range case — pick one. Recommendation: add the validation note; this is unit 4 (conditionals), not unit X (input validation).

## Demo seed audit (`scripts/seed_problem_demo.sql`)

- 1 course "Intro to Python — Problem Demo", 2 topics, 4 problems. Org-scoped (Bridge Demo School). 324 lines.
- Overlap with Python 101: minimal — different course id, different problem set. Topics ("Warm-ups", "Arrays") are subset-style, not duplicates.
- **Decision:** keep `seed_problem_demo.sql` for now; it's used by demo flows that don't need a full curriculum. It also has the same `topics.lesson_content` issue, BUT the demo seed only inserts 1 topic with `lesson_content`, so the breakage is limited to 1 row.
- Phase 6 follow-up: re-evaluate whether `seed_problem_demo.sql` should also be replaced with a YAML import. **Out of scope for plan 049.** Filed as a Phase 6 footnote, not a separate plan.

## Phase 6 retirement plan (re-stated from plan 049)

After Python 101 import lands and the demo class is wired to the cloned course:

1. `git rm scripts/seed_python_101.sql`. No runtime callers — clean delete.
2. Update `docs/setup.md` if it mentions the legacy seed. Search: `grep -rn "seed_python_101" docs/`.
3. `seed_problem_demo.sql` stays. Possibly retire in plan 050+ once we have authoritative demo-class wiring.
