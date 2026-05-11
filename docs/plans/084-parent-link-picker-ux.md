# Plan 084 — Parent-link picker UX polish

## Problem (browser review 011-2026-05-09 §P2 #7)

The "Add parent link" dialog's child picker is functional but easy to misread on first use. The reviewer ran into:

> A direct click/create attempt produced `Pick a child from the autocomplete suggestions`; typing `Alice` exposed the suggestion and then the link creation succeeded.

The user has to KNOW that typing reveals options. Empty field + Create-link click feels broken. Reviewer's three recommendations:

1. **Placeholder text** — clarify that the user must search by name/email.
2. **Open suggestion list on focus** for small student sets, so the empty-state isn't silent.
3. **Disable Create-link** until both parent email and selected child are present.

The picker (`src/components/org/create-parent-link-modal.tsx`) state at HEAD `1d0df27`:

- ✅ Placeholder exists at `:228`: `placeholder="Search by name or email…"`. Reviewer's #1 was satisfied during plan 070 phase 2; this is a no-op for plan 084.
- ❌ List only renders when `childQuery.trim()` is non-empty (suggestions memo at `:41-51` short-circuits on empty query, and the `<ul>` at `:241` is gated on `suggestions.length > 0`). On focus, nothing shows.
- ❌ Create button at `:315` is only disabled while `submitting`. The handler at `:77-84` does runtime validation and surfaces the "Pick a child" error after the user clicks Create — the broken-feeling first-click the reviewer described.

## Approach

Two focused changes to `create-parent-link-modal.tsx`:

### Fix 1 — Open suggestion list on focus (when small)

The current memo at `:41-51` returns `[]` when query is empty. Change it so that with an empty query AND `students.length <= 8`, return the FULL student list (capped at 8). For >8 students, keep the empty-query short-circuit so we don't blast the user with a long list before they've started typing. The `<ul>` rendering at `:241` is already gated on `suggestions.length > 0 && !childUserId`, so this widens the visible set without changing layout.

Additionally: focus state. The list is currently controlled purely by `suggestions.length`. After Fix 1, an empty query with a small student set produces non-empty suggestions and the list auto-renders. But a user who's tabbed AWAY and back should still see the list — currently it shows only when a non-empty memo is computed. Add an `isInputFocused` state with `onFocus={() => setIsInputFocused(true)}` / `onBlur={() => setIsInputFocused(false)}` and gate the list render on `(isInputFocused || childQuery.length > 0) && suggestions.length > 0 && !childUserId`.

**ARIA predicate drift — BLOCKER fix** (Codex round-1 NIT, DeepSeek round-1 NIT, GLM 5.1 round-1 BLOCKER — three reviewers converged). The current code at `:166` and `:173-177` sets `aria-expanded` and `aria-controls` from `suggestions.length > 0 && !childUserId` alone. If only the `<ul>` render gate widens to include `isInputFocused`, the two attributes will fall out of sync — specifically, with an empty query and a small org, `suggestions` is non-empty, so `aria-expanded="true"` and `aria-controls="child-autocomplete-listbox"` even when the listbox isn't in the DOM (blur path). WAI-ARIA combobox spec violation. Fix: extract a single derived boolean `listboxVisible` and use it everywhere:

```tsx
const listboxVisible = (isInputFocused || childQuery.length > 0)
                    && suggestions.length > 0
                    && !childUserId;
// then:
aria-expanded={listboxVisible}
aria-controls={listboxVisible ? "child-autocomplete-listbox" : undefined}
// and the <ul> render gate uses the same `listboxVisible` expression.
```

**Edge case**: blur fires when the user clicks a list option (`onMouseDown` already preventDefaults to keep focus, but the `onClick` path — used by screen-reader virtual cursors — doesn't). DELAY the blur close so the click registers: `onBlur={() => setTimeout(() => setIsInputFocused(false), 150)}`. **The 150ms is for AT virtual cursors specifically** (Kimi K2.6 round-1 NIT — add a code comment explaining this so future readers don't "optimize" the timeout away). The `onMouseDown` preventDefault path handles mouse selection without needing the timeout.

### Fix 2 — Disable Create-link until valid

The submit button at `:315` becomes:

```tsx
<Button
  type="submit"
  disabled={submitting || !parentEmail.trim() || !childUserId}
>
```

The runtime check at `:77-84` stays as a defense-in-depth (handles paste-and-submit corner cases) but the button no longer LOOKS clickable when invalid. Clearer first-use UX.

## Decisions to lock in

1. **Open list on focus only for small sets** (≤8 students). Larger orgs would get a wall of names before they've started typing; the existing empty-query short-circuit is correct for them. **Extract a named constant** `const AUTO_OPEN_THRESHOLD = 8` (DeepSeek + GLM round-1 NIT — the threshold and the display cap are coincidentally the same value but conceptually distinct UX decisions; a future change to one shouldn't silently move the other). Reuse the same constant for `.slice(0, 8)` at the existing display cap so they stay tied if you DO want them to track.
2. **Disable submit instead of dim-and-redirect.** The handler's runtime check stays as belt-and-suspenders. The visible UX shifts from "click → error message" to "can't click yet → fix the fields → can click". Standard form pattern.
3. **No placeholder change.** Existing copy ("Search by name or email…") matches the reviewer's recommendation; no edit needed.
4. **No backend changes.** This is purely a frontend polish.
5. **Keep the existing focus-and-blur timing pattern conservative.** 150ms blur delay is enough for click-to-select to fire without the list closing first. The `onMouseDown` preventDefault path already handles mouse selection; the delay is for the `onClick` (screen-reader / virtual cursor) path. Inline code comment will document this so future readers don't "optimize" it away.
6. **Null-safety on `students` prop** (Kimi K2.6 round-1 NIT). The prop is typed as `OrgStudentRow[]`, but a runtime `undefined` from a future loading-state refactor would crash `students.length`. Use `(students?.length ?? 0)` in the threshold check; the existing empty-state copy already handles `students.length === 0` separately.
7. **Add small Vitest regression guard** (Kimi K2.6 round-1 NIT). Three cases: small-set focus opens list, large-set focus stays closed, submit disabled until both fields valid. ~30 lines of test, high ROI given the picker's first-impression weight.

## Files

**Modify (1 file):**

- `src/components/org/create-parent-link-modal.tsx` — extract `AUTO_OPEN_THRESHOLD = 8` constant; update the `suggestions` memo (return full list for small orgs on empty query, using `(students?.length ?? 0) <= AUTO_OPEN_THRESHOLD`); add `isInputFocused` state + handlers (onFocus immediate, onBlur 150ms delay); extract `listboxVisible` derived boolean and use it for the `<ul>` render gate AND `aria-expanded` AND `aria-controls`; disable the submit button when `parentEmail` is empty or `childUserId` is null. ~+20 lines net.

**Create (1 file):**

- `tests/unit/create-parent-link-modal.test.tsx` — Vitest with `@vitest-environment jsdom`. Three cases: (a) small-set focus opens list, (b) large-set (>8 students) focus does NOT open list, (c) submit button disabled until both `parentEmail` and a selected child are present. Mock `fetch` for the submit-disabled case. ~50 lines.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Existing tests assert "listbox hidden when query empty" — would break | **MOOT** (self-review verified) | Pre-impl grep `grep -rln "create-parent-link-modal\|CreateParentLinkModal" tests/ e2e/` returns 0 hits. No existing test exercises the picker. The test gap is pre-existing but plan 084 isn't widening it. Consider a follow-up plan to add unit tests for the picker's open-on-focus + disable-until-valid behavior. |
| Focus/blur delay (150ms) timing-dependent in tests — flaky | low | Use `vi.useFakeTimers()` if any test exercises the blur path. Most tests assert state, not async timing. |
| User with 9+ students sees no list on first focus — confusing | low | Documented in §Decisions #1. Placeholder + "type to search" affordance is enough; large-org users learn quickly. A future plan could add a "view all students" toggle. |
| Disabling submit hides validation errors for fields the user hasn't touched yet | low (positive) | Standard form pattern. Form validity is visible (disabled button) rather than reactive (error after click). Better UX. |
| The `students` prop comes from the parent page's eligible-children fetch — may be slow / empty | very low | Existing code already handles empty + error states in the input area (`:284-294`). No change needed. |
| Blur-on-click race: user clicks an option → onMouseDown fires preventDefault, focus stays on input, list stays open, selectChild fires. Edge case: clicking the option's child element (the email span) might bypass the parent's preventDefault | low | The `<li>` itself owns onMouseDown, and `e.preventDefault()` on the mousedown event prevents focus loss across all descendants of the li. No new risk. |
| **ARIA predicate drift between listbox render gate and `aria-expanded` / `aria-controls`** (Codex + DeepSeek + GLM round-1 — three reviewers converged; GLM elevated to BLOCKER) | high (BLOCKER) | Extract `listboxVisible` derived boolean and use it for the `<ul>` render gate AND `aria-expanded` AND `aria-controls`. Single source of truth eliminates the drift. |
| Brief ARIA flicker during the 150ms blur window — `aria-expanded` flips to `false` slightly after the listbox visibly closes | very low | The visible UI and ARIA both transition within ~150ms; AT users see the change within a single interaction frame. Acceptable. |
| `<Input>` (shadcn wrapper) might not forward `onFocus`/`onBlur` to the native input (DeepSeek round-1 NIT) | low | Pre-impl: verify shadcn's `Input` is a thin wrapper around `<input>` (it is — `src/components/ui/input.tsx` uses `{...props}` spread). If not, switch to a native `<input>` or extend the wrapper. |

## Phases

### Phase 1 — UX polish (commit 1)

- Pre-impl grep: `grep -rln "create-parent-link-modal\|CreateParentLinkModal" tests/ e2e/` to inventory existing tests.
- Update the `suggestions` memo to return full student list (capped at 8) when query is empty and `students.length <= 8`.
- Add `isInputFocused` state, `onFocus`/`onBlur` handlers on the input. Blur uses 150ms setTimeout to tolerate click-to-select.
- Update the listbox render gate from `suggestions.length > 0 && !childUserId` to `(isInputFocused || childQuery.length > 0) && suggestions.length > 0 && !childUserId`.
- Update the submit button: `disabled={submitting || !parentEmail.trim() || !childUserId}`.
- Run `bun run test` — confirm baseline preserved + any test inventoried above passes (update if needed).
- Run `bunx tsc --noEmit` — baseline preserved.
- Commit: `plan 084 phase 1: parent-link picker UX — open-on-focus + disable-until-valid`.

### Phase 2 — Verify + post-execution report (commit 2)

- Manual smoke (post-merge): org admin → /org/parent-links → "Add parent link" → focus child field → see student list immediately (in small-org case) → select → Create button enabled.
- Update post-execution report.
- Commit: `docs: plan 084 post-execution report`.

After Phase 2, run the 5-way code review against the consolidated branch diff (single-PR-per-plan policy), fold findings, open the PR via Step 6.

## Plan Review

### Round 1 (2026-05-10)

#### Self-review (Opus 4.7) — 1 clarification folded

Folded: pre-impl grep verified zero existing tests touch `create-parent-link-modal.tsx`. The §Risks row about "tests would break" is now MOOT (kept as a note for record + a follow-up suggestion to add unit tests).

#### Codex — CONCUR (1 NIT, FIXED)

`[FIXED]` Q2 NIT: ARIA predicate drift. `aria-expanded`/`aria-controls` at `:166`/`:173-177` still gated only on `suggestions.length > 0 && !childUserId`; need to track the same visibility predicate as the new listbox render gate. → **Response**: extract `listboxVisible` derived boolean; use it for the `<ul>` render AND both ARIA attributes. Added §Risks BLOCKER row.

Codex round-1 also confirmed direction: ≤8 threshold defensible (reuses display cap), disable-submit is standard pattern, runtime check stays as defense-in-depth, `students` prop is always available from parent page.

#### DeepSeek V4 Pro — CONCUR (4 NITs, 3 FIXED + 1 verified-out)

1. `[FIXED]` Separate `AUTO_OPEN_THRESHOLD` constant from the `.slice(0, 8)` display cap. → **Response**: extract a named constant; reuse it in both places so they stay tied.
2. `[FIXED]` Verify `<Input>` forwards `onFocus`/`onBlur` to the native element. → **Response**: confirmed during plan-revise that `src/components/ui/input.tsx` is a thin `{...props}` spread wrapper. Added §Risks row noting the verification.
3. `[ACKNOWLEDGED]` Alternative `pointerDown` flag approach for click-tolerance instead of `setTimeout`. → **Response**: keeping the 150ms timeout — Kimi confirmed it's needed specifically for screen-reader virtual cursors that dispatch `click` without preceding `mousedown`. The existing `onMouseDown` preventDefault handles the mouse path. Documented in code comment.
4. `[FIXED]` ARIA brief mismatch during 150ms blur window — same root cause as Codex NIT, fixed by the `listboxVisible` extraction.

#### GLM 5.1 — CONCUR (1 BLOCKER + 1 NIT, all FIXED)

1. `[FIXED]` **BLOCKER** Q5: ARIA predicate drift — third reviewer to converge on this. GLM elevated to BLOCKER with explicit fix snippet (`const listboxVisible = ...`; `aria-expanded={listboxVisible}`; `aria-controls={listboxVisible ? ... : undefined}`). → **Response**: same fix as Codex; extracted `listboxVisible` boolean.
2. `[FIXED]` Q1 NIT: separate threshold constant. Overlap with DeepSeek; folded.

GLM round-1 also confirmed: large-org "type to search" is correct UX, `onMouseDown` + `setTimeout` correctly addresses both mouse and AT paths, runtime check stays as defense against programmatic submit.

#### Kimi K2.6 — CONCUR (5 NITs, 4 FIXED + 1 acknowledged)

1. `[FIXED]` Q1 NIT: code comment explaining the 150ms controlled-race window. → **Response**: §Decisions #5 documents this; implementation will include inline comment.
2. `[FIXED]` Q2 NIT: comment that the timeout is AT-specific. → **Response**: §Decisions #5 covers this.
3. `[FIXED]` Q3 NIT: null-safety on `students?.length`. → **Response**: added §Decisions #6 + impl uses `(students?.length ?? 0)`.
4. `[ACKNOWLEDGED]` Q4: hard ≤8 cut. Confirmed direction.
5. `[FIXED]` Q5 NIT: add a small Vitest. → **Response**: added §Files Create entry for `tests/unit/create-parent-link-modal.test.tsx` (3 cases, ~50 lines).

### Convergence

All 5 reviewers concur. **Codex + DeepSeek + GLM all independently caught the ARIA predicate drift** — strong cross-model signal. GLM elevated to BLOCKER; Codex+DeepSeek classified as NIT. Whichever severity, the fix is the same and is folded.

**Multi-reviewer ensemble value, plan 084**: 3 of 4 external reviewers converged on the ARIA drift bug — a real WAI-ARIA combobox spec violation that none of them would have caught alone if the others had said "looks fine". This is the pattern Bridge's gate is designed for.

## Code Review

(pending — 5-way at branch-diff time)

## Post-execution report

(pending)
