# Plan 069 — Org-admin write surface (invites, class drill-down, settings edit)

## Status

- **Date:** 2026-05-03
- **Origin:** Comprehensive browser review 010 §P2 ("Org-admin list pages are useful but remain read-only with no management affordances"). The teacher portal got a lot more capable in the previous cycle, but the org-admin portal renders six list pages (`teachers`, `students`, `courses`, `classes`, `units`, `settings`) with zero write operations. `/org/settings` literally says editing is coming later. Org admins can see who's in their school, but can't invite anyone, can't drill into a class, and can't update the school's contact email.
- **Scope:** Next portal pages + a small number of new Go endpoints for the invite flow (existing `AddMember` accepts an existing-user email; an "invite by email when user doesn't exist yet" flow needs a new path). No schema changes for the v1 invite — defer email-token invites to a follow-up; v1 invites add an *existing* user by email or send them a `/register` link.
- **Predecessor context:** The org member CRUD already exists at `platform/internal/handlers/orgs.go:30-34` (`AddMember`, `UpdateMember`, `RemoveMember`). The class CRUD exists at `platform/internal/handlers/classes.go:20-34`. The Org settings update is at `orgs.go:27` (`UpdateOrg`). All the backend pieces are in place; this plan adds the UI.

## Problem

Today, an org admin can do exactly two write actions in the entire portal:
- The "Add Member" backend exists but no UI exposes it from `/org/teachers` or `/org/students`.
- The "Update Org" backend exists but `/org/settings` shows "Settings editing coming later" instead of a form.

Concrete missing flows:

| Action | Backend | UI |
|---|---|---|
| Invite/add a teacher to the org | ✅ `POST /api/orgs/{id}/members` | ❌ no form |
| Invite/add a student to the org | ✅ same endpoint | ❌ no form |
| Update member's STATUS (pending/active/suspended) within the org | ✅ `PATCH /api/orgs/{id}/members/{memberId}` | ❌ no UI |
| Remove a member | ✅ `DELETE /api/orgs/{id}/members/{memberId}` | ❌ no UI |
| Drill into a class to see its roster + instructor | ✅ `GET /api/classes/{id}` + `/api/classes/{id}/members` | ❌ from `/org/classes`, no clickable rows |
| Update org contact email / domain | ✅ `PATCH /api/orgs/{id}` | ❌ "coming later" |

The reviewer's recommended priorities (from review 010 §P2 and the "Recommended Next Cycle"):
1. Invite teacher/student/member by email.
2. Class detail read-only drill-down.
3. Settings edit flow (contact email, contact name, domain) with audit trail.
4. Parent-child link management — DEFERRED to plan 070, see that plan.

## Out of scope

- Email-token invite flow (where the invitee gets a link with a one-time token that auto-creates their account on click). v1 just requires the invitee to register via `/register` first — the org admin then invites the existing user by email. This matches what the backend supports today; token-based invites are plan 069b material.
- Bulk import (CSV roster upload, etc.). Single-add forms for v1.
- Audit-trail log for settings edits. The reviewer recommends one but `org_audit_log` is a new table that warrants its own plan. v1 just persists the change; a simple "last updated by / at" field is enough for the immediate need.
- Cross-org member moves. Adding a teacher who already belongs to another org is fine (multi-org membership is supported); transferring is not in scope.
- Class CREATE from `/org/classes`. Teachers create classes from the teacher portal; org admin's role here is read-only with the option to inspect/archive.
- Class member management from org-admin. Teachers manage their own class roster; org admin sees but doesn't edit.
- Role escalation to org_admin. The first org_admin is bootstrapped via the platform admin (existing flow); this plan doesn't add a "promote teacher to org_admin" UI. Defer until product confirms desired escalation path.

## Approach

Five UI additions, all calling existing Go endpoints (one new endpoint for invite-via-domain validation; details below).

### 1. `/org/teachers` and `/org/students` get an "Add" form

Each page gets a sticky "+ Invite teacher" / "+ Invite student" button at the top-right. Clicking opens a modal with fields:
- Email (required, validated via zod `.email()`)
- Display name (read-only after the user is found by email; editable on the response if they don't have one yet)
- Role (preselected: `teacher` for `/org/teachers`, `student` for `/org/students`)

Submit POSTs `/api/orgs/{orgId}/members` with `{ email, role }`. The backend already finds the user by email and returns 404 if the user doesn't exist. The form catches the 404 and shows an inline message: "User not found. Send them this registration link: `https://bridge.../register`. After they register, return here and try the invite again." Copy-to-clipboard button.

**The link is plain `/register`, NOT `/register?intendedRole=teacher`** (Codex pass-1 caught this). The register page only reads `?invite=` today, not `?intendedRole=`. The role-intent URL pattern is plan 043's signup-flow concept but doesn't have URL wiring. v1 of the invite ships without role-prefill on the registration link; the invitee picks their role on the registration form. Plan 069b can add role-prefill if/when the register page learns to read it.

### 2. `/org/classes/{classId}` drill-down (read-only)

Server component. Fetches `GET /api/classes/{id}` + `GET /api/classes/{id}/members`. Renders:
- Class header (title, term, course title, instructor names).
- Roster table (student name, email, joined date, status). Search bar.
- "Open in teacher portal" link — for an org admin who is also a teacher in the class, links to `/teacher/classes/{id}`.
- "Archive class" button at the bottom (POSTs the `PATCH /api/classes/{id}` archive endpoint at `classes.go:27`). Confirmation dialog. Org admins can archive any class in their org.

Wire up by making each row in `/org/classes` a `<Link>` to `/org/classes/{id}`.

### 3. `/org/settings` edit form

Replace the "coming later" message with a form mirroring the read-only fields. Fields:
- Org name (string, required, 1-255)
- Contact email (email, required)
- Contact name (string, required, 1-255)
- Domain (string, optional, 1-255)

Submit PATCHes `/api/orgs/{orgId}`. On success, refresh the page (server component re-fetches). Backend already supports this (`orgs.go:27` `UpdateOrg`).

For the "audit trail" the reviewer asked for: defer the dedicated table, but persist `updated_at` (already in the schema) and surface "Last updated {x}" in the UI. **Codex pass-1 confirmed there is NO `updated_by` column on `organizations`.** Drop the "by {y}" half — show only `updated_at`. Adding `updated_by` is a 5-minute migration if/when product asks; not in v1.

### 4. `/org/teachers/{userId}` and `/org/students/{userId}` quick-actions menu

Beside each row, a 3-dot menu with:
- **Update STATUS** (modal with status select: `pending` / `active` / `suspended`) — Codex pass-1 caught that the existing `PATCH /api/orgs/{orgID}/members/{memberID}` accepts `{status}` not `{role}`. Role-update is NOT currently a backend operation; would need a new endpoint. v1 ships status-change only; "Promote to org_admin" / role changes are deferred to plan 069b.
- Remove from org (confirmation dialog)

Update status POSTs `PATCH /api/orgs/{orgId}/members/{memberId}` with `{ status }`. Remove POSTs `DELETE /api/orgs/{orgId}/members/{memberId}`. Both endpoints exist (`orgs.go:33-34`). Surface 403 inline.

### 5. Optional: domain-based hint on `/org/settings`

When the org has a `domain` set, the invite forms (§1) check the email's domain against the org's domain on submit. Same-domain → submit normally. Different-domain → confirmation: "This email's domain doesn't match the org's domain (`{domain}`). Continue anyway?" Catches typos like inviting a Gmail address to a school org. Optional polish; defer to Phase 5 if it's the most-deferrable item.

## Decisions to lock in

1. **No email-token invites in v1.** The current `AddMember` requires the user to already exist. Token invites are a meaningful schema addition (token table, expiry, single-use enforcement); plan 069b owns that.
2. **Server components for read; client components only for forms.** Same precedent as plan 066. Mutations go through `<form action={serverAction}>` where possible, otherwise `useState` + `onSubmit`.
3. **Role changes are out of scope for v1.** The backend's PATCH endpoint accepts `{status}` only. Member quick-actions ship as status-update + remove. (Role mutation is a separate scope; revisit when product asks.)
4. **Class-drill-down is read-only with the "open in teacher portal" escape hatch.** Org admins are operators, not instructors. If they need to mutate class state (assignments, sessions, etc.), they sign in as a teacher in that class. No duplicate UIs.
5. **Settings edit auto-saves on blur for low-stakes fields (contact name, domain), explicit Save for high-stakes (email).** Pulling back from this — too clever. Single Save button at the bottom; whole form is one PATCH. Reviewer didn't ask for granular saves.
6. **Settings audit surface is `updated_at` only.** Settings UI shows "Last updated {timestamp}" — no actor column. (No "by {y}" surface; that requires a schema addition that isn't in v1.)

## Files

### Phase 1 — `/org/teachers` + `/org/students` invite forms

**Add:**
- `src/components/org/invite-member-modal.tsx` — client component. Props: `{ orgId, role, onClose, onSuccess }`. **Mirror the structure of `src/components/org/create-parent-link-modal.tsx`** (plan 070, polished in #127): backdrop ref + click-to-close, Escape-key dismissal, autoFocus on first input, `role="dialog"` + `aria-modal`. Backend POSTs `/api/orgs/{orgId}/members` with `{ email, role }` (verified — `orgs.go:301-327`). 404 → "User not found, share `/register` link" with copy-to-clipboard. Handle 409 → "Already a member of this org" if backend returns it.

**Modify:**
- `src/app/(portal)/org/teachers/page.tsx` — add `+ Invite teacher` button (top-right of header) that opens the modal.
- `src/app/(portal)/org/students/page.tsx` — same pattern with `role="student"`.

### Phase 2 — Class drill-down

**Add:**
- `src/app/(portal)/org/classes/[classId]/page.tsx` — server component. Fetches class + members. Codex pass-1 noted `GET /api/classes/{id}` returns `courseId` but NOT `courseTitle`. Pass-2 caught a real auth gap with the original "two-fetch" approach: `GET /api/courses/{courseId}` is gated to creator/platform-admin/class-member only (`platform/internal/handlers/courses.go:141-172`). An org admin who isn't enrolled in the class would 403 on the second fetch. Resolution (option B from pass-1, now required not optional): **extend `GET /api/classes/{id}` to also return `courseTitle`**. Single backend change adds an inline join in the existing class query. Phase 2 includes the backend change explicitly.
- `src/components/org/archive-class-button.tsx` — client component. Confirmation dialog → empty-body PATCH → refresh. Codex pass-1 confirmed `PATCH /api/classes/{id}` takes NO request body — `ArchiveClass` unconditionally sets `status='archived'`. The fetch sends `{ method: 'PATCH' }` with no body.

**"Open in teacher portal" link** (self-review NIT #4): the page should compute `myRole = members.find(m => m.userId === identity.userId)?.role` and only render the link when `myRole === "instructor" || myRole === "ta"`. Org admins who aren't teachers in the class would 403 on `/teacher/classes/{id}` — better to hide the link.

**Modify:**
- `src/app/(portal)/org/classes/page.tsx` — wrap each class title in `<Link href={`/org/classes/${cls.id}`}>`.

### Phase 3 — Settings edit form

**Modify:**
- `src/app/(portal)/org/settings/page.tsx` — replace the "coming later" placeholder with a `<form>` (server action) that PATCHes the org. Show "Last updated {x}" derived from `updated_at`.
- `src/components/org/org-settings-card.tsx` — extend the local `OrgSettingsData` type (currently lines 5-14) to include `updatedAt: string` (DeepSeek post-impl finding). Backend already returns it on `org_dashboard.go:107`; frontend type just needs the field.

### Phase 4 — Member quick-actions

**Add:**
- `src/components/org/member-row-actions.tsx` — client component. 3-dot menu with Update Status + Remove. Each opens a small confirmation dialog. (Role updates deferred — see §"4. Quick-actions menu" above.)
  - **Self-suspend guard** (self-review NIT #3, GLM, DeepSeek): the status modal MUST disable the `suspended` option when `member.userId === identity.userId`. Backend has no self-action protection (`orgs.go:350-408`), so the UI is the only gate. Mirror the existing self-Remove guard pattern.

**Modify:**
- `src/components/org/teachers-list.tsx` (and the analogous `students-list.tsx`) — extend `OrgMemberRow` type to include `membershipId: string` (DeepSeek post-impl finding — frontend type must match the new backend field). Add a status badge column rendering the row's `status` (active / pending / suspended) so non-active members are visible.
- `src/app/(portal)/org/teachers/page.tsx` and `src/app/(portal)/org/students/page.tsx` — add the actions column to each row. Implementation must account for Phase 1's `+ Invite teacher` / `+ Invite student` button additions in the page header (DeepSeek collision flag).
- **Backend response shape change required** (Codex pass-1 important #1, DeepSeek): the existing `/api/org/teachers` + `/api/org/students` dashboard responses (`platform/internal/handlers/org_dashboard.go:115-145`) return `userId, name, email, role, joinedAt` — no membership `id`. Phase 4 actions need the membership id to call `PATCH/DELETE /api/orgs/{orgId}/members/{memberId}`. Going with **Option A**: extend the dashboard response to include `membershipId`. Single backend change in `org_dashboard.go:148-153`; UI consumes directly.
- **Backend visibility fix required** (GLM new finding): `listMembersByRole` at `platform/internal/handlers/org_dashboard.go:147` filters `m.Status == "active"`. Once an admin suspends a member, that member vanishes from the list and is unreachable from the quick-actions menu — suspension becomes one-way from the UI. **Relax the filter to include all statuses** (`active`, `pending`, `suspended`); the new status badge column makes the state visible. Without this, suspension is effectively irreversible.

### Phase 5 (optional) — Domain hint on invite

**Modify:**
- `src/components/org/invite-member-modal.tsx` — fetch the org's domain (already on `/api/orgs/{id}` payload); show confirmation when invitee email's domain mismatches.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| 404-on-AddMember UX is awkward (have to ask invitee to register first) | medium | Surface the registration link prominently, with copy-to-clipboard. Document the limitation in the form's help text. v2 / plan 069b adds token invites. |
| Class archive is irreversible from this UI | medium | Confirmation dialog is required. Backend's archive is reversible (`classes.is_archived` is a column flip), but UI doesn't expose un-archive in v1. Add an "Archived classes" filter on `/org/classes` if needed. |
| Settings edit could orphan org if email is invalid | low | zod validation client-side + backend re-validation. The org's `contact_email` field is informational; not used for sign-in. |
| Org admin removes themselves from their own org | medium | API doesn't currently prevent this. Add UI-side guard: compare the row's `userId` field to `identity.userId` (NOT membership `id` — Codex pass-2 caught the original guard compared the wrong fields). If equal, disable the Remove button with tooltip "Use the org transfer flow to leave an org." (No transfer flow exists; treat this as documentation that the path is blocked at v1.) |
| Cross-org member adds (teacher already in another org) | low | Backend allows; UI doesn't need to special-case. Role list shows both org memberships independently. |
| Inviting an existing user to an org they're already in | low | Backend should be checked — if it returns 409, UI maps to "Already a member of this org." |

## Phases

### Phase 0 — Pre-impl Codex review

Per CLAUDE.md plan-review gate. Dispatch `codex:codex-rescue` to review against:
- `platform/internal/handlers/orgs.go` (the endpoints the UI consumes)
- `platform/internal/handlers/classes.go` (class drill-down endpoints)
- `src/app/(portal)/org/teachers/page.tsx`, `students/page.tsx`, `classes/page.tsx`, `settings/page.tsx` (current read-only surface)
- `src/app/api/auth/register/route.ts` and `src/app/(portal)/onboarding/page.tsx` (the registration flow that the invite-not-found case links to)

Specific questions:
1. `POST /api/orgs/{id}/members` — does it return 409 when the user is already a member of the org, or does it idempotently re-add? Need to know to size the UI error handling.
2. Is there an `updated_by` column on `organizations` today? (For the "Last updated by {y}" surface.) If not, this plan should not promise the "by" half.
3. The "registration link with role intent" pattern — confirm it's `?intendedRole=teacher` and that the `/register` form picks it up. Plan 043 phase 5 added the cookie path; what's the URL surface?
4. Class archive (`PATCH /api/classes/{id}` per `classes.go:27`) — what's the request body shape? `{ archived: true }`? Need to confirm before writing the button's POST body.
5. `GET /api/classes/{id}/members` exists at `classes.go:30`; what's the role discrimination in the response (is the instructor distinguished from students)? Drives the roster-table column layout.
6. Are there any existing form-component patterns in the org portal to mirror, or is this the first write surface?

### Phase 1 — Invite forms (PR 1)

- Implement `<InviteMemberModal>`.
- Wire into both pages.
- Smoke test: invite an existing teacher → success; invite a non-existent email → 404 with reg link.
- Codex post-impl review.
- PR + merge.

### Phase 2 — Class drill-down (PR 2)

- Implement detail page + Archive button.
- Wrap rows on `/org/classes` in links.
- Smoke test: drill into a class, verify roster, archive, confirm removed from active list.
- Codex post-impl review.
- PR + merge.

### Phase 3 — Settings edit (PR 3)

- Replace placeholder with form.
- Smoke test: change name, save, refresh — change persists.
- Codex post-impl review.
- PR + merge.

### Phase 4 — Member quick-actions (PR 4)

- Implement actions menu component.
- Wire into both pages.
- Smoke test: change a teacher's status (active → suspended → active), verify; remove a student, verify.
- Codex post-impl review.
- PR + merge.

### Phase 5 (optional) — Domain hint (PR 5)

- Add domain check to invite modal.
- Smoke test: invite a same-domain email (no warning); invite a different-domain email (confirmation dialog).

## Post-execution report

**Status**: Phases 1-4 shipped. Phase 5 (optional domain hint) deferred — plan flagged it as deferrable polish, not core scope.

**Single-PR-deviation**: Phase 1 shipped as standalone PR #128 under the OLD per-phase pattern (before the single-PR-per-plan policy landed in #129). Phases 2-4 ship together as a single PR `Plan 069 phases 2-4` per the new policy. Plan 069 is the transitional split.

**Phase 1 (PR #128, commit 48a7db5)** — invite forms
- New: `src/components/org/invite-member-modal.tsx` (mirrored plan 070's `create-parent-link-modal.tsx` pattern), `invite-member-button.tsx`
- Modified: `/org/teachers/page.tsx`, `/org/students/page.tsx`
- Backend POST shape verified `{email, role}` against `orgs.go:301-327`
- 4-way code review found 1 BLOCKER (clipboard guard for HTTP/insecure contexts) + 1 NIT (label case) — both fixed, Codex round-2 confirmed

**Phase 2 (commit cb2450a)** — class drill-down + archive
- New: `src/app/(portal)/org/classes/[classId]/page.tsx`, `archive-class-button.tsx`
- Modified: `src/components/org/classes-list.tsx` (clickable rows preserving `?orgId=`), `src/app/(portal)/org/classes/page.tsx`
- Backend: extended `GET /api/classes/{id}` with inline-join to surface `courseTitle` (avoids the 403 the plan-pass-2 caught — `/api/courses/{id}` is gated to creator/class-member). Added `Class.CourseTitle` field with `,omitempty` so 9-column scans elsewhere stay backward-compat.
- "Open in teacher portal" link only renders when caller is instructor/TA in the class (self-review NIT #4 fold)
- Test: `TestGetClass_IncludesCourseTitle` regression locked

**Phase 3 (commits c851578 + f855c16 test fix)** — settings edit form
- New: `src/components/org/org-settings-form.tsx` — client component with `name`/`contactEmail`/`contactName`/`domain` editable + read-only Type/Status/Verified rows
- Modified: `org-settings-card.tsx` (delegates happy-path to form; added `updatedAt` to type per DeepSeek finding)
- PATCH body strategy: always-send all four fields. Backend handler uses `*string` pointer fields with `omitempty` — empty string is the supported clear semantic for `domain`.
- Test fix: `org-list-views.test.tsx` mocks `next/navigation` (form needs router context); switched to `getByDisplayValue` for the now-editable fields

**Phase 4 (commit 83b50c8)** — member quick-actions
- New: `src/components/org/member-row-actions.tsx` (3-dot menu with Update Status + Remove modals)
- Modified: `teachers-list.tsx`, `students-list.tsx` (extended `OrgMemberRow` with `membershipId` + `status`; added Status badge column + Actions column)
- Modified: `/org/teachers/page.tsx`, `/org/students/page.tsx` (fetch identity, pass `currentUserId` for self-action gating)
- Backend: added `MembershipID` + `Status` to `orgMemberSummary`; relaxed `listMembersByRole` filter from `role && status='active'` to `role` only (GLM finding — suspension was one-way otherwise)
- Self-action guards: Suspend disabled when `member.userId === identity.userId`; Remove disabled with tooltip "Use the org transfer flow to leave an org" (no transfer flow exists in v1; documents the path is blocked)
- Tests: 2 new Go integration tests + 15 new unit tests on `MemberRowActions` covering the self-action guards

**Phase 5 (deferred)** — optional domain hint on invite. Not implemented; plan flagged it as deferrable polish.

**Cross-phase verification**: full Go test suite passes; `tsc --noEmit` baseline of 10 unrelated errors maintained (zero new); `eslint` clean for all modified files; vitest covers Phases 2-4 (27/27 in `org-list-views.test.tsx` + `member-row-actions.test.tsx`).

## Code Review (consolidated PR for phases 2-4)

### DeepSeek V4 Flash — APPROVED, 2 advisory notes

- Phase 2 GetClass + `,omitempty`: SAFE (`COALESCE` always populates the field; `omitempty` only affects struct-literal encoding).
- Phase 3 always-send-all-four-fields: SOUND (handler uses `*string` pointer fields with `omitempty` Go struct tags; client safely sends all four).
- Phase 4 self-action backend gap: confirmed via `orgs.go:403, 451` — `UpdateMemberStatus` and `RemoveMember` only check the `org_admin` role, never compare `membership.UserID` against `claims.UserID`. UI is the only gate. Acceptable for v1; track as a known gap.
- Phase 4 filter relaxation: intentional, covered by `TestOrgList_SuspendedMemberVisible`. No other consumers of the old behavior.
- Phase 4 + Phase 1 collision: additive, no conflict.
- ArchiveClassButton empty-body PATCH: functionally fine (handler doesn't read body); semantically unusual but documented.
- GetClass INNER JOIN returning `(nil, nil)` for orphaned classes: actually MORE correct (handler maps nil → 404).

### GLM 5.1 — needs-attention → 1 BLOCKER REJECTED + 1 finding REJECTED + 1 nit FIXED

GLM raised 7 findings; on close inspection most are wrong-premise or already-known.

**F1 REJECTED (claimed shipping defect)** — GLM said `ArchiveClassButton`'s empty-body PATCH would silently no-op because "the PATCH handler uses `*string` pointer fields — all nil on empty body means nothing changes". Verified false against `platform/internal/handlers/classes.go:206-235`: `ArchiveClass` does NOT decode any request body — it directly calls `s.ArchiveClass(ctx, id)` which runs `UPDATE classes SET status = 'archived'` unconditionally. The empty-body PATCH works correctly. Plan even verified this in Codex pass-1. GLM confused this handler with a different one. No change required.

**F2 REJECTED (LEFT JOIN suggestion)** — GLM suggested switching GetClass to LEFT JOIN to handle "soft-deleted course or dangling course_id". Verified against `drizzle/0004_course-hierarchy.sql:46`: `class_memberships.course_id` is `NOT NULL REFERENCES courses(id) ON DELETE CASCADE`. A deleted course cascades to the class row itself — dangling course_id is impossible. INNER JOIN is correct (also matches DeepSeek's framing as "more correct"). No change required.

**F3 ACKNOWLEDGED (self-action backend gap)** — Same as DeepSeek; UI-only guard accepted for v1.

**F4-F5 OK** — same as DeepSeek.

**F6 ACKNOWLEDGED (modal inconsistency)** — Phase 2's `ArchiveClassButton` uses `window.confirm()`; Phase 4's `MemberRowActions` hand-rolls `<div role="menu">` modals. Real cosmetic divergence, but `Dialog` component doesn't exist in the codebase yet (would be a separate refactor scope). Acceptable for v1; track for a future polish pass that introduces a shared `<Dialog>`.

**F7 FIXED — StatusBadge duplication**: identical `StatusBadge` was defined in both `teachers-list.tsx` and `students-list.tsx`. Extracted to `src/components/org/member-status-badge.tsx` and imported by both. DRY restored.

## Plan Review

This plan predates the new 4-way review policy (CLAUDE.md commit 3e7397b). Codex passes 1-3 are preserved below as the Codex slot of the 4-way; self-review (Opus 4.7) + DeepSeek V4 Pro + GLM 5.1 added before any implementation.

### 4-way convergence

All four reviewers concur with changes (no blockers). Consolidated required updates folded into the §Files sections below:

- **Phase 1**: mirror plan 070's `create-parent-link-modal.tsx` modal pattern (backdrop dismissal, Escape key, ARIA, focus management)
- **Phase 2**: "Open in teacher portal" link only renders when caller has instructor/TA role in that class (compute `myRole` from class members fetch)
- **Phase 3**: extend frontend `OrgSettingsData` type at `org-settings-card.tsx:5-14` to include `updatedAt`
- **Phase 4**: status modal MUST disable `suspended` when row's `userId === identity.userId` (self-suspend guard, parallel to existing self-remove guard); relax `listMembersByRole` status filter to surface suspended/pending members and add a status badge column (otherwise suspension is one-way from UI); frontend `OrgMemberRow` type at `teachers-list.tsx:3-9` adds `membershipId`
- **Phase 1 ↔ Phase 4 same-file collision**: both modify `teachers/page.tsx` + `students/page.tsx`. Sequential PRs handle it, but Phase 4 implementation must account for Phase 1's prior additions



### DeepSeek V4 Pro — CONCUR-WITH-CHANGES (3 small gaps)

DeepSeek V4 Pro confirmed: AddMember shape; phase ordering safe; Option A correct on `membershipId` (also flagged that the frontend `OrgMemberRow` type at `teachers-list.tsx:3-9` needs the new field added too); `updated_at`-only audit correct (no `updated_by` column, would need a migration); no self-action paths beyond Remove + Suspend.

Three gaps to fold:

1. **Self-suspend guard** (also self-review NIT #3 + GLM #1): promote from NIT to must-ship. Backend has no self-action protection (`orgs.go:350-408` only checks `org_admin` role, not caller-vs-member). Phase 4's status modal MUST disable `suspended` when `member.userId === identity.userId`.
2. **OrgSettingsData type missing `updatedAt`**: backend ships it (`store/orgs.go:26`, returned by `org_dashboard.go:107`), but frontend type at `org-settings-card.tsx:5-14` omits it. Phase 3's "Last updated {x}" display needs the type field added.
3. **Phase 1 / Phase 4 same-file collision**: both phases modify `teachers/page.tsx` + `students/page.tsx`. Sequential PRs handle this fine, but the plan should flag that Phase 4 implementation must account for Phase 1's prior additions (invite button placement). Cosmetic, not a blocker.

### GLM 5.1 — CONCUR-WITH-CHANGES (3 changes; 1 new substantive finding)

GLM 5.1 confirmed: phase ordering safe; Option A on the membershipId response extension is correct; v1 invite UX (register-link fallback) is acceptable; `updated_at`-only audit is right; AddMember accepts `{email, role}` with internal lookup + 404 (matches our own verification).

Changes required before implementation:

1. **Self-suspend guard on Phase 4** (matches self-review NIT #3) — status modal must disable `suspended` when `member.userId === identity.userId`.
2. **NEW SUBSTANTIVE FINDING**: `listMembersByRole` (`platform/internal/handlers/org_dashboard.go:147`) filters `m.Status == "active"`. Once an admin suspends a member, the member vanishes from the teachers/students list and becomes unreachable for re-activation via the quick-actions menu. **Without this fix, suspension is effectively irreversible from the UI.** Add to Phase 4: either (a) relax the filter to include non-active members with a status badge column, or (b) add a "Suspended" tab/filter on `/org/teachers` and `/org/students`. Going with (a) — single-list-with-badge is the simpler UX and avoids tab plumbing for a low-frequency state.
3. **Mirror plan 070 modal pattern** (matches self-review NIT #1).

### Self-review (Opus 4.7) — 4 NITS, no blockers

1. **Pattern reuse**: Plan 070's `src/components/org/create-parent-link-modal.tsx` is a fresh precedent for Phase 1's invite modal (backdrop click-to-close, Escape key, ARIA combobox semantics, focus management — all polished in #127). Phase 1 should mirror that file's structure rather than invent a new modal pattern.
2. **Backend assumption to verify before Phase 1** [VERIFIED]: §1 says POST body is `{ email, role }`. Confirmed against `platform/internal/handlers/orgs.go:301-327` — `AddMember` reads `{Email, Role}` from the body, calls `GetUserByEmail`, returns 404 with "User not found" when absent. Plan's modal flow as written is correct; no 2-step lookup needed.
3. **Self-suspend gap (Phase 4)**: Risks-table mitigation guards self-Remove via `member.userId === identity.userId`. The same guard is needed on the Update Status path — an org admin could suspend themselves and lose access. Add to Phase 4 §Files: the status modal disables `suspended` when the row is the caller.
4. **"Open in teacher portal" link (Phase 2)**: Should only render when the caller is an instructor/TA in that class, not just any org_admin. The class members fetch already returns role per member; the page can compute `myRole = members.find(m => m.userId === identity.userId)?.role`. Add to Phase 2 §Files.

### Codex Review of This Plan

### Pass 3 — 2026-05-04: stale role-change + updated_by text scrubbed

Codex pass-3 confirmed the courseTitle backend fix and self-remove guard fix are sound but flagged remaining stale text. Final scrub:
- §Problem table row "Update teacher/student's ROLE within the org … deferred to plan 069b" deleted entirely (the column now lists status-only).
- Decisions §3 simplified to "Role changes are out of scope for v1." with no 069b reference.
- Decisions §6 simplified to "Settings audit surface is `updated_at` only" with no `updated_by` migration discussion.

### Pass 2 — 2026-05-03: BLOCKED → 1 auth blocker + 3 cleanup items folded

Codex pass-2 confirmed pass-1 fixes are clean. Two new findings folded:

1. **Course-title auth gap (BLOCKER)** — original Phase 2 said "make a second `GET /api/courses/{courseId}` fetch", but that endpoint requires the caller to be the creator, platform admin, or class member. An org admin not enrolled would 403. Resolved: Phase 2 now extends `GET /api/classes/{id}` to include `courseTitle` via inline join — single backend change, accessible to any caller authorized for the class.

2. **Self-remove guard compared wrong field** — original mitigation checked `memberId === currentUserId`, but membership row has separate `id` and `userId` fields. Resolved: Risks-table mitigation now compares `member.userId === identity.userId`.

3. **Stale role-change language** — Problem table row, Decision §3, smoke-test language all updated to status-only.

4. **Stale `updated_by` references** — Decision §6 cleaned up; settings UI is `updated_at` only (no "by").

### Pass 1 — 2026-05-03: BLOCKED → 2 blockers + 3 important folded in

Codex pass-1 returned BLOCKED with two blockers, both addressed:

1. **Member role-update endpoint doesn't exist** — `PATCH /api/orgs/{orgId}/members/{memberId}` accepts `{status}` (active/pending/suspended), not `{role}`. Resolved: Phase 4 quick-actions menu now offers Update Status + Remove only. Role changes deferred to plan 069b which would add the missing endpoint.
2. **`?intendedRole=` URL param not read by register page** — only `?invite=` is parsed today. Resolved: invite-not-found copy uses plain `/register` link instead of `/register?intendedRole=teacher`. The invitee picks their role on the registration form. Plan 069b can add role-prefill if the register page learns to read it.

Important non-blocking, all folded:

3. **Member-list dashboard responses don't include membership id** — needed for PATCH/DELETE actions. Phase 4 includes a small backend response extension to add `membershipId`.
4. **No `updated_by` column on `organizations`** — settings UI shows only `updated_at`, drops the "by {y}" half.
5. **`GET /api/classes/{id}` doesn't include `courseTitle`** — Phase 2 detail page makes a second `GET /api/courses/{courseId}` fetch.

CONFIRMED by Codex (no changes):
- Existing-user invite by email returns 404 when user doesn't exist.
- Settings update accepts partial `{name, contactEmail, contactName, domain}`.
- Org admins can archive classes via `RequireClassAuthority(..., AccessMutate)`.
- `PATCH /api/classes/{id}` takes NO request body (empty PATCH archives).
- `GET /api/classes/{id}/members` returns role per member with values `instructor|ta|student|observer|guest|parent`.
- Existing client-component pattern to mirror: `src/components/teacher/create-class-form.tsx`.

Verdict: **BLOCKED → all blockers resolved → ready for Phase 1** pending pass-2.
