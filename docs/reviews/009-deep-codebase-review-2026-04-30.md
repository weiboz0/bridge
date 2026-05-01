# 009 Deep Codebase Review - 2026-04-30

## Scope

Exhaustive source review of the Bridge repository — all Go platform code, TypeScript frontend, SQL migrations, Drizzle schema, seed/import scripts, store logic, and process documentation. This is a complementary pass to review 008, covering findings not surfaced in the first review or providing deeper root-cause analysis.

Review type: static source review. No automated tests or browser flows were run.

## P0 — Critical

### P0-1: `GetTopic` returns any topic to any authenticated user with no course access check

**Files:** `platform/internal/handlers/topics.go:138-155`

`GetTopic` only verifies `claims != nil` and fetches the topic by UUID. The sister handler `ListTopics` (lines 96-135) does a full course-access chain — loads the course, checks creator or `UserHasAccessToCourse`. Topic details can leak course structure, unit links, and metadata to any authenticated user.

**Fix:** Gate with the same course-access check used in `ListTopics`. Return 404 on both not-found and not-authorized to avoid leaking course existence.

### P0-2: `scheduled_sessions` table has NO CREATE TABLE migration but is used by FKs, stores, and tests

**Files:** `platform/internal/store/schedule.go:54-291`, `platform/internal/store/schedule_test.go:32-328`, `drizzle/0014_session_model.sql:75-92`

Migration 0014 adds a foreign key `sessions.scheduled_session_id → scheduled_sessions(id)` with a `DO $$ IF EXISTS` guard. Migration 0015 drops a column from `scheduled_sessions`. The Go store performs INSERT, SELECT, UPDATE, and DELETE on this table. No `CREATE TABLE scheduled_sessions` exists anywhere in the codebase — not in any `drizzle/*.sql` file, Go code, or script. The table exists in the developer's live database but was never created by a persisted migration, so a fresh database cannot bring it up.

**Fix:** Create a `drizzle/0023_create_scheduled_sessions.sql` with the full table definition matching what `schedule.go` expects: `id uuid PK`, `class_id uuid FK`, `teacher_id uuid FK`, `title varchar(255)`, `scheduled_start timestamptz`, `scheduled_end timestamptz`, `recurrence jsonb`, `topic_ids uuid[]`, `status varchar(24)`, `created_at timestamptz`, `updated_at timestamptz`. This must be numbered BEFORE 0014, or the FK in 0014 must be dropped and re-added with a working target.

### P0-3: Go `User` struct does not select or expose `intended_role` column

**Files:** `platform/internal/store/users.go:13-21`, `platform/internal/store/users.go:37-38`, `platform/internal/store/users.go:52-56`, `drizzle/0021_user_intended_role.sql:12-13`

The `User` struct has no `IntendedRole` field. All three SELECT queries (`GetUserByID`, `GetUserByEmail`, `ListUsers`) omit `intended_role` from their column lists. Migration 0021 adds the column and the Next.js onboarding page (`onboarding/page.tsx:24-27`) reads `intendedRole` from the DB. The Go API returns users without this field, so any Go-side auth/identity endpoints cannot surface intended role.

**Fix:** Add `IntendedRole *string json:"intendedRole"` to `User` struct. Add `intended_role` to all three SELECT column lists and their `Scan` targets.

### P0-4: `RegisterUser` not in a transaction — partial failure creates orphaned, unloggable user

**Files:** `platform/internal/store/users.go:111-143`

The `users` INSERT (line 121) and `auth_providers` INSERT (line 133) run in separate implicit transactions. If the `users` INSERT succeeds but the `auth_providers` INSERT fails (unique violation, DB error), the committed user row lacks any login method permanently.

**Fix:** Wrap both INSERTs in a `db.BeginTx` transaction with rollback on any failure.

### P0-5: Drizzle `documents` schema keeps `classroom_id` column and index that migration 0012 dropped

**Files:** `src/lib/db/schema.ts:413`, `src/lib/db/schema.ts:424`, `drizzle/0012_drop_legacy_classrooms.sql:48-49`

Migration 0012 runs `ALTER TABLE documents DROP COLUMN IF EXISTS classroom_id` and drops the associated index. The Drizzle schema still defines `classroomId: uuid("classroom_id")` (with a deprecation comment) and `index("documents_classroom_idx")`. A `drizzle-kit push` against a migrated database will attempt to re-add the dropped column and index.

**Fix:** Remove `classroomId` column definition and the `documents_classroom_idx` index from the Drizzle documents table definition.

## P1 — High

### P1-1: `ArchiveClass`, `ListMembers`, `UpdateMemberRole`, `RemoveMember` lack any class authority check

**Files:** `platform/internal/handlers/classes.go:231-249`, `platform/internal/handlers/classes.go:337-350`, `platform/internal/handlers/classes.go:353-390`, `platform/internal/handlers/classes.go:393-423`

These four handlers only verify `claims != nil`. `GetClass` uses `CanAccessClass` but the mutation and roster-read endpoints bypass it entirely. Any authenticated user who knows a class or membership UUID can archive classes, enumerate members with names/roles, change member roles, or remove members.

**Fix:** Gate each handler with `CanAccessClass` (or a narrower class-staff authority check) before any mutation or roster enumeration. Return 404 on both not-found and not-authorized. Add integration tests for non-members, student members, instructors, and org_admin.

### P1-2: `DEV_SKIP_AUTH` bypasses all security with no production guard

**Files:** `platform/internal/auth/middleware.go:89-104`

When `DEV_SKIP_AUTH` is set (any non-empty value), ALL authentication is skipped and a synthetic `Dev User` claims struct is injected. `DEV_SKIP_AUTH=admin` grants full platform-admin access. There is no check for `APP_ENV == "production"` — this env-var-based bypass would grant unrestricted access if the variable leaked into staging or production.

**Fix:** Add a startup guard: if `DEV_SKIP_AUTH != "" && APP_ENV == "production"`, either panic or refuse to start. Also explicitly zero `ImpersonatedBy` in the synthetic claims struct.

### P1-3: `SessionEvents` and `GetHelpQueue` skip session authority checks

**Files:** `platform/internal/handlers/sessions.go:610-644`, `platform/internal/handlers/sessions.go:646-669`

`GetParticipants` (immediately preceding, line 590) calls `isSessionAuthority`, but `SessionEvents` and `GetHelpQueue` only verify `claims != nil`. Any authenticated user can subscribe to another session's SSE stream or read the help queue with participant names and emails.

**Fix:** Gate both endpoints with `isSessionAuthority` or equivalent session access logic before streaming or returning data. Add regression tests for cross-user/cross-session denial.

### P1-4: `UpdateOrgStatus` auto-sets `verified_at` on any school activation

**Files:** `platform/internal/store/orgs.go:250-254`

When `status == "active"` and `type == "school"`, the store unconditionally sets `verified_at = now()` on org activation, regardless of whether any actual verification occurred. This bypasses any intended verification workflow.

**Fix:** Remove the auto-verification logic or gate it behind an explicit verification step with a separate payload field.

### P1-5: Hocuspocus WebSocket auth trusts client-forged `userId:role` tokens

**Files:** `server/hocuspocus.ts:15-24`, `server/hocuspocus.ts:46-51`, `server/hocuspocus.ts:74-90`, `src/lib/yjs/use-yjs-provider.ts:48-52`, `src/lib/yjs/use-yjs-tiptap.ts:98-106`

The Hocuspocus server splits a client-provided token on `:` to extract `userId` and `role`. Session documents accept any role of `teacher`, `user`, or `parent` without verifying session membership. Broadcast documents accept any parsed token. Unit documents accept any token with `role === "teacher"` without checking org/scope membership. All client code constructs the token as `${userId}:teacher` with no signing or server validation.

**Fix:** Replace `userId:role` with a server-issued signed token. Have Hocuspocus call Go's `CanAccessSession` or a lightweight auth endpoint on WebSocket upgrade. Deny by default when authorization cannot be verified.

### P1-6: `canViewUnit` still denies all org-scoped units to students despite shipped session/unit binding

**Files:** `platform/internal/handlers/teaching_units.go:132-168`, `src/components/session/student/student-session.tsx:92-103`, `src/app/(portal)/student/units/[id]/page.tsx:67-79`

The comment on line 157 says "Students are denied until plan 032 wires class/session binding." Plans 032, 044, and 048 shipped that binding. The student live-session panel renders links to `/student/units/{unitId}`, and the student unit page calls `fetchUnit` and `fetchProjectedDocument`. Both are gated by `canViewUnit`, which denies all org-scoped units to non-admin/non-teacher users. The Python 101 demo unit path is broken for students.

**Fix:** Widen student org-unit access through a verified class/session/assignment path. Validate class membership plus a linked topic that references the unit. Add integration coverage for enrolled students opening linked org units and non-members receiving denial.

### P1-7: `GetProjectedDocument` uses raw child document, not composed overlay-merged content

**Files:** `platform/internal/handlers/teaching_units.go:833-861`, `platform/internal/handlers/teaching_units.go:1271-1305`

`GetComposedDocument` merges parent + child + overlay rows. `GetProjectedDocument` calls `GetDocument` which returns only the raw child document. A forked unit with an empty child document and an overlay row renders empty or stale content through `/projected` — the endpoint the student unit page uses.

**Fix:** Have `GetProjectedDocument` project the composed document for overlay children before role filtering. Add a test where a forked unit with no child blocks still renders parent content to students.

### P1-8: `createUnit` silently drops `materialType` from the POST body

**Files:** `src/lib/teaching-units.ts:63-82`

The `CreateUnitInput` type includes `materialType` but the `createUnit` function body does NOT include `materialType` in the `JSON.stringify` payload. Created units always receive the backend default regardless of what the caller passes.

**Fix:** Add `materialType: data.materialType ?? "notes"` to the body.

### P1-9: `schedule.go` `List` and `ListUpcoming` handlers have no authorization check

**Files:** `platform/internal/handlers/schedule.go:139-153`, `platform/internal/handlers/schedule.go:155-176`

Both handlers only verify `claims != nil`, then return schedule rows. Any authenticated user can enumerate scheduled sessions for any class. The `Create` handler (line 57) verifies teacher/org_admin status, but the read endpoints do not.

**Fix:** Add class-membership or course-access checks before returning schedule data.

### P1-10: Annotation read, delete, and resolve endpoints lack authorization

**Files:** `platform/internal/handlers/annotations.go:72-91`, `platform/internal/handlers/annotations.go:94-129`

`ListAnnotations` returns annotations for any `documentId` with just `claims != nil` — no document ownership check. `DeleteAnnotation` and `ResolveAnnotation` have no ownership or authorization check — any authenticated user can delete or resolve any annotation.

**Fix:** Load the document and verify `doc.OwnerID == claims.UserID` (or platform admin). For delete/resolve, load the annotation and verify the caller is the author or platform admin. Return 404 on denial to avoid leaking annotation existence.

### P1-11: `GetAssignment` has no authorization beyond authentication

**Files:** `platform/internal/handlers/assignments.go:147-164`

Only verifies `claims != nil`. Any authenticated user can read any assignment metadata by guessing UUIDs, potentially leaking class/course associations.

**Fix:** Gate with class membership check via `assignment.ClassID` plus platform admin bypass. Return 404 on denied access.

### P1-12: Drizzle `teaching_units` is missing three critical partial unique indexes

**Files:** `src/lib/db/schema.ts:634-641`, `drizzle/0016_teaching_units.sql:38-39`, `drizzle/0017_topic_to_units.sql:15-16`, `drizzle/0019_discovery.sql:35-37`

The SQL migrations created three indexes on `teaching_units`: `scope_slug_uniq` (0016), `topic_id_uniq` (0017), and `search_idx` (0019). The Drizzle schema only defines `scopeStatusIdx` and `createdByIdx`. Go code at `teaching_units.go:324` relies on `topic_id_uniq` for duplicate detection. A `drizzle-kit push` will drop all three missing indexes, silently breaking the 1:1 unit-topic invariant.

**Fix:** Add the three missing indexes to the Drizzle table definition.

### P1-13: `UnitCollectionHandler.AddItem` does not verify unit visibility before attaching

**Files:** `platform/internal/handlers/unit_collections.go:306-356`, `platform/internal/store/unit_collections.go:235-250`

`AddItem` validates UUID/FK existence and inserts the unit into the collection. It does not load the unit or apply unit visibility/scope rules. Cross-org, draft, personal, or otherwise unusable units can be attached to any collection by UUID, which becomes a security concern if collections become the book/template entity discussed in plan 051.

**Fix:** Load the unit before insert, require the caller can view it in the collection scope, and reject units that cannot be consumed. Add negative tests for cross-org, draft, and personal units.

## P2 — Medium

### P2-1: `docs/setup.md` OAuth redirect URI shows port 3000 instead of 3003

**Files:** `docs/setup.md:103`, `CLAUDE.md:26`, `CODEX.md:46`, `OPENCODE.md:47`

The setup guide instructs users to add `http://localhost:3000/api/auth/callback/google` as the authorized redirect URI, but the app runs on port 3003. Following this instruction produces a broken OAuth flow.

**Fix:** Change to `http://localhost:3003/api/auth/callback/google`.

### P2-2: `CodeEditor` calls `setupMonaco()` at module scope during SSR

**Files:** `src/components/editor/code-editor.tsx:11`

`setupMonaco()` runs at module import time on line 11. During server-side rendering, `monaco-editor` is not available, and the `loader.init()` call inside `setupMonaco` may throw or hang.

**Fix:** Guard with `typeof window !== "undefined"` or move inside a `useEffect` / client-only context. The component is marked `"use client"` but module-level code in a `"use client"` file still executes during SSR.

### P2-3: `useYjsProvider` returns values from refs — first render with stale data

**Files:** `src/lib/yjs/use-yjs-provider.ts:27-41`

The hook stores provider/yDoc/yText in refs and uses `forceUpdate(n => n + 1)` to trigger re-renders after `useEffect` fires. The returned values come from refs (`yDocRef.current`), so the consuming component renders once with `null` values before the effect fires and triggers a second render.

**Fix:** Use `useSyncExternalStore` or move the connection state into `useState` instead of refs with forced re-renders.

### P2-4: `useYjsProvider` Yjs provider lifecycle race on document name change

**Files:** `src/lib/yjs/use-yjs-provider.ts:35-78`

When `documentName` changes (e.g., switching problem attempts), the cleanup destroys the old provider and creates a new one. `provider.destroy()` is asynchronous but the effect creates the new provider immediately without waiting for the old one to finish. Two providers briefly coexist, and the old cleanup may fire after the new setup.

**Fix:** Serialize provider creation — wait for old provider to fully disconnect before creating a new one, or use a key-based remount (`key={activeAttemptId}`) on the consuming component.

### P2-5: AI interaction endpoint has permanently disabled authorization

**Files:** `src/app/api/ai/interactions/route.ts:12`

The auth guard reads `if (false /* TODO: check org membership role */)` — permanently bypassed. This route is likely dead (Go handles the proxy), but the file exists and advertises a false authorization boundary.

**Fix:** Either implement the authorization check or remove the file. If the route is vestigial and proxied to Go, delete it.

### P2-6: In-memory SSE event bus breaks multi-instance deployments

**Files:** `src/lib/sse.ts:3-31`

`SessionEventBus` stores listeners in a per-process `Map`. With 2+ Next.js pods, clients connected to different pods do not receive cross-pod events.

**Fix:** Replace with Redis pub/sub for multi-instance, or at minimum document the single-instance limitation.

### P2-7: Student session SSE uses `window.location.href` for navigation

**Files:** `src/components/session/student/student-session.tsx:86-88`

The `session_ended` handler sets `window.location.href = returnPath`, causing a full-page reload with white flash and loss of React state. The router is already imported but not used.

**Fix:** Use `router.push(returnPath)` for client-side navigation.

### P2-8: Email format not validated in `AddMember` handler

**Files:** `platform/internal/handlers/classes.go:292-302`

The handler accepts any string as `Email` and passes it to `GetUserByEmail`. No email-format validation before the DB query, unlike Next.js where `z.string().email()` enforces format.

**Fix:** Add basic email format validation in the handler before the store call.

### P2-9: `maskURL` naively truncates URLs and may leak credentials

**Files:** `platform/cmd/api/main.go:308-312`

The function truncates the database URL to the first 30 characters. A typical URL like `postgres://user:password@host/db` may expose the password in the first 30 characters.

**Fix:** Use `net/url` to parse the URL, redact the password field, and reconstruct before logging.

### P2-10: No request body size limits on JSON-decoded endpoints

**Files:** `platform/internal/handlers/helpers.go:38-43`

`decodeJSON` uses `json.NewDecoder(r.Body).Decode(dst)` with no `http.MaxBytesReader`. Only the upload handler limits body size. An attacker can send arbitrarily large JSON payloads to any POST/PATCH endpoint.

**Fix:** Add `r.Body = http.MaxBytesReader(w, r.Body, 1<<20)` (1MB limit) to a shared body-reading helper.

### P2-11: Broadcaster subscriber callback runs inline with no timeout

**Files:** `platform/internal/events/broadcaster.go:44-62`

`Emit` copies callbacks under `RLock` then invokes them outside the lock. A slow SSE subscriber blocks the emitting goroutine indefinitely during `cb(event, data)`. This is a goroutine leak in `SessionEvents` which processes events inline.

**Fix:** Wrap each callback in a goroutine with a short timeout.

### P2-12: E2E test suite has ~30 permanently disabled `test.skip` calls

**Files:** Multiple `e2e/*.spec.ts` files

Many specs use `test.skip(true, "reason")` which permanently disables tests rather than gating on data availability. Only `problem-editor-responsive.spec.ts` uses conditional `test.skip(!url, ...)`. The impersonation spec has zero active tests.

**Fix:** Convert permanent skips to conditional gates checking for test data at runtime. Remove dead tests or fix the underlying data issues.

### P2-13: No error boundary anywhere in the React tree

**Files:** All client components

A single uncaught rendering error in any client component will white-screen the entire app. React 18+ unmounts the whole tree on uncaught errors.

**Fix:** Add `error.tsx` files to route segments and wrap critical components in `<ErrorBoundary>` with fallback UI.

### P2-14: `UpdateParticipantStatus` has confusing pseudo-status handling

**Files:** `platform/internal/store/sessions.go:449-471`

The switch case handles `"needs_help"` and `"active"` as pseudo-statuses that manipulate `help_requested_at` rather than actual `participant_status` enum values. The valid enum values are `invited`, `present`, `left`, but the handler also accepts `needs_help` and `active` without clear documentation.

**Fix:** Extract `help_requested_at` management into dedicated methods like `SetHelpRequested`/`ClearHelpRequested`, or heavily document the pseudo-status convention.

## Process Findings

### P2-15: Plan 028 missing post-execution report

**Files:** `docs/plans/028-problem-bank.md:1486-1504`

The plan has a completed code review but no `## Post-Execution Report` section. The checklist marks step 7 as unchecked. Per `development-workflow.md:91-95`, all shipped plans require a post-execution report.

**Fix:** Append the report with implementation details, file counts, test coverage, known limitations, and verification output.

### P2-16: Seven completed plans have stale top-level status labels

**Files:** `docs/plans/032-*.md:11` through `docs/plans/038-*.md:9`

Plans 032, 033a, 033b, 034, 035, 036 say "In progress" at the top while their post-execution reports say "Complete." Plan 038 says "Not started" while its post-execution report says "Complete."

**Fix:** Normalize all top-level status labels to match the post-execution report.

### P2-17: Plan 012 has duplicate numbering

**Files:** `docs/plans/012-assignments.md`, `docs/plans/012-monaco-editor-migration.md`

Two unrelated plans share the 012 prefix. Per repoconvention of sequential numbering, each plan must have a unique number.

**Fix:** Rename one to a free number and update all cross-references.

### P2-18: Deleted `seed_python_101.sql` still referenced in 20+ places across docs

**Files:** `docs/specs/009-problem-bank.md:267`, `docs/plans/028-problem-bank.md:1288-1361`, `docs/plans/032-teaching-unit-topic-migration.md:79-80`, `docs/plans/027-legacy-classroom-cleanup.md:191`

Plan 049 deleted `scripts/seed_python_101.sql`, but multiple living documents still instruct readers to modify or run it.

**Fix:** Strike or annotate these references as "Retired by plan 049."

### P2-19: Parent-linking deferrals refer to reassigned plan numbers

**Files:** `platform/internal/handlers/parent.go:24-41`, `src/app/onboarding/page.tsx:30-34`, `docs/plans/049-python-101-curriculum.md:462-466`

Parent endpoints and onboarding comments reference plans 048/049 for parent-linking and `/register-org`. Plan 049 was reassigned to Python 101. No concrete follow-up plan exists for these items.

**Fix:** File a concrete parent-linking plan or add explicit TODO ownership. Update stale plan references in code and docs.

### P2-20: `TODO.md` does not reflect any shipped plans from 028 onward

**Files:** `TODO.md`

The TODO file contains Phase 1-era items and lacks entries for teaching units (031-036), problem bank (028), session model (030a-e), Python 101 (049), auth hardening (039-043), and responsive layout (042).

**Fix:** Add high-level entries for major shipped systems with any known open items. Update or remove stale entries.

### P2-21: `docs/code-review.md` `[OPEN]` items remain in 3 completed plans

**Files:** `docs/plans/017-go-foundation.md:4044-4052`, `docs/plans/012-monaco-editor-migration.md:1352-1372`, `docs/plans/031-teaching-unit-mvp.md:864`

13 `[OPEN]` review items across 3 shipped plans reference real code defects (body size limits, maskURL credential leak, Monaco model leaks, CDN self-hosting, missing index). All have `→ Response:` text deferring to follow-ups but were never converted to `[WONTFIX]` with concrete plan citations. Per `CLAUDE.md:53`, deferrals require a numbered follow-up plan.

**Fix:** Either convert to `[WONTFIX]` with plan-number citations for each, or file concrete follow-up plans.

## Review Notes

- All findings were verified in source code using file reads, greps, and cross-references.
- The `scheduled_sessions` table (P0-2) has never been created by any persisted migration. A fresh database would fail on first use.
- The Drizzle schema has 5+ indexes that exist in SQL migrations but not in the TypeScript schema definition. A `drizzle-kit push` would drop them.
- Go handlers consistently apply authorization to "read + list" endpoints but omit it from "read one" and "delete" endpoints on sibling handlers — this is a systematic gap across `topics`, `classes`, `annotations`, `assignments`, and `schedule`.
- The Hocuspocus auth gap is acknowledged in code comments as deferred since plans 030b/030c/035 but has never been filed as a concrete follow-up plan.
- Prior review report 008 (also generated this session) is a companion with complementary findings focused on plan-execution alignment. That file was also lost.

## Not Verified

- Automated tests were not run.
- Browser flows were not run.
- Database migrations were not applied in a fresh database.
- Findings are source-review based. Each P0/P1 item should receive a regression test before closing.
