# Plan 094 — Excalidraw whiteboards in live sessions

**Branch:** `feat/094-session-whiteboard`
**Status:** Revision 2 — folds plan-review round 1 (3× CHANGES REQUESTED) + user decisions. Awaiting re-review.

## File scope

`drizzle/**` (new migration) ·
`src/lib/db/schema.ts` ·
`platform/internal/store/canvases.go` (new) + `platform/internal/store/canvases_test.go` (new) ·
`platform/internal/handlers/canvases.go` (new) + `platform/internal/handlers/canvases_integration_test.go` (new) ·
`platform/internal/handlers/realtime_token.go` + `platform/internal/handlers/realtime_token_test.go` ·
**`platform/internal/auth/realtime_jwt.go`** (add `readOnly` claim — scope-widened R1) ·
**`server/realtime-jwt.ts`** (mirror the claim — scope-widened R1) ·
**`platform/cmd/api/main.go`** (wire `CanvasStore` + handler — scope-widened R1) ·
`platform/internal/handlers/routes.go` (or where session routes register) ·
`server/hocuspocus.ts` ·
`next.config.ts` (only if `/api/sessions/{id}/canvases` isn't already covered by `/api/sessions/:path*`) ·
`src/lib/whiteboard/**` (new — the binding + hook) ·
`src/components/session/whiteboard/**` (new — canvas list, board surface, visibility control) ·
`src/components/session/teacher/teacher-dashboard.tsx` · `src/components/session/student/student-session.tsx` (add the whiteboard surface) ·
`package.json` (add `@excalidraw/excalidraw`, `y-excalidraw`) ·
`docs/api.md` · `docs/architecture/decisions.md` · `README.md` · this plan file.

Scope-widening (R1 blocker 1 / concern C1) authorized by the user 2026-08-06: the read-only viewer boundary cannot be built without a `readOnly` claim in both JWT files, and `CanvasStore` must be wired in `main.go`.

## Problem / goal

A live session is a shared code editor today. Add Excalidraw whiteboards, synced over the existing Yjs/Hocuspocus plumbing, with an **ownership + floor-based visibility** model. Persisted, and viewable after the session ends.

## Decisions (settled with the user)

| # | Decision | Source |
|---|----------|--------|
| 1 | **Ownership.** A *canvas* has one **owner** (durable — not tied to current session membership). The owner may write while the session is live. Who else may **read** is governed by visibility (Decision 4). | User |
| 2 | **Persisted + outlives the session** (read-only archive after end — Decision 8). Persisted via Hocuspocus `onLoadDocument`/`onStoreDocument` like attempt/session docs. | User |
| 3 | **Binding: `y-excalidraw`** with a Phase-1b vetting gate (maintenance, license, React 19, honors a read-only/view mode). **Fallback:** thin custom onChange↔Y.Map binding per `src/lib/yjs/use-yjs-tiptap.ts`. Server read-only enforcement is ours regardless. | User |
| 4 | **Visibility = 3 ordered levels + session floor.** Levels tight→loose: **`private`** (owner only) < **`host`** (owner + teacher) < **`session`** (all members). Host sets one **session floor** = the minimum any canvas may have; **default `private`**. A canvas's visibility is any level **≥ floor**. Ordering is **native pgEnum declaration order** (`private`,`host`,`session`) — `visibility < 'host'` works in SQL (per reviewer correction; no separate rank helper). | User |
| 5 | **Multiple canvases per owner**, with a **per-session cap** (Decision 10). | User |
| 6 | **Host override: floor only, for MVP.** No per-canvas host moderation yet — the floor is the supervision lever. | User |
| 7 | **Loosen-only — strict, no tightening (user's literal rule; reverses R1's interim reading).** An owner may only *raise* a canvas's visibility (`private`→`host`→`session`); tightening is rejected (400). This is deliberate: it makes visibility monotonic, so a minted read token never grants more than the canvas's *current* level and **there is no stale-token revocation problem**. Trade-off accepted: an accidental over-share is not undoable by the owner in MVP (a future host-moderation follow-up can address it). | User (2026-08-06) |
| 8 | **After the session ends: read-only archive.** No writes by anyone (incl. owner) once `status=ended`. Readable by the **owner** and by **whoever the canvas's final visibility allowed** — `host` → the session's teacher; `session` → users who were session participants. Canvas auth therefore **special-cases ended sessions** rather than reusing the live-membership gate (which returns `ended → no_access` for everyone). | User (2026-08-06) |
| 9 | **"Session member" = the plan-090 guard, reused, not re-rolled.** Read eligibility routes through the existing `CanAccessSession` / participant+class-authority logic (`store/sessions.go`), NOT a fresh membership query — a hand-rolled check is how plan 090's cross-org leak would reappear. | Reviewers (all 3) |
| 10 | **Per-session canvas cap** (e.g. 50) enforced at create — bounds the persisted-doc / storage-growth surface. Exact number set in Phase 1a; overridable at review. | Reviewer (opus C5) |

## Architecture (grounded; revised per R1)

A canvas is `documentName = canvas:{canvasId}`. Permission is enforced **server-side at mint** (`realtime_token.go` scope resolver, which today handles `attempt:` and rejects unknown scopes).

**The read-only boundary must be built (R1 blocker 1).** Today `RealtimeClaims` carries only `sub`/`role`/`scope`, and `hocuspocus.ts` hardcodes `ctx.readOnly=false`; Hocuspocus enforces writes via the connection's `readOnly`, not a context field. So:
- Add a **`readOnly bool` claim** to `RealtimeClaims` in **both** `platform/internal/auth/realtime_jwt.go` and `server/realtime-jwt.ts`.
- `MintToken` sets `readOnly` per the decision below.
- `hocuspocus.ts onAuthenticate` reads `claims.readOnly` and sets the **connection's `readOnly`** (Hocuspocus's write-enforcing field), not just context. Phase 2 adds a test that a `readOnly` connection's document update is rejected server-side. Existing `attempt:` behavior is preserved (owner-only, `readOnly=false`).

**Mint matrix — live session** (`status != ended`):
- Requester is the **owner** → write (`readOnly=false`).
- Requester is the **host** (`sessions.teacher_id`) AND visibility ∈ {`host`,`session`} → read (`readOnly=true`).
- Requester is an eligible **session member** (Decision 9) AND visibility = `session` → read.
- Else → **403**. A `private` canvas mints only for its owner.

**Mint matrix — ended session** (Decision 8): identical *read* rules, but **everyone is `readOnly=true`, including the owner** (archive). Membership for `session` visibility is evaluated against `session_participants` (who *were* in the session).

**Floor invariant is race-safe (R1 blocker 2).** `visibility ≥ floor` cannot be a Postgres cross-table CHECK, so both **create** and **set-session-floor** take a `SELECT … FOR UPDATE` on the `sessions` row; create computes `visibility = GREATEST(requested, floor)` under that lock. This serializes create against a concurrent floor-raise so no canvas lands below the floor.

## Phases

### Phase 1a — Backend: schema + store *(Codex)*
- Migration + `schema.ts`: `canvas_visibility` pgEnum `(private, host, session)` (declaration order = tight→loose); `sessions.canvas_floor canvas_visibility NOT NULL DEFAULT 'private'` (**backfills existing rows** to `private`); `session_canvases` (`id`, `session_id` FK, `owner_id` FK, `title`, `visibility canvas_visibility NOT NULL`, `created_at`, `updated_at`), org/tenant scoping like `sessions`.
- `store/canvases.go`: create (locks session row; `visibility = GREATEST(requested, floor)`; enforces per-session cap), get, `list-visible-to-user` (a single query with owner/host/member branches, scoped to the path `session_id`, reusing the Decision-9 helpers), set-visibility (**loosen-only**: reject target ≤ current or < floor → error), set-session-floor (host-only; locks session row; bumps canvases with `visibility < floor` up to floor in the same txn).
- `store/canvases_test.go`: floor default + backfill; `GREATEST` at create; **concurrent create-vs-raise leaves nothing below floor**; loosen accepted, tighten rejected; raise-floor bumps; list-by-role; per-session cap; cross-org isolation.

### Phase 1b — Backend: handlers + token mint + JWT claim *(Codex)*
- `auth/realtime_jwt.go` + `server/realtime-jwt.ts`: add `readOnly` claim (both must stay byte-compatible — same field name/JSON tag).
- `handlers/canvases.go`: `POST /api/sessions/{id}/canvases` (member; owner; starts at floor; cap-checked); `GET …/canvases` (visible-to-caller); `PATCH …/canvases/{cid}` (owner-only; loosen-only visibility, title); `DELETE …/canvases/{cid}` (owner-only; lifecycle — R1 opus N3); host-only `PATCH …/settings` for `canvas_floor`.
- `realtime_token.go`: add `canvas:{cid}` to the scope resolver implementing the live/ended mint matrix; sets the `readOnly` claim.
- `main.go`: wire `CanvasStore` + handler.
- `canvases_integration_test.go` + `realtime_token_test.go`: the mint matrix (below), incl. ended-session archive.
- **y-excalidraw vetting gate** (blocks Phase 3) — record findings here.

### Phase 2 — Realtime: read-only enforcement + persistence *(Codex / inline)* — load-bearing
- `hocuspocus.ts`: handle `canvas:` in `onAuthenticate` — read `claims.readOnly`, set the **connection `readOnly`** (write-enforcing), carry context. `onLoadDocument`/`onStoreDocument` persist the canvas Yjs doc (debounced snapshot, like attempts — not per-stroke Postgres writes).
- **Tests (the security core):** a `readOnly` connection's update is **rejected server-side** (not merely hidden client-side); owner writes persist + restore on reconnect; a non-eligible user cannot connect; an ended-session owner connection is read-only.

### Phase 3 — Frontend: Excalidraw surface + binding *(Sonnet)*
- `@excalidraw/excalidraw` + `y-excalidraw` (or fallback). `src/lib/whiteboard/use-whiteboard.ts` binds a `canvas:{id}` Yjs doc (via `useYjsProvider` + a minted canvas token) to Excalidraw; **dynamic-import** the board (bundle size).
- `src/components/session/whiteboard/`: canvas list (visible-to-me), create, owner visibility control (**loosen-only UI — only shows levels ≥ current**), board surface. Non-writers → `viewModeEnabled` (UX; the token is the real boundary).
- Add a "Whiteboard" surface to `teacher-dashboard.tsx` + `student-session.tsx` (thin entry points).
- Frontend tests: list by role; owner edit vs viewer view-mode; loosen control; create; ended-session read-only.

### Phase 4 — Integration tests (NAMED — required: API + realtime auth + persistence) *(Opus)*
- **Fixtures:** host + two members + one non-member + one other-org user; a session from the existing harness; an ended-session fixture.
- **Acceptance (exact names must exist + pass):** `TestMintToken_Canvas_OwnerWrite`, `TestMintToken_Canvas_HostReadWhenHostVisible`, `TestMintToken_Canvas_HostDeniedWhenPrivate`, `TestMintToken_Canvas_MemberReadWhenSessionVisible`, `TestMintToken_Canvas_MemberDeniedWhenHostVisible`, `TestMintToken_Canvas_NonMemberDenied`, `TestMintToken_Canvas_OtherSessionMemberDenied`, `TestMintToken_Canvas_EndedSessionArchiveReadOnly`, `TestCanvasStore_SetVisibility_RejectsTightenAndBelowFloor`, `TestCanvasStore_ConcurrentCreateVsRaiseFloor_NoneBelowFloor`, `TestCanvasStore_RaiseFloor_BumpsCanvases`, `TestCanvasStore_ListVisible_ByRole`, `TestCanvasStore_PerSessionCap`, `TestCanvases_CrossOrgIsolation`, plus the Phase-2 read-only-write-rejected realtime test.
- **Live vs fast:** all the above are Go integration / realtime (fast, DB-backed). One Playwright spec covers create→loosen→view (**not run against a live stack** — needs the booted stack + pinned `E2E_BASE_URL`, per `docs/testing.md`).

### Phase 5 — Docs + verify
- `docs/api.md` (canvas endpoints + `canvas:` scope + the live/ended permission matrix); `docs/architecture/decisions.md` (new §: whiteboard visibility floor + realtime read-only claim); `README.md` bullet.
- `bash scripts/ci-local.sh` green (attestation); `pre-merge-guard.sh`.

## Risks

| Risk | Mitigation |
|------|------------|
| **Realtime auth is a hard safeguard** — a permissive mint leaks a private/cross-org canvas. | Enforce at mint (server), reuse the plan-090 membership guard (Decision 9), test the full owner/host/member/non-member/cross-org matrix + ended-session archive. 4-way review must scrutinize `realtime_token.go` + `hocuspocus.ts`. |
| **Read-only not actually enforced** (the whole boundary). | Phase 2 sets the connection's `readOnly` and tests that a viewer's write is rejected *server-side*. `viewModeEnabled` is UX only. |
| Floor invariant race (create vs raise). | `SELECT … FOR UPDATE` on the session row in both; `GREATEST(requested, floor)`. Concurrency test required. |
| Owner over-shares by accident (loosen-only, Decision 7). | Accepted for MVP; a host-moderation follow-up can add tighten/override. UI shows a confirm on loosening to `session`. |
| `y-excalidraw` unmaintained / no read-only / React 19. | Phase-1b vetting gate + thin-custom fallback. Phase 3 blocked until it passes. |
| Persistence churns Postgres per stroke. | Debounced Hocuspocus snapshot, matching attempt/session doc persistence. |
| Excalidraw bundle on the session route. | Dynamic-import; load only when the whiteboard opens. |

## Out of scope
- Per-recipient/group sharing; **tightening / un-share** and per-canvas host moderation (Decisions 6, 7 — follow-up); export/import; templates; image/file embeds beyond Excalidraw defaults; laser-pointer; canvas comments; post-end *editing* (archive is read-only, Decision 8).

## Plan Review

### Round 1 — 4-way (2026-08-06): **Codex + independent Opus + GLM all CHANGES REQUESTED.** Autopilot halted for user decisions. Full findings + convergent blockers recorded in commit `159e622`. Resolutions folded into this revision:
- BLOCKER (no read-only claim) → scope widened to both JWT files + `main.go`; `readOnly` claim + connection-`readOnly` enforcement specified (Architecture, Phase 1b, Phase 2).
- BLOCKER (create races floor-bump) → `FOR UPDATE` + `GREATEST` locking (Architecture, Phase 1a) + concurrency test.
- BLOCKER (outlives-session contradiction) → Decision 8 (read-only archive; owner + final-visibility parties).
- BLOCKER/CONCERN (stale token on tighten) → Decision 7 strict loosen-only dissolves it (no tightening ⇒ nothing to revoke).
- CONCERN (session-member) → Decision 9 (reuse plan-090 guard).
- Mechanical: native pgEnum ordering (Decision 4); `NOT NULL DEFAULT`+backfill (Phase 1a); split Phase 1 → 1a/1b + Phase 2 read-only enforcement first-class; test matrix expanded (Phase 4); canvas cap (Decision 10); DELETE endpoint (Phase 1b); owner-durability (Decision 1).

### Round 2 — pending re-review of this revision.

## Code Review

_Pending._

## Post-Execution Report

_Pending._
