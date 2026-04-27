# 042 — Problem Editor Responsive Layout

**Goal:** Replace the problem editor's fixed three-pane layout (`min-w-[360px]` left + `min-w-[320px]` right inside `overflow-hidden`) with a responsive shape that works on small laptops and tablets without losing any functionality. Keep the existing wide-screen experience intact; below a breakpoint, collapse the side panels into a tab toggle so the editor (the load-bearing pane) gets full width.

**Source:**
- Codex comprehensive site review, 2026-04-26 (`docs/reviews/002-comprehensive-site-review-2026-04-26.md`) — P2 #11 ("Problem editor still has no responsive fallback").
- Plan 040 deferral — explicitly out-of-scope for 040 because it needs a design pass.

**Branch:** `feat/042-editor-responsive-layout`

**Status:** Draft — awaiting approval

---

## Problem Summary

`src/components/problem/problem-shell.tsx` (currently lines 177-303) renders three fixed-width columns:

- LEFT (problem description + test cases): `w-[32%] min-w-[360px]`
- CENTER (Monaco editor): `min-w-0 flex-1`
- RIGHT (inputs + results + terminal): `w-[28%] min-w-[320px]`

Inside an `overflow-hidden` viewport. On a 13" laptop with the portal sidebar open, the available width drops below the sum of the two `min-w` floors plus the editor's own minimum, causing horizontal scroll or pinched columns. There is no tablet/mobile fallback at all.

The fix is a viewport-driven layout: above a breakpoint keep today's experience; below it, collapse to one pane at a time selectable via a tab bar (Problem / Code / I/O). The editor stays mounted across tab switches so Yjs state and Monaco's history aren't disturbed.

---

## Scope

### In scope

- Responsive breakpoint logic in `problem-shell.tsx`. Default breakpoint: Tailwind's `lg` (≥1024px).
- Tab toggle (Problem / Code / I/O) that shows below the breakpoint. Only the active tab's content is visible; the others stay mounted via `display: none` (Tailwind `hidden`) so editor state survives.
- Above the breakpoint: existing three-column layout unchanged.
- Monaco's own internal layout: it already responds to container size, but verify it relayouts cleanly when its container goes from hidden → visible (and vice versa).
- Touch-friendly tab buttons: large enough hit targets for tablet use.
- Visual regression coverage at common viewport widths via Playwright.

### Out of scope (explicit deferrals)

- **Mobile-first redesign of the editor itself.** Monaco on a 6"-wide phone is unusable regardless of layout — that's a separate "phone-first IDE" question outside this plan.
- **Drawer/Sheet primitive.** A tab bar is simpler and more keyboard-friendly than a drawer for this content; we don't need (and the codebase doesn't have) a Sheet/Drawer primitive yet. If we ever need a slide-over for less-load-bearing content, that's a future follow-up.
- **Persisting the active tab across navigations.** Each visit starts on Code (the default). The existing user behavior is "open the problem, start coding" — surfacing Problem first below the breakpoint forces a context switch on every load and isn't worth the localStorage plumbing.
- **The teacher problem-watch page** (`/teacher/sessions/<id>/students/<id>/problem`) if/when it exists — same shell logic should apply but is a separate render path. Out of scope here.

---

## Phase 1: Responsive Layout

### Task 1.1: Tab state + breakpoint detection

**File:** `src/components/problem/problem-shell.tsx`

Introduce a `narrowTab` state: `"problem" | "code" | "io"`, default `"code"`. The state is only read by the rendered classNames below the breakpoint; above the breakpoint all three panes render unconditionally.

Tailwind handles the breakpoint statically — no `useEffect` width listener needed. Pattern:

- Wide (default, `lg` and above): three-column flex row, all panes visible.
- Narrow (below `lg`): single-column stack with the tab bar at the top. Each pane is wrapped in a div whose visibility is controlled by `narrowTab`.

The visible pane gets `flex-1`; the others get `hidden`. Above `lg` they all unconditionally `lg:flex` (or equivalent), overriding the narrow `hidden`.

### Task 1.2: Tab bar component

**File:** `src/components/problem/problem-tab-bar.tsx` (new)

Stateless `<TabBar active={narrowTab} onChange={setNarrowTab} />`. Renders three `<button>` elements with appropriate `aria-pressed` / role. Visible only at narrow widths (`lg:hidden`). Match the existing zinc/amber visual language — minimal extra CSS.

### Task 1.3: Wire the tab bar into `problem-shell.tsx`

**File:** `src/components/problem/problem-shell.tsx`

Render the tab bar above the existing flex row, behind `lg:hidden`. Add the visibility wrappers around the existing three `<aside>` / `<section>` blocks. The wide-screen path produces identical DOM to today (just an extra wrapper that's `flex` at `lg`).

The tab bar stays out of the way at `lg+`; the panes stay mounted at narrow widths so Monaco doesn't lose state.

### Task 1.4: Monaco relayout on tab switch

**File:** `src/components/editor/code-editor.tsx`

Verify Monaco re-measures its container when the tab visibility flips. If it doesn't (Monaco caches dimensions when the container was `display: none` at mount), call `editor.layout()` after the tab changes to `code`. Implement with a small effect listening to a new `visible: boolean` prop — pages that don't need it pass `true` (default).

### Task 1.5: Vitest unit test for the tab bar

**File:** `tests/unit/problem-tab-bar.test.tsx` (new)

Render the tab bar, assert each button toggles `active`, assert `aria-pressed` flips correctly. No layout assertions — the visibility logic is in the parent.

---

## Phase 2: Visual Regression E2E

### Task 2.1: Playwright spec at 3 viewport widths

**File:** `e2e/problem-editor-responsive.spec.ts` (new)

The test seeds a problem-page URL (using existing E2E fixtures or a created test problem) and visits at three widths:

1. **1440 × 900** (wide): all three panes visible, no tab bar.
2. **1024 × 768** (boundary): all three panes visible (`lg` is inclusive).
3. **800 × 1024** (narrow tablet portrait): tab bar visible, only the active tab visible, switching tabs swaps the visible pane.

For (3), explicit assertions:
- Tab bar `[data-testid="problem-tab-bar"]` is visible.
- Initially: `[data-testid="problem-pane-code"]` is in the viewport; the other two have `display: none` (or are hidden via the `hidden` class).
- Click "Problem" tab → `problem` pane visible, `code` and `io` hidden.
- Click "I/O" tab → similar swap.
- Click "Code" → returns to code, Monaco still shows the placeholder (state preserved across switches).

### Task 2.2: Screenshot snapshots (optional, MINOR)

If Playwright's screenshot snapshot infra is wired (check `playwright.config.ts`), capture one screenshot per width as a regression baseline. If not, skip — not worth standing up a new snapshot pipeline just for this.

---

## Phase 3: Accessibility + polish

### Task 3.1: Keyboard navigation

The tab bar buttons accept Tab/Enter/Space (default browser behavior). Verify with a manual pass — no extra wiring expected.

### Task 3.2: Reduce CLS at narrow widths

When a user lands on a narrow viewport, the wide layout flashes briefly before the `lg:` overrides kick in. Mitigation: ensure the narrow path is the static default, with `lg:` rules adding back the wide row. This is the natural Tailwind ordering and shouldn't need explicit work — but verify by manually loading a fresh tab at 800px and watching for layout shift.

If layout shift is visible, fall back to inline `<style>` tags or a small CSS-only media-query wrapper.

---

## Implementation Order

| Phase | Tasks | Why |
|-------|-------|-----|
| 1 | 1.1 → 1.2 → 1.3 → 1.4 → 1.5 | Build the responsive layout end-to-end. Monaco relayout (1.4) verified after wiring (1.3). |
| 2 | 2.1 → 2.2 | E2E lock once the implementation is stable. |
| 3 | 3.1 → 3.2 | Polish + manual verification |

One PR. ~4 commits.

---

## Verification per Phase

- **Phase 1:** Vitest unit test passes; manual: open the problem page in Chrome DevTools at 800px and 1440px, confirm the layout swap. Switch tabs, confirm Monaco state survives.
- **Phase 2:** new Playwright spec passes against the local stack. Visual snapshots reviewed.
- **Phase 3:** manual keyboard tab-through; manual fresh-load CLS check at 800px.
- **Whole plan:** Vitest + Go suite + new E2E green.

---

## Out-of-Scope Acknowledgements

- Mobile-first IDE redesign.
- Persisted tab state across visits.
- Teacher problem-watch page (separate render path).
- Drawer/Sheet UI primitive.

---

## Codex Review of This Plan

_To be added after the plan is dispatched to Codex via `/codex:rescue`._
