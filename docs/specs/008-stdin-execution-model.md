# 008 — Stdin Execution Model

## Problem

The current Pyodide worker streams stdout / stderr but has no path for `input()`: a Python program that calls `input("name? ")` hangs the worker. Piston (server-side) supports stdin as a one-shot blob but isn't wired into a multi-case test runner. Spec 007 reserved a Run / Test surface in the UI; this spec defines what happens when those buttons are pressed.

Two flows, two backends:

- **Run** — interactive. Browser worker. Streams output character-by-character. When the program asks for stdin, the terminal prompts; the user types; the worker resumes. Used for all human-driven exploration and debugging.
- **Test** — batch. Server-side Piston. One submission per canonical test case, each with the case's stdin piped in. Captures full stdout, compares to `expected_stdout`, returns pass/fail per case. Hidden cases are graded but never disclosed in detail.

## Design

### Run path (interactive, client-side)

**Worker boundary.** The existing `src/workers/pyodide-worker.ts` is extended. JS-language Run reuses the same boundary with a different in-worker interpreter.

**Stdin via SharedArrayBuffer + Atomics.** Pyodide's stdin hook can be synchronous when backed by a SAB. The worker calls `Atomics.wait`; the main thread writes the user's typed line into the buffer and signals. This is the only correct primitive — synchronous XHR is deprecated and `postMessage` is async, which doesn't satisfy `input()`'s blocking contract. Requires the page to be cross-origin isolated; see "Headers" below.

**Wire protocol** (worker ⇄ main):

```
worker → main:
  { type: "stdout", text }
  { type: "stderr", text }
  { type: "input_request", prompt }     // also flushed to terminal as a label
  { type: "done", id, exitCode, durationMs }

main → worker:
  { type: "init" }
  { type: "run", id, code, language, stdinBuffer? }
  { type: "stdin", text }               // legacy postMessage path; SAB is primary
  { type: "abort", id }
```

`stdinBuffer` is a `SharedArrayBuffer` allocated by the main thread when Run is invoked, used for the synchronous read. The stdin SAB has two slots: a control word (`Atomics.notify` target) and a UTF-8 line buffer with a length prefix. Cap line length at 64 KB.

**Inputs panel preload.** When the student selects an input case from the chip strip (canonical example, private case, or custom textarea), Run begins with that stdin pre-queued. Each `input()` call drains a line from the queue. When the queue runs dry, the terminal prompts interactively. This makes "run with example then continue interactively" a single button press.

**Cancellation.** A "Stop" affordance in the terminal sends `{ type: "abort", id }`. The worker terminates the current execution by re-creating the Pyodide micropython context (Pyodide does not support cooperative cancel). Last-resort: terminate the worker and re-init.

**Caps.**

| Knob | Value | Why |
|---|---|---|
| Output total | 100 KB stdout + 100 KB stderr | Browser DOM degrades past this; truncated with `… [output truncated]`. |
| Wallclock | 30 s, configurable per problem later | K-12 friendly; competitive workloads can lift this. |
| Idle wait on `input()` | indefinite | The user is at the keyboard; nothing else to do. Only counts against wallclock when *not* waiting. |
| Per-line stdin | 64 KB | Matches the SAB buffer slot. |

### Test path (batch, server-side)

**One Piston submission per case.** The Go API receives `POST /api/attempts/{id}/test` and runs canonical cases in parallel (concurrency cap = 4). For each case it calls `PistonClient.ExecuteWithStdin(language, code, case.stdin)`, captures stdout, and compares to `case.expected_stdout` after RTRIM-and-newline-normalize (`expected.trimEnd() == actual.trimEnd()` with `\r\n` → `\n`).

If `expected_stdout` is `NULL`, the case is informational — the run is reported as `executed` (not pass/fail) and the actual output is shown. Used for "does this even compile" sanity cases.

**Per-case timeout: 3 seconds. Total budget: 12 seconds.** A case that exceeds 3s is reported as `timeout`. If the total budget is exhausted, remaining cases are reported as `skipped`.

**Output disclosure.**

| Case visibility | On pass | On fail |
|---|---|---|
| Example | nothing extra | actual output + expected (side-by-side diff in the UI) |
| Hidden | nothing | "Hidden case 3 failed · wrong output" — no actual, no expected |
| Hidden, timeout | "Hidden case 3 timed out" | same |

This is the LeetCode model: students see enough to debug examples, never enough to memorize hidden cases.

**Persistence.** Add one column to `attempts`:

```
ALTER TABLE attempts ADD COLUMN last_test_result jsonb;
```

Shape:

```json
{
  "ranAt": "2026-04-17T19:22:01Z",
  "summary": { "passed": 3, "failed": 1, "skipped": 0, "total": 4 },
  "cases": [
    { "caseId": "...", "isExample": true,  "status": "pass",    "durationMs": 12 },
    { "caseId": "...", "isExample": true,  "status": "pass",    "durationMs":  9 },
    { "caseId": "...", "isExample": false, "status": "pass",    "durationMs": 11 },
    { "caseId": "...", "isExample": false, "status": "fail",    "durationMs": 18, "reason": "wrong_output" }
  ]
}
```

`actual_stdout` is **not** stored. We re-render diffs on the fly for example failures by re-running the example case if the student opens the diff (a sub-second operation). This avoids storing student output indefinitely and keeps the table compact.

**No Run history persisted.** Run output is ephemeral — lives in the client's terminal until cleared.

### Teacher observation

**Snapshot, not stream.** When the teacher opens the watch page, they see:

- The student's editor (live via Yjs; spec 007 covered this).
- The student's `last_test_result` (fetched via API; refreshed on awareness `test_completed` ping).
- The student's *last completed Run* output, capped at 8 KB and broadcast over Hocuspocus awareness as `last_run` after the Run finishes.

We do **not** stream stdout/stderr character-by-character to the teacher. That would dominate the awareness channel and interleave noisily across multiple students. The teacher sees what the student saw a moment ago — close enough for in-session support without hammering the network.

When the teacher pins to a non-active Attempt, they see that Attempt's `last_test_result` only; no Run output (Runs aren't persisted).

### Headers (Next.js cross-origin isolation)

SharedArrayBuffer requires the page be cross-origin isolated:

```
Cross-Origin-Opener-Policy:   same-origin
Cross-Origin-Embedder-Policy: require-corp
```

`next.config.ts` will set these on `/student/classes/*/problems/*` and `/teacher/classes/*/problems/*`. Other routes are unaffected — these headers break some embeddable third-party widgets (Google sign-in popups, etc.), so we scope them to the editor surface only.

The Pyodide CDN serves correct CORP headers; if a future asset doesn't, we proxy it through Next.

### API additions

```
POST /api/attempts/{id}/test
  body: {}                          // server uses the attempt's plain_text + problem.language
  response: { summary, cases[] }    // same shape persisted to last_test_result
  side effect: writes attempts.last_test_result

POST /api/attempts/{id}/test/{caseId}/diff
  for example cases only; re-runs that one case and returns
  { actualStdout, expectedStdout }  // never persisted
```

`/api/attempts/{id}/run` is intentionally **not** added. Run is client-side only.

### Languages in scope

Python (Pyodide) and JavaScript (in-worker Function eval with `prompt()` shim) for v1. Test uses Piston for both, plus any other language Piston supports — so a teacher *can* author a Problem with `language: "java"` and Test will work, but Run will be unsupported and the UI will hide the Run button (showing "Test only" badge). Real interactive run for compiled languages is spec 009 territory (xterm.js + PTY).

### Safety summary

- **Browser sandbox** for Run: Pyodide and the JS worker have no FS, no network unless explicitly granted. We grant nothing.
- **Piston container** for Test: existing isolation, already in use elsewhere on the platform.
- **No code paths share secrets**: the test endpoint loads canonical cases (server-side query) and the attempt's code (server-side query) before calling Piston; the wire never carries credentials.
- **Output truncation** at every layer prevents memory blow-ups in the browser and in Postgres.
- **Stdin is never a shell**: the SAB protocol carries plain UTF-8 lines, not commands.

### Non-goals

- Real PTY / xterm.js / interactive compiled-language Run (spec 009).
- Custom test-case authoring tools for teachers (covered by spec 006's plan).
- Output streaming to the teacher per-character.
- Preserving Run history across page reloads.
- Side-channel limiting (CPU pinning, fork limits) — Piston handles container-level; browser handles tab-level.

## Migration

```sql
ALTER TABLE attempts ADD COLUMN last_test_result jsonb;
```

No backfill. Existing `documents`-based execution paths are untouched.

## Rollout

Phased to interleave with spec 006 / 007 implementation:

1. **Pyodide stdin** — extend the existing worker with the SAB protocol; add the cross-origin-isolation headers; wire to the redesigned terminal panel.
2. **Test endpoint** — `POST /api/attempts/{id}/test` calling Piston in parallel with caps + truncation; persist `last_test_result`.
3. **Diff endpoint + UI** — example-case diff on demand.
4. **Teacher snapshot wiring** — broadcast `test_completed` and `last_run` over Hocuspocus awareness.
5. **JS interactive Run** — `prompt()` shim; same wire protocol.

(1) and (2) can land in the same PR; (3)–(5) are smaller follow-ups.
