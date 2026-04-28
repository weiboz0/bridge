# 043 — Review 003-005 Fixes: Security, Multi-Org Context, Editor Squeeze, OAuth, P2 Cleanup

**Goal:** Address findings across three pending reviews — the older teacher portal review (`003-teacher-portal-review-2026-04-26.md`), older student portal review (`004-student-portal-review-2026-04-26.md`), and the fresh comprehensive review (`005-comprehensive-site-review-2026-04-27.md`). Two of the older review findings are P0 security gaps that survived plans 039-042; ship those first. The rest is a tight cleanup pass on the remaining P1 and P2 items that aren't taxonomy-scoped.

**Sources:**
- `docs/reviews/003-teacher-portal-review-2026-04-26.md`
- `docs/reviews/004-student-portal-review-2026-04-26.md`
- `docs/reviews/005-comprehensive-site-review-2026-04-27.md`

**Branch:** `feat/043-review-003-fixes`

**Status:** Complete (pending PR review)

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

### Task 1.2: `JoinSession` requires class membership OR a valid pre-invitation

**Files:**
- Modify: `platform/internal/handlers/sessions.go::JoinSession` (lines 296-319). Before adding the participant, verify access:
  - Allow admin equivalence as above.
  - Allow if the caller has class membership for the session's owning class.
  - Allow if the caller has a `session_participants` row with status `invited` or `present` (pre-invited by the teacher via `AddParticipant`). **Status `left` does NOT grant re-entry** — a kicked or left student must be re-invited (Codex review correction #2).
  - Reject with 403 otherwise.
- The token-based join path (`JoinSessionByToken`) already validates the token — leave that path unchanged.

### Task 1.2b: `GetStudentPage` must also honor pre-invitations

**File:** `platform/internal/handlers/sessions.go::GetStudentPage` (lines 1129-1149 currently).

Codex review correction #2 (second part): the student session page calls `/student-page` BEFORE running `/join`. Today `GetStudentPage` only authorizes `teacher | admin | impersonating | class member`. A pre-invited non-class-member (e.g., a parent or a guest) gets a 403 from `/student-page` and never reaches the join POST. Add the same `invited|present` participant gate to `GetStudentPage` so pre-invited users can actually load the page.

### Task 1.3: Class-adjacent session endpoints must also gate on membership

Codex review correction #7 (CRITICAL — adjacent scope gap): `ListByClass`, `GetActiveByClass`, and `GetSessionTopics` (`platform/internal/handlers/sessions.go:189-221, 528-541`) check that claims are present but not that the caller is enrolled in the class. A direct API caller can enumerate sessions / read topic lists for any class by ID. Fix in the same phase:

**Files:**
- Modify: `platform/internal/handlers/sessions.go::ListByClass` — verify caller is admin equivalent OR has class membership for the path's `classId`. Reject with 404 (consistent with `GetClass`).
- Modify: `platform/internal/handlers/sessions.go::GetActiveByClass` — same gate.
- Modify: `platform/internal/handlers/sessions.go::GetSessionTopics` — same gate, but resolve the class via `session.ClassID` first since the path is `/sessions/{id}/topics`.

Reuse the helper added in Task 1.1 (`canAccessClass(ctx, classID, claims)`).

### Task 1.4: Tests for all touched endpoints

**Files:**
- Modify: existing `platform/internal/handlers/classes_test.go` and `sessions_test.go` (or create integration tests if they don't exist there). Cover for each touched endpoint (GetClass, JoinSession, GetStudentPage, ListByClass, GetActiveByClass, GetSessionTopics):
  - Member can read / join (happy path).
  - Non-member outside the org → 404 / 403 respectively.
  - Org admin → allowed.
  - Platform admin → allowed.
  - Admin impersonating a non-member → allowed (admin equivalence).
  - Pre-invited participant (`invited` status) → JoinSession + GetStudentPage allowed.
  - Participant with `left` status → JoinSession rejected (must be re-invited).
  - Random user with the session/class ID → 403.

### Task 1.5: Verify join-API response shape

Codex review correction #8 (IMPORTANT): `JoinClassDialog` reads `result.class.id`. Confirm `/api/classes/join` returns `{ class: { id, ... }, ... }` after Phase 1's GetClass change doesn't accidentally alter the join handler's response. Smoke test: render the dialog with a mocked join response missing `class.id` and assert the dialog's "didn't return a class" branch fires (already covered by `tests/unit/join-class-dialog.test.tsx` per plan 040 phase 4 — re-run to confirm).

### Phase 1 extraction gate

If Phase 1 grows beyond `platform/internal/handlers/`, `platform/internal/store/`, and the corresponding `_test.go` files — for example, if it requires schema changes to `session_participants` to add a `pending_invitation` table, or surfaces another unprotected endpoint that needs a separate plan — extract Phase 1 to a fast-shipping plan 043a and ship the rest of 043 in a follow-up PR.

---

## Phase 2: Ended Sessions Read-Only State

Per teacher review P1: every session in `/teacher/sessions` links to `/teacher/sessions/{id}` which renders the live `TeacherDashboard` regardless of status. Ended sessions need either a distinct review surface or no link.

### Task 2.1: Don't link ended sessions in the listing

**File:** `src/app/(portal)/teacher/sessions/page.tsx` (lines 82-86).

For ended sessions, render a non-link row (just text + status badge) instead of a `<Link>`. Keeps the listing honest without inventing a new surface. A "View summary" affordance is queued for a future product cycle when ended-session review UI is designed.

### Task 2.2: Server-side guard on the teacher page

**File:** `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx`.

Read the session status from the page-payload endpoint. If `status !== "live"`, render a "Session ended" notice with a back link to `/teacher/sessions` instead of the workspace. Guards against bookmarks and direct URL access, not just the listing-page link.

**Note (Codex correction #3):** the student route already handles this correctly — `GetStudentPage` returns 404 for non-live sessions and the server component maps the ApiError to `notFound()` (`platform/internal/handlers/sessions.go:1129-1131`, `src/app/(portal)/student/sessions/[sessionId]/page.tsx:23-30`). Only the teacher route needs the explicit notice.

### Task 2.3: Tests

- Vitest test for a small `<SessionRow>` (extract if helpful) asserting ended sessions render text rather than a link.
- E2E spec verifying that opening an ended session URL directly renders the "Session ended" notice, not the live workspace.

---

## Phase 3: Multi-Org Context (review 005 P1)

The fix has two parts: backend pages that pass an explicit `orgId`, and a UI selector for users with multiple `org_admin` memberships.

### Task 3.1: Org context helper + URL persistence

**Files:**
- Create: `src/lib/portal/org-context.ts` — pure helpers `parseOrgIdFromSearchParams(sp)` and `appendOrgId(path, orgId)`.
- Create: `src/components/portal/org-switcher.tsx` — client component using the existing **`Select`** primitive at `src/components/ui/select.tsx` (Codex correction #4 — reuse, don't rebuild). Lists the user's active `org_admin` memberships; pushes `?orgId=<new>` to the current pathname; preserves other query params; keys options by `orgId`.

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

Wrap the shell's outer flex in a `@container/shell`. Replace `lg:flex-row`, `lg:hidden`, `lg:flex`, `lg:w-[…]`, etc. with the container-query equivalents.

**Breakpoint correction (Codex review #5):** the original draft used `@5xl/shell` (1024px container width). In the sidebar-squeezed scenario the container starts at ~800px (1024 viewport − 224 sidebar) and never reaches 1024px, so wide layout would never trigger. Use **`@4xl/shell` (896px)** or **`@3xl/shell` (768px)** as the wide breakpoint instead — pick during implementation based on what produces a usable code pane. The min-w floors (360 + 320 = 680) plus a 200px editor minimum suggests `@3xl`/768px is the right floor; the editor gets ~88px above the floor at 768px container, which is tight but real.

Tailwind v4's native `@container` queries are confirmed available (`package.json` has `"tailwindcss": "^4.x"`). No fallback needed.

### Task 4.2: Update the responsive E2E

**File:** `e2e/problem-editor-responsive.spec.ts`.

- Add a deterministic problem to the spec setup (creates a class + problem if none exists) so the test never silently skips.
- At 1024×768 with the desktop sidebar visible, assert: code pane bounding box ≥ 360px wide.
- At 1280×800, assert wide layout AND code pane ≥ 360px wide.

---

## Phase 5: Google OAuth Invite + Role Carry-Through (review 005 P1)

**Approach revision (Codex correction #6):** the original draft created a `/post-oauth` redirect page. Cleaner: extend the existing Auth.js `signIn` callback (`src/lib/auth.ts:90-131`) which already creates the OAuth user. The challenge — the callback runs server-side without `searchParams` access — is solved with a short-lived signed cookie set by the register page before the OAuth redirect, then read in the callback when the user is created.

### Task 5.1: Set a signup-intent cookie before Google redirect

**File:** `src/app/(auth)/register/page.tsx`.

Before calling `signIn("google", ...)`, set an httpOnly cookie `bridge-signup-intent` with `{ role, inviteCode? }` JSON via a small server action (or a fetch to `POST /api/auth/signup-intent`). Sets `Max-Age=300` (5 min — long enough for the OAuth round-trip, short enough that stale state doesn't pollute the next signup). The Google `callbackUrl` is set to `/student?invite=<code>` when invite is present, else `/`.

**File:** `src/app/api/auth/signup-intent/route.ts` (new).

POST handler that accepts `{ role: "teacher" | "student", inviteCode?: string }` and writes the cookie. Reject any other shape.

### Task 5.2: Read the cookie in the `signIn` callback

**File:** `src/lib/auth.ts` (lines 90-131 — the existing Google `signIn` callback that creates the OAuth user).

When creating a new OAuth user, read `bridge-signup-intent` from `cookies()`, parse it, and pass `intendedRole` to the user insert. Clear the cookie afterward (set Max-Age=0 in the response). Existing users (re-signing in) ignore the cookie — no overwrite.

The `intendedRole` column already exists in `src/lib/db/schema.ts:129-139` (added in plan 040 migration 0021).

### Task 5.3: Tests

- Vitest integration test for `POST /api/auth/signup-intent`: valid body sets the cookie; invalid body rejected.
- Vitest test for the signIn-callback path: mock cookie read returning `{role:"student",inviteCode:"ABC"}`, create OAuth user, assert the user row has `intendedRole=student`. Existing user (lookup hits) doesn't get overwritten.
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
| 1 | 1.1 → 1.2 → 1.2b → 1.3 → 1.4 → 1.5 | **Urgent security.** Ship first. Extraction gate documented in Task 1.5. |
| 5 | 5.1 → 5.2 → 5.3 | OAuth — small, independent, security-adjacent (Codex correction #7 — moved earlier). |
| 6 | 6.1 → 6.2 → 6.3 → 6.4 | Tiny isolated fixes; pair with the Phase 1 nav-config touch. |
| 2 | 2.1 → 2.2 → 2.3 | Ended-session split — independent. |
| 3 | 3.1 → 3.2 → 3.3 → 3.4 → 3.5 | Multi-org context — independent. |
| 4 | 4.1 → 4.2 | Container-query shape — pairs E2E with the layout change. |

One PR. ~12-14 commits.

**Extraction gate after Phase 1:** see Task 1.5 — concrete threshold defined.

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

- **Date:** 2026-04-27
- **Reviewer:** Codex (pre-implementation, via `codex:rescue`)
- **Verdict:** Two `[CRITICAL]` + four `[IMPORTANT]` corrections applied.

### Corrections applied

1. `[CRITICAL]` **Class-adjacent endpoints unprotected.** `ListByClass`, `GetActiveByClass`, `GetSessionTopics` only check claims, not class membership — direct API callers can enumerate sessions/topics by class ID. → New Task 1.3 covers all three with the same `canAccessClass` gate as Phase 1.1.
2. `[CRITICAL]` **`session_participants` status filter under-specified.** Original draft said "already in session_participants" — but `left` participants must NOT get re-entry. Pre-invited non-class-members also can't load `/student-page` because it doesn't honor `invited` status. → Task 1.2 tightened to `invited|present` only; new Task 1.2b extends `GetStudentPage` with the same gate.
3. `[IMPORTANT]` **Phase 4 container breakpoint wrong.** Original `@5xl/shell` (1024px container) never triggers in the sidebar-squeezed scenario where the container starts at ~800px. → Task 4.1 now specifies `@3xl/shell` (768px) or `@4xl/shell` (896px), picked at implementation time based on usable code-pane width.
4. `[IMPORTANT]` **Phase 5 OAuth shape.** Original `/post-oauth` redirect page was clunky — Auth.js already has a `signIn` callback that creates OAuth users. → Task 5 reshaped: set a short-lived signed cookie before Google redirect, read it in the existing `signIn` callback, write `intendedRole` directly. No new redirect page.
5. `[IMPORTANT]` **OrgSwitcher should reuse the existing `Select` primitive** at `src/components/ui/select.tsx`. → Task 3.1 updated to use it.
6. `[IMPORTANT]` **Implementation order: Phase 5 should come right after Phase 1.** Small, independent, security-adjacent. → Order updated.
7. `[NOTE]` **Task 1.5 added** to verify `/api/classes/join` response shape doesn't break the dialog UX after `GetClass` changes.
8. `[NOTE]` **Phase 1 extraction gate threshold made concrete:** split if Phase 1 grows beyond `platform/internal/handlers/`, `platform/internal/store/`, and corresponding `_test.go` files.

### Codex notes (no plan change)

- `GetClass` returning 404 (vs. `GetCourse`'s 403) is consistent with `GetSession`. The inconsistency with `GetCourse` is documented but not fixed in this plan — it's existing tech debt across handlers.
- Student session route already handles ended sessions correctly (`GetStudentPage` returns 404, server component maps to `notFound()`). Phase 2.2 only needs the teacher-side fix.
- `searchParams: Promise<{ orgId?: string }>` is the correct Next.js 16 App Router pattern.
- Role-switcher composite key `${role}:${orgId}` is safe — schema guarantees `orgId` is non-null for org-scoped roles.
- Tailwind v4 native `@container` queries confirmed available in this project.

## Post-Execution Report

**Status:** Complete. All 6 phases shipped on `feat/043-review-003-fixes`.

**Phase 1 — P0 security gates** (commit `bb5d8c8`)
- `ClassHandler.CanAccessClass`: helper resolving class + verifying admin / class-membership / org-admin. Returns 404 for both not-found and not-authorized — no class-existence leak.
- `SessionHandler.canJoinSession`: same shape for session joins. Pre-invited rows with `invited|present` status grant access; `left` does NOT (kicked/left students must be re-invited).
- `GetStudentPage` (Task 1.2b): same pre-invitation gate so a teacher-invited non-class-member can load `/student-page` before the join POST.
- `SessionHandler.canAccessClass`: gate for the 3 class-adjacent endpoints (`ListByClass`, `GetActiveByClass`, `GetSessionTopics`) — Codex pre-impl correction #1 caught these were unprotected.
- New `GetSessionParticipant` store helper for the single-row lookup.
- 18 new Go integration tests covering happy/admin/impersonating/outsider/non-existent paths plus the `left` re-entry rejection and pre-invited `GetStudentPage` access.

**Phase 5 — Google OAuth intent carry** (commit `8831eb8`)
- `POST /api/auth/signup-intent`: writes a short-lived (5 min, HttpOnly, SameSite=Lax) `bridge-signup-intent` cookie with `{ role, inviteCode? }`. Zod-validated.
- `signIn` callback in `src/lib/auth.ts`: reads + clears the cookie when creating a brand-new OAuth user. Existing users re-signing in are not touched.
- `register/page.tsx`: Google button now POSTs the intent BEFORE redirecting to Google, and carries invite into `callbackUrl`.
- 5 new Vitest cases for the signup-intent route.

**Phase 6 — P2 cleanup** (commit `687f2d8`)
- Hold submit on `/teacher/units/new` until orgs load (kills the scope=personal → scope=org flicker).
- UUID validation on `/teacher/courses/[id]/create-class`, `/student/.../problems/[problemId]`, and the deeper `attempts/[attemptId]` route.
- Drop platform-admin Settings entry from nav + delete the placeholder page.
- Extend nav-page parity test to cover both `org_admin` and `admin` portals.

**Phase 2 — Ended sessions** (commit `7e34eb2`)
- `<SessionRow>` extracted from `/teacher/sessions/page.tsx`. Live → `<Link>` to workspace. Ended → plain `<div>` with the same metadata + status badge.
- `/teacher/sessions/[sessionId]/page.tsx`: server-side status check; non-live renders a "Session ended" notice with a back link.
- 2 new Vitest cases on the SessionRow.

**Phase 3 — Multi-org context** (commit `79538eb`)
- `src/lib/portal/org-context.ts`: `parseOrgIdFromSearchParams` + `appendOrgId` (UUID-validated).
- `src/components/portal/org-switcher.tsx`: native `<select>` reading `?orgId=` directly. Hidden when fewer than 2 options.
- `src/app/(portal)/org/layout.tsx`: server-side fetches active org_admin memberships, renders the switcher above content.
- All 6 org pages thread `orgId` through to the API call via `appendOrgId`.
- `RoleSwitcher` (Task 3.4): composite key `role:orgId` (no React-key dup), `destinationFor()` carries `?orgId=<id>` for org-scoped roles, visual disambiguation when multiple roles of the same type.
- 15 new Vitest cases across org-context, org-switcher, role-switcher.

**Phase 4 — Container-aware editor breakpoint** (commit `26b5d0b`)
- `problem-shell.tsx` outer container is now `@container/shell`. All viewport-`lg:` rules became `@3xl/shell:` (768px container width). The breakpoint reacts to actual pane width — sidebar-squeeze bug from review 005 is closed.
- `responsive-tabs.tsx`: `lg:hidden` → `@3xl/shell:hidden` to follow the same breakpoint.
- E2E spec rewritten: 1440px wide / 1280×800 desktop / 1024×768 squeezed / 800px narrow. Asserts code-pane bounding box ≥ 360px in wide cases (catches the pre-043 ~120px squeeze that visibility-only checks missed).

**Verification**
- Vitest: 449 passed / 11 skipped (was 426 — 23 new across phases 2/3/5/6).
- Go tests: all 14 packages green against `bridge_test`. New tests: 18 in `security_phase1_integration_test.go` covering all touched endpoints + status-transition assertions.
- TypeScript: clean for new/modified files.
- E2E: spec rewritten; runtime requires the local stack.

**Plan compliance**
- Pre-impl Codex review's 2 CRITICAL + 4 IMPORTANT corrections all applied.
- Post-impl Codex review found 1 CRITICAL + 1 IMPORTANT additional issues, both fixed in-PR (see Code Review section below).

**Out-of-scope (deferred to future plans)**
- Plan 044: Topic → Syllabus Area taxonomy refactor.
- Plan 045: My Work navigability redesign.
- Real platform-admin Settings UI.
- Multi-tenant cross-org leak audit.

## Code Review

### Review 1 — Pre-implementation plan review (commit `c940c40`)

- **Date:** 2026-04-27
- **Reviewer:** Codex (via `codex:rescue`)
- **Verdict:** 2 CRITICAL + 4 IMPORTANT corrections applied — see "Codex Review of This Plan" section above.

### Review 2 — Post-implementation review

- **Date:** 2026-04-27
- **Reviewer:** Codex (post-implementation, via `codex:rescue`)
- **Verdict:** 1 CRITICAL + 1 IMPORTANT both fixed in-PR. Several NOTE items confirmed correct without action.

**Fixed in commit applied after Review 2:**

1. `[CRITICAL]` **Pre-invited participants stuck at `invited` status after JoinSession.** `JoinSession` SQL used `ON CONFLICT DO NOTHING RETURNING`, so pre-invited rows (status=`invited`, joined_at=NULL) were never updated when the user actually joined. The teacher's roster never reflected the join. → SQL changed to `ON CONFLICT (session_id, user_id) DO UPDATE SET status='present', joined_at=COALESCE(...) WHERE status='invited' RETURNING ...`. Strengthened `TestJoinSession_PreInvited_Allowed` to assert status flip + joined_at population.
2. `[IMPORTANT]` **Signup-intent cookie not cleared after read.** `readSignupIntentRole` in `src/lib/auth.ts` parsed the cookie but never deleted it. A stale cookie could corrupt the next signup. → Now calls `cookieStore.delete(SIGNUP_INTENT_COOKIE)` before the parse, wrapped in try/catch since `cookies().delete()` can throw in render-only contexts where setting cookies isn't allowed.

**Codex notes (no action):**

- All Phase 1 P0 gates verified correct: outsider 404 from `CanAccessClass`/`canAccessClass`, `left` rows rejected by the join gate, `AddParticipant` correctly re-invites `left` rows in the store.
- `GetStudentPage` duplicates rather than reuses `canJoinSession` — logic matches but the helper extraction was rejected because `GetStudentPage` doesn't actually need the session-status check that `canJoinSession` runs first.
- P2 UUID validation parity confirmed: parent course detail validates, parent class detail validates, no inbound `/admin/settings` links remain.
- Card primitives all export correctly.
- `OrgSwitcher` server/client boundary works; `useSearchParams` is fine in client components rendered from server layout.
- Container query syntax correct for Tailwind v4.
