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

Out of scope.
This is a content/schema/presentation change with no API, auth, persistence, realtime, or cross-user access behavior.
The focused Vitest files cover the rendered copy and the authoring-schema guard.

## Plan Review

Not run as a five-way gate because this is a low-risk P2 content/presentation fix with no runtime behavior beyond rendered copy and title formatting.
Codex self-review checked the TODO item, the latest browser review section, affected pages, existing focus-area terminology tests, Python 101 content files, and Python 101 schema/import behavior.

## Code Review

### Self-review (Codex) — clean

Reviewed the final diff against `docs/reviews/011-comprehensive-browser-review-2026-05-09.md`.
The implementation follows the review's preferred model: canonical order remains in `course.yaml topics[]` / `topics.sort_order`, Python 101 titles no longer embed display numbers, and UI copy names the Course -> Focus Area -> Unit + Problems taxonomy.
No API, auth, persistence, realtime, or cross-user access behavior changed.

## Post-execution Report

Implemented in one phase:

- Added a Python 101 schema guard rejecting unit titles that start with a display-order prefix such as `1. `.
- Normalized all 12 `content/python-101/units/*.yaml` titles by removing leading numeric prefixes.
- Updated Python 101 schema test fixtures to use unnumbered titles.
- Added source-level copy regression tests for the teacher course detail and student class detail pages.
- Added concise taxonomy copy to those two pages while keeping the existing UI order prefix.
- Marked the Plan 082 TODO item complete.

Verification:

- `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/.bin/vitest run tests/unit/focus-area-rename.test.ts tests/unit/python-101-schema.test.ts` — 2 files passed, 44 tests passed.
- `rg -n "^title: [0-9]+\\." content/python-101/units tests/unit/python-101-schema.test.ts scripts/python-101/schema.ts docs/plans/082-focus-area-numbering.md` — no numbered unit-title matches.
- `/home/chris/.nvm/versions/node/v20.20.1/bin/node ./node_modules/typescript/bin/tsc --noEmit` — failed on 8 existing unrelated baseline errors in `src/app/(portal)/teacher/units/new/page.tsx`, `src/components/admin/user-actions.tsx`, and `tests/unit/identity-assert.test.ts`.
