# Plan 094 — Excalidraw whiteboards in live sessions

**Branch:** `feat/094-session-whiteboard`
**Status:** Phases 1a through 12 and Phase 14 (E2E stack attestation) are implemented; Phase 13 (closing) is in progress. Remaining: code-review consensus (Round 1 on `8ce0a6e` requested changes; nine of ten findings fixed, R2-1 open until the attestation has run against a live stack), the full pinned-E2E gate on the merge commit, and the plan-wide report.
Earlier status, kept for history: Phases 1a through 6 are complete.
Spec 013 passed its exact-commit Sol + Fable 5 design gate; the user approved the remediation scope widening on 2026-08-11.
The Spec 013 remediation plan gate reached consensus at exact substantive commit `fdaf90af2a6c9500396ced27f2ca0df4be780645`.
Phase 7 is complete; Phases 8 through 13 are authorized for phase-by-phase implementation, and Phase 14 is authorized once its plan-gate revision below reaches consensus.

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
**`src/lib/realtime/get-token.ts`** + **`src/lib/realtime/use-realtime-token.ts`** + **`tests/unit/realtime-get-token.test.ts`** + **`tests/unit/use-realtime-token.test.tsx`** (canvas-only session identity hint, cache identity, and signed-claim propagation) ·
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
**DEMO-SEED SCOPE (user-authorized 2026-08-31):** **`scripts/seed_problem_demo.sql`** · **`scripts/tests/test-problem-demo-seed.sh`** (new focused contract test) · **`docs/setup.md`** ·
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
**PHASE-13 E2E GATE REMEDIATION SCOPE (user-authorized 2026-08-31):**
**`e2e/auth-identity.spec.ts`** · **`e2e/auth.spec.ts`** · **`e2e/auth.setup.ts`** · **`e2e/courses.spec.ts`** · **`e2e/editor.spec.ts`** · **`e2e/help-queue.spec.ts`** · **`e2e/hocuspocus-auth.spec.ts`** · **`e2e/impersonation.spec.ts`** · **`e2e/live-session.spec.ts`** · **`e2e/portals.spec.ts`** · **`e2e/session-flow.spec.ts`** · **`e2e/unit-picker.spec.ts`** (repair stale route, fixture-role, and cross-test-state contracts exposed by the exact full local gate; no production auth or tenancy change is authorized by this widening) ·
**`.env.example`** · **`docs/setup.md`** · **`docs/project-structure.md`** (server-only control configuration and ports) ·
**`docs/reviewers.md`** + **`docs/development-workflow.md`** + **`docs/coding-agent.md`** (permanent review-gate and dispatch contracts).

**PHASE-14 E2E STACK ATTESTATION SCOPE (user-authorized 2026-09-21 via the “Build attestation now” decision on Plan-wide Review 2 finding R2-1):**
**`scripts/check-e2e-stack.mjs`** (new gate-side verifier, also imported by `e2e/seed.setup.ts`; governance from this phase on) · **`scripts/tests/e2e-stack-vector.json`** (new shared cross-implementation vector) · **`scripts/tests/check-e2e-stack.live.test.mjs`** (new live-`_test` verifier regressions) · `scripts/ci-local.sh` + `scripts/tests/test-guards.sh` (already scoped) ·
**`platform/internal/handlers/e2e_stack.go`** + **`platform/internal/handlers/e2e_stack_test.go`** + **`platform/internal/handlers/e2e_stack_integration_test.go`** (new flag-gated attestation endpoint) ·
`platform/internal/config/config.go` + its test, `platform/cmd/api/main.go`, `platform/internal/store/session_lifecycle.go` + its test (already scoped; the reserved lock-class constant lives beside the other two) ·
**`server/e2e-stack.ts`** + **`server/e2e-stack.test.ts`** (new Hocuspocus-side attestation) · `server/hocuspocus.ts` (already scoped; registers the `onRequest` hook) ·
**`src/app/api/e2e-stack/route.ts`** (new Next.js attestation route, outside every Go-proxied prefix) + **`tests/unit/e2e-stack-route.test.ts`** (new) + **`tests/unit/e2e-seed-attestation.test.ts`** (new) · `tests/unit/shadow-routes.test.ts` (already scoped) ·
`e2e/seed.setup.ts` (already scoped; re-verifies before its first mutation) · `package.json` (already scoped; adds the new Bun test files to `test`) · **`platform/cmd/api/main_test.go`** (flag refusal tests beside the existing guard tests) ·
`AGENTS.md`, `README.md`, `docs/api.md`, `docs/testing.md`, `docs/setup.md`, `docs/project-structure.md`, `.env.example`, `docs/architecture/decisions.md` (already scoped).
No file under `platform/internal/db/`, `platform/internal/realtime/`, `server/db.ts`, `src/lib/db/`, `next.config.ts`, `.githooks/`, or `scripts/pre-merge-guard.sh` changes in this phase.

**SCOPE-AUDIT CORRECTION (2026-09-21):** a mechanical audit of `git diff --name-only main...HEAD` against this section found five files changed on the branch under the 2026-08-10 blanket authorization but never listed here:
**`e2e/playwright.config.ts`** · **`tests/unit/playwright-config.test.ts`** · **`tests/unit/vitest.playwright-config.config.ts`** (the Phase-13 fail-closed Playwright configuration and its isolated regression) ·
**`tests/unit/realtime-jwt.test.ts`** · **`tests/unit/use-yjs-provider-hook.test.tsx`** (Phase-10 TypeScript JWT and provider-hook regressions).
This records prior scope drift at a paused gate rather than erasing it: the files were changed outside the declared scope, and they are listed now so the declared scope covers the branch diff, exactly or through the declared globs. No file's content changes because of this correction.

The existing broad entries for `drizzle/**`, `src/lib/db/schema.ts`, `platform/cmd/api/main.go`, `platform/internal/handlers/realtime_token*`, `server/hocuspocus*`, whiteboard frontend files, schema-probe files, documentation, and tests remain authoritative for remediation changes in those paths.

Scope-widening (Spec 013 remediation) authorized by the user 2026-08-11 via “approved” after the Sol + Fable 5 design gate passed.
This approval covers the exact additions above, including the governance files; database migrations remain subject to the non-test-database hard safeguard, and E2E remains forbidden without a separately started Bridge stack plus explicit pinned `E2E_BASE_URL`.

Scope-widening (canvas lifecycle lock-key correction) authorized by the user 2026-08-12 via “go ahead.”
The canvas document name remains `canvas:{canvasId}`, while the four added realtime helper/test files carry a required canvas-only `sessionId` hint through minting and cache identity.
The signed canvas JWT and every internal canvas recheck carry the authoritative session ID so, after ordinary request authentication, Go can acquire the matching shared lifecycle lock before any canvas, session, user, participant, class, or membership read in the canvas-authorization transaction.
The hint and claim select a lock and constrain the authoritative query; neither grants access.
Missing, malformed, or mismatched canvas session IDs fail closed, while non-canvas request and JWT contracts remain compatible.
This substantive Spec 013 revision must clear the uncapped Sol + Fable 5 design gate before implementation.

Scope-widening (Phase 13 broad E2E gate remediation) authorized by the user 2026-08-31 via “go ahead.”
The widening is limited to the twelve E2E files named above and does not authorize production auth, tenancy, impersonation, or session-verification changes.
The scoped `src/lib/whiteboard/**` tests must exercise the real `useWhiteboard` producer with the selected `(canvas:{canvasId}, sessionId)` pair and prove a changed hint clears the retained token before reminting.
The lock-key correction passed its exact-commit Round 16 design gate on `e91598fb8b177063a8afd2bc0fd9f791c9c76a0a`: Sol APPROVE and Fable 5 APPROVE, with no open findings.

#### Phase 10a — canvas lock identity RED tests (2026-08-12; Terra; tests only)


- `[RED]` Added focused contract tests for canvas mint and internal recheck missing, malformed, and mismatched session IDs; authoritative JWT payload identity; canvas-only Go/TypeScript JWT claim validation; a store-level exclusive-lock proof that fails unless the supplied session ID is locked before every authorization read; Hocuspocus authenticated context plus admission/mutation propagation; browser mint cache identity; and the real `useWhiteboard` producer pair.
  The tests intentionally remain RED until the approved Round 16 production changes make the session-ID hint required, acquire the lifecycle lock before canvas authorization reads, sign/verify/propagate the claim, and key browser state by the pair.
- `[UNVERIFIED]` The deterministic exclusive-lock ordering proof is intentionally RED because the current store method does not yet require a session ID; it will assert no authorization-table relation lock exists while an exclusive lifecycle lock is held once the approved transaction seam exists.
- `[RED evidence]` With both database variables pinned to `postgresql://work@127.0.0.1:5432/bridge_test`, the focused Go run fails as expected: canvas mint and internal auth return 200 for missing, malformed, and mismatched hints; the minted JWT payload omits `sessionId`; canvas JWT verification accepts missing/malformed claims and a non-canvas claim; and the store lacks the required session-bound authorization method.
  The focused Bun/Vitest runs fail as expected because TypeScript accepts canvas JWTs without the claim, browser cache and in-flight state reuse across distinct hints, the hook retains `canvas-A` on a hint-only change, the real whiteboard producer omits the second argument, and Hocuspocus context omits the verified ID.

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

#### Phase 9 backend execution evidence (2026-08-11; frontend remains pending)

- `[backend]` Added the strict Go control client with an overall two-second same-token retry budget, redirect refusal, 48 MiB bounded reads, exact response/base64/digest/ID validation, and best-effort terminal cleanup.
  API startup validates the separately generated control bearer and a numeric-loopback HTTP or verified-HTTPS origin before constructing that client.
- `[backend]` Added the freeze-auth callback and shared lifecycle authorization read; active leases now return retryable `409 session_freezing` rather than downgrading a writer to read-only.
  The exact route cutover, represented-teacher/present creation check, and archive-completeness settings response are covered in the Go handler suite.
- `[backend]` Reworked explicit end to commit the lease/list handoff before control HTTP, persist confirmed snapshots or a separate durable degraded result, and only then emit events/complete schedules.
  Create and scheduled-start settle every replacement after their already-durable transaction without requiring Hocuspocus.
- `[GREEN]` With both `DATABASE_URL` and `TEST_DATABASE_URL` pinned to `postgresql://work@127.0.0.1:5432/bridge_test`: `go test ./internal/realtime ./internal/config ./internal/handlers ./internal/store -run 'Test(CanvasControl|EndSession|CreateSessionReplacement|ScheduleStartReplacement|FreezeAuth|RealtimeAuthLifecycle|CanvasSettings|CanvasCreate|RealtimeControl)' -count=1 -timeout 120s`; then the full `go test ./internal/handlers -count=1 -timeout 120s`, `go test ./internal/store -count=1 -timeout 120s`, `go vet ./internal/realtime ./internal/config ./internal/handlers ./internal/store`, and `git diff --check`.
- `[PENDING]` Phase 9 is not complete: the frontend settings producer/consumer cutover and its required Vitest/typecheck evidence remain owned by the frontend slice.

#### Phase 9 backend review remediation (2026-08-11; partial backend evidence)

- `[backend]` The control client now uses the exact `freezeToken` wire and
  `/internal/canvas-sessions/{freeze,complete,unfreeze}` endpoints.
  It validates a sorted unique requested set of at most 50 IDs, a sorted subset snapshot response, per-snapshot and aggregate decoded-size limits, bounded body reads, no redirects, and an original two-second retry budget for transport and retryable-conflict failures.
- `[backend]` Control configuration now derives `HOCUSPOCUS_INTERNAL_URL` from the control port and requires a separate fixed-size hex bearer.
  Plain HTTP is restricted to canonical numeric loopback; HTTPS retains certificate verification; the freeze-auth bearer comparison is constant-time.
- `[backend]` Canvas document authorization and creation move current-state decisions under the lifecycle lock, with creator teacher/present eligibility checked in the creation transaction.
  Explicit end responses are top-level, include the durable completion flag, and use stable `whiteboard_server_archive_incomplete` warning metadata without a post-commit database reread.
- `[PENDING]` Phase 10's control listener and the Phase-9 frontend consumer remain out of scope for this backend remediation.

#### Phase 9 lifecycle-protocol proof remediation (2026-08-12)

- `[backend]` The control response schema is now exactly `snapshots[{canvasId,stateBase64,sha256}]` plus a required non-negative `closed` count.
  Terminal calls accept only exact HTTP 200 JSON acknowledgements: `{ "unfrozen": true }` or `{ "released": true }`.
  The client retries pre-header, retryable-conflict, and body-read transport failures with cryptographic bounded jitter inside one original two-second deadline.
- `[backend]` Freeze-auth rejects unknown and trailing JSON, uses a constant-time control bearer comparison, and status-first end returns the database transaction's durable `ended_at` rather than fabricating a process-clock timestamp.
  End/replacement ordering is durable transition, event, scheduled completion, then asynchronous best-effort terminal complete; failure cleanup uses a fresh bounded context and token-conditional abort.
- `[GREEN]` Before this commit, both `DATABASE_URL` and `TEST_DATABASE_URL` were pinned to `postgresql://work@127.0.0.1:5432/bridge_test` for `go test ./internal/realtime ./internal/config ./internal/handlers ./internal/store -run 'Test(CanvasControl|EndSession|CreateSessionReplacement|ScheduleStartReplacement|FreezeAuth|RealtimeAuthLifecycle|CanvasSettings|CanvasCreate|RealtimeControl|SessionLifecycle)' -count=1 -timeout 120s`, `go test ./... -count=1 -timeout 120s`, `go vet ./internal/realtime ./internal/config ./internal/handlers ./internal/store`, and `git diff --check`.

#### Phase 9 final backend boundary proofs (2026-08-12)

- `[backend]` The canvas auth transaction performs only a canvas-to-session identity lookup before its shared lifecycle lock, then rereads the canvas, session, participant, and access state under that lock so a deleted canvas cannot authorize after the lock wait.
  Freeze retry is limited to retryable control transport/409 paths; deterministic response bounds and schema errors fail closed without retry.
- `[backend]` Control terminal acknowledgements are exact 200 JSON, and replacement responses always serialize `replacedSessions` as an array.
  The phase remains backend-only; the Hocuspocus listener and frontend consumer work are not claimed by this evidence.

#### Phase 9 adversarial RED matrix (2026-08-12; test-only)

- `[RED]` Terra added mechanism-level rejection coverage before any further production change: invalid explicit control ports, numeric IPv4/IPv6 loopback boundaries, strict control request and bundle schemas, canonical base64/digest and decoded-size limits, exact terminal acknowledgements, freeze-auth malformed/expired/missing lifecycle cases, retryable ordinary canvas authorization, and a capture-to-completion stale-token race.
- `[RED]` With both URLs parsed and live-verified as `bridge_test`, the focused control command fails because `ValidateControlURL` accepts ports `0` and `65536` for both canonical loopback forms.
  The focused end command independently fails because a different unexpired lease installed after capture reaches the degraded completion branch as a generic `500 {"error":"Database error"}` rather than `409 {"code":"session_end_in_progress"}`.
- `[RED]` No production file changed in this matrix commit.

#### Phase 9 additional test-only protocol coverage (2026-08-12)

- `[GREEN]` Added direct end degradation coverage for timeout, transport loss, non-2xx, and malformed control outcomes; every case proves the durable false warning plus post-commit terminal cleanup.
  The response proof binds top-level `endedAt` to the stored transaction timestamp, while the terminal callback observes `ended` only after the durable transition.
- `[GREEN]` Added control-client proofs for local validation without a network attempt, byte-identical same-token retry bodies, caller deadline cancellation, verified HTTPS acceptance, rejected `InsecureSkipVerify`, bounded jitter, exact terminal negative acknowledgements, and settings/creator public-session/cap races.
  The handler-focused command passed with both configured and live database names verified as `bridge_test`.
- `[RED]` The preceding invalid-port and stale-live-token failures remain unresolved; this test-only update did not change production behavior.

#### Phase 9 canvas lock-identity cutover and boundary fixes (2026-08-12)

- `[RED]` The coordinated-cutover suite proved the old implementation accepted missing, malformed, and mismatched canvas session IDs; omitted the binding from signed JWTs; reused cached and in-flight tokens across different hints; omitted the hint at the real `useWhiteboard` producer; and lost it in Hocuspocus admission and mutation rechecks.
- `[backend]` Canvas mint and internal auth now canonical-validate a supplied session ID, acquire its shared lifecycle lock as the canvas-authorization transaction's first database operation, verify user existence and the exact canvas/session binding under that lock, and sign the authoritative binding into a required canvas-only JWT claim.
  Non-canvas request and JWT shapes remain unchanged, while legacy or malformed canvas claims fail closed in both Go and TypeScript.
- `[realtime/frontend]` Hocuspocus retains the verified binding in its authentication context and includes it in every canvas admission and mutation recheck.
  The browser token cache and in-flight map use the `(documentName, sessionId)` identity, `useRealtimeToken` derives an empty result synchronously when either identity component changes, and the real whiteboard producer supplies the selected canvas pair before mounting a provider.
- `[backend]` Control origins now reject explicit ports outside `1..65535` for canonical IPv4 and IPv6 loopback.
  A different live lease installed after capture returns stable `409 session_end_in_progress`; token-conditional cleanup cannot clear the foreign operation.
- `[GREEN]` With configured and live database identity pinned to `bridge_test`, the focused Go auth/handler/store lock-identity suite passed, as did the exact invalid-port and stale-token regression tests.
  The focused Vitest suites passed 43 tests across JWT/cache/hook files and four source-local whiteboard tests; the Hocuspocus canvas suite passed 19 tests; exact changed-file ESLint and `git diff --check` passed.
- `[UNVERIFIED]` Root `tsc --noEmit` remains red only in the pre-existing Phase 9 `whiteboard-panel.test.tsx` RED contract (`teacherControls` and strict fetch mock signatures), which the pending frontend slice owns.

#### Phase 9 code-review remediation (2026-08-12)

- `[FIXED]` A matching freeze token whose lease expires after successful capture now takes the separate degraded status-first transition and returns durable archive-incomplete success.
  A genuinely foreign live token still makes that degraded transaction return stable `409 session_end_in_progress`, and token-conditional cleanup cannot clear it.
- `[FIXED]` Empty prepared canvas lists are initialized as non-nil slices, so the strict control wire sends `canvasIds: []`; the real HTTP regression proves it never sends `null`.
- `[FIXED]` A missing session now maps the canvas store's nil result to HTTP 404 instead of serializing `201 null`.
- `[FIXED]` Explicit and replacement post-commit schedule settlement uses fresh bounded contexts rather than the canceled request context.
  The mechanism tests cancel at the durable event boundary and prove schedule completion precedes asynchronous terminal cleanup; replacement retains schedule-before-event ordering.
- `[FIXED]` Control-origin comments and operator documentation now consistently permit canonical numeric IPv4 and IPv6 loopback while requiring verified HTTPS elsewhere.
- `[GREEN]` With configured and live database identity verified as `bridge_test`, the four focused review regressions passed in the handler and realtime packages.
  Terra also corrected the Phase 9 panel test fetch typings and the archive's approved `sessionId` option expectation; root TypeScript compilation passed before production remediation.

#### Phase 9 frontend settings cutover (2026-08-12)

- `[frontend]` The live teacher dashboard now explicitly enables whiteboard teacher controls.
  `WhiteboardPanel` fetches only the dedicated `/api/sessions/{sessionId}/canvas-settings` route when live teacher controls or archive status need it, treats 403 as absent controls/status, and strictly rejects malformed or extra successful response fields.
- `[frontend]` The live selector exposes only `private`, `host`, and `participants` and PATCHes exactly `{canvasFloor}` to the dedicated route.
  Archive mode renders the durable confirmed or incomplete message for authorized teachers, treats an omitted legacy value as no completeness claim, and never renders mutation controls.
- `[GREEN]` With both test database URLs pinned to `bridge_test`, the focused whiteboard panel/archive Vitest command passed 13 tests.
  Forced-Bun `tsc --noEmit`, exact changed-production ESLint, and `git diff --check` passed.

#### Phase 9 frontend identity review remediation (2026-08-12)

- `[RED]` Session-rerender tests proved a late settings response, a failed next-session request, or an outstanding PATCH could retain or publish another session's floor/archive state.
  Near-miss PATCH responses also proved the shared GET parser accepted the GET-only archive field.
- `[FIXED]` Settings state is tagged with its session identity and derived as empty whenever the rendered session differs.
  Each GET owns a cancellation guard, every success/403/failure publishes an identity-scoped complete state, and late prior-session continuations cannot surface or enable a PATCH against the current session.
- `[FIXED]` PATCH has a separate exact one-field parser and preserves the last safe current-session floor while rendering a mutation error for non-200, malformed, GET-shaped, or unknown-field responses.
- `[GREEN]` The expanded panel/archive suite passed 21 tests with both database URLs pinned to `bridge_test`; forced-Bun TypeScript compilation, exact production lint, and `git diff --check` passed.
- `[RED/FIXED]` A final reverse-order regression proved a session-A PATCH continuation could overwrite the single state slot after session B had loaded.
  PATCH success, failure, and malformed-response continuations now publish only when the current stored identity still matches their captured request session; otherwise they leave B's complete state untouched.
- `[GREEN]` The final panel/archive suite passed 24 tests; forced-Bun TypeScript compilation, exact panel lint, and `git diff --check` passed.
- `[REVIEW]` Sol's exact-head confirmation on `249125f666057092f2e7707f15af014c5411b693` returned APPROVE with no open Phase 9 finding.

#### Phase 9 review-remediation RED proofs (2026-08-12; tests only)

- `[GREEN]` The real Go control-client/`httptest` wire now proves an empty authoritative canvas list serializes exactly as `"canvasIds":[]`, never `null`, while the listener accepts the exact empty snapshots bundle.
- `[RED]` A missing UUID session canvas `POST` returns `201` with `null` instead of the required `404`.
  A successful fake freeze that expires its own matching lease before completion returns `409 session_end_in_progress` instead of ending the session with durable `whiteboardServerArchiveComplete: false` and the incomplete-archive warning.
- `[RED]` Explicit and replacement post-commit settlement tests cancel the request context at the handoff boundary and prove the current schedule completion still uses it.
  The explicit path emits only after the durable end, then leaves the linked schedule `in_progress` before asynchronous terminal complete; the replacement path emits before a cancelled-context schedule transition and likewise leaves the schedule `in_progress`.
- Both `DATABASE_URL` and `TEST_DATABASE_URL` were explicitly set to `postgresql://work@127.0.0.1:5432/bridge_test`, and `CHECK_TEST_DATABASE_URL=... node scripts/check-test-database-url.mjs` live-verified the target before the focused database test.
  `go test ./internal/realtime ./internal/handlers -run 'Test(CanvasControlClient_FreezeZeroCanvasIDsSendsAnEmptyArray|CanvasHandler_CreateCanvasMissingSessionReturns404|EndSession_MatchingLeaseExpiresAfterFreezeEndsDegraded|PostCommitSettlementUsesFreshContextForExplicitAndReplacementEnds)' -count=1 -timeout 120s` recorded the three expected RED mechanisms and the control-wire GREEN result.
- The archive regression now requires `sessionId` in `UseWhiteboardOptions` while preserving the no-unselected-token boundary and requiring the dedicated archive settings fetch.
  The panel test's fetch mocks now use the real `fetch` parameter types only; `node_modules/.bin/tsc --noEmit` passed.
- `[GREEN]` The real `WhiteboardArchive` consumer makes exactly its canvas-list and dedicated `canvas-settings` GETs on mount, never the old generic settings route, without minting a selected-document token.
  It renders the durable archive result as distinct confirmed, incomplete, and legacy-omitted states while retaining the archive's existing read-only interaction behavior.
  After `CHECK_TEST_DATABASE_URL=... node scripts/check-test-database-url.mjs` live-verified `bridge_test`, `PATH=/home/chris/.bun/bin:$PATH DATABASE_URL=... TEST_DATABASE_URL=... bunx --bun vitest run tests/unit/whiteboard-archive.test.tsx --config vitest.config.ts` passed 9 tests; `PATH=/home/chris/.bun/bin:$PATH bunx --bun tsc --noEmit` also passed.
- No production file was edited by this test-only remediation; the pre-existing frontend production edits remain unstaged for their owner.

#### Phase 9 settings state-isolation RED proofs (2026-08-12; tests only)

- `[RED]` Added live-panel settings regressions for an immediate session A-to-B rerender, reverse-order late A settings completion, a failed or forbidden B settings read, and ensuring an old A floor cannot remain actionable as a PATCH to B.
  The same test matrix requires a successful PATCH response to reject both the GET-only `whiteboardServerArchiveComplete` field and unknown fields, show the settings error, and retain the previously safe floor.
- `[RED]` With `DATABASE_URL` and `TEST_DATABASE_URL` explicitly pinned to `postgresql://work@127.0.0.1:5432/bridge_test`, `/home/chris/.bun/bin/bunx --bun vitest run tests/unit/whiteboard-panel.test.tsx tests/unit/whiteboard-archive.test.tsx` recorded 6 expected panel failures: stale floor, stale archive warning, late-A overwrite, B-read failure retention, stale-floor PATCH exposure, and acceptance of GET-only archive completion in a PATCH response.
  The paired 403 and unknown-PATCH-field cases already pass; the archive suite remains green (9 tests), so the command reports 15 passed and 6 failed tests overall.
- `[GREEN]` The same explicitly pinned environment completed `/home/chris/.bun/bin/bunx --bun tsc --noEmit` successfully after adding the RED tests.
- `[RED]` No production file changed in this test-only proof.

#### Phase 9 in-flight settings PATCH isolation RED proof (2026-08-12; test-only)

- `[RED]` Added a terminal-producer race: begin a session A floor PATCH, rerender session B, and first let B's dedicated settings GET render its floor control without an error.
  Resolving A afterward as a valid PATCH success, a non-OK failure, or a malformed success must leave B's floor control and error-free settings status intact.
- `[RED]` With both database URLs explicitly pinned to `postgresql://work@127.0.0.1:5432/bridge_test`, `/home/chris/.bun/bin/bunx --bun vitest run tests/unit/whiteboard-panel.test.tsx tests/unit/whiteboard-archive.test.tsx` reports exactly the three new expected failures: each late A terminal outcome clears B's already-rendered `Canvas floor` control.
  The prior panel regressions and all 9 archive tests pass, yielding 21 passed and 3 failed tests across the focused command.
- `[GREEN]` `/home/chris/.bun/bin/bunx --bun tsc --noEmit` and `git diff --check` passed with the same pinned test-database environment.
  No production file changed.

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

#### Phase 11 frontend production slice (2026-08-12; Sonnet; production only — tests owned by Terra)

- `[frontend]` Implemented the durable Excalidraw binding contract: `excalidraw-yjs.ts` now persists only a `DURABLE_APP_STATE_KEYS` allowlist (`viewBackgroundColor`) instead of the whole `appState`, so remote scene application never clobbers a viewer's local viewport/zoom/selection/tool/collaborator/view-mode state, and `writeExcalidrawScene` skips the Yjs transaction entirely when the newly serialized scene is byte-identical to the currently stored one.
  `use-whiteboard.ts` batches `onChange` through a 100 ms trailing debounce (cleared on unmount and on doc swap) instead of writing synchronously; remote-echo suppression via the existing local-origin symbol is unchanged.
  `excalidraw-board.tsx` rejects pasted images (`onPaste` returns `false` when `ClipboardData.files` is non-empty) and native file drag/drop (a capture-phase `onDragOverCapture`/`onDropCapture` wrapper calls `stopPropagation` before Excalidraw's own bubble-phase drop handler ever sees a `Files`-bearing drag).
- `[frontend]` `whiteboard-panel.tsx` now confirms every visibility raise through the existing `ConfirmDialog` component before PATCHing (loosen-only per Decision 7, so every accepted raise is irreversible from the UI) and consumes a one-shot `sessionStorage` fallback warning (`whiteboard-archive-fallback:{sessionId}`, consume-on-read) when the archive's durable `GET canvas-settings` fails; a 200 durable response — any value — clears the fallback since it supersedes it.
- `[frontend]` `teacher-dashboard.tsx`'s `endSession` now decodes the durable `POST /api/sessions/{id}/end` response: a non-2xx or network failure renders a `role="alert"` error and does not navigate; a decoded `whiteboardServerArchiveComplete === false` writes the one-shot archive fallback flag before redirecting to `/sessions/{id}/whiteboards`.
- `[frontend]` `start-session-button.tsx` decodes the create response's top-level `replacedSessions` and shows a one-time acknowledgement dialog naming how many prior live sessions were ended and whether their archives may be incomplete, before navigating; the existing 422 unlinked-topic confirmation flow is unchanged.
  New `src/components/teacher/scheduled-session-list.tsx` renders a class's `planned` scheduled sessions (`GET /api/classes/{classId}/schedule`) with a "Start now" action (`POST /api/schedule/{id}/start`) that decodes the same `replacedSessions` contract and shows the same warning before navigating; wired into `teacher/classes/[id]/page.tsx`.
- `[frontend]` Fixed the neutral `/sessions/{id}` page: `GetStudentPage` 404s for both a genuinely missing session (`"Not found"`) and an existing ended session (`"Session has ended"`) with the same HTTP status, and the prior code redirected both to the archive. It now redirects only the ended case (checked via `ApiError.body.error` or, for callers that only set `message`, a tolerant `/session (has )?ended/i` match) and calls `notFound()` for a genuinely missing session.
  Removed `classId`/`returnPath` from `student-session.tsx`'s `session_ended` `EventSource` effect dependency array — neither was read inside the effect body, so they were stale deps causing unnecessary EventSource reconnects.
- `[frontend]` Removed the shadow `PATCH` export from `src/app/api/sessions/[id]/route.ts` (GET-only now) and the live `endSession` / dead `createSession` helpers from `src/lib/sessions.ts`. `/api/sessions/:path*` is a `beforeFiles` rewrite to the Go backend (`next.config.ts`), so this shadow file was already unreachable in normal operation; removal is defense-in-depth against a rewrite-config regression silently reviving a TypeScript session-status writer that bypasses the Go canvas-lifecycle protocol entirely.
- `[GREEN]` With `DATABASE_URL`/`TEST_DATABASE_URL` pinned to `postgresql://work@127.0.0.1:5432/bridge_test` (live-verified via `scripts/check-test-database-url.mjs`): the focused root Vitest command (`whiteboard-panel`, `start-session-button`, `whiteboard-archive`, `sessions-room-page`, `shadow-routes`, `excalidraw-yjs`, `use-yjs-provider`) reports **6 files / 64 tests passing**, plus the source-local `src/lib/whiteboard/vitest.config.ts` suite reporting **3/4 tests passing**; `bunx --bun tsc --noEmit` is clean; ESLint on every changed/added file reports 0 errors (2 pre-existing-shape warnings — unused `classId`/`returnPath` props in `student-session.tsx`, now genuinely unused after the dependency-array fix removed their only remaining reference); `git diff --check` is clean.
- `[RED — expected, owned by Terra]` `tests/unit/excalidraw-yjs.test.ts` (3 tests) and `src/lib/whiteboard/use-whiteboard.test.tsx` (1 test) fail against the new production contract: the pre-Phase-11 tests assert the *old* contract (whole-`appState` persistence including `scrollX`; synchronous, undebounced writes) that Phase 11 deliberately supersedes (durable allowlist; 100 ms trailing debounce). These tests are explicitly named in Terra's Phase 11 scope ("Extend the existing... binding suites for... `viewBackgroundColor` plus the exact durable allowlist... 100-millisecond trailing scene writes") and are not owned by this frontend slice.
- `[GAP]` `tests/unit/scheduled-session-list.test.tsx` does not exist yet (not yet authored by Terra), so it cannot run; `scheduled-session-list.tsx` has no test coverage in this commit.
- `[GAP]` The plan also asks this phase to "add a production-source scan proving no TypeScript session-status writer remains" and to update `tests/unit/shadow-routes.test.ts` — both are test-authorship, owned by Terra; `tests/unit/shadow-routes.test.ts` needed no edit for this commit (the PATCH-export removal doesn't change its route-file census, so the existing forward/reverse allowlist checks already cover it) and `TODO.md` has no entry referencing the removed writers, so neither needed a change.
- Phase 11 is **not** claimed complete: the RED/GAP items above (owned by Terra per the plan's test/production split) remain outstanding, and Phase 11's own GREEN command list cannot fully run until `scheduled-session-list.test.tsx` exists.

#### Phase 11 Terra test-first completion (2026-08-12)

- `[RED]` The real `useWhiteboard` producer initially retargeted a pending canvas-A scene to the current map after a canvas/Y.Doc swap, and still wrote after a live board became read-only.
  The new cross-canvas and read-only transition regressions failed against that behavior.
  The hook now clears a queued scene and its timer on Y.Doc, canvas, session, read-only, and unmount transitions, so a trailing write is either delivered to its original active canvas or dropped; it can never cross into a replacement canvas.
- `[RED]` Phase-11 contract tests found that the class-mode `StartSessionButton` constructed, but did not render, its replacement warning dialog, and that an archive `403` discarded the one-shot degraded-completion fallback despite the contract requiring fallback retention for every non-200 settings result.
  Class mode now renders the existing replacement dialog, and archive settings consumes the fallback for 403 alongside all other non-200/network outcomes; a 200 remains authoritative and clears it.
- `[GREEN]` Added mechanism-facing coverage for owner/viewer create and visibility controls, irreversible-raise confirmation and failure, archive fallback consumption and durable override, end failure/no-navigation and durable-false fallback-before-navigation, exact one-shot replacement warnings and preserved 422 confirmation, scheduled discovery/start/auth/error/navigation, neutral missing-versus-ended routing, shadow session-writer source scans, durable Excalidraw allowlist/identical-write/remote-origin behavior, native image paste/drop rejection, and trailing-write cross-canvas/read-only cancellation.
- `[GREEN evidence]` After live validation with `CHECK_TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test` and both `DATABASE_URL` and `TEST_DATABASE_URL` pinned to that same test database, the Phase-11 focused root Vitest command passed **8 files / 86 tests** and the source-local whiteboard Vitest configuration passed **1 file / 6 tests**.
  `bunx --bun tsc --noEmit`, scoped ESLint, and `git diff --check` passed.
  No service, E2E, migration, non-test database, or live provider was run.

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

#### Phase 12 non-E2E integration evidence (2026-08-12; Terra)

- `[GREEN]` Audited the frozen matrix against existing mechanism-facing Go, Bun, and frontend contracts rather than adding duplicate broad-count tests.
  Renamed the matching decisive tests to the exact required lifecycle/replacement/freeze-auth/realtime/mutation/scheduled-start acceptance names.
  The cross-language advisory-key vectors are exercised by the Go transaction-lock test and the Bun `sessionLifecycleKey` vectors, including signed `int32` boundaries.
- `[GREEN]` Added `TestEndSession_DifferentExpiredTokenEndsDegradedClearsLeaseWithoutReusingFreeze`: a stale caller cannot reuse a different expired operation's capture result; it commits only the durable degraded result and clears the expired residue.
  Added `TestEndSession_DatabaseFailureLeavesLiveClearsLeaseAndEmitsNoEvent`: a database-injected confirmed-end failure leaves the session live, removes only the matching lease, requests matching unfreeze, and never emits `session_ended`.
  Added a real internal-realtime authorization wait proof: it blocks behind the two-`int4` exclusive session lifecycle advisory lock and resumes only after release.
- `[GREEN evidence]` The guarded `_test` database validator accepted `bridge_test` with both database variables pinned.
  Focused lifecycle/replacement/freeze/auth/control Go tests passed; package runs for `./internal/handlers`, `./internal/store`, and `./internal/realtime` passed with `-count=1 -timeout 120s`, followed by `go vet` on the same packages.
  `bun test server/canvas-lifecycle.test.ts server/hocuspocus.canvas.test.ts` passed **82 tests / 247 assertions**; the focused frontend/retry suite passed **6 files / 62 tests**; `bunx --bun tsc --noEmit`, scoped ESLint, and `git diff --check` passed.
  No migration, non-test database, service, E2E, or live provider ran.
- `[RED → GREEN]` The newly added E2E-control seam tests first failed to compile because the explicit opt-in config, parsed/live database authorization, and client injection did not exist.
  `BRIDGE_E2E_CANVAS_CONTROL_FAILURE=1` now authorizes only after the parsed URL database name and independent `SELECT current_database()` name each end in `_test`; the disabled case does not probe, parsed non-test names reject before probing, live non-test names reject after probing, and the injected `Freeze` failure makes no control-listener request.
  The focused realtime/config Go tests and `go vet ./internal/realtime ./internal/config ./cmd/api` passed with both database variables pinned to `bridge_test` and the validator accepted.
- `[GREEN — static live-stack artifact]` Added guarded `e2e/session-whiteboard.spec.ts` for teacher create, public-session outsider denial, irreversible visibility confirmation, floor raise, participant view, rendered scene propagation, explicit end, and archive read-only controls.
  It is suite-skipped without an explicit `E2E_BASE_URL`; its incomplete-archive assertion is separately skipped unless the exact E2E failure flag is enabled.
  Playwright was not invoked.
  The current shell has no `bun`/`bunx` executable, so the repository-local `node_modules/.bin/tsc --noEmit`, scoped ESLint, and `git diff --check` were used as static fallbacks and passed.
- `[UNVERIFIED — hard safeguard]` The live-stack E2E acceptance remains intentionally unrun.
  It requires user-provisioned `HOCUSPOCUS_CONTROL_SECRET`, any necessary collision-free control-port override, a separately started Bridge stack with explicit `E2E_BASE_URL`, and (for the degraded-archive branch) a start-time `BRIDGE_E2E_CANVAS_CONTROL_FAILURE=1` whose configured and live database names are both `_test`; this phase did not read or edit `.env`, start a service, or run Playwright.

#### Phase 12 demo E2E seed recovery (2026-08-31; Terra)

- `[RED → GREEN]` Replaced the initial fragment matcher in `scripts/tests/test-problem-demo-seed.sh` with a guarded executable fixture harness.
  The harness runs the seed twice and compares its selected fixture fingerprint: identity/provider/membership fields, chapter/document values, and JSON row values for organization, course, topic, problem, attachment, solution, test case, class, class-membership, and class-setting rows.
  It rejects the exercised near-miss copies for an inactive/wrong membership, invalid bcrypt, invalid current-schema column, removed conflict no-op, and a late transactional failure.
- `[FIXED]` `scripts/seed_problem_demo.sql` now creates Bridge Demo School plus Eve, Alice, Bob, Frank, and Diana with fixed UUIDs, active role memberships, email providers, and the authentication-compatible bcrypt hash for `bridge123`.
  `admin@e2e.test` and its email provider are inserted only where `current_database() ~ '_test$'`, so the known-password platform-admin fixture is absent from non-test targets.
  The obsolete `teaching_units`/`unit_documents` writes now target the migrated `chapters`/`chapter_documents` schema without changing the seeded chapter or class content.
- `[GREEN evidence]` Immediately before all database actions, the guarded validator accepted `postgresql://work@127.0.0.1:5432/bridge_test`.
  Before that live check, a subprocess passed a fake `psql` sentinel while parser-rejecting `postgresql://127.0.0.1:5432/bridge`; the sentinel remained absent, proving rejected targets cannot run `psql`, the seed, or a DML-capable EXIT cleanup.
  A deliberate mutation that called the sentinel instead produced the expected RED failure and was restored before the GREEN run.
  After validation, the executable harness clears the complete dependency-ordered fixed fixture graph, including topic-owned chapters/documents, and requires a zero census before every real/candidate seed; this prevents pre-existing correct rows from masking an omitted Eve provider, Alice bcrypt, Bob membership, or chapters.
  Candidate recovery clears and reseeds only the transformed ephemeral fixture graph; final EXIT cleanup clears that graph again and requires a zero census rather than reseeding it.
  Chapter ownership is resolved through the two fixed topic IDs throughout clear, census, verification, and fingerprinting, rather than assuming the seed's preferred chapter UUIDs.
  A regression installs two supported non-fixed chapter UUIDs and documents for those topics, proves the canonical seed reuses them, then proves the topic-owned clear reaches a zero census; mutating that clear back to fixed IDs produced the expected foreign-key RED and was restored.
  The seed now inserts `topics` using the current schema (without dropped `lesson_content`); a candidate restoring that column is required to fail.
  The non-test proof substitutes only `current_database()` in the actual predicate, so an `OR true` guard candidate creates the test-only admin rows and demonstrates that the normal assertion would reject the bypass.
  Each harness run transforms the seed and verifier into a unique ID/email/slug/join-code namespace before DML, so no pre-existing Bridge Demo School fixture is cleared.
  An unrelated sentinel organization, course, class, and memberships is asserted after fixture clearing and removed only by exact IDs; its join code is derived from the per-run transformed class ID, and ephemeral or sentinel-cleanup failures fail the command rather than being suppressed.
  Two consecutive guarded `bridge_test` harness runs completed with each EXIT cleanup reporting its zero internal census; a post-run query found no `+seed-` users or `bridge-demo-school-` organizations.
  Its non-test admin proof substitutes a non-test literal only inside the seed copy and executes it against `bridge_test`; it never connects to a non-test database.
  No migration, non-test database, service, E2E, provider, or environment-file read ran.
  `[FIXED — live E2E correction]` A user-provisioned live-stack RED showed `Start Live Session` navigates to `/teacher/sessions/{sessionId}`, while this spec waited for the legacy class-dashboard route.
  The wait and UUID extraction now match the canonical `/teacher/sessions/{uuid}` route, consistent with `StartSessionButton` integration/unit coverage; E2E was not rerun in this correction.
  `[FIXED — live E2E correction]` The subsequent live RED created the board but failed strict locator resolution because its title is intentionally present in both the board-list button and details text.
  A further live RED showed that button's accessible name includes its visibility badge (for example, `${boardTitle} private`), so the role-scoped assertion now matches the title non-exactly; E2E was not rerun in this correction.
  `[FIXED — live E2E correction]` After a successful visibility raise, non-exact `getByLabel("Visibility")` matched both the control and confirmation dialog, triggering strict-mode failure.
  The implicit label's text includes option descendants, so the live and archive visibility assertions now use the accessibility-tree contract: exact named `combobox "Visibility"`; the same source/accessibility audit made the existing board-title clicks role-scoped board-button locators plus exact `Canvas floor` labels. E2E was not rerun in this correction.
  `[FIXED — live E2E correction]` After the participant joined, the class page navigated to `/student/sessions/{sessionId}`, while this spec waited for a legacy class-nested route.
  The participant wait now matches the canonical exact `/student/sessions/${sessionId}` path; E2E was not rerun in this correction.
  `[RED — live defect regression]` The live stack showed a locally drawn canvas never reached the participant because the pinned provider rejected the canvas construction options: `delay: 250` was below its `minDelay: 1000`.
  Added a focused real-provider hook construction regression requiring canvas setup not to throw under the pinned provider; `bunx --bun vitest run tests/unit/use-yjs-provider.test.ts` reproduced two unhandled `delay: 250 < minDelay: 1000` rejections at provider construction, so it is intentionally RED until the production reconnect configuration satisfies that invariant.
  `[FIXED — live E2E correction]` After the provider correction, both canvas sockets connected and authenticated, and the stored Yjs scene contained the teacher rectangle, but the participant pixel poll sampled Excalidraw's topmost interactive canvas and incorrectly remained unchanged despite the rectangle being visibly rendered.
  `sceneInk` now requires exactly one `canvas.excalidraw__canvas.static` rendered scene surface before applying the existing center-region, cross-browser pixel-delta proof; E2E was not rerun in this correction.
  `[RED → GREEN — live E2E configuration]` Bun's shell loads the repository `.env`, but the `playwright` executable runs under Node and previously skipped the injection-only assertion unless its variables were manually shell-exported.
  `e2e/playwright.config.ts` now explicitly loads `.env` through a direct `dotenv` development dependency, which preserves pre-existing shell values and leaves the existing `http://localhost:3003` fallback only when neither source provides `E2E_BASE_URL`.
  A no-setup isolated Node-runtime config test first observed the default URL and absent failure flag from a synthetic `.env`, then passed for `.env` loading, shell precedence, and the no-`.env` fallback; no live E2E, service, database, or repository `.env` read occurred.
  `[GREEN — live E2E acceptance]` The durable-loader verification reran the isolated configuration test (1/1, setup 0), scoped ESLint, TypeScript, and diff checks successfully.
  The guarded `bridge_test` seed restore accepted its target and confirmed six fixture users; the E2E seed also creates its fixture class and enrollments only in `bridge_test`.
  The normal live-stack run at `eaf59c3` passed 11/11 applicable tests in 20.2 seconds, with only the intentionally disabled failure-injection assertion skipped.
  The persistent-`.env` degraded run passed 12/12 in 15.6 seconds, including the injected-control incomplete-archive warning, without temporary exports for `E2E_BASE_URL`, the control-failure flag, Hocuspocus control secret, or database URLs; only the five external-provider keys were blanked for that command.
  Afterward the failure flag was restored to `0` with mode `0600`, and the owned stack stopped gracefully.
  No non-test database, migration, or external provider was used.
  `[RED → GREEN — full-gate fixture recovery]` The full local gate ran destructive Vitest and Go suites before Playwright, deleting the canonical `bridge_test` demo users, course, and memberships required by E2E authentication, so no clean full-gate run could reach Playwright reproducibly.
  `ci-local.sh` now skips recovery in `--fast`, loads only `E2E_BASE_URL` from persistent `.env` through dotenv when the shell has no explicit value, fails closed when still unpinned, and—only in the full pinned branch after the parsed/live `_test` gate validation—runs the canonical idempotent seed through `GATE_DATABASE_URL` before E2E.
  The recovery failure blocks Playwright, and the E2E command pins both database variables and blanks all five provider keys.
  Mocked governance selftests first failed against the absent branch, then proved fast no-op, persistent-URL recovery ordering, shell precedence, missing-URL refusal, restore-failure blocking, validated-URL-only seed invocation, validation-before-restore ordering, and provider-key isolation; no database, E2E, service, migration, or `.env` read was performed for this change.

### Phase 13 — Documentation, cross-phase verification, and shipping evidence *(orchestrator)*

- Update `docs/api.md`, `docs/architecture/decisions.md`, `docs/testing.md`, `docs/setup.md`, `docs/project-structure.md`, `.env.example`, and `README.md` for status-first lifecycle semantics, confirmed/degraded guarantees, control transport/config, replacement warnings, admission bounds, single-Hocuspocus limitation, no independent administrator/impersonator bypass for private canvases, and operator behavior; verify the Phase-9 config docs remain synchronized with the final implementation.
- Audit Spec 013 requirement-to-test coverage, scan the frozen scope and production session-status writers, and reconcile the plan's historical `[OPEN]` code-review findings only with verified implementation evidence.
- After Phase 14 lands, run focused TypeScript/Bun and Go suites first, then the complete `bash scripts/ci-local.sh` on the exact commit intended for review.
  The merge gate remains incomplete until the full command—including explicitly pinned E2E—is green and its attestation names that commit.
- Run the Tier-A code-review gate against the consolidated exact commit, resolve every `[OPEN]` finding to reviewer confirmation, write the post-execution report, update `TODO.md`, and run `bash scripts/pre-merge-guard.sh` before creating the PR.
  Create the draft PR, rerun `bash scripts/pre-merge-guard.sh --pr <number>` against its simulated merge state, rerun any invalidated exact-commit evidence, and squash-merge without `--admin` only after all local evidence is current.

### Phase 14 — E2E stack attestation *(Terra backend + Sonnet route; tests by Opus; gate script by the orchestrator)*

Phase 14 runs after the Phase 13 work recorded so far and **before** Phase 13's closing items.
Phase 13's cross-phase verification, documentation sweep, full pinned-E2E gate, code-review confirmation, and plan-wide report are (re-)run after Phase 14 lands, because a report written earlier would describe a gate that no longer exists.

**Problem (Plan-wide Review 2, R2-1).** `ci-local.sh` proves that *its own* `GATE_DATABASE_URL` is a live `_test` database, but the E2E tier drives a separately running stack selected by `E2E_BASE_URL`, and pinning `DATABASE_URL` for Playwright does not reconfigure a running service.
`e2e/seed.setup.ts` creates classes, enrolls users, and ends live sessions, and `e2e/session-whiteboard.spec.ts` writes Yjs canvas documents, so a stack pointed at development or production data would be mutated while the gate reports green.
Phase 13 made this easier to reach unattended by adopting `E2E_BASE_URL` from `.env`.

**Design.** The gate accepts a stack only when every database-holding service that E2E traffic actually reaches proves, live and per request, that its own pool sees **shared state the gate created in the validated database**.

- *Proof: a gate-held advisory lock, not metadata.*
  The verifier opens a postgres-js client through the already parsed-and-live-validated `GATE_DATABASE_URL` with `max: 1`, **reserves that single connection** (`sql.reserve()`), opens a transaction on it, and takes the **one-key** `pg_advisory_xact_lock(bigint)`; every later statement, including the final `ROLLBACK`, runs on that same reserved backend.
  The transaction-scoped form is released by rollback **or** disconnect, so a crashed verifier, a reaped backend, or a transaction-mode pooler can never strand it, and no explicit unlock exists to land on the wrong backend (`[codex]` BLOCKER, `[opus]` CONCERN).
  After acquiring, the verifier re-reads `pg_locks` on the reserved connection and aborts unless its own lock is granted there.
  The whole hold is bounded by one total deadline (30 seconds) on top of the per-fetch timeouts, and the verifier re-reads `pg_locks` on the reserved connection **again after sampling**: if its own lock is gone — a reaped backend, a pooler, or a server that sets `idle_in_transaction_session_timeout` — it reports the distinct class *lock lost* instead of blaming a stack that correctly saw no lock.
  *Key namespace.* The key is `(E2E_STACK_LOCK_CLASS << 32) | k`, where `E2E_STACK_LOCK_CLASS = 0x42523245` is a new reserved constant documented beside `sessionLifecycleLockClass` (`0x42524447`) and `classReplacementLockClass` (`0x4252434c`) in `platform/internal/store/session_lifecycle.go`, and `k` is the first eight hex digits of `sha256(nonce)` masked to 31 bits.
  In `pg_locks` a one-key lock has `objsubid = 1`, `classid` = the high word, and `objid` = the low word, so it is structurally disjoint from Bridge's two-key lifecycle locks (`objsubid = 2`), and its non-zero reserved high word is disjoint from the one-key `pg_advisory_xact_lock(hashtext(…))` in `store/sessions.go`, whose `int4` argument always widens to a high word of `0` or `0xFFFFFFFF`.
  The gate's lock therefore can neither block a real lifecycle transaction nor be impersonated by one.
  Each service, on **its own** pool, runs one bounded query that returns `current_database()` and whether a granted advisory lock with exactly that `classid`, `objid`, and `objsubid = 1` exists in `pg_locks` for the current database.
  A service attests only if it sees the lock; its response is `{"fingerprint": sha256(nonce ‖ 0x00 ‖ current_database()), "instance": <per-process id>}`.
  An earlier draft fingerprinted `current_database()` plus `pg_control_system()`'s `system_identifier`; `[opus]` correctly objected that a `pg_basebackup` clone or promoted standby shares that identifier, that it proves metadata identity rather than write destination, and that the function is superuser-restricted on stock and managed PostgreSQL.
  Advisory locks are live, per-database, per-cluster, non-replicated session state visible to any role through `pg_locks`: a clone, a standby, or another cluster holding its own `bridge_test` cannot show the gate's lock.
  No DDL, table, or migration is involved — deliberately, because `drizzle.config.ts` has no test-only migration path and a proof table would reach production.
  The observe side was probed on `bridge_test` on 2026-09-21 with the two-key form (seen while held, unseen for another key, unseen after release); the one-key, transaction-scoped form is asserted by the named tests below rather than assumed.
- *Single-instance topology — required and checked.*
  One attested request per origin proves nothing about a second backend behind the same origin (`[codex]` BLOCKER): a load balancer could route attestation to test-connected instances and later E2E traffic to one that is not.
  The E2E stack is therefore **defined** as exactly one Next.js, one Go, and one Hocuspocus process, each reached without load balancing; a reverse proxy that fronts a single upstream (the nginx hop on the primary development machine) is allowed.
  Every service generates a random `instance` id once per **process**, not per module evaluation, and returns it with each attestation: Go holds it in a package-level `sync.Once` and includes `os.Getpid()`; the two TypeScript services cache it on `globalThis`, so a Next.js dev recompile or fast refresh that re-evaluates the route module does not mint a new id.
  Live-reload supervisors (`air`, file-watch restarts) must be off for the E2E stack, because a process restarted mid-check is, correctly, a different instance.
  The verifier samples each origin five times on fresh connections (`Connection: close`, no keep-alive reuse) and fails with the class *multiple instances* unless the id is constant per origin.
  A subprocess cannot export into its parent, so the verifier CLI prints one machine-readable line, which `ci-local.sh` captures and passes to the Playwright child as `E2E_STACK_INSTANCE_GO`, `E2E_STACK_INSTANCE_NEXT`, and `E2E_STACK_INSTANCE_HOCUSPOCUS` in the same `env` block as the `E2E_BASE_URL` fix; `seed.setup.ts` fails unless its own samples return those three.
  Sampling detects heterogeneous routing with high probability but cannot prove its absence, so heterogeneous routing and a mid-run restart are both recorded as **unsafe and unsupported**, not as accepted risk; the topology requirement is stated in `AGENTS.md` and `docs/testing.md`.
- *Exact connection string.* Each service parses **the exact string its own pool was built from** — `cfg.Database.URL` for Go (which may come from TOML, not the environment), `process.env.DATABASE_URL` as read by `src/lib/db/index.ts` and `server/db.ts` — and requires that parsed name **and** the live `current_database()` to end in `_test`, the dual-proof shape of `NewE2ECanvasControlFailureInjection`.
  The Go query runs on the `*db.DB` the handlers already receive and Hocuspocus uses the existing `serverDb`; no new file under `platform/internal/db/` and no change to `server/db.ts` or `src/lib/db/index.ts` is needed.
  Those two TypeScript modules read `process.env.DATABASE_URL` at module load and export neither the string nor the client, so the TypeScript attestations re-read the same variable; a regression asserts they read exactly the variable name those modules use, so a future change of source cannot silently diverge.
- *Opt-in and non-disclosure.* `BRIDGE_E2E_STACK=1` (exactly `1`) exposes the attestation.
  In Go and Next.js, flag unset, a malformed nonce, a non-`_test` parsed or live name, or an unseen lock **all answer a byte-identical 404**, with no query run in the first two cases, so within those services the flag is indistinguishable from absent and nothing about the database is disclosed.
  Hocuspocus is different because its server answers every unhandled HTTP request with `200 text/plain` (`[opus]` CONCERN): with the flag unset the `onRequest` hook is **not registered at all**, and with it set every refusal **declines to handle the request so it falls through to that same default response**; on that port a refusal is thus byte-identical to the path not existing, and the verifier treats anything other than the attestation JSON as *not attested*.
  Refusal reasons are logged server-side at most once per ten seconds per reason, never with the URL, and that logging is covered by a named test.
  Responses carry `Cache-Control: no-store`, and the Next.js route exports `dynamic = "force-dynamic"`.
  The Go API refuses to start with the flag under `APP_ENV=production`.
  Under `BRIDGE_HOST_EXPOSURE=exposed` the Go API and Hocuspocus — the two services that already read that declaration at boot — refuse to start with the flag unless `ALLOW_E2E_STACK_OVER_TUNNEL=true` (exactly `true`) is also set, mirroring `ALLOW_DEV_AUTH_OVER_TUNNEL`.
  The surface grants nothing and discloses only a salted hash for a `_test` database, but `[codex]` is right that on an exposed host any anonymous caller could drive one database query per well-formed nonce; an explicit second opt-in keeps the nginx-fronted development machine able to run E2E while making that exposure a recorded choice rather than a default.
  Next.js has no start-time hook, but `BRIDGE_HOST_EXPOSURE` is readable at request time, so its route answers the same 404 under `exposed` without `ALLOW_E2E_STACK_OVER_TUNNEL=true`, restoring symmetry on the internet-facing process.
  Both boot refusals name `ALLOW_E2E_STACK_OVER_TUNNEL` in their error message.
- *Surfaces — every service is attested on the origin E2E traffic actually uses.*
  Go: `GET /api/health/e2e-stack?nonce=…`, fetched **through `E2E_BASE_URL`** and therefore through the same `/api/health/:path*` Next.js proxy hop and the same Go instance that serves E2E API traffic.
  Next.js: `GET /api/e2e-stack?nonce=…`, a route outside every `GO_PROXY_ROUTES` prefix, attests the legacy TypeScript API's `db` and additionally returns `realtimeUrl`, the `NEXT_PUBLIC_HOCUSPOCUS_URL` value baked into the client bundle.
  The route must reference the literal `process.env.NEXT_PUBLIC_HOCUSPOCUS_URL`, because Next.js inlines `NEXT_PUBLIC_*` at build time only where the literal appears; a destructured or dynamic read would return the runtime environment and reintroduce the build-versus-runtime split.
  A set `NEXT_PUBLIC_HOCUSPOCUS_URL` is an explicit E2E prerequisite: when it is unset the client falls back to `ws://<host>:4000` (`src/lib/yjs/use-yjs-provider.ts`, `src/components/session/student-tile.tsx`), which on the primary development machine is a foreign service, so the route reports no `realtimeUrl` and the verifier blocks.
  Hocuspocus: `GET /e2e-stack?nonce=…` served by the `onRequest` hook **on the client-facing websocket port**; the verifier derives its HTTP origin from the `realtimeUrl` Next.js reported (`ws→http`, `wss→https`).
  `[opus]` showed the earlier draft, which asked Go to relay Hocuspocus's answer over the control listener, could attest one Hocuspocus process while browsers write canvas documents into another; attesting the origin the browser is handed closes that.
  The control listener is not used for attestation.
- *Gate.* New `scripts/check-e2e-stack.mjs` exports `verifyE2EStack()` and is called by `run_e2e_gate` after the URL is resolved and **before** the demo-seed restore and Playwright.
  It prints the adopted `E2E_BASE_URL` and the derived realtime origin, takes the lock, fetches the three endpoints with bounded timeouts and `redirect: "error"`, requires all three fingerprints to equal its own, and releases the lock in a `finally`.
  On failure it prints, **per service**, the failure class — not attested, mismatch, multiple instances, no realtime origin, lock lost, unreachable, redirect, or timeout — plus a class-specific remediation line (“start all three services with `BRIDGE_E2E_STACK=1` against the gate's `_test` database, or point `E2E_BASE_URL` at such a stack”; for Hocuspocus it also prints the realtime origin it derived and that `NEXT_PUBLIC_HOCUSPOCUS_URL` is baked at build time; *no realtime origin* names `NEXT_PUBLIC_HOCUSPOCUS_URL` and its build-time baking; *multiple instances* says to run exactly one process per service with live reload off, or that a service restarted mid-check; *not attested* points at each service's rate-limited refusal log, which says whether the flag, the database name, or the lock was the reason), records a named gate failure, removes a stale attestation, and returns before the seed or Playwright run.
  `E2E_BASE_URL` is exported to the child (the `[opus]` nit).
  The module is import-safe: it performs no work at import time and guards its CLI entry, so Playwright collecting `seed.setup.ts` never opens a connection.
  No field is added to `.claude/ci-local-attestation.json`: a non-fast pass already implies the attestation passed, because a failed attestation fails the gate and deletes the file.
  An earlier draft wrongly said `pre-merge-guard.sh` enforces a non-fast, clean, exact-commit run; `[codex]` verified it never reads the attestation file.
  The real enforcement seams are `ci-local.sh` itself (which writes or deletes the file), the `br-autopilot` merge guard (which requires `fast: false`), and `docs/development-workflow.md` Step 6.8 — none of which change in this phase.
  The `pre-push` hook is **not** such a seam for E2E: it degrades to `--fast` unless `E2E_BASE_URL` is exported into the pushing shell, which is the normal case now that the value lives in `.env`; `.githooks/` is out of scope here, so this is stated rather than changed.
- *Time-of-check to time-of-use.* `e2e/seed.setup.ts` calls the same `verifyE2EStack()` as its first step with a **fresh** nonce and its own lock, through the `DATABASE_URL` the gate pins into the Playwright child, before its first mutating request; reusing the gate's nonce would let an intermediary cache replay the first answer.
  Playwright's project dependency chain (`seed` → `auth-setup` → tests) blocks every mutating project on a failed re-check.
  The residual window — a stack restarted against another database after `seed.setup.ts` and before a later spec — is accepted: it requires a deliberate mid-run restart, and `auth.setup.ts` then fails closed because the demo identities were seeded only into the gate's database.
- *Who starts the stack — unchanged.* `AGENTS.md` “never kill a process” is unconditional, and a plan cannot relax it.
  The stack is still started separately by the user, now with `BRIDGE_E2E_STACK=1`; the orchestrator does not start, stop, or signal services and does not read `.env`.
  An attestation failure is a pause, never something to work around.

**Out of scope for this phase.** Attesting non-database dependencies (object storage, LLM providers — the gate already blanks all five provider keys); any production health surface; agent-managed service lifecycle; load-balanced or multi-instance E2E topologies.
The attestation endpoints are intentionally session-less and tenant-independent, so the `AGENTS.md` endpoint-test axes “auth check” and “cross-user isolation” do not apply to them and are **explicitly exempt**; their place is taken by the flag-gating, refusal-uniformity, and cross-database isolation tests below.

**Integration tests (NAMED — required).**
Go: `TestE2EStack_DisabledReturns404AndRunsNoQuery`, `TestE2EStack_MalformedNonceReturns404AndRunsNoQuery`, `TestE2EStack_NonTestParsedNameReturns404`, `TestE2EStack_NonTestLiveNameReturns404`, `TestE2EStack_UnseenLockReturns404`, `TestE2EStack_AttestsWhenGateLockHeld` (takes the lock on a second connection to the live `_test` database), `TestE2EStack_LockInAnotherDatabaseIsNotSeen`, `TestE2EStack_AllRefusalsAreByteIdentical`, `TestE2EStack_NoStoreHeader`, `TestE2EStack_NeverLogsOrReturnsDatabaseURL`, `TestE2EStack_RefusalReasonIsLoggedAndRateLimited`, `TestE2EStack_InstanceIDIsStablePerProcess`, `TestE2EStack_LifecycleLockIsNotMistakenForGateLock` (a held two-key lifecycle lock and a one-key `hashtext` lock with the same low word are both unseen), `TestE2EStackLockClass_DisjointFromLifecycleClasses`, `TestE2EStackFlag_RefusedInProduction` and `TestE2EStackFlag_RefusedOnExposedHostWithoutOptIn` (in `platform/cmd/api/main_test.go`, beside the `DEV_SKIP_AUTH` guard tests, since that is where the exposure logic lives), and `TestConfig_E2EStackUsesPoolConnectionStringNotEnvironment`.
Bun (`server/e2e-stack.test.ts`): `e2e stack attestation is absent without the flag`, `e2e stack attestation is reachable on the client-facing realtime origin`, `e2e stack attestation refuses a non-test live database`, `e2e stack attestation refuses an unseen lock`, `e2e stack hook is not registered without the flag`, `e2e stack refusals fall through to the default unhandled response`, `e2e stack refuses to boot on an exposed host without the opt-in`, `e2e stack refusal logging is rate limited and omits the URL`, `e2e stack attestation is no-store`, `e2e stack instance id survives module re-evaluation`, `e2e stack reads the same DATABASE_URL variable as server/db`, `e2e stack key derivation and fingerprint match the shared vector`, and every other HTTP path on that port keeps its existing behaviour.
Vitest (`tests/unit/e2e-stack-route.test.ts`): `e2e-stack route returns 404 without the flag`, `e2e-stack route refuses a non-test database`, `e2e-stack route refuses an unseen lock`, `e2e-stack route reports the baked realtime URL`, `e2e-stack route reports no realtime URL when it is unset`, `e2e-stack route value equals the provider hook's realtime URL`, `e2e-stack route is force-dynamic and no-store`, `e2e-stack route refusals are byte-identical including a malformed nonce`, `e2e-stack route refuses on an exposed host without the opt-in`, `e2e-stack route refusal logging is rate limited and omits the URL`, `e2e-stack route instance id survives module re-evaluation`, `e2e-stack route reads the same DATABASE_URL variable as src/lib/db`, `e2e-stack route matches the shared vector`; plus `seed setup aborts before its first mutation when attestation fails` and `seed setup uses a fresh nonce` against a mocked verifier, and the shadow-route census still passes.
One shared fixture file, **`scripts/tests/e2e-stack-vector.json`** (nonce → `bigint` key, `classid`, `objid`; nonce + database name → fingerprint hex, including nonces whose low word lands on `0` and `2^31-1`), is asserted by Go, Bun, Vitest, and the gate selftests so the four implementations cannot drift.
Verifier regressions against the live `_test` database (`scripts/tests/check-e2e-stack.live.test.mjs`, **appended to the explicit `bun test` file list in `package.json`'s `test` script** — `vitest.config.ts` includes only `tests/**`, so without that line the file would silently never run — and therefore reached by `ci-local.sh`'s `bun run test` with the pinned `_test` URL, never against another target): the lock and every later statement run on one reserved backend (`pg_backend_pid()` is constant); the lock is gone after rollback; the lock is gone after the verifier's connection is destroyed without rollback; the verifier aborts when its own lock is not visible; a transaction killed mid-check is reported as *lock lost*; the total-hold deadline fires.
Gate selftests in `scripts/tests/test-guards.sh` (mocked database and `fetch`; no network, no database): mismatch blocks seed and Playwright; a changing `instance` id on any origin blocks as *multiple instances*; `seed.setup.ts` instance ids differing from the gate's block; each of 404, unreachable, redirect, and timeout blocks and is reported per service with the remediation line; a Hocuspocus origin is derived from the Next.js-reported `realtimeUrl`, never from configuration; a missing or non-websocket `realtimeUrl` blocks as *no realtime origin* with its own remediation line; each failure class prints its class-specific remediation; the derived realtime origin is printed; the three instance ids reach the child environment under their named variables; `--fast` performs no attestation and takes no lock; success orders attestation → seed → Playwright; the transaction is rolled back on success and on every failure path; importing the module opens no connection; the target URL is printed; `E2E_BASE_URL` reaches the child environment; a failure removes a stale attestation.

**Documentation.** `AGENTS.md` “Processes and ports” (correct the stale “defaults to `http://localhost:3003`” sentence and state that a full gate requires an attested, single-instance stack — the “never kill a process” rule is untouched) and its “Governance docs” list (add `scripts/check-e2e-stack.mjs`, which becomes load-bearing gate code alongside `check-test-database-url.mjs`), `docs/testing.md` “The E2E hazard”, `docs/setup.md`, `docs/project-structure.md`, `.env.example` (`BRIDGE_E2E_STACK`, `ALLOW_E2E_STACK_OVER_TUNNEL`), `decisions.md` §9, `docs/api.md` (the three flag-gated, test-only surfaces, marked as not part of the product API), and `README.md` (its “E2E tests … requires all services running” line gains the flag, the single-instance rule, and the tunnel opt-in).

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
| The E2E tier mutates a stack that is not connected to the validated test database (R2-1). | Phase 14: every service proves on its own pool that it sees an advisory lock the gate holds in the validated database — live shared state a clone, standby, or second cluster cannot show — on the origin E2E traffic really uses (Go through the Next.js proxy, Hocuspocus on the client-facing realtime origin Next.js bakes in); verified by the gate before the seed and again by `seed.setup.ts` with a fresh nonce before its first mutation. The stack is defined as single-instance and sampled per origin for a constant process id; load-balanced routing and mid-run restarts are unsafe and unsupported. |
| The attestation surface itself becomes an information leak or a production footgun. | Go and Next.js refusals are byte-identical 404s and Hocuspocus refusals are identical to its default unhandled `200` response, so on every port the flag is indistinguishable from absent; `no-store`; exposed hosts need a second explicit opt-in; discloses only a salted hash for a `_test` database; Go refuses the flag under `APP_ENV=production`; never logs or returns the database URL. |
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

### Phase 14 revision — plan gate Round 1 (2026-09-21) — commit `37e21af`

- **Verdicts:** `[glm]` APPROVE WITH NITS; `[opus]` CHANGES REQUESTED (2 blockers); `[codex]` and `[claude-self]` not yet recorded — Codex was occupied with an implementation task, and the revision below is material, so all four slots run on the revised commit.
- `[ADDRESSED]` `[opus]` BLOCKER — the Hocuspocus attested through Go's control listener need not be the one browsers write canvas documents into (`NEXT_PUBLIC_HOCUSPOCUS_URL` is a separate, build-time value). Hocuspocus is now attested directly on the client-facing origin Next.js reports; the control listener is no longer used.
- `[ADDRESSED]` `[opus]` BLOCKER — the “Who starts the stack” paragraph relaxed the unconditional `AGENTS.md` “never kill a process” safeguard without authority. Deleted; the stack remains user-started and the orchestrator neither starts nor stops services.
- `[ADDRESSED]` `[opus]` CONCERN — the `system_identifier` fingerprint proved metadata identity, not write destination, and survives clones and standbys. Replaced by a gate-held advisory lock each service must observe through its own pool. `[opus]` proposed a dedicated `_test`-only table; an advisory lock gives the same live shared-state proof with no DDL, which matters because Bridge has no test-only migration path. This also removes the `pg_control_system()` privilege dependency (`[opus]` NIT 10).
- `[ADDRESSED]` `[opus]` CONCERN — Phase 14 now follows Phase 13 in the file and states that Phase 13's closing verification re-runs after it.
- `[ADDRESSED — partly declined]` `[opus]` CONCERN — every refusal is now an identical 404 and Go refuses the flag in production. The `BRIDGE_HOST_EXPOSURE=exposed` axis is deliberately **not** mirrored; the reasoning is recorded in the design for the reviewer to accept or dispute.
- `[ADDRESSED]` `[opus]` CONCERNS — each service parses the exact string given to its pool constructor; `seed.setup.ts` uses a fresh nonce; `no-store` and `force-dynamic` are required and tested; the residual TOCTOU window is stated; scope wording corrected.
- `[ADDRESSED]` `[glm]` — per-service failure class and remediation line; named tests for the seed re-check; the shared vector is a named, scoped file; `docs/api.md` added; the Go-only production refusal is explained. `[glm]`'s process run-file suggestion is moot because the orchestrator no longer manages processes.

### Phase 14 revision — plan gate Round 2 (2026-09-21) — commit `a04cb5b`

- **Verdicts:** `[opus]` APPROVE WITH NITS (all eleven Round-1 findings RESOLVED; the declined host-exposure refusal accepted on its merits); `[codex]` CHANGES REQUESTED (2 blockers) — R2-1 “not resolved as written; the shared-state concept is sufficient once findings 1–2 are closed”; `[glm]` **no valid verdict** — the forwarder reported that the returned text contained no verdict line and did not discuss the diff, reading instead like a summary of unrelated orchestration state that merely asserted an approval. It is discarded, not counted, and GLM is re-dispatched with a session reset; `[claude-self]` CHANGES REQUESTED (key namespace).
- `[ADDRESSED]` `[codex]` BLOCKER + `[opus]` CONCERN + `[claude-self]` — lock ownership and namespace. One reserved postgres-js connection holds an open transaction and the **one-key** `pg_advisory_xact_lock(bigint)`; rollback or disconnect always releases it. `[codex]`'s one-key form (`objsubid = 1`) is adopted over the orchestrator's reserved-two-key-class idea because it is structurally disjoint from the lifecycle locks; a reserved non-zero high word additionally keeps it clear of the existing one-key `hashtext` lock in `store/sessions.go`, which `[codex]`'s suggestion alone would not.
- `[ADDRESSED]` `[codex]` BLOCKER — one request per origin does not prove every backend. Single-instance topology is now a stated requirement, checked by sampling a per-process instance id and re-checked by `seed.setup.ts`; heterogeneous routing is recorded as unsafe and unsupported, not accepted.
- `[ADDRESSED]` `[codex]` CONCERN — exposed hosts need `ALLOW_E2E_STACK_OVER_TUNNEL=true`; refusal logging is rate-limited. This revises the Round-1 decline that `[opus]` had accepted, so `[opus]` re-confirms.
- `[ADDRESSED]` `[codex]` CONCERN — the `pre-merge-guard.sh` rationale was factually wrong; the real enforcement seams are named and the unused attestation field is dropped.
- `[ADDRESSED]` `[opus]` CONCERNS — Hocuspocus refusals fall through to its default `200` and the hook is unregistered without the flag; the route reads the literal `NEXT_PUBLIC_HOCUSPOCUS_URL`, a set value is an E2E prerequisite, and a Vitest asserts equality with the provider hook.
- `[ADDRESSED]` NITs — `check-e2e-stack.mjs` joins the governance list; the auth and cross-user-isolation axes are explicitly exempt; the stale “Phases 8 through 13” line and Phase 13's ordering are fixed; the module is import-safe; the scope-audit wording records drift rather than erasing it.

### Phase 14 revision — plan gate Round 3 (2026-09-21) — commit `6ca8d47` — **CONSENSUS, gate passed**

- **Verdicts on the same exact commit:** `[codex]` APPROVE WITH NITS (F1–F5 RESOLVED; “R2-1 would now be RESOLVED by the design as written”); `[opus]` APPROVE WITH NITS (C1–C4 and N5–N8 RESOLVED; the namespace argument verified against `sessions.go:166` and PostgreSQL's `int4`→`bigint` sign extension; the Hocuspocus fall-through verified against the library source; “No blockers”); `[glm]` APPROVE WITH NITS (output validated by the forwarder as a genuine review: verdict line present, plan text quoted); `[claude-self]` APPROVE — its one finding, the key namespace, is resolved in this commit.
- No reviewer holds an open blocker, so the plan-review gate **passes at `6ca8d47`** and authorizes Phase 14 implementation.
- This was the third round; it converged, so no non-convergence checkpoint is owed.
- **Nits folded in the following commit, none changing the design:** process-scoped instance ids that survive module re-evaluation, with live reload off (`[opus]`, `[glm]`); a concrete transport for the instance ids from the verifier CLI through `ci-local.sh` to Playwright (`[codex]`, `[opus]`); a total-hold deadline plus a post-sampling lock re-read with a distinct *lock lost* class (`[opus]`); a *no realtime origin* class and class-specific remediation lines (`[glm]`); the live verifier test named in `package.json`'s explicit `bun test` list, without which it would never run (`[opus]`); the `pre-push` hook removed from the list of E2E enforcement seams (`[opus]`); a request-time exposed-host refusal for Next.js (`[opus]`); refusal-logging, `no-store`, byte-identity, and same-variable regressions for the TypeScript services (`[codex]`, `[glm]`, `[opus]`); `platform/cmd/api/main_test.go` and `README.md` added to scope and docs (`[opus]`, `[glm]`); the risk-table sentence that contradicted the Hocuspocus `200` fall-through corrected (`[codex]`).

### Phase 14 revision — plan gate pending (2026-09-21)

Plan-wide Review 2 finding R2-1 (`[codex]` Must Fix, `[opus]` Should Fix) requires a design the approved plan did not contain, and the user chose to build it inside Plan 094.
This revision adds Phase 14, its File scope, its named integration tests, two risk rows, and a scope-audit correction listing five already-changed files.
It is a material revision, so it had to clear the Tier-A plan-review gate at its exact commit before any Phase 14 code was written — it did, at `6ca8d47` in Round 3 above; the unrelated Review 2 fixes (R2-2 through R2-10) stay inside the previously approved scope and proceed independently.

## Code Review

### Phase 6 task reviews (2026-08-10)

- The database-validator/governance task reached spec and quality consensus after the validator canonicalized encoded test suffixes, rejected fragments and routing controls, bounded and destroyed its one-shot socket, and kept the executable guard contract aligned with `AGENTS.md` and `ci-local.sh`.
- The handler-fixture task reached spec and quality consensus after replacing all 31 ordinary cost-10 setup registrations, adding producer-parity and fail-closed URL-routing regressions, and preserving the dedicated real-registration tests.
- The store/contract task reached spec and quality consensus after validating effective pgx routing, preserving only same-host/same-port TLS fallback behavior, enforcing validator AST parity, and propagating cleanup errors.
- The Vitest task first received `[OPEN]` quality feedback that its error-identity assertion imported both producer and class from the same module and did not exercise the source-local runner.
  Commit `74adf8f` `[FIXED]` the finding by importing and executing the real signup-intent route, checking the root-produced error through the supported `zod/v4` entry point, and adding the same boundary to the standalone whiteboard suite.
  The original reviewer and an independent quality arbiter then approved; the repository has no production dependency that exposes a second physical Zod error producer, so an artificial nested-package fixture is not part of the application contract.

### Phase 13 local-gate remediation (2026-08-31; reviewer confirmation pending)

- `[FIXED] [quality]` `e2e/playwright.config.ts` no longer defaults a missing E2E target to port 3003.
  It loads dotenv first, then rejects configuration evaluation unless a persistent or explicit-shell `E2E_BASE_URL` exists; the isolated Node-runtime regression uses a nonsecret `DOTENV_CONFIG_PATH` fixture to prove dotenv loading, shell precedence, and the absent-value rejection, while the earlier live E2E run proved the default repository `.env` path.
- `[FIXED] [quality]` `scripts/check-test-database-url.mjs` now rejects every decoded, case-insensitive libpq routing query key that could make later `psql` seed execution consume a different target than the Node live probe.
  Parser regressions cover direct, case-varied, and percent-encoded keys, retain safe application/SSL options, and the mocked `ci-local` subprocess proves a rejected routing URL invokes neither seed nor Playwright.
- `[FIXED] [quality]` A dotenv-loader failure records a named gate failure, removes a stale attestation, and returns before seed/E2E under `set -e`.
  The production loader is exercised against a synthetic non-`.env` fixture path with shell precedence, while a fake Node failure proves stale-attestation removal and the exact failure record.
  Guard regressions capture every seed `psql` argument, one exact protected E2E environment assignment per key, no seed path for fast/unpinned/rejected targets, and restore-failure blocking.
- Reviewer confirmation remains pending for this exact remediation commit.

### Plan-wide Review 1 (2026-08-10)

- **Reviewers:** Claude self-review (Opus), Codex (`gpt-5.6-sol`, high), independent Claude (Opus), GLM 5.2.
- **Verdicts:** `[claude-self]` CHANGES REQUESTED; `[codex]` CHANGES REQUESTED; `[opus]` CHANGES REQUESTED; `[glm]` APPROVE with concerns.

**Must Fix**

1. `[FIXED]` `[codex]` The ended-session mutation recheck is not atomic with the status transition (`server/hocuspocus.ts:134-168`, `platform/internal/store/sessions.go:461-464`).
   A frame can observe `live`, the end transaction can commit, and the frame can then apply and relay; the later storage guard drops it, so connected peers see state absent from the archive.
    → Response: `[FIXED]` Spec 013 “Durable operation-owned freeze lease”. `PrepareSessionEnd` takes the lease under the exclusive lifecycle advisory key and `completeSessionConfirmedAt` writes the snapshots and `status='ended'` in one transaction guarded by the live lease (`platform/internal/store/session_lifecycle.go`); canvas auth and every mutation recheck reject an unexpired lease (`platform/internal/store/canvases.go`). Proved by `TestEndSession_ConfirmedBundlePersistsBeforeCommit`, `TestCanvasMutations_BlockBehindEndLifecycleLock`, `TestRealtimeAuth_BlocksBehindFreezeLifecycleLock`.
2. `[FIXED]` `[codex]` Established read-only canvas connections are not closed at JWT expiry (`server/hocuspocus.ts:202-238`, `server/realtime-jwt.ts:12-18`).
   A departed/revoked viewer can keep receiving updates indefinitely rather than for the accepted approximately 25-minute token window.
    → Response: `[FIXED]` Spec 013 “Realtime connection lifetime”. `scheduleCanvasJwtExpiry` in `server/hocuspocus.ts` closes writable and read-only canvas connections at `exp` (chained past the 2^31 ms timer limit, immediate when already expired, cancelled on close) from the registered `connected` hook. Proved by `server/hocuspocus.canvas.test.ts` “jwt expiry closes established reader” and “the registered connected hook closes an installed canvas connection at expiry”.
3. `[FIXED]` `[claude-self][opus][glm]` Remote Excalidraw scenes echo back into Yjs (`src/components/session/whiteboard/excalidraw-board.tsx:21-39`, `src/lib/whiteboard/excalidraw-yjs.ts:26-47`, `src/lib/whiteboard/use-whiteboard.ts:51-61`).
   `updateScene` triggers `onChange`, and the writer unconditionally stores the same serialized scene, so two writable tabs can ping-pong updates without user input.
    → Response: `[FIXED]` `observeExcalidrawScene` drops own-origin transactions and identical serializations short-circuit (`src/lib/whiteboard/excalidraw-yjs.ts`); `useWhiteboard` writes under a local-origin symbol behind a 100 ms trailing debounce. Proved in `tests/unit/excalidraw-yjs.test.ts` and `src/lib/whiteboard/use-whiteboard.test.tsx`.
4. `[FIXED]` `[codex]` The settled host floor has no frontend control (`src/components/session/whiteboard/whiteboard-panel.tsx:72-104,147-162`).
   No live surface reads or calls `PATCH /api/sessions/{id}/settings`, so teachers cannot use the supervision mechanism without a manual API call.
    → Response: `[FIXED]` `whiteboard-panel.tsx` reads and PATCHes the dedicated `/api/sessions/{id}/canvas-settings` route with the strict `{canvasFloor}` body and renders the `Canvas floor` control for the teacher. Proved in `tests/unit/whiteboard-panel.test.tsx`.
5. `[FIXED]` `[codex][claude-self][opus][glm]` The Phase-4 acceptance contract is incomplete (`docs/plans/094-session-whiteboard.md:143-146`).
   The exact named mint/ended-mutation tests were folded into differently named table tests, the live-panel create/visibility/owner-viewer tests are absent, and no Playwright create-to-loosen-to-view spec exists.
    → Response: `[FIXED]` Phase 13 (2026-09-21). All 17 exact `TestMintToken_Canvas_*` names now exist as top-level tests in `platform/internal/handlers/realtime_token_test.go`: four renames, twelve conversions of the former `TestCanvasMintMatrix` subtests onto a per-test `canvasMintFixture` (each test now owns its session, so live → public → ended state no longer leaks between cases), and one newly written `TestMintToken_Canvas_EndedArchive_OutsiderDenied` because the old `OutsiderDenied` subtest was really the public-viewer case. `TestCanvases_MutatingEndpointsReject_WhenEnded` was added to `canvases_integration_test.go`. Assertion count is unchanged (216) with `AllReadOnly` strengthened. The eight `TestCanvasStore_*` names, `TestCanvases_ListEndedArchive_ByRole`, and `TestCanvases_CrossOrgIsolation` already existed verbatim in `platform/internal/store/canvases_test.go`; the live-panel tests and `e2e/session-whiteboard.spec.ts` landed in Phases 11–12.

**Should Fix**

6. `[WONTFIX]` `[claude-self][opus]` Every Excalidraw `onChange` writes a full-scene Yjs frame and every writable canvas frame performs an uncached HTTP recheck plus several database queries (`excalidraw-board.tsx:34-39`, `hocuspocus.ts:134-168`, `realtime_token.go:320-371`).
   Active drawing can saturate the API/database and make the fail-closed guard disconnect clients under its own load.
    → Response: `[WONTFIX]` Split. The write side is `[FIXED]`: 100 ms trailing coalesce plus identical-scene skip. The uncached per-frame recheck is declined by settled design — Spec 013 Non-goals: “does not cache mutation authorization in Hocuspocus because session status must remain authoritative for every accepted mutation.” The load risk the finding named is bounded instead: eight active-plus-queued admissions per document and a 500 ms abortable authorization deadline (`server/canvas-lifecycle.ts`; `server/canvas-lifecycle.test.ts`). Recorded in `decisions.md` §11.
7. `[FIXED]` `[claude-self][opus]` The binding shares the entire Excalidraw `appState` (`src/lib/whiteboard/excalidraw-yjs.ts:33-36`, `src/components/session/whiteboard/excalidraw-board.tsx:23-26`).
   Writer viewport, zoom, selection, tool, and view-mode state can overwrite each viewer's local UI state.
    → Response: `[FIXED]` `DURABLE_APP_STATE_KEYS = ["viewBackgroundColor"]` is the only shared `appState`; viewport, zoom, selection, and tool stay local (`src/lib/whiteboard/excalidraw-yjs.ts`; `tests/unit/excalidraw-yjs.test.ts`).
8. `[FIXED]` `[claude-self][opus]` Any authenticated outsider admitted to a public class-less session can create private canvases until the per-session cap is exhausted (`platform/internal/handlers/canvases.go:70-79`, `platform/internal/store/canvases.go:152-160`).
   Decision 11 settled broad live reads, but did not settle outsider-owned writes or denial of service against the class.
    → Response: `[FIXED]` Spec 013 “Least-privilege canvas access” settled the fork: only the teacher or a `present` participant may create, enforced inside the locked create transaction before the cap count (`ErrCanvasCreatorUnauthorized`, `platform/internal/store/canvases.go`). Proved by `TestCanvasHandler_PublicClasslessOutsiderAndConcurrentCapCannotCreate` and `TestCanvasHandler_CreateCanvas_DeniesInvitedAndUnrepresentedAdministrator`.
9. `[FIXED]` `[codex]` The board discards Excalidraw `files` while image-reference elements remain enabled (`src/components/session/whiteboard/excalidraw-board.tsx:31-39`, `src/lib/whiteboard/excalidraw-yjs.ts:26-38`).
   A pasted image can appear locally but disappear for peers and after archive reload.
    → Response: `[FIXED]` Phase 13 (2026-09-21). File paste and native file drag/drop were already rejected and `files` is never stored, but the Excalidraw image tool was still enabled and both rejections were silent, short of Spec 013 (“disabled … rejected with a visible explanation”). `excalidraw-board.tsx` now passes a stable `UIOptions={{tools:{image:false}}}` and shows a `role="status"` notice on every rejected paste, drop, and dragover. New tests in `tests/unit/whiteboard-panel.test.tsx` were mutation-checked: four fail against the pre-change component and pass against the new one.
10. `[FIXED]` `[claude-self]` `onLoadDocument` ignores the current canvas `readOnly` response (`server/hocuspocus.ts:256-266`).
    A stale writable claim connecting after end is advertised writable until its first mutation is reclassified; the current decision should update the connection configuration immediately.
    → Response: `[FIXED]` Spec 013 requires current authorization per admission rather than reliance on `onLoadDocument`. `authorizeConnection` sets both `connectionConfig.readOnly` and `connection.readOnly` from the current Go answer on every canvas admission, including an already-loaded document (`server/hocuspocus.ts`; `server/hocuspocus.canvas.test.ts` “runs current authorization for every canvas admission, including an already-loaded document”).
11. `[WONTFIX]` `[glm]` Canvas authorization has no platform-admin/impersonation bypass (`platform/internal/handlers/realtime_token.go:320-371`), unlike other realtime document types.
    Private student-canvas oversight versus least-privilege denial is a trust-model decision, not a mechanical assumption.
    → Response: `[WONTFIX]` Settled trust-model decision, not an omission. Spec 013 Non-goals and “Least-privilege canvas access”: administrators and impersonators receive exactly the represented user's canvas access and gain no private-canvas oversight bypass. `authorizeCanvasDoc` has no `IsPlatformAdmin`/`ImpersonatedBy` branch; `TestCanvasSettings_ExactAuthorizationAndMissingEndedMatrix` and `TestCanvasHandler_CreateCanvas_DeniesInvitedAndUnrepresentedAdministrator` pin it. `docs/api.md` already said so; Phase 13 added the missing statement to `decisions.md` §10, which Spec 013 required.

**Nice to Have**

12. `[FIXED]` `[claude-self]` The irreversible `participants`/`session` visibility choices lack the plan's confirmation (`whiteboard-panel.tsx:90-104`), and a failed teacher end request gives no visible error (`teacher-dashboard.tsx:247-252`).
    → Response: `[FIXED]` Every visibility raise goes through `ConfirmDialog` (`whiteboard-panel.tsx`), and a failed end renders a `role="alert"` without navigating (`teacher-dashboard.tsx`). Proved in `tests/unit/whiteboard-archive.test.tsx` and `tests/unit/whiteboard-panel.test.tsx`.
13. `[FIXED]` `[claude-self][opus]` The neutral room redirects every student-page 404, including a nonexistent session, to the generic archive (`src/app/(portal)/sessions/[id]/page.tsx:110-119`).
    → Response: `[FIXED]` `src/app/(portal)/sessions/[id]/page.tsx` redirects to the archive only for the ended-session 404 body and calls `notFound()` otherwise (`tests/unit/sessions-room-page.test.tsx`).
14. `[FIXED]` `[claude-self]` `docs/testing.md:38-42` still recommends `bun run --env-file=/dev/null test` although the Phase-6 contract replaced that claim with five explicit empty provider keys; `student-session.tsx:102` also retains stale effect dependencies.
    → Response: `[FIXED]` The `student-session.tsx` effect dependencies were corrected in Phase 11. Phase 13 (2026-09-21) replaced the stale `--env-file=/dev/null` advice in `docs/testing.md` with the five explicit empty provider keys.
15. `[FIXED]` `[claude-self][opus][glm]` Minor cleanup remains around the dead canvas `plain_text` column/write path, the generic `/settings` route name, duplicate session lookup on list, the owner foreign-key delete policy, and the broadened blank-attempt load log.
    → Response: `[FIXED]` Four of five fixed, one declined. `plain_text` was removed from migration 0028 and the probe sentinels; only `canvas-settings` is registered (`TestCanvasSettings_AtomicRouteCutoverRejectsLegacySettingsRoute`); the duplicate list lookup was claimed fixed here but `[codex]` correctly disputed it in Plan-wide Review 2 — `ListVisibleCanvases` still reads the session through both `GetSession` and `CanAccessSession`, tracked as finding R2-6 below; the load log now fires only on a thrown error and canvas load errors rethrow — that last item was untested until Phase 13 added the three “narrowed hocuspocus document-load logging” tests. The owner foreign-key delete policy is `[WONTFIX]` by Spec 013 “Canvas API corrections”: it “keeps the existing data-retention behavior”.

Reconciliation (Phase 13, 2026-09-21): every response above was verified against code and tests at the review commit by a read-only audit, not taken from the post-execution entries; findings 5, 9, 14, and 15 had real residual gaps that Phase 13 closed.
The responses stay subject to confirmation by the Plan-wide Review 2 reviewers below.

The review converged strongly on the binding and missing-contract findings.
The atomic end transition, public-session create policy, admin visibility, and newly required end-session/E2E files cross the frozen scope or trust model, so the hard safeguard pauses implementation until those decisions and scope additions are explicit.

### Plan-wide Review 2 — Round 1 (2026-09-21) — exact commit `8ce0a6e`

- **Reviewers:** Claude self-review (Fable 5.1 substituting in slot 1), Codex (`gpt-5.6-sol`, high), independent Claude (Opus 5, fresh context), GLM 5.2.
- **Verdicts:** `[claude-self]` CHANGES REQUESTED; `[codex]` CHANGES REQUESTED; `[opus]` CHANGES REQUESTED; `[glm]` **no verdict** — the run timed out after 20 minutes with no output on the 136-file diff (its `limit.output` was 128000, so this was not the empty-review failure mode).
  GLM is neither substituted nor waived; it is re-dispatched on the Round 2 commit with a prompt narrowed to an explicit file list.
- **Disclosed deviation:** this round ran before the full pinned-E2E gate because no separately started Bridge stack was available; `ci-local.sh --fast` was green on the reviewed tree. A full gate on the exact merge commit is still required.
- **No reviewer found** a private or cross-org canvas read, a read-only-token write, a post-end mutation, or a migration/probe mismatch. `[codex]` confirmed Review 1 responses 1–14 and disputed 15; `[opus]` confirmed 1–15, scoping 11 to canvases only.
- Codex ran with `danger-full-access`; the working tree was verified clean after its run.

**Must Fix**

R2-1. `[FIXED]` `[codex][opus]` The gate never proves the running stack uses the test database (`scripts/ci-local.sh:51-63,73`, `e2e/playwright.config.ts:4`, `e2e/seed.setup.ts:100`).
   `GATE_DATABASE_URL` is parsed and live-validated as `_test`, but pinning `DATABASE_URL` for Playwright does not reconfigure an already-running service, and `E2E_BASE_URL` is now adopted silently from `.env` with no check that the target is Bridge at all.
   A stack mistakenly pointed at development or production data that contains the demo identities would have classes created, students enrolled, and live sessions ended while the gate passes.
   `[opus]` adds that line 63 assigns `E2E_BASE_URL` without `export`.
   → Response: `[FIXED]` Delivered by Phase 14 (plan gate at `6ca8d47`) and proven end to end on 2026-09-22 — see “Plan-wide Review 2 — live E2E bring-up” above: the full `ci-local.sh` on `b352fd0` attested all three services against `bridge_test` and ran Playwright green, attestation `e2e:true`. Originally recorded in progress through Phase 14, which cleared its own Tier-A plan gate at `6ca8d47`. Committed so far: the shared contract and vector (`7f34e7f`) and the gate-side verifier, `ci-local.sh` wiring before the seed restore, 29 guard selftests, and documentation (`456f015`). Not yet committed: the three service attestation surfaces and the `seed.setup.ts` re-check, which are implemented and compile but whose tests are still being written. This stays `[OPEN]` until those land and a full pinned-E2E gate has actually exercised the attestation against a running stack.
R2-2. `[FIXED]` `[opus]` `EndSession` dropped the platform-administrator path with no recorded decision and no test (`platform/internal/handlers/sessions.go:558`, removed in `ae19222`).
   Spec 013 scopes the no-bypass rule to private canvases and says the end handler authorizes “under the existing tenancy rules”, which on `main` admit an administrator; sibling teacher-only routes at `sessions.go:1007,1099,1145,1180` still do.
   Orchestrator note: an administrator impersonating the teacher can still end the session, so “no other route” overstates it, but the change is an undocumented trust-model tightening either way.
   → Response: `[FIXED]` `43af154`. The user chose to restore the administrator path. `EndSession` again allows `claims.IsPlatformAdmin`, matching `main` and the sibling teacher-only routes. `TestEndSession_PlatformAdminCanEnd` pins it, and `TestEndSession_PlatformAdminGainsNoCanvasAccess` pins that the same administrator still gets 403 from `canvas-settings` and cannot mint a private canvas token, before and after ending. `docs/api.md` states who may end a session.

**Should Fix**

R2-3. `[FIXED]` `[codex][opus][claude-self]` Create, update, and delete canvas still answer the ended-session 409 before authorization (`handlers/canvases.go:32-51,148,197`, `store/canvases.go:228-241`), the opposite of the ordering `52eacce` restored on `PatchCanvasSettings` (which all three reviewers judged correct).
   An unrelated authenticated user holding a cross-org session UUID can distinguish live from ended sessions.
   → Response: `[FIXED]` `b32901e`, authorized by the user as a tenancy-adjacent change. Create checks the creator rule, and update/delete check ownership, before the ended and end-lease answers, inside the same lifecycle-locked transaction; every authorized caller's outcome is unchanged. `TestCanvases_AuthorizationPrecedesTerminalState` (69 subtests across live, frozen, and ended), `TestCanvases_OutsiderCannotDistinguishSessionState` (byte-identical status and body across all three states), and `TestCanvasStore_MutationsDenyNonOwnerBeforeTerminalState` pin it; `docs/api.md` documents the ordering.
R2-4. `[FIXED]` `[codex][opus]` `EndSession` shapes its response from the local `confirmed` flag rather than `durableEnd.WhiteboardServerArchiveComplete` (`handlers/sessions.go:587,659`, `store/session_lifecycle.go:384`).
   When request A's lease expires and request B ends the session confirmed first, A answers `false` plus the incomplete-archive warning although the durable value is `true`.
   → Response: `[FIXED]` `43af154`. The flag and the warning are both shaped from `durableEnd.WhiteboardServerArchiveComplete`; a nil durable value keeps the conservative `false`. `TestEndSession_ResponseReflectsDurableArchiveFlagNotLocalAttempt` reproduces the exact interleaving deterministically through the existing `onFreeze` seam, and `TestEndSession_ResponseFlagAlwaysAgreesWithDurableColumn` pins the invariant across confirmed, degraded, and already-ended ends. Codex audited `StartScheduledSession` and the replacement-end path and found neither shares the bug.
R2-5. `[FIXED]` `[opus]` The gate runs `scripts/seed_problem_demo.sql` unattended while `AGENTS.md` still lists `scripts/seed_*.sql` as an always-pause safeguard with migration of the throwaway container as the only exception.
   It runs only after the live `_test` validation, so this is a governance/code inconsistency rather than a data risk.
   → Response: `[FIXED]` `54d99e4`. The user chose the narrow exception: `AGENTS.md` now allows `ci-local.sh` to reapply the canonical demo seed only through the parsed and live-validated `_test` `GATE_DATABASE_URL`; running a seed by hand or against any other target remains a pause.
R2-6. `[FIXED]` `[codex]` `ListVisibleCanvases` reads the session twice (`store/canvases.go:488,535`, `store/sessions.go:813`) and can combine different snapshots across a concurrent end; Review 1 response 15 wrongly claimed this fixed.
   → Response: `[FIXED]` `b32901e`. `ListVisibleCanvases` evaluates access from one repeatable-read snapshot through the shared evaluator instead of reading the session row twice. `TestListVisibleCanvases_SessionScopedVisibilityFollowsSharedRule` covers it. Review 1 response 15 has been corrected to say `[codex]` was right to dispute the earlier claim.
R2-7. `[FIXED]` `[claude-self]` `AuthorizeCanvasDocument` re-implements `CanAccessSession` inline (`store/canvases.go:106-121`) because it must run inside the lifecycle-locked transaction.
   The two rules match today (`[opus]` checked the same), but nothing pins them together, and Decision 9 requires the named helper precisely because a hand-rolled copy is how Plan 090's cross-org leak happened.
   One shared core taking a querier would also resolve R2-6.
   → Response: `[FIXED]` `b32901e`, authorized by the user. `CanAccessSession` is now a thin wrapper over one unexported loader and one evaluator taking a querier, and `AuthorizeCanvasDocument` calls that evaluator on its locked transaction, so the hand-copied rule is gone; signature and reason strings are unchanged, and the class-less-only comment lives on the single surviving copy. `TestCanAccessSession_FullMatrix` pins every reason string including the cross-org public class-bound denial, and `TestAuthorizeCanvasDocument_SessionAccessMatchesCanAccessSession` pins parity.

**Nice to Have**

R2-8. `[FIXED]` `[opus]` `realtime_token.go:206` logs a token-signing failure as “canvas authorization state query failed”.
   → Response: `[FIXED]` `43af154`. The token-signing failure is logged as “realtime token signing failed”.
R2-9. `[FIXED]` `[opus]` `src/lib/db/schema.ts:255-268` omits the `sessions_canvas_freeze_lease_pair` CHECK that `drizzle/0028` adds; a future `drizzle-kit generate` could emit a DROP, and Bridge has no down-migrations.
   → Response: `[FIXED]` `54d99e4`. `schema.ts` declares `sessions_canvas_freeze_lease_pair`, mirroring migration 0028.
R2-10. `[FIXED]` `[opus]` The full-scene last-writer-wins binding means two tabs of the same owner clobber rather than merge; add it to the accepted MVP limitations in `decisions.md` §10.
   → Response: `[FIXED]` `54d99e4`. `decisions.md` §10 records that the single-writer whole-scene binding overwrites rather than merges across two tabs of the same owner.

R2-11. `[WONTFIX]` `[claude-self]` (found while testing R2-3) A caller with no relationship to a session who knows **both** its UUID and a canvas UUID still gets 403 for a canvas that exists and 404 for one that does not, in every session state (`handlers/canvases.go` update/delete).
   → Response: `[WONTFIX]` proposed, for the Round 2 reviewers to accept or dispute. This is a canvas-existence oracle, not the session-lifecycle oracle R2-3 closed, and it requires two unguessable v4 UUIDs while disclosing only existence; the list endpoint already hides invisible canvases. Closing it means answering 404 to anyone who cannot *see* the canvas while keeping 403 for a participant who can see but does not own it, which changes a documented status code for legitimate callers and was not among the tenancy-adjacent edits the user authorized. The current behaviour is pinned as-is by the “missing canvas” rows of `TestCanvases_AuthorizationPrecedesTerminalState`.

### Plan-wide Review 2 — Round 2 (2026-09-21) — exact commit `905ab94`

- **Verdicts:** `[opus]` APPROVE WITH NITS; `[glm]` APPROVE WITH NITS (forwarder validated the verdict line and cited paths); `[codex]` CHANGES REQUESTED with **no Must Fix** and two Should Fix; `[claude-self]` CHANGES REQUESTED (R2-14).
- **Round-1 responses:** all three externals CONFIRMED R2-1 through R2-10 and accepted the R2-11 `[WONTFIX]` on its merits. On R2-1, `[codex]`: “resolved in code pending the required live-stack run … No false-success path, application-lock collision, or remote refusal disclosure was found”; `[opus]`: “I could not construct a scenario where all three attest and a write lands outside the gate's `_test` database.” R2-1 nevertheless stays `[OPEN]`, because the attestation has never run against a live stack.
- `[opus]` diffed the `CanAccessSession` split against `8ce0a6e` line by line and found it behaviourally equivalent, including that the evaluator's `status != "ended"` equals the old `== "live"` over the two-value enum; `[glm]` found no frontend caller depending on a `409`.
- `[glm]` marked its Review 1 findings 3, 5, 11, and 15 RESOLVED.

**Should Fix**

R2-12. `[FIXED]` `[codex]` The verifier's 30-second hold deadline covered only the fetches, not `BEGIN`, the lock, the re-reads, `ROLLBACK`, or close; a server that black-holed after the lock was taken would hang the gate with the lock held (`scripts/check-e2e-stack.mjs`).
   → Response: `[FIXED]` Every database phase is raced against a five-second deadline. A phase that never returns yields the distinct class *database timeout*, samples nothing, skips `ROLLBACK`, and force-closes the connection; a `ROLLBACK` that never returns also force-closes. A transaction-scoped lock cannot outlive its connection, so the lock is released on every path. The first version `unref()`'d the deadline timer, which let Node exit with an unfinished top-level await before the deadline could fire; the guard selftests caught that and the timer is no longer unref'd. Selftests: a never-resolving lock acquisition and a never-resolving rollback.
R2-13. `[FIXED]` `[codex]` `TestListVisibleCanvases_SessionScopedVisibilityFollowsSharedRule` uses static state and would also pass the old two-snapshot implementation, so it does not regression-test R2-6 (`platform/internal/store/canvases_test.go:900-939`).
   → Response: `[FIXED]` Accepted — the orchestrator had taken a test that could not fail for the property it claimed. `ListVisibleCanvases` gained a nil-in-production hook, `afterListSessionRead`, fired after the session read and before every later read. `TestListVisibleCanvases_UsesOneSnapshotAcrossAConcurrentEnd` holds two callers there while a second connection commits an end, deletes their participant and class-membership rows, and inserts a new canvas; production returns exactly what the first snapshot implies and omits the late canvas. A differential control runs the pre-R2-6 two-read body, recovered verbatim from `git show b32901e~1`, under the identical interleaving and gets `["late board"]` and `[]` — the inverse on every element — so a regression to two reads fails every assertion. A hook-free follow-up call proves the concurrent write really committed.
R2-14. `[FIXED]` `[opus][claude-self]` `attest_e2e_stack` split the verifier's line with an unquoted expansion, so its safety rested on a regex in another file in another language, although the ids originate from network services (`scripts/ci-local.sh`).
   → Response: `[FIXED]` The whole line is matched against the same alphabet in bash before anything is captured, and a partial line now captures nothing. Selftests cover a glob character, a shell metacharacter, and a missing id.
R2-15. `[FIXED]` `[opus]` The Next.js route compares `BRIDGE_HOST_EXPOSURE` exactly while Go and Hocuspocus trim and case-fold, so `Exposed` makes two services refuse to boot while the internet-facing one serves the endpoint without the opt-in; the shared vector did not pin the normalization (`src/app/api/e2e-stack/route.ts:210`).
   → Response: `[FIXED]` The normalization (trim, case-insensitive, exact word) and nine cases are in the shared vector; the route exports `isExposedHost`, and the opt-in stays exactly `"true"`. The same rows drive the Vitest route test, the Bun boot and registration test, and `TestE2EStackFlag_ExposureNormalizationMatchesSharedVector` in Go, so the three implementations can no longer disagree.
R2-16. `[FIXED]` `[opus]` Only refusal logging is rate-limited, not the observe queries; on an opted-in exposed host an anonymous caller can drive unbounded concurrent `pg_locks` queries against the shared `_test` pool (all three services).
   → Response: `[FIXED]` The vector pins at most two in-flight observe queries per process, with an excess request refused like any other (`observe_busy`) and running no query. Go uses a buffered-channel slot released by `defer`; Hocuspocus and Next.js decrement in `finally`, Next.js on `globalThis` so a dev recompile cannot zero a live count. Each service has tests for refusal-without-query, byte-identical refusal, release on success, error, and timeout, and a default equal to the vector.
R2-17. `[FIXED]` `[glm]` The *not attested* remediation and `docs/testing.md` said each service's refusal log names the reason, but a service started **without** the flag — the commonest mistake — has no endpoint and logs nothing.
   → Response: `[FIXED]` Both now say a silent service most likely lacks the flag, and that a flagged service logs only the refusal category, never the database name.
R2-18. `[FIXED]` `[glm]` A successful degraded end writes no server log; only the response warning and the database flag record it (`platform/internal/handlers/sessions.go:581-634`).
   → Response: `[FIXED]` One `slog.Warn("session ended with degraded whiteboard archive", sessionId, reason)` on the degraded-success path only. The reason is a fixed category — `canvas_control_unconfigured`, `freeze_failed`, `freeze_timeout`, `snapshot_count_mismatch`, `session_end_in_progress` — because the control client's error can embed a URL or bearer material. `TestEndSession_DegradedEndIsLoggedWithoutSecrets` reaches all five deterministically, asserts exactly one warning each, that a credential-bearing control URL in the freeze error leaks nothing, and that a confirmed end logs no such warning.
R2-19. `[FIXED]` `[glm]` `docs/api.md` and a handler comment claimed ending a session “reads no canvas content”, but `EndSession` persists the canvas snapshots from the freeze bundle.
   → Response: `[FIXED]` Both now say ending archives the snapshots server-side while the response discloses no canvas content and the administrator gains no canvas read, mint, or settings access. The orchestrator wrote the overstatement.

**Nice to Have**

R2-20. `[FIXED]` `[glm]` Every non-200 was *not attested*, so a proxy `5xx` with Go down produced flag advice. → Response: `[FIXED]` `5xx` is now *unreachable*, with a selftest.
R2-21. `[FIXED]` `[opus]` The Hocuspocus hook attaches at module scope whenever the flag is on, but the exposed-host boot refusal runs only under `import.meta.main`. → Response: `[FIXED]` The factory exposes `registrable` (flag on **and** boot permitted) and `server/hocuspocus.ts` spreads `onRequest` on that. A child-process import with the flag on, the host exposed, and no opt-in yields no `onRequest`; adding the opt-in restores it. The existing literal assertion was updated rather than deleted.
R2-22. `[FIXED]` `[glm]` The whiteboard panel shows a generic error for a `409` ended or end-in-progress race. → Response: `[FIXED]` A `409` with `code: "session_end_in_progress"` reads “This session is ending, so whiteboards can no longer be changed.”; any other `409`, including a non-JSON body, reads “This session has ended, …”; every other status keeps the generic message. The panel has no delete handler.
R2-23. `[FIXED]` `[glm]` `assertAttestedE2EStack` lists all three instance variables as missing when only one is. → Response: `[FIXED]` The helper names only the instance variables that are actually missing or empty, still failing closed.
R2-24. `[WONTFIX]` `[opus]` `AuthorizeCanvasDocument` reads `session_participants` twice per call. → Response: `[WONTFIX]` One indexed read inside the same transaction. Removing it means handing pre-read participant state into the shared access evaluator, which would let a caller supply the very facts the evaluator exists to establish; keeping that function self-contained is the point of R2-7.

### Plan-wide Review 2 — Round 3 (2026-09-21) — exact commit `debfe83` — **checkpoint**

- **Verdicts:** `[opus]` APPROVE WITH NITS (R2-12 through R2-23 RESOLVED, R2-24 `[WONTFIX]` ACCEPTED; “No new correctness, tenancy, or safety defect found in this delta, and no test in it that cannot fail”); `[glm]` APPROVE WITH NITS, but marking two of its own six items NOT RESOLVED; `[codex]` CHANGES REQUESTED with no Must Fix, R2-12 REOPENED, R2-13 RESOLVED; `[claude-self]` APPROVE.
- `[opus]` executed the `attest_e2e_stack` regex in a scratch shell against a glob, a `;`, a trailing `\r`, a two-line input, and a reordered line — none matched — and confirmed the snapshot test's differential control inverts on every element.

**Checkpoint (third consecutive non-converged code-review round, as `docs/reviewers.md` requires).**

| Round | Commit | Must Fix | Should Fix | Verdicts |
|---|---|---|---|---|
| 1 | `8ce0a6e` | 2 | 5 | 3 × CHANGES REQUESTED, GLM timed out |
| 2 | `905ab94` | 0 | 8 | 2 × APPROVE WITH NITS, 1 × CHANGES REQUESTED |
| 3 | `debfe83` | 0 | 4 | 2 × APPROVE WITH NITS, 1 × CHANGES REQUESTED |

Blockers are strictly decreasing, no Must Fix has appeared since Round 1, and no reviewer in any round found a tenancy defect or a path to a false attestation; every Round 3 defect fails closed.
One finding, R2-12, has reopened once. A second reopening of the same finding is a user-decision pause, so its fix below was made deliberately rather than quickly.
Two of the Round 3 findings are defects the Round 2 fixes themselves introduced (R2-26, R2-27), which is the honest cost of fixing under review pressure and the reason each fix now ships with a regression that would have caught it.

**Should Fix**

R2-12 (REOPENED). `[codex][opus]` `connect` was the one database phase with nothing to clean up on timeout: `connection` was still unset, so `finally` skipped it, and a handshake completing later left a reserved socket open (`scripts/check-e2e-stack.mjs`).
   → Response: `[FIXED]` The verifier keeps the connect promise and, when the deadline wins, force-closes whatever that promise later produces; `defaultConnect` ends its client if `reserve()` rejects, so a failed handshake leaves no socket. A forced close that itself times out can no longer throw out of `finally` and discard a decided result (`[opus]` nit). Selftests: a connection arriving after the deadline is force-closed and nothing else is done with it; a forced close that never returns neither hangs the gate nor loses the result.
R2-25. `[FIXED]` `[codex]` The two TypeScript timeout wrappers release their observe slot when the race expires, although the query is not cancelled and may still be running; with a stalled database, requests time out, more are admitted, and running queries accumulate past the cap of two (`server/e2e-stack.ts:175`, `src/app/api/e2e-stack/route.ts:288`). Go is unaffected because its context timeout cancels the query.
   → Response: `[FIXED]` Accepted — the cap limited waiting requests, not queries. In both TypeScript services the slot is now released by a once-only handler attached to the original query promise, so a request that times out keeps its slot until the abandoned query settles, and a late rejection is handled. `e2e stack holds its observe slot until the query settles, not until the request times out` and `… cannot exceed the cap however many requests time out` (Bun), and the two Vitest counterparts with a counting limiter, prove it; the test author built a pre-fix copy of the factory in a scratchpad and showed both new assertions invert against it. Side effect, noted rather than hidden: the route now starts `observe` one microtask after the call; the slot is still taken synchronously.
R2-26. `[FIXED]` `[glm]` **Regression introduced by R2-22.** A create that hits the per-session whiteboard limit returns `409` “Session canvas cap reached” with no `code`, and the new panel logic shows “This session has ended …” — a false statement in a live session, where the old generic text was at least true (`platform/internal/handlers/canvases.go`, `whiteboard-panel.tsx`).
   → Response: `[FIXED]` The handler emits `code: "session_ended"` (from `writeCanvasMutationError` and from `PatchCanvasSettings`) and `code: "canvas_cap_reached"`, with every status and `error` string unchanged; the panel shows session-specific copy only for codes it knows and the generic message otherwise, including a `409` with no code, an unknown code, or a non-JSON body. `TestCanvases_ConflictResponsesCarryStableCodes` pins the three exact bodies and asserts the cap row's session is still `live`; `whiteboard panel never claims the session ended for a 409 it does not recognise` is the regression guard. The three old test rows that encoded the false statement were moved to it with the opposite expectation. `docs/api.md` tells clients to branch on `code`, never on a bare `409`.
R2-27. `[FIXED]` `[glm]` **Inaccuracy introduced by R2-17.** The reworded remediation said a service without the flag “has no endpoint at all and logs nothing”, which is true of Go and Hocuspocus but false of Next.js, whose route file always exists and logs `flag_disabled`.
   → Response: `[FIXED]` Both the remediation and `docs/testing.md` now scope the silence to Go and Hocuspocus and say Next.js logs `flag_disabled`.
R2-28. `[FIXED]` `[glm]` The *database timeout* remediation said “the stack was not examined”, false when the timeout fires on the post-sampling lock re-read. → Response: `[FIXED]` It now says no verdict on the stack was produced.

**Nice to Have**

R2-29. `[FIXED]` `[opus]` Go's `strings.TrimSpace` and JavaScript's `trim()` disagree on a few non-ASCII code points, outside every pinned row. → Response: `[FIXED]` The contract now states that only ASCII whitespace is contracted and why; no environment file should contain the others.
R2-30. `[FIXED]` `[opus]` `tests/unit/e2e-stack-route.test.ts` holds evaluations on live 2-second timers across two further round-trips, a flake window on a loaded machine. → Response: `[FIXED]` The route test runs under fake timers restored in `finally` and never advances past the timeout, so no assertion depends on wall-clock time.
R2-31. `[WONTFIX]` `[glm]` `snapshot_count_mismatch` could log expected and actual counts. → Response: `[WONTFIX]` The sentinel carries neither; plumbing counts through the store's error for a log line is out of proportion, and the category already routes the investigation to the bundle-versus-session mismatch.

### Plan-wide Review 2 — Round 4 (2026-09-21) — exact commit `5688cd7`

- **Verdicts:** `[glm]` APPROVE (no nits; every remediation entry and the failure-class list verified against code); `[opus]` APPROVE WITH NITS (no Must Fix or Should Fix; R2-12 RESOLVED from its slot, R2-25 through R2-28 RESOLVED, no weakened assertion found, the held-slot consequence accepted — “No ceiling needed”); `[codex]` CHANGES REQUESTED with **R2-12 REOPENED a second time** as its only item, R2-25 RESOLVED, nothing else new; `[claude-self]` APPROVE.
- **User-decision pause.** A finding reopening twice after claimed resolutions is a genuine user decision under `docs/reviewers.md`. The orchestrator verified `[codex]`'s claim in the installed driver — postgres-js 3.4.9 `end()` sends a graceful FIN via `socket.end()`, not `destroy()`, so a server that black-holes after the lock is taken can leave the half-open socket and the transaction lock alive until TCP cleanup — and put four options to the user. The user chose to fix it properly, both halves.

R2-12 (REOPENED, second time). `[codex]` `sql.end({timeout:0})` ends the socket gracefully; if rollback stalls after lock acquisition the verifier can return while the half-open socket and lock survive until remote or TCP cleanup. Direction given: capture and destroy the raw socket on forced close, or install a server-side transaction deadline before taking the lock; test with a real socket that never completes shutdown.
   → Response: `[FIXED]` Both. The verifier hands postgres-js a one-shot socket factory — the pattern `scripts/check-test-database-url.mjs` already uses — keeps the raw socket, and `destroy()`s it on every forced close and on a failed handshake, so the RST ends the backend and its transaction lock regardless of what the graceful FIN would have done. Independently, `BEGIN` now runs `SET LOCAL idle_in_transaction_session_timeout = 30000` before the lock, so PostgreSQL ends an abandoned transaction on its own even if the client machine vanishes. Live regressions against the real `_test` database: the bound reads back as `30s`; a black-holed TCP server that never completes the handshake leaves a destroyed socket, not a half-open one; a forced close of an established lock-holding connection destroys its socket and the observer sees the server drop the lock. The late-disposal handler was also wrapped so a synchronously throwing `close` can never become an unhandled rejection (`[opus]` nit).

**Nice to Have** (`[opus]`, batched here rather than respun)

R2-32. `[FIXED]` A synchronously throwing `close` in the late-disposal handler would escape as an unhandled rejection. → wrapped, above.
R2-33. `[FIXED]` `lateConnectionIsDisposedAfterAConnectTimeout` relies on wall-clock margins. → Response: `[FIXED]` The selftest polls for the forced close up to five seconds instead of sleeping a fixed interval.
R2-34. `[FIXED]` `updateFloor` still shows a generic error although the settings route now returns usable codes. → Response: `[FIXED]` `updateFloor` goes through `whiteboardMutationErrorMessage`; a test covers ended, ending, the whiteboard limit, and a plain 403 keeping the generic text.

### Plan-wide Review 2 — Round 5 and confirmation (2026-09-21) — **CONSENSUS at `ddb17f3`**

- **Round 5** (`[codex]` only, on `0ae45f0`): APPROVE, R2-12 RESOLVED, no findings; TLS is not bypassed by the socket factory, and the `SET LOCAL` bound is valid and cannot pre-empt the client-side deadlines.
- **Confirmation on the same exact commit `ddb17f3`** (HEAD after the two nit fixes): `[codex]` APPROVE; `[glm]` APPROVE (the server-side bound is internal and needs no operator doc line); `[opus]` APPROVE WITH NITS (all three of its nits RESOLVED; verified postgres-js skips its own connect when given a socket and that `destroy()` tears down a TLS-upgraded connection too); `[claude-self]` APPROVE.
- Every required reviewer approves the same exact commit with no open blocker: **the code-review gate passes at `ddb17f3`.**
- `[opus]` confirmation nits, non-blocking and needing no re-review: (1) the black-holed-handshake test confirms the end state but does not isolate the new `destroy()` from postgres-js's own connect-timeout destroy; (2) the trailing `destroy()` in the non-forced close path means the forced-close test cannot distinguish destroy-before-`sql.end` from destroy-after — the `SET LOCAL` bound is the real guard there; (3) a reused nonce could let a stranded lock block a test. (3) is fixed in `6f381d1`; (1) and (2) are recorded as-is: the tests pin the guaranteed end state, which is the property that matters, and the plan says so rather than claiming more.
- R2-1 remains `[OPEN]` for one reason only: the attestation has never run against a live stack.

**Final tally of Plan-wide Review 2:** 34 findings over five rounds; 30 `[FIXED]`, 3 `[WONTFIX]` accepted by the flagging reviewers (R2-11, R2-24, R2-31), 1 `[OPEN]` (R2-1, pending the live run).

### Plan-wide Review 2 — live bring-up finding (2026-09-22) — consensus at `ddb17f3` INVALIDATED

While bringing up a real stack to run the pinned E2E gate, the attestation preflight failed on Hocuspocus only (Go and Next attested — flag on, both on `bridge_test`, both saw the gate lock). Root cause is a genuine gap I introduced in Phase 14, missed by all four reviewers:

R2-35. `[FIXED]` The E2E-stack attestation could not reach Hocuspocus behind a **path-prefix reverse proxy**. The dev/tunnel topology in `deploy/nginx/bridge.conf` mounts Hocuspocus under `/hocuspocus` on a single exposed port (3100), so `NEXT_PUBLIC_HOCUSPOCUS_URL` is `ws://host:3100/hocuspocus`. `realtimeOriginFrom()` dropped the path and probed the origin root `/e2e-stack`, which nginx routes to Next, not Hocuspocus; and `server/e2e-stack.ts` matched the request path with `=== "/e2e-stack"`, so even a path-preserving probe would have missed `/hocuspocus/e2e-stack`. The single-port proxy is a first-class supported setup (checked into the repo), and its own comment notes Hocuspocus "ignores the path" — true for the Yjs websocket, but the new HTTP `/e2e-stack` endpoint I added is path-sensitive.
   → Response: `[FIXED]` The user chose to fix it in scope (over a follow-up plan or an nginx-only workaround). `realtimeOriginFrom()` now preserves the base path (`ws://host:3100/hocuspocus` → `http://host:3100/hocuspocus`), so Hocuspocus is probed at `<base>/e2e-stack`; `server/e2e-stack.ts` matches any pathname ending in `/e2e-stack` (a segment boundary, so `/xe2e-stack` does not match). This makes the checked-in `deploy/nginx/bridge.conf` single-port topology attest with no nginx or `.env` change. The shared contract gained `realtimeUrlHandling`; `docs/testing.md` documents it; tests cover the base-path probe, the exact-root case, and the segment-boundary non-matches. Both files are within the Phase 14 File scope, so no scope widening.

This is a **material revision to attestation behaviour**, so the `ddb17f3` code-review consensus is invalidated and the Tier-A code-review gate re-runs on the new commit before merge. R2-1 remains `[OPEN]` (the attestation still has not completed a run against a live stack — this fix is a precondition for that run on this machine).

**Re-review consensus (2026-09-22) — `4c785ec`:** `[codex]` APPROVE (no findings), `[opus]` APPROVE (no findings; verified the URL edge cases, the segment-boundary matcher behind the flag/lock/`_test` gates, and that the tests fail against the old matcher), `[glm]` APPROVE (one non-blocking nit: the `deploy/nginx/bridge.conf:31-32` comment that Hocuspocus "ignores the /hocuspocus path" is now stale for the HTTP attestation endpoint — that file is outside Phase 14 File scope, left for a later docs touch), `[claude-self]` APPROVE. Code-review consensus re-established at `4c785ec`; R2-1 still `[OPEN]` pending the live-stack run.

### Plan-wide Review 2 — live E2E bring-up (2026-09-22) — code-review consensus at `4c785ec` carried forward

Running the full pinned gate against a user-started stack — the step R2-1 always required — surfaced three issues no prior tier could, then closed R2-1:

R2-36. `[FIXED]` Playwright could not load `e2e/seed.setup.ts`: it imported `e2e/helpers/e2e-stack.ts`, which statically imported `scripts/check-e2e-stack.mjs`. Node refuses to `require()` an ESM graph with a top-level await, and Playwright's TS transform then mangled the `.mjs` (`exports is not defined`). The unit test had mocked the verifier, so only a real E2E run exposed it. Fix (`445f2cf`): the verifier's CLI guard no longer uses top-level await, and the seed helper runs the verifier as a **subprocess** (`node scripts/check-e2e-stack.mjs`) — the same way `ci-local.sh` invokes it, one code path — with the gate-exported `E2E_STACK_INSTANCE_*` enforced by the verifier's `main()`. The seed helper stays fail-closed and never surfaces the database URL; the unit test was rewritten to inject the subprocess runner.

R2-37. `[FIXED]` Seven E2E specs the earlier session repaired but never ran failed on first live execution: the portal nav locators used `exact: true`, but the links' accessible names carry an emoji icon prefix (`"📖 Courses"`), and the past-session check matched the bare word "students", which also appears in hidden roster spans. Fix (`b352fd0`): portal nav locators are scoped to the `aside` without `exact`, and the past-session entry is matched by its link name; verified with a targeted 14-test run and then the full gate.

R2-1. `[FIXED]` The gate now proves the running stack uses the test database, exercised end to end: `bash scripts/ci-local.sh` on `b352fd0` ran the attestation (all three services observed the gate's advisory lock in `bridge_test`, over the single-port `3100/hocuspocus` proxy), restored the demo seed, and ran Playwright — 95 passed, 6 skipped (the failure-injection whiteboard test and other environment-guarded cases), 0 failed. The attestation names `b352fd0`, `tree_dirty:false`, `fast:false`, `e2e:true`. `E2E_BASE_URL` was the app's real origin `http://localhost:3100`; an earlier attempt at `:3101` failed only because the app's absolute/auth URLs use `3100`.

These are the only code changes since the `4c785ec` consensus (R2-36 in `445f2cf`, R2-37 in `b352fd0`), both test/harness-scoped and in File scope; a final Tier-A code-review pass on `b352fd0` re-confirms them before merge.

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

### Phase 8 race-proof follow-up — lifecycle replacement ordering (2026-08-11)

- RED: the deterministic race tests initially failed to compile because replacement had no observable class-guard/lifecycle-lock seam and no tested process-local representation of the required signed-key-plus-UUID lock order.
- GREEN: controlled two-pool tests pause `CreateSession` after the class guard and prove a preceding confirmed end commits archive-complete `true` without a later replacement overwrite.
  The opposite controlled order pauses after replacement has acquired the session lifecycle lock, proves the explicit confirmed end blocks behind it, then returns the conflict after replacement commits the only allowed archive-incomplete `false` result.
  The scheduled-start equivalent pauses after its class guard, lets confirmed completion win, then proves the scheduled replacement does not overwrite durable `true`.
- A collision test inserts two live UUIDs with the identical `12345678` signed-key prefix, holds the disjoint legacy one-argument advisory lock, and proves replacement acquires lifecycle locks in full UUID tie-break order without deadlock.
  The ordered helper is used by replacement after its SQL discovery order, retaining PostgreSQL's cross-process `ORDER BY` as the authoritative first ordering.
- The database-clock regression expires a lease with `clock_timestamp()` and proves a different token replaces it with a fresh approximately 15-second lease.
  Earlier ownership tests cover matching expired, different expired, different unexpired, matching cleanup, and durable-result cleanup.

### Phase 8 final lifecycle-cleanup semantics (2026-08-11)

- RED: final acceptance regressions first failed because `sessionEndResult` collapsed nullable archive completeness to `false`, the ended-row cleanup query cast the empty ordinary `EndSession` token to UUID, and scheduled-start had no discovery-entry proof seam.
- GREEN: lifecycle results now retain a nullable archive-complete pointer, so already-ended durable `true`, `false`, and `null` are returned exactly.
  Empty-token ordinary end retries clean only expired ended residue without a UUID cast and leave a different unexpired token untouched; matching/expired cleanup remains idempotent.
- The scheduled cancellation race records every live-session discovery entry and proves the count remains zero when cancellation occurs after class-guard acquisition but before the still-planned `FOR UPDATE` re-read.
  This test would fail under discovery-before-reread ordering.
- A database-clock confirmed-end regression expires a matching lease with `clock_timestamp()`, then proves confirmed completion returns the lifecycle conflict while the session remains live, no archive-true result persists, and no snapshot state is written.
- The active-freeze HTTP regression now also counts `session_canvases` and proves blocked create leaves no extra row, alongside the existing update/delete/floor no-write assertions.

### Phase 8 schema and lock hardening (2026-08-11)

- RED: probe tests first failed because lifecycle sentinels asserted names only, and malformed one-sided token/until writes were accepted by PostgreSQL.
- GREEN: lifecycle schema sentinels now pin each column's exact physical type and nullable state, and the schema probe returns a typed definition mismatch on wrong type or nullability.
  Parity recognizes the named `sessions_canvas_freeze_lease_pair` CHECK constraint declared through `ALTER TABLE`.
- Migration 0028 now enforces paired nullable lease fields with `CHECK ((canvas_freeze_token IS NULL) = (canvas_freeze_until IS NULL))`.
  The store regression proves both malformed pair forms are rejected.
- Test-only reconciliation was required after live verification returned `bridge_test`: the first constraint addition found one stale malformed local test row.
  In one transaction, the exact prerequisite repair cleared both lease fields on that malformed row and added `sessions_canvas_freeze_lease_pair`; afterward the constraint existed and the malformed-pair count was zero.
  No migration runner or non-test database was used.
- New class-less lifecycle tests register cleanup immediately after session creation, including subtests, so fixture users and sessions do not accumulate.

### Phase 8 deterministic advisory-lock proof (2026-08-11)

- RED: the first replacement of the timing-only assertion used the same one-connection test pool for waiter and observer, so the observer could not query PostgreSQL wait state and the proof timed out.
- GREEN: the lock regression now uses three independent verified test-database pools: holder, waiter, and observer.
  It records both backend PIDs, polls `pg_blocking_pids(waiterPID)` until PostgreSQL reports the exclusive advisory-lock holder, asserts the waiter cannot complete before release, commits the holder, then requires waiter completion under a bounded context.
  Transactions, pools, context cancellation, and goroutine completion paths are cleanup-safe; the proof passed twenty consecutive focused runs.

### Phase 8 physical schema parity (2026-08-11)

- RED: parser parity carried lifecycle ALTER-column names only, so a migration type or `NOT NULL` mutation could pass while runtime probe expectations drifted.
- GREEN: migration parsing normalizes `timestamptz` to PostgreSQL's information-schema name and records lifecycle type/nullability definitions bidirectionally.
  Mutation regressions prove both `uuid` to `text` and nullable to `NOT NULL` migration edits drift from sentinels; independent integration fixtures prove wrong type with correct nullability and correct type with wrong nullability each fail alone.
- The malformed-pair lifecycle test now registers immediate session cleanup; the Phase 8 class-less lifecycle fixtures were re-audited for cleanup registration.

### Phase 10 — lifecycle, control, admission, and provider RED contracts (2026-08-12; Terra; tests only)

- RED: added `server/canvas-lifecycle.test.ts` before the production module exists.
  Its 15 contracts currently fail with the intended missing-module error and bind signed advisory-key vectors; strict authenticated control operations; freeze/unfreeze/complete serialization; cached recovery; token-identity expiry; writer settlement; capture limits; eight-admission pre-allocation cap; rollback and cancellation; partial-overlap Yjs handoff; canvas-only parsed-frame limits; and generation/destroy/watchdog cleanup.
- RED: added four installed-Hocuspocus 3.4.4 hook contracts to `server/hocuspocus.canvas.test.ts`.
  The existing 19 tests pass; the four new contracts fail only because `createCanvasLifecycleHooks` and `scheduleCanvasJwtExpiry` are absent, covering per-admission current authorization, temporary-freeze retry semantics, JWT-expiry closure, and `MessageReceiver.apply` partial-overlap commit ordering.
- RED: added four `tests/unit/use-yjs-provider.test.ts` contracts for resettable 20-second `session_freezing` recovery, the two-second fast/30-second jittered outage tail, terminal classification, and attempt/session compatibility.
  After live validation of the pinned `bridge_test` target, all four fail because `canvasReconnectPolicy` is not yet exported.
- `CHECK_TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test node scripts/check-test-database-url.mjs` passed before the database-touching Hocuspocus suite ran.
  No migration, service, E2E, or live-provider command ran.
- Updated the normal test script so its Hocuspocus leg will execute both `server/hocuspocus.canvas.test.ts` and `server/canvas-lifecycle.test.ts` once the Phase 10 production slice lands.

### Phase 10 — lifecycle, control, admission, and provider production (2026-08-12; Terra)

- GREEN: added `server/canvas-lifecycle.ts` with the canonical signed session lifecycle key, strict control request decoding and constant-time bearer boundary, per-session token serializer, monotonic Go-validated lease expiry, cached capture accounting, reference-counted response writers, canvas admission turnstiles, shadow Yjs validation, parsed update ceiling, and generation-owned document cleanup.
- GREEN: wired canvas-only Hocuspocus hooks for every connection admission, pre-apply authorization and temporary-freeze response, JWT-expiry closure, installed `MessageReceiver.apply` commit/fallback handling, persistent canvas loading, a numeric-loopback internal API default, the 100 MiB shared websocket compatibility cap, and a separate strict control-listener factory.
- GREEN: added the canvas-specific reconnect classifier to `use-yjs-provider`, including a resettable 20-second freeze horizon, two-second fast retry ceiling, jittered 30-second outage tail, terminal classification, and attempt/session compatibility.
- Test-database reconciliation: `CHECK_TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test node scripts/check-test-database-url.mjs` passed before the database-touching Hocuspocus suite; no migration, service, E2E, non-test database, or live-provider command ran.
- `PATH=/home/chris/.bun/bin:$PATH DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun test server/hocuspocus.canvas.test.ts server/canvas-lifecycle.test.ts` passed 38 tests.
- `PATH=/home/chris/.bun/bin:$PATH DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bunx --bun vitest run tests/unit/use-yjs-provider.test.ts` passed 4 tests; `PATH=/home/chris/.bun/bin:$PATH bunx --bun tsc --noEmit`, phase-local ESLint, and `git diff --check` passed.
- `bun run lint` remains UNVERIFIED as a phase gate because the clean baseline reports 100 existing errors outside Phase 10; the phase-local files are lint-clean, and the full command's sole Phase-10 finding was corrected before the scoped lint rerun.

### Phase 10 — installed-path remediation RED evidence (2026-08-12; Terra; tests only)

- `[RED]` Added mechanism-falsifiable regressions to the Phase-10 Bun suites: a real Hocuspocus 3.4.4 `OutgoingMessage` sync envelope carrying the exact 1,048,576-byte decoded update boundary; installed `Document.saveMutex` and registered-connection capture/close count; an actual Go-style 409 `session_freezing` response; all-eight admission cancellation; token-keyed empty capture terminal reuse; the real `instance.documents` payload required by pinned `afterLoadDocument`/`beforeUnloadDocument`; strict outbound `GO_INTERNAL_API_URL` matrix; and provider CLOSE reason precedence, single-advance, connect reset, and terminal stop.
- `[FIXED]` (reconciled Phase 13, 2026-09-21: the installed-envelope decoder, 409 `session_freezing` classification, outbound URL validator, after-load/before-unload hooks, and provider-event bridge all exist and these tests are green.) The new tests intentionally failed on the then-current production slice: no installed-envelope decoder, no 409 freeze-response classification, no URL validator, no after-load/before-unload lifecycle hook, and no provider-event bridge exist; the current capture ignores the installed save mutex/connections, admission cancellation can resolve queued work, and a completed zero-snapshot token can start a fresh operation.
- `[RED evidence]` After `CHECK_TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test node scripts/check-test-database-url.mjs` accepted the parsed/live target, `PATH=/home/chris/.bun/bin:$PATH DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun test server/canvas-lifecycle.test.ts server/hocuspocus.canvas.test.ts` produced 39 passes and 8 expected RED failures.
  No migration, service, E2E, non-test database, or live-provider command ran.

### Phase 10 — installed-path remediation RED evidence, continuation (2026-08-12; Terra; tests only)

- `[RED]` Extended the test-only contract with an installed `Document.saveMutex`/connection-map capture proof, exact turnstile-before-auth deadline/abort ordering, terminal-queue foreign-token rejection, Hocuspocus `instance.documents` after-load/before-unload payload, listener-start rollback, and incremental backpressure/destroy streaming seams.
- `[FIXED]` (reconciled Phase 13, 2026-09-21: capture closes registered connections, abort keeps its `authorization_timeout` classification, and listener rollback and streaming are implemented and green.) These were deliberately RED against the then-committed production slice: capture returns before closing registered connections; authorization reaches a denial after abort instead of preserving its timeout classification; lifecycle hooks, listener rollback, and incremental streamer are absent.
- `[RED evidence]` The same pinned focused Bun command reports 16 passes and 6 expected lifecycle RED failures; the full two-file command remains expected RED pending the absent Hocuspocus listener hooks.

### Phase 10 — installed provider and registry RED evidence (2026-08-12; Terra; tests only)

- `[RED]` Added an installed Hocuspocus server `OutgoingMessage.writeCloseMessage` → provider `onMessage` contract requiring token refresh/re-auth/reconnect without an unauthenticated queued write, and an installed `Hocuspocus.createDocument` contract requiring failed pre-registry/after-load generations to release without touching a next-turn replacement.
- `[FIXED]` (reconciled Phase 13, 2026-09-21: the listener CLOSE reason reaches the canvas provider recovery producer and generation-owned load hooks run on the real `createDocument` path; both contracts are green.) Both contracts remained RED until the production hook wires the listener CLOSE reason to the canvas provider recovery producer and supplies generation-owned load hooks to the real `createDocument` path.
- `[RED evidence]` With both database URLs pinned to `postgresql://work@127.0.0.1:5432/bridge_test`, the focused provider Vitest suite has 5 passing existing contracts and 2 expected RED failures: `createCanvasProviderEventBridge` and `bindInstalledCanvasProvider` are absent.

### Phase 10 — control, unload, allocation, and apply RED evidence (2026-08-12; Terra; tests only)

- `[RED]` Added actual unbound-node-listener request emission for `createCanvasControlListener` (freeze must stream its cached token entry), installed Hocuspocus disconnect/unload hook contracts, exact ledger pre-reservation boundary/rollback, and pending admission fallback identity coverage.
- `[RED evidence]` After live `_test` URL validation, `bun test server/hocuspocus.canvas.test.ts` recorded 24 pass and 8 expected RED failures, including absent registry/disconnect/startup/stream hooks and the real listener currently calling `freeze` without incremental `stream`.
  No listener was bound, and no service, migration, E2E, non-test database, or live provider was started.

### Phase 10 — lifecycle production remediation evidence (2026-08-12; Terra)

- `[IMPLEMENTED]` The lifecycle now decodes the installed nested canvas update envelope at the exact 1 MiB boundary, preserves retryable Go `409 session_freezing`, aborts expiry outside its active serializer before cleanup, tracks each admission controller, reserves load/capture ledgers before allocation, and captures registered documents behind their admission turnstile and save mutex.
- `[IMPLEMENTED]` The installed hooks validate the outbound control URL at startup, close canvas sockets at JWT expiry, bind listener startup transactionally, wire disconnect/unload admission cancellation, and use the actual `instance.documents` identity at after-load/before-unload boundaries.
- `[IMPLEMENTED]` Canvas providers now use one close bridge, force-remint a canvas JWT before the installed CLOSE reauthentication/sync path, and keep attempt/session provider behavior unchanged.
- `[GREEN evidence]` With `DATABASE_URL` and `TEST_DATABASE_URL` pinned to `postgresql://work@127.0.0.1:5432/bridge_test`: `bun test server/hocuspocus.canvas.test.ts server/canvas-lifecycle.test.ts` passed 56 tests / 170 assertions; `vitest run tests/unit/use-yjs-provider.test.ts` passed 7 tests; `bunx --bun tsc --noEmit`, scoped ESLint, and `git diff --check` passed.

### Phase 10 — Sol Round 2 registered-path RED remediation (2026-08-12; Terra; tests only)

- `[FIXED]` `[sol][IMPORTANT]` The registered-path RED contracts below are superseded by exact-commit Sol approval at `83aa1f7`; nested decoding, lifecycle-owned authorization, load ownership, stable CLOSE reasons, provider recovery, incremental streaming, accounting conversion, cancellation, terminal coalescing, and redirect refusal all passed the installed-path review.
- `[RED evidence]` With both database URLs pinned to `postgresql://work@127.0.0.1:5432/bridge_test`, `PATH=/home/chris/.bun/bin:$PATH bun test server/canvas-lifecycle.test.ts server/hocuspocus.canvas.test.ts` yields 56 passing contracts and 11 expected behavioral RED failures: raw-frame admission, lifecycle-owned authorization identity, cold-load ordering/watchdog release, pre-close accounting conversion, awaitable connection cancellation, terminal coalescing, incremental streaming, freeze CLOSE reason propagation, and both redirect checks.
  `PATH=/home/chris/.bun/bin:$PATH bunx --bun vitest run tests/unit/use-yjs-provider.test.ts` yields 7 passes and the expected ArrayBuffer CLOSE/remint RED.
  No service, E2E, migration, non-test database, or live-provider command ran.

### Phase 10 — Sol Round 2 production remediation (2026-08-12; Terra)

- `[FIXED]` `[sol]` Registered Hocuspocus admissions decode the nested installed sync envelope and pass decoded bytes, authenticated user identity, and the lifecycle-owned 500ms abortable authorization into the eight-slot turnstile; pre-turnstile mutation authorization is removed.
- `[FIXED]` `[sol]` Lifecycle load uses a temporary reserved Y.Doc, an exact after-load registry claim, generation watchdogs, actual unload cancellation, destroy-only release, unique per-frame admission identities, coalesced terminal promises, and awaitable connection-owned cancellation.
- `[FIXED]` `[sol]` Capture reserves before encode, shrinks atomically to exact incremental-response bytes before socket closure, streams entry-by-entry with registered writer deadlines and backpressure, and preserves empty-list no-allocation behavior.
- `[FIXED]` `[sol]` Both bearer authorization fetch paths refuse redirects; temporary freeze errors expose the stable reason; the installed browser bridge handles `ArrayBuffer` CLOSE frames, fences outgoing writes during remint, then restores token/sendToken/sync without an unauthenticated replay.

### Phase 10 — Sol Round 3 remediation (2026-08-12; Terra; reviewer confirmation pending)

- `[FIXED]` `[sol]` Transactional after-load rollback, terminal-promise promotion, exact Yjs transaction-origin correlation, bounded completion tombstones, retryable capture CLOSEs, and abort-owned control streaming are reviewer-confirmed at exact commit `83aa1f7`.
- `[ADDRESSED]` `[sol]` The registered lifecycle now rolls an after-load initialization failure back transactionally, preserving a later generation; unfreeze promotes to complete behind one terminal cleanup slot; and an unrelated microtask Yjs transaction cannot commit the admitted Hocuspocus connection's reservation.
- `[ADDRESSED]` `[sol]` Canvas reconnect recovery keeps writes fenced through retryable `409 session_freezing` and installed PermissionDenied, emits Auth before sync on an attached provider, and resets its composed canvas retry policy after each successful reauthentication.
- `[ADDRESSED]` `[sol]` Installed `Connection.handleMessage` regressions exercise ninth-slot saturation and resident-ledger exhaustion through the pinned close path, while the control listener aborts a backpressured writer on request/response ownership loss.
- `[FIXED]` `[sol]` The named installed-path tests below were accepted by Sol at exact commit `83aa1f7`.
- `[ADDRESSED]` `[sol]` `getRealtimeToken` now preserves a strict optional retryable mint-error code on `RealtimeMintError`; the real `409 { error, code: "session_freezing" }` response reaches the installed canvas-provider recovery callback instead of degrading into terminal authentication failure.
- `[ADDRESSED]` `[sol]` Pinned `Connection.onClose` now proves numeric `1013` and one stable logical reason for saturation, initial resident, steady-state scratch resident, and successful capture-close paths; idle tombstones own identity-checked eviction timers; control abort ownership begins before freeze dispatch; and dead `beginLoad` façade coverage has moved to `prepareLoad`/`claimPreparedLoad`.
- `[GREEN evidence]` With both database URLs pinned to `postgresql://work@127.0.0.1:5432/bridge_test`, the focused realtime/provider/whiteboard Vitest selection passed 63 tests; `bunx --bun tsc --noEmit`, scoped ESLint, and `git diff --check` passed. No service, E2E, migration, non-test database, or provider was started.
- `[GREEN evidence]` With `DATABASE_URL` and `TEST_DATABASE_URL` pinned to `postgresql://work@127.0.0.1:5432/bridge_test`, `bun test server/canvas-lifecycle.test.ts server/hocuspocus.canvas.test.ts` passed 76 tests / 219 assertions; `vitest run tests/unit/use-yjs-provider.test.ts` passed 12 tests; `bunx --bun tsc --noEmit`, scoped ESLint, and `git diff --check` passed. No service, E2E, migration, non-test database, or provider was started.
- `[GREEN evidence]` Test database validation passed; `bun test server/canvas-lifecycle.test.ts server/hocuspocus.canvas.test.ts` passed 67 tests / 193 assertions; scoped provider/realtime/whiteboard Vitest passed 56 tests; `bunx --bun tsc --noEmit`, scoped ESLint, and `git diff --check` passed.

### Phase 10 — Sol Round 4/5 remediation (2026-08-12; reviewer confirmation pending)

- `[ADDRESSED]` `[sol]` The real realtime-token client now preserves strict `409 session_freezing` error identity, and the installed provider recovery test consumes that production error rather than a synthetic richer exception.
- `[ADDRESSED]` `[sol]` Numeric `1013` is verified through pinned `Connection.onClose` for saturation, initial resident exhaustion, steady-state multi-document scratch exhaustion, and successful freeze capture; the matching logical CLOSE reasons are decoded from the installed protocol frames.
- `[ADDRESSED]` `[sol]` Control request ownership begins before freeze dispatch, remembers an abort during capture, destroys rather than ends the response, rejects pre-aborted streams before reader registration, and makes late drain inert.
- `[ADDRESSED]` `[sol]` Completion tombstones own identity-checked eviction timers, provider retry timers are cancelled before an unmounted generation can remint, and production-dead `beginLoad` coverage is removed.
- `[ADDRESSED]` `[sol]` The production load adapter transactionally rolls back claim initialization failure; a distinct earlier registered extension failure leaves the pending watchdog armed for next-turn release, and startup rejects any extension ordered after Bridge's final claim hook.
- `[ADDRESSED]` `[sol]` A dedicated one-React-module hook test proves `useYjsProvider` installs the canvas recovery bridge, applies the live reconnect configuration after reauthentication, handles a second expiry, and detaches listeners/provider state on cleanup; real attached-provider tests separately prove Auth-before-sync and write fencing.
- `[FIXED]` `[sol]` All Round 4/5 mechanisms in this section were accepted by Sol at exact commit `83aa1f7`.
- `[GREEN evidence]` With both database URLs pinned to `postgresql://work@127.0.0.1:5432/bridge_test`, the database guard passed and `bun test server/hocuspocus.canvas.test.ts server/canvas-lifecycle.test.ts` passed 82 tests / 246 assertions after the Round 5 review corrections.
- `[GREEN evidence]` The paired hook/provider Vitest run passed 15 tests; `bunx --bun tsc --noEmit`, scoped ESLint, and `git diff --check` passed. No service, E2E, migration, non-test database, or live provider ran.
- `[ADDRESSED]` `[sol]` Provider recovery teardown now releases its bridge before destroying the provider, so destruction removes every restored listener; the hook regression requires zero authentication listeners after unmount.
- `[ADDRESSED]` `[sol]` Bridge asserts that its canvas `afterLoadDocument` hook is the final installed after-load extension. A distinct earlier Hocuspocus extension failure leaves the pending watchdog armed for exact next-turn release, any extension configured after Bridge is rejected, and the remaining dead `afterLoad` façade is removed.
- `[FIXED]` `[sol]` Sol found no remaining issue in these Round 5 corrections at exact commit `83aa1f7`.

### Phase 10 — exact Sol approval (2026-08-12)

- Sol reviewed exact commit `83aa1f731dc85e1767bd1c0ce6e6e20575a4aca4`, found no material issue, and returned **APPROVE**.
- All Phase 10 Sol findings are `[FIXED]`; Phase 11 may proceed.
- State verified green at `a9f1c3b` before the dispatch attempt: `bun test server/hocuspocus.canvas.test.ts server/canvas-lifecycle.test.ts` 82/82 (246 assertions); `vitest run tests/unit/use-yjs-provider.test.ts` 14/14; `bunx --bun tsc --noEmit` clean. Both database URLs pinned to `bridge_test`; no service, migration, or provider ran.
- **Resume action:** re-dispatch the Sol confirmation review (prompt: verify each Round 4/5 remediation against the exact commit) once quota resets or credits are added. No author-side work is pending; the loop is waiting on the reviewer only.

### Phase 10 — canvas provider construction delay/minDelay fix (2026-08-31)

- **Root cause:** the canvas `HocuspocusProvider` construction options (`src/lib/yjs/use-yjs-provider.ts`) set `delay: 250` for the fast reconnect phase but never set `minDelay`, so the pinned `@hocuspocus/provider@3.4.4` `HocuspocusProviderWebsocket` default (`minDelay: 1000`) applied. `@lifeomic/attempt`'s `retry()` (`node_modules/@lifeomic/attempt/dist/src/index.js:70-72`) unconditionally throws `delay cannot be less than minDelay` whenever `options.delay < options.minDelay`, regardless of the `jitter` flag, so every real canvas provider construction threw on its first connection attempt.
- **RED:** on commit `30e64b1` (pre-fix), `bunx --bun vitest run tests/unit/use-yjs-provider.test.ts` reported an unhandled rejection: `Error: delay cannot be less than minDelay (delay: 250, minDelay: 1000` from `node_modules/@lifeomic/attempt/dist/src/index.js:71`, raised inside `new HocuspocusProvider` at `src/lib/yjs/use-yjs-provider.ts:269`, exactly matching the reported live E2E defect.
- **Fix:** added `minDelay: 250` alongside the existing canvas construction options, matching the initial fast-phase `delay`. `canvasReconnectPolicy`'s fast-phase floor is also 250ms (`250 * 2 ** 0`) and its tail phase is always `> CANVAS_FAST_DELAY_MAX_MS` (2000ms), so every delay the policy ever issues stays `>= minDelay` for the lifetime of the provider; `minDelay` itself is never mutated by the per-close `websocket.setConfiguration` call, so this bound holds after every later reconfiguration too. Non-canvas (legacy `attempt:`/`session:`) provider construction is untouched — the `minDelay` key is only added inside the existing `canvas ? {...} : {}` spread.

### Phase 10 — StudentSession unused-prop lint ratchet fix (2026-08-31)

- **Root cause:** after the Plan 094 ended-session redirect moved to the neutral whiteboard archive, `StudentSession` (`src/components/session/student/student-session.tsx`) no longer reads its destructured `classId`/`returnPath` props inside the function body — the redirect target is now resolved by the caller (`src/app/(portal)/sessions/[id]/page.tsx`) before `StudentSession` ever mounts. The `StudentSessionProps` interface still declares both fields because the two rendering callers (`sessions/[id]/page.tsx` and `student/sessions/[sessionId]/page.tsx`) pass them; the legacy class-nested page only redirects to the canonical student-session route and does not render `StudentSession`.
- `[RED evidence]` On the pre-fix commit, `bunx eslint src/components/session/student/student-session.tsx` reported exactly the two flagged warnings: `39:3 'classId' is defined but never used` and `40:3 'returnPath' is defined but never used` (`@typescript-eslint/no-unused-vars`), 0 errors.
- **Fix:** dropped `classId` and `returnPath` from the function's destructuring pattern (`src/components/session/student/student-session.tsx:37-41`) while leaving `StudentSessionProps` and its two rendering call sites unchanged — the props remain part of the public type and are simply no longer bound to local variables the component does not use.
- `[FIXED] [spec-review]` Corrected the prior factual error that counted the legacy class-nested redirect as a third `StudentSession` rendering caller; reviewer confirmation is pending.
- `[GREEN evidence]` `bunx eslint src/components/session/student/student-session.tsx` → 0 problems. `bunx tsc --noEmit` → clean. `bunx --bun vitest run tests/unit/sessions-room-page.test.tsx tests/unit/whiteboard-archive.test.tsx` → 17/17 passed (existing room/archive redirect coverage; no new test added since it was already covering this behavior). `git diff --check` → clean. No service, migration, E2E, or live provider ran.
- **GREEN:** `bunx --bun vitest run tests/unit/use-yjs-provider.test.ts` passed 15/15 (the new "constructs the real pinned canvas provider with delay at least minDelay" test plus the 14 existing reconnect-policy/bridge tests). `bunx --bun eslint src/lib/yjs/use-yjs-provider.ts` and `bunx --bun tsc --noEmit` both passed clean. `git diff` scope confirmed to only `src/lib/yjs/use-yjs-provider.ts` (plus this plan file). No service, E2E, database, migration, or live provider ran.

### Phase 14 — E2E stack attestation (2026-09-21; Terra backend, Sonnet route, Opus tests, orchestrator gate)

- `[PLAN GATE]` The Phase 14 revision cleared its Tier-A plan gate at `6ca8d47` in Round 3: `[codex]`, `[opus]`, and `[glm]` APPROVE WITH NITS, `[claude-self]` APPROVE, no open blocker.
  Round 1 found the draft could attest one Hocuspocus while browsers wrote to another, and that it wrongly relaxed the `AGENTS.md` “never kill a process” safeguard; Round 2 found the lock could be stranded on a pooled connection and that one request per origin proves nothing about a second backend.
  One GLM pass returned text that was not a review and asserted an approval; it was discarded rather than counted, and GLM was re-dispatched.
- `[CONTRACT FIRST]` `scripts/tests/e2e-stack-vector.json` (`7f34e7f`) pins the flag, nonce pattern, reserved lock class `0x42523245`, key derivation, observe query, fingerprint, paths, and refusal shapes before any implementation, and is asserted verbatim by Go, Bun, Vitest, and the gate selftests.
  Deviation from the plan text: the vector carries direct low-word boundary cases (`0`, `2^31-1`) for key composition instead of nonces that hash to them, because finding such nonces would require inverting SHA-256.
- `[MECHANISM PROBED]` On `bridge_test`, a one-key `pg_advisory_xact_lock` taken in an open transaction on one reserved connection stayed on the same backend, was visible to a second session in `pg_locks`, was not visible under another objid or in the `hashtext` key space, and was gone after `ROLLBACK`.
  The durable proof is `scripts/tests/check-e2e-stack.live.test.mjs`, which runs in the gate.
- `[IMPLEMENTED]` Gate: `scripts/check-e2e-stack.mjs` plus `attest_e2e_stack` in `ci-local.sh`, ordered before the demo-seed restore, exporting `E2E_BASE_URL` and three instance ids to Playwright (`456f015`).
  Go: flag-gated `GET /api/health/e2e-stack`, dual `_test` proof on the pool's own connection string, refusals delegated to the router's NotFound, production and exposed-host start-time refusals (`ad8ab1f`).
  Hocuspocus: an `onRequest` hook on the client-facing port, registered only under the flag, whose refusals fall through to the library's default `200`; the success path rejects with no value, a convention confirmed in the installed library source (`817cce6`).
  Next.js: `/api/e2e-stack`, outside every proxied prefix, reading the literal `process.env.NEXT_PUBLIC_HOCUSPOCUS_URL` (`eded1fd`).
  Seed setup: re-attests as its first statement with a fresh nonce and the gate's instance ids, failing closed when the gate did not attest (`c2741eb`).
- `[DEFECTS FOUND BY THE TEST WORK — all fixed]` The verifier reported an expired hold deadline as *lock lost — the stack was not at fault, re-run* even when a service had simply hung; it now re-reads its own lock and lets the per-service timeouts stand, and the test that had pinned the wrong behaviour was corrected.
  The Next.js route's database-name parser skipped the scheme and single-segment checks the other two implementations make, and its exported pure functions did not validate the nonce; both now match.
  The verifier also gained an *unchecked* class, because reporting Hocuspocus as *no realtime origin* when Next.js itself had not attested pointed the operator at the wrong variable.
- `[TESTS]` Every name in the phase's named-test list exists and passes.
  Go: eleven unit tests through a test-local `database/sql` driver (no new dependency) so query counts and the non-`_test` live name are deterministic, three live integration tests, the flag and configuration tests, and the lock-class disjointness test.
  Bun: twelve hook tests, including a child-process import proving the hook is absent without the flag, and eight live verifier regressions.
  Vitest: fifteen route tests, including a genuine `vi.resetModules()` re-evaluation for the instance id, and the seed-attestation tests.
  Gate selftests: 29 new, 131 total.
- `[LIMITATIONS]` `TestE2EStack_LockInAnotherDatabaseIsNotSeen` uses a foreign lock class and a foreign objid rather than a literal second database, because creating one needs DDL; the `d.datname = current_database()` clause is pinned byte-for-byte by the shared-vector test.
  Bun has no in-process module re-evaluation seam, so the Hocuspocus instance-id test pins the `globalThis` cache with two independent factories instead.
  The Go parsed-name proof reuses `ValidateE2ECanvasControlFailureDatabaseURL`, which does not reject multi-host or `?host=` forms as the gate's validator does; the live `current_database()` check is the actual proof of destination.
  `[UNVERIFIED]` Whether `notFound()` from a Next.js route handler is byte-identical to a genuinely absent route, and the whole attestation end to end, can only be confirmed against a running stack; the verifier requires only a non-attesting answer either way.
- `[PROCESS NOTES]` Two test agents sharing `bridge_test` collided once: Vitest's truncating teardown landed between the two inserts of a Go store test and left an orphan user row that made that test fail deterministically.
  The next gate run cleared it, because the Vitest tier truncates `users` before the Go tier; no manual delete was needed. The underlying fragility — that test cleans up by id rather than by email — is outside this plan's scope.
  An accidental `git commit -a` swept eight uncommitted code files into a plan-only commit; it was local and unpushed, was undone with a soft reset that preserved every change, and the plan was re-committed alone.
  Two forwarding agents reported instruction-like text inside plugin tool output, one line about auto-approving destructive git commands; nothing acted on it.
- `[GREEN evidence]` `bash scripts/ci-local.sh --fast` on the complete Phase 14 tree: lint ratchet, type-check, all guards, 131 guard selftests, Vitest 966 passed and 11 skipped, Bun 105 pass, every Go package green against the validated `bridge_test`.
  This is a `--fast` run and is not merge evidence; the attestation itself has never yet run against a live stack.

### Phase 13 — finding reconciliation, residual gaps, and documentation (2026-09-21; orchestrator)

- `[AUDIT]` A read-only audit verified all fifteen Plan-wide Review 1 findings and the three Phase-10 RED bookkeeping entries against code and tests at `b6fe314`, rather than against earlier report entries.
  Eleven were already fixed with test evidence, three are declined by recorded Spec 013 decisions, and four had real residual gaps: the exact Phase-4 test names, the still-enabled Excalidraw image tool with silent rejections, the stale `--env-file=/dev/null` advice, and an untested narrowed load log.
  The audit also confirmed all fifteen Phase-12 Go names and nine Bun realtime names exist verbatim and unskipped, and that no TypeScript production code writes session status.
- `[RED → GREEN — contradictory authorization contracts]` The first `ci-local.sh --fast` run failed `TestCanvasSettings_ExactAuthorizationAndMissingEndedMatrix` (expected 403, got 409).
  Two tests demanded opposite answers for the same request — a student PATCH of `/canvas-settings` on an ended session.
  Commit `a320b82` had moved the ended-session conflict ahead of teacher authorization to satisfy a row in `TestCanvasHandler_MutationAuthAndEndedArchive`, and its recorded GREEN evidence ran that test but not the matrix test, which it broke.
  That row was the stale one: it came from the mechanical `/settings` → `/canvas-settings` rename inside a loop that uses student claims for every request, which is right for the student's own canvases and wrong for a teacher-only route.
  `PatchCanvasSettings` is restored to the ordering that passed the Phase-9 review — authorization first, so every non-teacher gets 403 whether the session is live or ended, matching the GET contract and disclosing no session state to an unauthorized caller — and the row now asserts both halves (student 403, teacher 409).
  This supersedes the `[FIXED]` claim about `PatchCanvasSettings` in the “Go canvas full-gate regression remediation” entry below; its two fixture fixes and the deadline-test fix stand.
- `[FIXED — exact test names]` Tests by Opus: the seventeen `TestMintToken_Canvas_*` names and `TestCanvases_MutatingEndpointsReject_WhenEnded` now exist as top-level tests.
  `TestCanvasMintMatrix` shared one session across live, public, and ended phases; each acceptance test now builds its own `canvasMintFixture`.
  The old `OutsiderDenied` subtest was really the public-viewer case, so it became `…EndedArchive_PublicViewerDenied` and a genuine non-public `…EndedArchive_OutsiderDenied` was written across all four visibility levels.
  Assertion count in the file is unchanged at 216.
- `[FIXED — image insertion]` Production by Sonnet, tests by Opus against a shared written contract.
  `excalidraw-board.tsx` passes a module-level `UIOptions={{tools:{image:false}}}` and shows a `role="status"` notice for five seconds on every rejected file paste, drop, or dragover.
  The production change landed before the tests were first run, so they were never observed RED in sequence; the orchestrator mutation-checked them instead by temporarily restoring the `HEAD` component, where the four behaviour tests failed and the two negative tests passed, then restored the new file byte-for-byte.
- `[FIXED — load log]` Three Bun tests now prove a document with no persisted state logs nothing, a thrown load error is logged, and a canvas load error rethrows instead of serving blank state.
- `[DOCS]` `decisions.md` §10 now states the no-administrator/impersonator bypass that Spec 013 required, the creator rule, token-bounded reader retention, and the no-binary-files rule; §11 states that PostgreSQL status outranks Hocuspocus availability, the confirmed and degraded guarantees, replacement warnings, the no-authorization-cache decision with its bounds (checked against the constants in `server/canvas-lifecycle.ts`), and the single-Hocuspocus-process limitation.
  `docs/api.md`, `docs/testing.md` (whiteboard test tiers and the five-key command), `docs/setup.md`, `docs/project-structure.md` (also correcting a stale claim that Playwright defaults to port 3003), `.env.example`, and `README.md` carry the operator- and reader-facing parts.
- `[E2E repair — UNVERIFIED]` The twelve named E2E specs plus `e2e/helpers*` and `e2e/seed.setup.ts` were repaired for stale routes, renamed labels (units → chapters, “My Code” → “My Work”, topics → focus areas), explicit login callback paths, and a seeded canonical chapter.
  They type-check and pass the lint ratchet after one `prefer-const` fix, but have **not** been run: no separately started Bridge stack was available, and the last recorded Playwright run before these edits had two failures.
  `e2e/unit-picker.spec.ts` now encodes an observed product inconsistency outside this plan's scope — the focus-area editor offers a Replace button although the link endpoint rejects replacing an already-linked chapter.
- `[GREEN evidence]` `bash scripts/ci-local.sh --fast` on the complete working tree: lint ratchet, type-check, all guards, 102 guard self-tests, Vitest 942 passed and 11 skipped, Bun realtime 85 pass, and every Go package green against the validated `bridge_test`.
  This is a `--fast`, dirty-tree run and is not merge evidence.
  One transient foreign-key failure was seen only while two test agents shared `bridge_test` concurrently; it did not recur in the serial gate.
  No migration, seed, E2E, service, provider call, non-test database, or `.env` read occurred.

### Phase 13 — Go canvas full-gate regression remediation (2026-08-31; Terra)

- `[RED]` The prior full local gate exposed three focused Go regressions: an ended session's non-owner canvas-settings PATCH returned `403` before the terminal `409`; two store visibility/archive fixtures tried to create participant-owned canvases without making the owner present; and the caller-deadline control-client test left its intentional blocked `httptest` handler open, so listener teardown timed out.
- `[FIXED]` `PatchCanvasSettings` now applies the ended-session conflict before teacher authorization, matching the existing mutation terminal-state precedence. Both store fixtures join their intended participant canvas owner as `present` before creation. The deadline test now releases only its own blocked test handler after proving the caller returned an error, so teardown is deterministic without closing or mutating the caller-supplied shared HTTP client.
- `[GREEN evidence]` `CHECK_TEST_DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test node scripts/check-test-database-url.mjs` accepted before the database tests. With both `DATABASE_URL` and `TEST_DATABASE_URL` pinned to that target, `go test ./internal/handlers ./internal/store ./internal/realtime -run "TestCanvasHandler_MutationAuthAndEndedArchive|TestCanvasStore_ListVisible_ByRole|TestCanvases_ListEndedArchive_ByRole|TestCanvasControlClient_RespectsCallerDeadlineAndVerifiedHTTPS" -count=1 -timeout 30s` passed; the isolated deadline test also passed with `-timeout 15s`. No migration, service, E2E, provider, or non-test database command ran.
