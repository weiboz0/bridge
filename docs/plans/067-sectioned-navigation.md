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
   Use `usePathname()` to detect the active basePath and
   default to that group expanded; the others collapsed.
2. **Persist user-toggled expansion state in localStorage.**
   If a user manually expands "Admin" while on a Teacher page,
   that stays expanded across renders within the same session.
   Cleared when the user signs out (alongside other client
   state).
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
6. **`PortalShell` props simplify to `currentRole` only** (no
   change). The shell still receives the role for theming /
   breadcrumb context; the sidebar gets ALL roles directly
   from `/api/me/portal-access` (the same payload it already
   consumes).
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
  - For each role in `roles` (the `UserRole[]` already passed
    in), build that role's `PortalConfig` via `getPortalConfig`,
    then render a `<SidebarSection>` per role.
  - Active section determination via `usePathname()` — match
    against `basePath` (`/teacher`, `/admin`, etc.).
  - Persist non-active group expansion state in localStorage
    keyed on `bridge.sidebar.expanded` (a `Record<string, boolean>`
    keyed by `${role}:${orgId ?? "none"}`).
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
- Any existing tests referencing `<RoleSwitcher>` — delete
  those test cases (or rewrite to assert sidebar sections).
  Audit:
  ```bash
  grep -rn "RoleSwitcher\|role-switcher" tests/ e2e/
  ```
- Add a unit test for `<SidebarSection>` covering: collapsed/
  expanded toggle, active-item highlight, persist-to-localStorage,
  org-id query-param injection.
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

- Audit and update any tests referencing RoleSwitcher.
- Add `<SidebarSection>` unit tests.
- Add e2e smoke for the sectioned nav.
- Delete `src/components/portal/role-switcher.tsx`.
- Codex post-impl review.
- PR + merge.

## Codex Review of This Plan

_(To be populated by Codex pass — see Phase 0.)_
