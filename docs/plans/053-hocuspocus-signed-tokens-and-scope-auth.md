# Plan 053 — Hocuspocus signed tokens + per-scope authorization (P0)

## Status

- **Date:** 2026-05-01
- **Severity:** P0 (token forgery, full WebSocket bypass)
- **Origin:** Reviews `008-...:9-15`, `008-...:17-23`, `009-...:17-23`. Plus the deferred-but-unfiled gates from plan 030b/030c/035.

## Problem

The Hocuspocus server treats the client-supplied connection token as a plain `userId:role` string and trusts whatever the browser sends.

| Layer | File:Line | Failure mode |
|---|---|---|
| Token construction (session teacher) | `src/components/session/teacher/teacher-dashboard.tsx:135` | `${userId}:teacher` literal |
| Token construction (session student) | `src/components/session/student/student-session.tsx:49` | `${userId}:user` literal |
| Token construction (parent viewer) | `src/components/parent/live-session-viewer.tsx:31` | `${parentId}:parent` literal |
| Token construction (problem attempt — student) | `src/components/problem/problem-shell.tsx:64` | `${userId}:user` literal |
| Token construction (problem attempt — teacher watch) | `src/components/problem/teacher-watch-shell.tsx:55` | `${teacherId}:teacher` literal — **6th site, missed in earlier draft** |
| Token construction (unit editor) | `src/lib/yjs/use-yjs-tiptap.ts:99` | `${userId}:teacher` literal |
| Token validation | `server/hocuspocus.ts:20-23` | `token.split(":")` — no signature, no DB check |
| Scope branching | `server/hocuspocus.ts:46-90` | Trusts the parsed `role` for `session:*`, `attempt:*`, `broadcast:*`, `unit:*` rooms |

**Document name format** (for the JWT scope claim): the actual Hocuspocus doc names are NOT one-flat-id-per-scope. They are:

- session: `session:{sessionId}:user:{studentId}` (one doc per student per session — confirmed at `server/hocuspocus.ts:46-47`)
- broadcast: `broadcast:{sessionId}` (teacher's broadcast doc)
- attempt: `attempt:{attemptId}` (one doc per attempt)
- unit: `unit:{unitId}` (one doc per unit)

The JWT's `scope` claim must be the **exact `documentName` string** the client will request, not a shortened scope-only key. `onAuthenticate` compares `claims.scope === documentName` byte-for-byte. The mint endpoint accepts the full doc-name from the caller and gates each path through the corresponding HTTP-side helper before signing.

A malicious client connects to `ws://hocuspocus/`, sends `{ token: "<victim-uuid>:teacher", documentName: "unit:<any-uuid>" }`, and gets read+write access to that document. Or `attempt:<victim-attempt-uuid>` works the same way. The forged role bypasses the role gating at lines 48-51.

Plans 030b and 030c promised to gate session-room access through the same class-membership-or-token rules as HTTP. Plan 035 explicitly deferred per-unit `canEditUnit` validation. Neither shipped.

## Out of scope

- Replacing Hocuspocus with a different realtime layer.
- Changing the Yjs document shape.
- Hocuspocus persistence (still in-memory; tracked separately in `TODO.md`).

## Approach

Two layers, both required:

1. **Sign the token.** Replace `userId:role` with a short-lived JWT minted by the Go API after a normal authenticated request. Sign with HMAC-SHA256 using a new `HOCUSPOCUS_TOKEN_SECRET` env var (separate from `NEXTAUTH_SECRET` so a leak of the WebSocket signing key doesn't compromise sessions). The token claim set:
   ```
   { sub: userId, role: "teacher"|"user"|"parent", scope: "<exact-documentName>", iat, exp }
   ```
   The `scope` claim is the **full Hocuspocus documentName** (`session:{sessionId}:user:{studentId}`, `attempt:{attemptId}`, `unit:{unitId}`, `broadcast:{sessionId}`) — see the doc-name format note above. `exp` ≤ 30 minutes; the client refreshes via a new `POST /api/realtime/token` endpoint.
2. **Verify on connect AND on document open.** `server/hocuspocus.ts::onAuthenticate` verifies the JWT signature and rejects mismatched `documentName` (exact-string compare with the JWT's `scope`). `onLoadDocument` does a defense-in-depth DB check via the Go API:
   - `session:{sessionId}:user:{studentId}` → caller passes `canJoinSession(sessionID)` AND (`sub == studentId` OR caller is the session's teacher/class-staff).
   - `attempt:{attemptId}` → owner-or-class-staff (new helper if needed).
   - `unit:{unitId}` → `canEditUnit` (existing helper).
   - `broadcast:{sessionId}` → class instructor of the session's class.

The Go-side helpers already exist for HTTP. Expose them via a single internal endpoint (`POST /api/internal/realtime/auth` taking `{documentName, sub}`) that Hocuspocus calls from Node; gate the endpoint to the Hocuspocus shared secret via `Authorization: Bearer <HOCUSPOCUS_TOKEN_SECRET>` so it isn't user-facing.

**Multi-pod / key rotation (Codex Phase 0 follow-up):** `HOCUSPOCUS_TOKEN_SECRET` is shared between Go and the Hocuspocus Node process. For multi-pod deployments, the secret must be sourced from the same secrets-manager entry by both. Initial implementation supports a single key; rotation is documented as a follow-up — when added, the JWT carries a `kid` header, the Hocuspocus verifier consults a small key map, and the Go side mints with the current key. Plan 053 ships single-key; key-rotation is a future plan (058+).

## Files

- Create: `platform/internal/handlers/realtime_token.go` — `POST /api/realtime/token` handler that mints a Hocuspocus JWT for the authenticated caller, scoped to a single `documentName` passed in the body. Verifies the caller can access that doc-name before signing (uses the same per-scope checks as the internal auth endpoint).
- Create: `platform/internal/handlers/realtime_auth.go` — internal-only `POST /api/internal/realtime/auth` (POST not GET because the body carries `documentName + sub` not query params) called by Hocuspocus during `onLoadDocument`. Gated by `Authorization: Bearer <HOCUSPOCUS_TOKEN_SECRET>`.
- Modify: `next.config.ts:10-36` — add a rewrite for `/api/realtime/:path*` so the client-side `POST /api/realtime/token` proxies to Go (currently absent — Phase 3 client fetch would 404 without it).
- Modify: `server/hocuspocus.ts` — verify JWT in `onAuthenticate`, call internal auth endpoint in `onLoadDocument`, reject on mismatch.
- Modify: every token-construction site listed above (six files) — fetch a token from `/api/realtime/token` instead of building the raw string. Cache token in-memory client-side; refresh on `exp - 60s`. Implement as a small shared helper `src/lib/realtime/get-token.ts` so all six sites share one cache.
- Add: `HOCUSPOCUS_TOKEN_SECRET` to `.env.example`, `docs/setup.md`, and the dev runbook.

**Test infrastructure (Codex Phase 0 follow-up):** the codebase has no Hocuspocus test harness today — neither vitest nor Go tests exercise the live WebSocket path. Plan 053 introduces it incrementally per phase (see the Phases section below for which test lands in which phase). Net additions across the four phases:
- A small Go in-process JWT verifier helper so subsequent phases can reuse parsing without booting Hocuspocus (Phase 1).
- `platform/internal/handlers/realtime_token_test.go` (Phase 1).
- `platform/internal/handlers/realtime_auth_test.go` (Phase 1).
- `tests/integration/realtime-token-mint.test.ts` — vitest end-to-end for `POST /api/realtime/token` (Phase 2).
- `e2e/hocuspocus-auth.spec.ts` — Playwright spec for the live mint → connect → verify path (Phase 3).

Update: `TODO.md:9` (Hocuspocus auth note).

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Token-mint endpoint adds latency to every realtime connect | low | Cache token in-memory client-side; refresh on `exp - 60s`. |
| Internal auth endpoint becomes a bypass surface | medium | Bearer-token gate + bind to a dedicated server-only port if needed. Document that the secret is environment-only, never client-shipped. |
| Existing in-flight sessions break when this rolls out | medium | Ship behind a feature flag (`HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1`). Stage off in dev first; flip on after a soak. |
| Parent viewer flow is fragile already (read-only stream) | low | Token claim has `role: "parent"` and the Go side returns the same `canJoinSession` answer; existing parent tests cover the flow. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch `codex:codex-rescue` on this plan focusing on (a) the JWT claim shape's compatibility with the existing Hocuspocus extension, (b) the internal-auth endpoint's bearer-token gate, (c) the feature-flag rollout. Capture verdict below. Iterate until concur.

### Phase 1: server-side mint + verify (Go)

- Implement `POST /api/realtime/token` and `POST /api/internal/realtime/auth` (POST on both — auth endpoint takes `{documentName, sub}` in the body, not query params).
- Add HMAC signing helpers reusing `golang-jwt/jwt/v5`.
- Tests added in this phase:
  - `platform/internal/handlers/realtime_token_test.go` — happy path + 403 for unauthorized scope + 401 for missing claims + boundary cases per doc-name shape (session/attempt/unit/broadcast).
  - `platform/internal/handlers/realtime_auth_test.go` — Bearer-token gate + per-doc-name dispatch + signature verification.
  - A small Go in-process JWT verifier helper so subsequent phases can reuse the parsing without booting Hocuspocus.
- Commit + push (no client changes yet — feature flag off).

### Phase 2: server-side verify (Hocuspocus)

- Update `server/hocuspocus.ts` to verify JWT signature when `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1`.
- Defense-in-depth `onLoadDocument` DB check via the new internal endpoint.
- **Backward-compat parsing (Codex Phase 0 follow-up):** with the flag OFF, the parser must accept BOTH the legacy `userId:role` shape (current behavior) AND a signed JWT (newly-deployed clients during the rollout window). Detect by string shape: if the token starts with `ey` (base64-encoded JWT header `{"alg":"HS256"}` always starts with `ey`) treat as JWT and verify; otherwise fall back to `split(":")`. With the flag ON, rejecting any unsigned token is unconditional.
- Tests added in this phase:
  - `tests/unit/realtime-jwt.test.ts` — Vitest unit coverage for the Node-side JWT verifier (`verifyRealtimeJwt`): round-trip, wrong secret, tampered payload, alg=none, wrong issuer, expired, future iat, missing claims, malformed, garbage body. Plus `rechckDocumentAccess` with mocked fetch covering 200/allow, 200/deny, 4xx, 5xx, network error.
  - **NOTE (revised vs Phase 0 plan):** the original plan also called for `tests/integration/realtime-token-mint.test.ts` as a Vitest end-to-end for `POST /api/realtime/token`. After landing, this is omitted because (a) the route has no Next.js file — it goes straight through the `next.config.ts` rewrite to Go; (b) Go integration tests in `platform/internal/handlers/realtime_token_test.go` already cover the endpoint exhaustively (22 cases); (c) no proxy-stub infrastructure exists in Bridge's Vitest setup, so a "mocked Go" test would test the mock, not the system. The full mint → connect → verify round-trip is best ratcheted by the Phase 3 Playwright e2e (`e2e/hocuspocus-auth.spec.ts`).

### Phase 3: client-side token fetch

- Add `src/lib/realtime/get-token.ts` — small shared helper that posts to `/api/realtime/token` with the desired `documentName`, caches the token in-memory, and refreshes on `exp - 60s`. Single source of truth so all six sites share one cache.
- Replace each of the six token-construction sites with a call to the helper.
- Tests added in this phase:
  - `e2e/hocuspocus-auth.spec.ts` — Playwright spec hitting the live Hocuspocus server with (a) a forged token (expect close), (b) a valid token (expect open), (c) an expired token (expect close). Integration ratchet for the full mint → connect → verify path.

### Phase 4: enable the flag in dev → staging → prod, retire the legacy path

**Status as of 2026-05-03: READY** — all 6 token construction sites
are migrated to `useRealtimeToken` (4 in plan 053 phase 3 + 2 in
plan 053b). Phase 4 is mostly operational (env flag flip + soak)
with one code-side cleanup at the end.

Pre-flight checklist (already shipped):
- [x] Go mint endpoint behind `HOCUSPOCUS_TOKEN_SECRET` (phase 1).
- [x] Hocuspocus verifies signed JWTs + DB recheck (phase 2).
- [x] Client mint helper + 4 callsite migrations (phase 3).
- [x] Plan 064 — parent-child linking schema + IsParentOf helper.
- [x] Plan 053b — teacher-watch + parent-viewer migrated; broadened
      `authorizeAttemptDoc` and `authorizeSessionDoc`.
- [x] Backward-compat parser sniffs `ey` prefix in
      `server/hocuspocus.ts` (legacy `userId:role` path stays
      active when `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN` is unset).

Operational rollout (needs real-time soak — NOT autonomously
deliverable):

1. **Set `HOCUSPOCUS_TOKEN_SECRET` in dev** (if not already). Verify
   the mint endpoint returns 200 and Hocuspocus accepts the JWT.
2. **Set `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN=1` in dev**. Soak for at
   least 24h. Watch error rates on the Hocuspocus side; failure
   modes look like "auth failed" and connection drops.
3. **Set the same in staging.** Soak for at least 7 days. Monitor:
   - Error rate on `/api/realtime/token` and Hocuspocus connect.
   - Any reports of teacher-watch / parent-viewer / unit-editor
     failures.
4. **Set the same in prod.** Watch the dashboards.

Code cleanup (queued for AFTER prod is stable, e.g. 7+ days post-
flip):

- Remove the unsigned fallback parser branch in
  `server/hocuspocus.ts::onAuthenticate` (the `else` after
  `isLikelyJwt(token)` check) and the `HOCUSPOCUS_REQUIRE_SIGNED_TOKEN`
  flag itself — once the legacy path is gone, the flag becomes
  vestigial.
- Delete the `ey`-prefix sniff (`isLikelyJwt`) and `tokenKind:
  "legacy"` paths.
- Update `e2e/hocuspocus-auth.spec.ts` to assert unsigned tokens
  are UNCONDITIONALLY rejected (currently the spec gates on
  `HOCUSPOCUS_TOKEN_SECRET` being set; post-cleanup it should also
  gate on the flag being ON in the test env, OR drop the gate
  entirely if cleanup happens after prod flip).
- Delete `server/attempts.ts::teacherCanViewAttempt` — only the
  legacy auth path called it; once the legacy path is gone, this
  TS helper has no caller. The Go-side
  `AttemptStore.IsTeacherOfAttempt` becomes the single source of
  truth.

This cleanup is its own PR, filed when prod is stable.

## Codex Review of This Plan

### Pass 1 — 2026-05-01: BLOCKED → fixes folded in

Codex Phase 0 review found 3 blockers + 3 [IMPORTANT] items, all addressed inline:

- `[CRITICAL]` Missing 6th token construction site at `src/components/problem/teacher-watch-shell.tsx:55`. **Fix:** added to the surface table.
- `[IMPORTANT/blocker]` Doc-name format mismatch — actual session doc names are `session:{sessionId}:user:{studentId}` not `session:{sessionId}`. **Fix:** Approach section now spells out the four exact doc-name shapes; JWT `scope` is the full doc-name string, byte-for-byte compared against `documentName`.
- `[IMPORTANT/blocker]` `next.config.ts` missing `/api/realtime/:path*` rewrite — Phase 3 client fetch would 404. **Fix:** Files section now lists the rewrite addition.
- `[IMPORTANT]` Phase 3 backward-compat: legacy `split(":")` can't parse a JWT during the rollout window. **Fix:** Phase 2 now describes a shape-sniffing parser (JWT starts with `ey`; else legacy split) for the flag-off period.
- `[IMPORTANT]` Multi-pod `HOCUSPOCUS_TOKEN_SECRET` rotation. **Fix:** Approach section documents single-key initial impl + `kid`-based rotation as a follow-up plan.
- `[IMPORTANT]` E2E test harness for Hocuspocus doesn't exist. **Fix:** Files section now describes the test infrastructure landed across Phases 1–3: Go in-process verifier helper + per-handler tests in Phase 1, vitest mint integration test in Phase 2, Playwright Hocuspocus auth e2e in Phase 3. Each phase's bullet list calls out which tests it adds.

**Status:** Pass-2 dispatch will confirm the resolutions land cleanly.

### Pass 5 — 2026-05-01: **PHASE-0 CONCUR**

After Passes 2-4 caught (a) test-harness placement inconsistency, (b) `GET` vs `POST` mismatch on the internal endpoint, (c) leftover "Phase 2" test-harness attribution, and (d) a stale-snapshot false alarm about "replay rejection," Pass 5 returned a clean concur.

**Status: ready for Phase 1 implementation** (server-side mint + verify in Go). PR-1 of plan 053 starts on a fresh branch.

---

## Phase 1 Post-Implementation Review (2026-05-02)

Phase 1 shipped on `fix/053-1-realtime-token-mint`. Three commits total:
`265432a` (initial), `d172c91` (pass-1 fixes), `cb91839` (pass-2 status-code fix).

### Pass 1 — 5 blockers, all fixed in `d172c91`

1. **Internal endpoint behind RequireAuth** — `/api/internal/realtime/auth`
   was registered inside the user-auth group, so the server-to-server
   callback got 401'd before its bearer-token check could run. **Fix:**
   split route registration: `Routes()` for the public mint endpoint
   stays under `RequireAuth`, `InternalRoutes()` registers the bearer-
   gated callback at the top level.

2. **Attempt scope had admin/impersonator bypass** — Phase 1 promised
   strict owner-only for attempt docs (teacher-watch path is deferred
   to Phase 2 alongside attempt → class resolution). **Fix:** removed
   the bypass. Test now asserts both admin and impersonator are
   denied.

3. **Broadcast scope was broader than the REST gate** —
   `authorizeBroadcastDoc` granted access to any class staff with
   `AccessMutate`, but `SessionHandler.ToggleBroadcast` only allows
   platform admin OR the session's teacher. **Fix:** dropped the
   class-staff path. New test `TestMintToken_BroadcastDoc_OrgAdminDenied`
   locks the parity down.

4. **Internal endpoint synthetic claims didn't carry admin status** —
   With the route split (blocker 1), the internal endpoint runs outside
   user auth, so it got only `body.Sub`. An admin demoted between mint
   and recheck would still pass because synthetic claims had
   `IsPlatformAdmin: false`. **Fix:** added `Users` field to
   `RealtimeHandler` and rebuild claims via `Users.GetUserByID`,
   reading `is_platform_admin` from the DB. `ImpersonatedBy`
   intentionally NOT rehydrated — impersonation is a session-level
   superpower; the recheck should evaluate the underlying user's
   actual permissions. New test
   `TestInternalAuth_RehydratesPlatformAdminFromDB` verifies it.

5. **docs/setup.md path mismatch** — doc said
   `/api/internal/realtime/authorize`; actual path is `/auth`. **Fix:**
   doc corrected and amended with the mount-location rationale.

### Pass 2 — 1 blocker, fixed in `cb91839`

1. **InternalAuth collapsed all errors into 200/Allowed:false** —
   Every `authDecision` failure (400 malformed doc-name, 404 missing
   resource, 500 DB error) turned into "200 + Allowed:false", hiding
   infrastructure problems as ordinary auth denials. **Fix:** split
   dispatch by `decision.Status`: only `403 Forbidden` collapses to
   `200 Allowed:false`; everything else surfaces via `writeError`.
   User-not-found is now `404` (not `200/Allowed:false`). Four new
   tests cover the paths:
   `TestInternalAuth_{UnknownSub_404, BadDocName_400,
   MissingResource_404, NilUsersStore_500}`.

### Pass 3 — **CONCUR**

> All four verification points confirmed: status dispatch correct,
> leaked-info acceptable (bearer gate filters non-Hocuspocus
> callers), test coverage sufficient, no regression on pass-1/2
> fixes. Phase 1 is clear to merge.

Two non-blocking gaps Codex noted: no test for `GetUserByID`
returning a non-nil error (vs nil result) and no test for
`authorizeDocument` returning `StatusInternalServerError`. Both are
exercised at the dispatcher level by `TestInternalAuth_NilUsersStore_500`;
producer-level coverage is belt-and-suspenders. Filed as a Phase 1
follow-up if a future regression demands it.

### Final test counts

- `platform/internal/auth`: 8 unit tests on `SignRealtimeToken` /
  `VerifyRealtimeToken` (round-trip, wrong secret, wrong issuer,
  expired, malformed, TTL clamp, ey prefix lock-in).
- `platform/internal/handlers`: 22 mint/internal-auth tests
  (auth required, all 4 doc-name shapes, owner/teacher/admin/
  impersonator matrices, bearer gate, status-code dispatch, DB
  rehydrate, missing user/resource).
- Full Go suite: green.

**Phase 1 status: ready to merge.** Phase 2 (server-side verify in
Hocuspocus + backward-compat parser) is the next plan-053 unit.

---

## Phase 2 Post-Implementation Review (2026-05-02)

Phase 2 shipped (PR #87, merged to main). Two commits:
`eba13e4` (initial), `ab48b03` (Codex pass-1 fix: plan rationale).

### Codex Pass 1 — 1 finding

> Phase 2 plan called for `tests/integration/realtime-token-mint.test.ts`
> (Vitest end-to-end through the Go-proxy stub). The actual ship had
> only Vitest unit coverage for the JWT verifier + recheck helper.

**Resolution (ab48b03):** plan updated to document why the integration
test was omitted: (a) `/api/realtime/token` has no Next.js route file
— it goes straight through the rewrite; (b) Go integration tests in
`platform/internal/handlers/realtime_token_test.go` already cover
the endpoint exhaustively (22 cases); (c) no Vitest proxy-stub
infrastructure exists in Bridge; a "mocked Go" test would test the
mock, not the system; (d) the full mint → connect → verify
round-trip is the Phase 3 Playwright e2e (`e2e/hocuspocus-auth.spec.ts`).

### Codex Pass 2 — **CONCUR**

Codex verified all three legs of the rationale against the codebase
and confirmed no pre-merge coverage gap. Phase 2 was ready to merge.

### Final test counts

- 14 Vitest unit tests for `verifyRealtimeJwt` (round-trip, wrong
  secret, tampered payload, alg=none, wrong issuer, expired, future
  iat, missing claims, malformed, garbage body).
- 6 Vitest unit tests for `rechckDocumentAccess` (200/allow,
  200/deny, 4xx, 5xx, network error, propagates).
- Full Vitest: 547 passed.
- Go suite unaffected (no Go changes in Phase 2).

---

## Phase 3 Post-Implementation Review (2026-05-02)

Phase 3 shipped on `fix/053-3-client-mint`. Commit `<TBD>`.

### Scope changes during execution

Three latent bugs surfaced. One was fixed inline; the other two are
deferred to plan 053b (which itself depends on plan 049 for one half).

1. **Broadcast scope was teacher-only (Phase 1 oversight) — FIXED INLINE.**
   In Phase 1, Codex pass-1 narrowed `authorizeBroadcastDoc` to mirror
   the REST broadcast handler `ToggleBroadcast` (admin OR session
   teacher). But broadcast docs are one-way: the teacher writes,
   class members read. Students need to mint a token to receive the
   broadcast — they couldn't under the Phase 1 narrow rule. Phase 3
   broadens `authorizeBroadcastDoc` to:
   - role="teacher" for platform admin OR session.TeacherID (write)
   - role="user" for any class member or session participant (read)

   Tests updated:
   `TestMintToken_BroadcastDoc_TeacherOK_StudentDenied` →
   `TestMintToken_BroadcastDoc_TeacherWrites_StudentReads`. New test
   `TestMintToken_BroadcastDoc_OrgAdmin_GetsReadRole` confirms
   org_admin gets reader role (not writer — start/stop is REST-gate-
   only).

2. **Teacher-watch attempt scope — DEFERRED to 053b.** The Phase 1
   owner-only rule plus a long-broken `teacherCanViewAttempt` query
   in `server/attempts.ts:72` (queries `problems.topic_id`, dropped
   in migration 0013) means teacher-watch can't migrate to the
   helper in Phase 3 without expanding the Go scope. Filed in
   `docs/plans/053b-teacher-watch-attempt-scope.md`.

3. **Parent-viewer auth — DEFERRED to 053b (depends on plan 049).**
   The Go `authorizeSessionDoc` has no parent path, and Bridge has no
   parent-child link in the DB (plan 049 was scheduled but didn't
   ship). The legacy Hocuspocus path accepts `role === "parent"`
   without checks — that IS the security hole this plan closes.
   Migrating requires plan 049's parent-child schema first.

**Phase 4 of plan 053 must NOT flip the flag in prod until plans
053b AND 049 ship.** With the flag flipped, both deferred sites lose
their legacy fallback and break.

### Final callsite tally

Of the 6 token-construction sites listed in the plan's failure-mode
table:

| # | File | Status |
|---|---|---|
| 1 | `teacher-dashboard.tsx` (session teacher) | Migrated |
| 2 | `student-session.tsx` (session student + broadcast) | Migrated |
| 3 | `live-session-viewer.tsx` (parent viewer) | Deferred (053b/049) |
| 4 | `problem-shell.tsx` (student attempt) | Migrated |
| 5 | `teacher-watch-shell.tsx` (teacher watch) | Deferred (053b) |
| 6 | `use-yjs-tiptap.ts` (unit editor) | Migrated |

4 sites migrated, 2 deferred. Plus prop-cleanup follow-on for
`student-tile.tsx` / `student-grid.tsx` / `broadcast-controls.tsx`
(each used to receive `token` via props from teacher-dashboard;
now each mints its own per-doc JWT internally).

### Files

**Created:**
- `src/lib/realtime/get-token.ts` — helper with per-doc-name cache,
  in-flight dedup, leeway-based refresh.
- `src/lib/realtime/use-realtime-token.ts` — React hook wrapper.
- `tests/unit/realtime-get-token.test.ts` — 13 tests (cache, dedup,
  refresh, error mapping, response validation).
- `tests/unit/use-realtime-token.test.tsx` — 4 hook tests
  (pending/initial, noop/empty, A→B clear, cancelled-on-unmount).
- `e2e/hocuspocus-auth.spec.ts` — 6 Playwright tests (3 HTTP mint +
  3 WS round-trip: valid/forged/expired).
- `docs/plans/053b-teacher-watch-attempt-scope.md` — follow-up.

**Migrated (4 sites):**
- `src/components/problem/problem-shell.tsx` — student attempt doc.
- `src/lib/yjs/use-yjs-tiptap.ts` — teacher unit doc.
- `src/components/session/student/student-session.tsx` — student
  session doc + broadcast.
- `src/components/session/teacher/teacher-dashboard.tsx` — teacher
  selected-student doc; dropped `token` prop pass-through.
- `src/components/session/student-tile.tsx` — each tile mints its
  own per-student-doc JWT (drop `token` prop).
- `src/components/session/student-grid.tsx` — drop `token` prop
  (no longer passing through).
- `src/components/session/broadcast-controls.tsx` — drop `token`
  prop, mint internally.

**Modified Go side (Phase 1 oversight fix):**
- `platform/internal/handlers/realtime_token.go` —
  `authorizeBroadcastDoc` now grants role=user to class members and
  session participants.
- `platform/internal/handlers/realtime_token_test.go` — broadcast
  test names + assertions updated; new `_OrgAdmin_GetsReadRole`
  test.

**Deferred (2 of the 6 callsites):**
- `src/components/problem/teacher-watch-shell.tsx` — kept legacy
  `${teacherId}:teacher` token. Phase 1 narrowed attempt scope to
  owner-only, plus the underlying `teacherCanViewAttempt` query is
  broken (drops `problems.topic_id` since plan 0013).
- `src/components/parent/live-session-viewer.tsx` — kept legacy
  `${parentId}:parent` token. There is no parent-child link in the
  DB (plan 049 was scheduled but didn't ship); the legacy
  Hocuspocus path accepts `role === "parent"` without checks, which
  IS the security hole this plan closes. Migrating requires plan
  049's parent-child schema first.

Both are tracked in `docs/plans/053b-teacher-watch-attempt-scope.md`.
Plan 053 phase 4 MUST NOT flip the flag in prod until 053b ships
(which requires plan 049 for the parent-viewer half).

### Codex Pass 1 — 3 blockers, all fixed

1. **Hook stale-token A→B race** — `use-realtime-token.ts` did not
   clear the previous doc's token before the new mint resolved, so
   a docName change passed the old scoped JWT into useYjsProvider
   for the new doc. Fixed: `setToken("")` synchronously at the top
   of the useEffect for non-noop docs. New regression test
   `tests/unit/use-realtime-token.test.tsx::CLEARS the previous
   token when docName changes`.

2. **e2e WS round-trip missing** — original spec was HTTP-only;
   plan promised forged + valid + expired WS coverage. Added three
   real WS tests using `@hocuspocus/provider` directly in the
   Playwright Node runner with the `ws` polyfill:
   - VALID JWT → `outcome: "connected"`
   - FORGED JWT (wrong secret) → not connected
   - EXPIRED JWT (correct sig, exp in past) → not connected
   All three skip gracefully when `HOCUSPOCUS_TOKEN_SECRET` is
   unset.

3. **Parent-viewer site missed** — plan listed it as the 3rd of 6
   construction sites at line 17. Added an inline deferral comment
   referencing plan 053b + extended 053b with bug 3 (parent-child
   linking dependency on plan 049).

### Codex Pass 2 — 2 blockers, both fixed in commit `<TBD>`

1. **WS test settled on `onConnect` instead of `onAuthenticated`.**
   Hocuspocus emits `connect` on the first server message, BEFORE
   the server has accepted/rejected the auth token. Forged/expired
   tests could spuriously settle as "connected" before the auth-
   failure event lands. Fix: settle the success path on
   `onAuthenticated`; failure paths still settle on
   `onAuthenticationFailed` / `onClose` / timeout.

2. **Plan inconsistency** — main file said "5 callsite refactors out
   of the planned 6" in one place, "2 of 6 deferred" in another.
   053b said "two latent bugs" while listing three. Fixed: this
   section now spells out 4 migrated + 2 deferred, with a per-row
   table; 053b retitled to "Deferred mint sites" with 3 bugs in
   scope.

### Codex Pass 3 — _pending_
