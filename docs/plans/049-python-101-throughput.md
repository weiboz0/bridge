# Plan 049 — Authoring throughput notes

Reference for Phase 4 estimation.

## Phase 3 canary (2026-04-30)

**Unit shipped:** `print-and-comments` (1 unit, 2 problems, 3 test cases, 1 topic).

The canary was authored in a single agentic session, so per-task minute breakdown is approximated rather than measured. The number that matters for Phase 4 estimation is the bound: **each unit-with-2-problems took on the order of one focused authoring pass**, NOT several days of iterative review. The schema + validator + importer (built in Phases 1+2) made authoring mechanical — no DB-shape guessing during writing.

| Activity | Approx minutes (canary) | Notes |
|---|---|---|
| YAML scaffolding (course.yaml + unit shell) | ~5 | uuidv4 generation + filling schema-required fields |
| Unit prose (Big idea / Worked example / pitfalls / vocabulary) | ~10 | Plan 049 calls out the block structure; pure writing |
| Problem 1 description + starter + solution | ~3 | Hello-world is trivial |
| Problem 1 test cases (1 example + 1 hidden) | ~2 | Mirror the legacy seed |
| Problem 2 description + starter + solution | ~5 | Three-lines, multi-line expected stdout |
| Problem 2 test cases (1 example) | ~2 | Verifying the `\|-` block-scalar handling |
| Validator round-trip | ~1 | First run was clean |
| Importer dry-run + --apply (bridge_test + bridge) | ~3 | Includes the post-insert SELECT verification |
| **Total** | **~31** | |

## Phase 4 projection (preliminary)

The remaining 11 units, by complexity (best-guess):

| Unit | Problems | Authoring effort vs. canary | Cumulative minutes |
|---|---|---|---|
| 1. print-and-comments | 2 | (canary) | 31 |
| 2. variables-and-types | 2-3 | similar | +35 |
| 3. arithmetic-and-operators | 2-3 | similar | +35 |
| 4. strings | 2-3 | medium (slicing edge cases) | +45 |
| 5. conditionals | 2-3 | similar | +35 |
| 6. loops | 2-3 | medium (range corners) | +45 |
| 7. lists | 2-3 | medium (mutation rules) | +50 |
| 8. dicts-and-sets | 2-3 | medium-hard | +60 |
| 9. functions | 3 | hard (def vs scope teach) | +75 |
| 10. files-and-exceptions | 2-3 | hard (try/except authoring) | +75 |
| 11. classes-and-objects | 2-3 | hard (state, methods) | +80 |
| 12. capstone | 1-2 long | very hard (mixed concepts) | +90 |

**Total projection: ~625 minutes ≈ 10.5 hours of focused authoring.**

This is well below the >120h "split into a separate plan" threshold from plan 049. The bottleneck is NOT authoring — it's:

1. **Sandbox verification** (currently blocked by Piston runtime — see Phase 3 status in plan 049). Each problem's reference solution × test cases = 1 Piston run. ~50 problems × ~5 cases each = ~250 Piston calls per import. Each call is ~500ms, so ~2 minutes of compute total. Trivial.

2. **Browser smokes** (manual). Phase 3 has 2 smokes; Phase 5 has full-scale verification. Each smoke = ~10 minutes of human time. Plan 049's full-scale verification step lists 6+ checkpoints; budget ~90 minutes.

3. **Pyodide ↔ Piston drift gotchas** that surface during Phase 5. Hard to estimate ahead of time. Reserve 2-4 hours.

## Recommendations for Phase 4

- **Author 1 unit per session.** Time-box each unit to 30-45 minutes. If a unit overruns, that's a signal the unit is doing too much and should be split.
- **Run the validator after every problem**, not just at unit-end. Reduces feedback latency.
- **Defer Piston pre-flight to a single end-of-Phase-4 batch** to avoid context-switching costs. The `--skip-sandbox` flag is fine during authoring; the full sandbox check is the Phase 5 entry gate.
- **Don't write hidden cases until the example case passes Piston.** A reference solution that prints "Pass\n" instead of "Pass" is a class of bug that's easier to catch on a single example case than buried in 4 hidden ones.

## Re-estimation gate

If, after authoring 4 units (units 1-4), the actual cumulative time exceeds **3 hours**, re-evaluate:

- Is each unit's structure causing rework? (Maybe scale back the prose blocks.)
- Are problem descriptions ballooning into multi-paragraph essays? (Aim for 2-3 sentences + I/O block.)
- Is the schema friction-free, or are authors fighting Zod? (If the latter, file a Phase 1.5 plan.)
