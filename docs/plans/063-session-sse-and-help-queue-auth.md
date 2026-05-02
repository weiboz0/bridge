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

- `SessionEvents` (student-visible stream of live activity) → fetch the session first via `GetSession` (404 if nil), then gate with `h.canJoinSession(r, session, claims)`. Class members of the session's class can subscribe. Both checks happen BEFORE any SSE headers go out.
- `GetHelpQueue` (teacher-only roster of who needs help) → gate with `h.isSessionAuthority(w, r, sessionID, claims)`. Helper writes the 403/404 response itself and returns `(session, true)` on success. Only the session's teacher / class instructor / TA / org_admin / platform admin pass.
- `ToggleHelp` (a participant flagging themselves) → fetch the session first via `GetSession` (404 if nil), then gate with `h.canJoinSession(r, session, claims)`. **Caller-is-target is already enforced by design**: the handler body only accepts `{ raised: bool }` and unconditionally updates `claims.UserID`. The frontend (`raise-hand-button.tsx:16-20`) sends only `{ raised }`; teacher UI is read-only.

**Helper signatures** (verified against `sessions.go`):
- `func (h *SessionHandler) canJoinSession(r *http.Request, session *store.LiveSession, claims *auth.Claims) (int, string)` — pass a pre-loaded session. Returns `(0, "")` on success, or `(status, message)` on deny. Caller writes the response.
- `func (h *SessionHandler) isSessionAuthority(w http.ResponseWriter, r *http.Request, sessionID string, claims *auth.Claims) (*store.LiveSession, bool)` — looks up the session itself, **writes the deny response** on failure (403), returns `(session, true)` on success. Caller checks the bool and uses the returned session for downstream work.

**Error shape**: helpers return **403** (matching `GetParticipants` at `sessions.go:597-599`, `canJoinSession` at `:497/:518`, and `isSessionAuthority` at `:1028`). Using 403 not 404 across the board for plan uniformity. The only 404 case is when `GetSession` returns nil (the session itself doesn't exist) — that's not an auth failure.

For `SessionEvents` the session MUST be loaded BEFORE writing any SSE headers — once headers go out we can't return a clean status.

## Files

- Modify: `platform/internal/handlers/sessions.go` — wire helpers into the three handlers.
- Add: integration auth matrix for the three endpoints. Existing `sessions_test.go` only has `TestToggleHelp_NoClaims` — no other coverage exists for `SessionEvents` / `GetHelpQueue` / `ToggleHelp`. New integration tests build their own session-fixture and exercise:
  - outsider (no membership) → 403 across all three
  - student in same class → SSE 200, GetHelpQueue 403, ToggleHelp 200
  - teacher of session → all three 200
  - class instructor / TA / org_admin / platform admin → all three 200
  - non-existent session id → 404 from each handler:
    - `SessionEvents` and `ToggleHelp` get 404 from the explicit `GetSession` call
    - `GetHelpQueue` gets 404 from `isSessionAuthority` (which calls `GetSession` internally and writes the 404 itself)
- Verify: existing `tests/integration/session-topics.test.ts` and any e2e specs for session SSE still pass.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| The SSE close-on-unauthorized path is unusual (long-poll connection that needs to error and disconnect cleanly) | medium | Inspect `SessionEvents` flush/close behavior; load the session via `GetSession` BEFORE writing SSE headers; on `canJoinSession` deny return the helper's 403 (or 404 if the session itself is missing). Once headers go out we can't return a clean status. |
| Existing dashboard polls SSE on every navigation; a stricter gate could break a flow that was implicitly relying on the lax check | low | The dashboard only opens the SSE stream when a teacher is on the session page; teacher membership in the class is the existing prerequisite. The strict gate matches the user intent. |
| `ToggleHelp` self-only constraint might be too restrictive if a teacher needs to clear another student's help flag | medium | Verify via a quick check of the live UX — if teachers can unflag students, allow `isSessionAuthority` as a second permitted path on `ToggleHelp`. |

## Phases

### Phase 0: pre-impl Codex review

Confirm the helper signatures (`canJoinSession`, `isSessionAuthority`) and the SSE close-on-error pattern. Verify `ToggleHelp`'s intended permission shape (self-only vs teacher-can-clear-others). Capture verdict.

### Phase 1: implement + matrix tests + smoke

Single branch, three commits (one per endpoint) with their auth matrix.

### Phase 2: post-impl Codex review

## Codex Review of This Plan

### Pass 1 — 2026-05-02: BLOCKED → fixes folded in

Codex found 3 blockers:
1. `canJoinSession` signature is `(r, *session, claims) → (int, string)` not `(ctx, claims, sessionID)` — must fetch session first. Plan now spells the correct shape.
2. Helpers return 403, not 404. Plan now uses 403 for plan uniformity.
3. `ToggleHelp` caller-is-target check is already enforced by design (body only accepts `{raised}`, no target param). Plan now drops the redundant check.

Plus: no integration tests exist for these endpoints. Plan now requires building them from scratch with a session fixture.

### Pass 2 — 2026-05-02: 2 inconsistencies, both fixed

- `isSessionAuthority` actual signature is `(w, r, sessionID, claims) → (*LiveSession, bool)` and writes its own 403 — corrected.
- "Easier: 404 early" wording in Risks contradicted the agreed 403 policy — reworded to "load session BEFORE headers; return 403 on deny, 404 only if session missing."

### Pass 3 — 2026-05-02: 2 inconsistencies, both fixed

- `GetHelpQueue` line in Approach still showed the old 3-arg helper signature — corrected to `(w, r, sessionID, claims)` with the writer-and-bool semantics.
- `404 (helper resolves via GetSession)` blanket statement was ambiguous — now spells out which 404 path each endpoint takes (`SessionEvents`/`ToggleHelp` from explicit `GetSession`; `GetHelpQueue` from `isSessionAuthority`'s internal lookup).

### Pass 4 — 2026-05-02: **CONCUR**

Plan ready for implementation.
