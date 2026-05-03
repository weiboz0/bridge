# Plan 053b: Deferred mint sites (teacher-watch, parent-viewer)

> Follow-up to plan 053 phase 3. Filed because three latent bugs
> surfaced while migrating to the new realtime-token helper. All three
> block Phase 4's flag flip.

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

**Codex pass-1 caught a privacy bug in the original proposal**:
`ListClassesContainingProblem(problemID)` + `RequireClassAuthority`
proves the teacher has roster access to a class that contains the
problem — but does NOT prove the attempt owner is a member of that
class. A teacher of Class A could get tokens for a student's attempt
in Class B if both classes use the same problem (likely scenario:
popular Python 101 unit linked to multiple courses across orgs).

The TS fix already gets this right by including
`cm_s.user_id = a.user_id` in the join. The Go side must mirror that
constraint.

**Corrected approach**: a single store method that verifies BOTH
sides of the relationship in one EXISTS query.

In `platform/internal/handlers/realtime_token.go::authorizeAttemptDoc`,
after the owner check, add:

```go
// Platform admin / impersonator bypass (lifting the Phase 1
// owner-only stricture — admins need cross-class oversight).
if claims.IsPlatformAdmin || claims.ImpersonatedBy != "" {
    return "teacher", nil
}

// Class-staff path. The teacher passes if they're an instructor /
// TA / org_admin in ANY class that contains the problem AND the
// attempt owner is a member of THAT SAME class. Single SQL EXISTS
// via the new store method.
if h.Attempts != nil && h.Classes != nil {
    ok, err := h.Attempts.IsTeacherOfAttempt(ctx, claims.UserID, attempt.ID)
    if err != nil {
        return "", &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
    }
    if ok {
        return "teacher", nil
    }
    // Org-admin path — not covered by IsTeacherOfAttempt (which is
    // class-membership-only). Resolve attempt → classes → orgs;
    // org_admin in any of those orgs passes.
    if h.Orgs != nil {
        ok, err := h.Attempts.IsOrgAdminOfAttempt(ctx, claims.UserID, attempt.ID)
        if err != nil {
            return "", &authDecision{Status: http.StatusInternalServerError, Message: "Database error"}
        }
        if ok {
            return "teacher", nil
        }
    }
}
return "", &authDecision{Status: http.StatusForbidden, Message: "Not authorized to watch this attempt"}
```

Add to `AttemptStore` (cleaner home than ProblemStore — these are
attempt-centric authorization checks):

```go
// IsTeacherOfAttempt reports whether `teacherID` has an instructor
// or TA class_membership in ANY class where (a) the class's course
// has a topic linking to the attempt's problem AND (b) the
// attempt's owner is a class_membership in the SAME class. Both
// constraints in one SQL.
func (s *AttemptStore) IsTeacherOfAttempt(ctx context.Context, teacherID, attemptID string) (bool, error)
```

SQL:

```sql
SELECT EXISTS (
    SELECT 1
    FROM attempts a
    INNER JOIN topic_problems tp  ON tp.problem_id = a.problem_id
    INNER JOIN topics t           ON t.id = tp.topic_id
    INNER JOIN courses co         ON co.id = t.course_id
    INNER JOIN classes c          ON c.course_id = co.id
    INNER JOIN class_memberships cm_t
        ON cm_t.class_id = c.id
        AND cm_t.user_id = $1
        AND cm_t.role IN ('instructor', 'ta')
    INNER JOIN class_memberships cm_s
        ON cm_s.class_id = c.id
        AND cm_s.user_id = a.user_id
    WHERE a.id = $2
);
```

```go
// IsOrgAdminOfAttempt — separate check because org_admin doesn't
// require a class_membership row (the existing
// `RequireClassAuthority(AccessRoster)` includes org_admins via
// org_memberships). To keep the pattern consistent with
// IsTeacherOfAttempt, do it as a single EXISTS over the org
// chain.
func (s *AttemptStore) IsOrgAdminOfAttempt(ctx context.Context, userID, attemptID string) (bool, error)
```

SQL:

```sql
SELECT EXISTS (
    SELECT 1
    FROM attempts a
    INNER JOIN topic_problems tp ON tp.problem_id = a.problem_id
    INNER JOIN topics t          ON t.id = tp.topic_id
    INNER JOIN courses co        ON co.id = t.course_id
    INNER JOIN class_memberships cm_s
        ON cm_s.class_id IN (SELECT id FROM classes WHERE course_id = co.id)
        AND cm_s.user_id = a.user_id
    INNER JOIN org_memberships om
        ON om.org_id = co.org_id
        AND om.user_id = $1
        AND om.role = 'org_admin'
        AND om.status = 'active'
    WHERE a.id = $2
);
```

The org_admin check still requires the attempt owner to be enrolled
in SOME class for that course — same privacy boundary.

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

### Bug 3 — parent-viewer has no Go-side auth path

`src/components/parent/live-session-viewer.tsx` connects to
`session:{sessionId}:user:{studentId}` with role=parent. The Go
`authorizeSessionDoc` has no parent branch — it would 403 every
parent connection.

The deeper blocker: **Bridge has no parent-child linking in the DB.**
Plan 049 was scheduled to add it but didn't ship; today
`POST /api/parent/children/{childId}/reports` returns 501. So even
with a parent branch added, there's no way to verify "is this user
the parent of this student?".

The legacy Hocuspocus auth at `server/hocuspocus.ts:46-52` accepts
`role === "parent"` without any check — that's the only reason the
parent-viewer works today, and it's also a security hole that
plan 053 is closing. Once `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1`, the
legacy parent path is gone and the unsigned `${parentId}:parent`
token is rejected.

**Phase 4 of plan 053 must NOT flip the flag in prod until both
plan 064 (parent-child linking; renamed from the original plan 049 intent) and 053b ship.**

## Out of scope

- Class-instructor read-only enforcement on the Hocuspocus side
  (currently teacher-watch displays the doc with `readOnly` set on
  the Monaco editor; server-side enforcement of read-only is a
  separate issue).
- Refactoring `teacherCanViewAttempt` to share a code path with the
  Go authorizer. Once plan 053 phase 4 retires the legacy
  `userId:role` parsing, the TS helper can be deleted entirely.
- Plan 064 (parent-child linking; the original "plan 049" intent
  before 049 was renumbered to Python 101 curriculum) is a hard
  prerequisite for the parent-viewer fix. It SHIPPED to main on
  2026-05-03 — this plan can now consume it directly.

## Phases

1. **Phase 1** — Go store method + `authorizeAttemptDoc` change for
   class-instructor + integration tests.
2. **Phase 2** — TS query fix in `server/attempts.ts`.
3. **Phase 3** — `teacher-watch-shell.tsx` migration to
   `useRealtimeToken`.
4. **Phase 4 — `authorizeSessionDoc` parent branch + `live-session-viewer.tsx`
   migration.** Plan 064 has shipped (the renamed parent-child plan;
   the original "plan 049" reference was renumbered after 049 became
   Python 101 curriculum). `parent_links` table + `IsParentOf` +
   parent-of-participant gate on `/api/sessions/{id}/topics` are
   already in main; this phase consumes them.
5. **Phase 5** — Codex review + PR + merge.

## Codex Review of This Plan

### Pass 1 — 2026-05-03: BLOCKED → fixes folded in

Codex found 2 blockers:

1. **Privacy bug in proposed Phase 1 Go check.** The original
   `ListClassesContainingProblem(problemID)` + `RequireClassAuthority`
   approach proves the teacher has roster access to a class
   containing the problem, but does NOT prove the attempt owner is
   in that same class. A teacher of Class A could mint tokens for a
   student's attempt in Class B if both classes use the same
   problem (a popular shared unit). **Fix:** rewrote Phase 1 with
   two single-EXISTS store methods (`IsTeacherOfAttempt`,
   `IsOrgAdminOfAttempt`) that join through `topic_problems →
   topics → courses → classes → class_memberships` and require
   BOTH (teacher in class) AND (attempt owner in class) in the
   same WHERE. Mirrors the TS query's `cm_s.user_id = a.user_id`
   constraint.

2. **Stale Phase 4 prerequisites.** Plan still described plan 049
   as "not shipped" / parent reports as 501. Plan 064 has shipped
   (parent_links + IsParentOf + IsParentOfAnyParticipant + parent-
   reports re-enabled). Updated Phase 4 + the "Out of scope" note
   to reflect current state.

Confirmed (no blockers):
- `authorizeAttemptDoc` is owner-only at `realtime_token.go:385`.
- `ListClassesContainingProblem` does not exist; SQL chain is
  valid against current schema.
- `server/attempts.ts:23,80` broken `topic_id` query confirmed.
- `authorizeSessionDoc` has no parent path; needs ParentLinks
  injection.
- `IsParentOfAnyParticipant` shape is correct for the parent-of-
  participant gate.
- `live-session-viewer.tsx` and `teacher-watch-shell.tsx` deferral
  comments + legacy tokens still in place.
- No other surfaces parent-viewer / teacher-watch hits that need
  parent/teacher gating beyond what's already in scope.

### Pass 2 — 2026-05-03: **CONCUR**

All three checks pass: SQL enforces both teacher-in-class AND
attempt-owner-in-class; org_admin path still requires attempt owner
to be enrolled in some class for the course; Phase 4 prerequisites
match current state. Plan cleared for implementation.

---

## Phase 1-4 Post-Implementation Review (2026-05-03)

Plan 053b shipped as PR #99 in three commits on `fix/053b-teacher-watch-and-parent-viewer`:
- `9fd47e7` — main implementation (8 files, 469 insertions)
- `0803cea` — pass-1 fix for the participant-status leak

### Codex post-impl pass 1 — 1 blocker, fixed

`authorizeSessionDoc` parent branch treated any `session_participants`
row as "child-in-session", including `status='left'`. A parent kept
access AFTER the child left the session.

**Fix**: required `existing.Status IN ('invited', 'present')` —
mirrors the same filter used for the own-doc fall-back later in
the function. New regression test
`TestMintToken_SessionDoc_ParentOfChildWhoLeft_403`.

### Codex post-impl pass 2 — **CONCUR**

Codex verified the status-filter applied correctly, confirmed all
other `session_participants` lookups in `authorizeSessionDoc` use
the same filter (consistent throughout), and ran the privacy
endpoint audit — no other surfaces leak.

### Final test count

6 mint integration tests (3 attempt + 3 session). All 6 pass:
- `_AttemptDoc_OwnerOnly_NoCourseBinding`
- `_AttemptDoc_TeacherOfAttemptOwnerInSameClass_OK`
- `_AttemptDoc_PopularProblemLeak_403` (Codex Phase-0 catch)
- `_SessionDoc_LinkedParent_OK`
- `_SessionDoc_ParentOfChildWhoLeft_403` (Codex post-impl pass-1 catch)
- `_SessionDoc_ParentOfDifferentChild_403`

Full Go handler suite: 150s, green.

### Plan 053 phase 4 unblocked

All 6 token construction sites are now on `useRealtimeToken`:

| Callsite | Status |
|---|---|
| `teacher-dashboard.tsx` | shipped (053 phase 3) |
| `student-session.tsx` | shipped (053 phase 3) |
| `live-session-viewer.tsx` | shipped (053b) |
| `problem-shell.tsx` | shipped (053 phase 3) |
| `teacher-watch-shell.tsx` | shipped (053b) |
| `use-yjs-tiptap.ts` | shipped (053 phase 3) |

Phase 4 of plan 053 (HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1 flag flip
in dev → staging → prod, then legacy-fallback removal) is now
operationally clear. See plan 053 §Phase 4 for the rollout
sequence.

## Risks

- **Cross-class teachers**: a teacher who instructs across orgs sees
  attempts from any of their classes. Ensure
  `RequireClassAuthority(AccessRoster)` is the correct gate (it
  matches the existing teacher-page parity).
- **Performance**: `ListClassesContainingProblem` runs on every
  teacher-watch token mint. Cached at `getRealtimeToken` (per doc-name,
  25 min) so the impact per teacher is one query per attempt-watch
  session.
