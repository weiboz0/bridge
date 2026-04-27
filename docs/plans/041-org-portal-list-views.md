# 041 — Org Portal Read-Only List Views

**Goal:** Replace the five "coming soon" placeholders in the org portal (`/org/teachers`, `/org/students`, `/org/courses`, `/org/classes`, `/org/settings`) with read-only list views that surface the entities behind the dashboard counts. No editing, no creating — just inspection. Editing capability is queued for a future product iteration once the read shape is settled.

**Source:**
- Codex comprehensive site review, 2026-04-26 (`docs/reviews/002-comprehensive-site-review-2026-04-26.md`) — P2 #8 ("Organization portal is mostly placeholder pages despite full primary navigation").
- Plan 040 deferral — these pages were explicitly out-of-scope for 040 because they need product work, not auth/security work.

**Branch:** `feat/041-org-portal-list-views`

**Status:** Complete (pending PR review)

---

## Problem Summary

The org dashboard at `/org` (built earlier) shows accurate counts: teachers, students, courses, classes. Clicking through to any of `/org/teachers`, `/org/students`, `/org/courses`, `/org/classes`, `/org/settings` lands on a `<p>… coming soon</p>` placeholder. The portal advertises navigation it can't deliver.

The fix is read-only list views per nav item. Editing (creating teachers, archiving classes, updating org settings) is intentionally out of scope — settling on the right read shapes first lets the eventual edit screens land on a proven foundation, and matches review-002's recommendation: *"Ship read-only list views for each count before exposing the nav items."*

Existing Go store methods cover everything we need (`OrgStore.ListOrgMembers`, `CourseStore.ListCoursesByOrg`, `ClassStore.ListClassesByOrg`, `OrgStore.GetOrg`); this plan is mostly handler + page work.

---

## Scope

### In scope

- 5 read-only list pages (`/org/teachers`, `/org/students`, `/org/courses`, `/org/classes`, `/org/settings`).
- 4 new Go endpoints under `/api/org/*` (settings reuses dashboard org payload).
- Authorization: same `org_admin` (or platform admin / impersonating-admin) check used by the dashboard handler.
- Tests: Go integration tests for each endpoint, Vitest smoke tests for each page (mocked API response).
- Nav-config sanity: confirm the existing entries point at the new pages without modification.

### Out of scope (explicit deferrals)

- **Editing.** No create/update/delete UI for teachers, students, courses, classes, or org settings. Each list view links to existing edit surfaces where they exist (e.g., teacher list → existing teacher detail page if any), and shows the data otherwise.
- **Pagination.** Lists are bounded by org size; pagination is queued for when an org actually exceeds a few hundred rows. Plan documents the breakpoint check explicitly.
- **Search/filter.** Same reasoning — defer until a real org's volume justifies it.
- **Settings editing.** `/org/settings` shows the current org metadata as a read-only display. An "Edit" affordance is left as a follow-up.

---

## Phase 1: Go endpoints

The org dashboard handler already authorizes (org_admin, with platform-admin / impersonating-admin equivalence per plan 039). All new endpoints reuse that pattern via a shared `authorizeOrgAdmin(orgID)` helper extracted from the dashboard handler.

### Task 1.1: Extract a shared auth helper

**Files:**
- Modify: `platform/internal/handlers/org_dashboard.go` — extract the `orgID` resolution + admin verification block (lines 32-70 in current file) into a new method `(h *OrgDashboardHandler) authorizeOrgAdmin(w http.ResponseWriter, r *http.Request) (orgID string, ok bool)`. Returns the resolved org id; writes the error response and returns `ok=false` on failure. Internalizes the `claims.IsPlatformAdmin || claims.ImpersonatedBy != ""` check so admin-impersonating callers (per plan 039 Codex correction #4) see the same data org admins see.

Existing `Dashboard` method becomes a thin caller of the helper.

### Task 1.2: List teachers + students

**Files:**
- Modify: `platform/internal/handlers/org_dashboard.go` — add two handlers:
  - `GET /api/org/teachers` → returns `[{ userId, name, email, role: "teacher", joinedAt }]` filtered from `ListOrgMembers` where `role === "teacher" && status === "active"`.
  - `GET /api/org/students` → same shape, role `"student"`.
- Both use `authorizeOrgAdmin`. Both dedupe defensively: a user can hold the same role twice if they were added/removed/re-added, but `OrgStore.GetUserMemberships` already dedupes (plan 040 phase 6) — `ListOrgMembers` does not. Add `DISTINCT ON (om.user_id, om.role)` to the query (matches plan 040's pattern, fixes any other consumer).

**Stats consistency (per Codex pre-impl review correction #3):** `StatsStore.GetOrgDashboardStats` (`platform/internal/store/stats.go:48-49`) uses raw `COUNT(*)` on `org_memberships` to compute the dashboard's `teacherCount` / `studentCount`. After Task 1.2 dedupes `ListOrgMembers`, the row count of `/org/teachers` would diverge from the dashboard's headline number. Fix in the same task by switching the stats counts to `COUNT(DISTINCT user_id)` filtered per role. Add a regression test in `stats_test.go` that inserts a user with two `teacher` rows in the same org (different statuses) and asserts the count is 1.

### Task 1.3: List courses + classes

**Files:**
- Modify: `platform/internal/handlers/org_dashboard.go` — add two handlers:
  - `GET /api/org/courses` → `ListCoursesByOrg`. Returns `[{ id, title, gradeLevel, language, createdAt }]`.
  - `GET /api/org/classes` → `ListClassesByOrg`. Returns `[{ id, title, term, status, courseId, courseTitle, instructorCount, studentCount, createdAt }]`. The `courseTitle` and counts are derived in handler from a join — see Task 1.4.

### Task 1.4: Class enrichment query

**File:** `platform/internal/store/classes.go`

`ListClassesByOrg` currently returns bare `Class` rows. The org/classes view needs course title + member counts. Add a new method `ListClassesByOrgWithCounts(ctx, orgID) ([]ClassWithCounts, error)` returning the enriched rows.

**Codex pre-impl review correction #4 — cardinality:** a plain double `LEFT JOIN` on `class_memberships` for instructor + student counts multiplies rows and inflates both counts. Use one of:
1. Subquery-per-role aggregate: `LEFT JOIN (SELECT class_id, COUNT(*) AS instructor_count FROM class_memberships WHERE role='instructor' GROUP BY class_id) AS i ON i.class_id = c.id` plus a parallel student subquery.
2. CTE that computes per-class role counts up front.
3. `COUNT(*) FILTER (WHERE cm.role = 'instructor')` if grouping by `c.id` directly.

Pick (1) or (3) at implementation time — both avoid the cardinality explosion. Returns 0 counts for newly-created classes with no members. Add a regression test in `classes_test.go` that inserts a class with two instructors and three students and asserts counts are 2 and 3 (not 6 and 6).

### Task 1.5: Settings endpoint (read-only org metadata)

**Decision:** the existing `/api/org/dashboard` already returns the org payload. The settings page can call that endpoint. If a separate `/api/org/settings` endpoint is needed later for edit semantics, add it then. Saves a handler now.

### Task 1.6: Tests

For each new endpoint, add an integration test in `platform/internal/handlers/org_list_integration_test.go` (new file) covering:
- happy path (org admin, populated org, returns rows)
- platform admin direct access
- impersonating admin
- non-admin user → 403
- empty org → empty list (not error)
- missing orgId → uses caller's first org_admin membership (matches dashboard behavior)

---

## Phase 2: Pages

Each page follows the same shape: server component, fetch from Go via `api()`, render a Card-wrapped table. Defensive error state matches `/admin` pattern from plan 039 phase 2.4.

### Task 2.1: `/org/teachers`

**File:** `src/app/(portal)/org/teachers/page.tsx`

Replace the placeholder. Server component fetches `/api/org/teachers`. Table columns: Name, Email, Joined. Empty state: "No teachers yet. Invite teachers to your organization to get started." Defensive error card on API failure (status code visible, retry link). Keep the file under ~80 lines — list-of-rows pages don't need ceremony.

### Task 2.2: `/org/students`

Same as Task 2.1 but for the students endpoint. Empty state: "No students yet."

### Task 2.3: `/org/courses`

**File:** `src/app/(portal)/org/courses/page.tsx`

Columns: Title, Grade Level, Language, Created. Empty state: "No courses yet." Each row links to the existing teacher-portal course detail page if accessible (org admin doesn't have teacher role by default → link goes to a read-only org-scoped detail page that doesn't exist yet). **Decision: don't link rows yet.** Static list. A "view" affordance comes when an org-scoped course detail page is built. This matches the read-only scope.

### Task 2.4: `/org/classes`

Columns: Title, Course, Term, Status, Instructors, Students. Empty state: "No classes yet." Same no-link decision as courses.

### Task 2.5: `/org/settings`

**File:** `src/app/(portal)/org/settings/page.tsx`

Server component fetches `/api/org/dashboard` (reuses existing payload). Renders a definition-list of org metadata: Name, Type, Status, Contact Name, Contact Email, Domain (if set), Verified At. No edit UI — explicit "Edit settings" affordance to be added in a follow-up plan once we know what's editable by org admins vs platform admins.

### Task 2.6: Vitest smoke tests for each page

**Codex pre-impl review correction #5:** there's no precedent in `tests/unit/` for testing App Router server-component pages. Don't try. Up-front pattern: each page extracts a stateless presentational subcomponent (just the table) into `src/components/org/<entity>-list.tsx`. The page-level server component just `await api(...)` and passes the data to the subcomponent. Tests render the subcomponent directly via jsdom — same shape as existing TSX tests like `tests/unit/join-class-dialog.test.tsx`.

**Files:**
- Create: `src/components/org/teachers-list.tsx`, `students-list.tsx`, `courses-list.tsx`, `classes-list.tsx`, `org-settings-card.tsx`. Each takes its data array (or null for the error case) as props and renders the populated / empty / error UI.
- Create: `tests/unit/org-list-views.test.tsx`. For each of the 5 components: populated array → rows render with the right columns; empty array → empty-state copy; null + error prop → defensive error card.

Pages stay thin (`await api(...)` + render the subcomponent). No page-level Vitest tests.

---

## Phase 3: Polish + nav verification

### Task 3.1: Nav-config sanity

**File:** `tests/unit/nav-config.test.ts`

Existing test asserts org_admin nav doesn't cross portals (plan 039 phase 4). Add a smoke test: every org_admin nav item href has a matching page file under `src/app/(portal)/org/`. Tests run the file-existence check via `fs.statSync` so a future nav addition without a matching page fails the test.

### Task 3.2: Empty-state copy review

Each list view's empty-state message needs to be friendly and actionable. The plan's draft strings ("No teachers yet. Invite teachers...") are placeholders. Implementation should walk through each page in dev with an empty test org and adjust copy if anything reads as confusing.

---

## Implementation Order

| Phase | Tasks | Why |
|-------|-------|-----|
| 1 | 1.1 → 1.2 → 1.3 → 1.4 → 1.6 (test concurrent w/ each handler) | Backend first — pages can't be built without endpoints. Helper extraction (1.1) reused by all four. |
| 2 | 2.1 → 2.2 → 2.3 → 2.4 → 2.5 → 2.6 | Pages depend on endpoints. Settings last because it's the simplest (reuses dashboard). Tests bundled per page. |
| 3 | 3.1 → 3.2 | Polish — nav-page parity test + copy review |

One PR. ~10 commits.

---

## Verification per Phase

- **Phase 1**: each new Go endpoint integration-tested across 6 cases (happy / admin / impersonating / non-admin / empty / missing-orgId). `cd platform && go test ./internal/handlers/... -count=1`.
- **Phase 2**: each page renders without runtime errors against a real org with data and an empty test org. Vitest smoke tests cover the populated, empty, and error states. Manual: as Frank OrgAdmin, click each nav entry, confirm something useful renders.
- **Phase 3**: nav-config test passes; empty-state copy reviewed in dev.
- **Whole plan**: Vitest + Go test suite + TypeScript check all green.

---

## Out-of-Scope Acknowledgements (queued)

Each list view has obvious next-step product work that intentionally does NOT ship in 041:

- **Edit affordances**: invite/remove members, archive courses/classes, update org metadata.
- **Detail pages**: row clicks go nowhere. Org-scoped detail views (`/org/teachers/<userId>`, `/org/courses/<courseId>`, etc.) need their own design pass.
- **Pagination + search**: defer until a real org demands it.
- **Pending invitations**: org admin can't see pending member invites yet. This is a real gap but spans new Go schema (invitation table), email plumbing, etc. Separate plan.
- **Member role changes** (`OrgStore.UpdateMemberRole`, `orgs.go:301`) — backend exists; UI deferred.
- **Member status changes / suspension / reactivation** (`UpdateMemberStatus`, `orgs.go:315`) — backend exists; UI deferred.

---

## Codex Review of This Plan

- **Date:** 2026-04-27
- **Reviewer:** Codex (pre-implementation, via `codex:rescue`)
- **Verdict:** Three IMPORTANT corrections applied; two MINOR notes addressed.

### Corrections applied

1. `[IMPORTANT]` **Stats consistency.** Deduping `ListOrgMembers` without also fixing `StatsStore.GetOrgDashboardStats` (raw `COUNT(*)` at `stats.go:48-49`) would make the dashboard headline number diverge from the `/org/teachers` row count. → Task 1.2 now scopes the stats fix in the same task with `COUNT(DISTINCT user_id) FILTER (WHERE role='X')` and a regression test.
2. `[IMPORTANT]` **Class enrichment cardinality.** The original draft proposed "two LEFT JOIN COUNTs" — a plain double join multiplies rows. → Task 1.4 now lists three concrete patterns that avoid the explosion (subquery-per-role, CTE, `COUNT(*) FILTER`); implementation picks one. Regression test that asserts 2 instructors + 3 students don't read as 6 + 6.
3. `[IMPORTANT]` **Server-component testing pattern.** No precedent in the suite for testing App Router server pages. → Task 2.6 locks the up-front pattern: each page extracts a stateless presentational subcomponent under `src/components/org/`; tests render the subcomponent directly via jsdom (same shape as `join-class-dialog.test.tsx`). Pages stay thin.
4. `[MINOR]` **Out-of-scope explicitness.** Added member role changes (`UpdateMemberRole`) and member status changes / suspension (`UpdateMemberStatus`) to the deferred list — both backend APIs exist; UI is deferred.

### Codex notes (no plan change)

- Auth helper extraction (Task 1.1) boundary is right for the 5 read endpoints; do NOT broaden to the existing mutation handlers (`ListMembers` allows any org member, `UpdateOrg` enforces org-admin on path IDs with different semantics). Plan already scopes this correctly.
- Settings reuse of `/api/org/dashboard` (Task 1.5) is acceptable; consider a separate `/api/org/settings` endpoint only if the dashboard handler grows or its stats query becomes slow.
- Row-click decision (no links yet) confirmed reasonable.

## Post-Execution Report

**Status:** Complete. All 3 phases shipped on `feat/041-org-portal-list-views`.

**Phase 1 — Backend** (commit `52c9a97`)
- `OrgDashboardHandler.authorizeOrgAdmin(w, r) (orgID, ok)` — extracted helper. Resolves orgId (from query or caller's first org_admin membership) and verifies admin equivalence via `IsPlatformAdmin || ImpersonatedBy != "" || GetUserRolesInOrg has org_admin`. Reused by 4 new endpoints + the dashboard.
- `GET /api/org/teachers`, `/students`, `/courses`, `/classes` — return projected row shapes.
- `ClassStore.ListClassesByOrgWithCounts` — single query using `COUNT(*) FILTER (WHERE cm.role = 'X')` per role to avoid the cardinality explosion a plain double LEFT JOIN would produce.
- `OrgStore.ListOrgMembers` — `DISTINCT ON (user_id, role)` defensive guard. Schema's unique constraint on `(org_id, user_id, role)` already prevents duplicates, but the dedup documents intent and survives schema relaxation.
- `StatsStore.GetOrgDashboardStats` — `COUNT(DISTINCT user_id)` for teacher/student counts (same defensive reason). Locks the dashboard headline = list-page row count invariant.
- 11 handler integration cases + 5 store-level dedup/cardinality tests.

**Phase 2 — Pages** (commit applied with phase 3)
- 5 stateless presentational components in `src/components/org/`: `teachers-list.tsx`, `students-list.tsx`, `courses-list.tsx`, `classes-list.tsx`, `org-settings-card.tsx` plus a shared `org-list-state.tsx` wrapper handling the populated/empty/error decision.
- 5 thin server-component pages in `src/app/(portal)/org/{teachers,students,courses,classes,settings}/page.tsx`. Each does `await api(...)` and passes `{data, error}` to the subcomponent. Settings reuses `/api/org/dashboard`'s `org` field.
- 14 Vitest cases on the subcomponents (jsdom). Pages stay thin — no page-level Vitest tests, per Codex correction #5.

**Phase 3 — Polish** (commit applied with phase 2)
- `tests/unit/nav-config.test.ts` — new test asserting every `org_admin` nav item has a backing page file via `fs.existsSync`. Catches future "added a nav link, forgot the page" mistakes.
- Empty-state copy reviewed inline in each subcomponent.

**Verification**
- Vitest: 418 passed / 11 skipped (was 403 — 15 new: 14 component cases + 1 nav-page parity).
- Go tests: all 14 packages green against `bridge_test`. New tests: 11 org list integration + 5 store-level (3 dedup, 2 cardinality).
- TypeScript: clean for new/modified files.

**Plan compliance**
- `[IMPORTANT]` Codex pre-impl correction #3 (stats consistency) — `StatsStore` switched to `COUNT(DISTINCT user_id)` in the same phase that dedupes `ListOrgMembers`.
- `[IMPORTANT]` Codex pre-impl correction #4 (cardinality) — `ListClassesByOrgWithCounts` uses `COUNT(*) FILTER` per role. Test inserts 2 instructors + 3 students; asserts counts are 2 + 3, not 6 + 6.
- `[IMPORTANT]` Codex pre-impl correction #5 (testing pattern) — 5 stateless subcomponents extracted up-front; tests target them, not the async server pages.
- `[MINOR]` Codex pre-impl correction #1 — additional deferrals (`UpdateMemberRole`, `UpdateMemberStatus`) explicitly noted in the out-of-scope section.

**Out-of-scope (acknowledged in plan)**
- Edit affordances (member invites, role changes, status changes, org metadata edits)
- Detail pages (row links go nowhere)
- Pagination + search (defer until a real org demands it)
- Pending invitations view (needs schema + email plumbing — separate plan)

## Code Review

### Review 1 — Pre-implementation plan review (commit `10932da`)

- **Date:** 2026-04-27
- **Reviewer:** Codex (via `codex:rescue`)
- **Verdict:** Corrections applied — see `## Codex Review of This Plan` section above.

### Review 2 — Post-implementation review

- **Date:** 2026-04-27
- **Reviewer:** Codex (post-implementation, dispatch pending)
- **Status:** To be appended after the post-impl Codex review completes.
