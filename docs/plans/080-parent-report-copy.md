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
- Update the empty-state copy at `:84-89` from `"No reports yet. Click 'Generate Weekly Report' to create one."` to a passive form: `"No reports yet for {child.name}. Progress reports will appear here once they're generated."`
- The page still renders existing reports (the 200 path) — that surface stays. Most parents will see the empty-state today since no generation pipeline exists, but accurately so.
- Drop unused imports (`Button`, `useState` for unused state, etc.) per cleanup.

### Fix 3 — child profile: add a Reports link

Per the reviewer's "Add a Reports affordance on the child profile only when the route has a useful read-only state": after Fix 2 makes the route useful, add a Link from `/parent/children/[id]/page.tsx` to `/parent/children/[id]/reports`. Small addition: under the existing class/document sections, a "Progress reports" subsection with a `<Link href={`/parent/children/${id}/reports`}>View progress reports →</Link>` (or similar pattern). Concrete UI choice deferred to implementation.

## Decisions to lock in

1. **`/parent/reports` becomes a server redirect to `/parent`.** No new copy; the dashboard is the canonical entry point.
2. **No MVP report content in plan 080.** The reviewer's "attendance, live-session participation, etc." MVP is product work needing its own plan. Plan 080 is copy-only.
3. **Remove the Generate button entirely.** Don't keep a disabled placeholder button — that signals "coming soon" with no commitment. The Go endpoint stays as-is (it's used by automated reports and tests); only the frontend button goes.
4. **Keep `child.name` in the empty-state copy.** The page already has access to the child's name via the parent-link query — using it makes the empty state feel like a real surface, not a generic placeholder.

## Files

**Modify (3 files):**

- `src/app/(portal)/parent/reports/page.tsx` — replace the one-line render with `import { redirect } from "next/navigation"; export default function() { redirect("/parent"); }`. Single function. ~5 lines.
- `src/app/(portal)/parent/children/[id]/reports/page.tsx` — delete `notImplemented` state, the 501 check, the stale copy block, the Generate button + handler. Update empty-state copy. Drop unused imports. Net diff: ~-30 lines / +5 lines.
- `src/app/(portal)/parent/children/[id]/page.tsx` — add a "Progress reports" subsection with a link to `/parent/children/{id}/reports`. ~+8 lines.

**Create (1 file):**

- `tests/unit/parent-children-reports.test.tsx` — Vitest covering the empty-state and report-list render paths. The Generate button is gone; the test should assert it's NOT in the DOM.

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

## Phases

### Phase 1 — Copy unstaling + dead-code removal (commit 1)

- Pre-impl: verify `tests/integration/parent-reports-page.test.tsx` exists and what it asserts. Either modify it or add a new unit test.
- Replace `src/app/(portal)/parent/reports/page.tsx` with the server-side redirect.
- Edit `src/app/(portal)/parent/children/[id]/reports/page.tsx`: remove the 501 check, the `notImplemented` state, the Generate button + handler, the stale copy block. Update empty-state copy.
- Add a "Progress reports" link to `src/app/(portal)/parent/children/[id]/page.tsx`.
- Add or update the Vitest. Assert: empty state shows the new copy; Generate button is NOT in DOM; reports list renders when data is present.
- Run `bun run test` — confirm the new/updated tests pass + baseline preserved.
- Run `bunx tsc --noEmit` — confirm 10 pre-existing baseline.
- Commit: `plan 080 phase 1: parent reports — accurate copy + remove dead Generate button`.

### Phase 2 — Verify + post-execution report (commit 2)

- Manual smoke (optional, deferred to merge-time): log in as `diana@demo.edu`, navigate to a linked child profile → click "Progress reports" → see empty state with accurate copy.
- Update post-execution report.
- Commit: `docs: plan 080 post-execution report`.

After Phase 2, run the 5-way code review against the consolidated branch diff (single-PR-per-plan policy), fold findings, open the PR via Step 6.

## Plan Review

(pending — 5-way before implementation)

## Code Review

(pending — 5-way at branch-diff time)

## Post-execution report

(pending)
