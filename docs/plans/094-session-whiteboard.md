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

## Decisions (settled with the user; sub-decisions overridable at plan-review)

| # | Decision | Source |
|---|----------|--------|
| 1 | **Ownership model.** A *canvas* is a first-class entity with one **owner**. The owner has write access. Other session members are **read-only viewers, and only when the canvas is shared.** A non-shared canvas is visible only to its owner. | User |
| 2 | **Persisted + restored.** Canvas content survives reconnects and outlives the live session (viewable afterward), persisted the same way session/attempt Yjs docs are today (Hocuspocus `onLoadDocument`/`onStoreDocument` → store). | User |
| 3 | **Binding: `y-excalidraw` community package** — with a Phase-1 vetting gate (maintenance, license, React 19 compat, does it honor a read-only/view mode). **Fallback:** a thin custom onChange↔Y.Map binding modeled on `src/lib/yjs/use-yjs-tiptap.ts` if vetting fails. Server-side read-only enforcement is ours regardless of the package. | User |
| 4 | **Sharing granularity (sub-decision):** `is_shared` is a boolean = *shared to the whole session* (every session member may view). Not per-user sharing. Simplest model that satisfies the requirement; revisit if per-recipient sharing is needed. | Claude — flag at review |
| 5 | **Multiple canvases per owner** allowed (canvas is first-class, not one-per-user). | Claude — flag at review |
| 6 | **Host visibility (sub-decision):** the host sees a canvas only when it is shared, same as any viewer — honoring the literal "viewers if shared." **Open question for review:** should the host (teacher) always see all participants' canvases, mirroring how they see all student *code* tiles today? Defaulting to shared-only; flag for the user to confirm. | Claude — OPEN, decide at review |

## Architecture (grounded in existing code)

**Realtime auth is the crux and reuses an existing pattern.** Hocuspocus `onAuthenticate` (`server/hocuspocus.ts:98`) rejects any connection whose JWT `scope !== documentName`. The Go mint endpoint `POST /api/realtime/token` (`realtime_token.go`, `MintToken`) resolves the doc-name scope in a switch that today handles `attempt:{id}` and rejects unknown scopes (`realtime_token.go:342`). There is already a **read-only** precedent: `attempt:` docs and a teacher-watch JWT carry `ctx.readOnly` (`hocuspocus.ts:123-128`).

So a canvas is `documentName = canvas:{canvasId}`, and permission is enforced **server-side at mint**:

- Owner of the canvas → write JWT (`readOnly=false`).
- Session member AND `canvas.is_shared` → read-only JWT (`readOnly=true`).
- Otherwise → 403 at mint (no token, no room access).

`onAuthenticate` carries `readOnly` into context for `canvas:` docs exactly like `attempt:`; a read-only connection must reject document updates (verify Hocuspocus enforces `connection.readOnly`, else drop writes in `onChange`/`beforeHandleMessage`). Client also renders Excalidraw with `viewModeEnabled` for viewers — UX only; **the server token is the security boundary.**

## Phases

### Phase 1 — Backend: schema, canvas store + API, token mint *(Codex)*
- Migration + `schema.ts`: `session_canvases` (`id`, `session_id` FK, `owner_id` FK, `title`, `is_shared` bool default false, `created_at`, `updated_at`) with org/tenant scoping consistent with `sessions`.
- `store/canvases.go`: create, get, list-visible-to-user (own + shared-in-session), set-shared, ownership checks.
- `handlers/canvases.go`:
  - `POST /api/sessions/{id}/canvases` (any session member creates; becomes owner).
  - `GET /api/sessions/{id}/canvases` (returns caller's own + shared canvases per Decision 6).
  - `PATCH /api/sessions/{id}/canvases/{canvasId}` (owner-only: `is_shared`, `title`).
- `realtime_token.go`: add `canvas:{canvasId}` to the scope resolver — mints write for owner, read-only for a session member of a shared canvas, 403 otherwise.
- **y-excalidraw vetting gate** (blocks Phase 3): record findings in this plan.
- Go tests: ownership, sharing visibility, cross-user isolation, cross-org isolation, mint write-vs-readonly-vs-403.

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
- **Scenarios:** owner creates a canvas → draws (persists) → shares → a second session member sees it read-only → the viewer's write attempt is rejected server-side → a non-member/other-org user is denied at mint. Reconnect restores content.
- **Fixtures:** two session members + one outsider; a session from the existing session test harness.
- **Live vs fast:** the mint/permission/persistence assertions are Go integration (fast, DB-backed); the Excalidraw render + view-mode is a component test; one Playwright spec covers create→share→view end-to-end (**not run against a live stack** — needs the booted stack + pinned `E2E_BASE_URL`, per `docs/testing.md`).
- **Acceptance criteria (exact test names must exist + pass):** `TestMintToken_Canvas_OwnerWrite`, `TestMintToken_Canvas_SharedViewerReadOnly`, `TestMintToken_Canvas_NonMemberDenied`, `TestCanvasStore_ListVisible_OwnAndShared`, `TestCanvases_CrossOrgIsolation`, plus the read-only-write-rejected realtime test.

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
- Per-recipient sharing (Decision 4 is session-wide boolean), export/import, templates, image/file embeds beyond Excalidraw defaults, presenter laser-pointer, and canvas-level comments. Each a follow-up if wanted.
- Retrofitting whiteboards onto class-bound vs class-less distinctions beyond what session membership already implies.

## Plan Review

_Pending — Tier A, 4-way (self Opus 5 + Codex + independent Opus + GLM). Reviewers MUST confirm the named integration-tests phase (present) and scrutinize the realtime-auth mint + read-only enforcement._

## Code Review

_Pending._

## Post-Execution Report

_Pending._
