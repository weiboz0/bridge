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
- `src/components/org/invite-member-modal.tsx` — client component. Props: `{ orgId, role, onClose, onSuccess }`. Renders the form, handles submit, surfaces 404 with the registration-link copy block.

**Modify:**
- `src/app/(portal)/org/teachers/page.tsx` — add `+ Invite teacher` button (top-right of header) that opens the modal.
- `src/app/(portal)/org/students/page.tsx` — same pattern with `role="student"`.

### Phase 2 — Class drill-down

**Add:**
- `src/app/(portal)/org/classes/[classId]/page.tsx` — server component. Fetches class + members. Codex pass-1 noted `GET /api/classes/{id}` returns `courseId` but NOT `courseTitle`. Pass-2 caught a real auth gap with the original "two-fetch" approach: `GET /api/courses/{courseId}` is gated to creator/platform-admin/class-member only (`platform/internal/handlers/courses.go:141-172`). An org admin who isn't enrolled in the class would 403 on the second fetch. Resolution (option B from pass-1, now required not optional): **extend `GET /api/classes/{id}` to also return `courseTitle`**. Single backend change adds an inline join in the existing class query. Phase 2 includes the backend change explicitly.
- `src/components/org/archive-class-button.tsx` — client component. Confirmation dialog → empty-body PATCH → refresh. Codex pass-1 confirmed `PATCH /api/classes/{id}` takes NO request body — `ArchiveClass` unconditionally sets `status='archived'`. The fetch sends `{ method: 'PATCH' }` with no body.

**Modify:**
- `src/app/(portal)/org/classes/page.tsx` — wrap each class title in `<Link href={`/org/classes/${cls.id}`}>`.

### Phase 3 — Settings edit form

**Modify:**
- `src/app/(portal)/org/settings/page.tsx` — replace the "coming later" placeholder with a `<form>` (server action) that PATCHes the org. Show "Last updated {x}" derived from `updated_at`.

### Phase 4 — Member quick-actions

**Add:**
- `src/components/org/member-row-actions.tsx` — client component. 3-dot menu with Update Status + Remove. Each opens a small confirmation dialog. (Role updates deferred — see §"4. Quick-actions menu" above.)

**Modify:**
- `src/app/(portal)/org/teachers/page.tsx` and `src/app/(portal)/org/students/page.tsx` — add the actions column to each row.
- **Backend response shape change required** (Codex pass-1 important #1): the existing `/api/org/teachers` + `/api/org/students` dashboard responses (`platform/internal/handlers/org_dashboard.go:115-145`) return `userId, name, email, role, joinedAt` — no membership `id`. Phase 4 actions need the membership id to call `PATCH/DELETE /api/orgs/{orgId}/members/{memberId}`. Two options:
  - **A**: extend the dashboard response to include `membershipId`. Single backend change; UI consumes directly. Preferred.
  - **B**: switch the UI to fetch from the canonical `GET /api/orgs/{orgId}/members` endpoint (which returns the full member shape including id). Bigger UI change but no backend response shape change.
  Going with A for v1 — single small response addition. Add a Phase 0 question for Codex pass-2 to confirm the dashboard response is safe to extend.

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

## Plan Review

This plan predates the new 4-way review policy (CLAUDE.md commit 3e7397b). Codex passes 1-3 are preserved below as the Codex slot of the 4-way; self-review (Opus 4.7) + DeepSeek V4 Pro + GLM 5.1 added before any implementation.

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
