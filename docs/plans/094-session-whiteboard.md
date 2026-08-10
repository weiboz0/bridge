# Plan 094 — Excalidraw whiteboards in live sessions

**Branch:** `feat/094-session-whiteboard`
**Status:** Plan review passed at Revision 5 under the user's temporary no-Claude-reviewer direction. Phases 1a, 1b, 2, 3, and 4 are complete; Phase 5 verification and the code-review gate are in progress.

## File scope

`drizzle/**` (new migration) ·
`src/lib/db/schema.ts` ·
`platform/internal/store/canvases.go` (new) + `platform/internal/store/canvases_test.go` (new) ·
`platform/internal/handlers/canvases.go` (new) + `platform/internal/handlers/canvases_integration_test.go` (new) ·
`platform/internal/handlers/realtime_token.go` + `platform/internal/handlers/realtime_token_test.go` ·
**`platform/internal/auth/realtime_jwt.go`** (add `readOnly` claim — scope-widened R1) ·
**`platform/internal/auth/realtime_jwt_test.go`** (JWT compatibility regression) ·
**`platform/internal/db/migrations.go`** (latest schema probe + sentinels — scope-widened verification fix) ·
**`platform/internal/db/schema_probe.go`** + **`platform/internal/db/schema_probe_test.go`** + **`platform/internal/db/schema_probe_parity_test.go`** + **`platform/internal/db/schema_probe_integration_test.go`** (multi-object probe + tests — scope-widened verification fix) ·
**`server/realtime-jwt.ts`** (mirror the claim — scope-widened R1) ·
**`platform/cmd/api/main.go`** (wire `CanvasStore` + handler — scope-widened R1) ·
`platform/internal/handlers/routes.go` (or where session routes register) ·
`server/hocuspocus.ts` + `server/hocuspocus.canvas.test.ts` (new — read-only + ended-write realtime tests) ·
`next.config.ts` (only if `/api/sessions/{id}/canvases` isn't already covered by `/api/sessions/:path*`) ·
`src/lib/whiteboard/**` (new — the binding + hook) ·
`src/components/session/whiteboard/**` (new — canvas list, board surface, visibility control) ·
`src/components/session/teacher/teacher-dashboard.tsx` · `src/components/session/student/student-session.tsx` (add the whiteboard surface) ·
**`src/app/(portal)/sessions/[id]/page.tsx`** (link its ended-session notice to the archive) ·
**`src/app/(portal)/teacher/sessions/[sessionId]/page.tsx`** (link its ended-session notice to the archive) ·
**`src/app/(portal)/sessions/[id]/whiteboards/page.tsx`** (new, dedicated read-only archive route) ·
**`src/app/(portal)/teacher/page.tsx`** · **`src/app/(portal)/teacher/sessions/page.tsx`** · **`src/app/(portal)/teacher/classes/[id]/page.tsx`** · **`src/app/(portal)/student/classes/[id]/page.tsx`** (link ended-session history rows to the archive) ·
**`tests/unit/teacher-session-row.test.tsx`** · **`tests/unit/ended-sessions-non-link.test.ts`** · **`tests/unit/sessions-room-page.test.tsx`** (update ended-session expectations) · **`tests/unit/whiteboard-archive.test.tsx`** (new archive interaction regression) ·
**`tests/unit/excalidraw-yjs.test.ts`** (custom-binding regression) · `package.json` + **`bun.lock`** (add `@excalidraw/excalidraw`; no `y-excalidraw`) ·
`docs/api.md` · `docs/architecture/decisions.md` · `README.md` · **`.claude/skills/br-system-review/SKILL.md`** (operator probe guidance) · this plan file.

Scope-widening (R1 blocker 1 / concern C1) authorized by the user 2026-08-06: the read-only viewer boundary cannot be built without a `readOnly` claim in both JWT files, and `CanvasStore` must be wired in `main.go`.

Scope-widening (archive-route decision) authorized by the user 2026-08-07: ended-session whiteboards must be reachable without re-enabling the live dashboards, so add the two existing ended-session route files and one dedicated archive page.

Scope-widening (archive-entry review fix) authorized by the user 2026-08-08: add every existing teacher/student ended-session history entry point so former readers have a durable archive link rather than depending on a live SSE redirect.

Scope-widening (archive test review fix) authorized by the user 2026-08-08: add the existing ended-session regression tests and one focused archive test, so the new archive contract is enforced rather than contradicted by stale no-link assertions.

Scope-widening (scope-audit review fix) authorized by the user 2026-08-08: add the already changed lockfile, Go JWT regression, and custom-binding test so every branch artifact is governed by the plan.

Scope-widening (schema-probe verification fix) authorized by the user 2026-08-09 via “resume”: add `platform/internal/db/migrations.go`, because migration `0028_session_canvases.sql` became the latest schema-bearing migration and the enforced bidirectional parity test requires its table, columns, constraints, and indexes to replace the prior `books` sentinels.

Scope-widening (complete multi-object probe) authorized by the user 2026-08-09: add the probe implementation, its unit/parity/integration tests, and the tracked operator-review skill. Review found that retargeting only `migrations.go` would leave three `books`-specific integration tests stale and would ignore `0028`'s new enum plus `sessions.canvas_floor` alteration.

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
| 8 | **After the session ends: read-only archive.** No writes by anyone (incl. owner) once `status=ended` — enforced **server-side before every canvas mutation is applied or relayed, in `onStoreDocument`, and on every mutating endpoint**, not only at mint (R2 blockers 1–2; R4 relay hardening). Readable by the **owner**, the **teacher** (if visibility ≥ `host`), and **former participants** (if visibility ≥ `participants`) — where "former participant" = a `session_participants` row with status **`present` or `left`** (NOT `invited`/never-joined). Canvas auth special-cases ended sessions rather than reusing the live gate (which returns `ended → no_access`). | User (2026-08-06); status filter added R2; pre-apply gate added R4 |
| 9 | **Membership checks by level, reusing existing helpers, not re-rolled.** `host` → `sessions.teacher_id`. `participants` → a `present` `session_participants` row (the strict join check — *not* the public-open-join clause). `session` → `CanAccessSession` (the plan-090 guard, which for a public class-less session admits any authenticated user — this is intentional per Decision 11, not a leak). A hand-rolled membership query is how 090's cross-org leak would reappear — reuse the named helpers. | Reviewers (all 3) + user |
| 10 | **Per-session canvas cap** (e.g. 50) enforced at create under the session-row lock — bounds persisted-doc growth. | Reviewer (opus C5) |
| 11 | **`session` visibility is intentionally "as public as the session" while live.** In a public, class-less live session (any authenticated user can join), a `session`-visibility canvas is readable by any authenticated user — the board is exactly as public as the room. On end, public admission ends too: `session` and `participants` collapse to the Decision-8 former-participant archive rule (`present`/`left` only). An owner who wants join-only sharing picks **`participants`** instead. This makes the plan-090 public surface a *conscious owner choice per canvas*, not an accidental cross-org leak. Documented in `docs/api.md` + `decisions.md`. | User (R2 trust-model fork); clarified after GLM archive review 2026-08-08 |
| 12 | **Archive UI is a dedicated neutral route.** `/sessions/{id}/whiteboards` renders only the read-only whiteboard archive rather than reviving either live teacher or student dashboard. It never writes a scene or offers mutation controls, even if opened while a session is still live. It does not pre-authorize through the ordinary session page APIs, because `CanAccessSession` deliberately total-rejects every ended session — including the teacher and former participants — while the canvas list and minted `canvas:{id}` token have explicit archive branches for owner, teacher, and former participant visibility. Those two endpoints remain the metadata and document authorization boundaries. The neutral live-session route redirects its otherwise-404 former-participant path to this archive, which returns a generic empty state for callers with no visible canvases and never attempts a document token mint until an item is selected. | User (2026-08-07); tightened after archive-route review 2026-08-08 |
| 13 | **Migration 0028 advances the startup schema probe as a multi-object end-state contract.** `session_canvases` becomes the primary probe table. Its nine declared columns and two indexes are sentinels; its named-constraint list is empty. The probe also verifies the altered-table column `sessions.canvas_floor` and exact ordered values of the new `canvas_visibility` enum. Parity tests extract these declarations from the latest migration. Generic named-constraint checking remains covered with an injected test sentinel even though 0028 declares no named constraint. | Verification review + user (2026-08-09) |

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

**Ended = no writes, enforced server-side before apply/relay (R2 blockers 1–2; R3–R4 hardening).** Mint alone is insufficient (a 25-min write token outlives `ended`), and `onStoreDocument` alone is insufficient too (Hocuspocus applies a writable connection's update in-memory and broadcasts it to peers *before* storage). Hocuspocus 3.4.4 provides the exact pre-apply boundary: `beforeHandleMessage` runs before `MessageReceiver` applies a Yjs frame, `connection.readOnly` is mutable, and `MessageReceiver` rejects sync updates from a read-only connection. For each mutation-bearing Yjs sync frame on an already-writable `canvas:` connection, `beforeHandleMessage` calls the existing Go `POST /api/internal/realtime/auth` recheck. The internal response is extended with the caller's **current `readOnly` decision**. If the session is now ended, Hocuspocus sets `connection.readOnly=true` before allowing `MessageReceiver` to continue, so that very update is rejected and never applied or broadcast; a deny or recheck failure closes the connection (fail-closed). A small frame classifier distinguishes mutation-bearing sync step 2/update messages from awareness/query traffic so cursor motion does not create a DB round-trip. This pre-apply recheck is the load-bearing transition mechanism — no cross-process force-close race or fallback window. `onStoreDocument` still rejects ended-session storage as a durable backstop; every mutating endpoint 409s when ended; mint issues only `readOnly=true` for ended sessions.

**Session floor is capped at `participants` (R3 Opus — trust-model guard).** The host may set the floor to `private`/`host`/`participants`, never `session`. A `session`-visibility canvas is therefore *only ever an owner's per-canvas choice* (Decision 11), never imposed on students by a floor raise — otherwise a host could make every private student board world-readable in a public room without consent. Supervision needs (`participants`/`host`) are fully served; publishing a whole room is not a floor power.

**Floor invariant is race-safe (R1 blocker 2 + R2 blocker 5).** `visibility ≥ floor` can't be a Postgres cross-table CHECK, so **create**, **set-visibility**, AND **set-session-floor** all take `SELECT … FOR UPDATE` on the `sessions` row; create/set-visibility compute against the locked floor (`GREATEST` / reject `< floor`). This serializes all three writers so no canvas ever lands below the floor.

## Phases

### Phase 1a — Backend: schema + store *(Codex)*
- Migration **`drizzle/0028_*`** (0027 is taken by plan 090 — run `scripts/check-migration-uniqueness.sh`) + `schema.ts`: `canvas_visibility` pgEnum `(private, host, participants, session)` (declaration order = tight→loose); `sessions.canvas_floor canvas_visibility NOT NULL DEFAULT 'private'` (**backfills existing rows**); `session_canvases` (`id`, `session_id` FK, `owner_id` FK, `title`, `visibility canvas_visibility NOT NULL`, `created_at`, `updated_at`), org/tenant scoping like `sessions`.
- `store/canvases.go`: create (locks session row; `visibility = GREATEST(requested, floor)`; per-session cap under the lock), get, `list-visible-to-user` (owner/host/participant/session branches, scoped to path `session_id`, reusing the Decision-9 helpers; **special-case ended sessions the same way mint does** (R3 GLM) — for `ended`, use the archive rules, since `CanAccessSession` returns `ended→no_access` and would otherwise hide the archive from former participants), set-visibility (**locks session row**; loosen-only: reject target ≤ current or < floor), set-session-floor (host-only; **rejects `session` — floor capped at `participants`, R3**; locks session row; bumps `visibility < floor` up to floor in-txn; **lowering the floor is allowed** and leaves existing canvases as-is). DELETE purges the canvas row **and its persisted Yjs doc** (no orphan).
- `store/canvases_test.go`: floor default + backfill; `GREATEST` at create; **concurrent create-vs-raise AND set-visibility-vs-raise leave nothing below floor**; loosen accepted, tighten rejected; raise-floor bumps; list-by-role across all 4 levels; per-session cap; cross-org isolation; an enum-ordinal assertion (`private<host<participants<session`) so a future reorder can't invert the floor compare.

### Phase 1b — Backend: handlers + token mint + JWT claim *(Codex)*
- `auth/realtime_jwt.go` + `server/realtime-jwt.ts`: add `readOnly` claim (both must stay byte-compatible — same field name/JSON tag).
- `handlers/canvases.go`: `POST /api/sessions/{id}/canvases` (member; owner; starts at floor; cap-checked); `GET …/canvases` (visible-to-caller); `PATCH …/canvases/{cid}` (owner-only; loosen-only visibility, title); `DELETE …/canvases/{cid}` (owner-only; lifecycle); host-only `PATCH …/settings` for `canvas_floor`. **Every mutating endpoint 409s when the session `status=ended`** (R2 blocker 2 — no post-end loosening that would widen the archive).
- `realtime_token.go`: add `canvas:{cid}` to the scope resolver implementing the live/ended mint matrix; set the `readOnly` claim. Refactor the shared document authorization result to carry `{role, readOnly}` so `POST /api/internal/realtime/auth` returns the same current `readOnly` decision to Hocuspocus (existing scopes default to `false`; canvas owners become `true` when ended).
- `main.go`: wire `CanvasStore` + handler.
- `canvases_integration_test.go` + `realtime_token_test.go`: the mint matrix (below), incl. ended-session archive.
- **y-excalidraw vetting gate** (blocks Phase 3) — record findings here.

### Phase 2 — Realtime: read-only enforcement + persistence *(Codex / inline)* — load-bearing
- `hocuspocus.ts`: handle `canvas:` in `onAuthenticate` — read `claims.readOnly`, set the **connection's `readOnly`** (Hocuspocus 3.4.4's write-enforcing `connectionConfig.readOnly`, mutated in `onAuthenticate` — verified propagates to `Connection.readOnly`), carry context. A token missing `readOnly` defaults to `false` so existing `attempt:` tokens are unaffected. Add a `beforeHandleMessage` guard for mutation-bearing canvas sync frames while `connection.readOnly=false`: call the existing Go internal recheck, fail closed on deny/error, and flip `connection.readOnly=true` before `MessageReceiver` handles the frame when the current decision is read-only. Non-mutating sync/awareness/query traffic bypasses this per-mutation recheck. `onLoadDocument`/`onStoreDocument` persist the canvas Yjs doc (debounced snapshot). **`onStoreDocument` also drops/rejects writes when the session is `ended`** as a durable backstop.
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

## Risks

| Risk | Mitigation |
|------|------------|
| **Realtime auth is a hard safeguard** — a permissive mint leaks a private/cross-org canvas. | Enforce at mint (server), reuse the plan-090 membership guard (Decision 9), test the full owner/host/member/non-member/cross-org matrix + ended-session archive. 4-way review must scrutinize `realtime_token.go` + `hocuspocus.ts`. |
| **Read-only not actually enforced** (the whole boundary). | Phase 2 sets the connection's `readOnly`, rechecks current state before every mutation-bearing frame from an existing writer, and tests both a viewer write and a pre-end writer's post-end write are rejected before apply/broadcast. `viewModeEnabled` is UX only. |
| Floor invariant race (create vs raise). | `SELECT … FOR UPDATE` on the session row in both; `GREATEST(requested, floor)`. Concurrency test required. |
| Owner over-shares by accident (loosen-only, Decision 7). | Accepted for MVP; a host-moderation follow-up can add tighten/override. UI shows a confirm on loosening to `participants`/`session`. |
| **Removed owner keeps write** (Decision 1 owner-durability + Decision 6 no per-canvas moderation) — a kicked participant who owns a canvas can still mint a write token while the session is live; the teacher has no per-canvas remedy. | **Accepted MVP risk, explicitly.** The host lever is the floor, not per-canvas control. Host-moderation follow-up adds owner-eviction / canvas takedown. Stated in `decisions.md`. |
| A member who **leaves** keeps a `participants`/`session` read token ~25 min (Decision 7). | Accepted, TTL-bounded; matches existing session/attempt-doc behavior. Not a tightening problem. |
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

## Code Review

_Pending._

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
