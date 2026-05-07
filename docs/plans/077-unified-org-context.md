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
- `org/parent-links/page.tsx:50-74` — calls `parseOrgIdFromSearchParams`, then if undefined fetches `/api/orgs` and picks the first active org_admin (literally inlining what `resolveOrgIdServerSide` does), then renders a no-org state. Adds `orgName` to the resolved bag — Pattern A doesn't expose `orgName`.

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
  | { kind: "ok"; orgId: string; orgName: string; role: string }
  | { kind: "no-org"; reason: "no-active-admin-membership" }
  | { kind: "error"; status: number; message: string };

export async function resolveOrgContext(
  searchParams: { orgId?: string | string[] | undefined } | undefined,
): Promise<OrgContext>;
```

Three discriminated outcomes, each carrying enough data for the caller:

- `ok` — the operator's working org is known. Includes `orgName` (a UI requirement everywhere except the dashboard) and `role` (so future pages can branch on `org_admin` vs `teacher` if needed).
- `no-org` — the operator has NO active admin membership matching the request. Distinct from "API failed". Caller renders a "you're not an admin of any active org" state.
- `error` — the resolver itself failed (5xx, network, etc.). Caller can render a retry CTA or surface the error.

### Resolution rules (preserve current behavior)

1. If `?orgId=<uuid>` is present and valid:
   1. Fetch `/api/orgs` to look up `orgName` for that id.
   2. If the operator has an active `org_admin` membership at that org → `ok`.
   3. If the operator has any other active membership at that org (teacher/student/parent) → `ok` with that role (matches Pattern A's accept-anything behavior; gives the helper room to also serve teacher/student pages, but they keep their existing flows for now).
   4. If the operator has NO membership at that org → `no-org` (was previously silently passed to the API, which would 403).
   5. If `/api/orgs` itself fails → `error`.
2. If no `?orgId=` (or invalid) — fall back to the caller's first active `org_admin` membership. If none → `no-org`. If `/api/orgs` fails → `error`.

The "look up orgName by orgId" step is the one place behavior expands beyond the existing helpers — Pattern B pages currently don't do this lookup at all, and Pattern A pages don't expose orgName. Cost: one extra call per page render, but most pages already call `/api/orgs` in some form (the existing `resolveOrgIdServerSide` does it for fallback-resolution).

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
2. **Discriminated union** rather than `{orgId, orgName, error}`. The tagged shape forces callers to handle each case explicitly — TypeScript's exhaustiveness check catches missed branches. Aligns with Bridge's existing patterns (e.g., `Result<T, E>` style).
3. **Server-side resolution always**. No client-side fallback. The helper runs in the page component (server component) on every render. Cost is tolerable; Next.js caches the `/api/orgs` response within a single request via React's `cache()` if needed (out of scope for this plan).
4. **Preserve `appendOrgId`.** That helper is still used by API URL builders and serves a distinct purpose. No change.
5. **No API-side changes**. The `/api/org/<resource>` endpoints' own fallback logic stays for now — a follow-up plan can tighten them to require explicit orgId. Keeping API behavior unchanged scopes plan 077 to the frontend.
6. **`teachers/students` pages (Pattern A)** also migrate, even though they're already on the modern helper. Removes the dual-helper API surface and gives them `orgName` access.

## Files

**Modify (7 files + 1 helper):**

- `src/lib/portal/org-context.ts` — replace existing exports with `resolveOrgContext` + `OrgContext` type. Delete `parseOrgIdFromSearchParams` (becomes private internal `parseOrgIdParam`) and `resolveOrgIdServerSide`. Keep `appendOrgId` unchanged. ~80 lines diff.
- `src/app/(portal)/org/page.tsx` — switch from `parseOrgIdFromSearchParams` to `resolveOrgContext`. Render no-org state on `kind:"no-org"`; render error state on `kind:"error"`.
- `src/app/(portal)/org/teachers/page.tsx` — same. Drop the now-redundant `if (!orgId)` empty-state. Now uses `ctx.orgName` for the page header (currently shows "Teachers" generically).
- `src/app/(portal)/org/students/page.tsx` — same.
- `src/app/(portal)/org/classes/page.tsx` — same.
- `src/app/(portal)/org/courses/page.tsx` — same.
- `src/app/(portal)/org/settings/page.tsx` — same. (Settings page already shows orgName fetched from the dashboard endpoint; can keep that or use ctx.orgName directly. Pick the simpler.)
- `src/app/(portal)/org/parent-links/page.tsx` — drop the hand-rolled fallback; use `resolveOrgContext`. The `OrgMembership` interface declared inline at line 37 becomes unused (deletable).

**Create (1 file):**

- `tests/unit/org-context.test.ts` — Vitest unit tests for `resolveOrgContext`. Mock `api()` to cover:
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
| Existing tests reference the old `parseOrgIdFromSearchParams` export | low | Pre-impl grep planned: `grep -rln "parseOrgIdFromSearchParams\|resolveOrgIdServerSide" tests/` returns 0 hits today (verified during plan drafting). |
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

(pending — 5-way before implementation)

## Code Review

(pending — 5-way at branch-diff time)

## Post-execution report

(pending)
