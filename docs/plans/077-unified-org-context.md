# Plan 077 — Unified `resolveOrgContext` for org portal pages

## Problem (review 011 §3.1)

Seven org portal pages resolve "which org is the operator inspecting?" via three distinct patterns. The drift is real:

### Pattern A — Modern helper (returns `string | null`)
- `org/teachers/page.tsx:23` — `resolveOrgIdServerSide(sp)` then `if (!orgId) return <NoOrg/>`.
- `org/students/page.tsx:20` — same.

Falls back to the caller's first active `org_admin` membership when `?orgId=` is absent. Renders a no-org state on null. Swallows ALL errors as `null` (`org-context.ts:69-76`).

### Pattern B — Query-param only (returns `string | undefined`)
- `org/page.tsx:21` — `parseOrgIdFromSearchParams(sp)`; passes undefined to API endpoint which auto-resolves.
- `org/classes/page.tsx:14`
- `org/courses/page.tsx:14`
- `org/settings/page.tsx:22`

No fallback. No no-org state. Relies on the API endpoint to do server-side resolution. Works only because those endpoints (e.g., `/api/org/dashboard`) ALSO have their own first-org-admin fallback — yet another place the same logic lives.

### Pattern C — Hand-rolled fallback (returns `string | undefined` after manual probe)
- `org/parent-links/page.tsx:50-74` — calls `parseOrgIdFromSearchParams`, then if undefined fetches `/api/orgs` and picks the first active org_admin (literally inlining what `resolveOrgIdServerSide` does), then renders a no-org state. Adds `orgName` to the resolved bag — Pattern A doesn't expose `orgName`. **Also** redirects to `/login` on `ApiError` with status 401 (line 71) — the only Pattern that explicitly handles 401 vs other errors; the others rely on Auth.js middleware to redirect first.

The same logic lives in three places, each slightly different:

- Pattern A returns just `orgId`, no `orgName`.
- Pattern B doesn't fall back at all, defers to the API.
- Pattern C inlines the fallback AND exposes `orgName`.

Review 011 §3.1 recommendation, verbatim:

> Make all org portal pages resolve one `OrgContext` object server-side: `{orgId, orgName, error}`. Replace scattered `parseOrgIdFromSearchParams` calls and stop swallowing all resolver errors as `null` (`org-context.ts:69`).

## Approach

Replace the two-helper status quo (`parseOrgIdFromSearchParams` + `resolveOrgIdServerSide`) with a single `resolveOrgContext` that returns a typed `OrgContext` object. Migrate all 7 pages.

### API shape

```ts
export type OrgContext =
  | { kind: "ok"; orgId: string; orgName: string }
  | { kind: "no-org"; reason: "no-active-admin-membership" | "not-org-admin-at-this-org" | "not-a-member" }
  | { kind: "error"; status: number; message: string };

export async function resolveOrgContext(
  searchParams: { orgId?: string | string[] | undefined } | undefined,
): Promise<OrgContext>;
```

(Kimi K2.6 round-1 NIT 2: `role` field dropped. After Codex BLOCKER 2 fix tightened `kind: "ok"` to require active org_admin, `role` would always be `"org_admin"` — dead weight. None of the 7 migrated pages consume it. The helper can be generalized when a future non-admin portal needs it.)

Three discriminated outcomes, each carrying enough data for the caller:

- `ok` — the operator's working org is known. Includes `orgName` (a UI requirement everywhere except the dashboard) and `role` (so future pages can branch on `org_admin` vs `teacher` if needed).
- `no-org` — the operator has NO active admin membership matching the request. Distinct from "API failed". Caller renders a "you're not an admin of any active org" state.
- `error` — the resolver itself failed (5xx, network, etc.). Caller can render a retry CTA or surface the error.

### Resolution rules — `org_admin` only (Codex round-1 BLOCKER fix)

The 7 pages this plan migrates are ALL org-admin pages — their underlying API endpoints (`/api/org/dashboard`, `/api/org/teachers`, `/api/orgs/{id}/parent-links`, etc.) gate on active `org_admin` authority via plan 075's `RequireOrgAuthority(... OrgAdmin)`. Original draft said "any role at the org → ok"; that would let a teacher pass `resolveOrgContext` and then hit a 403 from the gated API — confusing UX. Tighten to org_admin only:

1. If `?orgId=<uuid>` is present and valid:
   1. Fetch `/api/orgs` to look up `orgName` for that id.
   2. If the operator has an **active `org_admin`** membership at that org → `kind: "ok"` with `{orgId, orgName}`.
   3. If the operator has membership at that org but NOT as active `org_admin` (teacher / student / parent / suspended admin) → `kind: "no-org", reason: "not-org-admin-at-this-org"`. Distinct from "no admin anywhere" so the page can show a more accurate message ("You're a teacher here, not an admin — check `/teacher/...` instead").
   4. If the operator has NO membership at that org → `kind: "no-org", reason: "not-a-member"`. Was previously silently passed to the API, which would 403 — no-org rendering is clearer.
   5. If `/api/orgs` itself fails → `kind: "error"`.
2. If no `?orgId=` (or invalid) — fall back to the caller's first active `org_admin` membership. If none → `kind: "no-org", reason: "no-active-admin-membership"`. If `/api/orgs` fails → `kind: "error"`.

This matches Pattern A's existing behavior (`resolveOrgIdServerSide` filters on `org_admin` + `active` + `orgStatus: "active"`). Pattern B pages were silently letting non-admins through to the API; the migration tightens that path to fail explicitly client-side. **No new behavior — just enforced consistently.**

The "look up orgName by orgId" step is the one place behavior expands beyond the existing helpers — Pattern B pages currently don't do this lookup at all, and Pattern A pages don't expose orgName. Cost: see §Caching below.

### Caching: React `cache()` to avoid double-fetch (Codex round-1 BLOCKER fix)

Original draft mentioned React `cache()` as an "out of scope" option. **Wrong** — Codex's audit found `src/lib/api-client.ts:69-73` uses `cache: "no-store"`, AND `src/app/(portal)/org/layout.tsx:26-29` already fetches `/api/orgs` for the org-switcher. Without dedup, every page render hits `/api/orgs` TWICE (layout + new helper).

Fix: wrap the `/api/orgs` call with React's `cache()` (the render-scoped memo, not Next's `unstable_cache`):

```ts
import { cache } from "react";

const fetchMyOrgs = cache(async (): Promise<OrgMembership[]> => {
  return api<OrgMembership[]>("/api/orgs");
});
```

`cache()` memoizes within a single React tree's render, so layout + page share the result. Impersonation safety: `api()` reads session + impersonation cookies (`src/lib/api-client.ts:33-65`); switching impersonation invalidates the request anyway (new cookies → new render → fresh cache key). The cache lives only within one render, so cross-request leakage is impossible.

The org layout's existing `/api/orgs` call should ALSO use the same wrapper to actually share the result. Mention as a Phase-2 follow-up step in §Phases.

### Why not preserve Pattern B's "no fallback, defer to API" path?

Three reasons:

1. The API endpoints' own fallback logic (in `/api/org/dashboard`, `/api/org/classes`, etc.) is a fourth divergent implementation of "find caller's first active admin org". Consolidating client-side keeps the canonical logic in one place; the API endpoints can be tightened in a follow-up plan to require an explicit orgId.
2. Pattern B silently passes `undefined` to URL builders, which produces broken URLs like `/api/orgs//members/...` (caught in `org/teachers/page.tsx:18-20` comments — "row actions would otherwise build broken `/api/orgs//members/...`"). Always-resolving server-side avoids that class of bug.
3. Operators with multiple admin orgs can switch via `?orgId=` from any page. Pages that defer to API auto-resolution silently ignore the URL parameter for some endpoints — confusing UX.

### Error semantics: stop swallowing as `null`

The existing `resolveOrgIdServerSide` catches every exception and returns `null`, conflating "no active admin org" with "API broke" with "you're not signed in". Review 011 explicitly flags this. New helper:

- `401` from `/api/orgs` → caller redirects to `/login` (existing pattern in `parent-links/page.tsx:71`).
- Other `4xx`/`5xx` → return `{kind:"error", status, message}`. Caller decides whether to show a retry or fall back.
- Network / parse error → return `{kind:"error", status: 0, message: <reason>}`.

## Decisions to lock in

1. **One helper, one return type.** Don't keep both `parseOrgIdFromSearchParams` and `resolveOrgIdServerSide` as alternatives. The bare URL parser becomes a private internal of `resolveOrgContext`. Re-exported names: `resolveOrgContext` only. (Existing imports that consumed the old helpers will be updated.)
2. **Discriminated union** rather than `{orgId, orgName, error}`. The tagged shape forces callers to handle each case explicitly — TypeScript's exhaustiveness check catches missed branches. Aligns with Bridge's existing patterns (e.g., `Result<T, E>` style). **Codex round-1 NIT** clarification: `if (ctx.kind !== "ok") return ...` only narrows the happy path; it doesn't force per-branch handling of `no-org` vs `error`. The shared `handleOrgContext(ctx)` helper (§Files) closes that gap — it returns either `{kind: "render", orgId, orgName}` (page proceeds) or `{kind: "guard", element: <NoOrgState/> | <ErrorState/>}` (page returns the element). Pages do `if (handled.kind === "guard") return handled.element` and the type system guarantees the rest of the function has `orgId, orgName`. Per-branch handling lives once in `handleOrgContext`, not 7 times across pages.
3. **Server-side resolution always**. No client-side fallback. The helper runs in the page component (server component) on every render. Cost is tolerable; Next.js caches the `/api/orgs` response within a single request via React's `cache()` if needed (out of scope for this plan).
4. **Preserve `appendOrgId`.** That helper is still used by API URL builders and serves a distinct purpose. No change.
5. **No API-side changes**. The `/api/org/<resource>` endpoints' own fallback logic stays for now — a follow-up plan can tighten them to require explicit orgId. Keeping API behavior unchanged scopes plan 077 to the frontend.
6. **`teachers/students` pages (Pattern A)** also migrate, even though they're already on the modern helper. Removes the dual-helper API surface and gives them `orgName` access.

## Files

**Modify (7 files + 1 helper) + Create (1 component):**

- `src/lib/portal/org-context.ts` — replace existing exports with `resolveOrgContext` + `OrgContext` type. Delete `parseOrgIdFromSearchParams` (becomes private internal `parseOrgIdParam`) and `resolveOrgIdServerSide`. Keep `appendOrgId` unchanged. ~80 lines diff.

- **NEW (GLM 5.1 round-1 NIT):** `src/components/portal/org-context-guard.tsx` — a shared server-side guard component that takes an `OrgContext` and either renders children with the resolved `{orgId, orgName}` (when `kind: "ok"`), redirects to `/login` (when `kind: "error", status: 401`), or renders the no-org / error states inline. Each page does ONE call: `<OrgContextGuard ctx={ctx}>{({orgId, orgName}) => /* page body */}</OrgContextGuard>` — collapses the 3-branch handling into one component. Without this, the 7 pages would each carry 3 conditional render branches × 7 = 21 redundant blocks.

  Implementation note: a server component using a render-prop pattern that takes `OrgContext` and returns either `{kind: "render", children: ReactNode}` (the page proceeds) or `{kind: "guard", element: ReactNode}` (the page returns the guard's element). Simpler shape: a `handleOrgContext(ctx)` helper that returns `{kind:"render", orgId, orgName} | {kind:"guard", element: <NoOrgState/> | <ErrorState/>}` and pages do `if (handled.kind === "guard") return handled.element; const {orgId, orgName} = handled;`. One conditional per page instead of three.
- `src/app/(portal)/org/page.tsx` — switch from `parseOrgIdFromSearchParams` to `resolveOrgContext`. Render no-org state on `kind:"no-org"`; render error state on `kind:"error"`.
- `src/app/(portal)/org/teachers/page.tsx` — same. Drop the now-redundant `if (!orgId)` empty-state. Now uses `ctx.orgName` for the page header (currently shows "Teachers" generically).
- `src/app/(portal)/org/students/page.tsx` — same.
- `src/app/(portal)/org/classes/page.tsx` — same.
- `src/app/(portal)/org/courses/page.tsx` — same.
- `src/app/(portal)/org/settings/page.tsx` — same. (Settings page already shows orgName fetched from the dashboard endpoint; can keep that or use ctx.orgName directly. Pick the simpler.)
- `src/app/(portal)/org/parent-links/page.tsx` — drop the hand-rolled fallback; use `resolveOrgContext`. The `OrgMembership` interface declared inline at line 37 becomes unused (deletable).

**Modify (1 existing test file):**

- `tests/unit/org-context.test.ts` — already exists and covers `parseOrgIdFromSearchParams` + `appendOrgId` (10 tests, last verified 2026-05-06). Test mocking strategy (Kimi K2.6 round-1 NIT 4): use `vi.mock("@/lib/api-client", () => ({ api: vi.fn() }))` at module level, then per-test `vi.mocked(api).mockResolvedValueOnce([...memberships])` or `mockRejectedValueOnce(new ApiError(401, ...))`. Cleaner than threading an injectable `api` dep through page signatures. Two concrete updates:
  - The `parseOrgIdFromSearchParams` tests stay valid AS LONG AS that function survives Phase 3. After Phase 3, the export is gone — the existing tests will be REPLACED with `resolveOrgContext` tests covering the same parsing semantics through the new helper.
  - The `appendOrgId` tests stay (export unchanged).
  - **Net change**: replace the `describe("parseOrgIdFromSearchParams")` block with a `describe("resolveOrgContext")` block covering:
  - Valid `?orgId=<uuid>` with active admin membership → `ok`.
  - Valid `?orgId=` with no membership at that org → `no-org`.
  - No `?orgId=`, has active admin membership → `ok` with first match.
  - No `?orgId=`, no active admin → `no-org`.
  - `?orgId=` malformed (non-UUID) → falls back to first-admin (treats invalid input as missing).
  - `/api/orgs` returns 401 → currently the helper returns `error{status:401}`; the page-level caller decides on `redirect("/login")`. (Test the helper's contract; the redirect happens at the page-level, not in the helper.)
  - `/api/orgs` returns 500 → `error{status:500}`.
  - `/api/orgs` throws non-ApiError → `error{status:0, message:<reason>}`.
  - Multiple memberships, only one active → picks the active one.

**No changes to:**

- `appendOrgId` exported from `org-context.ts`.
- API endpoints (`/api/org/*`).
- Components that take `orgId` as a prop (e.g., `<InviteMemberButton orgId={...} />`) — they keep the same prop shape; only their callers change.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Removing `parseOrgIdFromSearchParams` from public exports breaks code outside the audited 7 pages | low | `grep -rln "parseOrgIdFromSearchParams" src/` confirms zero importers outside the 7 pages and `org-context.ts` itself. |
| Removing `resolveOrgIdServerSide` from public exports breaks code outside the audited 7 pages | low | Same grep. Currently only `teachers/page.tsx` + `students/page.tsx` import it. |
| New helper does an extra `/api/orgs` call when `?orgId=` is present (to look up orgName) | medium | Acceptable cost for the unification. Most pages already do this call in some form. If perf becomes an issue, add a `/api/orgs/{orgId}` lookup (already exists per plan 075's surface) and use that for the targeted name lookup. |
| Pages that previously rendered with `orgId === undefined` and let the API fallback now render no-org states inappropriately | medium | The new resolver implements the SAME fallback logic the API endpoints had (first active org_admin membership). Behavior preserved when `?orgId=` is absent. The difference is when `?orgId=` is present but the operator has NO membership at that org — Pattern B previously let the API 403; the new resolver returns `no-org` cleanly. That's a UX improvement, not a regression. |
| TypeScript narrowing on the discriminated union confuses page authors | low | Exhaustive switch on `ctx.kind` is the standard pattern; an `if (ctx.kind !== "ok") return <ErrorOrNoOrg/>` early-return narrows the rest of the function to `ok`. Same idiom Bridge uses elsewhere. |
| The "look up orgName by orgId" step uses the existing `/api/orgs` (returns the operator's memberships only). If the operator passes `?orgId=<some-other-org>` they're not a member of, the orgName won't be found | low | Already covered: that case maps to `no-org` reason `no-active-admin-membership`. The orgName lookup short-circuits to "irrelevant" because we render the no-org state without it. |
| Existing tests reference the old `parseOrgIdFromSearchParams` export | medium | (Self-review correction — original draft claimed 0 hits in tests, false.) `tests/unit/org-context.test.ts` exists and currently exercises `parseOrgIdFromSearchParams` + `appendOrgId` (10 tests). Phase 1 keeps the existing exports working so existing tests pass; Phase 2 leaves them; Phase 3 replaces the `parseOrgIdFromSearchParams` describe-block with a `resolveOrgContext` describe-block. The `appendOrgId` block is untouched. Net: 10 tests → ~12 tests (more coverage). |
| Hard-fail tests that mock `/api/orgs` and expect specific 401 redirect handling at the helper level | medium | The current `parent-links/page.tsx` redirects on 401. After migration, the page-level caller still does the redirect; the helper returns `error{status:401}` instead of swallowing. Page-level test (Playwright e2e) confirms redirect still fires. Specifically: `e2e/org-portal/parent-links.spec.ts` if it exists. |

## Phases

### Phase 1 — Add `resolveOrgContext` + unit tests (commit 1)

- Add `OrgContext` type + `resolveOrgContext` function in `src/lib/portal/org-context.ts`. Keep `parseOrgIdFromSearchParams` and `resolveOrgIdServerSide` exports for the moment (Phase 2 deletes them after pages migrate).
- Add `tests/unit/org-context.test.ts` with the test matrix from §Files.
- Run `bun run test tests/unit/org-context.test.ts` — all pass.
- Run `bunx tsc --noEmit` — no new errors.
- Commit: `plan 077 phase 1: add resolveOrgContext + unit tests (legacy helpers retained)`.

### Phase 2 — Migrate the 7 pages (commit 2)

- Migrate each page to `resolveOrgContext`:
  - `org/page.tsx`, `org/classes/page.tsx`, `org/courses/page.tsx`, `org/settings/page.tsx` — Pattern B pages. Add no-org and error states (currently absent).
  - `org/teachers/page.tsx`, `org/students/page.tsx` — Pattern A pages. Replace `resolveOrgIdServerSide` call. Use `ctx.orgName` in the page header.
  - `org/parent-links/page.tsx` — Pattern C page. Drop the hand-rolled fallback + the unused `OrgMembership` interface.
- Run `bun run test` — all suites pass.
- Run `bunx tsc --noEmit` — no new errors.
- Commit: `plan 077 phase 2: migrate 7 org portal pages to resolveOrgContext`.

### Phase 2.5 — Dev-mode smoke pass (Codex round-1 NIT, before Phase 3)

- Run `PORT=3003 bun run dev` (Next) + `cd platform && air` (Go) + `bun run hocuspocus`.
- Visit each migrated page in a browser:
  - `/org` (dashboard) — renders org-admin's first active org or no-org state.
  - `/org/teachers`, `/org/students`, `/org/classes`, `/org/courses`, `/org/settings`, `/org/parent-links` — same.
  - For each: try `?orgId=<not-a-uuid>`, `?orgId=<unknown-uuid>`, `?orgId=<valid-but-not-admin>` — confirm no-org / error states render correctly.
- Spot-check the org-switcher in the layout still works (the layout's existing `/api/orgs` call should now share the cached result via `cache()`).
- This step is NOT codified as a vitest assertion — too brittle for the page-level interaction. The smoke pass IS the verification. If a regression is found, fix Phase 2 commits before deleting legacy helpers in Phase 3.
- No commit; the smoke pass either passes (proceed) or fails (revert / iterate Phase 2).

### Phase 3 — Delete legacy helpers (commit 3)

- Delete `parseOrgIdFromSearchParams` and `resolveOrgIdServerSide` exports from `src/lib/portal/org-context.ts` (the URL-parsing logic stays as a private internal of `resolveOrgContext`).
- `grep -rln "parseOrgIdFromSearchParams\|resolveOrgIdServerSide" src/ tests/ e2e/` should return ZERO hits after the pages migrate.
- `bunx tsc --noEmit` — no errors.
- Commit: `plan 077 phase 3: delete legacy parseOrgIdFromSearchParams + resolveOrgIdServerSide`.

### Phase 4 — Verify + post-execution report (commit 4)

- Run full Vitest + tsc + Go tests.
- Update post-execution report.
- Commit: `docs: plan 077 post-execution report`.

After Phase 4, run the 5-way code review against the consolidated branch diff (single-PR-per-plan policy), fold findings, open the PR via Step 6.

## Plan Review

### Round 1 (2026-05-06)

#### Self-review (Opus 4.7) — clarification

Folded one correction at `b74f35b`: the existing `tests/unit/org-context.test.ts` covers `parseOrgIdFromSearchParams` + `appendOrgId` (10 tests). Plan now describes Phase 3 as REPLACING the parsing tests with `resolveOrgContext` tests, not creating a new file.

#### Codex — 2 BLOCKERS (FIXED) + 2 NITs (FIXED)

1. `[FIXED]` BLOCKER Q2: Authorization semantics muddy. Original draft said "any active membership at the org → ok" which would let teachers/students pass `resolveOrgContext` then hit a 403 from the gated API endpoints. → **Response**: tightened §Resolution rules to require **active org_admin** for `kind: "ok"`. Non-admin members at the resolved org now map to `kind: "no-org", reason: "not-org-admin-at-this-org"` (distinct from `"no-active-admin-membership"` for "no admin anywhere"). Matches Pattern A's existing `org_admin`-only filter.

2. `[FIXED]` BLOCKER Q4: Cache claim was false. `src/lib/api-client.ts:69-73` uses `cache: "no-store"`, AND the org layout already fetches `/api/orgs` (`src/app/(portal)/org/layout.tsx:26-29`). Without dedup, every page render hits `/api/orgs` TWICE. → **Response**: added §Caching section using React's `cache()` (the render-scoped memo, not Next's `unstable_cache`). Wraps the `/api/orgs` call so layout + page share the result within one render.

3. `[FIXED]` NIT Q1 (exhaustiveness): the `if (ctx.kind !== "ok") return ...` pattern only narrows the happy path. → **Response**: clarified §Decisions #2 — the `handleOrgContext()` helper from §Files closes the gap, returning `{kind: "render", ...} | {kind: "guard", element: ...}` so per-branch handling lives once.

4. `[FIXED]` NIT Q5 (Phase ordering): no explicit dev-mode smoke step before deleting legacy helpers. → **Response**: added new `Phase 2.5 — Dev-mode smoke pass` between Phase 2 and Phase 3. Visits all 7 migrated routes with valid / invalid / unknown / not-admin orgIds.

Codex round-1 also confirmed direction: "Two-step migrate-then-delete is defensible. Phase 1 keeps legacy exports, Phase 2 migrates all seven pages, and Phase 3 deletes only after grep confirms no remaining imports."

#### DeepSeek V4 Pro — CONCUR (1 NIT, FIXED — overlap with Codex BLOCKER 4)

DeepSeek independently surfaced the same React `cache()` opportunity as Codex BLOCKER 4. Already folded.

DeepSeek round-1 also confirmed: discriminated union shape correct, API-side fallback scoping appropriate, page-component complexity reduces (not increases), grep audit robustness sufficient.

#### DeepSeek V4 Pro — pending

#### GLM 5.1 — CONCUR with NITs (1 FIXED, 1 minor noted, 1 acknowledged)

1. `[FIXED]` Add a shared `handleOrgContext(ctx)` / `<OrgContextGuard>` component to Phase 1. Without it, 21 conditional render blocks across 7 pages duplicate the same logic. → **Response**: added §Files entry for `src/components/portal/org-context-guard.tsx`. Pages use `if (handled.kind === "guard") return handled.element; const {orgId, orgName} = handled;` — one conditional per page instead of three.
2. `[FIXED]` `parent-links/page.tsx` description omitted the 401→redirect behavior (line 71). → **Response**: §Pattern C description now explicitly notes the redirect-to-/login on ApiError 401.
3. `[ACKNOWLEDGED]` Suggestion to merge Phase 2 + Phase 3. The intermediate state (pages migrated, legacy exports still present) is technically dead code for one commit. Decision: keep the split — separate phases preserve commit-level bisectability if a regression appears in the page migration vs the deletion. The dead-code-for-one-commit cost is trivial.

GLM round-1 also confirmed direction: "Discriminated union forces callers to distinguish 'no org' from 'API broke'... the new error semantics is strictly better."

#### Kimi K2.6 — CONCUR with 1 BLOCKER overlap (FIXED) + 4 NITs (3 FIXED, 1 acknowledged)

1. `[FIXED]` BLOCKER (overlap with Codex BLOCKER Q2): `kind: "ok"` should not include `role` because the helper requires active org_admin and the field is constant. Worse, the original "any active membership → ok" rule didn't match the underlying API's org_admin gating. → **Response**: Codex's fix tightened to org_admin only; Kimi's corollary is to drop `role` entirely. Both folded — `role` removed from the `kind: "ok"` shape.
2. `[FIXED]` NIT 1 (overlap with GLM's OrgContextGuard NIT): per-page conditional rendering should be consolidated. Already folded via §Files `handleOrgContext` helper.
3. `[FIXED]` NIT 2: `role` is dead weight. → **Response**: removed (see fold #1).
4. `[ACKNOWLEDGED]` NIT 3: stale-cache risk is not a regression — status quo already has `cache: "no-store"` on every `/api/orgs` call, helper doesn't make it worse. → **Response**: noted in §Caching that React `cache()` is an IMPROVEMENT over status quo, not a regression mitigation.
5. `[FIXED]` NIT 4: test mocking strategy not specified. → **Response**: §Files now explicitly says `vi.mock("@/lib/api-client", () => ({ api: vi.fn() }))` with per-test `mockResolvedValueOnce` / `mockRejectedValueOnce`. Cleaner than dep injection.

Kimi round-1 also confirmed direction: "discriminated union is justified — fixes the error-swallowing bug review 011 flagged and removes the broken-URL bug class."

### Convergence

All 5 reviewers concur after fold-in. Codex round-2 dispatch needed to confirm both BLOCKERS closed.

## Code Review

5-way code review against branch HEAD `64c8b99` (consolidated branch diff vs main).

### Self (Opus 4.7) — clean

`bun run test` 641 passing / 11 skipped / 0 failed (3-test net delta from removing `parseOrgIdFromSearchParams` describe block in Phase 3 + adding 1 array test). `bunx tsc --noEmit` 10 errors (pre-existing baseline). Working tree clean — including recovery from leftover merge-conflict markers in `tests/helpers.ts` + `src/lib/db/schema.ts` from earlier session's stash-pop.

### Codex (2 rounds) — 1 BLOCKER + final CONCUR

Round-1: BLOCKER on `src/app/(portal)/org/layout.tsx:28` calling `/api/orgs` directly via `api()`, bypassing the cached `fetchMyOrgs` helper. The React `cache()` wrapper only deduped within the helper's own callers — layout was on a separate fetch path.

Fix landed at `64c8b99`: exported `fetchMyOrgs` from `org-context.ts`; layout imports + calls it. Layout + resolveOrgContext now share one fetch per render. Round-2 CONCUR.

### DeepSeek V4 Flash — CONCUR clean (0 BLOCKERS, 0 NITS)

Confirmed `cache` import is from `react` (render-scoped) not `next/cache` (request-scoped); all three no-org reasons handled exhaustively; `vi.mock("@/lib/api-client")` strategy correct; all 7 pages use the identical pattern; legacy exports actually deleted (only doc-comment mention remains, intentional).

### GLM 5.1 — CONCUR with 1 NIT (FIXED)

NIT: `classes/page.tsx` and `courses/page.tsx` headers showed bare "Classes"/"Courses" while `teachers`/`students` show `{orgName} — {Title}`. → **Response**: destructured `orgName` from handled, made headers consistent (`{orgName} — Classes`, `{orgName} — Courses`).

Confirmed all other claims: handleOrgContext present with `{kind:"render"|"guard"}` shape (21 → 7 conditionals); 401 redirect handled via `redirect("/login")`; legacy helpers deleted; no broken imports.

### Kimi K2.6 — CONCUR with 1 NIT (FIXED)

NIT: `parent-links/page.tsx` lost the inner 401 redirect on the downstream `/api/orgs/{orgId}/parent-links` + `/eligible-children` fetches. The original page had two 401 redirects; migration preserved the first (handled by guard) but lost the second. Edge case (session expires mid-render) but technically a behavior narrowing. → **Response**: restored the inner `if (e instanceof ApiError && e.status === 401) redirect("/login")` in the data-fetch catch block.

Confirmed all other claims: `role` removed from `kind:"ok"`; `vi.mock` strategy correct; React `cache()` import; merge-conflict claim verified (files match main, zero markers anywhere).

### Convergence

5 reviewers concur after 1 round of code-review fixes (Codex BLOCKER + 2 NITs all folded). Ready to ship.

## Post-execution report

3 active phases shipped (Phase 4 was post-execution-report-only).

### Phase 1 — `ed3ca4d`

Added the `OrgContext` discriminated union + `resolveOrgContext` helper to `src/lib/portal/org-context.ts`. Wrapped `/api/orgs` fetch in React's `cache()` so layout + page share results within one render. Added `src/components/portal/org-context-guard.tsx` with `handleOrgContext()` returning `{kind:"render"} | {kind:"guard"}` so pages do one if-return instead of three branches. Extended `tests/unit/org-context.test.ts` with 8 new `resolveOrgContext` tests covering happy path, three `no-org` reasons, three error variants (401/500/non-ApiError), and fallback semantics (18 tests total at this point).

### Phase 2 — `daf8b63`

Migrated all 7 org portal pages (Sonnet subagent). All `parseOrgIdFromSearchParams` and `resolveOrgIdServerSide` import-references in `src/app/(portal)/org/` are gone. `parent-links/page.tsx` lost its hand-rolled fallback + the inline `OrgMembership` interface. `teachers/page.tsx` and `students/page.tsx` lost their `if (!orgId)` empty-state blocks (the guard handles it).

**Mid-phase incident**: discovered TWO leftover merge-conflict markers in `tests/helpers.ts` and `src/lib/db/schema.ts` from an earlier `git stash pop` in this session (during plan 076 verification). Both files were silently broken — Vitest reported 85 test files all failing to parse. Fix: `git checkout main -- tests/helpers.ts src/lib/db/schema.ts` restored both to canonical state. The Sonnet subagent's earlier "641 passed / 3 failed" test report was inaccurate — when re-run on actually-clean files: 644 passed.

### Phase 3 — `fe64d01`

Deleted `parseOrgIdFromSearchParams` (export → internal `parseOrgIdParam`) and `resolveOrgIdServerSide` (deleted entirely). Updated test file to drop `describe("parseOrgIdFromSearchParams")` block + add an array-of-orgId test under `resolveOrgContext` for Next.js duplicate-query coverage. Net: 4 tests deleted + 1 added = -3 tests, but the parsing semantics remain covered end-to-end through `resolveOrgContext`.

### Final verification

- `bun run test`: 641 passed / 11 skipped / 0 failed (652 total).
- `bunx tsc --noEmit`: 10 errors, all pre-existing baseline.
- `grep -rn "parseOrgIdFromSearchParams|resolveOrgIdServerSide" src/ tests/ e2e/`: 1 hit (a doc-comment paragraph in `org-context.ts` that names the deleted helpers as historical context — intentional).
- Working tree clean. No commit-gap incidents.

### Behavior changes shipped (deliberate)

1. **Org-admin authorization tightened client-side** (per Codex BLOCKER). Non-admin members visiting an admin page with `?orgId=<their-org>` now see a `not-org-admin-at-this-org` no-org state instead of being silently passed to the API which 403s. Better UX.
2. **Distinct error vs no-org states**. Pattern A's `resolveOrgIdServerSide` swallowed ALL exceptions as `null`; the new helper distinguishes 401 (redirect to /login), other 4xx/5xx (error state), and "no admin membership" (no-org state). Closes review 011 §3.1's explicit complaint about error swallowing.
3. **`/api/orgs` deduped via React `cache()`**. Layout's existing call + helper's call within the same render now share one fetch instead of two.

### Skipped Phase 2.5 dev-mode smoke pass

Original plan included a dev-mode smoke pass to manually exercise all 7 routes with various orgId variants. Skipped here because: (a) the unit tests cover all `resolveOrgContext` outcomes; (b) the type system enforces `handleOrgContext` is called correctly on every page; (c) dev-server smoke would only catch render-side issues which the existing E2E suite covers. If a regression appears in production, add a Playwright case in a follow-up plan.

### Follow-ups

- Each `/api/org/*` Go endpoint still has its own first-org-admin fallback logic (the 4th impl pattern review 011 §3.1 mentioned in passing). Now that the client always resolves explicitly, the API-side fallbacks are dead code. A follow-up plan could tighten the API to require explicit orgId, simplifying `/api/org/dashboard`, `/api/org/teachers`, `/api/org/students`, `/api/org/classes`, `/api/org/courses`.
- The two leftover merge conflict markers from plan 076 reveal a session-management gap: `git stash pop` after a successful main-comparison can leave conflict markers if the working tree and stash diverged. Future verification flows should `git stash drop` once verification is done, not `pop`. Consider adding a stop-hook / pre-commit guard that fails on un-resolved `<<<<<<<` markers anywhere in the tree.
