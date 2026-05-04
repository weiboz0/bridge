# Plan 067 — Sectioned navigation (replace the role switcher)

## Status

- **Date:** 2026-05-03
- **Origin:** UI testing feedback — the role-switcher above the
  sidebar nav is awkward UX. A user with multiple roles
  (e.g., teacher + org_admin + admin) has to click "Switch role"
  to navigate between portal trees, even though their session
  carries permissions for all of them simultaneously.
- **Scope:** Sidebar shell only. No backend changes. No
  permissions changes. The portal-route segregation
  (`/admin/*`, `/teacher/*`, `/student/*`, `/parent/*`,
  `/org/*`) stays exactly as it is — the URL-space remains
  role-bucketed, only the navigation surface changes.
- **Predecessor context:** `src/components/portal/role-switcher.tsx`
  + `src/components/portal/portal-shell.tsx` +
  `src/lib/portal/nav-config.ts`. The current model: each
  portal has its own `PortalConfig`; the shell renders ONE
  config at a time and the role switcher routes to a different
  portal entirely.

## Problem

Today's role switcher behaves like a portal-level mode switch:

```
[Sidebar header]
[Switch role] [Teacher] [Admin] [Org]   ← active=Teacher (highlighted)
[Nav items for the active role only]
  Dashboard
  Units
  Problems
  Sessions
  Courses
  Classes
[Sidebar footer]
```

Limitations:
- A teacher who's also a platform admin must click "Admin" to
  see the admin nav, then click "Teacher" to come back. Even
  trivial actions (check user count → resume a class) need a
  round-trip.
- The active "role" carries through the URL — `/teacher/*`
  shows teacher nav; `/admin/*` shows admin nav. There's no
  unified place to see "everything I can do." The sidebar
  feels like five separate apps stitched together.
- Mental model overhead: users have to remember which role
  contains which feature. ("Did I edit my own classes via the
  Teacher portal or the Org Admin portal?")

The single role-switcher button strip also hides nav items
for non-active roles entirely — a user can't even SEE that
they have admin access without clicking the switcher.

## Out of scope

- Permissions / authorization changes. The route gating
  (`PortalShell` rejects users who don't have the role for the
  current portal path) stays unchanged. We're moving where the
  user STARTS their journey, not what they can do once there.
- Mobile bottom-nav redesign. The bottom nav today shows the
  first 4 items of the active role's nav list; sectioned nav
  doesn't fit a 4-icon strip well. Mobile keeps the existing
  pattern (active-role-derived) for now, but uses a "primary
  role" derivation rule (see Decisions §4) so the user gets
  the most-relevant 4 icons.
- The `bridge-impersonate` impersonation flow. Plan 065 phase 4
  already wired live identity; impersonation continues to
  work and shows up as a banner above the sidebar (existing
  pattern, unchanged).
- Cross-org context switching for `org_admin` users with multiple
  org memberships. The current role switcher's `orgId` carrying
  behavior gets preserved in the new design (see Decisions §3).

## Approach

Replace the role-switcher + single-role-nav with a **single
sidebar that shows ALL of the user's accessible portal sections
as collapsible/auto-expanded groups**. The current portal path
determines which group is auto-expanded; the others can be
manually expanded by clicking their header.

```
[Sidebar header]
─── Teacher ───────────  ← group header, collapsible
  Dashboard
  Units
  Problems          ← active item highlighted
  Sessions
  Courses
  Classes
─── Org Admin ────────  ← collapsed by default
─── Platform Admin ───  ← collapsed by default
[Sidebar footer]
```

A single user with three roles sees three groups; a single-role
user sees one group with no header (preserves the current look
for the common case). Within a group, the nav-item list is
unchanged (same labels, hrefs, icons from `nav-config.ts`).

### Why "sections" not "tabs" or "menus"

- **Sections (this plan)**: all roles' nav visible at once, one
  expanded at a time. Optimizes for users who frequently jump
  between roles.
- **Tabs across the top**: would force a portal-level mode like
  today's role switcher, just with a different UI affordance.
  No improvement.
- **Hover/popover menus on a single icon**: hides what's
  available. Bad for discoverability.

Sections preserve the current per-role nav grouping (each role
has its own list of nav items, curated for that audience) AND
make the full surface visible. The only true cost is vertical
space — for a user with all five roles, the sidebar would be
quite tall — addressed by collapsibility (only the active
group is expanded by default).

### Why no backend or routing changes

The existing portal trees (`/admin/*`, `/teacher/*`, etc.) are
the right URL-space partitioning. They map cleanly to a `<PortalShell>`
that knows which role the page renders for, which controls the
header / breadcrumb / theming. Sectioned nav doesn't change that —
it just makes ALL the destinations reachable from any portal
page's sidebar.

A sectioned nav rendered on `/teacher/dashboard` will still be
routed to `/admin/orgs` if the user clicks an Admin item — and
once that route loads, `<PortalShell>` re-renders for the new
role. The sidebar will now auto-expand the Admin group instead.

## Decisions to lock in

1. **Auto-expand the section that matches the current URL.**
   Use `usePathname()` + `useSearchParams()` to detect the
   active section. For non-org-scoped roles (`admin`, `teacher`,
   `student`, `parent`): match by `basePath` (`/admin`,
   `/teacher`, etc.). For `org_admin`: match by both
   `basePath === "/org"` AND `searchParams.get("orgId")` —
   without this, a user with two `org_admin` memberships
   couldn't distinguish which section is "active" since both
   share `basePath: "/org"` (`src/lib/portal/nav-config.ts:20`).
   The active key is therefore `${role}:${orgId ?? "none"}`,
   matching the same shape as `RoleSwitcher::rowKey`
   (`src/components/portal/role-switcher.tsx:24-26`). Codex
   pass-1 caught this as a blocker.
2. **Persist user-toggled expansion state in localStorage.**
   If a user manually expands "Admin" while on a Teacher page,
   that stays expanded across renders within the same session.
   Reuse the JSON-parse-with-try/catch pattern from
   `src/lib/hooks/use-panel-layout.ts:24-29`. Storage key:
   `bridge.sidebar.expanded`. The map is keyed on the same
   `${role}:${orgId ?? "none"}` shape as the active-section
   detection so they line up. Sign-out clearing: Phase 1
   adds `localStorage.removeItem("bridge.sidebar.expanded")`
   to BOTH `src/components/portal/sidebar-footer.tsx:12-18`
   AND `src/components/sign-out-button.tsx:6-15`. Codex pass-1
   caught the unwired clear; pass-3 caught the original
   "Phase 3 adds" wording (which would have left a window
   between PR 1 and PR 3 where one signout path cleared the
   key and the other didn't); both clears now ship in Phase 1.
3. **Org context preservation.** The current role switcher
   carries `?orgId=` for org-scoped roles. The new sectioned
   nav links keep the same `?orgId=` query param on each item
   destination. For users with multi-org `org_admin`, render
   one section per org membership: "Org Admin · School A",
   "Org Admin · School B".
4. **Mobile: derive a "primary role" from `getPortalConfig`'s
   role priority.** Same as today, but explicit. Plan 040
   defines the priority `admin > org_admin > teacher > student >
   parent`; pick the highest-priority role the user has and
   show its top 4 nav items in the bottom nav. Sectioned nav
   is a desktop-only refinement.
5. **Drop `<RoleSwitcher />` entirely.** No back-compat shim —
   it was the symptom we're treating, not a useful primitive.
6. **`PortalShell` keeps its existing `portalRole` prop** (no
   change). Codex pass-1 non-blocking #2 caught a naming-drift
   risk: the plan originally referenced "currentRole" in both
   contexts, but `PortalShell` actually uses `portalRole`
   (`src/components/portal/portal-shell.tsx:13-14`) and only
   `Sidebar` uses `currentRole` (`src/components/portal/sidebar.tsx:12, 16`).
   Implementers must NOT rename `portalRole` to `currentRole`
   — keep the existing names. The shell still receives the
   role for theming / breadcrumb context; the sidebar gets
   ALL roles directly from `/api/me/portal-access`.
7. **No "all roles" landing page.** When a user signs in, they
   land on their primary role's dashboard (today's behavior,
   unchanged). The sectioned sidebar simply gives them
   one-click access to the others.

## Files

### Phase 1 — Sectioned sidebar component

**Add:**
- `src/components/portal/sidebar-section.tsx` — collapsible
  group header + nav items for one role. Props:
  `{ role, label, navItems, collapsed (sidebar-collapsed),
    expanded (group-expanded), onToggle }`. Renders the role
  label as the group header (with a chevron toggle) and the
  nav items below when expanded. When the sidebar itself is
  collapsed (icon-only), groups become a vertical row of icons
  with a tooltip on hover (today's collapsed-sidebar pattern).

**Modify:**
- `src/components/portal/sidebar.tsx`:
  - Drop `<RoleSwitcher />`.
  - For each role in `roles` (the `UserRole[]` already passed in), build that role's `PortalConfig` via `getPortalConfig`, then render a `<SidebarSection>` per role.
  - **Active section determination** uses `usePathname()` AND `useSearchParams().get("orgId")` to compute the composite key `${role}:${orgId ?? "none"}` per Decisions §1. Codex pass-2 caught the original stub here (basePath-only) contradicting Decisions §1; the two now match. The matching rule:
    - For non-org-scoped roles (`admin`, `teacher`, `student`, `parent`): match by `basePath` only.
    - For `org_admin`: match by `basePath === "/org"` AND the role's `orgId` equals `searchParams.get("orgId")`. If the URL has no `orgId` and the user has multiple `org_admin` memberships, no `org_admin` section is auto-expanded (the user is presumed to be on a non-org-specific page).
  - Persist non-active group expansion state in localStorage keyed on `bridge.sidebar.expanded` — same composite-key shape (`${role}:${orgId ?? "none"}`) so localStorage and the active-section logic line up.
  - The Phase 1 PR wires `localStorage.removeItem("bridge.sidebar.expanded")` into BOTH sign-out paths — `src/components/portal/sidebar-footer.tsx` AND `src/components/sign-out-button.tsx`. Codex pass-3 flagged that wiring only one path in PR 1 leaves a window where some sign-out flows clear the key and others don't. Both clears are 1-line additions; ship them together.
- `src/components/portal/sidebar-nav.tsx`: rename to
  `sidebar-section-items.tsx` OR keep as-is and have
  `sidebar-section.tsx` reuse it. The latter is simpler — no
  rename.

**Delete (Phase 2 if rollout looks clean, or keep as-is for
back-compat — see Decisions §5):**
- `src/components/portal/role-switcher.tsx` and any imports.

### Phase 2 — Portal config grouping

**Modify:**
- `src/lib/portal/nav-config.ts`: no change required. Each
  role's `PortalConfig.navItems` continues to be the canonical
  list for that role's section.

**Note:** the org-context query param injection (today done
inside `role-switcher.tsx::destinationFor`) moves into
`sidebar-section.tsx` so each item's `href` carries `?orgId=`
when the role is org-scoped. The link the section renders
becomes `${item.href}?orgId=${role.orgId}` for org_admin items,
unchanged for non-org-scoped roles.

### Phase 3 — Mobile bottom-nav consistency

**Modify:**
- `src/components/portal/sidebar.tsx`'s mobile-bottom-nav
  block: instead of `navItems.slice(0, 4)` from the active
  role, derive the "primary role" via the role-priority list
  and use ITS first 4 nav items. Document the rule in a
  comment. (This actually fixes a real-today issue: a
  multi-role user on a `/admin/*` page sees admin nav at
  bottom; on a `/teacher/*` page sees teacher nav at bottom.
  Sectioned-nav design unifies on the primary role for
  consistency.)

### Phase 4 — Tests

**Modify:**
- `tests/unit/role-switcher.test.tsx` (the only existing test —
  Codex pass-1 confirmed there's no e2e referencing
  RoleSwitcher). Codex pass-1 non-blocking #3: do NOT just
  delete the file. Carry over the orgId-related assertions:
  - Composite key on `(role, orgId)` for users with the same
    role in two different orgs (`tests/unit/role-switcher.test.tsx:29, 34-35, 42`).
  - orgId-encoded destination URL for org-scoped roles
    (`tests/unit/role-switcher.test.tsx:46, 60`).
  Rewrite as `tests/unit/sidebar-section.test.tsx`.
- Add `<SidebarSection>` unit test cases covering: collapsed/
  expanded toggle, active-item highlight, persist-to-localStorage
  (mock storage), org-id query-param injection on item links.
- Add an e2e smoke (`e2e/sectioned-nav.spec.ts`) — sign in as
  the platform admin (`admin@e2e.test` per `e2e/helpers.ts`),
  verify multiple sections render, click into Admin section,
  verify nav.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Vertical space exhaustion for users with many roles | low | Auto-collapse all but the active group; localStorage persists user toggles. |
| Org-context query param gets dropped on a sub-link | medium | Centralize the `?orgId=` injection in `sidebar-section.tsx` so every link from that section carries it. Add a unit test. |
| Existing tests / e2e specs reference role-switcher | low | Audit + rewrite. Phase 4 includes the search. |
| Users surprised that role switcher disappeared | low | The new design is strictly more discoverable — they see ALL their roles at once. No retraining needed. Add a one-line note in the next CHANGELOG / release notes if applicable. |
| Mobile bottom-nav inconsistency between portals | low | Phase 3 fixes this by deriving from primary role uniformly. |
| LocalStorage write failures (private mode, quota) | low | Wrap in try/catch; treat as "use defaults" (active expanded, others collapsed). |

## Phases

### Phase 0: Pre-impl Codex review

Per CLAUDE.md plan-review gate. Dispatch `codex:codex-rescue`
to review against:
- `src/components/portal/sidebar.tsx`
- `src/components/portal/role-switcher.tsx`
- `src/components/portal/portal-shell.tsx`
- `src/lib/portal/nav-config.ts`
- Any tests/e2e referencing `RoleSwitcher`

Specific questions:
1. Does the `PortalShell.hasRole` check still make sense once
   the sidebar exposes nav items for all roles? Specifically:
   if a user navigates to `/admin/*` without the `admin` role,
   they should still get redirected (defense in depth) — but
   the sectioned sidebar would simply not show that section
   in the first place. Both layers should hold.
2. Is there an existing convention for localStorage-persisted
   client state in Bridge (e.g., the sidebar collapsed state)?
   Should I reuse it for section-expansion state?
3. Are there server components that depend on `RoleSwitcher`'s
   removal from the import graph? (Unlikely — it's a client
   component.)
4. Any user-facing text in the role-switcher (e.g.,
   "Switch role") that lives elsewhere and would orphan?
5. The mobile bottom-nav primary-role derivation — is the
   priority order (`admin > org_admin > teacher > student >
   parent`) documented anywhere as canonical? If not, this
   plan should establish it.
6. Is there any flow that relies on the role-switcher being a
   `useRouter().push()` (vs a `<Link>`)? E.g., does anything
   listen for the route change to clear in-memory state?

### Phase 1: Sectioned sidebar component (PR 1)

- Implement `<SidebarSection>`.
- Refactor `<Sidebar>` to render one section per role.
- Drop `<RoleSwitcher>` import.
- Smoke-test the multi-role admin user (`m2chrischou@gmail.com`)
  in dev — confirm all sections render with the right items.
- Codex post-impl review.
- PR + merge.

### Phase 2: Mobile bottom-nav primary-role unification (PR 2)

- Update mobile bottom-nav to derive from primary role
  consistently across all portals.
- Smoke-test on a phone-sized viewport.
- Codex post-impl review.
- PR + merge.

### Phase 3: Tests + cleanup (PR 3)

- Rewrite `tests/unit/role-switcher.test.tsx` →
  `tests/unit/sidebar-section.test.tsx` carrying over the
  orgId composite-key + orgId-URL assertions.
- Add additional `<SidebarSection>` unit test cases per Phase 4.
- Add e2e smoke for the sectioned nav.
- Delete `src/components/portal/role-switcher.tsx`.
- (LocalStorage `removeItem` for both sign-out paths landed in
  Phase 1 per Codex pass-3 — not in this phase.)
- Codex post-impl review.
- PR + merge.

## Code Review

### Phases 2+3 post-impl — 2026-05-04: 2 BLOCKERS fixed

Phase 2's substantive primary-role mobile-nav unification was actually merged inside Phase 1 PR #110 — the mobile-bottom-nav block already derives from `getPrimaryRole(roles)` since 2026-05-04. Phases 2+3 ship as a single bundled PR (`feat/067-phases-2-3-mobile-nav-tests-cleanup`) covering the residual polish + cleanup:

- Active-state highlight on the mobile bottom-nav
- E2E smoke (`e2e/sectioned-nav.spec.ts`)
- Delete the orphan `role-switcher.tsx` + its now-redundant unit test (assertions live in `tests/unit/sidebar-section.test.tsx` from Phase 1)

Codex post-impl review verdict: 2 BLOCKERS (both fixed inline). Plus a plan-compliance note: no fresh phone-viewport smoke was run in this PR — Phase 2's mobile work was verified in Phase 1's stack.

BLOCKER 1 (FIXED): Active-match prefix collision. The naive `pathname.startsWith(itemPath + "/")` rule was already in `sidebar-section.tsx` (Phase 1) and I propagated it to the mobile bottom-nav. On `/teacher/units` it would highlight BOTH "Dashboard" (`/teacher`) and "Units" (`/teacher/units`). Fix: extracted `findActiveIndex` in `src/lib/portal/active-match.ts` — longest-match wins. Both desktop sidebar and mobile bottom-nav now use the helper. Regression locked with `tests/unit/active-match.test.ts` (6 cases including the prefix-collision and the partial-segment guard `/admin` vs `/admins`).

BLOCKER 2 (FIXED): The e2e originally asserted `count() > 1` SidebarSection headers for `admin@e2e.test`, but `e2e/helpers.ts` documents that account as `is_platform_admin=true` only with no org memberships. Multi-section is unit-test territory (`sidebar-section.test.tsx` covers it with controlled seed data); the e2e's substantive contract is "no role-switcher button" + "sidebar renders + clickable", which is what it now asserts.

## Codex Review of This Plan

### Pass 4 — 2026-05-04: line 158 sign-out clear narrative aligned

Codex pass-4 caught a remaining "Phase 3 adds" reference in Decisions §2 narrative (line 158) that contradicted the pass-3 fold-in (which moved both clears to Phase 1). Updated to "Phase 1 adds" with the historical context preserved.

### Pass 3 — 2026-05-03: CONCUR-WITH-CHANGES → localStorage convergence

Codex pass-3 confirmed the stub contradiction is fully resolved. One remaining concern: the `sign-out-button.tsx` localStorage clear was scheduled for Phase 3 but the `sidebar-footer.tsx` clear was already in Phase 1, creating a window between PR 1 and PR 3 where one sign-out flow clears the key and the other doesn't. Folded: both clears now ship in Phase 1 (1-line additions to both files).

### Pass 2 — 2026-05-03: BLOCKED → 1 stub-contradiction folded in

Codex pass-2 confirmed pass-1 fixes are clean (orgId composite key in Decisions §1, localStorage clear wiring, prop naming guidance, test rewrite plan). One BLOCKER: Phase 1 file stub for `sidebar.tsx` still said "basePath only" for active-section determination, contradicting Decisions §1's composite-key rule. Folded: stub now mirrors §1's rule explicitly with the matching-rule sub-bullets for non-org-scoped vs `org_admin` roles. Phase 1 also pulls forward the `sidebar-footer.tsx` localStorage clear so there's no persistence-without-cleanup window between PR 1 and PR 3 (Codex pass-2 non-blocking concern).

### Pass 1 — 2026-05-03: BLOCKED → 1 blocker + 3 non-blocking folded in

Codex returned BLOCKED with one blocker plus three important
non-blocking concerns. All folded:

1. **Multi-org `org_admin` section activation under-specified**
   (BLOCKER) — every `org_admin` section shares
   `basePath: "/org"`, so `usePathname()` alone can't tell
   them apart for the auto-expand logic. Resolution: active
   key is `${role}:${orgId ?? "none"}` and uses
   `useSearchParams().get("orgId")` for org-scoped roles.
   Decisions §1 updated with the explicit detection rule.

2. **Sign-out localStorage clear unwired** (non-blocking #1)
   — the plan promised it but neither sign-out path actually
   removes the storage key. Phase 3 now explicitly adds
   `localStorage.removeItem` to both `sidebar-footer.tsx` and
   `sign-out-button.tsx`.

3. **Prop naming drift risk** (non-blocking #2) — `PortalShell`
   uses `portalRole`, `Sidebar` uses `currentRole`. Decisions
   §6 calls this out; implementers must NOT rename either.

4. **RoleSwitcher tests should be rewritten, not deleted**
   (non-blocking #3) — the orgId-related assertions are
   load-bearing. Phase 4 (tests) now specifies a
   rewrite-to-`sidebar-section.test.tsx` flow that carries
   over the composite-key + orgId-URL assertions.

Confirmed by Codex (no resolution needed):
- Single-source canonical role priority lives in
  `src/lib/portal/roles.ts:3,50-55`.
- `PortalShell.hasRole` gating remains correct as defense in
  depth against deep-linking to a portal a user lacks.
- `useSidebar` localStorage pattern is established (`bridge-sidebar-collapsed`).
- No e2e currently references RoleSwitcher (only unit test).
- No server components import RoleSwitcher directly.

Verdict: **BLOCKED → CHANGES FOLDED → ready for Phase 1**
pending one more Codex pass to confirm convergence.
