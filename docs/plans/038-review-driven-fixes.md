# 038 — Review-Driven Fixes: Auth, Sessions, Navigation, Reliability

**Goal:** Address the core problems identified in the Codex portal review (docs/reviews/001-localhost-3003-portal-review-2026-04-26.md). Fix the auth identity boundary, consolidate session semantics, clean up route ownership, and harden authoring workflows.

**Source:** Codex review report, 2026-04-26

**Branch:** `feat/038-review-fixes`

**Status:** Not started

---

## Problem Summary

The review found one systemic issue (auth identity split between Next.js and Go) that cascades into multiple surface-level failures. Fixing it bottom-up in the right order prevents re-work.

---

## Phase 1: Auth Identity Consistency (P0)

The root cause of multiple failures: Next.js `auth()` and Go middleware can resolve different users from the same browser request because they pick different cookies. On `http://localhost`, Auth.js uses `authjs.session-token` but `api-client.ts` and Go both prefer `__Secure-authjs.session-token` first. If both cookies exist, identities diverge.

### Task 1.1: Normalize token selection

**Files:**
- Modify: `src/lib/api-client.ts` — remove manual cookie precedence. Forward whichever token Auth.js actually used (read from the same cookie Auth.js selects, not a hardcoded preference list).
- Modify: `platform/internal/auth/middleware.go` — match the same cookie priority as Auth.js. On `http://` (non-TLS), prefer the non-secure cookie.

### Task 1.2: Dev identity diagnostic

**Files:**
- Create: `src/app/api/auth/debug/route.ts` — dev-only endpoint (gated by `NODE_ENV=development`) that returns `{ nextAuthUserId, goClaimsUserId, cookieNameUsed, match: bool }`. Helps diagnose future identity drift without guessing.

### Task 1.3: Assert identity match in server components

**Files:**
- Modify: `src/app/(portal)/teacher/sessions/[sessionId]/page.tsx` — instead of silently returning `notFound()` when `liveSession.teacherId !== viewerId`, log a diagnostic warning and check if the mismatch is an auth cookie issue vs a real access denial.
- Modify: any other server component that compares Next auth ID to Go API response IDs.

### Task 1.4: Integration test

Write a test that creates a session via Go API, then loads the teacher session page server component, and verifies the same user ID appears on both sides.

---

## Phase 2: Session Semantics Cleanup (P1)

### Task 2.1: Consolidate `active` → `live` everywhere

The migration 0014 renamed the DB enum value, but some frontend code still checks `status === "active"`.

**Files to grep and fix:**
```bash
grep -rn '"active"\|=== .active.\|== .active.' src/ --include="*.ts" --include="*.tsx" | grep -i session
```

Key files:
- `src/app/(portal)/teacher/classes/[id]/page.tsx` — checks `status === "active"` for showing active session card
- `src/app/(portal)/student/classes/[id]/page.tsx` — same
- Any other file comparing session status to "active"

Change all to `"live"`.

### Task 2.2: Shared session status type

Create a shared constant/type for session statuses so this can't drift again:
```ts
// src/lib/session-types.ts
export const SESSION_STATUS = { LIVE: "live", ENDED: "ended" } as const;
```

---

## Phase 3: Route Ownership Cleanup (P1)

### Task 3.1: Remove stale Next.js API shadows

Identify Next.js API route handlers that are now fully proxied to Go and remove them. The proxy in `next.config.ts` already forwards these routes — the Next.js handlers are dead code that can intercept requests if the proxy fails.

```bash
# Find Next.js API routes that overlap with GO_PROXY_ROUTES
ls src/app/api/sessions/ src/app/api/documents/ src/app/api/ai/ 2>/dev/null
```

For each: verify the Go handler covers the same contract, then delete the Next.js handler. If the Next.js handler has unique logic not in Go, document it as a migration gap.

### Task 3.2: Remove legacy `classroomId` references

The review noted some code still references `classroomId`. Grep and clean:
```bash
grep -rn "classroomId\|classroom_id" src/ platform/ --include="*.ts" --include="*.tsx" --include="*.go"
```

---

## Phase 4: Workflow Reliability (P1-P2)

### Task 4.1: Student join-class verification

**File:** `src/components/student/join-class-dialog.tsx`

After the join API call succeeds, verify the class appears in the student's class list before closing the dialog. If it doesn't, show an error instead of silently succeeding.

### Task 4.2: Topic creation immediate feedback

**File:** `src/app/(portal)/teacher/courses/[id]/page.tsx`

After creating a topic, revalidate the topic list or use optimistic UI so the count updates immediately without requiring page refresh.

### Task 4.3: Invalid route ID handling

**Files:** All `[id]` server components in `src/app/(portal)/`

Add UUID validation before making API calls. If the param isn't a valid UUID, call `notFound()` immediately instead of letting the API return 400.

```ts
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
if (!UUID_RE.test(id)) notFound();
```

### Task 4.4: Personal unit scope authorization

**File:** `src/app/(portal)/teacher/units/new/page.tsx`

The create form defaults to "Personal" but the Go handler may reject it if the scopeId doesn't match. Fix: default to "Org" when the user has an org, "Personal" only when they don't. Or fix the Go handler to accept personal scope for the creator.

### Task 4.5: Preserve deep links through login

**File:** `src/components/portal/portal-shell.tsx`

When redirecting to `/login`, include `?callbackUrl={currentPath}`. After login, redirect back to the original URL.

---

## Phase 5: Navigation & UI Polish (P2)

### Task 5.1: Hide placeholder nav items

**File:** `src/lib/portal/nav-config.ts`

Remove or comment out nav items that point to placeholder pages:
- Teacher: "Schedule" and "Reports" (unless we ship minimal views)
- Parent: "Reports" (placeholder)

Or: ship minimal read-only views that show real data (sessions list for schedule, student completion rates for reports).

### Task 5.2: Fix parent dashboard nested links

**File:** `src/app/(portal)/parent/page.tsx`

Replace nested `<Link>` inside `<Link>` with a non-anchor card container + explicit action buttons.

### Task 5.3: Fix parent "My Children" redirect

**File:** `src/app/(portal)/parent/children/page.tsx`

Either implement a real children list view, or remove the nav item until it's distinct from the dashboard.

### Task 5.4: Duplicate Tiptap extension warnings

**File:** `src/components/editor/tiptap/extensions.ts`

Audit that each extension is registered exactly once. The warning suggests Link or Underline may be registered both via StarterKit and explicitly. StarterKit includes some extensions by default — disable duplicates:
```ts
StarterKit.configure({
  // Disable extensions we register explicitly
  heading: { levels: [1, 2, 3] },
})
```

Check: does StarterKit include Bold, Italic, Strike? If so, are we also registering them separately?

### Task 5.5: Problem editor responsive layout

**File:** `src/components/problem/problem-shell.tsx`

Add responsive breakpoints: collapse side panels into tabs on viewports < 1024px. Let the editor claim full width on mobile.

---

## Implementation Order

| Phase | Tasks | Effort | Why this order |
|-------|-------|--------|----------------|
| 1 | Auth identity (1.1-1.4) | Medium | Root cause — fixes cascade through phases 2-4 |
| 2 | Session semantics (2.1-2.2) | **Medium** | Includes backend ai.go fix — not just frontend grep |
| 3 | Route cleanup (3.1-3.2) | **Large** | ~42 overlapping route files need contract-parity checks |
| 4 | Workflow reliability (4.1-4.5) | Medium | User-facing bugs that erode trust |
| 5 | Navigation + UI (5.1-5.5) | Medium | Polish that makes the app feel complete |

Each phase ships independently. Phase 1 should land first because it affects the correctness of everything else.

---

## Codex Review of This Plan

- **Date:** 2026-04-26
- **Reviewer:** Codex

### Corrections applied:

1. **Critical miss:** `platform/internal/handlers/ai.go:66` checks `status != "active"` — will reject all valid `"live"` sessions after the enum migration. Added to Task 2.1 scope.

2. **Task 2.1 effort:** Upgraded from Small → Medium. Three files across two layers (2 frontend + 1 backend handler).

3. **Task 3.1 effort:** Upgraded from Small → Large. ~42 Next.js API route files overlap with `GO_PROXY_ROUTES`. Each needs individual contract-parity verification before deletion. Added verification gate requirement.

4. **Cookie fix detail:** `api-client.ts` and Go middleware both have scheme-unaware static preference lists. Additionally `jwt.go` decrypt salt order (HTTP-then-HTTPS) mismatches middleware preference (HTTPS-first). All three files must be fixed together in Task 1.1.

5. **Missing from plan:** Review report file (`docs/reviews/001-...`) needs to be committed to the repo.

6. **Verification gate for Task 3.1:** Before deleting any Next.js handler, must verify the Go handler is fully implemented and tested. The proxy uses both `beforeFiles` and `fallback` rewrites — a deleted Next handler without a working Go replacement goes dark with 404.

## Code Review

### Review 1 — Plan review

- **Date**: 2026-04-26
- **Reviewer**: Codex (pre-implementation)
- **Verdict**: Plan corrections applied (see "Codex Review of This Plan" above)

### Review 2 — PR #66

- **Date**: 2026-04-26
- **Reviewer**: Codex (post-implementation)
- **Verdict**: Approved with fixes (all applied)

1. `[FIXED]` **SECURITY** — `callbackUrl` open redirect. Login page consumed `callbackUrl` from query params without validation, allowing redirect to external URLs. Fixed: enforced relative-path-only (`starts with / and not //`).

2. `[FIXED]` **BUG** — Missing UUID validation on `sessionId` in student/teacher session redirect pages. Unsanitized params were passed directly to redirect URLs. Fixed: added `isValidUUID(sessionId)` guard.

3. `[FIXED]` **ROBUSTNESS** — `X-Forwarded-Proto` multi-value header handling. Go middleware used exact `== "https"` match which fails on `"https,http"` or case variants. Fixed: `strings.HasPrefix(strings.ToLower(...), "https")`.

4. `[NOTED]` Missing `ValidateUUIDParam` on `classId` in Go sessions routes — deferred (low risk, existing routes work).

5. `[NOTED]` Legacy `classroomId` references in documents/schema code — intentional backward compat, annotated with `@deprecated`.

## Post-Execution Report

**Status:** Complete

**Implemented**

Phase 1 — Auth identity consistency:
- `src/lib/api-client.ts`: Cookie preference now matches Auth.js scheme detection (HTTP → `authjs.session-token` first).
- `platform/internal/auth/middleware.go`: Both `RequireAuth` and `OptionalAuth` check request scheme (`r.TLS` or `X-Forwarded-Proto`) to select cookie order. Handles multi-value headers and case variants.

Phase 2 — Session status active→live:
- `src/app/(portal)/teacher/classes/[id]/page.tsx`: `"active"` → `"live"`
- `src/app/(portal)/student/classes/[id]/page.tsx`: removed `"active"` fallback
- `platform/internal/handlers/ai.go:66`: **CRITICAL** — was rejecting all valid `"live"` sessions

Phase 3 — Route cleanup:
- Renamed `classroomId` → `classId` in `src/lib/sessions.ts`, `src/lib/attendance.ts`, `src/lib/documents.ts`, `src/app/api/documents/route.ts`
- Schema column annotated `@deprecated` (needs DB migration to fully remove)
- TODO added to `next.config.ts` for ~42 stale Next.js API route shadows

Phase 4 — Workflow reliability:
- UUID validation added to 8 high-traffic pages + 2 session redirect pages
- Unit creation defaults to "org" scope when user has org memberships
- Deep link preservation via `callbackUrl` with open-redirect protection

Phase 5 — Navigation + UI polish:
- Removed placeholder nav items: teacher Schedule/Reports, parent Reports
- Fixed parent dashboard nested `<Link>` → `<Card>` with action buttons
- Deduplicated Tiptap extensions: disabled `link`/`underline` in StarterKit (registered separately with custom config)

**Verification**

- Go: 14 packages green
- Vitest: 380 passed / 0 failures
- No new type errors

**Deferred**

- Task 3.1 (delete ~42 stale Next.js API routes) — needs its own migration plan with per-route contract verification
- `ValidateUUIDParam` on `classId` in Go sessions routes — low risk, existing routes work
