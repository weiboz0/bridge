# Plan 052 — Resource access authorization audit (P0)

## Status

- **Date:** 2026-05-01 (re-scoped from "Class membership mutation auth" to absorb every "missing auth on individual-resource handler" finding under one helper-pattern sweep)
- **Severity:** P0 — data tampering, roster leaks, and metadata enumeration are all reachable by any authenticated user with a UUID guess.
- **Origin:** Reviews `008-...:33-39`, `009-...:9-15` (class), plus `009-2026-04-30.md` §P0-1 (`GetTopic`), §P1-9 (`schedule.go`), §P1-11 (`GetAssignment`), §P1-13 (`UnitCollection.AddItem`).

## Problem

Across the Go handler tree, a systematic pattern: sibling handlers in the same file use a proper authority check, but a single `Get`/`Mutate` variant only checks `claims != nil`. One conceptual fix — apply the matching authority check — applies to all of them.

### Class handlers (original scope)

| Handler | File:Line | Auth check today | Exposure |
|---|---|---|---|
| `ArchiveClass` | `platform/internal/handlers/classes.go:233` | `claims != nil` | Archive any class |
| `AddMember` | `platform/internal/handlers/classes.go:284` | `claims != nil` | Inject any user with any role |
| `ListMembers` | `platform/internal/handlers/classes.go:339` | `claims != nil` | Read full roster |
| `UpdateMemberRole` | `platform/internal/handlers/classes.go:355` | `claims != nil` | Promote/demote any member |
| `RemoveMember` | `platform/internal/handlers/classes.go:395` | `claims != nil` | Remove any member |

`GetClass` (`:212`) already does the right thing via `CanAccessClass`.

### Topics (P0-1)

| `GetTopic` | `platform/internal/handlers/topics.go:138-155` | `claims != nil` | Leaks course structure, unit links, and metadata for any topic by UUID |

`ListTopics` (`:96-135`) already does the full course-access check via `UserHasAccessToCourse`. `GetTopic` should mirror it; return 404 on both not-found and not-authorized.

### Schedules (P1-9)

| `Schedule.List` | `platform/internal/handlers/schedule.go:139-153` | `claims != nil` | Enumerate scheduled sessions for any class |
| `Schedule.ListUpcoming` | `platform/internal/handlers/schedule.go:155-176` | `claims != nil` | Same |

`Schedule.Create` (`:57`) verifies teacher/org_admin status; the reads should require class membership or course access.

### Assignments (P1-11)

| `GetAssignment` | `platform/internal/handlers/assignments.go:147-164` | `claims != nil` | Read any assignment metadata by UUID, leaking class/course associations |

Gate via `assignment.ClassID` membership + platform-admin bypass; 404 on denial.

### Unit collections (P1-13)

| `UnitCollection.AddItem` | `platform/internal/handlers/unit_collections.go:306-356` | UUID/FK existence only | Cross-org, draft, or personal units can be attached to any collection |

The handler must load the unit and apply unit visibility (`canViewUnit`) before insert.

### Shadow routes

The Next-side shadow routes at `src/app/api/classes/[id]/members/route.ts` and `.../[memberId]/route.ts` have the same gap. Plan 055 deletes them; this plan focuses on the Go side.

## Out of scope

- Annotation auth — separate finding, plan 056.
- Session realtime auth (SSE/help-queue) — plan 053 / a session-realtime sibling.
- Hocuspocus — plan 053.
- `canViewUnit` widening for students — plan 061 (different shape: not "missing check," but "check is too restrictive").
- The Next shadow routes — plan 055.

## Approach

One per-resource authority helper, all 404-on-deny. Each helper resolves the resource then applies the membership check; handlers call the helper and bail on the (status, error) tuple. Helpers are deliberately small and stateless so unit testing is straightforward.

| Resource | Helper | Levels |
|---|---|---|
| Class | `requireClassAuthority(ctx, claims, classID, level)` | `read` / `mutate` |
| Topic | reuse `topicAccess(ctx, claims, topicID)` (already exists for `ListTopics`); call from `GetTopic` |
| Schedule | reuse `CanAccessClass` keyed on `class_id` from the row(s) |
| Assignment | resolve `assignment.ClassID` then call `CanAccessClass` |
| UnitCollection.AddItem | resolve the candidate `unit` then call `canViewUnit` |

For class:
- `read` = active class member, instructor, org_admin of class's org, or platform admin.
- `mutate` = instructor, active org_admin, or platform admin. Students/TAs can NOT mutate.

`ListMembers` is borderline (students legitimately need it for help-queue UX). Pick `read` level for now; revisit if a stricter rule is needed.

All helpers return `404 Not Found` (not `403`) when the resource exists but the caller has no relationship — preserves existing `GetClass` behavior so we don't leak resource existence by UUID. Decisions logged at `slog.InfoLevel` for dev/staging debugging.

## Files

- Modify: `platform/internal/handlers/classes.go` — add `requireClassAuthority` + wire into five handlers.
- Modify: `platform/internal/handlers/topics.go` — wire `topicAccess` into `GetTopic`.
- Modify: `platform/internal/handlers/schedule.go` — gate `List` + `ListUpcoming` with class access.
- Modify: `platform/internal/handlers/assignments.go` — gate `GetAssignment` with class membership.
- Modify: `platform/internal/handlers/unit_collections.go` — gate `AddItem` with `canViewUnit` on the candidate unit.
- Add/modify: `platform/internal/handlers/security_phase1_integration_test.go` — extend with the auth matrix (outsider / student / instructor / org_admin / platform-admin) for every newly-gated endpoint.
- Verify: `tests/integration/class-members-api.test.ts` (TS side) still passes — routes proxy to Go and inherit the new gates.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Existing teacher tooling silently breaks if it called these endpoints without proper membership | medium | Run the integration matrix BEFORE landing; eve@demo.edu's flows on the demo class are the smoke test. |
| 404-vs-403 ambiguity confuses debugging | low | Log decisions at `slog.InfoLevel`. |
| Org admin without a class connection gets blocked from a legitimate roster lookup | medium | Spec: org_admin of the *owning* org passes — implementation joins `classes` → `organizations` and checks `org_memberships`. |
| `canViewUnit` returning false for students breaks `UnitCollection.AddItem` for org-scope flows | medium | The collection-add path is teacher-only by intent; verify in the matrix. |
| Six handlers across five files = larger blast radius if any helper has a bug | medium | Each helper lands in its own commit; integration matrix per resource; PR ships only after every matrix is green. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch `codex:codex-rescue` on this plan. The plan now spans five files and several existing helper utilities (`CanAccessClass`, `UserHasAccessToCourse`, `canViewUnit`, `topicAccess`). The Codex pass should confirm:
- Each helper exists with the assumed shape.
- The 404-on-deny convention is consistent with the rest of the codebase.
- No handler is missed (cross-check `git grep "claims != nil" platform/internal/handlers/*.go | grep -v _test.go` for the full picture).

Capture verdict in `## Codex Review of This Plan` below. Iterate until concur.

### Phase 1: per-resource implementation + test matrix

Land one resource at a time on a single branch, each in its own commit:
1. Class handlers + matrix tests (original scope; 25 cases minimum: 5 handlers × 5 roles).
2. `GetTopic` + matrix tests.
3. Schedule reads + matrix tests.
4. `GetAssignment` + matrix tests.
5. `UnitCollection.AddItem` + matrix tests.
6. Run `cd platform && go test ./... -count=1 -timeout 180s`. Run `bun run test`. Run the affected Playwright e2e specs if any.
7. Self-review.
8. Open PR.

### Phase 2: post-impl Codex review

Dispatch `codex:codex-rescue` on the diff before merge. Resolve findings inline. Iterate to CONCUR.

## Codex Review of This Plan

(Filled in after Phase 0.)
