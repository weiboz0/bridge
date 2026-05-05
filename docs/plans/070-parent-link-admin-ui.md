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

3. **Update parent-onboarding copy** (`src/app/onboarding/page.tsx` or wherever the parent-flow text lives) to match: "Your school administrator will link your account to your child's. Reach out to your child's teacher or your school's admin if you don't see your child within 24 hours."

4. **Optional read-only teacher view**: `/teacher/classes/{id}/students/{studentId}` adds a "Parents" section listing the linked parents (with no actions). This gives teachers visibility without write access — they can see who to contact about a linkage problem.

### Why org-admin-only for v1

The reviewer noted: "Org admin-managed is safer for school data governance. Teacher-requested, org-admin-approved is a good next step if teachers know the family context."

For v1 we ship the safer path. The upgrade path to teacher-requested is non-breaking (add a `parent_link_requests` table, add teacher-side request UI, org-admin sees pending requests in the same `/org/parent-links` page). v2 doesn't invalidate v1.

### Why a new endpoint family instead of widening `/api/admin/parent-links`

The platform-admin endpoints are intentionally global — they can list every link in the system, link any parent to any child. Org admins should only see/manage links involving students in THEIR org's classes. Two different scope queries; cleanest as two different endpoints. (The store layer is shared.)

## Decisions to lock in

1. **v1 is org-admin-managed.** Teachers see (read-only) but don't write. Token-based parent-claim flow is plan 070b. Captured here so the product decision doesn't get re-litigated mid-implementation.
2. **Children scoped by org via class membership.** Authorization rule: `child_user_id` must be a member with `role='student'` of at least one ACTIVE class in the calling org_admin's org. The EXISTS query (Codex pass-1 §Q1): `SELECT 1 FROM class_memberships cm JOIN classes c ON c.id = cm.class_id WHERE cm.user_id = $1 AND cm.role = 'student' AND c.org_id = $2 AND c.status = 'active'`. `classes.org_id` exists directly — no course join needed. (The "any role" wording earlier was wrong; only students should be linkable as children.)
3. **Parent DOES need an `org_memberships` row** (Codex pass-1 finding — original plan said the opposite). Bridge's portal-access logic at `src/components/portal/portal-shell.tsx:37-44` and `platform/internal/handlers/me.go:117-124` derives the user's accessible portals from their `org_memberships` rows. A parent without an org_membership row cannot reach `/parent` at all — they'd land on `/onboarding` indefinitely. Resolution: when an org-admin creates a parent link, the `CreateLink` endpoint ALSO upserts an `org_memberships` row with `role='parent'`, `status='active'` for the parent in the calling org. The upsert MUST be `DO UPDATE SET status='active'` (not the existing `DO NOTHING` from `AddOrgMember`) so a previously-suspended parent membership reactivates — see Phase 1 §`CreateLink` for the new `UpsertActiveMembership` store method. Decisions §6 spells out the invariant: every active `parent_links` row implies a corresponding `org_memberships{role:'parent', status:'active'}` row in the org of at least one of the parent's children.

    **Revoke does NOT remove the org membership** (Codex pass-2 confirmed the design is sound but flagged a subtlety): a parent might have multiple children in the same org, so revoking one link mustn't break access to the others. The trade-off is that a revoked parent retains `/parent` portal access, but the parent dashboard will show "no children linked yet" because the data layer queries `parent_links` (not org_memberships). For v1 we accept this — it's "stale portal access without data leak". A separate admin action (or a follow-up plan) can sweep org_memberships for parents with no active links if/when product asks.
4. **Idempotent re-link** (matches existing platform-admin behavior at `admin.go:307`). If an active link already exists for the (parent, child) pair, return 409 — UI maps to "Already linked, no action needed."
5. **Revoke is soft-delete.** The schema flips status to `revoked` + sets `revoked_at` (plan 064 §"Soft-revoke"). Re-creating later is allowed (partial-unique index).
6. **List filters: by parent email OR by child name OR by class.** Three independent filters; org admin picks whichever scope they're investigating.
7. **No bulk-revoke v1.** If the org admin needs to revoke many at once (e.g., end-of-year cleanup), they revoke one at a time. Bulk operations are rare and add UI complexity.
8. **Onboarding copy update is non-trivial wording, not just a substring rename.** The full sentence shifts from "your teacher will link" → "your school admin (or teacher acting on their behalf) will link". Phase 4 includes the copy review.

## Files

### Phase 1 — Backend `/api/orgs/{orgId}/parent-links` endpoints

**Add:**
- `platform/internal/handlers/org_parent_links.go` — handler holding `OrgParentLinksHandler` with three methods:
  - `ListByOrg(w, r)` — `GET /api/orgs/{orgId}/parent-links?status=active|revoked|all&parent={email?}&child={uid?}&class={uid?}`. Returns links scoped to children in the org. Filtering composable. Codex pass-1 important: `ParentLinkStore` returns IDs/status/timestamps only — needs an enriched query method or per-row JOINs to surface parent email + child name + class. Plan 070 Phase 1 includes that enrichment as a small store-layer addition.
  - `CreateLink(w, r)` — `POST /api/orgs/{orgId}/parent-links` with body `{ parentEmail, childUserId }`. Resolves parent by email (404 if missing — UI surfaces "ask parent to register"), validates child is in the org via `EXISTS (SELECT 1 FROM class_memberships cm JOIN classes c ON c.id = cm.class_id WHERE cm.user_id = $1 AND cm.role = 'student' AND c.org_id = $2 AND c.status = 'active')` (Codex pass-1 §Q1 — `classes.org_id` exists directly, no course join needed), calls `parentLinks.CreateLink(ctx, parentId, childId, callerId)`, **AND upserts `org_memberships{user_id: parentId, org_id: orgId, role: 'parent', status: 'active'}` (Decisions §3 — without this, the parent can't reach `/parent`).**

    Codex pass-2 caught a subtlety: the existing `OrgStore.AddOrgMember` uses `ON CONFLICT (org_id, user_id, role) DO NOTHING` (`platform/internal/store/orgs.go:275, :284`), which would silently keep a suspended/pending parent membership inactive at link time. Plan 070 needs a NEW store method that does `ON CONFLICT (org_id, user_id, role) DO UPDATE SET status='active'` so a previously-suspended membership reactivates. Add `OrgStore.UpsertActiveMembership(ctx, orgId, userId, role)` for this. Phase 1 includes the new store method + an integration test for the reactivation case.

    Returns 201 + the link, 409 if already active.
  - `RevokeLink(w, r)` — `DELETE /api/orgs/{orgId}/parent-links/{linkId}`. Validates the link's child belongs to the org; reuses `parentLinks.RevokeLink(ctx, linkId)`. Does NOT remove the org_memberships row (might be needed for other children — see Decisions §3).
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

### Phase 3 — Read-only teacher view (popover on class detail page)

Phase 3 adds a "Parents" popover affordance to the existing class detail page at `src/app/(portal)/teacher/classes/[id]/page.tsx`. Each student row gets a small "P" badge with parent count; clicking opens a popover listing linked parents (read-only). This is a lighter v1 scope than a new student-detail page (no such page exists today).

**Modify:**
- `src/app/(portal)/teacher/classes/[id]/page.tsx` — add a parent-count badge per student row, plus a popover with parent details. Pulls from a new lightweight teacher-scoped endpoint `GET /api/teacher/classes/{classId}/parent-links` (returns one row per (student, parent) tuple for students in the class). Backend addition listed below.

**Add:**
- `platform/internal/handlers/teacher_parent_links.go` — `GET /api/teacher/classes/{classId}/parent-links` handler. Auth: caller is teacher/ta in `classId` OR org_admin in the class's org. Returns `[]{ studentUserId, parentUserId, parentEmail, parentName, linkId, createdAt }` for active links of all students in the class.

### Phase 4 — Onboarding copy

**Modify:**
- `src/app/onboarding/page.tsx` — find the "Your child's teacher will link your account" string and rewrite. New copy: "Ask your child's teacher or school administrator to link your account so you can see {child}'s class progress. The link usually appears within 24 hours of joining."
- Sweep for any other parent-onboarding copy (registration page, parent dashboard empty state) and align the wording.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Cross-org parent (one parent, two children at two schools) — both org admins create links independently | low | Schema supports it; multiple links per parent are fine. UI lists them as expected. |
| Org admin links a child outside their org by typing the child's id | medium | Backend validates child belongs to the org's classes (Decisions §2). Returns 403 if not. UI maps to "Student not in this organization." |
| Org admin revokes a link by mistake | low | Revoke is soft (Decisions §5). Re-creating restores the relationship. Confirmation dialog catches accidents. |
| Parent registers AFTER the org admin tries to link them | medium | UI's create-with-unknown-parent path (returns 404) shows a copy-link helper with plain `/register` link (NOT `?intendedRole=parent` — Codex pass-1: register page only reads `?invite=`, and the Go register handler at `platform/internal/handlers/auth.go:21-27` doesn't accept `parent` as `intendedRole` today). The parent registers with no role intent; the admin then links them via Phase 2 UI; the link-time `org_memberships{role:'parent'}` upsert (Decisions §3) grants `/parent` portal access on next sign-in. Same pattern as plan 069's invite. |
| Privacy: org admin sees ALL parent emails for their org | low | This is an intentional admin power. School data governance accepts this; parents who don't want their email visible would not link. |
| Schema-drift surface: we now have org-scoped + platform-scoped CRUD | low | Both share the same store layer. No data divergence. |
| Onboarding copy wording change introduces a translation-string break | low | Bridge has no i18n today; single English string. Rewrite-in-place is safe. |

## Phases

### Phase 0 — Pre-impl Codex review

Per CLAUDE.md plan-review gate. Dispatch `codex:codex-rescue` to review against:
- `platform/internal/handlers/admin.go` (existing platform-admin parent-link handlers — pattern to mirror)
- `platform/internal/store/parent_links.go` (the store layer the new handler reuses)
- `platform/internal/handlers/orgs.go` (the existing org route group the new endpoints mount under)
- `src/app/onboarding/page.tsx` (copy to update)
- `src/app/(portal)/parent/page.tsx` (parent dashboard that shows "No children linked yet")
- The reviewer's recommendation in `docs/reviews/010-comprehensive-browser-review-2026-05-03.md` §"Parent onboarding copy promises teacher-driven linking"

Specific questions:
1. The "child belongs to the org" check (Decisions §2) — what's the simplest SQL? Is there an existing helper in `parent_links.go` or `org_memberships.go` we can reuse?
2. (Codex pass-1 confirmed: parents DO need `org_memberships` rows — Bridge's portal-access logic derives roles from that table. Decisions §3 added the link-time upsert. Question retained for audit trail.)
3. Is there an existing teacher-portal page for "students" detail (`/teacher/classes/{id}/students/{studentId}`) or do we need to add it as part of Phase 3?
4. The onboarding flow: where exactly does the parent-specific copy live? Walk the flow from `/register` (no role intent — Codex pass-1 confirmed `?intendedRole=parent` is not supported) through `/onboarding` to `/parent`.
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

- Implement the Parents popover on the existing class-detail page (`src/app/(portal)/teacher/classes/[id]/page.tsx`). The original "student detail page" framing was abandoned in Phase 3 (no such page exists; pivoted to a popover affordance instead).
- Smoke test: as `eve@demo.edu` (teacher), open the class detail page (`/teacher/classes/{id}`), confirm a "P" badge appears next to each linked student and the popover lists their linked parents.
- Codex post-impl review.
- PR + merge.

### Phase 4 — Onboarding copy update (PR 4)

- Sweep onboarding flow for parent-facing copy.
- Update wording per Decisions §8.
- Smoke test: register a fresh user (no role intent — register page doesn't read `?intendedRole=parent`), have an org admin link them via Phase 2 UI, verify the user can now reach `/parent` and sees their child(ren). Confirm onboarding copy reads correctly.
- PR + merge (no Codex needed for copy-only changes; flag this as a deviation in the PR description).

## Code Review

### Phase 3 post-impl — 2026-05-04 (in progress)

First post-impl review under the new 4-way policy (CLAUDE.md commit 3e7397b). Self-review committed first; external reviewers dispatched in parallel and verdicts will land here as they arrive.

**Self-review (Opus 4.7) — 1 NIT:**
- `ListByClass` SQL doesn't filter `classes.status = 'active'`. A parent linked to a student in an archived class would surface if the popover were opened. Acceptable defense (the class-detail page itself usually blocks archived classes), but defense-in-depth would tighten it. Marking as a NIT — not a blocker.

**DeepSeek V4 Flash — APPROVED.** Confirmed self-review's archived-class NIT (cross-method consistency: `ListByOrg` filters `classes.status='active'`, `ListByClass` doesn't). Found one harmless dead-code branch: the page's `.catch` handles 403, but the handler actually returns 401 (no claims) or 404 (denied). Test coverage is thorough; outside-click dismissal is correct; type drift is zero (Go JSON ↔ TS field names map 1:1). Acceptable to ship; minor cleanup welcome.

**Codex — CONCUR with 1 BLOCKER + 1 NIT** (both fixed inline):
- BLOCKER: archived-class query escalated from "NIT" to "BLOCKER". Codex correctly noted that `GetClass` does not gate on `status='active'`, so a teacher navigating directly to an archived class URL CAN reach this endpoint and see parent emails. **FIXED**: `ListByClass` SQL now joins `classes` and filters `c.status = 'active'` (matching `ListByOrg`'s pattern). Regression locked with `TestTeacherParentLinks_ArchivedClass_NotShown`.
- NIT: the parent-count badge had `title` but no `aria-label`. **FIXED**: added explicit `aria-label` describing the parent-link count + click action for screen readers.

Drive-by from DeepSeek's dead-code finding: the page's `.catch` now only handles 404 (was `404 || 403`); 403 is dead code because the handler emits 401 or 404 only.

**GLM 5.1 — needs-changes (1 BLOCKER REJECTED + 1 BLOCKER FIXED + 1 NIT DEFERRED):**

- BLOCKER 1 **REJECTED**: GLM claimed `ListByClass` SQL must filter `cm.status = 'active'` because deactivated student memberships would leak parent links. False premise — `class_memberships` table has NO `status` column (only `id`, `class_id`, `user_id`, `role`, `joined_at` per `drizzle/0004_course-hierarchy.sql:46-52`). Member removal is DELETE-on-row, not a status flip. The recommended filter would be a SQL error. No change required.
- BLOCKER 2 **FIXED**: observer/guest denial path was implicit (covered only via student-role denial). Added `TestTeacherParentLinks_ObserverAndGuest_Denied` with sub-tests for both roles. Test count: 11 → 13.
- NIT **DEFERRED**: popover lacks focus-trap (no focus management on open). Consistent with phase 2's deferred ARIA polish — file as a follow-up. Not release-blocking.

**Final verdict (round 1)**: blockers either fixed inline (Codex archived-class, GLM observer/guest test) or rejected with technical evidence (GLM cm.status). NITs not deferred (DeepSeek dead-code, Codex aria-label) also fixed.

#### Round 2 — confirmation pass

Per the policy, round 2 re-dispatches only the reviewers who flagged blockers in round 1.

- **Codex round-2 — CONCUR.** Clean confirmation; both archived-class BLOCKER fix and aria-label NIT fix accepted, no new findings.
- **GLM 5.1 round-2** — round-1 dispatch used a typo'd identifier (`volcengine/glm-5.1` doesn't exist in opencode's registry; the canonical path is `volcengine-plan/glm-5.1`). Re-dispatched with the correct identifier; verdict pending.

### Phase 2 post-impl — 2026-05-04: NITS, 2 fixed inline + 1 deferred

Codex post-impl review of `feat/070-phase-2-org-parent-links-ui` (commit 8c57340 + follow-ups). Verdict: NITS only. Three follow-ups; two fixed in-PR, one deferred:

1. **NIT-1 FIXED**: eligible-children fetch errors were silently swallowed by a `.catch(() => [])` so an admin would see "no enrolled students" when the backend was actually broken. Fix: capture the failure separately as `studentsError`, pass it through to the create modal which now shows "Couldn't load the student roster (…)" instead of the generic "no students" hint.

2. **NIT-2 FIXED**: backdrop-click-to-close compared `e.target` against the inner-dialog ref (which never matches when the user clicks the backdrop). Fix: ref attached to the outer backdrop div, hit-test compares against it directly.

3. **NIT-3 DEFERRED** (a11y polish): the autocomplete dropdown isn't a proper combobox — missing `aria-expanded`, `aria-activedescendant`, roving focus. Listed as a future polish pass; not release-blocking absent a hard WCAG gate.

CONCUR on all other questions: org_admin resolution fallback handles the teacher-in-A/admin-in-B case correctly, free-typed child rejection is appropriate for v1, eligible-children fetch on every render is acceptable, revoke confirmation copy is clear, router.refresh() pattern is consistent with the rest of the codebase, and the eligible-children backend extension is properly scoped (covered by 3 integration tests).

### Phase 1 post-impl — 2026-05-04: 4 NITS, all fixed inline

Codex post-impl review of `feat/070-phase-1-org-parent-links-backend` (commits 571a350 + follow-ups). Verdict: NITS only, no blockers. Four items fixed:

1. **Cross-org-admin test gap (Q1)** — `requireOrgAdmin` correctly 403s an org_admin from a different org, but no explicit test covered that path. Added `TestOrgParentLinks_List_CrossOrgAdminForbidden` that seeds an active org_admin in `otherOrgID` and asserts the same user gets 403 against `orgID`'s endpoint.

2. **Class-filter scoping (Q5)** — the `cm2` EXISTS subquery on class filter didn't join `classes` to assert the filtered class belongs to the org or is active. Added `JOIN classes c2` with `c2.org_id = $1 AND c2.status = 'active'`. The displayed `classId/className` still comes from the lateral most-recent class, which may differ from the filter id when a child is in multiple classes — documented as acceptable v1 behavior.

3. **Link/membership transactionality (Q7)** — link insert + membership upsert ran as two independent statements. Failure between them produced an orphan link with no membership ("child shown in API but parent can't reach /parent"). Fix: new `ParentLinkStore.CreateLinkWithMembership` runs both in a single tx; on either failure, both roll back. Handler swaps in the new method. The naked `CreateLink` stays for the platform-admin handler which has no org context.

4. **UUID validation middleware (Q8)** — routes mounted without `ValidateUUIDParam("orgID")` or `("linkID")`, unlike sibling routes in `orgs.go`. Malformed IDs would surface as 500 instead of 400. Added `r.Use(ValidateUUIDParam("orgID"))` and a nested route group with `ValidateUUIDParam("linkID")`.

Drive-by: fixed a flake in plan-071's `TestProblemStore_CreateProblem_SlugAllowedInDifferentScope` — the test inserted a hardcoded email derived from `t.Name()` that collided on second run without DB cleanup. Added timestamp-suffix + `t.Cleanup`.

Full Go suite passes including all 15 org-parent-link tests.

## Codex Review of This Plan

### Pass 3 — 2026-05-04: 3 stale-text scrubs

Codex pass-3 confirmed the UpsertActiveMembership design and the revoke trade-off are sound but caught remaining stale references. Scrubbed:
- Phase 3 narrative no longer presents "Option A: new student detail page" — only the popover-on-class-detail design remains (the option-A framing was a pass-2 artifact).
- Phase 3 smoke test wording changed from "view a student page" to "open the class detail page and confirm the P badge + popover".
- Phase 0 historical question on line 144 (the "parents typically have NO org_memberships" framing) annotated as superseded by Decisions §3's link-time upsert.
- Phase 0 historical question on line 152 (`/register?intendedRole=parent`) annotated as superseded.

### Pass 2 — 2026-05-04: CONCUR-WITH-CHANGES → 3 cleanup items folded

Codex pass-2 confirmed pass-1's substantive direction (org_memberships upsert at link time, child-belongs-to-org EXISTS query, Phase 3 popover pivot). Three cleanup items folded:

1. **Stale "any role" wording** — Decisions §2 said "any role" but the EXISTS query and intent are `role='student'` only. Decisions §2 rewritten with the explicit query.
2. **Stale "student detail page" reference** in Phase 3 — pivoted in pass-1 to a popover on the existing class-detail page. Phase 3 checklist updated to match.
3. **`ON CONFLICT DO NOTHING` upsert gap** — `OrgStore.AddOrgMember` uses `DO NOTHING`, which would silently keep a suspended/pending parent membership inactive at link time. Phase 1 now adds a NEW `OrgStore.UpsertActiveMembership` method with `DO UPDATE SET status='active'` plus an integration test for the reactivation case.
4. **Revoke retains stale portal access** — Decisions §3 now documents this trade-off explicitly (no data leak; parent dashboard shows "no children linked yet" because data layer queries `parent_links`, not `org_memberships`). Sweep-stale-memberships is a follow-up if product asks.
5. **Stale `/register?intendedRole=parent` references** in smoke-test text — rewritten to reflect the no-role-intent registration flow.

### Pass 1 — 2026-05-03: BLOCKED → 2 blockers + 4 important folded in

Codex pass-1 returned BLOCKED with two blockers, both addressed:

1. **Parent portal access derives from `org_memberships`, not `parent_links`** — original plan's "parents typically don't need org_memberships" assumption was wrong. Without an `org_memberships{role:'parent'}` row, a linked parent can't reach `/parent`. Resolved: Decisions §3 rewritten — `CreateLink` upserts the org_membership at link time. The invariant is now: every active `parent_links` row implies a corresponding `org_memberships{role:'parent'}` row in at least one of the parent's children's orgs.

2. **`/register?intendedRole=parent` doesn't exist** — register page only reads `?invite=`, and Go's register handler doesn't accept `parent` as an intendedRole. Resolved: invite-not-found copy uses plain `/register`. The link-time `org_memberships` upsert (per #1) means the parent doesn't need to declare the role at registration; the admin's link grants the role retroactively.

Important non-blocking, all folded:

3. **Onboarding file path was wrong** — `src/app/onboarding/page.tsx`, NOT `src/app/(portal)/onboarding/page.tsx`. All references updated.
4. **`ParentLinkStore` returns IDs/timestamps only** — needs an enriched query method to surface parent email + child name + class. Phase 1 includes the store-layer addition.
5. **No teacher student-detail page exists** — Phase 3 pivots from "add new student-detail page" to "add affordance to existing class-detail page" (the lighter v1 scope).
6. **Child-belongs-to-org SQL is direct** — `classes.org_id` exists; no course join needed. CreateLink validation uses the simpler EXISTS query Codex pass-1 §Q1 spelled out.

CONFIRMED by Codex (no changes):
- Plan-064 platform-admin parent-link CRUD exists at `/api/admin/parent-links`.
- Duplicate active link returns 409 via `ErrParentLinkExists`.
- Revoke is soft-delete via `status='revoked'` + `revoked_at`.
- `parent_links` schema has no `claim_token` — plan 070b token-based claim flow needs its own migration.
- Parent dashboard currently shows "No children linked yet" with teacher-linking copy.
- Review-010 file IS present in the repo (Codex's "not found" was a fetch glitch — verified locally).

Verdict: **BLOCKED → all blockers resolved → ready for Phase 1** pending pass-2.
