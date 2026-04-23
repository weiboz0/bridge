# Session Model Phase 4: Orphan Sessions + Generic Session Surfaces Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow teachers to create sessions without a class, expose the new queryable session list API, and add UI routes that work for both class-associated and orphan sessions.

**Architecture:** Keep `sessions.class_id` nullable and introduce canonical list/create/read flows that do not require a class route prefix. Frontend session pages move to generic teacher and student routes that can render either class-linked or orphan sessions, while the existing class pages become thin wrappers or redirectors.

**Tech Stack:** Go, Chi, PostgreSQL, Next.js App Router, React 19, testify, Vitest

**Spec:** `docs/specs/010-session-model.md`

**Branch:** `feat/010-session-model`

**Depends On:** `030a-session-model-schema.md`, `030b-session-invite-tokens.md`, `030c-session-direct-add-and-access.md`

**Unblocks:** `030e-session-scheduled-backref.md`

---

## File Structure

| File | Responsibility |
|---|---|
| `platform/internal/store/sessions.go` | Accept nullable `classId`, add queryable list filters (`classId`, `teacherId`, `status`), and return session metadata needed by generic routes. |
| `platform/internal/store/sessions_test.go` | Cover orphan-session creation, me/history listing, and cross-user isolation. |
| `platform/internal/handlers/sessions.go` | Support the new `POST /api/sessions` body and canonical `GET /api/sessions?...` list contract while keeping old class-specific endpoints as temporary aliases. |
| `platform/internal/handlers/sessions_integration_test.go` | Add API tests for orphan creation and filtered lists. |
| `src/components/teacher/start-session-button.tsx` | Support titled orphan-session creation instead of always requiring `classId`. |
| `src/app/(portal)/teacher/page.tsx` | Add "New session" / "My sessions" surfaces and stop depending exclusively on `active/{classId}`. |
| `src/app/(portal)/teacher/classes/[id]/page.tsx` | Continue supporting class-linked starts, but route through the canonical create/list APIs. |
| `src/app/(portal)/student/classes/[id]/page.tsx` | Keep showing active class sessions while consuming the canonical list response. |
| `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx` | New generic teacher session entry page. |
| `src/app/(portal)/student/sessions/[sessionId]/page.tsx` | New generic student session entry page. |
| `src/app/(portal)/teacher/classes/[id]/session/[sessionId]/dashboard/page.tsx` | Convert into a wrapper or redirect to the generic teacher session page. |
| `src/app/(portal)/student/classes/[id]/session/[sessionId]/page.tsx` | Convert into a wrapper or redirect to the generic student session page. |
| `docs/api.md` | Document the canonical session list/create filters and response shape. |
| `TODO.md` | Add any follow-up cleanup work for deprecated class-specific aliases if they remain temporarily. |

---

## Task 1: Canonical Go API

**Files:**
- Modify: `platform/internal/store/sessions.go`
- Modify: `platform/internal/store/sessions_test.go`
- Modify: `platform/internal/handlers/sessions.go`
- Modify: `platform/internal/handlers/sessions_integration_test.go`
- Modify: `docs/api.md`

- [ ] Change `POST /api/sessions` to accept `{ title, classId?, scheduledSessionId?, settings?, inviteToken?, inviteExpiresAt? }`.
- [ ] Add canonical filtered listing on `GET /api/sessions`.
- [ ] Keep `/api/sessions/by-class/{classId}` and `/api/sessions/active/{classId}` only as compatibility wrappers if the frontend still needs them during this phase; document them as deprecated if retained.
- [ ] Ensure orphan sessions can be read only by the creator, directly added participants, valid token users, or platform admins per the earlier access phases.

**Testing plan:**
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/store -run 'TestSessionStore_.*List|TestSessionStore_.*Orphan' -count=1`
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/handlers -run 'TestSessionHandler_.*List|TestSessionHandler_.*Create' -count=1`

## Task 2: Generic Frontend Session Routes

**Files:**
- Create: `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx`
- Create: `src/app/(portal)/student/sessions/[sessionId]/page.tsx`
- Modify: `src/app/(portal)/teacher/classes/[id]/session/[sessionId]/dashboard/page.tsx`
- Modify: `src/app/(portal)/student/classes/[id]/session/[sessionId]/page.tsx`

- [ ] Build generic entry pages that load the session first and branch on whether `classId` is present.
- [ ] Convert the existing class-nested pages into wrappers or redirects so old links keep working.
- [ ] Avoid duplicating dashboard logic between class-linked and orphan sessions; share the same page-level data loading where possible.

**Testing plan:**
- `node_modules/.bin/tsc --noEmit`
- `bun run test`

## Task 3: Teacher + Student Dashboard Updates

**Files:**
- Modify: `src/components/teacher/start-session-button.tsx`
- Modify: `src/app/(portal)/teacher/page.tsx`
- Modify: `src/app/(portal)/teacher/classes/[id]/page.tsx`
- Modify: `src/app/(portal)/student/classes/[id]/page.tsx`
- Modify: `TODO.md`

- [ ] Add a teacher-dashboard control for starting an orphan session with a title.
- [ ] Update dashboard and class pages to consume the canonical session list API.
- [ ] Keep the class-linked active-session card on the student class page working during the route transition.

**Testing plan:**
- `bun run test`
- `node_modules/.bin/tsc --noEmit`

## Task 4: Whole-Phase Verification

**Files:**
- Modify: `docs/plans/030d-orphan-sessions.md`

- [ ] Verify all routes still work for class-linked sessions after the generic pages land.
- [ ] Verify orphan sessions can be created, opened, ended, and re-opened from the teacher dashboard.

**Verification commands:**
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./... -count=1 -timeout 120s`
- `bun run test`
- `node_modules/.bin/tsc --noEmit`

---

## Code Review

Reviewers append findings here following `docs/code-review.md`. Author responds inline with `→ Response:` and updates status to `[FIXED]` or `[WONTFIX]`.

## Post-Execution Report

**Status:** Complete

**Implemented**

- Store: `CreateSessionInput.ClassID` is now `*string` (nullable). Added `ListSessions` with `ListSessionsFilter` (teacherID, classID, status, cursor pagination). Orphan sessions (class_id NULL) create and retrieve correctly.
- Handlers: `POST /api/sessions` accepts optional `classId`. Orphan creation authorized for any teacher or platform admin. `GET /api/sessions` returns filtered list defaulting to caller's own sessions; platform admins can query any teacherId.
- Frontend: Generic routes `/teacher/sessions/{sessionId}` and `/student/sessions/{sessionId}` load sessions independent of class context. Old class-nested routes (`/teacher/classes/{id}/session/{sessionId}/dashboard` and `/student/classes/{id}/session/{sessionId}`) redirect to generic routes. Teacher dashboard page gains orphan session support (sessions list, "Start Session" with just a title). Student session page derives editorMode from class settings when class-linked, defaults to "python" for orphan sessions.
- TeacherDashboard component updated with nullable `classId`, `returnPath` prop for correct back-navigation from orphan sessions.

**Verification**

- Go full suite: 12 packages green (handlers 50s, store 22s)
- Vitest: 269 passed / 11 skipped / 0 failures
- Fixed test fixture: org status was `pending` (default from `CreateOrg`), causing `isTeacherOrOrgAdmin` to reject all orphan session creates. Set to `active` in fixture setup.

**Deviations From Plan**

- Tasks 2 and 3 combined into one commit since the frontend changes are tightly coupled (generic routes + dashboard updates both need the teacher page and start-session-button changes).
- `start-session-button.tsx` expanded with orphan session mode (title-only creation) rather than just a classId toggle.

**Follow-Up**

- E2E test coverage for orphan session flows (create → join via token → teacher dashboard → end).
- Hocuspocus auth still uses the permissive token format (deferred since 030b).
