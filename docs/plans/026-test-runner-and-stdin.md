# 026 — Test Runner + Interactive Stdin

**Goal:** Ship the two execution flows from spec 008 — server-side **Test** (Piston, batch, hidden cases) and client-side interactive **Run** (SharedArrayBuffer-backed stdin). Wire teacher snapshot of last test/run over Hocuspocus awareness.

**Architecture:** Test path is a new Go endpoint `POST /api/attempts/{id}/test` that runs the code against every canonical case via the existing Piston client, persists a summary to `attempts.last_test_result`. Run path replaces the buffered-stdin worker with an SAB-Atomics protocol so `input()` is truly blocking. Teacher snapshot reuses the existing Hocuspocus awareness layer — no new server-side state.

**Tech Stack:** Go (Piston already wired) · Web Worker + SharedArrayBuffer + Atomics · Hocuspocus awareness (existing) · Postgres JSONB · Vitest + Playwright.

**Branch:** `feat/026-test-runner-and-stdin`

**Prereqs:** Plan 024 (schema), Plan 025 (student page), Plan 025b (Yjs + teacher watch).

---

## File structure

| File | Responsibility |
|---|---|
| `drizzle/0010_attempts_last_test_result.sql` | `ALTER TABLE attempts ADD COLUMN last_test_result jsonb` |
| `platform/internal/store/attempts.go` | New methods: `UpdateLastTestResult`, `GetLastTestResult` |
| `platform/internal/store/attempts_test.go` | Coverage for the new methods |
| `platform/internal/handlers/attempt_test.go` | New file: `POST /api/attempts/{id}/test` + `POST /api/attempts/{id}/test/{caseId}/diff` |
| `platform/internal/handlers/attempt_test_test.go` | Auth + run-budget + per-case timeout tests |
| `platform/internal/handlers/problems.go` | Inject the runner into `ProblemHandler` (or split into `AttemptHandler` if it grows) |
| `next.config.ts` | Headers: `Cross-Origin-Opener-Policy` + `Cross-Origin-Embedder-Policy` for `/student/.../problems/*` and `/teacher/.../problems/*` |
| `src/workers/pyodide-worker.ts` | Replace buffered-stdin with SAB Atomics-blocked `stdin()` |
| `src/lib/pyodide/use-pyodide.ts` | New `provideStdin(line: string)` callback; preserves the simple-stdin path for batch/test |
| `src/components/editor/output-panel.tsx` | Render an active stdin prompt when the worker is awaiting input |
| `src/components/problem/test-results-card.tsx` | Per-case pass/fail summary (LeetCode-style disclosure) |
| `src/components/problem/problem-shell.tsx` | New "Test" button next to "Run"; renders `TestResultsCard` after Test completes |
| `src/components/problem/teacher-watch-shell.tsx` | Snapshot card on the right pane consuming awareness `last_run` + `last_test_result` |

---

## Tasks

### Task 1: `attempts.last_test_result` schema + store

**Files:**
- Create: `drizzle/0010_attempts_last_test_result.sql`
- Modify: `platform/internal/store/attempts.go`
- Modify: `platform/internal/store/attempts_test.go`

```sql
ALTER TABLE attempts ADD COLUMN last_test_result jsonb;
```

Store methods:

```go
type TestCaseResult struct {
  CaseID     string `json:"caseId"`
  IsExample  bool   `json:"isExample"`
  Status     string `json:"status"`     // "pass" | "fail" | "timeout" | "skipped"
  DurationMs int    `json:"durationMs"`
  Reason     string `json:"reason,omitempty"`  // optional, for non-pass
}

type TestRunSummary struct {
  RanAt   time.Time        `json:"ranAt"`
  Summary struct {
    Passed  int `json:"passed"`
    Failed  int `json:"failed"`
    Skipped int `json:"skipped"`
    Total   int `json:"total"`
  } `json:"summary"`
  Cases []TestCaseResult `json:"cases"`
}

// UpdateLastTestResult persists a fresh run summary to the attempt.
func (s *AttemptStore) UpdateLastTestResult(ctx context.Context, attemptID string, summary TestRunSummary) error
```

Tests:
- Round-trip: write summary, read it back via GetAttempt and assert JSON equality.
- Empty cases array works.
- Schema migration applies cleanly.

- [ ] Apply migration to dev + test DBs.
- [ ] Commit.

---

### Task 2: `POST /api/attempts/{id}/test` handler

**Files:**
- Create: `platform/internal/handlers/attempt_test.go`
- Create: `platform/internal/handlers/attempt_test_test.go`
- Modify: `platform/cmd/api/main.go` — wire the new handler

Auth: owner only (cross-user 404, matches the rest of the attempt API). Body: empty — we use the attempt's stored code + the problem's canonical cases.

Logic:

```go
1. Load attempt; verify owner == claims.UserID (or platform admin).
2. Load problem (for language). Bail if attempt.problem.language not in {"python", "javascript", "java", ...} (whatever Piston supports).
3. Load canonical cases from TestCaseStore.ListCanonical(problemID).
4. Run cases in parallel (concurrency cap 4) with per-case context timeout (3s).
5. Total wallclock budget: 12s. Cases not started by then -> "skipped".
6. For each case, compare actual stdout vs expected_stdout (RTRIM + normalize \r\n -> \n).
7. Persist summary to attempt.last_test_result.
8. Return summary in response.
```

Concurrency: `errgroup.Group` with `SetLimit(4)`. Per-case context: `context.WithTimeout(ctx, 3*time.Second)` derived from a parent context with the 12s budget.

Tests (handler-level, no Piston):
- 401 no claims
- 404 cross-user
- 400 attempt has no canonical cases (return empty summary, status 200, but warn — actually just return the empty summary; no error)
- 200 happy path with a stub Piston that echoes stdin and a single case where expected==stdin

Mock Piston via an interface — the handler accepts `PistonRunner` interface, real impl is `*sandbox.PistonClient`, test impl is a stub.

- [ ] Commit.

---

### Task 3: Diff endpoint for example cases

**Files:**
- Modify: `platform/internal/handlers/attempt_test.go`

```
POST /api/attempts/{id}/test/{caseId}/diff
```

Owner-only. Loads attempt + the specific case. Verifies the case is `is_example == true` (hidden cases never reveal output). Re-runs that one case via Piston. Returns `{ actualStdout, expectedStdout }`. **Not persisted.**

- [ ] Test: 403 when caseId is hidden.
- [ ] Commit.

---

### Task 4: Test button + results card UI

**Files:**
- Create: `src/components/problem/test-results-card.tsx`
- Modify: `src/components/problem/problem-shell.tsx`
- Modify: `src/components/problem/attempt-header.tsx` — add a Test button slot next to Run

UI flow:
1. Click **Test** → button enters loading state.
2. POST `/api/attempts/{currentAttemptId}/test`.
3. On response, render `TestResultsCard` in the right pane (above or replacing the terminal output, by toggle).
4. Failed example cases get an "expand" affordance that calls the diff endpoint and renders side-by-side stdout vs expected.
5. Failed hidden cases show only `Hidden case 3 failed · wrong output`.

`TestResultsCard` shape mirrors the design mock at `/design/problem-teacher` (the teacher view's compact summary).

- [ ] Tests: render with mixed pass/fail, hidden-case redaction, expand-diff fetch behavior.
- [ ] Commit.

---

### Task 5: Cross-origin isolation headers

**Files:**
- Modify: `next.config.ts`

```typescript
async headers() {
  return [
    {
      source: "/student/classes/:classId/problems/:rest*",
      headers: [
        { key: "Cross-Origin-Opener-Policy", value: "same-origin" },
        { key: "Cross-Origin-Embedder-Policy", value: "require-corp" },
      ],
    },
    {
      source: "/teacher/classes/:classId/problems/:rest*",
      headers: [
        { key: "Cross-Origin-Opener-Policy", value: "same-origin" },
        { key: "Cross-Origin-Embedder-Policy", value: "require-corp" },
      ],
    },
  ];
}
```

These enable `crossOriginIsolated` (required for SharedArrayBuffer). Scoped to editor routes only — other routes (Google sign-in popups, etc.) keep current headers.

Verify the Pyodide CDN serves `Cross-Origin-Resource-Policy: cross-origin`. If not, proxy through Next.

- [ ] Commit.

---

### Task 6: SAB-backed interactive stdin

**Files:**
- Modify: `src/workers/pyodide-worker.ts`
- Modify: `src/lib/pyodide/use-pyodide.ts`
- Modify: `src/components/editor/output-panel.tsx`

Protocol:

```
Main thread on Run:
  - Allocate SharedArrayBuffer with control word + 64KB line buffer
  - Pass SAB to worker in the run message
  - Listen for { type: "input_request", prompt } messages

Worker:
  - Pyodide stdin function:
    1. Atomics.wait(controlInt32, 0, 0) until main thread signals
    2. Read length from control word
    3. Decode UTF-8 from buffer
    4. Reset control word to 0
    5. Return string (Python sees as input() result)
  - Before waiting, postMessage({ type: "input_request", prompt }) so the UI shows a prompt

Main thread on user enter:
  - Encode line as UTF-8
  - Write to SAB buffer
  - Set control word length, Atomics.notify
```

OutputPanel: when `running` and the latest worker message is `input_request`, render an inline input field that on Enter calls the hook's `provideStdin(line)`.

The pre-supplied stdin path is preserved: when a Run is invoked with `{ stdin: "<lines>" }`, those lines are pre-fed into the SAB queue before Pyodide can ask. Mixed mode = pre-supplied lines drain first, then prompt for more.

- [ ] Tests: synchronous test in jsdom is hard (no SAB). Use a node test that exercises just the SAB protocol primitives (encoder/decoder + Atomics). Worker-level integration is manual smoke for v1.
- [ ] Commit.

---

### Task 7: Teacher snapshot wiring

**Files:**
- Modify: `src/components/problem/problem-shell.tsx` — broadcast `{ lastRunStdout, lastRunExitCode, lastRunMs }` to provider.awareness after each Run finishes (capped at 8KB stdout). Broadcast `{ testRanAt, summary }` after each Test.
- Modify: `src/components/problem/teacher-watch-shell.tsx` — read awareness for these fields, render a small "Last run · 24s ago" card and the Test summary card on the right.

No server changes. Hocuspocus awareness is already in use for cursor positions in existing session pages — this just adds two fields to the local awareness state.

- [ ] Tests: component test rendering both cards from a fixed awareness snapshot.
- [ ] Commit.

---

### Task 8: JS interactive Run

**Files:**
- Modify: `src/workers/pyodide-worker.ts` (or a new `js-runner-worker.ts`)
- Modify: `src/lib/pyodide/use-pyodide.ts` (rename to `use-runner.ts`?)

For JS: skip the heavy Pyodide path entirely. Use a plain Function() in the worker with `console.log`/`prompt` shims that map onto our existing wire protocol. `prompt()` becomes the same SAB-blocked call as Python's `input()`.

Keep the public hook shape — `runCode(code, { stdin })` works for both languages, switching by `problem.language`.

- [ ] Tests: minimal — just the dispatcher routing.
- [ ] Commit.

---

### Task 9: E2E

**Files:**
- Create: `e2e/test-runner.spec.ts`

- Student opens a problem with 2 examples + 1 hidden case
- Click Test → assert summary card appears with correct pass/fail counts
- Click expand on a failed example → assert side-by-side diff renders
- Skip the SAB stdin flow in E2E (Playwright + SAB is finicky); manual smoke

- [ ] Commit.

---

### Task 10: Verify + review + PR

- [ ] Full Go suite green
- [ ] Vitest green (existing 40 pre-existing failures unchanged)
- [ ] `bun run test:e2e` green
- [ ] Manual smoke of SAB stdin: Python `input()` shows a prompt, accepts typing, Enter resumes the program
- [ ] Code review pass
- [ ] Post-execution report
- [ ] PR open

---

## Out of scope

- **Multi-language support beyond Python + JS for interactive Run.** Compiled languages (Java, C++) get **Test-only**; Run button is hidden/disabled with a tooltip.
- **Real PTY / xterm.js.** Spec 009 territory.
- **Submission flow** — assigning the attempt to an Assignment + writing a graded `submissions` row. Future plan once Assignment ↔ Problem linkage exists.
- **Custom-stdin during interactive Run.** The Inputs panel's stdin pre-feed still works; once the buffer drains, the SAB prompt takes over. Mid-run "edit my custom stdin" is not a v1 feature.

## Risks

- **COOP/COEP headers break embedded widgets.** Scoped to editor routes only, but verify nothing in the `(portal)` shell needs cross-origin embeds (sign-in popups are on `/login`, not on editor pages — fine).
- **SAB browser support.** Modern Chromium and Firefox have it. Safari is touchier. K-12 deployment is mostly Chromebooks → fine.
- **Piston quotas.** A class hammering Test simultaneously could rate-limit our shared Piston instance. Add a per-user concurrency cap in the handler if it becomes an issue (note as follow-up; not in this plan).
- **Hidden-case info leak via timing.** A student could time a hidden case to infer characteristics. Not a concern for K-12; would be for competitive-prog future. Tracked but deferred.
