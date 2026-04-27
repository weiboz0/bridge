# 043 — Review 003-005 Fixes: Security, Multi-Org Context, Editor Squeeze, OAuth, P2 Cleanup

**Goal:** Address findings across three pending reviews — the older teacher portal review (`003-teacher-portal-review-2026-04-26.md`), older student portal review (`004-student-portal-review-2026-04-26.md`), and the fresh comprehensive review (`005-comprehensive-site-review-2026-04-27.md`). Two of the older review findings are P0 security gaps that survived plans 039-042; ship those first. The rest is a tight cleanup pass on the remaining P1 and P2 items that aren't taxonomy-scoped.

**Sources:**
- `docs/reviews/003-teacher-portal-review-2026-04-26.md`
- `docs/reviews/004-student-portal-review-2026-04-26.md`
- `docs/reviews/005-comprehensive-site-review-2026-04-27.md`

**Branch:** `feat/043-review-003-fixes`

**Status:** Draft — awaiting approval

---

## Problem Summary

After plans 039-042 closed the auth identity drift, replaced org placeholders, and shipped editor responsive layout, three reviews surface a remaining mix of issues:

1. **Two P0 security gaps** — `GetClass` returns class metadata to any authenticated user, and `JoinSession` joins by session ID alone with no class-membership check. Both pre-date plan 039 and aren't dependent on the auth identity work; fixing them requires explicit per-endpoint authorization.
2. **Workflow gaps** — multi-org admins silently locked to the first org, editor still squeezed at 1024px+sidebar, Google OAuth drops invite/role intent, ended sessions render the live dashboard.
3. **P2 polish** — Create Unit scope flicker, missing UUID validation on a few deep routes, platform-admin Settings placeholder, responsive E2E coverage gaps.

The Topic→Syllabus Area taxonomy refactor (teacher review P1, student review P1) and the My Work redesign (student review P2) are deferred to follow-up plans because they're substantial product work, not bug fixes.

---

## Scope

### In scope (this plan)

- **P0**: `GetClass` membership check, `JoinSession` access check.
- **P1**: ended-session read-only state, multi-org context selector, editor sidebar-squeeze fix, Google OAuth invite/role.
- **P2**: Create Unit scope flicker, create-class UUID validation, student problem route UUID validation, platform-admin Settings trim, responsive E2E hardening.

### Out of scope (explicit deferrals)

- **Plan 044 — Topic → Syllabus Area taxonomy refactor.** Teacher review P1 + Student review P1 both call for demoting Topic to a syllabus/focus-area organizer with Unit references. This spans schema changes, content migration, teacher authoring UI changes, and student delivery rewiring. Substantial product/architecture work — its own plan.
- **Plan 045 — My Work navigability.** Student review P2 says My Work doesn't link saved work back to its owning class/problem. Needs a product call about what "My Work" should actually do.
- **Multi-tenant data isolation broader audit.** The Phase 1 P0 fixes target the two known endpoints. A wholesale audit of every Go endpoint for cross-org leaks would be its own plan.
- **Real platform-admin Settings UI.** Phase 6 trims the placeholder; shipping the actual settings page is a future product cycle.

---

## Phase 1: P0 Security Fixes — Class + Session Access

These two findings are pre-039 bugs that the auth identity work didn't touch. Treat as urgent: if implementation grows beyond a small change, extract Phase 1 to a fast-shipping plan 043a and continue the rest of 043 separately.

### Task 1.1: `GetClass` requires membership or staff/admin role

**Files:**
- Modify: `platform/internal/handlers/classes.go::GetClass` (lines 157-174 currently). After fetching the class, check authorization:
  - Allow if `claims.IsPlatformAdmin || claims.ImpersonatedBy != ""` (admin equivalence per plan 039 correction #4).
  - Allow if the caller is in `class_memberships` for the class.
  - Allow if the caller has an active `org_admin` membership in the class's owning org.
  - Reject with 404 (not 403, to avoid leaking class existence) otherwise.
- Reuse the existing `isSessionAuthority` helper pattern from `platform/internal/handlers/sessions.go` if it can be generalized; otherwise add a `canAccessClass(ctx, classID, claims)` helper to `classes.go` that returns the same shape.

### Task 1.2: `JoinSession` requires class membership OR a valid invite

**Files:**
- Modify: `platform/internal/handlers/sessions.go::JoinSession` (lines 296-319). Before adding the participant, verify access:
  - Allow admin equivalence as above.
  - Allow if the caller has class membership for the session's owning class.
  - Allow if the caller is already in `session_participants` (pre-invited by the teacher via `AddParticipant` or by token).
  - Reject with 403 otherwise.
- The token-based join path (`JoinSessionByToken`) already validates the token — leave that path unchanged.

### Task 1.3: Tests for both endpoints

**Files:**
- Modify: existing `platform/internal/handlers/classes_test.go` and `sessions_test.go` (or create integration tests if they don't exist there). Cover:
  - Member can read class / join session (happy path).
  - Non-member outside the org → 404 / 403 respectively.
  - Org admin → allowed.
  - Platform admin → allowed.
  - Admin impersonating a non-member → allowed (admin equivalence).
  - Pre-invited participant → join allowed.
  - Random user with the session ID → 403.

---

## Phase 2: Ended Sessions Read-Only State

Per teacher review P1: every session in `/teacher/sessions` links to `/teacher/sessions/{id}` which renders the live `TeacherDashboard` regardless of status. Ended sessions need either a distinct review surface or no link.

### Task 2.1: Don't link ended sessions in the listing

**File:** `src/app/(portal)/teacher/sessions/page.tsx` (lines 82-86).

For ended sessions, render a non-link row (just text + status badge) instead of a `<Link>`. Keeps the listing honest without inventing a new surface. A "View summary" affordance is queued for a future product cycle when ended-session review UI is designed.

### Task 2.2: Server-side guard on the page itself

**File:** `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx`.

Read the session status from the page-payload endpoint. If `status !== "live"`, render a "Session ended" notice with a back link to `/teacher/sessions` instead of the workspace. Guards against bookmarks and direct URL access, not just the listing-page link.

### Task 2.3: Tests

- Vitest test for a small `<SessionRow>` (extract if helpful) asserting ended sessions render text rather than a link.
- E2E spec verifying that opening an ended session URL directly renders the "Session ended" notice, not the live workspace.

---

## Phase 3: Multi-Org Context (review 005 P1)

The fix has two parts: backend pages that pass an explicit `orgId`, and a UI selector for users with multiple `org_admin` memberships.

### Task 3.1: Org context helper + URL persistence

**Files:**
- Create: `src/lib/portal/org-context.ts` — pure helpers `parseOrgIdFromSearchParams(sp)` and `appendOrgId(path, orgId)`.
- Create: `src/components/portal/org-switcher.tsx` — client dropdown listing the user's active `org_admin` memberships. Pushes `?orgId=<new>` to current pathname; preserves other query params; keys by `orgId`.

### Task 3.2: Pass orgId through every org page

**Files:** `src/app/(portal)/org/page.tsx`, `org/teachers/page.tsx`, `org/students/page.tsx`, `org/courses/page.tsx`, `org/classes/page.tsx`, `org/settings/page.tsx`. Each accepts `searchParams: Promise<{ orgId?: string }>` and appends `?orgId=<value>` to the `api(...)` call when set.

### Task 3.3: Render the switcher in the org portal

**Files:** `src/app/(portal)/org/layout.tsx` (or whichever file renders the org portal layout). Render `<OrgSwitcher>` near the top when the caller has 2+ active org_admin memberships.

### Task 3.4: Fix the role-switcher duplicate-key

**File:** `src/components/portal/role-switcher.tsx`.

- Key buttons by `${r.role}:${r.orgId ?? "none"}` so two `org_admin` roles for different orgs have distinct keys.
- When switching to a role with `orgId`, navigate to `/<portal>?orgId=<id>`.

### Task 3.5: Tests

- Unit tests for `parseOrgIdFromSearchParams` / `appendOrgId`.
- Component test for `<OrgSwitcher>` (two memberships → click switches `?orgId=`).
- Component test for `<RoleSwitcher>` with two `org_admin` roles in different orgs (no React key warning, switching pushes correct URL).

---

## Phase 4: Container-Aware Editor Breakpoint (review 005 P1 + P2)

### Task 4.1: Switch from viewport `lg:` to a container query

**File:** `src/components/problem/problem-shell.tsx`.

Wrap the shell's outer flex in a `@container/shell`. Replace `lg:flex-row`, `lg:hidden`, `lg:flex`, `lg:w-[…]`, etc. with the container-query equivalents (`@5xl/shell:flex-row`, `@5xl/shell:hidden`, etc.). Container queries use the parent container's width, not the viewport — so the sidebar's 56-224px is automatically excluded.

If `@container` queries don't fit the project's Tailwind setup, fall back to a `ResizeObserver` on the shell that toggles a `data-narrow="true"` attribute, with CSS keyed off that attribute.

### Task 4.2: Update the responsive E2E

**File:** `e2e/problem-editor-responsive.spec.ts`.

- Add a deterministic problem to the spec setup (creates a class + problem if none exists) so the test never silently skips.
- At 1024×768 with the desktop sidebar visible, assert: code pane bounding box ≥ 360px wide.
- At 1280×800, assert wide layout AND code pane ≥ 360px wide.

---

## Phase 5: Google OAuth Invite + Role Carry-Through (review 005 P1)

Approach: pass invite + role through Google's `callbackUrl` to a new `/post-oauth` server-component route that updates the user's `intendedRole` and redirects to the right destination.

### Task 5.1: Pass intent through Google `callbackUrl`

**File:** `src/app/(auth)/register/page.tsx`.

Replace the static `signIn("google", { callbackUrl: "/" })` with a dynamic callback that includes invite + role:

```tsx
const oauthCallback = inviteCode
  ? `/post-oauth?invite=${encodeURIComponent(inviteCode)}&role=${role}`
  : `/post-oauth?role=${role}`;
```

### Task 5.2: `/post-oauth` route

**File:** `src/app/post-oauth/page.tsx` (new).

Server component. Reads `searchParams`, calls `auth()`, looks up the current user via Drizzle. If `intendedRole` is null AND `searchParams.role` is `"teacher"` or `"student"`, write it. Then `redirect()` to either `/student?invite=<code>` or `/`.

### Task 5.3: Tests

- Vitest integration test for the post-oauth route logic: pre-existing user with no intent + `role=student` query → user updated. Pre-existing user with intent already set → no-op. Invalid role → no-op.
- Manual verification gate: actual Google OAuth flow requires real credentials and is out of automated test scope.

---

## Phase 6: P2 Cleanup

Bundled small fixes from across the three reviews.

### Task 6.1: Create Unit scope flicker (teacher review P2)

**File:** `src/app/(portal)/teacher/units/new/page.tsx`.

The form starts with `scope='personal'` and switches to `org` after `/api/orgs` returns. Fast submit can create a unit in the wrong scope. Fix: keep the form in a loading/disabled state until orgs load.

### Task 6.2: Create-class UUID validation (teacher review P2)

**File:** `src/app/(portal)/teacher/courses/[id]/create-class/page.tsx`.

Add UUID validation to `[id]` per the existing `isValidUUID` helper. Mirror the parent course detail page's pattern.

### Task 6.3: Student problem route UUID validation (student review P2)

**File:** `src/app/(portal)/student/classes/[id]/problems/[problemId]/page.tsx`.

Validate `classId`, `problemId`, and (where present) `attemptId` before fetching. Map bad UUIDs to `notFound()`.

### Task 6.4: Platform-admin Settings trim (review 005 P2)

**Files:**
- Modify: `src/lib/portal/nav-config.ts` — drop `Settings → /admin/settings` from the `admin` block.
- Delete: `src/app/(portal)/admin/settings/page.tsx`.
- Modify: `tests/unit/nav-config.test.ts` — extend the existing nav-page parity test to also cover the `admin` portal (currently org_admin only).

### Task 6.5: Responsive E2E hardening (review 005 P2)

Bundled into Task 4.2.

---

## Implementation Order

| Phase | Tasks | Why |
|-------|-------|-----|
| 1 | 1.1 → 1.2 → 1.3 | **Urgent security.** Ship first; if it grows, extract to 043a and split. |
| 6 | 6.1 → 6.2 → 6.3 → 6.4 | Tiny isolated fixes; pair with the Phase 1 nav-config touch. |
| 2 | 2.1 → 2.2 → 2.3 | Ended-session split — independent. |
| 3 | 3.1 → 3.2 → 3.3 → 3.4 → 3.5 | Multi-org context — independent. |
| 4 | 4.1 → 4.2 | Container-query shape — pairs E2E with the layout change. |
| 5 | 5.1 → 5.2 → 5.3 | OAuth flow — independent. |

One PR. ~10-12 commits.

**Extraction gate after Phase 1:** if security fixes turned out non-trivial, the rest of 043 ships in a follow-up PR.

---

## Verification per Phase

- **Phase 1**: every test path passes; manual: as a non-member student, GET `/api/classes/<known-id>` returns 404; POST to `/api/sessions/<id>/join` for an unrelated session returns 403.
- **Phase 2**: ended sessions in the listing render text not links; `/teacher/sessions/<ended-id>` renders the "ended" notice.
- **Phase 3**: as a multi-org admin, the switcher appears, switching changes URL, every org page reflects the chosen org.
- **Phase 4**: at 1024×768 with the sidebar visible, the editor is usable (≥360px wide).
- **Phase 5**: Vitest passes; manual Google flow lands at the expected destination with intent persisted.
- **Phase 6**: malformed routes 404 instead of erroring; Create Unit shows correct default; admin Settings link is gone.
- **Whole plan**: full Vitest + Go suite + new E2E green.

---

## Out-of-Scope Acknowledgements

Per the three reviews + consequential follow-ups:

- **Plan 044**: Topic → Syllabus Area taxonomy refactor (teacher P1 + student P1 from older reviews).
- **Plan 045**: My Work navigability redesign (student review P2).
- **Real platform-admin Settings UI** — Phase 6 trims; the actual settings UI is a separate product cycle.
- **Multi-tenant cross-org leak audit** — out of scope; existing per-endpoint authorization holds after Phase 1's fixes.

---

## Codex Review of This Plan

_To be added after the plan is dispatched to Codex via `/codex:rescue`._
