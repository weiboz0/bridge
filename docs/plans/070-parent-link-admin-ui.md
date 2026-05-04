# Plan 070 — Parent-link admin UI

## Status

- **Date:** 2026-05-03
- **Origin:** Comprehensive browser review 010 §P2 ("Parent onboarding copy promises teacher-driven linking, but only admin/API machinery currently exists"). Plan 064 shipped the `parent_links` schema + platform-admin CRUD endpoints (`/api/admin/parent-links`); no org-admin or teacher UI exists. Meanwhile `/onboarding` tells parent users "your child's teacher will link your account" — copy that doesn't match any shipped flow.
- **Scope:** Adds an org-admin-managed parent-link UI under `/org/parent-links` plus the supporting Go endpoint family `/api/orgs/{orgId}/parent-links`. Updates parent-onboarding copy to match. Does NOT add teacher-side write surface in v1 (deferred — see "Decisions §1" for product fork).
- **Predecessor context:** Plan 064 (`parent_links` table + `IsParentOf` gate + `/api/admin/parent-links` CRUD). Plan 053b (parent-viewer realtime authorization that depends on `parent_links`). Plan 069 (org-admin write surface — sister plan, parent-links are intentionally split out because the model question is non-trivial).

## Problem

Today's parent-link state:

| Layer | Status |
|---|---|
| Schema (`parent_links` table) | ✅ shipped (plan 064, drizzle/0024) |
| Backend authorization (`IsParentOf` gate) | ✅ shipped (used by `/api/parent/*` and the parent-viewer realtime token mint) |
| Platform-admin CRUD endpoints | ✅ shipped (`platform/internal/handlers/admin.go:46-49`) |
| Platform-admin UI (`/admin/parent-links`?) | ❌ doesn't exist |
| Org-admin CRUD endpoints | ❌ doesn't exist |
| Org-admin UI | ❌ doesn't exist |
| Teacher-facing UI | ❌ doesn't exist |
| Parent onboarding copy | ⚠️ misleading — promises teacher linking |

Concrete failures:
- The reviewer logged in as Diana Parent and saw "No children linked yet" (after the migration is applied). There's no shipped path to create that link.
- Even if a platform admin wanted to help, the only path is `curl -X POST` against `/api/admin/parent-links` — no UI.
- The onboarding copy promises a flow that doesn't exist.

## Out of scope

- Self-service parent-claim flow (parent enters their child's email + waits for the child / teacher to approve). Real product, real schema (claim tokens, expiry, single-use), real plan. Plan 070 is the operator-managed v1.
- Audit trail beyond `created_by` / `revoked_at` (already on the schema). A full event log is plan 070b material.
- Bulk parent-link import (CSV). Single-create form for v1.
- Multi-parent-per-child UI affordances. The schema supports many parents per child; the v1 UI just lists them — no special "primary parent" treatment.
- Email notifications when a link is created/revoked. Add when the org admin needs that signal; for v1 the parent just sees the child appear next time they sign in.
- Teacher-side write surface. **Decision §1 forks here.** v1 is org-admin-only; teacher-side is plan 070b if/when product confirms.

## Approach

Add an org-admin-managed flow as v1:

1. **New Go endpoint family** `/api/orgs/{orgId}/parent-links`: List, Create, Revoke. Gated by org_admin in `orgId`. The store layer (plan 064) is reusable; only the handler + auth check is new.

2. **New org-admin page** `/org/parent-links`: lists active links for students in the org, supports create + revoke. The create form takes parent email + child email (or child id from a picker); backend resolves both, validates both belong to the org's classes, and creates the link.

3. **Update parent-onboarding copy** (`src/app/(portal)/onboarding/page.tsx` or wherever the parent-flow text lives) to match: "Your school administrator will link your account to your child's. Reach out to your child's teacher or your school's admin if you don't see your child within 24 hours."

4. **Optional read-only teacher view**: `/teacher/classes/{id}/students/{studentId}` adds a "Parents" section listing the linked parents (with no actions). This gives teachers visibility without write access — they can see who to contact about a linkage problem.

### Why org-admin-only for v1

The reviewer noted: "Org admin-managed is safer for school data governance. Teacher-requested, org-admin-approved is a good next step if teachers know the family context."

For v1 we ship the safer path. The upgrade path to teacher-requested is non-breaking (add a `parent_link_requests` table, add teacher-side request UI, org-admin sees pending requests in the same `/org/parent-links` page). v2 doesn't invalidate v1.

### Why a new endpoint family instead of widening `/api/admin/parent-links`

The platform-admin endpoints are intentionally global — they can list every link in the system, link any parent to any child. Org admins should only see/manage links involving students in THEIR org's classes. Two different scope queries; cleanest as two different endpoints. (The store layer is shared.)

## Decisions to lock in

1. **v1 is org-admin-managed.** Teachers see (read-only) but don't write. Token-based parent-claim flow is plan 070b. Captured here so the product decision doesn't get re-litigated mid-implementation.
2. **Children scoped by org via class membership.** Authorization rule: `child_user_id` must be a member (any role) of at least one class belonging to a course in the calling org_admin's org. Same query shape as the existing `IsTeacherOfAttempt` from plan 053b — reuse the join logic.
3. **Parent doesn't need org membership.** Parents are typically NOT members of any org (their account is just a Bridge user). The org-admin can link any parent email to any child in their org. Trust model: school administrators know who the parents are.
4. **Idempotent re-link** (matches existing platform-admin behavior at `admin.go:307`). If an active link already exists for the (parent, child) pair, return 409 — UI maps to "Already linked, no action needed."
5. **Revoke is soft-delete.** The schema flips status to `revoked` + sets `revoked_at` (plan 064 §"Soft-revoke"). Re-creating later is allowed (partial-unique index).
6. **List filters: by parent email OR by child name OR by class.** Three independent filters; org admin picks whichever scope they're investigating.
7. **No bulk-revoke v1.** If the org admin needs to revoke many at once (e.g., end-of-year cleanup), they revoke one at a time. Bulk operations are rare and add UI complexity.
8. **Onboarding copy update is non-trivial wording, not just a substring rename.** The full sentence shifts from "your teacher will link" → "your school admin (or teacher acting on their behalf) will link". Phase 4 includes the copy review.

## Files

### Phase 1 — Backend `/api/orgs/{orgId}/parent-links` endpoints

**Add:**
- `platform/internal/handlers/org_parent_links.go` — handler holding `OrgParentLinksHandler` with three methods:
  - `ListByOrg(w, r)` — `GET /api/orgs/{orgId}/parent-links?status=active|revoked|all&parent={email?}&child={uid?}&class={uid?}`. Returns links scoped to children in the org. Filtering composable.
  - `CreateLink(w, r)` — `POST /api/orgs/{orgId}/parent-links` with body `{ parentEmail, childUserId }`. Resolves parent by email (404 if missing — UI surfaces "ask parent to register"), validates child is in the org, calls `parentLinks.CreateLink(ctx, parentId, childId, callerId)`. Returns 201 + the link, 409 if already active.
  - `RevokeLink(w, r)` — `DELETE /api/orgs/{orgId}/parent-links/{linkId}`. Validates the link's child belongs to the org; reuses `parentLinks.RevokeLink(ctx, linkId)`.
- `platform/internal/handlers/org_parent_links_test.go` — table-driven tests covering: list happy path, list filtered, create happy, create-with-unknown-parent (404), create-with-cross-org-child (403), revoke happy, revoke-cross-org-link (403), unauthorized (no org_admin role).

**Modify:**
- `platform/cmd/api/main.go` — wire the new handler under the existing org-routes block (must be inside the `RequireAuth` group; will inherit the live-admin claim from plan 065 phase 3).

### Phase 2 — `/org/parent-links` page

**Add:**
- `src/app/(portal)/org/parent-links/page.tsx` — server component. Fetches `GET /api/orgs/{orgId}/parent-links?status=active`. Renders header + filter bar + table. Each row has parent email, child name + class, created at, "Revoke" button.
- `src/components/org/create-parent-link-modal.tsx` — client component. Form: parent email (required), child picker (autocomplete searching the org's students). Submit POSTs.
- `src/components/org/revoke-parent-link-button.tsx` — confirmation dialog → DELETE → refresh.

**Modify:**
- `src/lib/portal/nav-config.ts` — add a `Parent links` nav item to the org_admin portal config, after `Settings`. Icon: `users` or `link` (lucide).

### Phase 3 — Read-only teacher view

**Modify:**
- `src/app/(portal)/teacher/classes/[id]/students/[studentId]/page.tsx` (verify path; if it doesn't exist, this becomes "Add" instead). Adds a "Parents" card section listing linked parents (read-only). Pulls from the new `GET /api/orgs/{orgId}/parent-links?child={studentId}` endpoint OR a new `/api/teacher/students/{id}/parents` if cross-org-call is awkward (Phase 0 question for Codex).

### Phase 4 — Onboarding copy

**Modify:**
- `src/app/(portal)/onboarding/page.tsx` — find the "Your child's teacher will link your account" string and rewrite. New copy: "Ask your child's teacher or school administrator to link your account so you can see {child}'s class progress. The link usually appears within 24 hours of joining."
- Sweep for any other parent-onboarding copy (registration page, parent dashboard empty state) and align the wording.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Cross-org parent (one parent, two children at two schools) — both org admins create links independently | low | Schema supports it; multiple links per parent are fine. UI lists them as expected. |
| Org admin links a child outside their org by typing the child's id | medium | Backend validates child belongs to the org's classes (Decisions §2). Returns 403 if not. UI maps to "Student not in this organization." |
| Org admin revokes a link by mistake | low | Revoke is soft (Decisions §5). Re-creating restores the relationship. Confirmation dialog catches accidents. |
| Parent registers AFTER the org admin tries to link them | medium | UI's create-with-unknown-parent path (returns 404) shows a copy-link helper: "Send the parent this registration link." Same pattern as plan 069's invite. |
| Privacy: org admin sees ALL parent emails for their org | low | This is an intentional admin power. School data governance accepts this; parents who don't want their email visible would not link. |
| Schema-drift surface: we now have org-scoped + platform-scoped CRUD | low | Both share the same store layer. No data divergence. |
| Onboarding copy wording change introduces a translation-string break | low | Bridge has no i18n today; single English string. Rewrite-in-place is safe. |

## Phases

### Phase 0 — Pre-impl Codex review

Per CLAUDE.md plan-review gate. Dispatch `codex:codex-rescue` to review against:
- `platform/internal/handlers/admin.go` (existing platform-admin parent-link handlers — pattern to mirror)
- `platform/internal/store/parent_links.go` (the store layer the new handler reuses)
- `platform/internal/handlers/orgs.go` (the existing org route group the new endpoints mount under)
- `src/app/(portal)/onboarding/page.tsx` (copy to update)
- `src/app/(portal)/parent/page.tsx` (parent dashboard that shows "No children linked yet")
- The reviewer's recommendation in `docs/reviews/010-comprehensive-browser-review-2026-05-03.md` §"Parent onboarding copy promises teacher-driven linking"

Specific questions:
1. The "child belongs to the org" check (Decisions §2) — what's the simplest SQL? Is there an existing helper in `parent_links.go` or `org_memberships.go` we can reuse?
2. Parent users typically have NO `org_memberships` rows (they're just Bridge users with `intendedRole=parent`). Does any existing code path assume parents must be in an org?
3. Is there an existing teacher-portal page for "students" detail (`/teacher/classes/{id}/students/{studentId}`) or do we need to add it as part of Phase 3?
4. The onboarding flow: where exactly does the parent-specific copy live? Walk the flow from `/register?intendedRole=parent` through `/onboarding` to `/parent`.
5. Plan 070b (token-based parent claim) — does the schema (plan 064's `parent_links`) accommodate a "claim_token" column without a migration, or will v2 need its own migration? Want to know if v1 should leave any bread crumbs.
6. Privacy: should the org-admin's parent-links list mask parent emails by default with a click-to-reveal? Most school admin tools show emails plain; flagging as a question only.

### Phase 1 — Backend endpoints (PR 1)

- Implement handler + tests.
- Wire into `cmd/api/main.go`.
- `cd platform && go test ./...` clean.
- Codex post-impl review.
- PR + merge.

### Phase 2 — Org-admin UI (PR 2)

- Implement page + create-modal + revoke-button.
- Wire nav-config.
- Smoke test: as `frank@demo.edu` (org admin), create a link between a real parent + child.
- Codex post-impl review.
- PR + merge.

### Phase 3 — Teacher read-only view (PR 3)

- Implement (or extend) the student detail page with the Parents card.
- Smoke test: as `eve@demo.edu` (teacher), view a student page, see linked parents.
- Codex post-impl review.
- PR + merge.

### Phase 4 — Onboarding copy update (PR 4)

- Sweep onboarding flow for parent-facing copy.
- Update wording per Decisions §8.
- Smoke test: register a fresh parent account, walk through `/register?intendedRole=parent` → `/onboarding` → `/parent`, confirm new copy reads correctly.
- PR + merge (no Codex needed for copy-only changes; flag this as a deviation in the PR description).

## Codex Review of This Plan

_(To be populated by Codex pass — see Phase 0.)_
