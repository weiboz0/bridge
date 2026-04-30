# Python 101 — Authoring Guide

> Plan 049. Source-of-truth for the Bridge HQ Python 101 curriculum.

The course lives as a small YAML tree:

```
content/python-101/
├── course.yaml            # course manifest: id, topics order, unit slugs
├── units/
│   ├── print-and-comments.yaml
│   ├── variables-and-types.yaml
│   ├── ...
│   └── capstone.yaml
└── README.md              # this file
```

Each unit YAML carries:
- unit metadata (title, description, grade level, tags),
- inline `blocks` (heading / paragraph / list / callout / code / problem-ref),
- one or more `problems`, each with starter code, a reference solution, and test cases.

The validator (`bun run content:python-101:validate`) enforces both the per-file Zod schema and cross-file invariants. The importer (`bun run content:python-101:import`) is a precondition of grading correctness — every reference solution is run through Piston before the database transaction opens, and a solution that doesn't pass its own tests aborts the import.

---

## Stable identity (required reading)

Each unit, problem, topic, and course carries an explicit `id:` field — a uuidv4 generated **once** at file creation time. Keep these UUIDs stable forever:

- **Renaming a `slug:`** is allowed (uses `--allow-rename` at import). Keep the `id` the same.
- **Renaming an `id:`** is forbidden — the importer treats it as a brand-new entity, leaves the old row alone, and likely strands its references.

To generate a new uuidv4 from the shell:

```bash
bun -e 'console.log((await import("uuid")).v4())'
```

(Plan 049 doesn't ship a `bun run new-uuid` helper. If you find yourself running that one-liner often, file plan 050+.)

---

## YAML parsing policy

The parser is configured with `parseDocument(input, { merge: false, schema: 'core' })` and the validator additionally rejects YAML that uses anchors, aliases, or merge keys (`<<: *base`). Multiline strings must use `|`-block scalars. This keeps content diffs human-readable, makes review tractable, and keeps the importer simple.

**Allowed:**
```yaml
solution:
  language: python
  code: |
    name = input()
    print(f"Hello, {name}!")
```

**Disallowed:**
```yaml
shared: &shared            # anchor — rejected
  - one
  - two
first: *shared             # alias — rejected
```

---

## Test-case authoring (Pyodide ↔ Piston drift)

Server-side grading runs on **Piston / CPython** (`platform/internal/sandbox/piston.go`), but students "Run" their code in the browser via **Pyodide**. The two runtimes are not identical. Authoring rules:

1. **Every problem MUST have a deterministic single-correct-output.** No "any non-empty line" semantics — Bridge's grader is exact-string match (with whitespace normalization). Where a real-world problem might allow multiple outputs (e.g., word-cloud ordering), constrain it ("print words in descending count, ties broken alphabetically").

2. **Avoid Pyodide-only modules** (`micropip`, `pyodide.ffi`, `js`, etc.). Stick to the standard library subset that Piston's CPython image exposes (3.10+ baseline).

3. **Avoid emoji or other extended-Unicode bytes in expected stdout** unless you've verified them through the importer. The Piston Python image's locale and stdout encoding can differ from Pyodide on user platforms. The legacy `Greet by Name` problem's emoji hidden case is being dropped for this reason.

4. **No randomness, no time-of-day, no I/O.** A reference solution that passes once must pass forever.

5. **No trailing newlines in `expectedStdout`.** Python's `print()` adds one; the grader normalizes it. Write outputs as the visible string the user expects to see, ending with the last printed line:

   ```yaml
   testCases:
     - name: Example
       stdin: ""
       expectedStdout: "Hello, World!"
       isExample: true
   ```

6. **Hidden vs example.** Set `isExample: true` for cases shown inline to students. Set `isExample: false` for hidden grader cases. Every problem must have at least one example (validator-enforced).

---

## Running the toolchain locally

The importer talks to Piston via a tiny Go shellout (`platform/cmd/run-piston`, Phase 2). Piston must be reachable.

```bash
# 1. Bring up Piston (one-time):
docker run -d --rm --name piston -p 2000:2000 ghcr.io/engineer-man/piston

# 2. Install the Python 3.10 image (one-time):
curl -X POST http://localhost:2000/api/v2/packages \
  -H 'Content-Type: application/json' \
  -d '{"language":"python","version":"3.10.0"}'

# 3. Validate the content tree (no DB writes):
bun run content:python-101:validate

# 4. Run the full importer (Phase 2+ — DB writes; transactional).
PISTON_URL=http://localhost:2000 \
  bun run content:python-101:import --apply

# 5. Or just import the platform-scope library (no Bridge HQ course):
bun run content:python-101:import --apply --library-only
```

Set `PISTON_URL` to override the default `http://localhost:2000`.

---

## Where the content lands in the database

The full mapping lives in `docs/plans/049-python-101-curriculum.md` ("Importer DB-field mapping"). Short version:

| YAML | DB row | Notes |
|---|---|---|
| `units/<slug>.yaml` | `teaching_units` (scope='platform') | one row per file, `topic_id=NULL` until course pass links it |
| `units/*.yaml#blocks` | `unit_documents.blocks` (jsonb) | document-shape JSON the editor renders |
| `units/*.yaml#problems[]` | `problems` (scope='platform') + `problem_solutions` + `test_cases` | reference solution stored, never shown to students |
| `course.yaml` | `courses` (org_id=Bridge HQ) + `topics` + `topic_problems` | `topic.id` decoupled from unit id |

The 1:1 unit↔topic relation (plan 044's `teaching_units_topic_id_uniq` partial-unique index) is preserved by Pass 3 (`LinkUnitToTopic`).

---

## Reviewing changes

The validator's exit code is part of the review story. Run it locally before opening a PR:

```bash
bun run content:python-101:validate
```

If a problem changes test cases or expected output, also run the full importer against `bridge_test` to verify the reference solution still passes:

```bash
PISTON_URL=http://localhost:2000 \
  TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test \
  bun run content:python-101:import --apply --target-db=bridge_test
```

(The `--target-db` flag is Phase 2; document the actual flag name in this README once Phase 2 lands.)
