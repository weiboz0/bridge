# Plan 094 — Excalidraw whiteboards in live sessions

**Branch:** `feat/094-session-whiteboard`
**Status:** Phases 1a through 6 are complete.
Spec 013 passed its exact-commit Sol + Fable 5 design gate; the user approved the remediation scope widening on 2026-08-11.
The Spec 013 remediation plan gate reached consensus at exact substantive commit `fdaf90af2a6c9500396ced27f2ca0df4be780645`.
Phase 7 is complete; Phases 8 through 13 are authorized for phase-by-phase implementation.

## File scope

`drizzle/**` (new migration) ·
`src/lib/db/schema.ts` ·
`platform/internal/store/canvases.go` (new) + `platform/internal/store/canvases_test.go` (new) ·
`platform/internal/handlers/canvases.go` (new) + `platform/internal/handlers/canvases_integration_test.go` (new) ·
`platform/internal/handlers/realtime_token.go` + `platform/internal/handlers/realtime_token_test.go` ·
**PHASE-6 SCOPE:**
**`platform/internal/handlers/user_fixture_test.go`** (new test-only fast user fixture) ·
**`platform/internal/handlers/access_org_test.go`** · **`platform/internal/handlers/admin_live_admin_test.go`** · **`platform/internal/handlers/admin_parent_links_test.go`** · **`platform/internal/handlers/admin_test.go`** ·
**`platform/internal/handlers/annotations_integration_test.go`** · **`platform/internal/handlers/books_integration_test.go`** · **`platform/internal/handlers/chapters_integration_test.go`** ·
**`platform/internal/handlers/internal_sessions_test.go`** · **`platform/internal/handlers/me_test.go`** · **`platform/internal/handlers/org_list_integration_test.go`** · **`platform/internal/handlers/org_parent_links_test.go`** ·
**`platform/internal/handlers/org_self_action_guard_test.go`** · **`platform/internal/handlers/parent_test.go`** · **`platform/internal/handlers/problems_integration_test.go`** (also owns the shared `integrationDB`) ·
**`platform/internal/handlers/schedule_test.go`** · **`platform/internal/handlers/sessions_integration_test.go`** · **`platform/internal/handlers/sessions_page_integration_test.go`** ·
**`platform/internal/handlers/teacher_parent_links_test.go`** · **`platform/internal/handlers/topics_link_chapter_test.go`** · **`platform/internal/handlers/topics_strict_decode_test.go`** (the 20 newly scoped direct-call-site files; with `canvases_integration_test.go` and `realtime_token_test.go` already listed above, the exact census is 22) ·
**`platform/internal/auth/realtime_jwt.go`** (add `readOnly` claim — scope-widened R1) ·
**`platform/internal/auth/realtime_jwt_test.go`** (JWT compatibility regression) ·
**`platform/internal/db/migrations.go`** (latest schema probe + sentinels — scope-widened verification fix) ·
**`platform/internal/db/schema_probe.go`** + **`platform/internal/db/schema_probe_test.go`** + **`platform/internal/db/schema_probe_parity_test.go`** + **`platform/internal/db/schema_probe_integration_test.go`** (multi-object probe + tests — scope-widened verification fix) ·
**`platform/internal/store/orgs_test.go`** (shared store test-DB guard) · **`platform/tests/contract/cleanup_test.go`** (remove non-test fallback and guard cleanup) ·
**`server/realtime-jwt.ts`** (mirror the claim — scope-widened R1) ·
**`platform/cmd/api/main.go`** (wire `CanvasStore` + handler — scope-widened R1) ·
`server/hocuspocus.ts` + `server/hocuspocus.canvas.test.ts` (new — read-only + ended-write realtime tests) ·
`next.config.ts` (only if `/api/sessions/{id}/canvases` isn't already covered by `/api/sessions/:path*`) ·
`src/lib/whiteboard/**` (new — the binding + hook) ·
**`src/lib/yjs/use-yjs-provider.ts`** + **`tests/unit/use-yjs-provider.test.ts`** (shared retry classification and bounded reconnect policy; preserve attempt/session compatibility) ·
**`src/lib/whiteboard/vitest.config.ts`** (mirror Bun/Vitest Zod boundary) ·
`src/components/session/whiteboard/**` (new — canvas list, board surface, visibility control) ·
`src/components/session/teacher/teacher-dashboard.tsx` · `src/components/session/student/student-session.tsx` (add the whiteboard surface) ·
**`src/app/(portal)/sessions/[id]/page.tsx`** (link its ended-session notice to the archive) ·
**`src/app/(portal)/teacher/sessions/[sessionId]/page.tsx`** (link its ended-session notice to the archive) ·
**`src/app/(portal)/sessions/[id]/whiteboards/page.tsx`** (new, dedicated read-only archive route) ·
**`src/app/(portal)/teacher/page.tsx`** · **`src/app/(portal)/teacher/sessions/page.tsx`** · **`src/app/(portal)/teacher/classes/[id]/page.tsx`** · **`src/app/(portal)/student/classes/[id]/page.tsx`** (link ended-session history rows to the archive) ·
**`tests/unit/teacher-session-row.test.tsx`** · **`tests/unit/ended-sessions-non-link.test.ts`** · **`tests/unit/sessions-room-page.test.tsx`** (update ended-session expectations) · **`tests/unit/whiteboard-archive.test.tsx`** (new archive interaction regression) ·
**`tests/unit/whiteboard-panel.test.tsx`** (new live-panel create/list/floor/visibility and scene-UX regression) ·
**`tests/unit/zod-vitest-interop.test.ts`** (new Bun/Vitest named-export and error-identity regression) ·
**`tests/unit/excalidraw-yjs.test.ts`** (custom-binding regression) · `package.json` + **`bun.lock`** (add `@excalidraw/excalidraw`; no `y-excalidraw`) ·
**`vitest.config.ts`** (Bun/Vitest Zod interop — scope-widened local-gate fix) ·
**`scripts/check-test-database-url.mjs`** (new) · **`scripts/ci-local.sh`** · **`scripts/tests/test-guards.sh`** (parsed/pinned local-gate database guard — provisional Round-10 governance scope) ·
**`AGENTS.md`** (classify the validator and its executable guard proof as governance; correct the LLM-isolation contract; mirror the permanent review gates, unavailable-reviewer rule, uncapped consensus safeguard, and explicit test-model override) ·
`docs/api.md` · `docs/architecture/decisions.md` · **`docs/testing.md`** · `README.md` · **`.claude/skills/br-system-review/SKILL.md`** (operator probe guidance) · this plan file.

**DESIGN-REMEDIATION SCOPE (approved by the user 2026-08-11 after Spec 013 consensus):**
**`docs/specs/013-session-whiteboard-review-remediation.md`** ·
**`platform/internal/realtime/canvas_control.go`** + **`platform/internal/realtime/canvas_control_test.go`** (new bounded Go control client) ·
**`platform/internal/store/session_lifecycle.go`** + **`platform/internal/store/session_lifecycle_test.go`** (new advisory keys, leases, confirmed/degraded persistence helpers) ·
**`platform/internal/store/sessions.go`** + **`platform/internal/store/sessions_test.go`** ·
**`platform/internal/store/schedule.go`** + **`platform/internal/store/schedule_test.go`** ·
**`platform/internal/handlers/sessions.go`** + **`platform/internal/handlers/sessions_test.go`** + **`platform/internal/handlers/sessions_integration_test.go`** ·
**`platform/internal/handlers/schedule.go`** + **`platform/internal/handlers/schedule_test.go`** + **`platform/internal/handlers/schedule_auth_integration_test.go`** ·
**`platform/internal/config/config.go`** + **`platform/internal/config/config_test.go`** ·
**`server/canvas-lifecycle.ts`** + **`server/canvas-lifecycle.test.ts`** (new freeze serializer, admission, accounting, and control listener) ·
**`src/app/api/sessions/[id]/route.ts`** (remove the shadow PATCH producer; retain GET only) ·
**`src/lib/sessions.ts`** (remove the TypeScript create/end writers) ·
**`src/components/teacher/start-session-button.tsx`** + **`tests/unit/start-session-button.test.tsx`** (new replacement warning regression) ·
**`src/components/teacher/scheduled-session-list.tsx`** + **`tests/unit/scheduled-session-list.test.tsx`** (new rendered scheduled-start consumer and replacement warning regression) ·
**`tests/unit/shadow-routes.test.ts`** · **`TODO.md`** ·
**`e2e/session-whiteboard.spec.ts`** + **`e2e/helpers.ts`** + **`e2e/helpers/**`** + **`e2e/seed.setup.ts`** (new lifecycle flow, fixtures, and controlled failure seam; explicit pinned-stack execution only) ·
**`.env.example`** · **`docs/setup.md`** · **`docs/project-structure.md`** (server-only control configuration and ports) ·
**`docs/reviewers.md`** + **`docs/development-workflow.md`** + **`docs/coding-agent.md`** (permanent review-gate and dispatch contracts).

The existing broad entries for `drizzle/**`, `src/lib/db/schema.ts`, `platform/cmd/api/main.go`, `platform/internal/handlers/realtime_token*`, `server/hocuspocus*`, whiteboard frontend files, schema-probe files, documentation, and tests remain authoritative for remediation changes in those paths.

Scope-widening (Spec 013 remediation) authorized by the user 2026-08-11 via “approved” after the Sol + Fable 5 design gate passed.
This approval covers the exact additions above, including the governance files; database migrations remain subject to the non-test-database hard safeguard, and E2E remains forbidden without a separately started Bridge stack plus explicit pinned `E2E_BASE_URL`.

Governance provenance: before this revision the user explicitly directed that “all plan reviews” and then “all review” pursue consensus rather than stop at a numeric cap; the later design-gate direction separately fixed the permanent design roster to Sol + Fable 5 and removed its round cap.
Phase 7 materializes both directions without retroactively changing the gate governing this committed plan revision.

Scope-widening (R1 blocker 1 / concern C1) authorized by the user 2026-08-06: the read-only viewer boundary cannot be built without a `readOnly` claim in both JWT files, and `CanvasStore` must be wired in `main.go`.

Scope-widening (archive-route decision) authorized by the user 2026-08-07: ended-session whiteboards must be reachable without re-enabling the live dashboards, so add the two existing ended-session route files and one dedicated archive page.

Scope-widening (archive-entry review fix) authorized by the user 2026-08-08: add every existing teacher/student ended-session history entry point so former readers have a durable archive link rather than depending on a live SSE redirect.

Scope-widening (archive test review fix) authorized by the user 2026-08-08: add the existing ended-session regression tests and one focused archive test, so the new archive contract is enforced rather than contradicted by stale no-link assertions.

Scope-widening (scope-audit review fix) authorized by the user 2026-08-08: add the already changed lockfile, Go JWT regression, and custom-binding test so every branch artifact is governed by the plan.

Scope-widening (schema-probe verification fix) authorized by the user 2026-08-09 via “resume”: add `platform/internal/db/migrations.go`, because migration `0028_session_canvases.sql` became the latest schema-bearing migration and the enforced bidirectional parity test requires its table, columns, constraints, and indexes to replace the prior `books` sentinels.

Scope-widening (complete multi-object probe) authorized by the user 2026-08-09: add the probe implementation, its unit/parity/integration tests, and the tracked operator-review skill. Review found that retargeting only `migrations.go` would leave three `books`-specific integration tests stale and would ignore `0028`'s new enum plus `sessions.canvas_floor` alteration.

Scope-widening (local-gate unblock) uses the user's exact 2026-08-10 authorization: “you are authorized to edit or add any files in this repository, no need to ask for scope question anymore.”
Hard safeguards in `AGENTS.md` remain non-waived by that authorization; the named governance changes still require this plan gate.
The provisional additions are the 22 enumerated handler tests, new fixture/interop regressions, root Vitest configuration, and parsed/pinned local-gate database guard named above; their gate must still pass before implementation.
The authoritative gate proved that `internal/handlers` cannot complete its fixed 120-second timeout while ordinary fixtures repeatedly perform cost-10 bcrypt work, and Bun 1.3.12's Vitest fork runner resolves Zod's externalized ESM namespace without its named `z` export.

## Problem / goal

A live session is a shared code editor today. Add Excalidraw whiteboards, synced over the existing Yjs/Hocuspocus plumbing, with an **ownership + floor-based visibility** model. Persisted, and viewable after the session ends.

## Decisions (settled with the user)

| # | Decision | Source |
|---|----------|--------|
| 1 | **Ownership.** A *canvas* has one **owner** (durable — not tied to current session membership). The owner may write while the session is live. Who else may **read** is governed by visibility (Decision 4). | User |
| 2 | **Persisted + outlives the session** (read-only archive after end — Decision 8). Persisted via Hocuspocus `onLoadDocument`/`onStoreDocument` like attempt/session docs. | User |
| 3 | **Binding: `y-excalidraw`** with a Phase-1b vetting gate (maintenance, license, React 19, honors a read-only/view mode). **Fallback:** thin custom onChange↔Y.Map binding per `src/lib/yjs/use-yjs-tiptap.ts`. Server read-only enforcement is ours regardless. | User |
| 4 | **Visibility = 4 ordered levels + session floor.** Levels tight→loose (each a strict superset of the previous): **`private`** (owner only) < **`host`** (owner + teacher) < **`participants`** (+ users with a `present` participant row — those who actually joined) < **`session`** (anyone who can access the session per `CanAccessSession` — see Decision 11). Host sets one **session floor** = the minimum any canvas may have; **default `private`**. A canvas's visibility is any level **≥ floor**. Ordering is **native pgEnum declaration order** (`private`,`host`,`participants`,`session`) — `visibility < 'host'` works in SQL. | User (R2: added `participants`) |
| 5 | **Multiple canvases per owner**, with a **per-session cap** (Decision 10). | User |
| 6 | **Host override: floor only, for MVP — and the floor is capped at `participants`.** No per-canvas host moderation yet; the floor (max `participants`) is the supervision lever. The host cannot set the floor to `session` — that would force students' boards world-public without consent (R3). | User + R3 Opus |
| 7 | **Loosen-only — strict, no tightening.** An owner may only *raise* a canvas's visibility; tightening is rejected (400). Visibility is monotonic, so a minted read token never grants more than the canvas's *current* level — **no canvas-tightening revocation problem.** *Membership-change revocation is separate and bounded:* a viewer who **leaves** the session while holding a read token keeps reading until the token TTL (~25 min) or a reconnect re-checks — **accepted for MVP** (matches existing session/attempt-doc behavior), not denied. Trade-off: an accidental over-share isn't undoable by the owner in MVP (host-moderation follow-up). | User (2026-08-06); reworded R2 |
| 8 | **After the session ends: read-only archive.** No new write authorization or persistence is allowed once `status=ended`; the confirmed lifecycle fence prevents post-boundary apply/relay, while a degraded end may permit only the finite already-authorized transient fan-in defined by Spec 013 before every later frame/reconnect is denied. `onStoreDocument` and every mutating endpoint remain durable backstops. Readable by the **owner**, the **teacher** (if visibility ≥ `host`), and **former participants** (if visibility ≥ `participants`) — where "former participant" = a `session_participants` row with status **`present` or `left`** (NOT `invited`/never-joined). Canvas auth special-cases ended sessions rather than reusing the live gate (which returns `ended → no_access`). | User (2026-08-06); status filter added R2; lifecycle semantics superseded by Spec 013 |
| 9 | **Membership checks by level, reusing existing helpers, not re-rolled.** `host` → `sessions.teacher_id`. `participants` → a `present` `session_participants` row (the strict join check — *not* the public-open-join clause). `session` → `CanAccessSession` (the plan-090 guard, which for a public class-less session admits any authenticated user — this is intentional per Decision 11, not a leak). A hand-rolled membership query is how 090's cross-org leak would reappear — reuse the named helpers. | Reviewers (all 3) + user |
| 10 | **Per-session canvas cap** (e.g. 50) enforced at create under the session-row lock — bounds persisted-doc growth. | Reviewer (opus C5) |
| 11 | **`session` visibility is intentionally "as public as the session" while live.** In a public, class-less live session (any authenticated user can join), a `session`-visibility canvas is readable by any authenticated user — the board is exactly as public as the room. On end, public admission ends too: `session` and `participants` collapse to the Decision-8 former-participant archive rule (`present`/`left` only). An owner who wants join-only sharing picks **`participants`** instead. This makes the plan-090 public surface a *conscious owner choice per canvas*, not an accidental cross-org leak. Documented in `docs/api.md` + `decisions.md`. | User (R2 trust-model fork); clarified after GLM archive review 2026-08-08 |
| 12 | **Archive UI is a dedicated neutral route.** `/sessions/{id}/whiteboards` renders only the read-only whiteboard archive rather than reviving either live teacher or student dashboard. It never writes a scene or offers mutation controls, even if opened while a session is still live. It does not pre-authorize through the ordinary session page APIs, because `CanAccessSession` deliberately total-rejects every ended session — including the teacher and former participants — while the canvas list and minted `canvas:{id}` token have explicit archive branches for owner, teacher, and former participant visibility. Those two endpoints remain the metadata and document authorization boundaries. The neutral live-session route redirects its otherwise-404 former-participant path to this archive, which returns a generic empty state for callers with no visible canvases and never attempts a document token mint until an item is selected. | User (2026-08-07); tightened after archive-route review 2026-08-08 |
| 13 | **Migration 0028 advances the startup schema probe as a multi-object end-state contract.** `session_canvases` becomes the primary probe table. Its eight declared columns and two indexes are sentinels; its named-constraint list is empty. The probe also verifies `sessions.canvas_floor`, nullable `canvas_freeze_token uuid`, nullable `canvas_freeze_until timestamptz`, nullable `whiteboard_server_archive_complete boolean`, and exact ordered values of the new `canvas_visibility` enum. Parity tests extract these declarations from the latest migration. Generic named-constraint checking remains covered with an injected test sentinel even though 0028 declares no named constraint. | Verification review + user (2026-08-09) |
| 14 | **Canvas creation is least privilege.** Only the represented session teacher or a participant whose row is currently `present` may create a canvas while the session is live. Invited/left users and public outsiders are denied; platform administrators and impersonators apply only the represented user's row and receive no independent bypass. Ended sessions reject creation. The cap remains serialized under the session/lifecycle lock. | Spec 013 consensus resolving Review-1 findings 8 and 11 |

## Architecture (grounded; revised per R1)

A canvas is `documentName = canvas:{canvasId}`. Permission is enforced **server-side at mint** (`realtime_token.go` scope resolver, which today handles `attempt:` and rejects unknown scopes).

**The read-only boundary must be built (R1 blocker 1).** Today `RealtimeClaims` carries only `sub`/`role`/`scope`, and `hocuspocus.ts` hardcodes `ctx.readOnly=false`; Hocuspocus enforces writes via the connection's `readOnly`, not a context field. So:
- Add a **`readOnly bool` claim** to `RealtimeClaims` in **both** `platform/internal/auth/realtime_jwt.go` and `server/realtime-jwt.ts`.
- `MintToken` sets `readOnly` per the decision below.
- `hocuspocus.ts onAuthenticate` reads `claims.readOnly` and sets the **connection's `readOnly`** (Hocuspocus's write-enforcing field), not just context. Phase 2 adds a test that a `readOnly` connection's document update is rejected server-side. Existing `attempt:` behavior is preserved (owner-only, `readOnly=false`).

**Mint matrix — live session** (`status != ended`). Requester qualifies for the *canvas's visibility level* per Decision 9:
- **owner** → write (`readOnly=false`).
- visibility=`host` and requester = `teacher_id` → read.
- visibility=`participants` and requester = `teacher_id` OR has a `present` participant row → read.
- visibility=`session` and `CanAccessSession(session, requester)` allows → read (intentionally broad for public sessions — Decision 11).
- else → **403**. (`private` mints only for its owner.)

**Implement as an admit-tier compare, not a scalar "eligible-level ≥ visibility"** (R3 Codex — that shorthand was wrong: it would let a `participants`-eligible requester read a `host`-only canvas). Assign the requester the **tightest** level that admits them — owner→`private`, teacher→`host`, present-participant→`participants`, session-accessible→`session`, none→deny — and allow iff `admit_tier ≤ canvas.visibility`. Teacher (admit `host`) reading a `participants` canvas: `host ≤ participants` ✓; participant (admit `participants`) reading a `host` canvas: `participants ≤ host` ✗ denied. This is the per-level matrix above, expressed as one comparison.

**Mint matrix — ended session** (Decision 8): **everyone `readOnly=true`, including the owner** (archive). Reads: owner; teacher if visibility ≥ `host`; former participant (`session_participants` status `present`/`left`) if visibility ≥ `participants`. `session` and `participants` collapse to "former participant" in the archive (no live session to be public to).

**Historical pre-remediation boundary (superseded by Spec 013 and Phases 8–10).** Mint alone and `onStoreDocument` alone are insufficient, so the branch added a `beforeHandleMessage` current-state recheck before `MessageReceiver.apply` and kept storage rejection as a durable backstop.
That recheck is now defense in depth: the confirmed path gains the lifecycle lease/fence/lock protocol, while the degraded path honestly permits only the finite already-authorized transient fan-in defined by Spec 013 rather than claiming cross-process apply-and-relay atomicity.

**Session floor is capped at `participants` (R3 Opus — trust-model guard).** The host may set the floor to `private`/`host`/`participants`, never `session`. A `session`-visibility canvas is therefore *only ever an owner's per-canvas choice* (Decision 11), never imposed on students by a floor raise — otherwise a host could make every private student board world-readable in a public room without consent. Supervision needs (`participants`/`host`) are fully served; publishing a whole room is not a floor power.

**Floor invariant is race-safe (R1 blocker 2 + R2 blocker 5).** `visibility ≥ floor` can't be a Postgres cross-table CHECK, so **create**, **set-visibility**, AND **set-session-floor** all take `SELECT … FOR UPDATE` on the `sessions` row; create/set-visibility compute against the locked floor (`GREATEST` / reject `< floor`). This serializes all three writers so no canvas ever lands below the floor.

## Phases

### Phase 1a — Backend: schema + store *(Codex)*
- Migration **`drizzle/0028_*`** (0027 is taken by plan 090 — run `scripts/check-migration-uniqueness.sh`) + `schema.ts`: `canvas_visibility` pgEnum `(private, host, participants, session)` (declaration order = tight→loose); `sessions.canvas_floor canvas_visibility NOT NULL DEFAULT 'private'` (**backfills existing rows**); `session_canvases` (`id`, `session_id` FK, `owner_id` FK, `title`, `visibility canvas_visibility NOT NULL`, `created_at`, `updated_at`), org/tenant scoping like `sessions`.
- `store/canvases.go`: create (locks session row; `visibility = GREATEST(requested, floor)`; per-session cap under the lock), get, `list-visible-to-user` (owner/host/participant/session branches, scoped to path `session_id`, reusing the Decision-9 helpers; **special-case ended sessions the same way mint does** (R3 GLM) — for `ended`, use the archive rules, since `CanAccessSession` returns `ended→no_access` and would otherwise hide the archive from former participants), set-visibility (**locks session row**; loosen-only: reject target ≤ current or < floor), set-session-floor (host-only; **rejects `session` — floor capped at `participants`, R3**; locks session row; bumps `visibility < floor` up to floor in-txn; **lowering the floor is allowed** and leaves existing canvases as-is). DELETE purges the canvas row **and its persisted Yjs doc** (no orphan).
- `store/canvases_test.go`: floor default + backfill; `GREATEST` at create; **concurrent create-vs-raise AND set-visibility-vs-raise leave nothing below floor**; loosen accepted, tighten rejected; raise-floor bumps; list-by-role across all 4 levels; per-session cap; cross-org isolation; an enum-ordinal assertion (`private<host<participants<session`) so a future reorder can't invert the floor compare.

### Phase 1b — Backend: handlers + token mint + JWT claim *(Codex; historical contract superseded where Phases 8–12 say otherwise)*
- `auth/realtime_jwt.go` + `server/realtime-jwt.ts`: add `readOnly` claim (both must stay byte-compatible — same field name/JSON tag).
- `handlers/canvases.go`: the branch initially added member creation and host-only `PATCH …/settings`; Decision 14 and Phase 9 supersede those two contracts with represented-teacher/present creation and the atomic dedicated canvas-settings GET/PATCH cutover. The remaining list, owner loosen/title, delete, and ended-session rejection behavior carries forward.
- `realtime_token.go`: add `canvas:{cid}` to the scope resolver implementing the live/ended mint matrix; set the `readOnly` claim. Refactor the shared document authorization result to carry `{role, readOnly}` so `POST /api/internal/realtime/auth` returns the same current `readOnly` decision to Hocuspocus (existing scopes default to `false`; canvas owners become `true` when ended).
- `main.go`: wire `CanvasStore` + handler.
- `canvases_integration_test.go` + `realtime_token_test.go`: the mint matrix (below), incl. ended-session archive.
- **y-excalidraw vetting gate** (blocks Phase 3) — record findings here.

### Phase 2 — Realtime: read-only enforcement + persistence *(Codex / inline; historical boundary superseded by Phases 8–10 for temporary freeze and degraded fan-in)*
- `hocuspocus.ts`: handle `canvas:` in `onAuthenticate` — read `claims.readOnly`, set the **connection's `readOnly`** (Hocuspocus 3.4.4's write-enforcing `connectionConfig.readOnly`, mutated in `onAuthenticate` — verified propagates to `Connection.readOnly`), carry context. A token missing `readOnly` defaults to `false` so existing `attempt:` tokens are unaffected. Add a `beforeHandleMessage` guard for mutation-bearing canvas sync frames while `connection.readOnly=false`: call the existing Go internal recheck and flip `connection.readOnly=true` only for a permanent viewer/ended decision before `MessageReceiver` handles the frame. Phases 8–10 supersede generic fail-closed handling for an active freeze with the categorized retryable `session_freezing` outcome and do not claim degraded-path apply/relay atomicity. Non-mutating sync/awareness/query traffic bypasses this per-mutation recheck. `onLoadDocument`/`onStoreDocument` persist the canvas Yjs doc (debounced snapshot). **`onStoreDocument` also drops/rejects writes when the session is `ended`** as a durable backstop.
- `server/realtime-jwt.ts`: extend `rechckDocumentAccess`'s response type with `readOnly`, preserving fail-closed handling for every non-200 response.
- Tests file: **`server/hocuspocus.canvas.test.ts` (new — already in File scope)**. Assertions (the security core): a `readOnly` connection's update is **rejected server-side**; owner writes persist + restore on reconnect; a non-eligible user cannot connect; a writer connected before end sends a mutation after end and the pre-apply recheck flips it read-only, the mutation is not present in the document and an already-connected observer receives no update; `onStoreDocument` independently refuses a late ended-session write; awareness/non-mutating traffic does not invoke the mutation recheck.

### Phase 3 — Frontend: Excalidraw surface + binding *(Sonnet)*
- `@excalidraw/excalidraw` + `y-excalidraw` (or fallback). `src/lib/whiteboard/use-whiteboard.ts` binds a `canvas:{id}` Yjs doc (via `useYjsProvider` + a minted canvas token) to Excalidraw; **dynamic-import** the board (bundle size).
- `src/components/session/whiteboard/`: canvas list (visible-to-me), create, owner visibility control (**loosen-only UI — levels ≤ current are disabled, not hidden; equal is a no-op**), board surface. Non-writers → `viewModeEnabled` (UX; the token is the real boundary).
- Add a "Whiteboard" surface to `teacher-dashboard.tsx` + `student-session.tsx` (thin entry points).
- Add the dedicated `/sessions/{id}/whiteboards` archive route and links from both existing ended-session notices and all existing ended-session history rows. The archive page intentionally renders only list + board, always forces `viewModeEnabled`, suppresses local Yjs writes, and never mints a `canvas:` token until a visible item is selected; `student-session.tsx` redirects its `session_ended` event there, and `teacher-dashboard.tsx` redirects a successful end action there. The neutral route sends its otherwise-404 former-participant fallback to the archive rather than reusing the live student-page authorization path.
- Frontend tests explicitly cover: archive direct access while live is view-only **and its change callback creates no Yjs transaction/provider update**; the initial archive render calls only `GET /canvases` (never `teacher-page`, `student-page`, or `join`), an empty list mints no token, and selecting one returned item makes exactly one realtime mint for `canvas:{id}`; no create/visibility/live-dashboard controls render; an ended former participant sees their visible archive, while an ended public non-participant gets the generic empty state; teacher end redirects only after a 2xx response (a non-2xx remains on the live dashboard), student-end redirects target the archive; each ended notice/history row links to it.
- Frontend tests: list by role; owner edit vs viewer view-mode; loosen control; create; ended-session read-only.

### Phase 4 — Integration tests (NAMED — required: API + realtime auth + persistence) *(Opus)*
- **Fixtures:** host + two members + one non-member + one other-org user; a session from the existing harness; an ended-session fixture.
- **Acceptance (exact names must exist + pass):** `TestMintToken_Canvas_OwnerWrite`, `TestMintToken_Canvas_HostReadWhenHostVisible`, `TestMintToken_Canvas_HostDeniedWhenPrivate`, `TestMintToken_Canvas_ParticipantReadWhenParticipantsVisible`, `TestMintToken_Canvas_NonParticipantDeniedWhenParticipantsVisible` (a non-joined but session-accessible user is denied at `participants` level), `TestMintToken_Canvas_SessionVisiblePublicAllowsAnyAuthed` (Decision 11 — public class-less session), `TestMintToken_Canvas_SessionVisibleClassBoundDeniesOutsider`, `TestMintToken_Canvas_MemberDeniedWhenHostVisible`, `TestMintToken_Canvas_NonMemberDenied`, `TestMintToken_Canvas_OtherSessionMemberDenied`, `TestMintToken_Canvas_EndedArchive_OwnerRead`, `TestMintToken_Canvas_EndedArchive_TeacherReadWhenHostVisible`, `TestMintToken_Canvas_EndedArchive_FormerParticipantReadNotInvitee`, `TestMintToken_Canvas_EndedArchive_PublicViewerDenied` (a live public `session` viewer is not a former participant and loses archive access), `TestMintToken_Canvas_EndedArchive_OutsiderDenied`, `TestMintToken_Canvas_EndedArchive_AllReadOnly`, `TestMintToken_Canvas_HostReadsParticipantsAndSessionLevels`, `TestCanvasStore_SetFloor_RejectsSession`, `TestCanvasStore_SetVisibility_RejectsTightenAndBelowFloor`, `TestCanvasStore_ConcurrentCreateVsRaiseFloor_NoneBelowFloor`, `TestCanvasStore_ConcurrentSetVisibilityVsRaiseFloor_NoneBelowFloor`, `TestCanvasStore_RaiseFloor_BumpsCanvases`, `TestCanvasStore_ListVisible_ByRole`, `TestCanvasStore_PerSessionCap`, `TestCanvasStore_EnumOrdinalOrder`, `TestCanvases_ListEndedArchive_ByRole` (GET/list: owner, teacher at `host+`, present/left former participants at `participants+`; invited, public non-participant, and outsider denied/omitted), `TestCanvases_MutatingEndpointsReject_WhenEnded`, `TestCanvases_CrossOrgIsolation`, plus the Phase-2 read-only-write-rejected, pre-end-writer-post-end-no-relay, and ended-store-rejected realtime tests.
- **Live vs fast:** all the above are Go integration / realtime (fast, DB-backed). One Playwright spec covers create→loosen→view (**not run against a live stack** — needs the booted stack + pinned `E2E_BASE_URL`, per `docs/testing.md`).

### Phase 5 — Docs + verify
- `docs/api.md` (canvas endpoints + `canvas:` scope + the live/ended permission matrix + Decision 11's public-board semantics); `docs/architecture/decisions.md` (new §: whiteboard visibility floor + realtime read-only claim + the accepted MVP risks + a **note that `canvas_visibility` is append-only in Postgres, so inserting a level between existing ones later would break the `<` ordering** — a new level must go at an end or the compare must migrate to explicit ranks); `README.md` bullet.
- Advance the boot-time schema probe from `books`/0026 to migration 0028's full end state (Decision 13), and update the tracked system-review command/copy. Tests first define parity for the primary table, `sessions.canvas_floor`, exact enum values, and both indexes; preserve the generic constraint-check regression with an injected sentinel.
- Run `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./internal/db/ -count=1` from `platform/` before the plan-wide Go suite. The test database already has 0028; this phase runs no migration.
- `bash scripts/ci-local.sh` green (attestation); `pre-merge-guard.sh`.

### Phase 6 — Local-gate test infrastructure *(Terra; tests only)*
- Add `platform/internal/handlers/user_fixture_test.go` with one transaction-safe `insertFixtureUser` helper.
  Its exact signature is `insertFixtureUser(t *testing.T, db *sql.DB, input store.RegisterInput) *store.RegisteredUser`; it calls `t.Helper()` and uses `require`/`t.Fatal` for setup failures, so callers cannot ignore a rejected input.
  It inserts `users` plus the required email `auth_providers` row using one precomputed, valid **cost-4** bcrypt hash for `testpassword123`, rejects any other fixture password, accepts only nil/`teacher`/`student` `IntendedRole` values, returns `store.RegisteredUser`, and leaves cleanup ownership with each existing fixture.
  It begins one SQL transaction internally, rolls back on any failed user/provider statement, and commits only after both dependent rows exist.
  Production `UserStore.RegisterUser`, its cost-10 bcrypt behavior, auth-handler registration tests, and `platform/internal/store/users_test.go` remain unchanged as the real password-hashing coverage.
- Audit the current `RegisterUser` producer before replacement: it writes exactly the `users` row and one `auth_providers(provider='email', provider_user_id=user.ID)` row, with no normalization, membership, profile, or audit side effects.
  Add `TestInsertFixtureUser_MatchesRegisterUserPersistenceContract` to compare one real registration with one fixture registration across the observable user/provider shape while allowing their password hashes and generated IDs to differ.
  Direct inserts are preferred over a production bcrypt-cost option because exposing a low-cost constructor, field, or environment switch in production code would create a credential-hardening footgun; real registration remains covered in its dedicated producer/auth tests and this parity regression.
- Replace the 31 ordinary handler-test `RegisterUser` call sites in the 22 exact files enumerated in `## File scope`, including `problems_integration_test.go`.
  Repository inspection confirmed every one currently supplies `testpassword123`; any future password-variation or real-registration behavior test must continue using `UserStore.RegisterUser` rather than this helper.
  Repository inspection also found no handler test that asserts password-hash byte uniqueness; the deliberately shared test-only salt/hash is not production salt coverage.
  Preserve unique emails and each fixture's dependency-ordered cleanup.
  Duplicate-email, normalization, and real registration rejection behavior stays in the dedicated `UserStore.RegisterUser` and auth-handler tests; these 31 audited sites are happy-path fixture construction only.
- Harden the shared handler `integrationDB` before those direct inserts: parse `DATABASE_URL`, require its URL database name to end `_test`, open it, and independently require `SELECT current_database()` to end `_test` before any mutation.
  Use `url.Parse`, decode `strings.TrimPrefix(parsed.EscapedPath(), "/")`, and validate that decoded path value rather than the raw URL string.
  An absent URL may skip as today, but an unparseable URL, empty database path, query error, or non-test parsed/live database name must fail closed with `t.Fatal` or `require`; no migration runs.
  Audit the unchanged `auth_test.go` and `topics_unlink_test.go` consumers of this shared helper through the complete handler suite; no source edit is expected unless their behavior actually requires one and a reviewed scope revision names it.
- Add `TestInsertFixtureUser_PersistsHashRoleAndProvider` and `TestInsertFixtureUser_RejectsUnsupportedInputWithoutWrite`.
  They prove the cost-4 hash accepts `testpassword123`, valid `IntendedRole` round-trips, the email auth-provider row exists, and a different password or invalid role fails before any insert.
- Record the RED run as a measured lower bound (`internal/handlers` exceeds 120 seconds) and the GREEN package duration after replacement.
  Static call-site expansion suggests the four shared fixtures dominate, but is not treated as an exact runtime hash count because table-driven and skipped paths can change executions.
- Update only the stale teacher branch of `tests/unit/sessions-room-page.test.tsx` because `TeacherDashboard` no longer accepts `classId` or `returnPath`.
  Retain the student class-less `/sessions` override assertion; `tests/unit/whiteboard-archive.test.tsx` keeps the named “redirects the teacher to the archive only after a successful end response” coverage through the real dashboard.
- Add `ssr.noExternal: [/^zod(?:\/.*)?$/]` to the root `vitest.config.ts`.
  This is the durable test-runner compatibility boundary for the repository's locked Bun 1.3.12, Vitest 4.1.4, and Zod 4.3.6 combination: it keeps Zod inside Vite's transform pipeline without changing or downgrading application imports.
  Add `resolve.dedupe: ["zod"]`, an inline config comment, and `tests/unit/zod-vitest-interop.test.ts` proving the named `z` export works and a thrown validation error retains `ZodError` identity; also run the complete root Vitest suite so already-green consumers remain covered.
  Repository inspection finds no current `ZodError`/`ZodType`/`z.instanceof` identity consumer, while dedupe prevents dependencies from resolving a second physical root copy.
  Mirror the same `noExternal` and dedupe boundary in `src/lib/whiteboard/vitest.config.ts`, because it is a standalone configuration and the normal `bun run test` contract covers both Vitest processes.
- Replace `ci-local.sh`'s whole-string `_test` regex with the new `scripts/check-test-database-url.mjs` validator.
  Preserve and strengthen the existing ambient guard first: if `DATABASE_URL` is set, validate its decoded pathname and live `current_database()` even when a safe `TEST_DATABASE_URL` is also set, so a hostile ambient value cannot flow to any later step.
  Then resolve one gate URL from `TEST_DATABASE_URL`, `DATABASE_URL`, or the existing `bridge_test` fallback and validate it independently.
  The Node 18-compatible validator uses the existing `postgres` client with `max: 1`, a five-second connect timeout, a five-second query timeout around its sole `SELECT current_database()` probe, and URL-provided SSL behavior; it parses with `new URL`, requires `postgres:` or `postgresql:`, URL-decodes the non-empty pathname database name, opens one connection, requires the live database name to end `_test`, fails closed without retry on connection/query errors, closes before any gate mutation, and never prints the URL or credentials.
  `ci-local.sh` invokes it with the system `node` executable, and implementation starts with a Node 18 import smoke test for the existing `postgres` dependency.
  “Read-only” here means the validator executes only `SELECT current_database()` and performs no DML or DDL; the client is not relied on to enforce a read-only session mode.
  A parse-only mode exists solely for executable URL-parser self-tests, including both accepted and rejected cases, and is never used by the gate path.
  The shell owns a non-empty `GATE_DATABASE_URL` variable throughout and passes it to the validator through a dedicated environment variable; the validator returns only an exit status and prints no URL to stdout, so an empty-output capture cannot disable integration tests.
  Pass the validated gate URL explicitly as both `DATABASE_URL` and `TEST_DATABASE_URL` to the Vitest, Go, and E2E runner steps, so query-string decoys and ambient production URLs cannot reach mutating tests.
  Preserve all five empty provider-key exports on the Vitest step (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `DASHSCOPE_API_KEY`, `OPENROUTER_API_KEY`) so Bun cannot reload live billing credentials from `.env`.
  This duplicates the application-test check in `tests/helpers.ts` intentionally: the gate must reject before invoking a test runner, while the helper remains defense in depth for direct Vitest commands.
- Extend `scripts/tests/test-guards.sh` with executable validator cases for a valid `_test` path, percent-decoded `_test` path, empty path, wrong scheme, non-test path, and a production path whose query ends in `_test`; also enforce that the Go step pins both database variables.
  Include a test-path URL carrying `?dbname=production` and prove parse-only accepts only the safe pathname while documenting that the mandatory live gate check remains authoritative for routing/mapping overrides.
  Update `docs/testing.md` to name the decoded-path validation and shared pinned URL, and correct its identical stale LLM-isolation claim so it documents the five explicit empty provider-key exports rather than the unused `bun run --env-file=/dev/null` mechanism.
  The same guard file owns an allowlist scan that rejects direct handler-test `RegisterUser` calls outside `user_fixture_test.go`, and static assertions that the Vitest step preserves all five empty provider keys while Go pins both database variables.
  Add both `scripts/check-test-database-url.mjs` and `scripts/tests/test-guards.sh` to `AGENTS.md`'s governance-doc safeguard beside `scripts/ci-local.sh`, so future plans cannot weaken the validator or its load-bearing proofs without declaring them at gate time.
  Correct `AGENTS.md`'s stale LLM-isolation sentence and `scripts/ci-local.sh`'s matching stale header comment to describe the five explicit empty key exports actually used by the gate.
- Inventory and intentionally activate the existing Go integration tiers under the validated URL: `internal/db/db_test.go` is read-only; `internal/db/schema_probe_*_test.go` performs existing cleanup-safe schema DDL against `_test` only; the handler and store packages mutate fixture rows and clean them; and `tests/contract/cleanup_test.go` deletes only contract-pattern fixtures.
  This phase runs no migration, but the schema-probe integration DDL is expected test behavior rather than a skipped tier.
- `internal/store/canvases_test.go` already uses its separate parsed/live-checked `TEST_DATABASE_URL` opener; `internal/config/config_test.go` only tests configuration selection and performs no database I/O.
- Apply the same decoded-path plus live `current_database()` fail-closed guard to the shared store `testDB` in `platform/internal/store/orgs_test.go` before its 22 direct and transitive consumer files can mutate.
  Remove the non-test `bridge` fallback from `platform/tests/contract/cleanup_test.go`; when no validated URL is present its cleanup skips without connecting, and when present it verifies the live `_test` name before deletes.
- Add a guard assertion that handler tests contain no direct `RegisterUser` call outside the named parity/real-registration fixture test, so a copied cost-10 setup path cannot silently reintroduce the timeout.
- RED evidence is the exact gate failure: the isolated handler package times out inside `bcrypt.GenerateFromPassword` after 120 seconds; the root Vitest run reports `z.string` or `z.object` on an undefined named export; and the teacher room test receives an empty obsolete prop.
  GREEN commands are the pinned `go test ./internal/handlers -count=1 -timeout 120s` and `go test ./internal/store -count=1 -timeout 120s`, each with a reported duration at or below 60 seconds; full pinned `go test ./... -count=1 -timeout 120s`, whose handler and store package durations also remain at or below 60 seconds under package contention; both Vitest configurations; the Hocuspocus suite; and `bash scripts/ci-local.sh --fast` with Bun pinned in `PATH` and provider keys empty.
  The fast gate is phase-local evidence only; Phase 5's full `bash scripts/ci-local.sh` plus attestation remains the merge gate, still requires a user-pinned `E2E_BASE_URL`, and must run last so a later fast attestation cannot be mistaken for merge evidence.

### Phase 7 — Governance: permanent consensus review gates *(orchestrator; governance)*

- Update `AGENTS.md`, `docs/reviewers.md`, and `docs/development-workflow.md` so every new design spec has an exact-commit, read-only two-reviewer gate: Codex `gpt-5.6-sol` at high reasoning plus Claude Code `claude-fable-5`.
- Design, plan, and code-review gates iterate to consensus without a numeric round cap.
  Every three non-converged rounds produces a user-visible checkpoint; genuine repeated non-convergence or a judgment fork pauses for the user, but elapsed rounds alone never convert an open blocker into approval.
- This all-gate rule implements the user's earlier explicit “all plan reviews” and “all review” consensus directions; the Sol + Fable 5 roster applies only to design reviews unless a later user direction changes another roster.
- Preserve the existing risk-tiered plan/code roster unless the user explicitly changes it; remove the current `max_review_rounds = 3` cap and apply the same consensus/no-open-blocker rule.
- Replace `AGENTS.md`'s obsolete “unresolved findings at the round-cap” safeguard with an unresolved-review-finding safeguard that remains independent of elapsed rounds.
  Mirror the design roster and define that a temporarily unavailable required reviewer pauses the gate rather than being silently replaced or waived.
- Update `docs/coding-agent.md` to mirror the complete canonical design-gate rule as well as retaining domain dispatch and documenting that an explicit user model pin overrides the default.
  Mirror the override in `AGENTS.md`; for this remediation, all new or changed tests are delegated to Terra per the user's instruction, while reviewers remain independent and read-only.
- Extend `scripts/tests/test-guards.sh` with static governance checks proving the design roster names exactly Sol + Fable 5, review approvals bind to an exact commit, all three review gates are uncapped consensus loops, all four canonical documents mirror the rule, and no text retains a conflicting numeric cap.
- Run `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bash scripts/tests/test-guards.sh`, `bash scripts/check-plan-uniqueness.sh`, `bash scripts/check-spec-uniqueness.sh`, and `git diff --check` before committing this phase.

### Phase 8 — Durable lifecycle schema, locks, and replacement transitions *(Terra backend; tests by Terra)*

- Tests first create `platform/internal/store/session_lifecycle_test.go` and extend `sessions_test.go` and `schedule_test.go` for the exact signed advisory-key vectors, transaction-scoped lock order, 15-second database-clock lease, conditional confirmed end, false/no-snapshot degraded end, token cleanup, affected-count rollback, sorted collision-safe replacement locking, and explicit-end-versus-replacement races.
- Before rewriting migration 0028, record repository and remote-history evidence that it has not shipped through `main`.
  Rewrite `drizzle/0028_session_canvases.sql` and matching `src/lib/db/schema.ts` state with nullable `canvas_freeze_token`, `canvas_freeze_until`, and `whiteboard_server_archive_complete` columns; remove the dead `plain_text` column in the same pre-ship migration.
  Update the multi-object schema probe and parity tests to require the complete revised 0028 end state.
- Reconcile the already-used local `bridge_test` schema through an explicit approved test-database-only SQL step after both the decoded URL and live `current_database()` end in `_test`.
  Apply only the exact 0028 delta needed by the stale local test schema, record the SQL and before/after probe evidence, and never connect to or alter a non-test database.
- Implement `platform/internal/store/session_lifecycle.go` as the single owner of lifecycle advisory-key derivation, lock helpers, lease acquisition/validation/cleanup, conditional true end followed by atomic bundle persistence, and separate degraded transition.
  Canvas create, visibility, delete, and floor mutation take the shared lifecycle lock before their existing row lock and reject an unexpired lease with stable `409 session_end_in_progress` before writing.
  Canvas authorization takes only the shared lifecycle lock, locks no second entity, and exposes an unexpired lease to Hocuspocus as categorized retryable `session_freezing`, never a permanent read-only downgrade.
  A concurrent end request returns `409 session_end_in_progress`, never clears another token, and a stale request follows the exact different-token expired/unexpired rules from Spec 013.
- Refactor `SessionStore.CreateSession` and `ScheduleStore.StartScheduledSession` to take the class-replacement guard, acquire lifecycle locks in derived-key/UUID order, mark replaced live sessions archive-incomplete, clear leases, return replacement metadata, and preserve the newly created session atomically.
  Scheduled start re-reads the still-planned row after acquiring the class guard.
- Focused GREEN commands use only a parsed and live-verified `_test` database:
  `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./internal/store -run 'Test(SessionLifecycle|CreateSessionReplacement|StartScheduledSessionReplacement)' -count=1`,
  `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./internal/db -count=1`, and `go vet ./internal/store ./internal/db` from `platform/`.
  No migration or reconciliation may target a non-test database.

### Phase 9 — Go control client, every end producer, and atomic settings-route cutover *(Terra backend + Sonnet client; tests by Terra)*

- Tests first define `platform/internal/realtime/canvas_control_test.go` for strict URL/startup validation, loopback-only HTTP, verified non-loopback HTTPS, redirect refusal, bounded same-token retries, response-size/schema/base64/digest validation, transport-loss recovery, freeze/complete/unfreeze calls, and credential-safe errors.
- Implement `platform/internal/realtime/canvas_control.go` with injected `http.Client`, one overall two-second freeze budget, bounded jitter, strict 48 MiB response reading, exact bundle validation, and best-effort terminal calls.
  Add server-only configuration to `platform/internal/config/config.go`, startup validation/wiring in `platform/cmd/api/main.go`, and no reuse of the realtime signing secret.
  Preserve Spec 013's production `HOCUSPOCUS_CONTROL_PORT=4001` default and derive the numeric-loopback internal URL from it; require a separately generated control secret, and fail startup on a missing/invalid secret or invalid explicit override.
- Extend `RealtimeHandler` with the control-bearer-protected `POST /api/internal/canvas-sessions/freeze-auth` callback.
  It takes the shared lifecycle advisory lock and returns positive integer `remainingMs` only for the exact live unexpired token.
- Refactor the existing ordinary `POST /api/internal/realtime/auth` canvas authorization path to take the same shared lifecycle lock around its current-state decision; a freeze therefore cannot pass validation while an ordinary mutation authorization is outstanding.
  Its unexpired-lease response is distinct from permanent deny/read-only and `beforeHandleMessage` maps it to retryable `session_freezing` without mutating `connectionConfig.readOnly`.
- Replace `PATCH /api/sessions/{id}/settings` with dedicated `GET` and `PATCH /api/sessions/{id}/canvas-settings` routes in the same phase, with no compatibility alias.
  GET returns exactly the floor plus the optional durable archive-complete boolean; PATCH accepts only the floor schema and rejects `session`.
  Both return 404 for a missing session and 403 for every non-teacher, including a platform administrator not impersonating the teacher.
  Tests first create `tests/unit/whiteboard-panel.test.tsx` for strict settings schemas, old-route absence, teacher floor rendering/update, and non-teacher/archive behavior.
  Implement the complete live-panel floor client/control and migrate every whiteboard-panel and archive consumer in this same phase, then commit the producer/consumer cutover atomically.
  Before removal, record repository and `main`-history evidence that no deployed caller used the feature-branch-only old route.
- Tighten canvas creation to Decision 14's represented-teacher-or-currently-present matrix with no independent admin/impersonation bypass, and keep create plus every other canvas mutation behind the shared lifecycle lock.
- Refactor explicit `SessionHandler.EndSession` to authorize the teacher, acquire and commit the lease/list transaction before HTTP, retry the same freeze token, strictly validate the bundle, run the confirmed transaction or the separate degraded transaction, emit/schedule only after commit, and expose durable `whiteboardServerArchiveComplete` plus the stable warning.
- Update session-create and scheduled-start handlers to perform the same scheduled-session completion and event work for every replaced session, best-effort complete cleared tokens, and return `replacedSessions` without making replacement depend on Hocuspocus.
- Remove the canvas-list duplicate session lookup and preserve the existing canvas-owner foreign-key retention behavior unless repository user-deletion evidence requires otherwise.
- Update `.env.example`, `docs/setup.md`, and `docs/project-structure.md` in the same phase for the server-only internal URL, listener binding, default/override port, distinct secret, and loopback-or-verified-HTTPS rules.
- Focused GREEN commands:
  `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./internal/realtime ./internal/handlers -run 'Test(CanvasControl|EndSession|CreateSessionReplacement|ScheduleStartReplacement|FreezeAuth|RealtimeAuthLifecycle|CanvasSettings|CanvasCreate)' -count=1 -timeout 120s`,
  `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./... -count=1 -timeout 120s`, `go vet ./internal/realtime ./internal/handlers` from `platform/`,
  and `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bunx --bun vitest run tests/unit/whiteboard-panel.test.tsx tests/unit/whiteboard-archive.test.tsx` plus `bunx tsc --noEmit` from the repository root.

### Phase 10 — Hocuspocus fence, admission, capture, and control listener *(Terra backend; tests by Terra)*

- Move the new lifecycle machinery into focused `server/canvas-lifecycle.ts`; `server/hocuspocus.ts` wires its hooks and starts a separate authenticated control listener.
  Preserve the shared websocket listener's 100 MiB compatibility cap; enforce the exact 1,048,576-byte decoded-update limit only after parsing a `canvas:` message.
- Validate both control directions with the same transport rule and replace the current plain-HTTP `localhost` default with an explicit numeric loopback address; document that non-loopback endpoints require verified HTTPS.
  Hocuspocus must fail startup if the control listener cannot bind or the explicit secret or TLS/port override is invalid; an omitted port uses Spec 013's 4001 default.
- Implement the exact signed advisory-key derivation fixture in `server/canvas-lifecycle.ts` and prove its five boundary UUID vectors match Go and PostgreSQL, even though Node reaches the database lock only through the Go callback.
- Implement the spec's per-session freeze/unfreeze/complete serializer, database-validated monotonic lease deadline, immutable cached bundles, reference-counted deadline-owned response writers, token/entry identity cleanup, 256 MiB capture ledger, and shared admission turnstile with an eight-operation pre-allocation cap.
- Implement the per-document shadow Y.Doc admission contract, 128 MiB resident ledger, owned 500-millisecond authorization fetch, local-fence rechecks after every yield, pending-struct rejection, synchronous update-listener commit, actual-state fallback, generation-keyed load watchdog, and destroy-only irreversible cleanup.
  Freeze installs the fence and acquires the same turnstile before save mutex and capture.
- Run the current internal authorization check in the per-connection admission hook, including connections joining an already-loaded document; do not rely on `onLoadDocument` for admission.
  A stale writable claim that now resolves ended/viewer becomes permanently read-only for that connection, a temporary freeze returns the retryable outcome without permanent downgrade, and every established canvas socket closes at the JWT expiry under a controlled Bun timer.
- Update the shared Yjs provider with an explicit retry classification: `session_freezing` uses a reset-on-each-rejection recovery horizon of at least 20 seconds with a two-second per-attempt ceiling.
  An uncategorized close while believed live also stays at the two-second ceiling for the first 20 seconds, then grows with jitter toward a 30-second ceiling while retrying indefinitely until a terminal condition; no retryable freeze or ordinary outage becomes a manual-reload terminal state.
  Add compatibility tests proving attempt/session documents retain their existing behavior.
- Add the three strict control endpoints on the separate listener: freeze, unfreeze, complete.
  Capture every loaded authoritative canvas under its save mutex, reserve before encode, yield between documents, cache before 200, stream with backpressure, close connections only after complete capture, and never write PostgreSQL from the freeze path.
- Extend `server/hocuspocus.canvas.test.ts` and new `server/canvas-lifecycle.test.ts` with the complete Spec 013 Hocuspocus matrix, including installed-hook `MessageReceiver.apply` cases, partial-overlap Yjs updates, every pre-handoff rejection, saturated admission, half-open writers, same-token recovery, complete races, failed/unregistered load, reconnect-aborted unload, actual destroy, empty canvas lists, expiry, and all non-canvas namespace compatibility.
- Narrow the broadened blank-attempt load log in `server/hocuspocus.ts` so expected missing state does not masquerade as an operational failure while real non-canvas load errors remain visible.
- GREEN commands under Bun:
  `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun test server/hocuspocus.canvas.test.ts server/canvas-lifecycle.test.ts`,
  `bunx --bun tsc --noEmit`, and `git diff --check`.
  Update `package.json` so the normal `bun run test`/`scripts/ci-local.sh` path executes `server/canvas-lifecycle.test.ts` rather than relying on the focused command alone.

### Phase 11 — Teacher controls, durable warnings, and legacy-writer removal *(Sonnet frontend; tests by Terra)*

- Extend the existing `tests/unit/whiteboard-panel.test.tsx` and binding suites for live create/list-by-role, owner-versus-viewer controls, confirmation before irreversible visibility raises, image/paste/drop rejection, identical serialized-scene skipping, 100-millisecond trailing scene writes, remote-update echo suppression, `viewBackgroundColor` plus the exact durable allowlist, and stable local viewport/zoom/selection/tool/collaborator/view-mode state.
- Extend the Phase-9 live panel with the remaining settled error/confirmation and owner-versus-viewer UX; keep all archive boards read-only.
- Update the teacher dashboard end flow to decode the durable completion/warning response, render a failed-end error without navigating, and write a one-shot session warning before a successful archive redirect when completion is false.
  The archive page calls `GET /api/sessions/{id}/canvas-settings` first: a 200 durable value is authoritative and consumes the fallback, while a network/non-200 result retains it; an incomplete warning displays at most once per visit.
- Update `src/components/teacher/start-session-button.tsx` to decode the successful top-level session plus `replacedSessions`, show the exact prior-session archive warning once before navigation, and preserve the existing 422 unlinked-topic confirmation flow.
- Render a scheduled-session list from the existing teacher class page; its start action calls `POST /api/schedule/{id}/start`, decodes the same `replacedSessions` contract, and shows the archive warning before navigation.
- Fix the neutral session page to redirect only an existing ended-session former participant to the archive while preserving 404 for a missing session; correct the stale `student-session.tsx` effect dependencies.
- Remove the shadow `PATCH` export from `src/app/api/sessions/[id]/route.ts`, remove the live `endSession` and dead `createSession` helpers from `src/lib/sessions.ts`, update `tests/unit/shadow-routes.test.ts` and `TODO.md`, and add a production-source scan proving no TypeScript session-status writer remains.
- GREEN commands:
  `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bunx --bun vitest run tests/unit/whiteboard-panel.test.tsx tests/unit/start-session-button.test.tsx tests/unit/scheduled-session-list.test.tsx tests/unit/whiteboard-archive.test.tsx tests/unit/sessions-room-page.test.tsx tests/unit/shadow-routes.test.ts tests/unit/excalidraw-yjs.test.ts tests/unit/use-yjs-provider.test.ts`,
  the source-local whiteboard Vitest configuration, `bun run lint`, and `bunx tsc --noEmit`.

### Phase 12 — Integration tests (NAMED: lifecycle + realtime + API + persistence) *(Terra tests)*

- **Fixtures:** `_test`-guarded host, present/left/invited participants, outsider, other-org user, live class and class-less sessions, scheduled session, 0/1/50 loaded canvases, controlled Hocuspocus server, fake clock, half-open control transport, and two concurrent database connections.
- **Fast integration acceptance:** implement the exhaustive Spec 013 matrix, including the exact named tests `TestEndSession_ConfirmedBundlePersistsBeforeCommit`, `TestEndSession_DegradedWhenHocuspocusUnavailableWarnsTeacher`, `TestEndSession_TransportLossRecoversSameTokenBundle`, `TestEndSession_BatchFailureRollsBackTrueAndSnapshots`, `TestEndSession_DatabaseFailureLeavesLiveClearsLeaseAndEmitsNoEvent`, `TestEndSession_LeaseExpiryUsesSeparateDegradedTransaction`, `TestEndSession_OverlappingRequestsCannotClearForeignLease`, `TestEndSession_LateFreezeResponseCannotReinstallLease`, `TestEndSession_DifferentExpiredTokenEndsDegradedClearsLeaseWithoutReusingFreeze`, `TestCreateSession_ReplacementEndsIncompleteAndWarns`, `TestStartScheduledSession_ReplacementCompletesScheduleAndWarns`, `TestReplacementRacePreservesExplicitConfirmedResult`, `TestFreezeAuth_SharedLockAndExactToken`, `TestRealtimeAuth_BlocksBehindFreezeLifecycleLock`, and `TestCanvasMutations_BlockBehindEndLifecycleLock`.
- **API integration acceptance:** add named happy/auth/error/cross-user tests for freeze-auth; the complete teacher/present/invited/left/public-outsider/platform-admin/impersonator/ended/cap-race canvas-creation matrix; all four active-freeze canvas mutations returning `409 session_end_in_progress`; concurrent/stale end conflicts; strict canvas-settings GET/PATCH schemas, missing-session 404s, non-teacher 403s, ended-teacher true/false/null reads, and old `/settings` absence.
  Include cross-language Go/TypeScript/PostgreSQL advisory-key fixture vectors and the exact Phase-4 mint/store test names already listed above.
- **Realtime acceptance:** the Bun suite must include `freeze complete serializes terminal cleanup`, `half-open writer permits same-token retry`, `admission cap rejects ninth before allocation`, `denied auth releases turnstile and accounting`, `partial-overlap update commits exact state`, `failed unregistered load releases reservation`, `reconnect-aborted unload retains instrumentation`, `destroy releases generation exactly once`, `jwt expiry closes established reader`, and exact fake-clock scheduling for resettable 20-second freeze recovery plus the uncategorized 20-second fast window and jittered 30-second long tail.
- **Frontend warning acceptance:** name tests for explicit-end error/no-navigation, session fallback write-before-redirect, durable 200 override/consume, non-200 retention, once-per-visit display, ordinary-create replacement warning, scheduled-start replacement warning, and missing-session 404 preservation.
- **Live-stack acceptance:** create `e2e/session-whiteboard.spec.ts` and minimal helpers/fixtures for teacher create → floor/visibility raise → participant view → public-outsider creation denial → scene sync → explicit end → read-only archive.
  Exercise incomplete-archive warning through a named Go control-client failure injection that startup accepts only when both the parsed and live database names end in `_test` and an explicit E2E-only flag is set; the seam is otherwise rejected and never kills or disrupts a service.
  Before the stack can start, pause for the user to provision `HOCUSPOCUS_CONTROL_SECRET` and, only when default 4001 is occupied, a collision-free `HOCUSPOCUS_CONTROL_PORT` override in the safeguarded local environment; do not read or edit `.env`.
  Run only against that separately started Bridge stack with explicit `E2E_BASE_URL`; without those prerequisites record E2E as `UNVERIFIED`, do not claim the phase or merge gate complete, and pause before shipping.
- Happy, auth-denial, malformed/timeout, cross-user, cross-session, and cross-org paths are mandatory; no broad green count substitutes for the named tests.

### Phase 13 — Documentation, cross-phase verification, and shipping evidence *(orchestrator)*

- Update `docs/api.md`, `docs/architecture/decisions.md`, `docs/testing.md`, `docs/setup.md`, `docs/project-structure.md`, `.env.example`, and `README.md` for status-first lifecycle semantics, confirmed/degraded guarantees, control transport/config, replacement warnings, admission bounds, single-Hocuspocus limitation, no independent administrator/impersonator bypass for private canvases, and operator behavior; verify the Phase-9 config docs remain synchronized with the final implementation.
- Audit Spec 013 requirement-to-test coverage, scan the frozen scope and production session-status writers, and reconcile the plan's historical `[OPEN]` code-review findings only with verified implementation evidence.
- Run focused TypeScript/Bun and Go suites first, then the complete `bash scripts/ci-local.sh` on the exact commit intended for review.
  The merge gate remains incomplete until the full command—including explicitly pinned E2E—is green and its attestation names that commit.
- Run the Tier-A code-review gate against the consolidated exact commit, resolve every `[OPEN]` finding to reviewer confirmation, write the post-execution report, update `TODO.md`, and run `bash scripts/pre-merge-guard.sh` before creating the PR.
  Create the draft PR, rerun `bash scripts/pre-merge-guard.sh --pr <number>` against its simulated merge state, rerun any invalidated exact-commit evidence, and squash-merge without `--admin` only after all local evidence is current.

## Risks

| Risk | Mitigation |
|------|------------|
| **Realtime auth is a hard safeguard** — a permissive mint leaks a private/cross-org canvas. | Enforce at mint (server), reuse the plan-090 membership guard (Decision 9), test the full owner/host/member/non-member/cross-org matrix + ended-session archive. 4-way review must scrutinize `realtime_token.go` + `hocuspocus.ts`. |
| **Read-only not actually enforced** (the whole boundary). | Phase 2 establishes permanent viewer/ended read-only enforcement; Phases 8–10 supersede temporary-freeze handling with retryable `session_freezing`, a confirmed fence, and honest degraded finite fan-in. Tests distinguish those paths; `viewModeEnabled` is UX only. |
| Floor invariant race (create vs raise). | `SELECT … FOR UPDATE` on the session row in both; `GREATEST(requested, floor)`. Concurrency test required. |
| Owner over-shares by accident (loosen-only, Decision 7). | Accepted for MVP; a host-moderation follow-up can add tighten/override. UI shows a confirm on loosening to `participants`/`session`. |
| **Removed owner keeps write** (Decision 1 owner-durability + Decision 6 no per-canvas moderation) — a kicked participant who owns a canvas can still mint a write token while the session is live; the teacher has no per-canvas remedy. | **Accepted MVP risk, explicitly.** The host lever is the floor, not per-canvas control. Host-moderation follow-up adds owner-eviction / canvas takedown. Stated in `decisions.md`. |
| A member who **leaves** keeps a `participants`/`session` read token ~25 min (Decision 7). | Accepted, TTL-bounded; matches existing session/attempt-doc behavior. Not a tightening problem. |
| `y-excalidraw` unmaintained / no read-only / React 19. | Phase-1b vetting gate + thin-custom fallback. Phase 3 blocked until it passes. |
| Persistence churns Postgres per stroke. | Debounced Hocuspocus snapshot, matching attempt/session doc persistence. |
| Excalidraw bundle on the session route. | Dynamic-import; load only when the whiteboard opens. |
| A test-only cost-4 fixture accidentally replaces real registration coverage. | Restrict the helper to audited setup inputs, preserve producer/auth registration tests at production cost, compare persisted shape against one real registration, and guard against future direct handler-test copies. |
| The local-gate rewrite weakens database or LLM-billing safeguards. | Validate parsed and live database names before every runner, pin both database variables, preserve all five empty provider keys, and enforce those properties in executable governance self-tests. |
| Activating formerly skipped Go integration tiers mutates the wrong database or overruns the package timeout. | Guard every shared opener before mutation, remove the contract cleanup fallback, inventory DDL/DML consumers, and require isolated plus full-run duration evidence on the validated test database. |
| A final snapshot is reported complete while a mutation applies after capture or bundle persistence partially fails. | Durable lease plus shared/exclusive advisory locks, the shared admission turnstile, conditional true-first transaction with full rollback, and a separate durable false degraded transaction. |
| A half-open control request, response writer, mutation authorization, or cancelled waiter strands a fence or leaks memory. | Owned deadlines and abort settlement, eight-admission pre-allocation cap, reader references, token/generation identity cleanup, and controlled half-open/saturation tests. |
| Replacement session creation silently ends a whiteboard session without archive status or scheduled cleanup. | Class guard plus sorted lifecycle locks, durable incomplete result, same scheduled completion/events, returned replacement metadata, and teacher warning without a realtime dependency. |
| Canvas-specific transport hardening breaks existing attempt/chapter/session documents. | Preserve the shared 100 MiB websocket cap and enforce the 1 MiB limit only after parsing a canvas mutation; test every supported namespace. |
| Governance edits weaken or ambiguously cap review gates. | Executable text checks require the exact Sol + Fable design roster, exact-commit binding, uncapped consensus for all gates, complete canonical mirroring, and no contradictory numeric cap. |

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

### Round 2 — 4-way (2026-08-07): **all three CHANGES REQUESTED, but R1 blockers B1–B3 confirmed RESOLVED by all.** New (convergent) findings:

1. `[BLOCKER]` `[codex][opus]` **Ended-session write window.** `readOnly` is set at mint (25-min TTL); an owner holding a live *write* token whose session then ends keeps writing until expiry, and `onStoreDocument` has no status gate → violates Decision 8. **Fix:** gate writes on `status=ended` in `onStoreDocument` (server, durable), not only at mint.
2. `[BLOCKER]` `[glm]` **Ended mutating-endpoint guard.** POST/PATCH/DELETE have no `status=ended` check — an owner could loosen a canvas after end, retroactively widening the archive. **Fix:** `status=ended` guard on every mutating canvas endpoint.
3. `[BLOCKER]` `[codex][opus]` **`CanAccessSession` admits public class-less outsiders** — reusing it (Decision 9) grants a `session`-visibility canvas to any authenticated user (any org) on a live `public && class_id IS NULL` session, contradicting cross-org denial. **This is a trust-model fork (user):** is a public session's board public-to-all, or restricted to actual participants? Likely fix: canvas read-membership = an actual `session_participants` row (`present`), not the public-open-join clause.
4. `[BLOCKER/CONCERN]` `[codex][opus][glm]` **Ended-archive participant status filter unspecified** — `session_participants` keeps `invited`/`left` rows; a bare EXISTS leaks to no-shows / briefly-present users. **Fix:** name the exact status set (proposed: `present`, and decide whether `left` retains archive read).
5. `[BLOCKER]` `[glm]` **set-visibility-vs-raise-floor race** — only create + set-floor take the session lock; set-visibility reads floor unlocked. **Fix:** set-visibility also locks the session row.
6. `[BLOCKER]` `[glm]` **Migration number.** `drizzle/0027_session_visibility.sql` is already taken (plan 090). **Fix:** pin `0028`, run `check-migration-uniqueness.sh`.
7. `[CONCERN]` `[opus]` Decision 7 overclaims "no revocation problem" — a member who *leaves* while holding a `session` token still reads until TTL (~25 min). Reword to scope it (accepted MVP, TTL-bounded), don't deny it.
8. `[CONCERN]` `[opus]` Owner durability strands the teacher — a removed owner keeps write while live. State as accepted MVP risk + host-moderation follow-up.
9. `[NIT]` Decision 2 still says "outlives"; reconcile with archive wording. Add server read-only test file to scope. pgEnum append-only future hazard note. Disable (not hide) the current level in the loosen UI. Add ended non-member/cross-org tests.

**Verdict: CHANGES REQUESTED ×3 (round 2).** Resolutions folded into **Revision 3**:
- Finding 1 (ended write window) → `onStoreDocument` rejects writes when `ended` (Architecture, Phase 2).
- Finding 2 (mutating-endpoint ended guard) → all canvas mutations 409 when `ended` (Phase 1b).
- Finding 3 (public-outsider trust-model fork) → **user added a 4th level `participants`** (Decisions 4, 9, 11): `participants` = strict `present`-joiners; `session` = intentionally as-public-as-the-session. Owner chooses.
- Finding 4 (archive status filter) → `present`/`left`, not `invited` (Decision 8).
- Finding 5 (set-visibility race) → set-visibility now locks the session row (Architecture, Phase 1a).
- Finding 6 (migration 0027 taken) → pin `0028` + run the guard (Phase 1a).
- Findings 7–9 → Decision 7 reworded; owner-durability + leave-revocation as accepted MVP risks (Risks); Decision 2 reconciled; server test file added to scope; enum append-only note (Phase 5); loosen-UI wording.

### Round 3 — 4-way (2026-08-07): **GLM APPROVE WITH NITS; Codex + Opus CHANGES REQUESTED (mechanical, no forks).** All R2 items confirmed resolved. R3 fixes folded into Revision 4:
- Host floor→`session` footgun (Opus B1) → **floor capped at `participants`** (Decision 6, Architecture); host can never force students' boards world-public.
- `onStoreDocument` insufficient to stop post-end live relay (all 3) → **force-close/flip canvas connections on the `ended` transition** + storage-drop backstop + endpoint 409s + ended mint read-only; explicit MVP-note fallback if force-close is impractical in Hocuspocus 3.4.4 (Architecture).
- Mint "max-eligible-level ≥ visibility" shorthand was wrong (Codex) → **admit-tier ≤ visibility** compare specified (Architecture).
- `list-visible` must special-case ended sessions (GLM) → stated (Phase 1a).
- Nits: ended teacher-read + host-reads-participants/session + floor-rejects-session tests added (Phase 4); DELETE purges the Yjs doc; floor-lowering allowed; enum append-only wording; invited-live-vs-archive asymmetry doc (Phase 5 / api.md).

### Round 4 — Codex confirmation (2026-08-07): **CHANGES REQUESTED.** Independent Claude confirmation was unavailable due weekly quota and the user directed the run to skip all Claude models until Sunday 16:00; GLM's Round-3 approval remains standing. Codex confirmed the host-floor and admit-tier fixes, and raised two blockers:
- Transition-time relay defense was architecture prose only: no implementable phase path prevented a pre-end writable connection from relaying after end. **Resolution in Revision 5:** replace the racy force-close/fallback sketch with Hocuspocus 3.4.4's exact `beforeHandleMessage` boundary. The existing internal Go recheck now returns current `readOnly`; a mutation-bearing canvas frame flips the connection before `MessageReceiver` can apply/broadcast it, failing closed on errors. Phase 2 and its observer/no-relay test spell out the producer/consumer path.
- Ended-session list-visible rules lacked named handler coverage. **Resolution in Revision 5:** add `TestCanvases_ListEndedArchive_ByRole` with visibility thresholds and owner/teacher/present/left/invitee/outsider cases.

### Round 5 — Codex confirmation (2026-08-07): **APPROVE.** Codex verified both Round-4 blockers are resolved and checked the installed Hocuspocus 3.4.4 implementation: `beforeHandleMessage` is awaited before `MessageReceiver.apply`, `Connection.readOnly` is mutable, and sync step-2/update frames are rejected when read-only. The named ended-session GET/list test covers owner, teacher, present/left participants, invitee, outsider, and visibility thresholds. Static plan review only; no tests were run. With GLM's standing Round-3 approval and the user's temporary direction to skip both Claude reviewers until Sunday 16:00, the plan-review gate passes for this run.

### Archive-route addendum — Round 1 (2026-08-08): **CHANGES REQUESTED ×3.** The user selected the dedicated archive route and authorized its first scope expansion. Independent Codex reviewers found that the route needed to force read-only behavior when opened while live, and that former participants and hosts needed durable entry links beyond the current-session SSE redirect. GLM's output was recovered from its completed OpenCode session export after its 128k limit was confirmed: it found that the archive bypass must state the teacher's total ended-session `CanAccessSession` denial too, and that live public `session` viewers must explicitly lose archive access under Decision 8. Resolutions folded into the addendum:
- Archive always sets Excalidraw view mode and suppresses local Yjs writes, regardless of current session status.
- Successful teacher end and student `session_ended` redirect to the archive; neutral former-participant fallback redirects there instead of calling live-session access again.
- User authorized scope expansion for every existing teacher/student ended-session history row, each linking to the archive.
- Route-level test obligations now name direct-live view-only behavior, no live page API calls, no empty-list metadata/token leak, redirects, and all archive links.
- Decisions 11–12 now distinguish public live access from the former-participant-only archive and name `ListVisibleCanvases`/canvas mint as the explicit ended-session authorization branches.

### Archive-route addendum — Round 2 (2026-08-08): **CHANGES REQUESTED (Codex quality).** The scope and navigation fixes were accepted for re-review, but the quality pass required three precise test/behavior additions: bind the archive write prohibition to the custom binding's actual `onChange` path (not only Excalidraw view mode), distinguish initial list fetch/no-token from selected-board token mint, and gate the teacher's archive redirect on a successful end response. These are folded into the Phase-3 frontend test contract above; the fresh contract verdict is pending.

### Archive-route addendum — Round 3 (2026-08-08): **CHANGES REQUESTED (Codex contract).** The revised behavior and assertions were sufficient, but the existing tests that must change were outside File scope. The user authorized the explicitly named test files above; fresh confirmation is pending.

### Archive-route addendum — Round 4 (2026-08-08): **CHANGES REQUESTED (GLM recovery).** GLM's recovered review required the completed public-live-to-archive transition to be explicit and testable. Decision 11 now limits public admission to live sessions, Decision 12 names the total ended-session live-guard denial (including the teacher), and Phase 4 now requires the public viewer denial regression. Fresh confirmation is pending.

### Archive-route addendum — Round 5 (2026-08-08): **CHANGES REQUESTED (Codex quality).** The archive behavior was approved by the contract reviewer, but a full fixed-scope audit found three earlier branch artifacts omitted from File scope. The user authorized their exact paths; fresh confirmation is pending.

### Archive-route addendum — Round 6 (2026-08-08): **APPROVE.** The scope confirmation approved all previously omitted branch artifacts. A fresh Codex contract pass approved forced archive no-write behavior, token sequencing, success-only end redirect, and public-live versus archive semantics. GLM 5.2 approved the resolved teacher total-denial rationale and the named public-viewer denial tests. The user continues to skip Claude reviewers until Sunday 16:00, so no Claude slot was dispatched. The archive addendum gate is therefore clear.

### Schema-probe verification addendum — Round 7 (2026-08-09): **CHANGES REQUESTED (Claude ×2, GLM, Codex).**
The user authorized `migrations.go` after the plan-wide Go suite proved that migration 0028 had not advanced the latest schema probe.
All reviewers confirmed that bump was necessary, but the independent passes found it insufficient: `schema_probe_integration_test.go` still destructively asserted `books` sentinels; 0028 has no named constraint; its `canvas_visibility` enum and cross-table `sessions.canvas_floor` alteration were not representable; and Phase 5 did not name a pinned-DB probe run.
The user authorized the complete scope expansion.
Decision 13 and Phase 5 now define a multi-object end-state probe, exact enum/altered-column parity, preserved generic constraint coverage, retargeted integration tests, and updated operator guidance.
Fresh confirmation is pending.

### Schema-probe verification addendum — Round 8 (2026-08-10): **APPROVE (Claude ×2, GLM, Codex).**
All reviewers confirmed Revision `d2b224c` resolves the Round-7 scope and contract blockers.
The nine primary-table columns, two indexes, empty named-constraint set, `sessions.canvas_floor`, ordered `canvas_visibility` values, injected generic constraint regression, pinned `bridge_test` command, and operator guidance are now fully specified within File scope.
No reviewer reported a remaining blocker, so the addendum gate is clear.

### Local-gate addendum — Round 9 (2026-08-10): **CHANGES REQUESTED (independent Claude, GLM, Codex); Claude self-review APPROVE with corrections.**
The independent review found that Phase 6 omitted `problems_integration_test.go`, even though it defines both `newProblemFixture` and the shared handler `integrationDB`; the globbed scope also defeated the fixed-scope audit.
Fresh repository counts corrected the load model to at least 1,090 cost-10 hashes from the four dominant fixtures, and the plan now enumerates all 22 direct-call-site files plus the two new regressions.
The cost-4 fixture hash, production-shape parity test, valid input boundary, exact named tests, fail-closed URL/live-database checks, and 60-second handler-package target resolve the fixture correctness and headroom findings.
The Zod boundary now names the locked toolchain, requires an identity regression and the complete root Vitest suite, and records `--fast` as phase-local rather than merge evidence.
GLM's database-guard blocker remained open for confirmation rather than author resolution.
The dispatch concern is rejected under the user's direct instruction, “for tests coding, let's use gpt terra,” which overrides the repository default for this run.

### Local-gate addendum — Round 10 (2026-08-10): **CHANGES REQUESTED (independent Claude and GLM).**
`[FIXED]` The independent confirmation disproved the Round-9 response: `ci-local.sh` checks the whole URL string, so a production pathname with a query value ending `_test` passes, and the Go step inherits that unsafe value.
The user-authorized provisional scope now explicitly includes `scripts/ci-local.sh`, its new decoded-path validator, executable guard tests, and `docs/testing.md`; implementation remains blocked until the governance revision passes this plan gate.
`[FIXED]` The validator checks the decoded non-empty pathname and allowed PostgreSQL schemes, and the gate passes the one validated URL as both database variables to Vitest and Go.
`[FIXED]` Zod resolution adds root deduplication, an identity regression, and the complete root suite; repository inspection found no current cross-package Zod class-identity consumer.
`[FIXED]` The fixture bypass now records why a production cost knob is rejected, audits and parity-tests the current producer contract, restricts helper inputs, and treats call-site multiplication only as an estimate while requiring measured runtime headroom.
`[FIXED]` The handler URL guard now names exact URL/path decoding and fail-closed query-error behavior; review findings remain explicit until reviewer confirmation.
`[FIXED]` The gate validator now performs its own live `current_database()` check before any mutation and pins the validated URL into both test runtimes, closing the query-string-decoy and proxy/misrouting path across all packages.
`[FIXED]` The provisional marker is adjacent to the added File-scope entries; the fixture signature fails through `testing.T`, all unchanged shared-helper consumers run in the package suite, and the existing teacher redirect regression is named exactly.
Fresh confirmation is pending from the flagging reviewers.

### Local-gate addendum — Round 11 (2026-08-10): **CHANGES REQUESTED (Claude self-review; independent Claude and GLM pending).**
`[FIXED]` The first revised resolver would have ignored an unsafe ambient `DATABASE_URL` whenever `TEST_DATABASE_URL` was set.
The new contract retains an independent fail-closed decoded-path plus live-database check for any ambient URL, validates the resolved gate URL separately, and pins both variables into Vitest, Go, and E2E.
`[FIXED]` The validator now names its existing `postgres` client, one-connection/five-second behavior, credential-safe diagnostics, and gate-only live-check requirement; self-tests include both query-suffix and `dbname` decoys.
`[FIXED]` The root and source-local Vitest configurations are distinguished explicitly, the two duplicate File-scope paths are removed, and the review ledger uses canonical status tags.
Fresh confirmation is pending from all external reviewers because the governance revision is material.

### Local-gate addendum — Round 12 (2026-08-10): **CHANGES REQUESTED (independent Claude; GLM pending).**
`[FIXED]` Pinning the gate URL intentionally activates previously skipped Go integration tiers; the plan now inventories their read/write behavior, preserves the schema-probe DDL's existing `_test` guard, and adds live guards to the shared store opener plus the contract cleanup before any mutation.
`[FIXED]` The validated URL never crosses process stdout: Bash owns and asserts the non-empty variable, while the validator receives it through the environment and returns status only.
`[FIXED]` Both independent Vitest configurations receive the Zod boundary and both run in GREEN evidence.
`[FIXED]` The fixture helper now explicitly wraps its two inserts in a transaction, dedicated producer tests retain duplicate/normalization behavior, and a guard prevents new handler setup from copying direct cost-10 registration.
`[FIXED]` Full-run handler duration, not only isolated duration, must retain at least 60 seconds of timeout headroom.
`[FIXED]` The new validator is declared as governance in `AGENTS.md`, and no new package is required because the repository already depends on the Node-compatible `postgres` client.
Fresh confirmation is pending from the complete roster.

### Local-gate addendum — Round 13 (2026-08-10): **Claude self-review and GLM APPROVE; independent Claude pending.**
`[FIXED]` The Vitest rewrite must preserve and executable guard tests must enforce all five empty provider-key exports, so the gate cannot bill live LLM calls through Bun's `.env` reload.
`[FIXED]` The store package receives the same 60-second isolated/full-run headroom target; its shared opener, the separately guarded canvas opener, harmless config tests, schema DDL tests, and contract cleanup are all inventoried.
`[FIXED]` The validator runs under system Node 18 with an explicit existing-`postgres` import smoke check, status-only handoff, bounded connection, and credential-safe diagnostics; its self-tests and direct-call-site allowlist have an exact owner in `scripts/tests/test-guards.sh`.
`[FIXED]` Both the validator and its executable guard proofs become named governance in `AGENTS.md`, whose stale LLM-isolation description is corrected in the same reviewed change.
`[FIXED]` The standing file authorization explicitly does not waive any hard safeguard.
Fresh independent confirmation is pending; no Phase-6 implementation is authorized yet.

### Local-gate addendum — Round 14 (2026-08-10): **APPROVE WITH NITS — consensus reached.**

`[claude-self]` **APPROVE WITH NITS.** No blockers; the exact call-site census, executable database-safety contract, Vitest boundary, governance scope, and five-key billing isolation were confirmed.
`[codex]` **APPROVE WITH NITS.** No blockers; corrected the store-opener inventory to 22 direct/transitive consumers and clarified that parse-only covers positive and negative parser self-tests only.
`[opus]` **APPROVE WITH NITS.** No blockers; removed duplicate scope entries, added the gate script's own stale LLM-isolation header to the correction, and promoted Phase-6 safety concerns into `## Risks`.
`[glm]` **APPROVE WITH NITS.** No blockers; independently confirmed the database and governance vulnerabilities plus the proposed fail-closed controls.
GLM's reported 30-call census is rejected: `rg -n '\.RegisterUser\(' platform/internal/handlers/*_test.go` returns 31 distinct source lines across the 22 enumerated files.
Its Zod RED concern is also closed by the exact current gate evidence recorded above; regardless, the new interop regression and complete root suite are the acceptance boundary rather than an assumed diagnosis.
All accepted nits are incorporated in this revision, every Tier-A reviewer has no open blocker, and Phase 6 is authorized for implementation.

### Spec 013 remediation revision — plan gate pending (2026-08-11)

- The user approved widening the frozen scope after Spec 013 reached exact-commit consensus at `aa34784e1e51596bf1b6779176a86187fe3306ab` (`[sol]` APPROVE; `[fable]` APPROVE WITH NITS).
- Phases 7–13 translate that approved design into governance, durable lifecycle storage, Go control/end ownership, Hocuspocus admission/capture, teacher UX, named integration tests, and final verification.
- This revision is Tier A because it touches stores, Hocuspocus, migration state, tests, and governance.
- No Phase-7+ implementation is authorized until the current four-slot Tier-A plan roster returns APPROVE or APPROVE WITH NITS on the same committed revision with no open blocker.
- The current governance's three-round cap remains authoritative for this plan gate; the uncapped consensus rule becomes effective only after the approved Phase-7 governance change ships.

### Spec 013 remediation plan gate — Round 1 — commit `9e38c2f56e7790c275b4821d6903e01928178e6f`

- **Reviewers:** Claude self-review (`claude-opus-5`), Codex (`gpt-5.6-sol`, high), fresh independent Claude (`claude-opus-5`), and OpenCode GLM 5.2.
- **Verdicts:** `[claude-self]` CHANGES REQUESTED; `[codex]` CHANGES REQUESTED; `[opus]` CHANGES REQUESTED; `[glm]` APPROVE WITH NITS.
- `[ADDRESSED]` `[claude-self][opus][glm]` Assign the settled teacher-or-present creator matrix, reject admin/impersonator bypass, and name its complete API/E2E tests.
- `[ADDRESSED]` `[claude-self][codex][opus]` Define the producer-and-consumer atomic migration from the feature-branch `/settings` PATCH to strict dedicated canvas-settings GET/PATCH routes, including old-route absence and exact 403/404/schema contracts.
- `[ADDRESSED]` `[claude-self][codex][opus][glm]` Add a decoded/live `_test`-only reconciliation step for the already-applied old 0028 state, record unshipped-through-main evidence, and pin both database URLs in every focused command.
- `[ADDRESSED]` `[codex][opus]` Put ordinary realtime mutation authorization under the shared lifecycle lock and name paused-auth/fan-in regressions.
- `[ADDRESSED]` `[claude-self][codex][glm]` Implement the explicit-end durable-warning producer/consumer protocol, failed-end UX, scheduled-start warning consumer, browser fallback consumption rules, and once-per-visit tests.
- `[ADDRESSED]` `[opus]` Scope and implement the shared Yjs provider's retryable-freeze and long-tail reconnect contract without changing attempt/session behavior.
- `[ADDRESSED]` `[claude-self][opus]` Add the new lifecycle Bun suite to the normal local gate, make the E2E failure seam `_test`-only and process-safe, and make control-secret/port provisioning an explicit hard-safeguard pause before live-stack verification.
- `[ADDRESSED]` `[claude-self][codex][opus][glm]` Record the user's prior all-review uncapped-consensus direction, mirror it consistently through canonical governance, replace the obsolete round-cap safeguard, define unavailable-reviewer behavior, and mirror the explicit Terra test override.
- `[ADDRESSED]` `[claude-self][codex][opus]` Own the remaining Review-1 cleanup: missing-session 404, stale student effect dependencies, duplicate canvas-list lookup, owner-FK retention evidence, blank-attempt log, and precise settings/admin behavior.
- `[ADDRESSED]` `[codex][opus][glm]` Expand the named lifecycle, freeze-auth, mutation, creator, settings, frontend-warning, cross-language key, and public-outsider E2E matrices so a broad green count cannot substitute for the spec contract.
- `[ADDRESSED]` `[codex][claude-self]` Correct shipping order to run the non-PR guard before PR creation and the simulated-merge guard afterward; require the full pinned E2E/local attestation rather than treating `UNVERIFIED` as merge evidence.
- `[ADDRESSED]` `[claude-self][opus]` Remove the nonexistent route-file hedge, distinguish new tests from extensions, validate both control directions, use numeric loopback instead of `localhost`, and require explicit collision-free control configuration.

All Round-1 responses are author-side `[ADDRESSED]`, not self-certified `[FIXED]`.
The three reviewers that requested changes must confirm the next exact substantive commit; GLM has no blocker and is not redispatched under the current gate's reviewer-response rule.

### Spec 013 remediation plan gate — Round 2 — commit `49770ddc556b9f83caf6ba2247229e708994dd9b`

- **Verdicts:** `[claude-self]` CHANGES REQUESTED; `[codex]` CHANGES REQUESTED; `[opus]` CHANGES REQUESTED; `[glm]` retained APPROVE WITH NITS from Round 1 and was not redispatched.
- `[ADDRESSED]` `[claude-self]` Name and scope `tests/unit/whiteboard-panel.test.tsx`, run it in Phases 9 and 11, and enumerate the complete live-panel/binding contract.
- `[ADDRESSED]` `[codex][claude-self]` Restore the approved production port 4001 default and require only an E2E collision override when that default is occupied.
- `[ADDRESSED]` `[codex]` Rename the different-expired-token test to require degraded end, exact expired-token cleanup, and no freeze-result reuse.
- `[ADDRESSED]` `[codex]` Specify the uncategorized reconnect schedule's first 20-second two-second ceiling, subsequent jittered growth toward 30 seconds, indefinite retry, and fake-clock proof.
- `[ADDRESSED]` `[codex]` Make the Phase-9 producer/consumer route cutover executable with the panel/archive Vitest suite and typecheck in the same phase.
- `[ADDRESSED]` `[opus]` Require every mutation, canvas authorization, and concurrent end path to reject an unexpired lease with stable `409 session_end_in_progress`, including the exact token-ownership rules.
- `[ADDRESSED]` `[claude-self][opus]` Assign the TypeScript advisory-key derivation and five-vector parity proof to `server/canvas-lifecycle.ts`.
- `[ADDRESSED]` `[opus]` Name the per-connection admission hook for current authorization and explicitly forbid relying on `onLoadDocument` for an already-loaded document.
- `[ADDRESSED]` `[claude-self][opus]` Assign `.env.example`, setup/project-structure docs, old-route `main`-history evidence, and the exact control-config contract to implementation phases.
- `[ADDRESSED]` `[opus]` Name identical-scene skipping and `viewBackgroundColor`, pin both database URLs for the focused governance/frontend commands, and move the blank-attempt log correction into Phase 10 with its exact file.
- `[ADDRESSED]` `[claude-self][opus]` Mark the pre-Spec lifecycle and Phase-1b settings/creator prose as historical and superseded.
- `[ADDRESSED]` `[claude-self]` Mirror the complete design-gate rule in `docs/coding-agent.md`, assign the static checks to `scripts/tests/test-guards.sh`, and include both existing E2E helper locations in scope.

All Round-2 responses are author-side `[ADDRESSED]` pending exact-commit confirmation.
Because Round 3 is the current governance checkpoint, any remaining open material finding triggers the hard-safeguard pause; approval by all three flagging reviewers authorizes Phase 7.

### Spec 013 remediation plan gate — Round 3 checkpoint — commit `579974245ba510ea900a85816bd90ef2796d861f`

- **Verdicts:** `[claude-self]` APPROVE WITH NITS; `[codex]` CHANGES REQUESTED; `[opus]` APPROVE WITH NITS; `[glm]` retained APPROVE WITH NITS from Round 1.
- `[ADDRESSED]` `[codex]` The user authorized continuation beyond the checkpoint; Phase 9 now owns creation of `whiteboard-panel.test.tsx` and the complete floor client/control, while Phase 11 only extends the existing suite and panel for unrelated scene/owner UX.
- `[ADDRESSED]` `[claude-self][opus]` HTTP `409 session_end_in_progress` is scoped to the four HTTP mutations/concurrent end; internal-auth freeze rejection maps to retryable `session_freezing`, and authorization locks no second entity.
- `[ADDRESSED]` `[claude-self][opus]` Decision 8, Phase 2, and the risk row now distinguish confirmed fencing from degraded transient fan-in and temporary-freeze handling; the contradictory risk-row numeric checkpoint is removed.
- `[ADDRESSED]` `[opus]` Phase 13 explicitly records the no-independent-administrator/impersonator bypass decision.

The current three-round governance therefore requires a user-visible hard-safeguard pause with Phase 7 still unauthorized.
No implementation, Round-4 plan edit, migration, service, E2E, or remote action followed this checkpoint.

### Spec 013 remediation plan gate — Round 4 response authorized by the user

- The user directed the uncapped consensus loop to continue until every problem is resolved.
- The exact Round-3 blocker and all accepted nits are addressed above.
- Only `[codex]`, the reviewer retaining a material finding, is redispatched against the next exact substantive commit; the two Opus approvals remain authoritative because their nits are incorporated without changing the settled design.

### Spec 013 remediation plan gate — Round 4 consensus — commit `fdaf90af2a6c9500396ced27f2ca0df4be780645`

- `[codex]` **APPROVE.** No residual findings; Phase 9 now owns the complete settings producer/consumer cutover and Phase 11 only extends the existing panel/test for separate UX.
- `[claude-self]` and `[opus]` retain **APPROVE WITH NITS** from Round 3; every accepted nit is incorporated in the approved substantive commit.
- `[glm]` retains **APPROVE WITH NITS** from Round 1; it had no blocker and no later response invalidated its findings.
- The Round-3 `[OPEN]` settings-cutover finding is now `[FIXED]` by reviewer confirmation.
- All four Tier-A roster slots approve the same final substantive plan state with no open material finding; Phase 7 is authorized.

## Code Review

### Phase 6 task reviews (2026-08-10)

- The database-validator/governance task reached spec and quality consensus after the validator canonicalized encoded test suffixes, rejected fragments and routing controls, bounded and destroyed its one-shot socket, and kept the executable guard contract aligned with `AGENTS.md` and `ci-local.sh`.
- The handler-fixture task reached spec and quality consensus after replacing all 31 ordinary cost-10 setup registrations, adding producer-parity and fail-closed URL-routing regressions, and preserving the dedicated real-registration tests.
- The store/contract task reached spec and quality consensus after validating effective pgx routing, preserving only same-host/same-port TLS fallback behavior, enforcing validator AST parity, and propagating cleanup errors.
- The Vitest task first received `[OPEN]` quality feedback that its error-identity assertion imported both producer and class from the same module and did not exercise the source-local runner.
  Commit `74adf8f` `[FIXED]` the finding by importing and executing the real signup-intent route, checking the root-produced error through the supported `zod/v4` entry point, and adding the same boundary to the standalone whiteboard suite.
  The original reviewer and an independent quality arbiter then approved; the repository has no production dependency that exposes a second physical Zod error producer, so an artificial nested-package fixture is not part of the application contract.

### Plan-wide Review 1 (2026-08-10)

- **Reviewers:** Claude self-review (Opus), Codex (`gpt-5.6-sol`, high), independent Claude (Opus), GLM 5.2.
- **Verdicts:** `[claude-self]` CHANGES REQUESTED; `[codex]` CHANGES REQUESTED; `[opus]` CHANGES REQUESTED; `[glm]` APPROVE with concerns.

**Must Fix**

1. `[OPEN]` `[codex]` The ended-session mutation recheck is not atomic with the status transition (`server/hocuspocus.ts:134-168`, `platform/internal/store/sessions.go:461-464`).
   A frame can observe `live`, the end transaction can commit, and the frame can then apply and relay; the later storage guard drops it, so connected peers see state absent from the archive.
2. `[OPEN]` `[codex]` Established read-only canvas connections are not closed at JWT expiry (`server/hocuspocus.ts:202-238`, `server/realtime-jwt.ts:12-18`).
   A departed/revoked viewer can keep receiving updates indefinitely rather than for the accepted approximately 25-minute token window.
3. `[OPEN]` `[claude-self][opus][glm]` Remote Excalidraw scenes echo back into Yjs (`src/components/session/whiteboard/excalidraw-board.tsx:21-39`, `src/lib/whiteboard/excalidraw-yjs.ts:26-47`, `src/lib/whiteboard/use-whiteboard.ts:51-61`).
   `updateScene` triggers `onChange`, and the writer unconditionally stores the same serialized scene, so two writable tabs can ping-pong updates without user input.
4. `[OPEN]` `[codex]` The settled host floor has no frontend control (`src/components/session/whiteboard/whiteboard-panel.tsx:72-104,147-162`).
   No live surface reads or calls `PATCH /api/sessions/{id}/settings`, so teachers cannot use the supervision mechanism without a manual API call.
5. `[OPEN]` `[codex][claude-self][opus][glm]` The Phase-4 acceptance contract is incomplete (`docs/plans/094-session-whiteboard.md:143-146`).
   The exact named mint/ended-mutation tests were folded into differently named table tests, the live-panel create/visibility/owner-viewer tests are absent, and no Playwright create-to-loosen-to-view spec exists.

**Should Fix**

6. `[OPEN]` `[claude-self][opus]` Every Excalidraw `onChange` writes a full-scene Yjs frame and every writable canvas frame performs an uncached HTTP recheck plus several database queries (`excalidraw-board.tsx:34-39`, `hocuspocus.ts:134-168`, `realtime_token.go:320-371`).
   Active drawing can saturate the API/database and make the fail-closed guard disconnect clients under its own load.
7. `[OPEN]` `[claude-self][opus]` The binding shares the entire Excalidraw `appState` (`src/lib/whiteboard/excalidraw-yjs.ts:33-36`, `src/components/session/whiteboard/excalidraw-board.tsx:23-26`).
   Writer viewport, zoom, selection, tool, and view-mode state can overwrite each viewer's local UI state.
8. `[OPEN]` `[claude-self][opus]` Any authenticated outsider admitted to a public class-less session can create private canvases until the per-session cap is exhausted (`platform/internal/handlers/canvases.go:70-79`, `platform/internal/store/canvases.go:152-160`).
   Decision 11 settled broad live reads, but did not settle outsider-owned writes or denial of service against the class.
9. `[OPEN]` `[codex]` The board discards Excalidraw `files` while image-reference elements remain enabled (`src/components/session/whiteboard/excalidraw-board.tsx:31-39`, `src/lib/whiteboard/excalidraw-yjs.ts:26-38`).
   A pasted image can appear locally but disappear for peers and after archive reload.
10. `[OPEN]` `[claude-self]` `onLoadDocument` ignores the current canvas `readOnly` response (`server/hocuspocus.ts:256-266`).
    A stale writable claim connecting after end is advertised writable until its first mutation is reclassified; the current decision should update the connection configuration immediately.
11. `[OPEN]` `[glm]` Canvas authorization has no platform-admin/impersonation bypass (`platform/internal/handlers/realtime_token.go:320-371`), unlike other realtime document types.
    Private student-canvas oversight versus least-privilege denial is a trust-model decision, not a mechanical assumption.

**Nice to Have**

12. `[OPEN]` `[claude-self]` The irreversible `participants`/`session` visibility choices lack the plan's confirmation (`whiteboard-panel.tsx:90-104`), and a failed teacher end request gives no visible error (`teacher-dashboard.tsx:247-252`).
13. `[OPEN]` `[claude-self][opus]` The neutral room redirects every student-page 404, including a nonexistent session, to the generic archive (`src/app/(portal)/sessions/[id]/page.tsx:110-119`).
14. `[OPEN]` `[claude-self]` `docs/testing.md:38-42` still recommends `bun run --env-file=/dev/null test` although the Phase-6 contract replaced that claim with five explicit empty provider keys; `student-session.tsx:102` also retains stale effect dependencies.
15. `[OPEN]` `[claude-self][opus][glm]` Minor cleanup remains around the dead canvas `plain_text` column/write path, the generic `/settings` route name, duplicate session lookup on list, the owner foreign-key delete policy, and the broadened blank-attempt load log.

The review converged strongly on the binding and missing-contract findings.
The atomic end transition, public-session create policy, admin visibility, and newly required end-session/E2E files cross the frozen scope or trust model, so the hard safeguard pauses implementation until those decisions and scope additions are explicit.

## Post-Execution Report

_Plan-wide report pending later phases._

### Phase 1a — schema and store (2026-08-07)

- Added the ordered `canvas_visibility` enum, the backfilled non-null `sessions.canvas_floor`, and the `session_canvases` persistence row (including its Yjs state) in `0028_session_canvases.sql`.
- Added the schema declarations and `CanvasStore` create/get/list/update-floor/delete operations, including session-row locking for the floor invariant and archive-specific visibility checks.
- `bash scripts/check-migration-uniqueness.sh` passed.
- Applied `0028_session_canvases.sql` to the explicitly pinned throwaway `bridge_test` database.
- Focused Go store tests passed against that database, including resolved-database URL guard attacks, isolated migration defaults/backfill (including generated canvas IDs), PostgreSQL-observed deterministic floor-race checks with cleanup-safe failure paths, archive roles, cap, and class-bound cross-org isolation.

### Phase 1b — handlers, realtime mint, and JWT claim (2026-08-07)

- Added the byte-compatible `readOnly` JWT claim in Go and TypeScript; the existing signing API mints `false`, and canvas archive/viewer decisions mint or recheck `true`.
- Added scoped canvas metadata routes and wired their store plus the `canvas:{id}` resolver into the API server.
- Canvas mutations re-check `sessions.status` while holding the same session-row transaction lock as their store write, so post-end requests return 409 without a handler-to-store TOCTOU window.
- Focused Go auth, store, handler, mint-matrix, internal-auth, and class-bound/cross-session isolation tests passed only against the explicitly pinned `bridge_test` database; no migration ran in this phase.
- **y-excalidraw vetting gate: FAIL.** npm latest is 2.0.12 (2024-12-10), the upstream README still lists tests as TODO, and its development matrix is React 18.3.1 while Bridge uses React 19.2.4. Its MIT license passes and Excalidraw itself supports host-controlled `viewModeEnabled`, but there is no maintained React-19/read-only evidence for the binding. Phase 3 must use the plan-authorized thin custom `onChange` ↔ Yjs binding; no package was installed.

### Phase 1b — review-fix evidence (2026-08-07)

- Preserved rolling JWT compatibility: an absent token `readOnly` claim normalizes to false, a present non-boolean claim is rejected, and non-canvas internal-auth 200 responses may omit `readOnly` while canvas responses may not.
- Internal auth now validates both `allowed` and current canvas `readOnly` response types before accepting a 200 body, failing closed on malformed data.
- Replaced the separate parameterized session router mount with direct canvas route registrations, so it cannot shadow `/api/sessions/public` or existing `{id}` session routes; a combined-router test covers public listing, canvas creation, and session end.
- Added stable title validation errors, UUID validation before the canvas document query, expanded internal-auth branch coverage, and bounded fixture connection/cleanup contexts.
- Focused Go suites and TypeScript type-check passed against the explicitly pinned `bridge_test` database with `DATABASE_URL` unset.
- Follow-up compatibility fix: non-canvas 200 responses that omit `readOnly` retain the legacy `{allowed, reason}` object shape; canvas responses remain strict. The scoped Bun compatibility test and the established `tests/unit/realtime-jwt.test.ts` Vitest regression test both passed via the pinned Bun runtime.
- The authoritative `bun run test` command now runs Vitest and the scoped Hocuspocus canvas compatibility suite, so the fail-closed `readOnly` contract is included in the normal local/CI test step.
- The script uses `bunx --bun vitest run` because the bare Vitest binary selected the incompatible system Node runtime here. With provider keys explicitly empty and both database variables pinned to `bridge_test`, the scoped Bun suite and TypeScript type-check passed. A full Vitest attempt emitted only `tests/integration/python-101-import.test.ts (0 test)` before terminating without a normal suite summary, so that wider Vitest result remains unverified in this environment.

### Phase 2 — Hocuspocus canvas persistence and mutation-guard tests (2026-08-07)

- `PATH=/home/chris/.bun/bin:$PATH DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun test server/hocuspocus.canvas.test.ts` passed 16 tests with 55 assertions.
- The persistence test writes an owner-style Yjs map update to a live canvas, reloads its base64 snapshot, applies it to a fresh `Y.Doc`, and proves the map content survives before separately proving a post-end write is rejected.
- The guard tests reject a malformed raw frame and a mutation without authenticated user context before any access recheck can allow them.
- The exported `loadCanvasYjsState` seam rejects an invalid database query instead of silently returning blank state.
- Every database-touching test parses `DATABASE_URL` and requires a `_test` database name, independently verifies `current_database()` before writes, performs no migration, and deletes only its generated canvas, session, and user rows.
- A RED run against absent Phase 2 behavior was not possible in this delegated test pass because commit `f72d8ab` already contained the Phase 2 implementation before these tests were added; hiding or changing the shared production work would have violated this task's scope.
- `PATH=/home/chris/.bun/bin:$PATH bunx --bun tsc --noEmit` passed.

### Phase 2 — test portability correction (2026-08-07)

- The first scoped portability run with `TEST_DATABASE_URL` unset failed exactly two database tests because their helper required that variable, while `scripts/ci-local.sh` supplies only `DATABASE_URL`.
- The helper now accepts CI/service-container connection URLs, requiring only that `DATABASE_URL` parses to a `_test` database name; the independent `current_database()` assertion remains before any write.

### Phase 2 — missing-canvas persistence regression (2026-08-07)

- The regression supplies a valid random UUID with no `session_canvases` row and requires the exported `loadCanvasYjsState` helper to reject rather than treating absence as a blank Yjs state.
- Commit `a8ac3ca` had already landed the missing-row fail-closed production fix before the test's first execution, so no behavior-level RED against `c328609` could be reproduced without rolling back shared production.
- The first assertion expected an unnecessarily specific error string and failed only because the existing fix reports `Canvas does not exist`; the final regression asserts the required rejection behavior without coupling to message wording.

### Phase 2 — implementation review gate (2026-08-07)

- Independent contract and quality/security reviewers both approved after the registered-hook tests and database-cleanup guard were added.
- The final mutation regression dispatches the exact hook object passed to `new Server`, then proves an ended-session update neither changes the Yjs document nor reaches an observer.
- The final persistence regressions dispatch the registered load and store hooks, reject a valid missing canvas row, restore the pre-end snapshot, and leave it unchanged after an ended-session store attempt.

### Phase 3 — archive interaction regression (2026-08-08)

- Added `tests/unit/whiteboard-archive.test.tsx` for the neutral archive's list-first/token-on-selection boundary, generic empty state, absent mutation controls, forced `readOnly` binding options, and inert archive board-change path.
- The first isolated command was blocked by the test harness because the inherited `DATABASE_URL` named `bridge`; rerunning against the explicitly pinned `bridge_test` database exposed the missing mocked auth context rather than production behavior.  The final focused Vitest run passed 3 tests, and `bunx --bun tsc --noEmit` passed; neither command ran a migration or started a service.
- Replaced stale teacher history no-link assertions with durable `/sessions/{id}/whiteboards` archive-link assertions, and added the neutral room's former-participant ended-session redirect regression without a live-room join.
- Added a direct `useWhiteboard` hook regression that invokes its real `onChange`: read-only archive mode produces no custom-binding Yjs write, while writable mode does.  The root Vitest glob intentionally covers only `tests/**`, so the source-local whiteboard Vitest configuration is now invoked by the normal `bun run test` script.  Focused history/room tests passed 10 tests, the hook suite passed 2 tests, and the TypeScript check passed against the pinned `bridge_test` environment without a migration or service.
- Mounted the actual teacher and student session components behind only their external/session-heavy dependencies to prove redirect timing: a non-2xx end response leaves the teacher route unchanged, a 204 end response routes to the archive, and a dispatched student `session_ended` SSE event assigns the archive URL.  The expanded focused archive/history/room suite passed 16 tests; the direct hook suite and TypeScript check remained green.

### Phase 5 — multi-object startup schema probe (2026-08-10)

- RED: the first pinned `bridge_test` run of `go test ./internal/db/ -count=1` showed that the boot probe still targeted `books`, while missing `session_canvases.yjs_state` and `session_canvases_session_idx` incorrectly returned success; the existing parity test also reported 0028's two untracked indexes.
- GREEN: `ExpectedSchemaSentinels` is now a multi-object contract for the primary `session_canvases` table, the altered `sessions.canvas_floor` column, and the exact ordered `canvas_visibility` labels.  Runtime checks each table, table-scoped columns/constraints/indexes, and ordered enum labels, returning a typed `ErrSchemaEnumMismatch` diagnostic on an enum mismatch.
- The parser extracts `CREATE TYPE ... AS ENUM` and `ALTER TABLE ... ADD COLUMN` alongside table/index declarations, and parity checks both directions for stale or missing table/enum sentinels.  Generic named-constraint coverage remains via a synthetic test-table sentinel.
- Integration tests now fail closed unless both the parsed `DATABASE_URL` path and `SELECT current_database()` end in `_test`; synthetic DDL remains limited to `bridge_test` and cleans up immediately.  No migration or service ran.
- `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./internal/db/ -count=1` and `go test ./... -count=1 -timeout 120s` passed.  `go vet ./internal/db`, `gofmt`, and `git diff --check` passed.

### Phase 6 — local-gate test infrastructure (2026-08-10)

- Replaced the 31 ordinary handler-test `RegisterUser` setup calls with the transaction-safe cost-4 `insertFixtureUser` helper while retaining real cost-10 registration coverage in the producer/auth tests.
  The fixture validates password and intended-role inputs before insertion, writes the user and email-provider rows atomically, and has persistence, rejection-without-write, and producer-parity regressions.
- Hardened the handler, store, and contract test database openers against parsed and effective routing overrides, non-test database names, cross-host fallbacks, and cleanup-error loss.
  Same-host/same-port PostgreSQL TLS fallback remains supported, and the duplicated store/contract validators are AST parity-enforced.
- Added the Node-18-compatible one-shot database validator and wired `ci-local.sh` to validate the ambient and selected URLs, pin both database variables into every mutating test runner, and keep all five live-provider keys explicitly empty.
  The guard self-test now exercises 58 database, billing-isolation, governance, and fixture-census cases.
- Added the Bun/Vitest Zod transform boundary to both Vitest configurations and a regression that imports the real signup-intent route, proves `z.object` module initialization and a 400 validation path, and preserves root-to-`zod/v4` error compatibility.
  The standalone whiteboard suite directly exercises the same Zod boundary.
- TDD RED evidence included the handler package exceeding 120 seconds in bcrypt setup, unsafe URL-routing forms being accepted by the earlier guards, Bun/Vitest failing at the app schema with `z.object` undefined when the Zod boundary was removed, and the stale teacher room assertion expecting a removed prop.
- `PATH=/home/chris/.bun/bin:$PATH DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bash scripts/ci-local.sh --fast` passed on commit `74adf8f` with all five provider keys empty.
  Evidence: root Vitest 112 files passed and 2 skipped, 875 tests passed and 11 skipped; standalone whiteboard Vitest 3/3; Hocuspocus 18/18; guard self-tests 58/58; all Go packages passed, including handlers in 33.691 seconds, store in 30.742 seconds, and contract in 0.023 seconds.
  E2E was intentionally skipped by `--fast`, so this attestation is phase evidence only and is not acceptable for merge.

### Phase 7 — permanent consensus review governance (2026-08-11)

- Added the exact-commit, read-only design gate to all four canonical governance documents with exactly two required reviewers: Codex Sol (`gpt-5.6-sol`, high) and Claude Code (`claude-fable-5`).
- Made design, plan, and code gates uncapped consensus loops, retained the risk-tiered plan/code roster, replaced the obsolete numeric-cap safeguard, and required an unavailable reviewer to pause rather than be silently substituted or waived.
- Recorded the three-round non-convergence checkpoint and repeated-finding/no-net-reduction user-decision triggers without converting elapsed rounds into approval.
- Mirrored the explicit user model-pin override in `AGENTS.md` and `docs/coding-agent.md`; Plan 094 continues to route all new or changed tests to Terra.
- Terra added executable governance assertions first; the initial pinned run produced the intended RED with 14 missing-governance failures.
  After implementation, `PATH=/home/chris/.bun/bin:$PATH DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bash scripts/tests/test-guards.sh` passed 73 checks with zero failures.
- `bash scripts/check-plan-uniqueness.sh`, `bash scripts/check-spec-uniqueness.sh`, `bash -n scripts/tests/test-guards.sh`, and `git diff --check` passed.
  A first run without Bun on `PATH` made the lint ratchet's suppressed ESLint command yield empty JSON; rerunning with the repository's installed Bun path passed without any lint-script change.

### Phase 8 — durable lifecycle schema, locks, and replacement transitions (2026-08-11)

- RED: the initial pinned store command failed at compile time because `sessionLifecycleAdvisoryKey`, lease ownership, confirmed/degraded transitions, and the stable lifecycle conflict errors did not yet exist.
  The scheduled replacement regression then failed with `started.ReplacedSessions` empty, proving the old direct end producer did not return the required durable replacement metadata.
- Pre-ship migration provenance: `git show main:drizzle/0028_session_canvases.sql` reported that the path does not exist on `main`; `git log --all -- drizzle/0028_session_canvases.sql` names only feature-branch commits `f5ad8e0` and `cdd0483`; and `git log origin/main..HEAD -- drizzle/0028_session_canvases.sql` names the same commits while `git log HEAD..origin/main -- drizzle/0028_session_canvases.sql` is empty.
  Migration 0028 was therefore rewritten before it shipped through `main`.
- Test-only reconciliation: both configured URLs were literally `postgresql://work@127.0.0.1:5432/bridge_test`, and the preflight `SELECT current_database()` returned `bridge_test`.
  Before reconciliation, the probe found only `session_canvases.plain_text`; the exact SQL delta was `ALTER TABLE sessions ADD COLUMN canvas_freeze_token uuid`, `ALTER TABLE sessions ADD COLUMN canvas_freeze_until timestamptz`, `ALTER TABLE sessions ADD COLUMN whiteboard_server_archive_complete boolean`, and `ALTER TABLE session_canvases DROP COLUMN plain_text` in one transaction.
  Afterward, the live probe returned all three nullable columns with types `uuid`, `timestamp with time zone`, and `boolean`, and the `plain_text` count was zero.
  No non-test connection, migration runner, service, or E2E command ran.
- GREEN: the lifecycle owner derives the fixed signed two-int4 advisory key vectors, uses transaction-scoped shared/exclusive locks and PostgreSQL `clock_timestamp()` with the named 15-second lease duration, conditionally commits confirmed snapshots, rolls back failed snapshot batches, and makes the false/no-snapshot degraded transition separately.
  Canvas create, update, floor, and delete take the shared lifecycle lock before their session row lock and return the stable end-in-progress error while a lease is live.
  Class creation and scheduled start take the replacement guard followed by collision-safe sorted lifecycle locks, persist archive-incomplete replacement state, clear leases, and return replacement metadata.
- `DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test go test ./internal/store -run 'Test(SessionLifecycle|CreateSessionReplacement|StartScheduledSessionReplacement)' -count=1` passed.
  The pinned `go test ./internal/store -count=1` passed in 28.874 seconds; pinned `go test ./internal/db -count=1` and `go vet ./internal/store ./internal/db` passed; `gofmt` and `git diff --check` passed.

### Phase 8 review-fix — lifecycle ownership, scheduling order, and schema parity (2026-08-11)

- RED: lifecycle review tests first failed to compile because the owner had no token-conditional abort or idempotent end-result API, and the scheduled-start test had no after-class-guard seam.
  The handler integration then returned 500 `Database error` instead of the required stable freeze 409.
- GREEN: abort now clears only a matching live token; a degraded end rejects a different unexpired token, consumes any expired residue, and an already-ended retry returns the durable true/false archive result without rewriting it while cleaning matching or expired residue.
  Store tests cover matching/different and expired/unexpired ownership, both already-ended archive values, confirmed bundle persistence, zero snapshots, rollback on an affected-count mismatch, and shared/exclusive transaction-scoped advisory lock behavior.
- Scheduled start now acquires the class guard, re-reads the still-planned schedule row `FOR UPDATE`, and only then discovers and locks live sessions.
  The controlled cancellation-after-guard regression proves the pre-existing live session remains live when that re-read rejects the cancelled schedule.
- Canvas create, update, delete, and floor mutations now return `409` with exact JSON `{"code":"session_end_in_progress"}` and leave persisted canvas/floor state unchanged during an active lease.
- The Drizzle `sessionCanvases` model no longer declares the removed `plainText` column.
  The scoped system-review operator query now requires exactly eight canvas columns and all three nullable lifecycle columns with their exact PostgreSQL types.
- Focused pinned store, handler, and DB suites passed; `go vet ./internal/store ./internal/handlers ./internal/db`, `gofmt`, `git diff --check`, and `PATH=/home/chris/.bun/bin:$PATH bunx tsc --noEmit` passed.
  No database reconciliation, migration, service, E2E, or non-test database access ran for this review fix.
