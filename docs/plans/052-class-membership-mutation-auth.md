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

`ListTopics` (`:96-135`) already does the full course-access check via `UserHasAccessToCourse`, returning **403** on deny (per `topics.go:122-124` precedent). `GetTopic` should mirror that — 403, not 404.

### Schedules (P1-9)

| `Schedule.List` | `platform/internal/handlers/schedule.go:139-153` | `claims != nil` | Enumerate scheduled sessions for any class |
| `Schedule.ListUpcoming` | `platform/internal/handlers/schedule.go:155-176` | `claims != nil` | Same |

`Schedule.Create` (`:57`) verifies teacher/org_admin status; the reads should require class membership or course access.

### Assignments (P1-11)

| `GetAssignment` | `platform/internal/handlers/assignments.go:147-164` | `claims != nil` | Read any assignment metadata by UUID, leaking class/course associations |

Gate via `assignment.ClassID` membership + platform-admin bypass; **403** on denial (matches `assignments.go:133-135` precedent).

### Unit collections (P1-13)

| `UnitCollection.AddItem` | `platform/internal/handlers/unit_collections.go:306-356` | UUID/FK existence only | Cross-org, draft, or personal units can be attached to any collection |

The handler must load the unit and apply unit visibility (`canViewUnit`) before insert.

### CloneCourse (added 2026-05-01 from Phase 0 Codex review)

| `CloneCourse` | `platform/internal/handlers/courses.go:265-283`, store at `platform/internal/store/courses.go:175-228` | `claims != nil` only | Any authenticated user can clone any course by ID, getting a private copy of the topics into their account |

The clone keeps the source course's `org_id`; the cloned course's `created_by` becomes the caller. A logged-in outsider can clone competitive content to study offline. Required: caller must have access to the source course (`UserHasAccessToCourse` or platform admin).

### Shadow routes

The Next-side shadow routes at `src/app/api/classes/[id]/members/route.ts` and `.../[memberId]/route.ts` have the same gap. Plan 055 deletes them; this plan focuses on the Go side.

### Other auth-only surfaces explicitly out of scope

Codex Phase 0 review surfaced two more auth-only Go handlers that share the same shape but already have plans dedicated to them:

- Annotations (`platform/internal/handlers/annotations.go:28-128`) — covered by plan 056.
- Session SSE + help queue + ToggleHelp (`platform/internal/handlers/sessions.go:611-684`) — covered by plan 063.

These stay in their own plans because (a) annotation auth requires document-ID parsing and class-context resolution that don't fit the simple class/topic/assignment pattern, and (b) the SSE auth surface has long-poll close semantics that warrant their own treatment.

## Out of scope

- Annotation auth — separate finding, plan 056.
- Session realtime auth (SSE/help-queue) — plan 053 / a session-realtime sibling.
- Hocuspocus — plan 053.
- `canViewUnit` widening for students — plan 061 (different shape: not "missing check," but "check is too restrictive").
- The Next shadow routes — plan 055.

## Approach

One per-resource authority pattern. Each handler resolves the resource then applies the membership check; bails with the appropriate status code on deny. The class-side change extracts a small helper because three levels of access are needed; the other surfaces inline the existing helpers because they only need one level each.

| Resource | Pattern | Levels |
|---|---|---|
| Class | new `requireClassAuthority(ctx, claims, classID, level)` helper | `read` / `roster` / `mutate` |
| Topic | inline call to `UserHasAccessToCourse` (mirrors `ListTopics:106-124`) | course-access |
| Schedule | call `RequireClassAuthority` (extracted in PR-A) keyed on the row's `class_id` | class-access |
| Assignment | resolve `assignment.ClassID` then `RequireClassAuthority` | class-access |
| UnitCollection.AddItem | resolve the candidate `unit` then `canViewUnit` | unit-visibility |
| CloneCourse | resolve the source course, then `UserHasAccessToCourse` | course-access |

For class:
- `read` = caller passes `CanAccessClass` — class member, class staff, org_admin of the class's org, or platform admin. Used for class metadata.
- `roster` = caller is class staff (instructor/TA), org_admin, or platform admin. **Students do NOT pass.** Used for `ListMembers` and `AddMember`/`UpdateMemberRole`/`RemoveMember`. Tightened from the original `read` per Codex Phase 0 review: `ListMembers` returns email + name PII at `platform/internal/store/classes.go:45-52,368-374`; the help-queue UI uses `session_participants` (`src/components/help-queue/...`), not class members, so students don't legitimately need it.
- `mutate` = instructor, active org_admin, or platform admin. Used for `ArchiveClass`. TAs and students can NOT mutate.

**404-vs-403 convention:** mixed across the codebase. **Pin each subsystem to its existing precedent** rather than imposing a global rule (Codex Pass 2):

- **Class** subsystem: **404** on deny — matches `CanAccessClass`'s existing convention at `classes.go:218-225`.
- **Topic** subsystem: **403** on deny — matches `ListTopics`'s existing precedent at `topics.go:122-124`.
- **Schedule** subsystem: **403** on deny — matches `schedule.go:85-87`'s existing precedent.
- **Assignment** subsystem: **403** on deny — matches `assignments.go:133-135`'s existing precedent.
- **UnitCollection.AddItem**: **404** on deny — collection items are scope-private; existence shouldn't leak. Verify against the surrounding handlers' precedent during implementation.
- **CloneCourse**: **403** — matches `courses.go:160-168`'s existing precedent.

Decisions logged at `slog.InfoLevel` so dev/staging debugging shows the actual reason.

**Helper extraction (PR-A includes this; PR-B and PR-C consume it):**

`CanAccessClass` is currently a method on `ClassHandler` (`classes.go:172`). PR-B's `ScheduleHandler` and `AssignmentHandler` cannot call a method on a different handler type. PR-A's first commit extracts class-access into a free function in a shared location:

```go
// platform/internal/handlers/access.go (new file)
func RequireClassAuthority(ctx context.Context, classes *store.ClassStore, orgs *store.OrgStore, claims *auth.Claims, classID, level string) (*store.Class, *AccessDecision)
```

Where `AccessDecision` carries `(httpStatus int, message string)`. Wrap as needed in handler-specific helpers. The original `ClassHandler.CanAccessClass` becomes a thin shim that delegates to the new function for back-compat — or is replaced wholesale, since plan 052 is the only consumer that mutates the membership graph.

## Files

- Modify: `platform/internal/handlers/classes.go` — add `requireClassAuthority` + wire into five class handlers (Archive/Add/List/Update/Remove).
- Modify: `platform/internal/handlers/topics.go::GetTopic` — inline the `UserHasAccessToCourse` call from `ListTopics`.
- Modify: `platform/internal/handlers/schedule.go` — gate `List` + `ListUpcoming` with `RequireClassAuthority` (extracted in PR-A).
- Modify: `platform/internal/handlers/assignments.go::GetAssignment` — gate via `RequireClassAuthority` after resolving `assignment.ClassID`.
- Modify: `platform/internal/handlers/unit_collections.go::AddItem` — load candidate unit + `canViewUnit` check.
- Modify: `platform/internal/handlers/courses.go::CloneCourse` — load source course + `UserHasAccessToCourse` check.

Per-subsystem test files (Codex Phase 0 review pinned these — `security_phase1_integration_test.go` is the plan-043 regression file, NOT a generic bucket):

- Modify: `platform/internal/handlers/classes_test.go` — auth matrix for the five class handlers.
- Modify: `platform/internal/handlers/topics_test.go` — `GetTopic` matrix.
- Modify: `platform/internal/handlers/schedule_test.go` — Schedule reads matrix.
- Add: `platform/internal/handlers/assignments_test.go` (does not exist today per Codex Phase 0 audit) — `GetAssignment` auth matrix.
- Modify: `platform/internal/handlers/unit_collections_integration_test.go` — `AddItem` matrix.
- Modify: `platform/internal/handlers/courses_test.go` — `CloneCourse` matrix.

Verify (no expected break):
- `tests/integration/class-members-api.test.ts` (TS side proxies to Go).
- `platform/internal/handlers/security_phase1_integration_test.go` — class scenarios stay green.

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

Codex Phase 0 review flagged blast radius (six files in one PR is too many for a clean review). Split into **three PRs** that can land independently:

**PR-A — Class handlers** (the original scope). 5 handlers × 5 roles = 25-case matrix.

**PR-B — Cross-handler auth (topics + schedule + assignment + clone-course).** Single helper-pattern fix per surface; share the `CanAccessClass`/`UserHasAccessToCourse` plumbing.

**PR-C — `UnitCollection.AddItem`.** Smallest surface; isolated risk.

Each PR:
1. Implements the gates.
2. Ships its test matrix in the per-subsystem test file (per Codex Phase 0 — NOT in `security_phase1_integration_test.go`).
3. Runs `cd platform && go test ./... -count=1 -timeout 180s` and `bun run test`.
4. Self-review.
5. Codex post-impl review pass on the diff.
6. Merge.

Land in order: PR-A → PR-B → PR-C. Each independently revertable.

### Phase 2: post-impl Codex review

Dispatched per PR. Resolve findings inline. Iterate to CONCUR before merge.

## Codex Review of This Plan

### Pass 2 — 2026-05-01: BLOCKED → fixes folded in (round 2)

Two new blockers caught after Pass 1's fixes were folded in:

- `[CRITICAL]` 404-vs-403 table contradicted existing precedent. `topics.go:122-124`, `schedule.go:85-87`, and `assignments.go:133-135` all return 403, not 404 as the plan stated. **Fix:** pinned each subsystem's deny status to its existing precedent (topic/schedule/assignment all 403; class stays 404; clone-course 403; unit-collection 404).
- `[CRITICAL]` `CanAccessClass` is a method on `ClassHandler`; PR-B handlers (`ScheduleHandler`, `AssignmentHandler`) are different types and cannot call it directly. **Fix:** PR-A's first commit extracts a free `RequireClassAuthority` function into a new `platform/internal/handlers/access.go` so all handler types can consume it. Documented in the Approach section.
- `[PARTIAL]` `assignments_test.go` doesn't exist today. **Fix:** Files section now says "Add" not "Modify."

**Status:** Pass-3 dispatch will confirm the resolutions land cleanly.

### Pass 5 — 2026-05-01: **PHASE-0 CONCUR**

After Passes 3 and 4 caught two more inconsistencies (Assignments row's deny code; Files section call-site references), Pass 5 returned a clean concur. **Status: ready for implementation.** PR-A (class handlers + helper extraction) lands first.

### Pass 1 — 2026-05-01: BLOCKED → fixes folded in

Codex Phase 0 review found 3 blockers + 3 [IMPORTANT] items, all addressed inline:

- `[CRITICAL]` `topicAccess` helper does not exist — `ListTopics` does the access check inline at `topics.go:106-124`. **Fix:** plan now says `GetTopic` inlines the same `UserHasAccessToCourse` call rather than reusing a non-existent helper.
- `[CRITICAL]` Coverage was not exhaustive. Codex found **`CloneCourse`** as a third auth-only surface (auth-only at `courses.go:265-283`; lets any authenticated user clone any course). **Fix:** added to plan scope with the recommended `UserHasAccessToCourse` gate. Also documented that annotations (plan 056) and session SSE/help-queue (plan 063) are explicitly deferred to their own plans.
- `[CRITICAL]` `ListMembers` student `read` access exposes name + email PII — confirmed at `store/classes.go:45-52,368-374`. The help-queue UI uses `session_participants`, NOT class members (`src/components/help-queue/...`). **Fix:** added a new `roster` level distinct from `read`. `ListMembers` (and the three mutation handlers that operate on members) now require `roster` (instructor / TA / org_admin / platform-admin); students and class members in general do NOT pass.
- `[IMPORTANT]` 404-vs-403 convention is mixed. **Fix:** documented per-subsystem rule in the Approach section, following each existing handler's precedent.
- `[IMPORTANT]` Test files: use per-subsystem files, not the plan-043 security regression file. **Fix:** Files section now lists the per-subsystem test files explicitly.
- `[IMPORTANT]` Blast radius: six files in one PR. **Fix:** split into three PRs in the Phases section (A: class, B: topic/schedule/assignment/clone-course, C: unit-collection).

**Status: re-dispatch Pass 2 to confirm the resolutions land cleanly.**
