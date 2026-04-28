# Plan 045 — Unit Picker UI

## Status

- **Date:** 2026-04-27
- **Branch:** `feat/045-unit-picker` (chained off `feat/044-topic-syllabus-refactor`)
- **Depends on:** Plan 044 (link-unit endpoint must exist)
- **Goal:** Replace the paste-Unit-ID text input on the topic editor with a real searchable Unit picker — and along the way, fix two latent issues the picker exposes (link auth model for platform-scope Units, broken cursor pagination).

## Problem

Plan 044 shipped a primitive paste-Unit-ID text input on the teacher topic editor. It works but:

1. Teachers must navigate to `/teacher/units`, copy the UUID by hand, and paste it back.
2. The input gives no signal about which Units are *linkable* to this specific course (cross-org reachability).
3. There's no preview of the Unit's title, material type, status, or grade level before linking.
4. There's no way to filter or search for the right Unit.
5. If the Unit is already linked to a *different* topic, the user only finds out after clicking Link (409 error).

Two adjacent issues surface as soon as the picker exists:

- **Platform-scope Units are unlinkable today.** Plan 044's `LinkUnit` handler enforces `canEditUnit` on the Unit being linked. For platform-scope, `canEditUnit` is platform-admin-only. So a teacher can SEE a published platform Unit (visibility) but cannot LINK it to their course's topic — even though published platform Units exist precisely to be reused by teachers. This wasn't visible with paste-ID because nobody was browsing platform Units; it becomes a blocker the moment the picker shows them.
- **Cursor pagination is wired but broken.** `SearchUnits` emits `nextCursor` but the handler never parses an incoming `cursor` back into the store filter — so "Load More" clicks would re-fetch the first page. Pre-existing latent bug; the picker is the first UI to actually need pagination.

## Goal

Replace the text input with a search-and-pick UI that:

- Shows only Units linkable to *this* course's org (`scope='platform' OR (scope='org' AND scope_id=course.org_id)`), matching the link-unit endpoint's reachability gate.
- Supports text search (FTS) by title/summary, plus filters by grade level and material type.
- Displays Unit metadata (title, material type, status, grade level, summary preview) inline so the teacher can pick confidently.
- Surfaces Units that are *already linked* to a different topic with a visible "Already linked" badge and disables the Pick button (preventing the avoidable 409). When the linked topic is in a *different* org's course, the badge says "Already linked" with no topic title (no cross-org leakage).
- Distinguishes loading, empty, and error states so a server failure doesn't silently look like "no results."
- Keyboard-navigable.
- Works correctly with cursor pagination + active filters (Load More preserves filters and advances pages).

Along the way, fix:
- The link auth model so a course teacher CAN link a published platform-scope Unit to their topic.
- The cursor pagination roundtrip in `SearchUnits`.

## Non-Goals

- Multi-Unit-per-topic (schema is 1:1 — out of scope, plan 047 may revisit).
- Bulk-link many topics to many Units — separate plan.
- Cross-course move (relink a Unit from topic A to topic B) — for now, the user must unlink-then-relink.
- Unit creation from inside the picker — keep that flow at `/teacher/units/new` (an "Or create a new Unit →" link is fine).
- Renaming Topic → Syllabus Area (plan 047).

## Design

### Backend changes

**1. Loosen the link/unlink auth gate for published platform-scope Units.**

`LinkUnit` and `UnlinkUnit` (new) should accept platform-scope Units as long as their status is one of the published statuses (`classroom_ready`, `coach_ready`, `archived`). The course-edit gate still applies (only the course creator or a platform admin can attach Units to "their" topic), so this doesn't widen who can modify a course — it just widens which library Units they can pull in.

New gate logic for both Link and Unlink (after course-edit + topic-belongs-to-course are verified):

```
if unit.Scope == "platform":
    if unit.Status in ("classroom_ready", "coach_ready", "archived") OR claims.IsPlatformAdmin:
        allow
    else:
        403 (cannot link a draft platform Unit)
elif unit.Scope == "org" AND unit.ScopeID == course.OrgID:
    if canEditUnit(unit, claims):  # teacher/admin in that org, or platform admin
        allow
    else:
        403
elif unit.Scope == "org" AND unit.ScopeID != course.OrgID:
    403  # cross-org reachability — same as plan 044 fix
elif unit.Scope == "personal":
    403  # personal Units cannot be linked (read-side join filters them out anyway)
else:
    403
```

This replaces (and supersedes) plan 044's `LinkUnit` Unit-edit gate at `platform/internal/handlers/topics.go:374-404`. Same logic powers the new Unlink endpoint and the picker's `canLink` decoration (see #3 below).

**2. Add `linkableForCourse` parameter and `materialType` filter to `GET /api/units/search`.**

When the caller passes `?linkableForCourse=<courseId>`, the SearchUnits handler:

1. Loads the course and verifies the caller can edit it (creator or platform-admin). If not, returns 403.
2. Constrains the SearchUnitsFilter to `(scope='platform' AND status in published-statuses) OR (scope='org' AND scope_id=course.org_id)`. Personal-scope is excluded — read-side join filters them out anyway.
3. Decorates each result item with `linkedTopicId`, `linkedTopicTitle`, and `canLink`:
   - `linkedTopicId`: present whenever the Unit has `topic_id IS NOT NULL` — this is OK to expose, the topic_id alone isn't sensitive.
   - `linkedTopicTitle`: present ONLY when the linked topic's course is in the SAME org as the picker's course OR the caller is a platform admin. Cross-org case returns null, and the UI renders "Already linked" with no topic name. **This is a cross-org leak guard required by Codex pre-impl review.**
   - `canLink`: boolean — `true` if applying the link auth gate (#1 above) to this Unit + course + caller would succeed and the Unit is not already linked to a different topic. The UI uses this to disable rows; the backend still enforces the gate on the actual POST.

Add `materialType` to the SearchUnitsFilter and the handler query-param parsing. The store's WHERE clause adds `material_type = $N` when set. Include in tests.

**3. Fix cursor pagination.**

Handler currently emits `nextCursor` of shape `<updatedAt>|<unitID>` but doesn't parse an incoming `cursor` query param. Add parsing: split on `|`, parse left half as RFC3339 timestamp into `CursorCreatedAt`, right half as string into `CursorID`. Reject malformed cursors with 400. The store already has the keyset clause `(updated_at, id) < ($N, $M)` (`platform/internal/store/teaching_units.go:760-765`).

Active filters (q, scope, status, gradeLevel, materialType, linkableForCourse) MUST be re-sent with each Load More request — the cursor is a position pointer, not a query memo. Document this in the test for the picker dialog (the dialog sends them on every search call already, so this is essentially "don't break it during phase 2").

**4. New `DELETE /api/courses/{cid}/topics/{tid}/link-unit` (Unlink).**

Why this URL shape (not `DELETE /api/units/{id}/topic-link` or `PATCH /api/topics/{id}` with `{topic_id: null}`)? Because the dual auth gate (course-edit AND linked-Unit access) needs both course and topic context in the path; the `units/{id}/topic-link` shape lacks the course context, and `PATCH topic_id=null` leaks a storage detail at the API layer.

Full handler gate order, spelled out per Codex review:
1. Auth: `claims = GetClaims(ctx)`, return 401 if nil.
2. Load course by `courseId`; return 404 if not found; return 403 if not creator and not platform-admin.
3. Load topic by `topicId`; return 404 if not found OR `topic.CourseID != courseId` (path traversal guard).
4. Lookup current Unit via `GetUnitByTopicID(topicId)`. If nil, return 200 (idempotent — nothing to unlink).
5. Apply the linked-Unit access gate (the same #1 logic above — including the platform-published exception). 403 if denied.
6. `UnlinkUnitFromTopic(unit.ID)` — `UPDATE teaching_units SET topic_id = NULL WHERE id = $1`. Return 200.

### Frontend changes

**New component: `src/components/teacher/unit-picker-dialog.tsx`.**

A shadcn Dialog containing:
- A search Input at the top (debounced ~250ms). Triggers `searchUnits({q, linkableForCourse, gradeLevel, materialType, limit: 20})`.
- A grade-level dropdown (K-5 / 6-8 / 9-12 / Any) and a material-type dropdown (notes / slides / worksheet / reference / Any).
- A scrollable list of result rows. Each row: title (bold), material type + status + grade level (pills), summary (line-clamped to 2). Right side renders one of:
  - **Pick** button if `canLink === true`
  - Disabled "Already linked" badge with the linked topic title when present, otherwise a bare "Already linked" badge
  - Disabled "Cannot link" tooltip if `canLink === false` for any other reason (defensive — backend should not surface these, but the UI shouldn't crash if it does)
- A "Load more" button at the bottom that uses `nextCursor`. Disabled while loading.
- States: loading skeleton (3 placeholder rows), error banner ("Couldn't load units. Try again."), empty state ("No matching units. Try a broader search or create one in the Units library.").

The Dialog is opened by a "Pick a unit…" Button in the Topic editor's "Teaching Unit" Card (replacing the paste-ID input). The existing linked-Unit display (plus "Edit Unit →") is preserved; new "Replace…" Button next to "Edit Unit →" reopens the picker, and a new "Unlink" Button calls the DELETE endpoint.

**`src/lib/unit-search.ts` updates.**
- Add `linkableForCourse?: string`, `materialType?: string` to SearchParams.
- Add `linkedTopicId: string | null`, `linkedTopicTitle: string | null`, `canLink: boolean` to SearchResultItem.
- Distinguish failures: change return type to `{ items: [], nextCursor: null, error?: "network" | "server" }`. The dialog reads `error` to render the error banner instead of treating it as empty results.

## Phases (revised order per Codex review #4)

### Phase 1: Backend — auth gate fix + Unlink endpoint + cursor parsing

Bundles every backend change so Phase 2's frontend has a complete, working API surface.

**Files:**
- Modify: `platform/internal/handlers/topics.go::LinkUnit` — replace the Unit-edit gate (lines ~374-404) with the new gate logic from Design #1. Extract the gate into a helper `canLinkUnitToCourse(claims, unit, course) bool` so Unlink can reuse it.
- Modify: `platform/internal/handlers/topics.go` — add `UnlinkUnit` handler per Design #4. Wire to `DELETE` method on the same path in `platform/cmd/api/main.go`.
- Modify: `platform/internal/store/teaching_units.go` — add `UnlinkUnitFromTopic(ctx, unitID) error`.
- Modify: `platform/internal/handlers/teaching_units.go::SearchUnits`:
  - Parse `materialType` query param; pass to filter.
  - Parse `cursor` query param: split on `|`, RFC3339 timestamp + ID; reject malformed with 400; populate `CursorCreatedAt` + `CursorID`.
  - Parse `linkableForCourse`: load course; gate creator-or-admin (403 otherwise); set new `LinkableForCourseOrgID` on filter.
  - Extend SQL to LEFT JOIN topics ON `topics.id = teaching_units.topic_id`, AND a second LEFT JOIN through `courses` for the cross-org title check, AND a CASE expression that returns `topics.title` only when `picker_course.org_id = linked_topic_course.org_id` OR caller is platform admin (`$N::bool`).
  - Compute `canLink` per row applying the gate logic + already-linked check; include in response payload.
- Modify: `platform/internal/store/teaching_units.go::SearchUnitsFilter` — add `MaterialType string`, `LinkableForCourseOrgID *string`, `LinkedTopicVisibilityOrgID *string` (the org used to gate `linkedTopicTitle`), `IsPlatformAdminForLinkVisibility bool` (or just reuse the existing `IsPlatformAdmin`).
- Modify: `platform/internal/store/teaching_units.go::SearchUnits` SQL builder — implement the LEFT JOINs, CASE expression for `linked_topic_title`, and material_type WHERE clause.
- Modify: `src/lib/unit-search.ts` — add new SearchParams fields, new SearchResultItem fields, distinguished error return.

**Tests:**
- `platform/internal/handlers/topics_link_unit_test.go` — extend with:
  - Teacher links a published platform Unit (was 403 in plan 044, now 200)
  - Teacher cannot link a *draft* platform Unit (still 403)
  - All existing tests still pass with the new gate (idempotent / outsider / wrong-org / etc.)
- `platform/internal/handlers/topics_unlink_test.go` (new) — full gate matrix:
  - Unlink linked Unit as creator → 200
  - Unlink as platform-admin → 200
  - Unlink as outsider (not course creator) → 403
  - Unlink when nothing is linked → 200 (idempotent)
  - Unlink with mismatched topic-course path → 404
  - Unlink a published platform Unit (currently linked) as course creator → 200 (mirrors the link permission)
- `platform/internal/handlers/teaching_units_search_picker_test.go` (new):
  - `linkableForCourse` without course-edit access → 403
  - Personal-scope Units excluded
  - Wrong-org Units excluded
  - Draft platform Units excluded (they're not linkable)
  - Already-linked Units appear with `linkedTopicId` set; same-org case populates `linkedTopicTitle`; cross-org case returns null title
  - `canLink` reflects gate + already-linked combined
  - `materialType` filter narrows results correctly
  - Cursor pagination: page 1 returns nextCursor, page 2 with that cursor returns the next page (no overlap, filters preserved)
  - Malformed cursor → 400

### Phase 2: Frontend — Picker dialog + topic editor swap + e2e

**Files:**
- Create: `src/components/teacher/unit-picker-dialog.tsx` — Dialog component per Design.
- Modify: `src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx`:
  - Remove paste-Unit-ID input + handleLinkUnit() body.
  - Add `<UnitPickerDialog open={...} onPicked={...} courseId={...}>` triggered by a "Pick a unit…" Button.
  - Add "Replace…" Button next to "Edit Unit →" when a Unit is linked.
  - Add "Unlink" Button that calls `DELETE /api/courses/.../link-unit` and refreshes the linked-Unit display.

**Tests:**
- `tests/integration/unit-picker-dialog.test.tsx` (new, Vitest + React Testing Library):
  - Opens dialog, types "python" → searchUnits called with debounced query
  - Filter dropdown changes propagate to searchUnits
  - Click Pick on a row → POST to link-unit, dialog closes, parent's `onPicked` called
  - Already-linked rows have a disabled Pick (assert by ARIA / button disabled)
  - Already-linked row in cross-org case shows "Already linked" without topic title
  - Server error → error banner shown (mocked fetch returns 500)
  - Empty results → empty state shown
  - Load more → nextCursor sent on next request
  - Loading state → skeleton placeholders visible during fetch
- `e2e/unit-picker.spec.ts` (new Playwright):
  - Log in as teacher (via `auth.setup.ts` saved state), navigate to a topic editor.
  - Click "Pick a unit…", search, pick a Unit, assert it appears in the topic editor's linked-Unit display.
  - Click "Replace…", pick a different Unit, assert the displayed title updates.
  - Click "Unlink", assert the Unit display disappears and the "Pick a unit…" Button reappears.

### Phase 3: Documentation

**Files:**
- Modify: `docs/plans/044-topic-syllabus-refactor.md` — append a one-line note under Out-of-Scope: "Plan 045 widened the link auth gate to allow published platform-scope Units."
- Modify: `TODO.md` — remove the "Plan 045: Unit-attach picker UI" entry (this plan completes it).

## Verification per Phase

- **Phase 1:** `cd platform && DATABASE_URL=... go test ./internal/handlers/ ./internal/store/ -run "Search|LinkUnit|UnlinkUnit" -count=1` passes (existing 9 LinkUnit + 4-6 new Unlink + 8-10 new Search picker = ~25 tests in this slice).
- **Phase 2:** `node_modules/.bin/vitest run tests/integration/unit-picker-dialog.test.tsx` passes; `bun run test:e2e -- e2e/unit-picker.spec.ts` passes against a running stack (Next.js + Go + Hocuspocus all up). Manual smoke via Playwright recorder: open topic editor, pick → replace → unlink.
- **Final (across both phases):** Full Vitest + Go suites green. Type-check clean for plan-045 surface.

## Risk Surface

- **Auth gate change is a real semantic widening.** Plan 045 modifies plan 044's auth model (platform-published Units become linkable by any course teacher). This is intentional — published platform Units are content-library content, meant for reuse — but it's not a bug fix, it's a decision. If product changes its mind later (e.g., "platform Units should be sandboxed"), the test in `topics_link_unit_test.go` named "TestLinkUnit_TeacherLinksPlatformPublishedUnit_Allowed" is the contract that needs flipping. Documented in the post-execution report.
- **Cross-org title leak guard requires careful SQL.** The CASE expression in the SearchUnits SQL is the only thing preventing a teacher from seeing another org's topic titles via the picker. Phase 1 tests must explicitly cover the cross-org case (different orgs, both Units visible to the caller via platform-scope, picker should redact the linked topic title).
- **Cursor parsing must reject malformed cursors.** Otherwise a malicious or stale cursor crashes the SQL query. 400 + clear error message.

## Out-of-Scope Acknowledgements

- Plan 046 — drop deprecated topic columns. Independent of 045 but can ship in either order.
- Plan 047 — Topic rename + multi-Unit-per-Topic exploration.

## Codex Pre-Implementation Review

- **Date:** 2026-04-27
- **Verdict:** Plan needed substantive rewrite; this document IS the post-correction plan.

### Corrections applied

1. `[CRITICAL]` **Cross-org title leak via LEFT JOIN.** Original draft populated `linkedTopicTitle` from a plain LEFT JOIN, which would leak another org's topic title for a linked platform-scope Unit. → Design #2 (linkedTopicTitle) now CASE-redacts to null unless caller is platform-admin or the linked topic's course org matches the picker's course org. Phase 1 test "Already-linked Units in cross-org case returns null title" guards the regression.
2. `[CRITICAL]` **Picker offers Units the user can't link.** Original plan inherited plan 044's Unit-edit gate, which blocks teachers from linking platform-scope Units. → Design #1 explicitly widens the link/unlink auth gate to allow published platform-scope Units when the caller has course-edit rights. Phase 1 test "Teacher links a published platform Unit" exercises the new path.
3. `[IMPORTANT]` **Phase ordering ships Unlink button before Unlink endpoint.** → New Phase 1 bundles all backend work (auth gate fix + Unlink endpoint + cursor parsing); Phase 2 ships the frontend with a complete API surface.
4. `[IMPORTANT]` **Unlink auth gate spelled out.** → Design #4 walks the full chain: claims → course → course-edit → topic → topic-belongs-to-course → linked-Unit lookup → linked-Unit access gate → unlink.
5. `[IMPORTANT]` **Material type filter unwired.** Original plan promised it without an implementation task. → Phase 1 lists `materialType` as a SearchParams + SearchUnitsFilter + WHERE clause + handler parse + test.
6. `[IMPORTANT]` **Cursor pagination broken.** Pre-existing bug — handler emits cursor but doesn't parse it. → Phase 1 adds the parse + Phase 1 test for "page 1 returns cursor, page 2 returns next results without overlap."
7. `[IMPORTANT]` **Already-linked UI invariant lacked tests.** → Phase 2 dialog test asserts already-linked rows have a disabled Pick AND cross-org rows show no topic title.
8. `[MINOR]` **Unlink URL choice rationale missing.** → Design #4 now includes a 2-sentence justification.
9. `[MINOR]` **Loading/error states indistinguishable from empty.** → `unit-search.ts` returns a `{ error }` field; dialog renders separate loading skeleton, error banner, and empty state. Phase 2 test covers each.
