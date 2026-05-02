# Plan 053b: Teacher-watch attempt scope (and topic_id query fix)

> Follow-up to plan 053 phase 3. Filed because two latent bugs surfaced
> while migrating teacher-watch to the new realtime-token helper, and
> both block Phase 4's flag flip.

## Status

Open. Filed 2026-05-02. Owner: TBD.

## Problem

Two related bugs in the teacher-watch flow (`/teacher/.../problems/{problemId}/students/{studentId}`):

### Bug 1 — `teacherCanViewAttempt` is broken (since plan 0013)

`server/attempts.ts:72` queries `problems.topic_id`. That column was
dropped in migration `0013_problem_bank.sql` when problems became
many-to-many with topics via `topic_problems`. The query has been
silently failing (or returning false) ever since — meaning teacher-
watch via the LEGACY token path has been broken for months. Nobody
caught it because the UI shows "Disconnected" without a stack trace.

### Bug 2 — Phase 1 attempt scope is owner-only

Plan 053 phase 1 deliberately narrowed `authorizeAttemptDoc` (Go) to
"attempt's owner only" because the resolution chain
`attempt → problem → topic_problems → topic → course → class` wasn't
plumbed. Teachers minting a JWT for a student's attempt get 403.

Result: in plan 053 phase 3, `src/components/problem/teacher-watch-shell.tsx`
was DELIBERATELY NOT migrated to the new helper. It keeps the legacy
`${teacherId}:teacher` token. With `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1`
(phase 4 cutover), that view will fail.

**Phase 4 of plan 053 must NOT flip the flag in prod until 053b ships.**

## Fix

### Phase 1 — extend Go `authorizeAttemptDoc` to class-staff

In `platform/internal/handlers/realtime_token.go`, after the owner
check, add:

```go
// Class-staff path. Resolve attempt → problem, then look up which
// classes contain that problem via topic_problems → topic → course
// → classes. The teacher passes if they have AccessRoster on ANY
// such class.
if h.Problems != nil && h.Classes != nil {
    classIDs, err := h.Problems.ListClassesContainingProblem(ctx, attempt.ProblemID)
    if err != nil {
        return "", &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
    }
    for _, cid := range classIDs {
        if _, ok, err := RequireClassAuthority(ctx, h.Classes, h.Orgs, claims, cid, AccessRoster); err == nil && ok {
            return "teacher", nil
        }
    }
}
return "", &authDecision{Status: http.StatusForbidden, Message: "Not authorized to watch this attempt"}
```

Add `ListClassesContainingProblem(ctx, problemID) ([]string, error)`
to `ProblemStore`. SQL:

```sql
SELECT DISTINCT c.id
FROM topic_problems tp
INNER JOIN topics t ON t.id = tp.topic_id
INNER JOIN courses co ON co.id = t.course_id
INNER JOIN classes c ON c.course_id = co.id
WHERE tp.problem_id = $1;
```

Also need: platform admin bypass (already in the impersonator
discussion — owner-only was a Phase 1 stricture, lift it here).

### Phase 2 — fix `teacherCanViewAttempt` in TS

`server/attempts.ts:72` query rewrite: drop the broken `p.topic_id`
join; resolve via `topic_problems` instead. The TS-side helper is
called by the legacy Hocuspocus auth path, so the fix matters until
Phase 4 of plan 053 retires that path.

```sql
WITH attempt_classes AS (
    SELECT DISTINCT c.id AS class_id
    FROM attempts a
    INNER JOIN topic_problems tp ON tp.problem_id = a.problem_id
    INNER JOIN topics t ON t.id = tp.topic_id
    INNER JOIN courses co ON co.id = t.course_id
    INNER JOIN classes c ON c.course_id = co.id
    WHERE a.id = $1
)
SELECT EXISTS (
    SELECT 1
    FROM class_memberships cm_t
    INNER JOIN class_memberships cm_s ON cm_s.class_id = cm_t.class_id
    INNER JOIN attempts a ON a.id = $1
    INNER JOIN attempt_classes ac ON ac.class_id = cm_t.class_id
    WHERE cm_t.user_id = $2
      AND cm_t.role = 'instructor'
      AND cm_s.user_id = a.user_id
) AS allowed;
```

### Phase 3 — migrate teacher-watch-shell to the helper

Drop the deferral comment in `src/components/problem/teacher-watch-shell.tsx`
and replace the legacy token construction with `useRealtimeToken`.

### Phase 4 — Go integration tests

Update `realtime_token_test.go::TestMintToken_AttemptDoc_OwnerOK_OthersDenied`
to reflect the new rule:
- attempt owner → role=user, OK
- class instructor (any class containing the problem) → role=teacher, OK
- platform admin → OK
- non-instructor outsider → 403

Add a test for the `ListClassesContainingProblem` store method.

## Out of scope

- Class-instructor read-only enforcement on the Hocuspocus side
  (currently teacher-watch displays the doc with `readOnly` set on
  the Monaco editor; server-side enforcement of read-only is a
  separate issue).
- Refactoring `teacherCanViewAttempt` to share a code path with the
  Go authorizer. Once plan 053 phase 4 retires the legacy
  `userId:role` parsing, the TS helper can be deleted entirely.

## Phases

1. **Phase 1** — Go store method + handler change + integration tests.
2. **Phase 2** — TS query fix in `server/attempts.ts`.
3. **Phase 3** — client refactor (replace deferral comment with
   `useRealtimeToken`).
4. **Phase 4** — Codex review + PR + merge.

## Codex Review of This Plan

_Pending dispatch._

## Risks

- **Cross-class teachers**: a teacher who instructs across orgs sees
  attempts from any of their classes. Ensure
  `RequireClassAuthority(AccessRoster)` is the correct gate (it
  matches the existing teacher-page parity).
- **Performance**: `ListClassesContainingProblem` runs on every
  teacher-watch token mint. Cached at `getRealtimeToken` (per doc-name,
  25 min) so the impact per teacher is one query per attempt-watch
  session.
