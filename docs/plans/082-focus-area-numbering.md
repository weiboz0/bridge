# Plan 082 — Focus-area numbering deduplication

## Problem

Browser review 011 queued a P2 display bug in `docs/reviews/011-comprehensive-browser-review-2026-05-09.md`.
Teacher and student course/class pages display Python 101 focus areas as `1. 1. Print & Comments` and `2. 2. Variables & Types`.
The UI is using course order while the Python 101 unit titles also embed display numbers.

The same area also needs clearer taxonomy copy.
The review recommendation is to store order separately from title, render either order or title number but never both, normalize seeded titles, and use **Course -> Focus Area -> Unit + Problems** taxonomy.
In Bridge's classroom model, a Course contains ordered Focus Areas; each Focus Area can have a linked Unit and attached Problems.
The teacher and student detail pages should communicate that relationship without reintroducing the old "topic" terminology.

## Scope

Keep the UI order display because `topics.sort_order` / `course.yaml topics[]` already provide canonical order.
Normalize Python 101 unit titles so title text is just the human-readable label.
Add a schema guard so future authored units do not reintroduce leading numeric display prefixes.
Do not alter topic sorting, course APIs, or importer ordering behavior.

## Files

Modify:

- `src/app/(portal)/teacher/courses/[id]/page.tsx`
- `src/app/(portal)/student/classes/[id]/page.tsx`
- `scripts/python-101/schema.ts`
- `tests/unit/python-101-schema.test.ts`
- `content/python-101/units/*.yaml`
- `tests/unit/focus-area-rename.test.ts`
- `TODO.md`

Create:

- `docs/plans/082-focus-area-numbering.md`

## Phases

### Phase 1 — Regression tests

Add regression tests before implementation.
The tests should fail before implementation by asserting:

- Teacher copy names the Course → Focus Area → Unit + Problems relationship.
- Student copy names the Focus Area → Unit + Problems relationship.
- `tests/unit/python-101-schema.test.ts` rejects a unit title such as `1. Print & Comments`.

Run:

```bash
/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/focus-area-rename.test.ts tests/unit/python-101-schema.test.ts
```

Expected before implementation: the two copy assertions fail and the schema still accepts the numbered title.

### Phase 2 — Normalize content and add taxonomy copy

Update `scripts/python-101/schema.ts`:

- Reject unit titles matching `/^\d+\.\s+/`.
- Error message should direct authors to use course topic order for display order.

Update `content/python-101/units/*.yaml`:

- Remove the leading `N. ` prefix from all 12 Python 101 unit titles.
- Keep `content/python-101/course.yaml` topic order unchanged.
- Do not rename slugs, UUIDs, problems, blocks, or descriptions.

Update `tests/unit/python-101-schema.test.ts` fixtures:

- Use unnumbered valid unit titles.
- Keep the failing numbered-title test.

Update `src/app/(portal)/teacher/courses/[id]/page.tsx`:

- Keep the `Focus Areas ({topicList.length})` heading.
- Add concise helper copy explaining that course focus areas organize units and problems.
- Keep the existing `{i + 1}. {topic.title}` display because order now lives outside the title.

Update `src/app/(portal)/student/classes/[id]/page.tsx`:

- Keep the `Focus Areas` heading.
- Add concise helper copy explaining that each focus area contains the linked unit and practice problems.
- Keep the existing `{i + 1}. {topic.title}` display because order now lives outside the title.

Run:

```bash
/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/focus-area-rename.test.ts tests/unit/python-101-schema.test.ts
```

Expected after implementation: both targeted test files pass.

### Phase 3 — Verify and handoff

Run a broader frontend verification command that is useful for this JSX-only change:

```bash
/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/typescript/bin/tsc --noEmit
```

If the existing TypeScript baseline fails, record the exact status in the post-execution report instead of hiding it.
Update this plan's post-execution report and mark the Plan 082 TODO item complete.
Commit the plan, tests, UI changes, and TODO update together as one logical unit.

## Integration-Test Phase

Amended after external code review.
The original implementation treated this as out of scope, but the existing-demo-clone path is persistence behavior and can preserve stale numbered titles.
Add an integration regression for `--wire-demo-class` reusing an existing demo clone whose cloned topic/unit titles still contain display-order prefixes.

## Plan Review

Process correction: the initial implementation was committed and pushed before the mandatory external code-review gate ran.
That was a workflow miss; external implementation review is mandatory before PR.
The gate was run post-push and before PR, and this follow-up commit records and resolves the review findings.

## Code Review

### Self-review (Codex) — clean

Reviewed the final diff against `docs/reviews/011-comprehensive-browser-review-2026-05-09.md`.
The implementation follows the review's preferred model: canonical order remains in `course.yaml topics[]` / `topics.sort_order`, Python 101 titles no longer embed display numbers, and UI copy names the Course -> Focus Area -> Unit + Problems taxonomy.
No API, auth, persistence, realtime, or cross-user access behavior changed.

### External code review round 1

Reviewers: Codex subagent, GLM 5.1, DeepSeek V4 Flash, Kimi K2.6.

- [FIXED] Existing Bridge Demo School Python 101 clones can keep the old numbered topic/unit titles because `--wire-demo-class` reused an existing clone without refreshing content. The importer now normalizes only legacy display-order prefixes on existing demo-clone topic/unit titles and keeps the clone idempotent.
- [FIXED] Regression coverage missed the stale existing-clone path. `tests/integration/python-101-import.test.ts` now simulates an existing demo clone with `1. Print & Comments`, re-runs `--wire-demo-class`, and verifies the cloned topic/unit titles are normalized without duplicate cloned units.
- [FIXED] The schema guard only rejected `N. ` prefixes. It now rejects one- or two-digit `N.`, `N)`, and `N:` display-order prefixes with or without a following space, with fixture coverage for `1. `, `01. `, `1.`, `1)`, and `1:` forms.
- [FIXED] Taxonomy helper copy did not fully name the course/focus-area/unit/problems relationship. Teacher and student copy now explicitly ties focus areas to the course and the linked unit/practice problems.
- [ACCEPTED] Render-level UI coverage would be stronger than source-level copy assertions, but this repository's existing focus-area terminology coverage is source-level and the changed JSX is static copy. The stale-clone persistence path now has integration coverage.
- [KNOWN] `tsc --noEmit` still fails on unrelated baseline errors in `src/app/(portal)/teacher/units/new/page.tsx`, `src/components/admin/user-actions.tsx`, and `tests/unit/identity-assert.test.ts`.

### External code review round 2

Reviewers: Codex subagent, GLM 5.1, DeepSeek V4 Flash, Kimi K2.6.

- [FIXED] **Integration test query diverged from production scope filtering.** The stale-clone integration test now filters cloned units by both `scope = "org"` and the Bridge Demo School `scopeId`, and asserts exactly one row before destructuring.
- [FIXED] **DRY violation — duplicated prefix regex.** `displayOrderPrefix` is now exported from `scripts/python-101/schema.ts` and reused by `scripts/python-101/import.ts`.
- [FIXED] **Schema regex false positives.** The shared regex now targets optional-leading-zero one- or two-digit display prefixes and avoids decimal/ratio titles such as `2026. Annual Report` and `1:1 Mapping`.
- [FIXED] **Missing acceptance tests for boundary-case titles.** `tests/unit/python-101-schema.test.ts` now asserts acceptance for titles such as `Python 3.12`, `Chapter 1: Intro`, `1x1 Matrix`, `2026. Annual Report`, `1:1 Mapping`, and `101. Advanced Topics`.
- [FIXED] **Empty-string normalization edge case.** Existing demo-clone title normalization now writes non-numbered fallback titles (`Untitled focus area` / `Untitled unit`) when a stale cloned title consists only of a display-order prefix.
- [FIXED] **Integration test gap.** The stale-clone integration test now also asserts the existing-clone path did not create duplicate cloned topics.
- [ACCEPTED] **Return-value semantic mismatch.** `normalizeExistingDemoCloneTitles` returns `cloneTopics.length` as `unitCount`. This is pre-existing and harmless given the 1:1 topic-unit invariant, but the naming is misleading.
- [ACCEPTED] **N+1 normalization updates.** Existing demo clones have about 12 topics and 12 units; the simple loop keeps the behavior clear and is acceptable for this targeted maintenance path.

## Post-execution Report

Implemented in one phase:

- Added a Python 101 schema guard rejecting unit titles that start with a display-order prefix such as `1. `.
- Broadened that guard after external review to reject `1.`, `01.`, `1)`, and `1:` style prefixes.
- Normalized all 12 `content/python-101/units/*.yaml` titles by removing leading numeric prefixes.
- Updated Python 101 schema test fixtures to use unnumbered titles.
- Added source-level copy regression tests for the teacher course detail and student class detail pages.
- Added concise taxonomy copy to those two pages while keeping the existing UI order prefix.
- Added existing-demo-clone normalization for stale display-order prefixes in cloned topic/unit titles.
- Added integration coverage for re-running `--wire-demo-class` against a stale existing demo clone.
- Shared the display-order prefix regex between schema validation and demo-clone normalization, tightened it to avoid numeric-title false positives, and added acceptance coverage for numeric titles that are not display-order prefixes.
- Marked the Plan 082 TODO item complete.

Verification:

- `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/focus-area-rename.test.ts tests/unit/python-101-schema.test.ts tests/integration/python-101-import.test.ts` — 3 files passed, 67 tests passed.
- `rg -n "^title: [0-9]+[.)]|Course focus areas organize units|Each focus area includes a linked|/\\^\\\\d\\+" content/python-101/units tests/unit/python-101-schema.test.ts scripts/python-101/schema.ts scripts/python-101/import.ts src/app docs/plans/082-focus-area-numbering.md` — no stale numbered-title, old-copy, or old-regex matches.
- `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/typescript/bin/tsc --noEmit` — failed on 8 existing unrelated baseline errors in `src/app/(portal)/teacher/units/new/page.tsx`, `src/components/admin/user-actions.tsx`, and `tests/unit/identity-assert.test.ts`.
