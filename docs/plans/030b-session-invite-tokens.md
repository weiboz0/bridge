# Session Model Phase 2: Invite Tokens Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let authorized teachers issue, rotate, expire, and revoke session invite links, and let authenticated users join a session through `/s/{token}` without class membership.

**Architecture:** Activate the `invite_token` and `invite_expires_at` fields added in 030a through new Go endpoints, a token join entry flow, and shared access checks for HTTP reads and Hocuspocus document auth. This phase adds token-based access only; direct-add roster access waits for 030c, and orphan-session creation waits for 030d.

**Tech Stack:** Go, Chi, PostgreSQL, Next.js App Router, Hocuspocus, Vitest, testify

**Spec:** `docs/specs/010-session-model.md`

**Branch:** `feat/010-session-model`

**Depends On:** `030a-session-model-schema.md`

**Unblocks:** `030c-session-direct-add-and-access.md`, `030d-orphan-sessions.md`

---

## File Structure

| File | Responsibility |
|---|---|
| `platform/internal/store/sessions.go` | Add token lookup, token rotation, invite expiry update, and access-evaluation helpers for class membership or valid token. |
| `platform/internal/store/sessions_test.go` | Cover token generation, rotation invalidation, expiry cutoffs, and class-member fallback. |
| `platform/internal/handlers/sessions.go` | Add `PATCH /api/sessions/{id}`, `POST /api/sessions/{id}/rotate-invite`, `DELETE /api/sessions/{id}/invite`, and `POST /api/s/{token}/join`. |
| `platform/internal/handlers/sessions_test.go` | Unit coverage for validation and auth on the new routes. |
| `platform/internal/handlers/sessions_integration_test.go` | New end-to-end API coverage for token join status codes (`404`, `410`, `200`) and invite rotation behavior. |
| `platform/cmd/api/main.go` | Wire the `/api/s/{token}/join` route outside the `/api/sessions` subtree. |
| `server/hocuspocus.ts` | Enforce session document access with the same class-membership-or-valid-token rules instead of the current permissive `session:{id}:user:{uid}` shortcut. |
| `next.config.ts` | Proxy `/api/s/:path*` to Go so token joins do not fall back to stale Next routes. |
| `src/app/s/[token]/page.tsx` | Resolve the token join flow, handle auth redirect if needed, and route the user into the correct session page. |
| `src/components/session/teacher/teacher-header.tsx` | Add invite-link controls (copy link, rotate, close lobby, optional expiry display). |
| `src/app/(portal)/teacher/classes/[id]/session/[sessionId]/dashboard/page.tsx` | Fetch invite metadata so the dashboard can render the controls. |
| `docs/api.md` | Document the invite token endpoints and failure semantics. |

---

## Task 1: Store + Access Rules

**Files:**
- Modify: `platform/internal/store/sessions.go`
- Modify: `platform/internal/store/sessions_test.go`

- [ ] Add token helpers to create secure invite codes, fetch sessions by token, rotate existing tokens, and revoke tokens.
- [ ] Implement a single access-check helper for session reads and websocket auth that returns allow/deny for:
  - class membership
  - valid token before `invite_expires_at`
  - ended sessions rejected with `410 Gone`
- [ ] Keep direct-add access out of scope for now; 030c will extend this helper rather than rewrite it.

**Testing plan:**
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/store -run 'TestSessionStore_.*Token|TestSessionStore_.*Access' -count=1`

## Task 2: HTTP API Surface

**Files:**
- Modify: `platform/internal/handlers/sessions.go`
- Modify: `platform/internal/handlers/sessions_test.go`
- Create: `platform/internal/handlers/sessions_integration_test.go`
- Modify: `platform/cmd/api/main.go`
- Modify: `next.config.ts`
- Modify: `docs/api.md`

- [ ] Add `PATCH /api/sessions/{id}` for mutable invite/title/settings fields only; do not mix end-session semantics into this route.
- [ ] Add `POST /api/sessions/{id}/rotate-invite` and `DELETE /api/sessions/{id}/invite`.
- [ ] Add `POST /api/s/{token}/join` with the spec's response contract:
  - `404` unknown or rotated-away token
  - `410` expired invite or ended session
  - `200` successful join
- [ ] Proxy `/api/s/:path*` to Go in `next.config.ts`.

**Testing plan:**
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/handlers -run 'TestSessionHandler_.*Invite|TestSessionHandler_.*Token|TestSessionHandler_.*Patch' -count=1`

## Task 3: Teacher UI + Token Landing Flow

**Files:**
- Create: `src/app/s/[token]/page.tsx`
- Modify: `src/components/session/teacher/teacher-header.tsx`
- Modify: `src/app/(portal)/teacher/classes/[id]/session/[sessionId]/dashboard/page.tsx`

- [ ] Add a token-entry page that calls the join endpoint, handles `404`/`410`, and redirects authenticated users into the correct session route.
- [ ] Add dashboard controls so the session owner can copy the invite URL, rotate it, or close the lobby immediately.
- [ ] Keep the existing class-based dashboard route; orphan-session routing is a separate phase.

**Testing plan:**
- `node_modules/.bin/tsc --noEmit`
- `bun run test`

## Task 4: Realtime Verification

**Files:**
- Modify: `server/hocuspocus.ts`
- Modify: `docs/plans/030b-session-invite-tokens.md`

- [ ] Replace the permissive `session:{id}:user:{uid}` auth shortcut with a real access check against the session access helper or a purpose-built Go auth endpoint.
- [ ] Verify that a token-authorized user can open the session document and that an expired/unknown token cannot.
- [ ] Note any Hocuspocus limitations or follow-ups in the post-execution report instead of leaving silent gaps.

**Verification commands:**
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./... -count=1 -timeout 120s`
- `bun run test`
- `node_modules/.bin/tsc --noEmit`

---

## Code Review

Reviewers append findings here following `docs/code-review.md`. Author responds inline with `→ Response:` and updates status to `[FIXED]` or `[WONTFIX]`.

## Post-Execution Report

Populate during Step 6 of `docs/development-workflow.md` after this phase ships.
