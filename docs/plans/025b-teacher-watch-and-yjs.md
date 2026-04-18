# 025b — Teacher Watch + Yjs on Attempts

**Goal:** Ship the teacher live-watch page from spec 007 and bring real-time co-editing to the student page. Today the student page autosaves over plain HTTP; this plan switches it to Yjs so teachers can see typing live and so multi-tab edits converge cleanly.

**Architecture:** Each Attempt becomes its own Yjs document keyed `attempt:{id}`. Hocuspocus is extended with auth + persistence for that prefix; the student page uses the existing `useYjsProvider` hook. The teacher watch page is a read-only mirror that follows the student's currently-focused attempt over Hocuspocus awareness.

**Tech Stack:** Hocuspocus (server) · Yjs + y-monaco (client) · Go (one new endpoint, one new store query).

**Branch:** `feat/025b-teacher-watch-and-yjs`

**Prereqs:** Plans 024 + 025 merged.

---

## File structure

| File | Responsibility |
|---|---|
| `drizzle/0009_attempts_yjs.sql` | Add `attempts.yjs_state text` column |
| `server/attempts.ts` | New helpers: `loadAttemptYjsState`, `storeAttemptYjsState` (mirror `server/documents.ts`) |
| `server/hocuspocus.ts` | Extend `onAuthenticate`, `onLoadDocument`, `onStoreDocument` to handle `attempt:*` |
| `platform/internal/store/attempts.go` | New `ListAttemptsForTeacherView` query — joins attempts with class membership for teacher access |
| `platform/internal/handlers/teacher_problems.go` | New `GET /api/teacher/problems/{id}/students/{studentId}/attempts` endpoint |
| `platform/internal/handlers/teacher_problems_test.go` | Auth-guard + cross-class isolation tests |
| `src/lib/problem/use-attempt-yjs.ts` | Wraps `useYjsProvider` with `attempt:{id}` doc-name + token plumbing |
| `src/components/problem/problem-shell.tsx` | Replace plain autosave with Yjs binding; broadcast `{problemId, attemptId}` over awareness |
| `src/lib/problem/use-autosave-attempt.ts` | Becomes a fallback/migration helper — kept around for the unauthenticated/preview path; primary path is Yjs |
| `src/app/(portal)/teacher/classes/[id]/problems/[problemId]/students/[studentId]/page.tsx` | Teacher watch page (server component shell) |
| `src/components/problem/teacher-watch-shell.tsx` | Client-side shell — attempt cards row + read-only Monaco + compact terminal |
| `src/components/problem/attempt-cards-row.tsx` | The 3-card horizontal strip from the design mock |

---

## Tasks

### Task 1: Schema — `attempts.yjs_state`

**Files:**
- Create: `drizzle/0009_attempts_yjs.sql`

```sql
ALTER TABLE attempts ADD COLUMN yjs_state text;
```

- [ ] Apply to dev + test DBs.
- [ ] No store changes needed yet — Hocuspocus is the only writer.
- [ ] Commit.

---

### Task 2: Hocuspocus persistence for `attempt:*`

**Files:**
- Create: `server/attempts.ts`
- Modify: `server/hocuspocus.ts`

- [ ] **`server/attempts.ts`** — mirror `server/documents.ts` shape:

```typescript
import { eq, sql } from "drizzle-orm";
import { pgTable, uuid, text, timestamp, varchar } from "drizzle-orm/pg-core";
import { serverDb } from "./db";

const attempts = pgTable("attempts", {
  id: uuid("id").primaryKey(),
  problemId: uuid("problem_id").notNull(),
  userId: uuid("user_id").notNull(),
  yjsState: text("yjs_state"),
  plainText: text("plain_text").notNull().default(""),
  language: varchar("language", { length: 32 }).notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).defaultNow().notNull(),
});

export async function loadAttemptYjsState(attemptId: string): Promise<string | null> {
  const [row] = await serverDb.select({ yjsState: attempts.yjsState }).from(attempts).where(eq(attempts.id, attemptId));
  return row?.yjsState ?? null;
}

export async function storeAttemptYjsState(attemptId: string, yjsState: string, plainText: string): Promise<void> {
  await serverDb.execute(sql`
    UPDATE attempts SET yjs_state = ${yjsState}, plain_text = ${plainText}, updated_at = now()
    WHERE id = ${attemptId}
  `);
}

export async function loadAttemptOwner(attemptId: string): Promise<{ userId: string; problemId: string } | null> {
  const [row] = await serverDb.select({ userId: attempts.userId, problemId: attempts.problemId }).from(attempts).where(eq(attempts.id, attemptId));
  return row ?? null;
}
```

- [ ] **`server/hocuspocus.ts`** — extend `onAuthenticate`, `onLoadDocument`, `onStoreDocument`:

```typescript
// onAuthenticate: parse "attempt:{id}", look up the attempt, allow if:
//   - userId == attempt.user_id (owner read+write), OR
//   - role is teacher AND user is enrolled as instructor in a class whose course
//     contains the problem's topic (read-only — teacher.connection.readonly = true)
// To avoid wide blast radius, the teacher check delegates to the Go API rather
// than recomputing here.

// onLoadDocument: if "attempt:{id}", call loadAttemptYjsState
// onStoreDocument: if "attempt:{id}", call storeAttemptYjsState
```

The teacher access check requires a server-to-server call to Go: `GET /api/teacher/problems/.../attempts` (see Task 4) returning whether the caller is allowed to read this attempt. To keep auth latency low, cache the answer in Hocuspocus connection context for the lifetime of the WebSocket.

- [ ] **Tests** — `server/__tests__/attempts.test.ts` (or a Vitest spec under `tests/server/`): mock the DB, assert the right SQL fires.

- [ ] Commit.

---

### Task 3: Student page switches to Yjs

**Files:**
- Create: `src/lib/problem/use-attempt-yjs.ts`
- Modify: `src/components/problem/problem-shell.tsx`

`useAttemptYjs(attemptId)` wraps `useYjsProvider({ documentName: \`attempt:${attemptId}\`, token })` and returns `{ yText, provider, connected }`. On mount it asserts `attemptId` non-null — if there's no Attempt yet, the page falls back to a "starter code, click to start coding" path (creating an Attempt is now a server-side POST kicked off by a single first-keystroke event, then the editor reconnects to the new attempt's room).

The hook eats the Yjs doc binding into the `<CodeEditor>`'s `yText` + `provider` props, replacing the `onChange={setCode}` plumbing. `useAutosaveAttempt` is no longer the autosave engine — Hocuspocus handles persistence.

- [ ] **First-keystroke create** — on the very first edit when no Attempt exists, POST `/api/problems/{id}/attempts` with the current Monaco text, then immediately swap the Hocuspocus doc to `attempt:{newId}`. (Yjs doc name change forces a reconnect; that's fine.)

- [ ] Validate that Monaco doesn't lose the keystrokes during the swap (small edge case — buffer them client-side and replay into the new Y.Text).

- [ ] **Tests** — `tests/unit/use-attempt-yjs.test.ts`: uses a Hocuspocus mock provider; asserts doc name is `attempt:{id}`, asserts no connection when `attemptId === null`.

- [ ] Commit.

---

### Task 4: Teacher access endpoint

**Files:**
- Create: `platform/internal/handlers/teacher_problems.go`
- Modify: `platform/internal/store/attempts.go` — add `ListAttemptsForTeacherView`
- Create: `platform/internal/handlers/teacher_problems_test.go`

```
GET /api/teacher/problems/{problemId}/students/{studentId}/attempts
```

Returns the student's attempts for this problem, ordered by `updated_at DESC`. Auth: caller must be the teacher of a class whose course contains the topic containing the problem AND the student must be a member of one of those classes.

The Hocuspocus auth check (Task 2) reuses this same access policy via a parallel internal helper or a HEAD on this URL.

```go
type TeacherProblemHandler struct {
  Problems *store.ProblemStore
  Topics   *store.TopicStore
  Courses  *store.CourseStore
  Classes  *store.ClassStore
  Attempts *store.AttemptStore
}

// Routes — registered under RequireAuth in main.go
func (h *TeacherProblemHandler) Routes(r chi.Router) {
  r.Get("/api/teacher/problems/{problemId}/students/{studentId}/attempts", h.ListStudentAttempts)
}
```

Tests: 401 no-claims, 403 not-a-teacher-of-this-class, 200 happy-path with attempts in the right order.

- [ ] Commit.

---

### Task 5: Teacher watch page

**Files:**
- Create: `src/app/(portal)/teacher/classes/[id]/problems/[problemId]/students/[studentId]/page.tsx`
- Create: `src/components/problem/teacher-watch-shell.tsx`
- Create: `src/components/problem/attempt-cards-row.tsx`

Layout matches `/design/problem-teacher`:

- LEFT (26%): `ProblemDescription` (already built — reuse) + a small "Watching" card with the student's name + live indicator.
- CENTER (48%): `AttemptCardsRow` — top-3 most-recent attempts as cards; selecting a card pins the watch to that attempt. Below the row, a read-only `<CodeEditor readOnly yText={...} provider={...} />` keyed on `attempt:{currentAttemptId}`.
- RIGHT (26%): compact terminal showing the student's `last_test_result` summary card. Run output mirroring is deferred (spec 008's snapshot model — we can wire it once the Test runner from plan 026 ships).

**Live-follow with pin**: on mount, subscribe to Hocuspocus awareness on the topic-room `student:{studentId}:focus` (a tiny presence-only doc) for `{problemId, attemptId}` updates. When the student's focus changes, the watch updates unless the teacher has pinned an attempt. A "Follow live" button reactivates following.

Tests: snapshot rendering of `AttemptCardsRow` with 3 attempts (active, pass, fail).

- [ ] Commit.

---

### Task 6: Student broadcasts focus

**Files:**
- Modify: `src/components/problem/problem-shell.tsx`

Connect a small awareness-only Hocuspocus client to `student:{studentId}:focus` on mount, publishing `{problemId, attemptId}` whenever either changes. No persistence — pure presence.

- [ ] Commit.

---

### Task 7: E2E

**Files:**
- Create: `e2e/teacher-watch.spec.ts`

Two parallel browsers (test as before — Playwright supports this natively):
- Student types in `/student/.../problems/{pid}` → assert teacher's `/teacher/.../students/{studentId}` mirror updates.
- Student switches attempt → assert teacher view follows.
- Teacher pins a different attempt → assert no further follow until "Follow live" clicked.

Skip Pyodide/Run from this E2E — focus on Yjs sync and live-follow behavior.

- [ ] Commit.

---

### Task 8: Verify + review + PR

- [ ] Full Vitest + Go suites green
- [ ] `bun run test:e2e` green
- [ ] Code review pass, address Critical + Important
- [ ] Post-execution report
- [ ] PR open

---

## Out of scope

- **JS Run support** — still Python-only; tracked as a separate small follow-up.
- **Run-output mirroring to teacher** — needs a snapshot path (spec 008's "last_run" awareness field). Lands with plan 026.
- **Test runner / hidden-case grading** — plan 026.
- **Multi-student dashboard view for teachers** — one student at a time, by design.
- **Submitting an Attempt to an Assignment** — separate spec/plan once grading is in scope.

## Risks

- **Doc-name swap during first-keystroke**: Yjs doc reconnect can drop a few in-flight characters if the user types fast. Buffer-and-replay (Task 3) needs careful testing. If it proves flaky, fall back to "create the empty Attempt eagerly on page load" — accepts one stray attempt row per visit but eliminates the race.
- **Teacher auth latency on Hocuspocus connect**: server-to-server HTTP call from Hocuspocus to Go on every connection. Cache aggressively (per WebSocket lifetime).
- **Yjs state column size**: `attempts.yjs_state` is `text`. For very large programs (>1MB Y.Doc state) this could be a problem. K-12 problems are tiny — fine for v1.
