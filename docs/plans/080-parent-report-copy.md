# Plan 080 — Parent-report copy unstaling + dead-button removal

## Problem (browser review 011-2026-05-09 §P1 #4)

Parent-report surfaces still communicate states that haven't been true since plan 064 (parent-child linking) shipped:

### `/parent/reports/page.tsx` (one-liner)

```tsx
export default function ParentReportsPage() {
  return <div className="p-6"><h1>Reports</h1>
    <p>AI-generated progress reports coming soon.</p>
  </div>;
}
```

The page is unreachable from any nav (parent portal `navItems` only has Dashboard — `src/lib/portal/nav-config.ts:63-65`). Direct-URL-only, but its bookmark/deeplink claim "AI-generated reports coming soon" implies a near-term feature when there's no AI-generation pipeline at all. The Go endpoint stores externally-supplied reports; AI is out of scope.

### `/parent/children/[id]/reports/page.tsx` (113 lines)

Three stale paths:

1. **Dead 501 check** at `:30` and `:47`. Plan 047 phase 2 stubbed parent-report endpoints with 501 until parent linking shipped. Plans 064/070 finished linking; the Go handler at `platform/internal/handlers/parent.go:24-30` now returns 200/400/401/403/500 — never 501. The frontend's `setNotImplemented(true)` branch is dead code.
2. **Stale "linking still being built" copy** at `:76-81`:
   > Reports coming soon. We're still building parent-child account linking. Once that ships you'll see weekly progress summaries for your child here.

   Linking IS built — the page wouldn't render at all if it weren't. The copy contradicts the working `/org/parent-links` and `/parent/children/[id]/live` flows.
3. **Broken "Generate Weekly Report" button** at `:65-67`. It POSTs to `/api/parent/children/{id}/reports` with `method: "POST"` and no body. The Go handler at `parent.go:103-114` REQUIRES `content`, `periodStart`, and `periodEnd` — the empty-body POST 400s every time. The button is dead AND there's no AI pipeline to wire it to.

The browser review's recommendation:

> Update report copy to reflect the current blocker accurately, for example "Progress reports are not generated yet."
> Add a Reports affordance on the child profile only when the route has a useful read-only state.
> Define the minimum report MVP: attendance, live-session participation, recent code, completed problems, and teacher notes before AI-generated narrative.

The MVP recommendation is out of scope for plan 080 — that's product work needing its own design pass. Plan 080 just unstale the copy and remove the dead button.

## Approach

Three discrete fixes:

### Fix 1 — `/parent/reports/page.tsx`: redirect to `/parent`

The page is unreachable from nav; no inbound links anywhere in `src/`. Anyone who hits it via bookmark/deeplink is better served by the parent dashboard (which lists their children — the actual reports surface lives at the per-child route). Replace the one-liner with a server-side `redirect("/parent")`.

Alternative considered: keep the page and update the copy. Rejected — the page exists only as a dead-end placeholder. A redirect is cleaner and preserves the URL contract for any external bookmark.

### Fix 2 — `/parent/children/[id]/reports/page.tsx`: remove dead code + accurate copy

- Delete the `notImplemented` state + the 501 check at `:30` and `:47`.
- Delete the stale "we're still building parent-child account linking" copy block at `:73-83`.
- Delete the "Generate Weekly Report" button at `:64-68` and the `handleGenerate` function at `:39-58`.
- Update the empty-state copy at `:84-89` from `"No reports yet. Click 'Generate Weekly Report' to create one."` to a passive form: `"No reports yet. Progress reports will appear here once they're generated."` (Codex round-1 BLOCKER Q5: original draft used `{child.name}` but the page only has `params.id`, no child data fetch. Adding a fetch is scope creep; generic copy is fine since the page header already says "Progress Reports" and the URL identifies the child contextually.)
- The page still renders existing reports (the 200 path) — that surface stays. Most parents will see the empty-state today since no generation pipeline exists, but accurately so.
- Drop unused imports (`Button`, `useState` for unused state, etc.) per cleanup.

### Fix 3 — child profile: add a Reports link

Per the reviewer's "Add a Reports affordance on the child profile only when the route has a useful read-only state": after Fix 2 makes the route useful, add a Link from `/parent/children/[id]/page.tsx` to `/parent/children/[id]/reports`. Small addition: under the existing class/document sections, a "Progress reports" subsection with a `<Link href={`/parent/children/${id}/reports`}>View progress reports →</Link>` (or similar pattern). Concrete UI choice deferred to implementation.

## Decisions to lock in

1. **`/parent/reports` becomes a server redirect to `/parent`.** No new copy; the dashboard is the canonical entry point.
2. **No MVP report content in plan 080.** The reviewer's "attendance, live-session participation, etc." MVP is product work needing its own plan. Plan 080 is copy-only.
3. **Remove the Generate button entirely.** Don't keep a disabled placeholder button — that signals "coming soon" with no commitment. The Go endpoint stays as-is (it's used by automated reports and tests); only the frontend button goes.
4. **Generic empty-state copy** (Codex round-1 BLOCKER Q5 fix). Original draft said "use `{child.name}`" but the per-child reports page only has `params.id` — no child data fetch. Adding one would expand scope. Generic copy ("No reports yet. Progress reports will appear here once they're generated.") is fine since the page header already says "Progress Reports" and the URL contextually identifies the child.

## Files

**Modify (3 files):**

- `src/app/(portal)/parent/reports/page.tsx` — replace the one-line render with `import { redirect } from "next/navigation"; export default function() { redirect("/parent"); }`. Add a defensive header comment (GLM 5.1 round-1 NIT 1): `// Plan 080: server-side redirect placeholder. Don't re-add page content here without revisiting the parent-portal nav decision (review 011-2026-05-09 §P1 #4).` ~8 lines.
- `src/app/(portal)/parent/children/[id]/reports/page.tsx` — delete `notImplemented` state, the 501 check, the stale copy block, the Generate button + handler. Update empty-state copy. Drop unused imports. Net diff: ~-30 lines / +5 lines.
- `src/app/(portal)/parent/children/[id]/page.tsx` — add a "Progress reports" subsection with a link to `/parent/children/{id}/reports`. ~+8 lines.

**Modify (existing test file, NOT new):**

- `tests/integration/parent-reports-page.test.tsx` — already exists (66 lines, 3 tests). Two of its tests assert the EXACT paths plan 080 removes (the 501 "coming soon" copy + the Generate button presence). Self-review correction: the §Files originally said "Create (1 file)" — wrong. The plan rewrites this file:
  - **Delete** the `'renders Reports coming soon when GET returns 501'` test (dead path; the 501 check is gone in production code).
  - **Delete** the `'hides the Generate button when 501 is returned'` test (the Generate button is gone in BOTH branches now).
  - **Update** the `'renders the empty state when 200 with []'` test: replace `expect(getByRole("button", {name: /generate weekly report/i})).toBeInTheDocument()` with `expect(queryByRole("button", {name: /generate weekly report/i})).toBeNull()`. Update the empty-state copy assertion to match the new accurate copy.
  - **Add** a new test asserting the report-list render path (200 with non-empty array).
  - File-level update: replace the `describe("ParentReportsPage — Plan 047 phase 2 disabled state")` header with `describe("ParentReportsPage")` since the disabled state no longer exists.

**No changes to:**

- `platform/internal/handlers/parent.go` — Go handlers stay. The POST endpoint is still useful for system-generated reports (LLM agent, scheduled task, etc.) when those land.
- `src/lib/portal/nav-config.ts` — parent nav is intentionally minimal; no change.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| `/parent/reports` redirect breaks an external bookmark someone is using | very low | Page is unlinked; reaching it requires a deliberate bookmark. Redirecting to `/parent` keeps them in the portal — not a dead link. |
| Removing the Generate button removes a way to test the POST endpoint manually | low | Go endpoint tests cover the POST path. Manual testing can use curl. UI-driven generation needs a real AI pipeline before resurfacing. |
| Empty-state copy uses `child.name` but the parent might prefer a generic message | very low | Personalization is a small UX win. If feedback comes in, swap to generic in a one-line follow-up. |
| Missing tests — the existing `parent-reports-page.test.tsx` may already cover the page; adding a new file would duplicate | low | Pre-impl grep for an existing test file. If one exists, modify it instead of creating a new one. (`tests/integration/parent-reports-page.test.tsx` exists per `git ls-tree` — verify scope before adding new.) |
| Plan 080 doesn't address the actual "no reports get generated" problem — just hides it | medium (intended) | This IS the intended scope. The browser reviewer explicitly said "Update report copy to reflect the current blocker accurately." A separate MVP plan handles content generation. Plan 080 is honest-copy-only. |
| Child profile getting a "Progress reports" link surfaces the empty-state to parents who didn't know about it | low (positive) | Reveals the gap (no reports yet) explicitly rather than hiding it behind a deep URL. The reviewer's recommendation flagged this affordance as desirable once the route is useful. |
| Future dev shadows the `/parent/reports` redirect by adding a real page at the same path (GLM 5.1 round-1 NIT 5a) | low | Defensive header comment in the redirect file warns future maintainers; reviewers catch a re-introduction at PR time. |

## Phases

### Phase 1 — Copy unstaling + dead-code removal (commit 1)

- Pre-impl: verify `tests/integration/parent-reports-page.test.tsx` exists and what it asserts. Either modify it or add a new unit test.
- Replace `src/app/(portal)/parent/reports/page.tsx` with the server-side redirect.
- Edit `src/app/(portal)/parent/children/[id]/reports/page.tsx`: remove the 501 check, the `notImplemented` state, the Generate button + handler, the stale copy block. Update empty-state copy. Add a one-line code comment near the deleted button site documenting the Go POST contract (DeepSeek round-1 Q3 NIT): `// Go POST /api/parent/children/{id}/reports requires { content, periodStart, periodEnd }; no UI flow yet — see plan 080.`
- Add a "Progress reports" link to `src/app/(portal)/parent/children/[id]/page.tsx`.
- Add or update the Vitest. Assert: empty state shows the new copy; Generate button is NOT in DOM; reports list renders when data is present. The new report-list test should mock data matching the `Report` interface shape (`id`, `periodStart`, `periodEnd`, `content`, `createdAt`) to avoid false positives (Kimi K2.6 round-1 NIT Q5).
- Run `bun run test` — confirm the new/updated tests pass + baseline preserved.
- Run `bunx tsc --noEmit` — confirm 10 pre-existing baseline.
- Commit: `plan 080 phase 1: parent reports — accurate copy + remove dead Generate button`.

### Phase 2 — Verify + post-execution report (commit 2)

- Manual smoke (optional, deferred to merge-time): log in as `diana@demo.edu`, navigate to a linked child profile → click "Progress reports" → see empty state with accurate copy.
- Update post-execution report.
- Commit: `docs: plan 080 post-execution report`.

After Phase 2, run the 5-way code review against the consolidated branch diff (single-PR-per-plan policy), fold findings, open the PR via Step 6.

## Plan Review

### Round 1 (2026-05-09)

#### Self-review (Opus 4.7) — clarification

Folded one correction at the §Files step: original draft said "Create (1 file)" for the test. Wrong — `tests/integration/parent-reports-page.test.tsx` already exists and directly tests the dead 501 path + Generate button presence. Plan now specifies REWRITING the file: delete the 501-path tests, update the empty-state test to expect Generate button is GONE, add a new test for the report-list render path.

#### Codex — CONCUR with 1 BLOCKER (FIXED)

`[FIXED]` BLOCKER Q5: original draft said empty-state copy uses `{child.name}`, but the per-child reports page only has `params.id` — no child data fetch. Either use generic copy or add a fetch (scope creep). → **Response**: §Approach Fix 2 + §Decisions #4 updated to use generic copy. Page header already says "Progress Reports" and the URL contextually identifies the child. Kimi K2.6 independently confirmed the same finding.

Codex round-1 also confirmed direction: redirect defensible (no inbound app links), Generate button removal is right call, plan 080 scope is tight, child-profile link is net positive.

#### DeepSeek V4 Pro — CONCUR with 1 NIT (FIXED)

`[FIXED]` Q3 NIT: plan documents removals but not the Go POST endpoint contract. Future LLM-pipeline plan would re-discover it by reading Go. → **Response**: added one-line code comment at the deleted-button site documenting the contract: `// Go POST /api/parent/children/{id}/reports requires { content, periodStart, periodEnd }; no UI flow yet — see plan 080.`

DeepSeek round-1 also confirmed direction: honest empty state is the right call for this cycle (MVP needs cross-system aggregation; product work for a separate plan); risk table is comprehensive.

#### GLM 5.1 — CONCUR (5 NITs, 2 actionable, 1 misread, 2 acknowledged)

1. `[FOLD-PENDING]` NIT 1: add a defensive comment in the redirect page (e.g., `// Plan 080: redirect — do not re-add a page here without …`) so future devs don't accidentally shadow it. Will fold during impl.
2. `[ACKNOWLEDGED]` NIT 2: removing Generate button is "acceptable v1; honest not destructive."
3. `[ACKNOWLEDGED]` NIT 3: child-profile "Progress reports" link reveals empty state — net positive, transparent beats hidden.
4. `[ACKNOWLEDGED]` NIT 4: rewriting the test file is correct; `.skip` would mislead.
5. `[FOLD-PENDING]` NIT 5a: future-dev shadow risk on the `/parent/reports` route segment. Will add §Risks row.
6. `[REJECT-MISREAD]` NIT 5b: GLM claims the test file will need adjustment when the page goes client→server. This is a misread — `tests/integration/parent-reports-page.test.tsx` imports the PER-CHILD page (`/parent/children/[id]/reports/page.tsx`), not `/parent/reports`. The per-child page stays a client component; only `/parent/reports` becomes a server redirect, and that file has no test. No adjustment needed.

#### Kimi K2.6 — CONCUR with 1 BLOCKER overlap + 2 NITs (1 FIXED + 1 acknowledged)

1. `[FIXED]` Q2 (overlap with Codex BLOCKER Q5): `child.name` claim is false. → **Response**: same fix — generic copy. Two reviewers caught the same finding.
2. `[FIXED]` Q5 NIT: test rewrite should assert mock data matches the `Report` interface to avoid false positives. → **Response**: added to §Phases — "mock data matching the `Report` interface shape".
3. `[ACKNOWLEDGED]` Q3: don't archive deleted assertions in comments — git history preserves them. Don't litter tests with commented-out dead paths. Plan already does the clean delete.

Kimi round-1 also confirmed direction: parent dashboard at `/parent` HAS linked-children grid, so redirect lands the user with a clear next step (verified `parent/page.tsx` renders children grid + "View Profile" links).

### Convergence

All 5 reviewers concur. 1 BLOCKER (Codex Q5 / Kimi Q2 — same finding, child.name) + cosmetic NITs all folded. Plan 080 ready for implementation.

**Multi-reviewer ensemble value, plan 080 edition**: Codex AND Kimi independently caught that the page lacks child-data fetch. Two-reviewer consensus on a real factual error in the plan that affected impl steps.

## Code Review

5-way code review against branch HEAD `6c94c3b`.

### Self (Opus 4.7) — clean

`bun run test` 646 PASS / 11 skipped / 0 failed. `bunx tsc --noEmit` 10 errors (pre-existing baseline). All 5 plan-review folds verified in code: defensive comment in redirect file; `{child.name}` replaced with generic copy; Generate button + handler + state + 501 check entirely removed; Go POST contract code comment present; test mock data matches `Report` interface.

### Codex — CONCUR with 1 BLOCKER (FIXED)

`[FIXED]` BLOCKER Q5: `e2e/parent.spec.ts:62-69` still expected `/parent/reports` to render a "Reports" heading + "coming soon" copy. With the redirect, that e2e test would fail. → **Response (commit `19ee4ff`)**: rewrote the test as `"/parent/reports redirects to /parent"` asserting `await expect(page).toHaveURL(/\/parent$/)`. The other 4 reviewers (Self/Opus, GLM, DeepSeek, Kimi) all missed this because they didn't grep `e2e/` — only Codex's wider scan caught it.

Codex round-1 also confirmed Q1-Q4 PASS: generic copy, no Generate button, defensive comment, test assertions correct.

### DeepSeek V4 Flash — CONCUR clean (0 BLOCKERS, 0 NITS)

Confirmed: Go POST contract block comment at `parent/children/[id]/reports/page.tsx:17-22`; `notImplemented`/`generating` state, `handleGenerate` function, `Button` import, 501 fetch path all gone — no leftovers; 3 tests pass (empty state, list rendering, error surface); no regression; redirect at `/parent/reports` works as intended.

### GLM 5.1 — CONCUR clean (0 BLOCKERS, 0 NITS)

Confirmed: defensive comment present at lines 1-5 of redirect file (server component, no `"use client"`); child-profile "Progress reports" link targets `/parent/children/{id}/reports` correctly; reports page fully cleaned (no `notImplemented`, no Generate button, no 501 check, all imports used); generic empty-state copy (no `{child.name}`); no regressions.

### Kimi K2.6 — CONCUR clean (0 BLOCKERS, 1 minor NIT no-action)

Confirmed: generic empty-state copy (BLOCKER resolved); test mock data matches `Report` interface (NIT Q5 resolved); redirect lands on `/parent` with linked-children grid; deleted assertions truly gone (no `.skip` / commented-out); no subtle bugs (try/catch wraps fetch, all awaits present, conditional render correct).

NIT (no action): Unicode arrow `→` in child-profile link is fine for screen readers.

### Convergence

All 5 reviewers concur. Codex caught 1 BLOCKER (e2e test stale) that the other 4 missed — only Codex's wider scan included `e2e/`. Pattern lesson: review scope diversity matters as much as model diversity.

## Post-execution report

Single phase shipped at `b187d59`.

### Changes

- `src/app/(portal)/parent/reports/page.tsx` — server-side redirect to `/parent` + defensive header comment.
- `src/app/(portal)/parent/children/[id]/reports/page.tsx` — removed 501 check, `notImplemented` state, Generate button + handler, stale copy block. Added header comment documenting the Go POST contract. Generic empty-state copy. Net diff: ~+10 / -40 lines.
- `src/app/(portal)/parent/children/[id]/page.tsx` — added "Progress reports" section + link.
- `tests/integration/parent-reports-page.test.tsx` — rewrote: deleted 2 dead-path tests, updated empty-state test, added report-list and fetch-error tests. 3 tests total (same count, different coverage).

### Verification

- `bun run test`: 646 PASS / 11 skipped / 0 failed (no net change from main baseline).
- `bunx tsc --noEmit`: 10 errors, all pre-existing baseline.
- Pre-impl grep `grep -rn "/parent/reports\|coming soon\|parent-child account linking" src/`: only the parent-report files (now updated). No remaining stale references.

### No deviations from plan

All 5 reviewer fold-ins applied: GLM (defensive comment + shadow risk), Codex/Kimi (generic empty-state copy — child data not available), DeepSeek (Go POST contract code comment), Kimi (mock Report shape in test).

### Follow-ups

- **Reports MVP** (browser review §P1 #4 deferred): attendance, live-session participation, recent code, completed problems, teacher notes — before AI-generated narrative. Backend aggregation work; needs its own plan.
- **System-generated report pipeline**: scheduled job or LLM agent that POSTs to the existing Go endpoint with full content. Out of scope here.
