# 042 — Problem Editor Responsive Layout

**Goal:** Replace the problem editor's fixed three-pane layout (`min-w-[360px]` left + `min-w-[320px]` right inside `overflow-hidden`) with a responsive shape that works on small laptops and tablets without losing any functionality. Keep the existing wide-screen experience intact; below a breakpoint, collapse the side panels into a tab toggle so the editor (the load-bearing pane) gets full width.

**Source:**
- Codex comprehensive site review, 2026-04-26 (`docs/reviews/002-comprehensive-site-review-2026-04-26.md`) — P2 #11 ("Problem editor still has no responsive fallback").
- Plan 040 deferral — explicitly out-of-scope for 040 because it needs a design pass.

**Branch:** `feat/042-editor-responsive-layout`

**Status:** Complete (pending PR review)

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

### Task 1.2: Tab bar (inline in problem-shell)

Per Codex pre-impl review #4, keep the tab bar inline in `problem-shell.tsx` rather than extracting a new `<TabBar>` component. Three buttons + a wrapper is small enough that a separate file fragments the layout file rather than clarifying it.

Per Codex pre-impl review #7, use the **standard ARIA tabs pattern**, not `aria-pressed` toggles:

- Tab bar: `role="tablist"`, `aria-label="Problem editor sections"`, visible only at narrow widths (`lg:hidden`).
- Each button: `role="tab"`, `aria-selected={active === id}`, `aria-controls="problem-pane-<id>"`.
- Each pane wrapper: `role="tabpanel"`, `id="problem-pane-<id>"`, `aria-labelledby="problem-tab-<id>"`.
- The role attributes work fine when both tablist and panel are visible at wide widths (the `role="tab"` element being hidden via `lg:hidden` doesn't break the panels; screen readers see the panels normally).

Visual style: zinc background, amber active indicator, large enough hit targets for tablet use (~44px tall).

### Task 1.3: Wire the tab bar into `problem-shell.tsx`

**File:** `src/components/problem/problem-shell.tsx`

Render the tab bar above the existing flex row, behind `lg:hidden`. Add the visibility wrappers around the existing three `<aside>` / `<section>` blocks. The wide-screen path produces identical DOM to today (just an extra wrapper that's `flex` at `lg`).

The tab bar stays out of the way at `lg+`; the panes stay mounted at narrow widths so Monaco doesn't lose state.

### Task 1.4: Monaco relayout via ResizeObserver

**File:** `src/components/editor/code-editor.tsx`

Per Codex pre-impl review #1: a `visible: boolean` prop doesn't fit cleanly because Tailwind owns the wide-vs-narrow visibility decision. The parent's `narrowTab` state doesn't tell the truth at wide widths (where the editor is visible regardless of `narrowTab`'s value).

Better shape: detect dimension changes inside `CodeEditor` itself with a `ResizeObserver` on the container. When the observed `contentRect.width` or `height` transitions from 0 to non-zero, call `editor.layout()`. This is robust to any parent visibility scheme (Tailwind, JS state, `display:none`, `visibility:hidden`) and to the hidden-at-mount case Codex review #2 flagged.

Implementation sketch:

```ts
useEffect(() => {
  const editor = editorRef.current;
  const container = containerRef.current;
  if (!editor || !container) return;

  let prevSize = 0;
  const ro = new ResizeObserver((entries) => {
    const entry = entries[0];
    const size = entry.contentRect.width * entry.contentRect.height;
    if (prevSize === 0 && size > 0) {
      editor.layout();
    }
    prevSize = size;
  });
  ro.observe(container);
  return () => ro.disconnect();
}, []);
```

No new prop on `CodeEditor`. The ResizeObserver is cleaned up on unmount and only triggers on the meaningful 0→non-zero transition.

### Task 1.5: Vitest unit test for the inline tab semantics

**File:** `tests/unit/problem-shell-responsive.test.tsx` (new)

Tab bar is inline now (Task 1.2 change), so the test renders a small wrapper that exercises the active-tab state machine. Asserts:
- Initial state: `code` tab has `aria-selected="true"`.
- Click `problem` → `problem` selected, `code`/`io` deselected.
- Click `io` → `io` selected, others deselected.
- Each tab's `aria-controls` matches an existing `role="tabpanel"` element with the corresponding `id`.

Layout-visibility assertions stay out of Vitest (they're in Phase 2 Playwright).

---

## Phase 2: Visual Regression E2E

### Task 2.1: Playwright spec at 3 viewport widths

**File:** `e2e/problem-editor-responsive.spec.ts` (new)

The test seeds a problem-page URL (using existing E2E fixtures or a created test problem) and visits at three widths:

1. **1440 × 900** (wide): all three panes visible, no tab bar.
2. **1024 × 768** (boundary): all three panes visible (`lg` is inclusive).
3. **800 × 1024** (narrow tablet portrait): tab bar visible, only the active tab visible, switching tabs swaps the visible pane.

For (3), explicit assertions:
- Tab bar `[role="tablist"]` is visible.
- Initially: `#problem-pane-code` is in the viewport; the other two have `display: none` (or are hidden via the `hidden` class).
- Click "Problem" tab → `problem` pane visible, `code` and `io` hidden.
- Click "I/O" tab → similar swap.
- Click "Code" → returns to code, Monaco still shows the placeholder (state preserved across switches).

Per Codex pre-impl review #9, also assert at 800px and 1024px:
- `document.documentElement.scrollWidth <= window.innerWidth` (no horizontal overflow).
- The active pane's container has non-zero width AND height (catches a Monaco-rendered-at-zero failure mode that visibility-only assertions miss).

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

- **Date:** 2026-04-27
- **Reviewer:** Codex (pre-implementation, via `codex:rescue`)
- **Verdict:** Four `[IMPORTANT]` corrections applied; two `[MINOR]` accepted with explanation; three `[NOTE]` items confirmed no plan change needed.

### Corrections applied

1. `[IMPORTANT]` **State/visibility mismatch in Task 1.4.** A `visible: boolean` prop doesn't fit Tailwind-owned visibility — the parent's `narrowTab` state isn't the truth about whether the editor is visible at wide widths. → Task 1.4 now uses a `ResizeObserver` inside `CodeEditor` itself, watching the container for 0→non-zero transitions and calling `editor.layout()` on the boundary. Robust to any parent visibility scheme.
2. `[IMPORTANT]` **ARIA pattern.** Plan said `aria-pressed` but these controls switch mutually exclusive panels — that's the standard `role="tablist"` / `role="tab"` / `role="tabpanel"` pattern, not toggles. → Task 1.2 now spells out the full ARIA wiring (tab IDs, `aria-controls`, `aria-selected`, panel `aria-labelledby`).
3. `[IMPORTANT]` **Playwright is load-bearing.** Vitest only covers tab state machines; the actual responsive layout (Tailwind `lg:` rules, pane visibility at viewport boundaries) needs Playwright at multiple widths. → Phase 2 spec confirmed in scope.
4. `[IMPORTANT]` **Horizontal-overflow assertion.** The original review-002 defect is fixed-min-width panes inside `overflow-hidden`. Tab/visibility assertions could pass while horizontal overflow still clips content. → Task 2.1 now asserts `document.documentElement.scrollWidth <= window.innerWidth` and the active pane has non-zero dimensions at 800px and 1024px.

### Minor adjustments

- `[MINOR]` **Tab bar inline, not extracted.** Codex flagged the `<TabBar>` extraction as more abstraction than three buttons need. → Task 1.2 now keeps the tab markup inline in `problem-shell.tsx`. Test moved from `problem-tab-bar.test.tsx` to `problem-shell-responsive.test.tsx`.
- `[MINOR]` **Hidden-at-mount recovery.** Same fix as #1 (ResizeObserver) covers the deferred-tab-state edge case where the editor could mount inside a `display:none` container.

### Codex notes (no plan change)

- Yjs provider is owned by `ProblemShell`, lives outside any pane wrapper, and `useYjsProvider` cleanup only fires on unmount/dependency change — visibility doesn't drop the Hocuspocus connection.
- `lg` (1024px) is a defensible breakpoint; the fixed floors sum to 680px and `xl` would be a comfort upgrade, not a correctness requirement.
- CLS mitigation already correctly identified (narrow as the static default, `lg:` rules add the wide row back).

## Post-Execution Report

**Status:** Complete. All 3 phases shipped on `feat/042-editor-responsive-layout`.

**Phase 1 — Responsive layout** (commit applied with phase 2/3)
- `src/components/editor/code-editor.tsx`: ResizeObserver on the editor container watches for 0→non-zero size transitions and calls `editor.layout()`. Robust to any parent visibility scheme (Tailwind, JS state, `display:none`, `visibility:hidden`) and to the hidden-at-mount case Codex pre-impl review #2 flagged. No new prop on `CodeEditor`.
- `src/components/problem/problem-shell.tsx`:
  - New `narrowTab` state (`"problem" | "code" | "io"`, default `"code"`).
  - Outer container switches from row to column flex below `lg` (`flex-col lg:flex-row`).
  - Inline tab bar with full ARIA tabs pattern (`role="tablist"` / `role="tab"` / `aria-selected` / `aria-controls`, panels with `role="tabpanel"` / `id` / `aria-labelledby`). Visible only at narrow widths (`lg:hidden`). Per Codex correction #4, kept inline rather than extracted.
  - Each pane wrapped with `paneClass(id)` returning `"flex"` when active narrow OR `"hidden lg:flex"` when not — Tailwind's `lg:flex` overrides `hidden` at wide widths so all three panes render side-by-side on desktop. Pane widths gated to `lg:` (`w-full lg:w-[32%] lg:min-w-[360px]` etc.) so narrow widths use full column width.
- `tests/unit/problem-shell-responsive.test.tsx`: 4 cases on a self-contained tab-state harness (initial selection, click switch, ARIA wiring, active-pane visibility classes).

**Phase 2 — Visual regression E2E** (commit applied with phase 1/3)
- `e2e/problem-editor-responsive.spec.ts`:
  - Wide (1440×900): tab bar hidden, all 3 panes visible.
  - Boundary (1024×768): all 3 panes visible (lg is inclusive); horizontal overflow assertion.
  - Narrow (800×1024): tab bar visible, only active pane visible, switching swaps. Non-zero pane bounding box. Horizontal overflow assertion (catches the original review-002 failure mode).
- Skips when no problem exists in the test data (responsive shape is what's under test, not data).

**Phase 3 — Polish** (commit applied with phase 1/2)
- ARIA pattern verified — full tabs/tabpanel wiring instead of `aria-pressed` toggles per Codex correction #2.
- CLS mitigated by Tailwind's natural ordering — narrow is the static default, `lg:` rules add the wide row back. No layout shift on fresh narrow load.

**Verification**
- Vitest: 422 passed / 11 skipped (was 418 — 4 new responsive cases).
- Go tests: untouched, pre-existing pass.
- TypeScript: clean for new/modified files.
- E2E: spec added; runtime requires the local stack (not run in this loop).

**Plan compliance**
- `[IMPORTANT]` Codex pre-impl correction #1 (state/visibility mismatch) — replaced `visible: boolean` prop with ResizeObserver inside CodeEditor. No parent contract assumption.
- `[IMPORTANT]` Codex pre-impl correction #2 (ARIA pattern) — full `role="tablist"` / `role="tab"` / `role="tabpanel"` wiring with `aria-controls` / `aria-labelledby` / `aria-selected`.
- `[IMPORTANT]` Codex pre-impl correction #3 (Playwright load-bearing) — kept the spec.
- `[IMPORTANT]` Codex pre-impl correction #4 (horizontal overflow assertion) — added at 800px and 1024px.
- `[MINOR]` Codex pre-impl correction #5 (inline tab bar) — kept inline in problem-shell.
- `[MINOR]` Codex pre-impl correction #6 (hidden-at-mount recovery) — same fix as #1 covers it.

**Out-of-scope acknowledgements (queued)**
- Mobile-first IDE redesign.
- Persisted tab state across visits.
- Teacher problem-watch page (separate render path).
- Drawer/Sheet UI primitive.

## Code Review

### Review 1 — Pre-implementation plan review (commit `74e0d36`)

- **Date:** 2026-04-27
- **Reviewer:** Codex (via `codex:rescue`)
- **Verdict:** Corrections applied — see `## Codex Review of This Plan` section above.

### Review 2 — Post-implementation review

- **Date:** 2026-04-27
- **Reviewer:** Codex (post-implementation, dispatch pending)
- **Status:** To be appended after the post-impl Codex review completes.
