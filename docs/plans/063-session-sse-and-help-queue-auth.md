# Plan 063 — Session SSE stream + help-queue authorization (P1)

## Status

- **Date:** 2026-05-01
- **Severity:** P1 (live activity + roster leak via session UUID)
- **Origin:** Reviews `008-...` and `009-2026-04-30.md` §P1-3.

## Problem

Three HTTP endpoints in `platform/internal/handlers/sessions.go` only check `claims != nil`:

| Handler | File:Line | Exposure |
|---|---|---|
| `SessionEvents` (SSE stream) | `sessions.go:611-644` | Subscribe to live join/leave/broadcast/help events for any session by UUID |
| `GetHelpQueue` | `sessions.go:646-669` | Read participants with `help_requested_at` for any session |
| `ToggleHelp` | `sessions.go:673+` | Toggle a participant's help-request status for any session (likely; verify) |

The sibling `GetParticipants` (`:590`) already calls `isSessionAuthority` — the auth helper exists. The three above just don't use it.

A logged-in outsider who knows or guesses a session UUID can watch the live join/leave traffic in real time, see who's asking for help, and (if `ToggleHelp` is similarly gated) toggle help status for someone else.

This is distinct from plan 053 (Hocuspocus / WebSocket session-room auth) because these are HTTP/SSE endpoints. Different code path, same conceptual fix.

## Out of scope

- Hocuspocus / WebSocket session rooms — plan 053.
- Class-mutation auth (Archive/AddMember/etc.) — plan 052.
- The session-creation / start / end endpoints — verified separately in `security_phase1_integration_test.go`.

## Approach

Apply the existing helpers:

- `SessionEvents` (student-visible stream of live activity) → gate with `canJoinSession(ctx, claims, sessionID)`. Students who are class members of the session's class can subscribe; outsiders 404.
- `GetHelpQueue` (teacher-only roster of who needs help) → gate with `isSessionAuthority(ctx, claims, sessionID)`. Only the session's teacher / class instructor / org_admin / platform admin can read.
- `ToggleHelp` (a participant flagging themselves) → gate with `canJoinSession`. The handler should ALSO verify the participant being toggled is the caller (a student can flag/unflag themselves but not someone else).

All three return 404 (not 403) for unauthorized access — preserves the "session existence shouldn't leak by UUID" convention.

## Files

- Modify: `platform/internal/handlers/sessions.go` — wire helpers into the three handlers.
- Modify: `platform/internal/handlers/sessions_test.go` (or `security_phase1_integration_test.go`) — auth matrix for outsider / student-not-in-class / student-in-class / teacher-of-other-class / instructor / platform-admin against each of the three endpoints.
- Verify: existing `tests/integration/session-topics.test.ts` and any e2e specs for session SSE still pass.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| The SSE close-on-unauthorized path is unusual (long-poll connection that needs to error and disconnect cleanly) | medium | Inspect `SessionEvents` flush/close behavior; ensure the unauthorized path returns BEFORE the SSE headers are written. Easier: 404 early. |
| Existing dashboard polls SSE on every navigation; a stricter gate could break a flow that was implicitly relying on the lax check | low | The dashboard only opens the SSE stream when a teacher is on the session page; teacher membership in the class is the existing prerequisite. The strict gate matches the user intent. |
| `ToggleHelp` self-only constraint might be too restrictive if a teacher needs to clear another student's help flag | medium | Verify via a quick check of the live UX — if teachers can unflag students, allow `isSessionAuthority` as a second permitted path on `ToggleHelp`. |

## Phases

### Phase 0: pre-impl Codex review

Confirm the helper signatures (`canJoinSession`, `isSessionAuthority`) and the SSE close-on-error pattern. Verify `ToggleHelp`'s intended permission shape (self-only vs teacher-can-clear-others). Capture verdict.

### Phase 1: implement + matrix tests + smoke

Single branch, three commits (one per endpoint) with their auth matrix.

### Phase 2: post-impl Codex review

## Codex Review of This Plan

(Filled in after Phase 0.)
