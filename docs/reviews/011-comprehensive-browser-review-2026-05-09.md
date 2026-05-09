# 011 - Comprehensive Browser + Repository Review

**Date:** 2026-05-09  
**Target:** `http://localhost:3003` through the in-app browser and SSH tunnel  
**Repository:** local `main` at `/Users/chris/Workshop/bridge`  
**Reviewer stance:** fresh pass as platform admin, organization admin, teacher, student, and parent. Prior review conclusions were not assumed.

## Summary

The main role portals are much more usable than earlier cycles. Admin, teacher, org-admin, student, and parent shells all render; protected deep links preserve `callbackUrl`; teacher live-session entry no longer 404s; student class pages now organize syllabus focus areas with linked Units as teaching material; org-admin can create a parent-child link in the browser; and parent dashboards can show a linked child with live-watch entry points.

The biggest release risk is not a single page bug: the browser target and the local repository appear to be out of sync. The SSH-tunneled app exposes an `/org/parent-links` workflow and sidebar item, but the checked-out repository does not contain that route or nav item. That makes this review inherently two-layered: browser findings describe the running product, while source findings describe local `main`. The next cycle should first make the deploy/test revision explicit, then fix live collaboration configuration and cross-portal admin navigation.

## Accounts And Routes Exercised

- Platform admin: `admin@e2e.test`; `/admin`, `/admin/orgs`, `/admin/users`, `/admin/units`, admin unit edit link target.
- Organization admin: `frank@demo.edu`; `/org`, `/org/teachers`, `/org/students`, `/org/courses`, `/org/classes`, `/org/units`, `/org/settings`, `/org/parent-links`; created a Diana Parent -> Alice Student parent link through the UI.
- Teacher: `eve@demo.edu`; `/teacher`, `/teacher/units`, `/teacher/units/new`, representative unit detail/edit links, `/teacher/problems`, `/teacher/problems/new`, `/teacher/sessions`, `/teacher/sessions/{id}`, `/teacher/courses`, representative course/focus-area routes, `/teacher/classes`, representative class detail, `/teacher/schedule`, `/teacher/reports`.
- Student: `alice@demo.edu`; `/student`, `/student/classes`, representative class detail, live session route, `/student/code`, `/student/help`, malformed problem URL.
- Parent: `diana@demo.edu`; `/parent`, `/parent/children/{aliceId}`, `/parent/children/{aliceId}/live`, `/parent/reports`.
- Auth/public: `/login`, `/register`, unauthenticated deep link to a teacher class route.

## Findings

### [P0] Browser target and local `main` are not the same product revision

The in-app browser shows an org-admin sidebar item for **Parent links** and a working `/org/parent-links` page. I used it to create a parent link between Diana Parent and Alice Student, after which Diana's parent dashboard showed Alice and a `Watch Live` link. Local `main`, however, has no `src/app/(portal)/org/parent-links/page.tsx`, and `src/lib/portal/nav-config.ts:21-29` lists org-admin nav items without Parent links.

This is a release-process blocker for the review itself. A developer following this report from local `main` cannot reproduce or patch the exact browser workflow I tested. It also makes route inventory, source references, and plan-gap analysis unreliable.

Recommended fix:

- Add a build/revision endpoint or footer, for example `/api/build-info`, exposing git SHA, branch, build time, and feature flags.
- Make the SSH-tunneled review server deploy from the same pushed `main` revision being reviewed, or document the remote working tree/branch in the report handoff.
- Do not start the next implementation cycle from browser-only observations until the tested deploy SHA is known.

### [P0] Live collaboration cannot be accepted because realtime token minting is not configured

Teacher, student, and parent live views now render useful shells, but all live collaboration paths show the same blocking alert: `Live collaboration is unavailable`, with `HOCUSPOCUS_TOKEN_SECRET` called out. I saw this on `/teacher/sessions/11665fc0-85df-4fad-ab90-4bc2438736a5`, `/student/sessions/...`, and `/parent/children/{aliceId}/live`. The browser console also logged repeated `Realtime tokens not configured (HOCUSPOCUS_TOKEN_SECRET unset)` failures.

Relevant source: `src/lib/realtime/get-token.ts:65-83` maps `/api/realtime/token` HTTP 503 to the missing-secret state. `docs/setup.md` documents the same secret requirement for both Go and Hocuspocus.

Impact:

- Teacher watch mode, student live coding, parent read-only live watch, and unit-editor realtime behavior cannot be release-accepted in this environment.
- The current UI fallback is good, but the core classroom value is still disabled.

Recommended fix:

- Set the same `HOCUSPOCUS_TOKEN_SECRET` on the Go API and Hocuspocus processes in the remote environment.
- Add a visible operator health check covering Go API, Hocuspocus, realtime token minting, and Bridge session auth flags.
- Add a smoke test that opens one teacher live page and one student live page and fails if token minting returns 503.

### [P1] Platform-admin unit links route into the teacher editor and bounce the admin away

As platform admin, `/admin/units` renders unit titles as links to `/teacher/units/{id}/edit`. Opening one of those links redirected me back to `/admin`, so the admin table advertises a detail/edit action that the admin cannot use.

Relevant source: `src/app/(portal)/admin/units/page.tsx:154-159`.

Impact:

- Platform admins can list platform library units but cannot inspect, edit, publish, or audit them from the admin portal.
- The link target crosses portal boundaries and relies on teacher authorization semantics.

Recommended fix:

- Add `/admin/units/{id}` and optionally `/admin/units/{id}/edit`, with platform-admin authorization and a clear read/edit distinction.
- Until that exists, render admin unit titles as plain text or link to a read-only admin detail route only.
- Keep org-admin read-only unit behavior separate from platform-admin library governance.

### [P1] Parent-report surfaces are stale now that parent linking exists

The browser route `/org/parent-links` can now create a parent-child link, and the parent dashboard can show the linked child. But report routes still communicate old or placeholder states. `/parent/reports` says `AI-generated progress reports coming soon`, and `src/app/(portal)/parent/children/[id]/reports/page.tsx:73-80` says reports are blocked because parent-child account linking is still being built.

Impact:

- Parents who were successfully linked still cannot discover a useful progress-report workflow.
- Product copy contradicts the current parent-linking capability.

Recommended fix:

- Update report copy to reflect the current blocker accurately, for example "Progress reports are not generated yet."
- Add a `Reports` affordance on the child profile only when the route has a useful read-only state.
- Define the minimum report MVP: attendance, live-session participation, recent code, completed problems, and teacher notes before AI-generated narrative.

### [P2] Focus-area numbering is duplicated in teacher and student course views

The Python 101 class/course views render focus areas like `1. 1. Print & Comments` and `2. 2. Variables & Types`. Source confirms the UI always prefixes the display index while seeded focus-area titles also include numbering.

Relevant source:

- `src/app/(portal)/teacher/courses/[id]/page.tsx:81-86`
- `src/app/(portal)/student/classes/[id]/page.tsx:132-139`

Impact:

- The syllabus hierarchy looks unpolished and makes it harder for teachers/students to trust curated content.
- It blurs whether the ordering belongs to the title or to the course agenda.

Recommended fix:

- Store order separately from title and render either the order badge or the title number, never both.
- Normalize seeded titles by removing leading numeric prefixes.
- Taxonomy recommendation: use **Course -> Focus Area -> Unit + Problems**. Focus Areas should be syllabus/agenda organizers; Units should be the canonical teaching material; Problems should be practice/assessment attached to the relevant focus area or unit.

### [P2] Teacher schedule and report routes are direct-access placeholders

Teacher primary nav no longer advertises Schedule or Reports, which is an improvement. But `/teacher/schedule` and `/teacher/reports` still render single-line `Coming soon` pages.

Relevant source:

- `src/app/(portal)/teacher/schedule/page.tsx:1`
- `src/app/(portal)/teacher/reports/page.tsx:1`

Impact:

- Bookmarked or shared links still lead to dead-end surfaces.
- The routes imply product scope that does not yet exist.

Recommended fix:

- Either remove/redirect these routes until the workflow is ready, or ship minimal read-only MVPs.
- Schedule MVP: upcoming/live/ended sessions with create/resume affordances.
- Reports MVP: class-level attendance, session participation, problem completion, and recent work summaries.

### [P2] Parent linking UI works, but the child picker needs a more obvious interaction pattern

The Add Parent Link dialog uses a combobox for the child field. A direct click/create attempt produced `Pick a child from the autocomplete suggestions`; typing `Alice` exposed the suggestion and then the link creation succeeded. The behavior is functional, but the first-use path is easy to miss because there is no placeholder or visible default list explaining that the user must search by child name/email.

Impact:

- School admins may interpret the empty child field as broken, especially when creating the first parent link.
- This is a high-leverage workflow because it is the gateway to the parent portal.

Recommended fix:

- Add a placeholder like `Search student name or email`.
- Open the suggestion list on focus for small student sets.
- Disable `Create link` until both a parent email and a selected child object are present.

## Positive Observations

- Deep-link preservation worked: unauthenticated access to a teacher class redirected to `/login?callbackUrl=...`.
- Teacher dashboard, class detail, sessions list, and live-session entry are coherent and no longer hit the previous 404 path.
- Ended teacher sessions are no longer linked into the live control room.
- Student class pages now expose Focus Areas with linked Units as materials, matching the desired taxonomy direction.
- Org-admin core read-only pages for teachers, students, courses, classes, units, and settings render useful information.
- Parent dashboard, child profile, and parent live-watch routes render once a parent link exists.
- Malformed teacher create-class and student problem URLs now fail with stable 404s instead of broken shells.

## Suggested Next Cycle

1. Pin the review server to a known git SHA and expose build info in-app.
2. Configure realtime secrets and run a live teacher/student/parent smoke test.
3. Build or remove the platform-admin unit detail/edit path.
4. Finish parent reports as a minimal non-AI progress surface, or hide report routes until they are real.
5. Clean up Focus Area numbering and codify the Course -> Focus Area -> Unit + Problems taxonomy in UI copy, seed data, and tests.
6. Decide whether Schedule/Reports are near-term teacher features; if not, remove direct placeholder routes.
