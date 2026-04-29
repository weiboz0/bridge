# Plan 047 — Review 006 Fixes (P0 + P1)

## Status

- **Date:** 2026-04-28
- **Branch:** `feat/047-review-006-fixes`
- **Source:** `docs/reviews/006-comprehensive-browser-review-2026-04-28.md`
- **Goal:** Address review 006's 2 P0s and 2 P1s. P2s (topic-editor error state, My Work navigability) are deferred — see "Out of scope."

## Sources + scope

Review 006 was the first browser-driven review since plans 044-046 landed. The reviewer could not complete a normal walkthrough because Next.js refused to compile the middleware (`/login` and every portal page returned an error in dev), so visual UX checks were limited. API spot-checks via the in-app browser still surfaced two security gaps and confirmed the taxonomy cutover from plans 044-045 is rendering correctly in shape.

This plan ships the four review-006 findings ranked P0 + P1:

1. **P0** — Middleware matcher import isn't statically analyzable; Next.js can't load any portal page or `/login`. Blocks all browser QA.
2. **P0** — `/api/parent/children/{childId}/reports` accepts any authenticated caller for any `childId`. Real authorization gap.
3. **P1** — Go `/api/auth/register` discards `intendedRole`; teacher onboarding then redirects to a non-existent `/register-org` route.
4. **P1** — A session can be started with focus areas that have no linked teaching unit, so the teacher walks into class with no material on screen.

## Non-Goals

- **Build `/register-org` UI form.** Real org-onboarding form (name + slug + type + contact + auto-add caller as org_admin) is meaningful UI work and product copy. Plan 048 territory. For 047, replace the broken redirect with truthful pre-production copy ("Have your school admin invite you").
- **Build `parent_links` schema + auth.** Parent-to-child linking is its own product feature: schema, invitation flow, admin UI. Plan 049 territory. For 047, disable the parent reports endpoints and hide the parent reports route in the UI until that plan ships.
- **Backfill seeded demo courses with linked Units.** Out of scope; the workflow guard (P1 #4) makes the missing link visible to the teacher before they start, which is sufficient for pre-production.
- **My Work navigability** (review 006 P2). Already filed as the originally-deferred plan 048.
- **Topic/focus-area editor error states** (review 006 P2). Filed as plan 048 territory; the page is already changing to "Edit Focus Area" in the Topic-rename plan, so error-state UX should be redesigned alongside.

## Phases

### Phase 1: P0 — Middleware matcher inline

**Files:**

- Modify: `src/middleware.ts` — replace the `import { middlewareMatcher }` + `config = { matcher: middlewareMatcher }` indirection with the literal matcher array inline. Next.js/Turbopack requires the `matcher` config field to be statically parseable in the middleware file itself; importing a constant from another module fails the static-analysis check.

**Approach:**

```ts
// src/middleware.ts
export { auth as middleware } from "@/lib/auth";

export const config = {
  matcher: [
    "/api/orgs/:path*",
    "/api/admin/:path*",
    "/teacher/:path*",
    "/student/:path*",
    "/parent/:path*",
    "/org/:path*",
    "/admin/:path*",
  ],
};
```

`src/lib/portal/middleware-matcher.ts` stays in place because the existing unit test (`tests/unit/middleware-matcher.test.ts` — verify) still imports it. The two need to stay in sync; we add a new test that asserts the inline array in `middleware.ts` matches the exported `middlewareMatcher` to prevent drift.

**Files:**

- Modify: `src/middleware.ts` (above).
- Test: `tests/unit/middleware-matcher.test.ts` — add a parity assertion: the inline matcher in `middleware.ts` (read via `import { config } from "@/middleware"`) must equal `middlewareMatcher`.

**Verification:**

- `node_modules/.bin/tsc --noEmit` clean.
- Start the Next dev server (`PORT=3003 bun run dev`); GET `/login` returns 200 (form HTML), not 302 to itself, not 500.
- Manual browser smoke: load `/login`, submit a credentials form, navigate to `/teacher`, observe portal renders.
- Plan 045's e2e (`e2e/unit-picker.spec.ts`) becomes runnable; we kick it off as part of Phase 5 verification, no longer blocked by the redirect loop.

### Phase 2: P0 — Disable parent reports until parent-child linking exists

**Files:**

- Modify: `platform/internal/handlers/parent.go` — short-circuit `ListReports` and `CreateReport` to return `503 Service Unavailable` with body `{"error": "Parent reports require parent-child linking, scheduled for plan 049"}`. Keep the route registered so the Next-side fetch still gets a structured response (avoids 404 spam in the UI). Drop the `TODO` comments referencing the gap; reference plan 049 in code comments instead.
- Modify: `src/app/(portal)/parent/children/[id]/reports/page.tsx` — render an explicit "Reports coming soon" empty state when the API returns 503. Stop showing the "Generate report" button when the endpoint is disabled.
- Modify: `src/lib/portal/nav-config.ts` — verify the parent nav doesn't surface a "Reports" link directly (it doesn't currently — confirmed in the file). If a parent dashboard section advertises reports, replace the link with informative copy.

**Tests:**

- `platform/internal/handlers/parent_test.go` (new) — assert `GET /api/parent/children/{any}/reports` returns 503 from any authenticated caller (including platform admin, until parent linking lands). Assert `POST` same.
- Update / delete any existing parent-handler tests that asserted 200 on the now-disabled endpoints.

**Verification:**

- Go: `cd platform && go test ./internal/handlers/ -count=1 -run TestParent`.
- Browser: navigate to `/parent/children/<child-id>/reports`, see the "coming soon" state.

### Phase 3: P1 — Go register persists `intendedRole`; drop broken `/register-org` redirect

**Files:**

- Modify: `platform/internal/handlers/auth.go::Register`:
  - Add `IntendedRole *string` to the request body (optional, defaults to nil).
  - Validate against `{"teacher", "student", nil}` (signup_intent enum values).
  - Pass through to `RegisterInput`.
- Modify: `platform/internal/store/users.go`:
  - Add `IntendedRole *string` to `RegisterInput`.
  - Insert into `users.intended_role` column (already exists per migration 0021).
- Modify: `src/app/(auth)/register/page.tsx`:
  - The form already collects role; verify it sends `intendedRole` to `/api/auth/register` (not just `role`). The Go handler will read whichever field name we standardize on — pick `intendedRole` for parity with the schema column.
- Modify: `src/app/onboarding/page.tsx`:
  - Replace the `redirect("/register-org")` for `intendedRole === "teacher"` with rendering the existing "I'm a teacher or school administrator" card with copy that says: "Your school administrator will invite you to your organization. If you ARE the school admin, contact us — org self-registration ships in plan 048."
  - Drop the `<Link href="/register-org">` button (or change the href to a `mailto:` / contact form). Pre-production stopgap.
- Modify: `src/app/api/auth/register/route.ts` — delete the now-dead TS route (Next proxies `/api/auth/register` to Go per `next.config.ts`, so this file is unreachable). Confirm with `git grep "/api/auth/register"` afterward.

**Tests:**

- `platform/internal/handlers/auth_test.go` — extend with:
  - `TestRegister_PersistsIntendedRole` (teacher + student variants).
  - `TestRegister_RejectsInvalidIntendedRole` (e.g. `"admin"` returns 400).
  - `TestRegister_NoIntendedRole_OK` (missing field → user created with NULL intended_role).
- `tests/unit/onboarding.test.ts` (new or extended) — assert the teacher branch no longer redirects to `/register-org`.

**Verification:**

- Vitest + Go suite green.
- Browser: register a new account with role=teacher via email/password, observe `/onboarding` renders the teacher card with the new copy (no redirect to `/register-org`). DB inspection: `users.intended_role = 'teacher'`.

### Phase 4: P1 — Session start guard for unlinked focus areas

**Files:**

- Modify: `platform/internal/handlers/sessions.go::CreateSession`:
  - When `body.ClassID != nil`, fetch the class's `course_id`, then list topics for that course AND their linked Units (reusing `ListUnitsByTopicIDs`).
  - If any topic in the course has `unit_id IS NULL`, allow the create but include a `warnings` array in the response: `{"unlinked_topic_ids": [...], "unlinked_topic_titles": [...]}`. Do not block.
  - Rationale for warn-not-block: a teacher may legitimately start a session with a partial syllabus and link Units mid-class. Blocking would be over-aggressive.
- Modify: `src/app/(portal)/teacher/sessions/new/page.tsx` (or wherever Start Session lives — verify with grep):
  - When the create response includes `warnings.unlinked_topic_titles`, render a confirmation Dialog: "These focus areas have no teaching unit linked: {list}. Students will see 'No material yet for this topic.' Start session anyway?"
  - Two buttons: "Start anyway" (proceed to teacher dashboard) and "Cancel" (stay on the create form so the teacher can attach Units first).
- Modify: `src/app/(portal)/teacher/courses/[id]/page.tsx` (the course-edit topics list — verify): show a small warning badge next to topics with no linked Unit, so teachers see the gap before they ever hit Start Session. Compose with plan 045's link-unit dialog.

**Tests:**

- `platform/internal/handlers/sessions_test.go` — extend with:
  - `TestCreateSession_AllTopicsLinked_NoWarnings` (200 with empty `warnings`).
  - `TestCreateSession_SomeTopicsUnlinked_ReturnsWarnings` (200 with populated `warnings.unlinked_topic_titles`).
  - `TestCreateSession_NoClassID_NoWarnings` (ad-hoc session, no class context).
- `tests/integration/sessions-create-warnings.test.tsx` (new) — Vitest + RTL: mock fetch returning warnings, assert the confirmation Dialog renders with the right titles, "Start anyway" proceeds, "Cancel" stays.

**Verification:**

- Vitest + Go suite green.
- Browser: create a class with one topic (no linked Unit), click Start Session, observe the warning Dialog with the topic title.

## Implementation Order

Strict order:

1. **Phase 1 first.** Without the middleware fix, no UI testing is possible — every other phase's verification step depends on the dev server rendering portal pages. Phase 1 is also a 5-line change with the smallest blast radius.
2. **Phase 2** next. Security gap; defensive disable is short and removes a real risk.
3. **Phase 3** next. The Go register change is small; the onboarding copy update is just text.
4. **Phase 4** last. The largest of the four; depends on Phase 1's UI verification path being unblocked.

Each phase commits separately. The first commit on `feat/047-review-006-fixes` is this plan file with the agreed-on Codex review summary embedded (per CLAUDE.md plan review gate).

## Risk Surface

- **Phase 1 — middleware change in production.** Inlining the matcher is identical at runtime; the static-analysis fix changes Next's compile path, not the middleware behavior. Low risk; the parity test catches drift between the inline literal and the existing `middlewareMatcher` export.
- **Phase 2 — disabling endpoints.** The Next-side fetch must handle 503 gracefully or the parent UI breaks more visibly. We render an explicit empty state and verify in the browser. The endpoint can be re-enabled by a follow-up plan without a schema change.
- **Phase 3 — TS route deletion.** `next.config.ts` proxies `/api/auth/register` to Go, so the TS route is dead. We grep before delete to confirm no internal imports.
- **Phase 4 — over-warning.** If too many sessions trigger the warning, teachers will dismiss it reflexively. We scope the warning to "at least one topic has no linked Unit" rather than "every topic"; if even that's noisy in practice, plan 048 can dial it back.

## Out of scope (explicit deferrals)

- **Plan 048 — `/register-org` form + `addCreatorAsOrgAdmin` flow.** Real teacher org self-onboarding.
- **Plan 049 — Parent-child linking.** `parent_links` table, invitation flow, parent admin UI, then re-enable parent reports endpoints with proper auth.
- **My Work navigability redesign** (review 006 P2 #6). Originally filed; folded into plan 048 territory.
- **Topic / focus-area editor "Edit Focus Area" rename + error states** (review 006 P2 #5). Folded into plan 048's Topic-rename phase.

## Codex Review of This Plan

(Pending — dispatch `codex:codex-rescue` per CLAUDE.md plan review gate before any implementation begins.)
