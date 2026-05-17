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
3. **Role detection inside the page**: read the NextAuth session server-side; branch on `session.user.isPlatformAdmin` and on the set of active org memberships from `/api/orgs`.
4. **Owner-name resolution**:
   - Always call `/api/orgs` (caller's memberships) — covers teacher / org_admin.
   - If platform admin, additionally call `/api/admin/orgs` — covers cross-org books.
   - Org-name map merges both sources; unknown `scopeId` falls back to the raw UUID rendered as `Org · <prefix>`.
5. **Create-button affordances**:
   - Platform admin: "Create book" with scope picker (platform | any org from `/api/admin/orgs`).
   - Org member with `teacher` or `org_admin` role in ≥1 active org: "Create book" restricted to their own org(s) — scope picker shows org(s) only.
   - All other roles (student / parent / unaffiliated): no create button (backend would 403 anyway).
6. **Detail page**: same role-aware data fetches; chapter list is already scope-filtered correctly by the existing `/api/chapters?bookId=…` endpoint. The 403 fallback UI from `/admin/books/[id]` is dropped — the new page uses 404-no-leak semantics for unauthorized callers (consistent with `canViewBook` behavior).
7. **Nav consolidation**: one entry per portal config still — but all three (`admin`, `org_admin`, `teacher`) point at `/library`. Justification for keeping the entry in each role config: the sidebar groups entries by role section, so removing it from one role would hide it for users with only that role. Identical href across roles is fine; the active-match logic already handles it.
8. **Redirects**: 308 in `next.config.ts` for old paths:
   - `/admin/books` → `/library`
   - `/admin/books/:id` → `/library/:id`
   - `/teacher/books` → `/library`
   - `/teacher/books/:id` → `/library/:id`
   - `/org/chapters` → `/library` (this one is debatable — `/org/chapters` is the old flat chapter list; it's about to be removed from nav, but the page itself is still useful as "all chapters under this org". Decision: redirect to `/library` since the user explicitly framed this as "Library" navigation. If the flat chapter list is still needed, surface it from inside `/library` under a tab/filter later.)
9. **Cleanup**: delete the 6 old role-specific files (admin/books/page.tsx, admin/books/[id]/page.tsx, admin/books/book-create-trigger.tsx, admin/books/[id]/book-edit-trigger.tsx, teacher/books/page.tsx, teacher/books/[id]/page.tsx, teacher/books/teacher-book-create-trigger.tsx, teacher/books/[id]/book-edit-trigger.tsx — 8 files actually). Leave `src/components/books/` (BookActions, BookEditDialog, BookPickerDialog, etc.) intact — they're shared library components.
10. **BookActions `detailBasePath` prop**: change default from `/admin/books` to `/library`, drop the prop everywhere it was being passed (since there's now only one detail base).

## Non-goals

- **No backend changes.** `/api/books`, `canViewBook`, `canEditBook` are all already correct for this work.
- **No new `/library` chapter filter / tab.** The page lists books. Chapters appear on the book detail page. If a "show all chapters across my books" view is wanted, that's a follow-up.
- **No org-admin-specific UI affordances.** Org admin sees the same Library view as teacher (with the same create scope). The role separation is at the org membership level, not the UI level.
- **No `/library` for student / parent.** Those roles don't have a Library nav entry and shouldn't see books in their portal. They can still hit the URL directly and the backend will return only what they can see (probably empty).

## Phases

### Phase 1 — New `/library` list + detail pages

**Files**:
- Create: `src/app/(portal)/library/page.tsx` (role-aware list, ~200 lines).
- Create: `src/app/(portal)/library/[id]/page.tsx` (role-aware detail, ~250 lines).
- Create: `src/app/(portal)/library/library-book-create-trigger.tsx` (~50 lines — merges admin + teacher triggers with role-conditional scope picker).
- Create: `src/app/(portal)/library/[id]/library-book-edit-trigger.tsx` (~40 lines — based on admin's; same as teacher's since edit dialog is shared).

**Tasks**:
1. Server-side `auth()` call to get session + role flags + memberships.
2. Parallel fetch: `api<{ items: Book[] }>("/api/books")` + `api<OrgMembership[]>("/api/orgs")` + (admin only) `api<{ items: Org[] }>("/api/admin/orgs")`.
3. Compose org-name map from member memberships ∪ admin orgs.
4. Compute create-button affordance per Decision #5.
5. Render list (matches teacher books page visual treatment — clean table, no admin-specific scope filter dropdown for now).
6. Detail page: same fetch pattern + `/api/books/${id}` + `/api/chapters?bookId=${id}&scope=…`.
7. 404 fallback for both pages when book is invisible (no 403 UI — consistent with `canViewBook` no-leak semantics).

**Verify**: page renders for admin / teacher / org_admin / student (empty); create button appears for admin + teacher + org_admin only.

### Phase 2 — Nav consolidation + redirects

**Files**:
- Modify: `src/lib/portal/nav-config.ts` — 3 entries change href from `/admin/chapters`, `/org/chapters`, `/teacher/chapters` (or whatever PR #154 set them to) → `/library`. Label "Library", icon "library".
- Modify: `src/lib/portal/icons.ts` — add `"library": "📚"` (the entry PR #154 was going to add).
- Modify: `next.config.ts` — append 5 redirect rules (Decision #8).
- Modify: `src/components/books/book-actions.tsx` — default `detailBasePath` from `/admin/books` → `/library`; audit callers and drop the prop where they were passing the default.
- Modify: `src/components/portal/sidebar.tsx` + `sidebar-section.tsx` + `src/lib/portal/active-match.ts` — update inline comments that reference "Chapters" / per-role paths.

**Verify**: clicking "Library" from any portal lands at `/library`; old URLs 308-redirect; active highlight works on `/library`, `/library/[id]`, and during redirect.

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

1. **Role detection in server component**: NextAuth session shape on the server differs from the client hook. Need to use `auth()` from `@/lib/auth` — verify the helper exists and returns `isPlatformAdmin` + memberships. If memberships aren't on the session, fetch via `/api/orgs` as the page does today.
2. **Mobile bottom-nav 4-item slice**: `sidebar.tsx:71` slices `navItems.slice(0, 4)`. If "Library" lands at index 5+ in some role's config after the rewrite, it drops off mobile. Sequence: dashboard → library → next 2 items.
3. **PR #154 supersession**: with #154 unmerged, the relabel work it did still applies — I'll fold the icon-map addition and label rename into Phase 2 inline rather than rebasing #154's branch.
4. **Detail page chapter list relies on `chapters?bookId=` filter** added in plan 088 phase 3 follow-up (`f868da6`). Confirmed present.
5. **Redirect of `/org/chapters` → `/library`** (Decision #8) is a behavior change for any direct-link / bookmark / email pointing at the old org chapter list. Low risk on local dev, but flag in PR body.

## Plan Review

(Pending — Codex dispatch after self-review.)

## Code Review

(Pending — after Phase 4.)

## Post-Execution Report

(Pending.)
