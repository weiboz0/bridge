# Plan 052 — Class membership mutation authorization (P0)

## Status

- **Date:** 2026-05-01
- **Severity:** P0 (data tampering + roster leak via UUID)
- **Origin:** Reviews `docs/reviews/008-...:33-39` and `docs/reviews/009-...:9-15`.

## Problem

Five class endpoints check only `claims != nil` — any logged-in user can mutate or read class membership across orgs and classes by guessing or learning a class UUID:

| Handler | File:Line | Auth check today | Exposure |
|---|---|---|---|
| `ArchiveClass` | `platform/internal/handlers/classes.go:233` | `claims != nil` | Archive any class |
| `AddMember` | `platform/internal/handlers/classes.go:284` | `claims != nil` | Inject any user with any role |
| `ListMembers` | `platform/internal/handlers/classes.go:339` | `claims != nil` | Read full roster |
| `UpdateMemberRole` | `platform/internal/handlers/classes.go:355` | `claims != nil` | Promote/demote any member |
| `RemoveMember` | `platform/internal/handlers/classes.go:395` | `claims != nil` | Remove any member |

`GetClass` (`:212`) already does the right thing via `CanAccessClass`. The mutation handlers don't.

The Next-side shadow routes at `src/app/api/classes/[id]/members/route.ts` and `.../[memberId]/route.ts` have the same gap — but those should also be deleted under plan 055 since plan 021 deferred the cleanup.

## Out of scope

- The Next shadow routes — handled by plan 055.
- Annotation auth (separate finding, plan 056).
- Session realtime auth (plan 053).

## Approach

Add a single helper `requireClassAuthority(ctx, claims, classID, level)` in `platform/internal/handlers/classes.go` that returns the class row or a (status, error) decision:

- `level = "read"` → caller must be an active class member, instructor of the class, org_admin of the class's org, or platform admin.
- `level = "mutate"` → caller must be the class instructor, an active org_admin of the class's org, or a platform admin. Students and TAs can NOT mutate.

The helper returns `404 Not Found` (not `403`) when the class exists but the caller has no relationship to it — matches the existing `GetClass` behavior so we don't leak class existence by UUID.

`ListMembers` is borderline: a roster is sensitive but students legitimately need it for help-queue UX. Pick `read` level for now; revisit if a stricter rule is needed.

## Files

- Modify: `platform/internal/handlers/classes.go` — add helper, wire into the five handlers.
- Modify: `platform/internal/handlers/classes_test.go` — add unit tests for the helper.
- Modify: `platform/internal/handlers/security_phase1_integration_test.go` — extend with the same outsider/student/teacher/org_admin/platform-admin matrix that already exists for `GetClass` (lines 50-94), now applied to all five mutation handlers.
- Verify: `tests/integration/class-members-api.test.ts` (TS side) still passes — the routes proxy to Go and inherit the new gates.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Existing teacher tooling silently breaks if it called these endpoints without proper class membership | low | The dev demo data has eve as instructor of `Python 101 · Period 3`; she'll continue to pass. Verify with the integration matrix. |
| 404-vs-403 ambiguity confuses debugging | low | Log the decision at `slog.InfoLevel` so dev/staging tooling can see the actual reason. |
| Org admin without a class connection gets blocked from a legitimate roster lookup | medium | Spec: org_admin of the *owning* org passes — implementation must JOIN classes→organizations and check `org_memberships`. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch `codex:codex-rescue` on this plan. Capture verdict in `## Codex Review of This Plan` below. Iterate until concur.

### Phase 1: helper + handler wiring + tests

- Add `requireClassAuthority` with read/mutate levels.
- Wire into all five handlers.
- Add the auth matrix tests (5 handlers × 5 caller roles = 25 cases minimum). Use existing `security_phase1_integration_test.go` patterns.
- Run `cd platform && go test ./... -count=1 -timeout 120s`. Run `bun run test`.
- Self-review.
- Commit + open PR.

### Phase 2: post-impl review

- Dispatch a second Codex review of the diff before merge.
- Resolve findings.

## Codex Review of This Plan

(Filled in after Phase 0.)
