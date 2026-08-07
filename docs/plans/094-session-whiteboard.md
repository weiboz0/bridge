# Plan 094 — Excalidraw whiteboards in live sessions

**Branch:** `feat/094-session-whiteboard`
**Status:** Draft — awaiting plan-review gate (Tier A: `platform/internal/{store,handlers}`, realtime auth, `drizzle/`, `server/hocuspocus.ts` → 4-way)

## File scope

`drizzle/**` (new migration) ·
`src/lib/db/schema.ts` ·
`platform/internal/store/canvases.go` (new) + `platform/internal/store/canvases_test.go` (new) ·
`platform/internal/handlers/canvases.go` (new) + `platform/internal/handlers/canvases_integration_test.go` (new) ·
`platform/internal/handlers/realtime_token.go` + `platform/internal/handlers/realtime_token_test.go` ·
`platform/internal/handlers/routes.go` (or wherever session routes register) ·
`server/hocuspocus.ts` ·
`next.config.ts` (proxy route for `/api/sessions/{id}/canvases` if not already covered by `/api/sessions/:path*`) ·
`src/lib/whiteboard/**` (new — the y-excalidraw binding + hook) ·
`src/components/session/whiteboard/**` (new — canvas list, board surface, share control) ·
`src/components/session/teacher/teacher-dashboard.tsx` · `src/components/session/student/student-session.tsx` (add the whiteboard surface/tab) ·
`package.json` (add `@excalidraw/excalidraw`, `y-excalidraw`) ·
`docs/api.md` · `docs/architecture/decisions.md` · `README.md` · this plan file.

Changes nothing under `e2e/` except adding one spec (named integration-tests phase).

## Problem / goal

A live session today is a shared code editor. Teachers want a **whiteboard** to sketch, diagram, and explain visually. Add Excalidraw canvases to a session, synced in real time over the existing Yjs/Hocuspocus plumbing, with an **ownership + sharing** permission model.

## Decisions (settled with the user)

| # | Decision | Source |
|---|----------|--------|
| 1 | **Ownership.** A *canvas* is a first-class entity with one **owner** who has write access. Who else may **read** is governed by the canvas's visibility level (Decision 4). | User |
| 2 | **Persisted + restored.** Canvas content survives reconnects and outlives the live session, persisted the same way session/attempt Yjs docs are today (Hocuspocus `onLoadDocument`/`onStoreDocument` → store). | User |
| 3 | **Binding: `y-excalidraw` community package** — with a Phase-1 vetting gate (maintenance, license, React 19 compat, honors a read-only/view mode). **Fallback:** a thin custom onChange↔Y.Map binding modeled on `src/lib/yjs/use-yjs-tiptap.ts`. Server-side read-only enforcement is ours regardless of the package. | User |
| 4 | **Visibility = three ordered levels + a session floor.** Levels tightest→loosest: **`private`** (owner only) < **`host`** (owner + teacher) < **`session`** (all session members). The host sets one **session-level floor** = the minimum visibility any canvas may have; **default `private`**. A canvas's visibility is any level **≥ floor**, chosen by its owner. The owner may **loosen** (raise) but **never set below the floor**. Raising the floor bumps every canvas below it up to the new floor; lowering the floor leaves existing canvases where they are. "Host sees everything" is simply floor = `host`. | User |
| 5 | **Multiple canvases per owner** allowed (canvas is first-class, not one-per-user). | User |
| 6 | **Host override: floor only, for MVP.** The host controls the session floor; owners set their own canvas within `[floor, session]`. No direct per-canvas host override (moderation) yet — the floor is the host's supervision lever. Deferred to a follow-up. | User |
| 7 | **Owner visibility change is floor-relative, not a strict ratchet (implementation reading of "loosen only").** The owner may set visibility to any level in `[floor, session]`; "can't tighten" = can't go below the floor. This permits an owner to correct an accidental over-share back down to the floor. *Alternative (strict monotonic loosen-only, over-share irreversible) is a one-line flip — flag at plan-review if the stricter reading was intended.* | Claude — reading of user rule |

## Architecture (grounded in existing code)

**Realtime auth is the crux and reuses an existing pattern.** Hocuspocus `onAuthenticate` (`server/hocuspocus.ts:98`) rejects any connection whose JWT `scope !== documentName`. The Go mint endpoint `POST /api/realtime/token` (`realtime_token.go`, `MintToken`) resolves the doc-name scope in a switch that today handles `attempt:{id}` and rejects unknown scopes (`realtime_token.go:342`). There is already a **read-only** precedent: `attempt:` docs and a teacher-watch JWT carry `ctx.readOnly` (`hocuspocus.ts:123-128`).

So a canvas is `documentName = canvas:{canvasId}`, and permission is enforced **server-side at mint**, using the canvas's effective visibility (already `≥` the session floor by construction):

- Requester is the **owner** → write JWT (`readOnly=false`).
- Requester is the session's **host/teacher** AND visibility ∈ {`host`, `session`} → read-only JWT.
- Requester is any **session member** AND visibility = `session` → read-only JWT.
- Otherwise → **403** at mint (no token, no room access). A `private` canvas mints only for its owner.

Because a canvas's stored visibility is always kept `≥ floor`, "floor = `host`" automatically makes every canvas host-readable without any per-request special-casing.

`onAuthenticate` carries `readOnly` into context for `canvas:` docs exactly like `attempt:`; a read-only connection must reject document updates (verify Hocuspocus enforces `connection.readOnly`, else drop writes in `onChange`/`beforeHandleMessage`). Client also renders Excalidraw with `viewModeEnabled` for viewers — UX only; **the server token is the security boundary.**

## Phases

### Phase 1 — Backend: schema, canvas store + API, token mint *(Codex)*
- Migration + `schema.ts`:
  - a session-visibility enum `canvas_visibility` = (`private`, `host`, `session`) with a defined tight→loose ordering (encode order explicitly — enum text isn't ordered; use a rank helper or an ordered domain).
  - `sessions.canvas_floor` (enum, default `private`) — the session-level minimum.
  - `session_canvases` (`id`, `session_id` FK, `owner_id` FK, `title`, `visibility` enum, `created_at`, `updated_at`), org/tenant scoping consistent with `sessions`. On insert, `visibility` defaults to the session's current floor.
- `store/canvases.go`: create (visibility = floor), get, list-visible-to-user (own + those whose visibility grants the caller read given host/member status), set-visibility (validates target `≥ floor`), set-session-floor (host-only; **bumps every canvas with visibility < new floor up to the floor in the same txn**), ownership checks.
- `handlers/canvases.go`:
  - `POST /api/sessions/{id}/canvases` — any session member; becomes owner; canvas starts at floor.
  - `GET /api/sessions/{id}/canvases` — returns the canvases the caller may read (own, plus host/session-visible per the caller's role).
  - `PATCH /api/sessions/{id}/canvases/{canvasId}` — owner-only; `visibility` (must be `≥ floor`; reject `< floor` with 400), `title`.
  - `PATCH /api/sessions/{id}/settings` (or existing session-settings route) — host-only `canvas_floor`; triggers the bump.
- `realtime_token.go`: add `canvas:{canvasId}` to the scope resolver — write for owner; read-only for host when visibility ∈ {host, session}; read-only for a session member when visibility = session; else 403.
- **y-excalidraw vetting gate** (blocks Phase 3): record findings in this plan.
- Go tests: floor default; owner loosen within `[floor, session]`; reject tighten-below-floor (400); raising floor bumps canvases; visibility-gated list per role; cross-user + cross-org isolation; mint write/host-read/member-read/403 matrix.

### Phase 2 — Realtime: Hocuspocus canvas docs + read-only enforcement + persistence *(Codex / inline)*
- `server/hocuspocus.ts`: handle `canvas:` in `onAuthenticate` (carry `readOnly` from claims), `onLoadDocument`/`onStoreDocument` (persist canvas Yjs doc like attempts/session docs).
- **Enforce read-only server-side**: confirm a `readOnly` connection cannot mutate the doc; add a guard if Hocuspocus doesn't reject writes natively.
- Tests: read-only connection's writes are rejected; owner writes persist and restore; non-member cannot connect.

### Phase 3 — Frontend: Excalidraw surface + binding *(Sonnet)*
- `@excalidraw/excalidraw` + `y-excalidraw` (or fallback binding). `src/lib/whiteboard/use-whiteboard.ts` wires a canvas Yjs doc (via `useYjsProvider` + a realtime token minted for `canvas:{id}`) to Excalidraw.
- `src/components/session/whiteboard/`: canvas list (own + shared), create-canvas, owner share toggle, board surface. Viewers → `viewModeEnabled`.
- Add a "Whiteboard" surface/tab to `teacher-dashboard.tsx` and `student-session.tsx` (thin entry points reusing the shared component).
- Frontend tests: list renders own+shared; owner sees edit UI, viewer sees view-mode; share toggle PATCHes; create flow.

### Phase 4 — Integration tests (NAMED — required; touches API + realtime + persistence) *(Opus)*
- **Scenarios:** owner creates a canvas (starts at floor `private`) → draws (persists) → loosens to `session` → a second session member sees it read-only → the viewer's write attempt is rejected server-side → a non-member/other-org user is denied at mint. Host raises floor `private`→`host` → a still-`private`-intended canvas is bumped and now mints read-only for the host. Owner attempt to set visibility below floor → 400. Reconnect restores content.
- **Fixtures:** host + two session members + one outsider (+ one other-org user); a session from the existing session test harness.
- **Live vs fast:** mint/permission/persistence/floor assertions are Go integration (fast, DB-backed); Excalidraw render + view-mode is a component test; one Playwright spec covers create→loosen→view end-to-end (**not run against a live stack** — needs the booted stack + pinned `E2E_BASE_URL`).
- **Acceptance criteria (exact test names must exist + pass):** `TestMintToken_Canvas_OwnerWrite`, `TestMintToken_Canvas_HostReadWhenHostVisible`, `TestMintToken_Canvas_MemberReadWhenSessionVisible`, `TestMintToken_Canvas_PrivateDeniesNonOwner`, `TestMintToken_Canvas_NonMemberDenied`, `TestCanvasStore_SetVisibility_RejectsBelowFloor`, `TestCanvasStore_RaiseFloor_BumpsCanvases`, `TestCanvasStore_ListVisible_ByRole`, `TestCanvases_CrossOrgIsolation`, plus the read-only-write-rejected realtime test.

### Phase 5 — Docs + verify
- `docs/api.md` (canvas endpoints + `canvas:` realtime scope + permission table); `docs/architecture/decisions.md` (new §: whiteboard ownership/sharing + realtime auth reuse); `README.md` feature bullet.
- `bash scripts/ci-local.sh` green (attestation); `pre-merge-guard.sh`.

## Risks

| Risk | Mitigation |
|------|------------|
| **Realtime auth is a hard safeguard** — a permissive mint leaks a private/other-org canvas. | Enforce at mint (server), not client `viewModeEnabled`. Cross-org + non-member denial are required tests. Plan-review is 4-way and MUST scrutinize `realtime_token.go` + `hocuspocus.ts`. |
| `y-excalidraw` unmaintained / no read-only / React 19 incompat. | Phase-1 vetting gate with a thin-custom-binding fallback (Decision 3). Do not proceed to Phase 3 until vetting passes. |
| Read-only not enforced by Hocuspocus natively → a "viewer" writes to the shared doc. | Phase 2 explicitly verifies and, if needed, adds a server-side write guard; test asserts rejection. |
| Excalidraw bundle size on the session route. | Dynamic-import the board; load only when the whiteboard surface opens. |
| Persistence format churns Postgres per stroke. | Persist via Hocuspocus debounced `onStoreDocument` (snapshot), matching existing attempt/session doc persistence — not per-update writes. |

## Out of scope
- Per-recipient / group sharing (visibility is the 3-level floor model, not a recipient list), per-canvas host override / moderation (Decision 6 — floor only for MVP), export/import, templates, image/file embeds beyond Excalidraw defaults, presenter laser-pointer, and canvas-level comments. Each a follow-up if wanted.
- Retrofitting whiteboards onto class-bound vs class-less distinctions beyond what session membership already implies.

## Plan Review

### Round 1 — 4-way (2026-08-06). **Codex + independent Opus + GLM all CHANGES REQUESTED.** Autopilot halted for user decisions.

Convergent blockers (found by ≥2 reviewers):

1. `[BLOCKER]` `[codex][opus]` **No read-only mechanism exists.** `RealtimeClaims` (`auth/realtime_jwt.go` + `server/realtime-jwt.ts`) carries only `sub`/`role`/`scope` — no `readOnly`. `hocuspocus.ts` hardcodes `ctx.readOnly=false` and Hocuspocus enforces writes via `connectionConfig.readOnly`, not a context field. The viewer boundary the whole plan rests on isn't there. Fix needs a new `readOnly` claim in **both** JWT files (out of scope) + setting `connectionConfig.readOnly`. → **needs scope widening (user).**
2. `[BLOCKER]` `[codex][opus]` **Create races the floor bump.** `visibility ≥ floor` is application-only (no cross-table CHECK in PG). A create reading floor=`private` can insert after a floor-raise commits, landing below floor. → mechanical fix: `SELECT … FOR UPDATE` on the session row in create + set-floor, or `GREATEST(requested, floor)` under lock.
3. `[BLOCKER]` `[opus]` **"Outlives the session" contradicts the auth model.** Every membership path returns `ended → no_access` for *everyone incl. the owner*, so a persisted canvas becomes inaccessible the moment the session ends. → **needs a product decision (user):** who can open a canvas after the session ends?
4. `[BLOCKER/CONCERN]` `[codex]` **Stale read token after tightening.** Auth is checked at connect + `onLoadDocument`, tokens live ~25 min; a viewer connected before a canvas is tightened keeps getting updates. → **needs a decision (user):** forbid tightening (the strict "loosen-only" reading — reverses Decision 7) vs. accept ≤25-min staleness vs. add eviction.
5. `[CONCERN]` `[codex][opus][glm]` **"session member" must reuse the plan-090 guard** (`CanAccessSession` / participant check), not a fresh query — a hand-rolled membership check is exactly how 090's cross-org leak would reappear. → mechanical: mandate reuse.

Other findings (mechanical, will fold on revise): file-scope also needs `main.go` (handler + store wiring); use **native pgEnum ordering** (`visibility < 'host'` works by declaration order — my "enums aren't ordered" note was wrong, per Opus N1); migration needs `NOT NULL DEFAULT 'private'` + backfill; split Phase 1 and give read-only *enforcement* first-class treatment (Phase 2 under-specified); add tests for host-denied-private, member-denied-host, cross-session, canvas/path mismatch, concurrent create-vs-raise, stale-token; decide owner-leaves-session; add a per-session canvas cap; add a canvas DELETE/lifecycle endpoint.

**Verdict: CHANGES REQUESTED ×3. Not cleared. Revision pending user decisions on blockers 3, 4, and the scope-widening for blocker 1.**

## Code Review

_Pending._

## Post-Execution Report

_Pending._
