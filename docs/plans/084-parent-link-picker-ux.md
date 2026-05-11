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

**Edge case**: blur fires when the user clicks a list option (`onMouseDown` already preventDefaults to keep focus, but the `onClick` path doesn't — needs a slight delay or to gate the blur logic). Simpler: tie list visibility to the input having focus OR the query being non-empty, NOT to a freshly-managed focus state. Use `document.activeElement === inputRef.current` indirectly via the `onFocus` + `onBlur` events, but DELAY the blur close so the click registers. Specifically: `onBlur={() => setTimeout(() => setIsInputFocused(false), 150)}` — common pattern for combobox close-on-blur with click tolerance.

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

1. **Open list on focus only for small sets** (≤8 students). Larger orgs would get a wall of names before they've started typing; the existing empty-query short-circuit is correct for them. The 8-student threshold matches the existing `.slice(0, 8)` cap — same number, no new magic constant.
2. **Disable submit instead of dim-and-redirect.** The handler's runtime check stays as belt-and-suspenders. The visible UX shifts from "click → error message" to "can't click yet → fix the fields → can click". Standard form pattern.
3. **No placeholder change.** Existing copy ("Search by name or email…") matches the reviewer's recommendation; no edit needed.
4. **No backend changes.** This is purely a frontend polish.
5. **Keep the existing focus-and-blur timing pattern conservative.** 150ms blur delay is enough for click-to-select to fire without the list closing first. The `onMouseDown` preventDefault path already handles mouse selection; the delay is for the `onClick` (screen-reader / virtual cursor) path.

## Files

**Modify (1 file):**

- `src/components/org/create-parent-link-modal.tsx` — update the `suggestions` memo (return full list for small orgs on empty query), add `isInputFocused` state + handlers, gate the listbox visibility on focus OR non-empty query, disable the submit button when `parentEmail` is empty or `childUserId` is null. ~+15 lines net.

**No changes to:**

- Any existing tests (the existing keyboard/click tests at `tests/unit/create-parent-link-modal.test.tsx` (if present) — verify scope before assuming. If a test asserts "the listbox is hidden on empty query", plan 084 changes that contract and the test needs updating.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Existing tests assert "listbox hidden when query empty" — would break | **MOOT** (self-review verified) | Pre-impl grep `grep -rln "create-parent-link-modal\|CreateParentLinkModal" tests/ e2e/` returns 0 hits. No existing test exercises the picker. The test gap is pre-existing but plan 084 isn't widening it. Consider a follow-up plan to add unit tests for the picker's open-on-focus + disable-until-valid behavior. |
| Focus/blur delay (150ms) timing-dependent in tests — flaky | low | Use `vi.useFakeTimers()` if any test exercises the blur path. Most tests assert state, not async timing. |
| User with 9+ students sees no list on first focus — confusing | low | Documented in §Decisions #1. Placeholder + "type to search" affordance is enough; large-org users learn quickly. A future plan could add a "view all students" toggle. |
| Disabling submit hides validation errors for fields the user hasn't touched yet | low (positive) | Standard form pattern. Form validity is visible (disabled button) rather than reactive (error after click). Better UX. |
| The `students` prop comes from the parent page's eligible-children fetch — may be slow / empty | very low | Existing code already handles empty + error states in the input area (`:284-294`). No change needed. |
| Blur-on-click race: user clicks an option → onMouseDown fires preventDefault, focus stays on input, list stays open, selectChild fires. Edge case: clicking the option's child element (the email span) might bypass the parent's preventDefault | low | The `<li>` itself owns onMouseDown, and `e.preventDefault()` on the mousedown event prevents focus loss across all descendants of the li. No new risk. |

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

#### Codex — pending

#### DeepSeek V4 Pro — pending

#### GLM 5.1 — pending

#### Kimi K2.6 — pending

## Code Review

(pending — 5-way at branch-diff time)

## Post-execution report

(pending)
