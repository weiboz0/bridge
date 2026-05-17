---
plan: 089
title: Consolidate per-role Library pages into one role-aware top-level page
status: drafting
owner: orchestrator
branch: feat/089-library-consolidation
supersedes: PR #154 (chore/nav-library-rename — "Chapters" → "Library" relabel)
---

# Plan 089 — Library consolidation

## Why

Plan 088 introduced books as the top-level organizing concept above chapters. PR #154 was a quick nav-only follow-up that relabeled "Chapters" → "Library" and repointed each of the 3 portal roles (admin / teacher / org_admin) at its own books page. The result still carries the per-role duplication:

| Role | Current href | Page |
|---|---|---|
| admin | `/admin/books` | 218-line list + 254-line detail |
| teacher | `/teacher/books` | 163-line list + 251-line detail |
| org_admin | `/org/chapters` | flat chapter list (no books page at all) |

~1000 lines across 8 files. The pages diverge only cosmetically — the backend (`GET /api/books`, `canViewBook` / `canEditBook` from plan 088 round-1 cascade) already does visibility-filtered fetches that are uniformly correct for every role. Consolidating into one page removes the duplication, gives org_admin the books view they should have had since plan 088, and pre-empts the per-role drift that always follows nav forks like this.

Supersedes PR #154 entirely — single nav entry repointed once, instead of 3 entries relabeled.

## Decisions

1. **URL**: `/library` (matches the nav label, role-neutral, easy to redirect to from old paths). Detail at `/library/[id]`.
2. **File location**: `src/app/(portal)/library/` — inside the portal shell so it inherits auth + sidebar. Not under `/admin`, `/teacher`, or `/org`.
3. **Role detection inside the page**: fetch `/api/me/portal-access` (see Decision #11 for the full rationale). Branch on `isPlatformAdmin = roles.some(r => r.role === "admin")` and on `orgMemberships = roles.filter(r => r.role === "teacher" || r.role === "org_admin")`. Do NOT call `auth()` directly or fetch `/api/orgs` — both contradict Decision #11.
4. **Owner-name resolution**:
   - Build the org-name map from `roles[]` of the `/api/me/portal-access` response (each `UserRole` carries `orgId` + `orgName`).
   - If platform admin, additionally fetch `api<{ items: Org[] }>("/api/admin/orgs")` and merge — covers cross-org books the caller isn't a member of.
   - Unknown `scopeId` falls back to the raw UUID rendered as `Org · <prefix>`.
   - Do NOT call `/api/orgs` separately — its data is already inside `/api/me/portal-access`.
5. **Create-button affordances**:
   - Platform admin: "Create book" with scope picker (platform | any org from `/api/admin/orgs`).
   - Org member with `teacher` or `org_admin` role in ≥1 active org: "Create book" restricted to their own org(s) — scope picker shows org(s) only.
   - All other roles (student / parent / unaffiliated): no create button (backend would 403 anyway).
6. **Detail page**: same role-aware data fetches; chapter list is already scope-filtered correctly by the existing `/api/chapters?bookId=…` endpoint. The 403 fallback UI from `/admin/books/[id]` is dropped — the new page uses 404-no-leak semantics for unauthorized callers (consistent with `canViewBook` behavior).
7. **Nav consolidation**: dedupe by href in the sidebar render layer rather than in nav-config. The 3 role configs (`admin`, `org_admin`, `teacher`) each keep their own "Library" entry pointing at `/library`. Sidebar groups entries by role section, but when a user has multiple roles and the same href appears in 2+ sections, the second occurrence is suppressed — keeps single-role users covered while multi-role users see one "Library" link.

   **Implementation site (Codex finding #2)**: dedupe lives in `src/components/portal/sidebar.tsx` in a new helper invoked between `roles.map(...)` (line 84) and rendering each `SidebarSection`. Walk sections in `roles` order; maintain a `Set<string>` of hrefs already surfaced (compared by `href` stripped of trailing `?orgId=…` query — that suffix is appended later by `SidebarSection` at line 42, so the dedupe key is the raw nav-config href). Pass the section's filtered `navItems` (excluding already-seen hrefs) to `SidebarSection`. If filtering leaves a section empty, still render the section header (multi-role view) — but suppress the rendering of an empty `<nav>` list. Test in `tests/unit/sidebar-section.test.tsx` (new case) AND in a new `tests/unit/sidebar-dedupe.test.tsx` covering: single-role unchanged, multi-role dedupes by href, dedupe key ignores `?orgId=` suffix, empty-after-dedupe section renders without nav list.
8. **Redirects**: 308 in `next.config.ts` for old book paths only:
   - `/admin/books` → `/library`
   - `/admin/books/:id` → `/library/:id`
   - `/teacher/books` → `/library`
   - `/teacher/books/:id` → `/library/:id`
   - **NOT redirected**: `/org/chapters`, `/admin/chapters`, `/teacher/chapters`. Those are the legacy flat chapter lists — semantically different from "Library = books". They stay reachable by URL; nav stops linking them. A follow-up plan can deprecate them once a "show all my chapters across books" view exists inside Library (if needed).
9. **Cleanup**: delete the 8 old role-specific book files (admin/books/page.tsx, admin/books/[id]/page.tsx, admin/books/book-create-trigger.tsx, admin/books/[id]/book-edit-trigger.tsx, teacher/books/page.tsx, teacher/books/[id]/page.tsx, teacher/books/teacher-book-create-trigger.tsx, teacher/books/[id]/book-edit-trigger.tsx). Leave the legacy `/{admin,teacher,org}/chapters` flat-list pages and `src/components/books/` shared components intact.
10. **BookActions `detailBasePath` prop**: change default from `/admin/books` to `/library`, drop the prop everywhere it was being passed (since there's now only one detail base).
11. **Role-detection helper (Codex finding #3)**: there is no `@/lib/auth/session` module. The canonical session helper exports `auth` from `@/lib/auth` (`src/lib/auth.ts:247`) but the session object only carries `user.id` and `user.isPlatformAdmin` (`src/lib/auth.ts:191`) — memberships are NOT on the session. The page must fetch `api<PortalAccessResponse>("/api/me/portal-access")` (same call `PortalShell` makes at `src/components/portal/portal-shell.tsx:31`) to get `{ authorized, userName, roles: UserRole[] }`. From the `roles` array: `isPlatformAdmin = roles.some(r => r.role === "admin")` (treat any admin role as platform admin); `orgMemberships = roles.filter(r => r.role === "teacher" || r.role === "org_admin")` for create-button + owner-name resolution. Avoids the double-fetch of `/api/orgs` (the portal-access endpoint already aggregates the same data).
12. **Backend reference points** (no changes — confirming existing surface): `/api/books` visibility filter at `platform/internal/store/books.go:103-138`; `canViewBook`/`canEditBook` helpers at `platform/internal/handlers/books.go:34-74`; chapter-by-book filter at `platform/internal/handlers/chapters.go` via `ChapterBookFilter` (added in plan 088 phase 3 follow-up `f868da6`).
13. **Portal shell for `/library` (Codex finding #1)**: each existing portal sub-tree has its own `layout.tsx` that calls `<PortalShell portalRole="…">`. `/library` is role-neutral, so it needs either (a) extend `PortalShell` to accept `portalRole?: PortalRole | null` and skip the per-role gate when null (any-authenticated-portal-user), or (b) add a sibling `PortalAnyShell` component. **Decision: (a)**. Smaller diff, single source of truth. Gate becomes: when `portalRole != null`, require `roles.some(r => r.role === portalRole)` as today; when null, require `roles.length > 0` (i.e., authenticated user with at least one portal role). Pass `currentRole = roles[0].role` so the `Sidebar` `currentRole` prop stays populated (it's marked as kept-for-backward-compat anyway). New layout at `src/app/(portal)/library/layout.tsx`: `<PortalShell portalRole={null}>{children}</PortalShell>`. Update `tests/unit/portal-shell.test.tsx` to cover the null-role case.
14. **BookActions callers (Codex finding #4)**: three production sites today — admin list page (uses the default), teacher list page (passes `detailBasePath="/teacher/books"`), unit test `tests/unit/book-actions.test.tsx:72` asserts the old default. After consolidation: default flips to `/library`, teacher-list-page override drops with the page itself (Phase 3 delete), and the unit test asserts the new default. No other production callers — verified via `grep -rn "detailBasePath\|<BookActions" src/`.

## Non-goals

- **No backend changes.** `/api/books`, `canViewBook`, `canEditBook` are all already correct for this work.
- **No new `/library` chapter filter / tab.** The page lists books. Chapters appear on the book detail page. If a "show all chapters across my books" view is wanted, that's a follow-up.
- **No org-admin-specific UI affordances.** Org admin sees the same Library view as teacher (with the same create scope). The role separation is at the org membership level, not the UI level.
- **No `/library` for student / parent.** Those roles don't have a Library nav entry and shouldn't see books in their portal. They can still hit the URL directly and the backend will return only what they can see (probably empty).

## Phases

### Phase 1 — Portal shell + new `/library` list + detail pages

**Files**:
- Modify: `src/components/portal/portal-shell.tsx` — make `portalRole` accept `PortalRole | null` per Decision #13; when null, gate as any-authenticated-portal-user.
- Modify: `src/lib/portal/types.ts` — update `PortalShellProps` (or wherever portalRole type lives) to allow null.
- Modify: `tests/unit/portal-shell.test.tsx` — add null-role case (auth pass, no role-specific gate).
- Create: `src/app/(portal)/library/layout.tsx` — `<PortalShell portalRole={null}>{children}</PortalShell>` (~5 lines).
- Create: `src/app/(portal)/library/page.tsx` (role-aware list, ~200 lines).
- Create: `src/app/(portal)/library/[id]/page.tsx` (role-aware detail, ~250 lines).
- Create: `src/app/(portal)/library/library-book-create-trigger.tsx` (~50 lines — merges admin + teacher triggers with role-conditional scope picker).
- Create: `src/app/(portal)/library/[id]/library-book-edit-trigger.tsx` (~40 lines — based on admin's; same as teacher's since edit dialog is shared).

**Tasks**:
1. Extend `PortalShell` per Decision #13 (Codex finding #1). Add null-role branch + currentRole fallback to `roles[0].role`.
2. Add `/library/layout.tsx`.
3. Library `page.tsx`: server component, parallel fetch — `api<PortalAccessResponse>("/api/me/portal-access")` (Decision #11) + `api<{ items: Book[] }>("/api/books")` + (if admin) `api<{ items: Org[] }>("/api/admin/orgs")`. No separate `/api/orgs` call (portal-access already has membership data).
4. Compose org-name map from `roles[].orgId / orgName` (always) ∪ `/api/admin/orgs` (admin only).
5. Compute create-button affordance per Decision #5: admin → "Create book" + scope picker (platform + all orgs); org member with teacher/org_admin role → "Create book" + scope picker restricted to their orgs; otherwise no button.
6. Render list (matches teacher books page visual treatment — clean table, no admin-specific scope filter dropdown for now).
7. Detail page: same fetch pattern + `/api/books/${id}` + `/api/chapters?bookId=${id}&scope=…` (use `ChapterBookFilter` from Decision #12).
8. 404 fallback for both pages when book is invisible (no 403 UI — consistent with `canViewBook` no-leak semantics).

**Verify**: page renders for admin / teacher / org_admin / student (empty); create button appears for admin + teacher + org_admin only.

### Phase 2 — Nav consolidation + sidebar dedupe + redirects

**Files**:
- Modify: `src/lib/portal/nav-config.ts` — 3 entries (admin, org_admin, teacher) change href to `/library`, label "Library", icon "library". (Main is currently still on the pre-#154 "Chapters" labels — assume that's the baseline.)
- Modify: `src/lib/portal/icons.ts` — add `"library": "📚"`.
- Modify: `src/components/portal/sidebar.tsx` — add href-dedupe helper per Decision #7 implementation site spec (Codex finding #2). Walk `sections` in `roles` order; track seen hrefs in a `Set<string>`; pass filtered `navItems` to each `SidebarSection`. Dedupe key = nav-config `href` (not the `?orgId=…`-augmented form).
- Modify: `next.config.ts` — append 4 redirect rules (Decision #8): `/admin/books`, `/admin/books/:id`, `/teacher/books`, `/teacher/books/:id` → `/library`(/`:id`). Do NOT redirect `/admin/chapters`, `/teacher/chapters`, `/org/chapters` (Decision #8 rationale).
- Modify: `src/components/books/book-actions.tsx` — default `detailBasePath` from `/admin/books` → `/library`. Production callers per Decision #14: admin list page (uses default — no change needed in the page itself since it'll be deleted in Phase 3 anyway), teacher list page (override `/teacher/books` — also deleted Phase 3). No remaining callers post-delete will pass the prop.
- Modify: `src/components/portal/sidebar.tsx` + `sidebar-section.tsx` + `src/lib/portal/active-match.ts` — update inline comments that reference "Chapters" / per-role paths.
- Create / modify: `tests/unit/sidebar-dedupe.test.tsx` (new) — 4 cases per Decision #7 spec: single-role unchanged, multi-role dedupes by href, dedupe key ignores `?orgId=` suffix, empty-after-dedupe section renders header without nav list.
- Modify: `tests/unit/book-actions.test.tsx:72` — assert new `/library` default.

**Verify**: clicking "Library" from any portal lands at `/library`; old book URLs 308-redirect; active highlight works on `/library`, `/library/[id]`, and during redirect; multi-role users see exactly one "Library" entry in the sidebar.

### Phase 3 — Delete old per-role pages + add tests

**Files to delete** (8):
- `src/app/(portal)/admin/books/page.tsx`
- `src/app/(portal)/admin/books/[id]/page.tsx`
- `src/app/(portal)/admin/books/book-create-trigger.tsx`
- `src/app/(portal)/admin/books/[id]/book-edit-trigger.tsx`
- `src/app/(portal)/teacher/books/page.tsx`
- `src/app/(portal)/teacher/books/[id]/page.tsx`
- `src/app/(portal)/teacher/books/teacher-book-create-trigger.tsx`
- `src/app/(portal)/teacher/books/[id]/book-edit-trigger.tsx`
- Plus any now-empty `__tests__` siblings.

**Tests** (Sonnet — frontend domain):
- Create: `tests/unit/library-page.test.tsx` — render-as-admin / render-as-teacher / render-as-org_admin / render-as-student (empty) / unauthorized 404.
- Create: `tests/unit/library-book-detail-page.test.tsx` — same role matrix + chapters listed + invisible-book 404.
- Update / delete: tests for the removed admin-books / teacher-books pages.

**Verify**: `bun run test` baseline unchanged (3 pre-existing auth-jwt failures), no new failures from removed files.

### Phase 4 — Verify + docs

- `bun run lint` baseline unchanged (currently 145 problems on main).
- `bunx tsc --noEmit` baseline unchanged (currently 8 errors).
- `bun run test` — new library page tests pass.
- `bun run test:e2e` — at minimum the chapters-related e2e specs need a smoke check; if any reference `/admin/books` or `/teacher/books` paths, redirects keep them working.
- Update `docs/api.md` if any endpoint usage example references the old paths (likely none — that doc is API-shape focused).
- Update `docs/project-structure.md` portal-routes table to reflect the consolidation.

## Risks

1. **Role detection in server component**: resolved per Decision #11 — use `api<PortalAccessResponse>("/api/me/portal-access")` (same endpoint `PortalShell` already calls). Do NOT use `auth()` from `@/lib/auth` directly and do NOT fall back to `/api/orgs` — both contradict Decision #11. Risk-level concern: a future change to `/api/me/portal-access`'s response shape (currently `{ authorized, userName, roles: UserRole[] }`) breaks the page in step with `PortalShell` — one breakage point, not two.
2. **Mobile bottom-nav 4-item slice**: `sidebar.tsx:71` slices `navItems.slice(0, 4)`. If "Library" lands at index 5+ in some role's config after the rewrite, it drops off mobile. Sequence: dashboard → library → next 2 items.
3. **PR #154 supersession**: with #154 unmerged, the relabel work it did still applies — I'll fold the icon-map addition and label rename into Phase 2 inline rather than rebasing #154's branch.
4. **Detail page chapter list relies on `chapters?bookId=` filter** added in plan 088 phase 3 follow-up (`f868da6`). Confirmed present.
5. **No e2e specs hit `/admin/books` or `/teacher/books`** (Codex finding #6) — confirmed via `grep -rn "/admin/books\|/teacher/books" e2e/`. The Phase 2 308 redirects carry zero regression risk from current e2e. If future specs target Library, they should hit `/library` directly.

## Plan Review

### Round 1

#### Self-review (Opus 4.7) — CONCUR with 5 fixes folded

Concerns folded into Decisions #7-12:

1. Original Decision #7 contradicted the user framing by keeping 3 separate Library nav entries. Revised: dedupe by href in the sidebar render layer; keep single-role coverage via per-role configs.
2. Original Decision #8 redirected `/org/chapters → /library`, conflating "flat chapter list" with "books library". Revised: only redirect old book paths; leave legacy flat-chapter pages reachable by URL but unlinked from nav.
3. Role-detection helper was unpinned. New Decision #11 specifies the session helper + fallback path.
4. Backend reference points weren't called out — implementer would re-discover them. New Decision #12 cites file:line for the `/api/books` filter, `canViewBook`/`canEditBook`, and `ChapterBookFilter` from plan 088 phase 3 follow-up.
5. Phase 3 file-delete list said "6 old role-specific files" but counted 8. Phrasing corrected.

No remaining blockers from self-review. Ready for Codex dispatch.

#### Codex — BLOCKER (5 findings + 1 low) — folded in `<next commit>`

1. **`/library` has no portal shell** — folded into Decision #13 + Phase 1 task #1. Plan now specifies extending `PortalShell` to accept `portalRole: PortalRole | null` plus the null-role gate semantics; new `/library/layout.tsx` calls it with `portalRole={null}`; `tests/unit/portal-shell.test.tsx` gets a null-role case.

2. **Dedupe logic not itemized** — folded into Decision #7's implementation site spec + Phase 2's `tests/unit/sidebar-dedupe.test.tsx` deliverable. Spec names the file + function placement (helper between `roles.map(...)` and `<SidebarSection>` at `src/components/portal/sidebar.tsx:84`), the Set-based filter, and the 4 test cases (single-role, multi-role dedupe, `?orgId=` key normalization, empty-after-dedupe section render).

3. **`@/lib/auth/session` doesn't exist** — folded into Decision #11 (replaced with correct citation). Plan now references `auth` from `@/lib/auth:247`, notes that memberships aren't on the session, and routes role-detection through `/api/me/portal-access` (same endpoint `PortalShell` already calls — avoids double-fetching `/api/orgs`).

4. **BookActions callers** — folded into Decision #14 listing all 3 sites (admin list, teacher list, unit test) and what changes for each.

5. **Risk #5 contradicted Decision #8** — Risk #5 rewritten to drop the wrong claim and instead record the validated `e2e/` grep finding (no specs hit `/admin/books` or `/teacher/books`).

6. **(Low)** No e2e regression risk from the redirect — captured in revised Risk #5.

**Verdict after fold-in**: pending re-dispatch of Codex against the revised plan tip.

### Round 2

#### Codex — BLOCKER (1 new finding from round-1 fold)

- Findings 1, 2, 4, 5 confirmed RESOLVED.
- **New blocker**: Risk #1 still described `auth()` + `/api/orgs` fallback, contradicting Decision #11's `/api/me/portal-access`-only mandate. An implementer reading Risk #1 could choose the wrong auth path. → **Fixed in next commit**: Risk #1 rewritten to cite Decision #11 explicitly and forbid the old paths.

### Round 3

#### Codex — BLOCKER (2 stale contradictions surfaced)

Round-2 BLOCKER (Risk #1) confirmed RESOLVED, but Codex flagged 2 more contradictions the round-2 fold missed:

- **Decision #3** still read "read the NextAuth session server-side; branch on `session.user.isPlatformAdmin` and on the set of active org memberships from `/api/orgs`" — contradicts Decision #11.
- **Decision #4** still read "Always call `/api/orgs`" — contradicts Decision #11.

→ **Fixed in next commit**: Decisions #3 and #4 rewritten to source role + memberships from `/api/me/portal-access` and explicitly prohibit `auth()` / `/api/orgs` direct calls. All references to `/api/orgs` in Decisions / Risks / Phases now appear only as explicit "do NOT" warnings or historical-audit-trail notes.

### Round 4

#### Codex — CONCUR (gate clean)

Items 1 (Decision #3), 2 (Decision #4), and 3 (no remaining contradictions across Decisions / Risks / Phases) all confirmed PASS. The 2-way plan gate is satisfied: **self-review ✓ + Codex ✓**. Plan 089 ready for implementation.

## Code Review

### Self-review (Opus 4.7) — 1 fix folded

After reading the consolidated diff, found one issue Risk #2 had warned about:

- **`org_admin` had Library at index 5 in nav-config**, which falls outside the mobile bottom-nav `slice(0, 4)`. The other 2 roles (admin index 2, teacher index 1) were safe. **Fixed**: moved Library to index 1 for `org_admin` (right after Dashboard, mirroring the teacher pattern). All 7 sidebar-dedupe tests still pass.

Other concerns considered + resolved without changes:
- **Dedupe key vs `?orgId=` suffix**: dedupe runs before `SidebarSection` appends the suffix, so the key is the raw nav-config href. Correct.
- **Multi-role active-match on `/library`**: highlight fires for whichever role section keeps the entry post-dedupe (the user's primary role per `roles` order). Acceptable — the user sees they're on Library either way.
- **Mobile uses pre-dedupe `primaryConfig.navItems`**: no dedupe needed for mobile (it's single-role). Correct.
- **canEdit gate on detail page chapter list** (Phase 1 deviation): correct UX — verified the chapter-link path renders for read-only viewers too (just no edit button).

Ready for Codex + GLM dispatch.

### Round 1 — Codex + GLM (parallel, dispatched against `e793a2a`)

#### Codex — BLOCKER (2 findings + 1 WARNING)

1. **BLOCKER — multi-role users on `/library` see Library hidden inside collapsed section.** `computeActiveKey` in `src/components/portal/sidebar.tsx:157` only matched role `basePath`s; `/library` isn't under any basePath, so the helper returned `null` and every section stayed collapsed. The Library link lives in the deduped first section, but a collapsed `SidebarSection` doesn't render its nav list. Tests in `sidebar-dedupe.test.tsx` pre-seeded sections as expanded, so the bug slipped through. → **FIXED in `<next commit>`**: `computeActiveKey` now does a second pass after the basePath loop, walking roles in dedupe order and matching by nav-item href. Plus 2 new Case-5 tests in `sidebar-dedupe.test.tsx` that exercise the auto-expand path (no localStorage seed).

2. **BLOCKER — `org_admin` chapter links route to `/teacher/chapters`, but `/teacher/*` requires teacher role.** `chapterBasePath()` at `src/app/(portal)/library/[id]/page.tsx:62` mapped `org_admin` → `/teacher/chapters`. An org_admin without teacher role would hit the teacher portal layout's role gate (`portalRole="teacher"` at `src/app/(portal)/teacher/layout.tsx:1`) and bounce. Detail-page tests missed it (no `org_admin` case). → **FIXED in `<next commit>`**: `chapterBasePath()` now splits — admin → `/admin/chapters`, teacher → `/teacher/chapters`, org_admin (no teacher) → `/org/chapters`, else null. New "Case 7" test block in `library-book-detail-page.test.tsx` adds 2 cases verifying org_admin chapter links target `/org/chapters/*` and the edit button shows.

3. **WARNING — PortalShell null-role branch is dead code.** `PortalShell` redirects on `!data.authorized` before reaching the `roles.length === 0` branch, and `/api/me/portal-access` sets `authorized = (len(roles) > 0)`. So the null-role fallback inside PortalShell is unreachable for the current backend. → **WONTFIX**: the branch is defensive code in case the backend's `authorized` semantics ever drift (e.g., if it starts gating purely on session validity instead of role count). The current redirect-to-`/login` for no-role users is also arguably correct UX (they need an admin to assign a role). Documented inline in the plan; no code change.

#### GLM 5.1 — CONCUR (2 medium + 3 low)

- **M1 — Double fetch of `/api/me/portal-access`** (layout's PortalShell + page both call it). Next.js App Router auto-dedupes same-URL `fetch` calls within a single server render pass, so this is likely a non-issue in practice. → **WONTFIX with verification deferred**: if perf measurement later shows it matters, wrap with React `cache()`. Not blocking.
- **M2 — `Promise.all([portalAccess, book])` priority inversion.** If book rejects with 404 before portal-access rejects with 401, we'd show "Book not found" instead of redirecting to /login. → **WONTFIX**: PortalShell's layout already enforces 401 before the page renders. The race is theoretical (session expires between layout and page, same request) and the user-visible difference (404 vs login) is minor. If it ever matters, split the `Promise.all`.
- **L1 — Empty-chapters case untested**: `[id]/page.tsx:211` renders "No chapters assigned yet" but no test exercises it. → Noted; not blocking.
- **L2 — Scope-filter dropdown dropped vs admin-books-page.** Plan 089 Phase 1 explicitly deferred this (no admin-specific scope filter in MVP). Acceptable. → Noted as follow-up.
- **L3 — Student-only user can hit `/library`** and sees empty list. Confusing UX but not a security issue. → Noted; future plan can role-gate `/library` if needed.

### Round 2 — pending Codex re-dispatch against the BLOCKER fixes.

## Post-Execution Report

### Commits on the branch (in order)

- Plan iterations (`671e66c → 821fbbf → 2815c90 → e7345a2 → b25cc9c → b8610c0`): draft → self-review fold → Codex round-1 fold (5 BLOCKERs) → round-2 fold (1 BLOCKER) → round-3 fold (2 stale contradictions) → round-4 Codex CONCUR (2-way gate clean).
- Reviewer-policy update (`49e3fb2`): switch to 2-way plan review + 3-way code review; drop DeepSeek; bundled into this branch per "no trivial / doc-only PRs" feedback.
- Phase 1 (4 commits `f6f7e9c → 25649a1`): Sonnet — extend `PortalShell` for null role + 9-case test; new `/library/layout.tsx`, list page (199 lines), detail page (289 lines), create+edit triggers (33+35 lines).
- Phase 2 (4 commits `852e249 → 11961ab`): Sonnet — nav-config + library icon, sidebar href-dedupe + 7-case test, 4× 308 redirects in `next.config.ts`, `BookActions detailBasePath` default flipped to `/library` + test update.
- Phase 3 (2 commits `76556fa → b2dbede`): Sonnet — `git rm` 8 old per-role book files + 2 stale test files; new `library-page.test.tsx` (21 cases) + `library-book-detail-page.test.tsx` (17 cases).
- Phase 4 (this commit): orchestrator — `docs/api.md` Library-consolidation note, post-execution report.

### File census (branch vs main)

- 28 files changed, **net –728 lines** (1370 deleted vs 642 added).
- 8 source files deleted (the old admin/books + teacher/books pages).
- 2 source files added (`/library/page.tsx`, `/library/[id]/page.tsx`).
- 4 new helper/trigger files (layout, create-trigger, edit-trigger).
- 5 modified files: `portal-shell.tsx`, `sidebar.tsx`, `sidebar-section.tsx`, `active-match.ts`, `nav-config.ts`, `icons.ts`, `book-actions.tsx`, `next.config.ts`.
- 4 new test files (`portal-shell.test.tsx`, `sidebar-dedupe.test.tsx`, `library-page.test.tsx`, `library-book-detail-page.test.tsx`) + 2 deleted (`admin-books-page.test.tsx`, `admin-book-detail-page.test.tsx`) + 1 modified (`book-actions.test.tsx`).
- 54 new/updated test cases across 5 test files.

### Deviations from the plan

- **`PortalShellProps` type lived inline** in `portal-shell.tsx`, not in `src/lib/portal/types.ts` as the plan assumed. Modified in place. Phase 1 deliverable #7 was a no-op.
- **No `canCreatePlatformBook` prop** added to `library-book-create-trigger.tsx` (Phase 1) — would require a corresponding `BookEditDialog` prop change, which is Phase 2 scope at best and arguably its own follow-up. Backend `canEditBook` will 403 illegitimate creates as a safety net. Noted in the trigger's comment.
- **`canEdit` gate added to detail page chapter list** (Phase 1) — plan was silent but it's the correct UX. Edit button shows only for platform admin or for org members whose `orgId` matches `book.scopeId`.
- **`docs/project-structure.md` portal-routes table doesn't exist** — Phase 4 deliverable #2 was a no-op. The doc is a 37-line directory map; routes aren't enumerated there.

### Verification (final)

- `bunx tsc --noEmit` — 8 errors, identical to main baseline. Zero from plan-089 files.
- `bun run lint` — 145 problems, identical to main baseline. Zero introduced.
- `bun run test` — 830 pass / 3 pre-existing `auth-jwt-refresh` failures / 11 skipped. Same baseline as plan 088.
- New test files all pass: `portal-shell` (9), `sidebar-dedupe` (7), `library-page` (21), `library-book-detail-page` (17) — 54 new cases, 100% pass.
- Manual smoke (deferred to user): clicking "Library" from any portal → `/library`; `/admin/books` → 308 → `/library`; visiting `/library/<book-id>` shows the chapter list.

### Known limitations / follow-up work

- **Flat `/admin/chapters`, `/teacher/chapters`, `/org/chapters` pages remain** (Decision #8). They're no longer in nav but still reachable by URL — useful for "show all chapters across books" until Library exposes that view directly. A follow-up plan can either delete them or surface a `Library → All chapters` tab.
- **No e2e smoke test added** for `/library` — plan 089 doesn't add e2e (none existed for the old book pages either). A future plan touching Library should add one Playwright spec.
- **Sidebar dedupe is implemented for the Library case** but generalizes. If two roles ever share another href, dedupe applies automatically — no per-href config needed.
- **`/org/books` doesn't exist as a separate route** (org admin lands at `/library` like everyone else). The plan-088 follow-up about creating a dedicated `/org/books` page is moot.

### PR notes

- Supersedes PR #154 (closed `2026-05-17` with supersession comment).
- 3-way code review gate (self + Codex + GLM per new reviewer policy) pending dispatch before PR open.
