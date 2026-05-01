# 009 Deep Codebase Review — 2026-05-01

## Scope

Reviewed current code by responsibility area: auth surface, Go handlers and multi-tenant data exposure, Hocuspocus/WebSocket token handling, frontend/server-component trust boundaries, Drizzle schema versus query code, and critical test coverage gaps. Findings below cite only code read in this pass.

## Findings

### [P0] Class archive and membership endpoints allow any authenticated user to mutate any class

**Files:** platform/internal/handlers/classes.go:231, platform/internal/handlers/classes.go:239, platform/internal/handlers/classes.go:282, platform/internal/handlers/classes.go:323, platform/internal/handlers/classes.go:337, platform/internal/handlers/classes.go:345, platform/internal/handlers/classes.go:353, platform/internal/handlers/classes.go:385, platform/internal/handlers/classes.go:393, platform/internal/handlers/classes.go:414

`ArchiveClass`, `AddMember`, `ListMembers`, `UpdateMemberRole`, and `RemoveMember` only require `claims != nil`. There is no check that the caller created the class, teaches it, administers the owning org, or is a platform admin. Any logged-in student who knows a class UUID can archive it, enumerate members, add arbitrary users, promote members, or remove members.

**Recommendation:** Add a shared class-authority helper for mutations, distinct from read access if needed, and require instructor/org-admin/platform-admin for all five methods. Candidate plan: `Class Membership Mutation Authorization`.

### [P0] Hocuspocus accepts forgeable `userId:role` tokens

**Files:** server/hocuspocus.ts:15, server/hocuspocus.ts:20, server/hocuspocus.ts:46, server/hocuspocus.ts:56, server/hocuspocus.ts:74, server/hocuspocus.ts:86, src/lib/yjs/use-yjs-provider.ts:48, src/lib/yjs/use-yjs-provider.ts:52, src/lib/yjs/use-yjs-tiptap.ts:99, src/lib/yjs/use-yjs-tiptap.ts:100

The WebSocket server treats the client-supplied token as a plain string and trusts whatever `userId` and `role` the browser sends. Session rooms, attempt rooms, broadcast rooms, and unit rooms all branch from those unverified values. A direct WebSocket client can forge teacher/user/admin-like roles and target document names without possessing an Auth.js session or Go-verified claims.

**Recommendation:** Replace the token with a signed JWT/JWE or short-lived Hocuspocus-specific token minted by the authenticated Next/Go path. Bind the token to allowed document scopes, and reject tokens whose subject or scope does not match `documentName`.

### [P1] Session event streams and help queues leak by session ID

**Files:** platform/internal/handlers/sessions.go:611, platform/internal/handlers/sessions.go:618, platform/internal/handlers/sessions.go:635, platform/internal/handlers/sessions.go:646, platform/internal/handlers/sessions.go:654, platform/internal/handlers/sessions.go:669

`SessionEvents` checks only that claims exist, then subscribes the caller to the requested session ID. `GetHelpQueue` also checks only claims, then returns all participants with `help_requested_at` for the requested session. A logged-in outsider can observe live join/leave/broadcast/help activity for sessions they do not teach or attend if they know or guess the UUID.

**Recommendation:** Reuse `canJoinSession` for student-visible streams and `isSessionAuthority` for teacher-only queues. Return 404 for unauthorized session IDs where existence should not be leaked. Candidate plan: `Session Realtime Endpoint Authorization`.

### [P1] Annotation APIs are not scoped to document owner, class, or teacher authority

**Files:** platform/internal/handlers/annotations.go:28, platform/internal/handlers/annotations.go:57, platform/internal/handlers/annotations.go:72, platform/internal/handlers/annotations.go:85, platform/internal/handlers/annotations.go:93, platform/internal/handlers/annotations.go:100, platform/internal/handlers/annotations.go:112, platform/internal/handlers/annotations.go:119, platform/internal/store/annotations.go:66

The annotation handler lets any authenticated caller create annotations against any `documentId`, list annotations for any `documentId`, and delete or resolve any annotation by ID. The store layer performs direct `code_annotations` operations without joining to `documents`, sessions, classes, or attempts. This leaks teacher feedback and lets outsiders tamper with annotation state.

**Recommendation:** Resolve the target document/attempt, then require owner, class instructor/TA/org-admin, or platform-admin as appropriate for read/write. Candidate plan: `Annotation Document Access Enforcement`.

### [P1] Platform-admin authorization is cached in the session JWT and trusted by Go

**Files:** src/lib/auth.ts:181, src/lib/auth.ts:183, src/lib/auth.ts:188, src/lib/auth.ts:189, src/lib/auth.ts:194, platform/internal/auth/jwt.go:156, platform/internal/auth/middleware.go:121, platform/internal/auth/middleware.go:193, platform/internal/auth/middleware.go:201

The NextAuth `jwt` callback refreshes `token.isPlatformAdmin` only when `user` is present, which is the sign-in path. Later requests reuse the cached JWT claim, and Go extracts `isPlatformAdmin` from that token and uses it in `RequireAdmin`. Admin promotion or demotion in the database does not reliably take effect until the user gets a fresh token, so demoted admins can retain privileged Go and Next access until expiry.

**Recommendation:** Refresh authorization-critical claims on every `jwt()` callback or move admin checks to a server-side DB lookup. Candidate plan: `Refresh JWT Claims on Every NextAuth jwt() Call`.

### [P1] Drizzle schema still models a dropped `documents.classroom_id` column

**Files:** src/lib/db/schema.ts:405, src/lib/db/schema.ts:412, src/lib/db/schema.ts:424, drizzle/0012_drop_legacy_classrooms.sql:47, drizzle/0012_drop_legacy_classrooms.sql:48, drizzle/0012_drop_legacy_classrooms.sql:49, platform/internal/store/documents.go:98, platform/internal/store/documents.go:101

Migration 0012 drops `documents.classroom_id` and its index, while the Go document store now resolves navigation through a `LEFT JOIN sessions`. The Drizzle schema still declares `classroomId` and `documents_classroom_idx`. Any TypeScript code or migration tooling using the schema as authoritative can emit invalid SQL against a migrated database.

**Recommendation:** Remove the stale column/index from `src/lib/db/schema.ts` and add a schema drift test for `documents`. Candidate plan: `Remove Stale Documents Classroom Schema`.

### [P2] Critical auth tests cover missing claims but not cross-user denial

**Files:** platform/internal/handlers/classes_test.go:89, platform/internal/handlers/classes_test.go:99, tests/integration/class-members-api.test.ts:21, tests/integration/class-members-api.test.ts:40, platform/internal/handlers/annotations_test.go:15, platform/internal/handlers/annotations_test.go:54, tests/integration/annotations-api.test.ts:16, tests/integration/annotations-api.test.ts:72

Class member tests assert unauthenticated and malformed-input behavior, and the integration tests exercise successful teacher-looking calls plus wrong-class IDs. They do not assert that an authenticated outsider is forbidden. Annotation tests similarly cover no-claims, required fields, and happy paths, but no test creates a second user and proves they cannot read, delete, or resolve another user's annotation.

**Recommendation:** Add negative integration tests for outsider student, unrelated teacher, same-org non-teacher, and platform-admin bypass for class member and annotation APIs. Make these fail first so the missing auth gates are locked after implementation.
