# 008 Executed Plans Codebase Review — 2026-05-01

## Scope

Reviewed executed plans `docs/plans/021-*` through `docs/plans/049-*` for drift against the current codebase at main `88ee84e`. Focus: promises that did not land, unresolved deferrals without concrete follow-up plans, auth/privacy gaps, schema/code mismatches, and tests that do not guard the behavior they claim to cover.

## Findings

### [P0] Hocuspocus session access rules promised in 030b/030c never shipped

**Files:** docs/plans/030b-session-invite-tokens.md:31, docs/plans/030b-session-invite-tokens.md:98, docs/plans/030c-session-direct-add-and-access.md:31, docs/plans/030c-session-direct-add-and-access.md:92, server/hocuspocus.ts:20, server/hocuspocus.ts:29, TODO.md:9

Plan 030b explicitly required replacing the permissive `session:{id}:user:{uid}` shortcut with the same class-membership-or-token rules used by HTTP, and plan 030c required extending that gate for direct-added users. Current Hocuspocus auth still splits an unsigned `userId:role` string, and the code comment says a forged token can open session documents for the forged owner. This leaves the realtime path outside the session access model the plans claimed to unify.

**Recommendation:** File and execute a dedicated plan titled `Hocuspocus JWT and Session Access Gate`. Require signed tokens plus a Go-backed `CanAccessSession` check on WebSocket upgrade, and add negative tests for outsider, expired invite, revoked participant, and forged-token cases.

### [P1] Unit collaboration auth was deferred as a known gap but not turned into a plan

**Files:** docs/plans/035-teaching-unit-realtime-ai.md:101, docs/plans/035-teaching-unit-realtime-ai.md:152, docs/plans/035-teaching-unit-realtime-ai.md:168, server/hocuspocus.ts:79, server/hocuspocus.ts:86, src/lib/yjs/use-yjs-tiptap.ts:99

Plan 035's review accepted that `unit:*` Hocuspocus auth only checks `role === "teacher"` and deferred per-unit `canEditUnit` validation to a future purpose-built endpoint. The shipped server still allows any `teacher` token into any `unit:{unitId}` document, and the client constructs that token as `${userId}:teacher`. Even ignoring the unsigned-token P0, this means org-scoped unit collaboration is not bound to the unit's scope.

**Recommendation:** Include unit document auth in `Hocuspocus JWT and Session Access Gate`, or file `Hocuspocus Teaching Unit Scope Auth`. The gate should call the same `canEditUnit` logic used by `POST /api/units/{id}/draft-with-ai` and `PUT /api/units/{id}/document`.

### [P1] Drizzle still declares `documents.classroom_id` after the migration dropped it

**Files:** docs/plans/027-legacy-classroom-cleanup.md:352, docs/plans/027-legacy-classroom-cleanup.md:369, docs/plans/048-session-agenda-and-ux-cleanup.md:198, docs/plans/048-session-agenda-and-ux-cleanup.md:201, drizzle/0012_drop_legacy_classrooms.sql:47, src/lib/db/schema.ts:405, src/lib/db/schema.ts:412, src/lib/db/schema.ts:424

Plan 027 says migration 0012 dropped `documents.classroom_id`, and plan 048 relies on resolving class navigation via `LEFT JOIN sessions`, not the dropped column. Current Drizzle schema still exports `documents.classroomId` and `documents_classroom_idx`. Any Drizzle-generated query, test helper, or migration diff using that schema can reference a column and index that the migration chain removes.

**Recommendation:** File `Remove Stale Drizzle Documents Classroom Column`. Delete the stale schema field/index, update tests that import it, and add a schema regression that compares the Drizzle model against the applied migration shape for `documents`.

### [P1] Plan 021's Next backend deletion remains incomplete and preserves duplicate unguarded class-member routes

**Files:** docs/plans/021-frontend-cleanup.md:59, docs/plans/021-frontend-cleanup.md:66, docs/plans/021-frontend-cleanup.md:1148, src/app/api/classes/[id]/members/route.ts:18, src/app/api/classes/[id]/members/route.ts:43, src/app/api/classes/[id]/members/route.ts:56, src/app/api/classes/[id]/members/[memberId]/route.ts:15, src/app/api/classes/[id]/members/[memberId]/route.ts:38

Plan 021's goal was to remove all non-auth Next API/backend logic, but the post-execution report deferred backend deletion. The shadow Next class-member routes still contain the same failure mode as the Go handlers: they check only that a session exists before adding, listing, updating, or removing class members. Rewrites normally proxy `/api/classes/*` to Go, but the duplicate route files are still importable in tests and remain a runtime fallback/maintenance hazard.

**Recommendation:** File `Delete or Lock Down Shadow Next API Routes`. Until deletion, apply the same class authority gate to the Next routes and add a parity test that proves the Go proxy is the only production path.

### [P2] Plan 049 references a demo wire-up script that is not present

**Files:** docs/plans/049-python-101-curriculum.md:448, scripts/python-101/import.ts:145, scripts/python-101/import.ts:892, scripts/python-101/import.ts:1050

Plan 049's post-execution report says the demo class points directly at Bridge HQ Python 101 through `scripts/wire_python_101_demo_class.sql`, but that file is absent from `scripts/`. The current importer instead exposes `--wire-demo-class` and updates the demo class to a cloned course path. The report is now misleading about the shipped operational path and where cross-org course semantics are documented.

**Recommendation:** File `Python 101 Demo Wire-Up Documentation Cleanup`. Either restore the referenced SQL artifact if it is still the intended operator path, or update plan 049 and content docs to describe the importer-backed clone/wire flow that exists in code.

### [P2] Plan 049 closed with open follow-ups explicitly marked "no plan filed yet"

**Files:** docs/plans/049-python-101-curriculum.md:454, docs/plans/049-python-101-curriculum.md:462, docs/plans/049-python-101-curriculum.md:464, docs/plans/049-python-101-curriculum.md:465, docs/plans/049-python-101-curriculum.md:466

The Python 101 plan shipped while browser smokes, different-org picker discovery, cross-org subscription semantics, Pyodide/Piston drift cataloging, and authoring CLI helpers were all left as "Open follow-ups (non-blocking; no plan filed yet)." Two of these affect release confidence for the curriculum in real classrooms: browser verification and cross-org reuse semantics.

**Recommendation:** Create a small tracking plan titled `Python 101 Post-Ship Validation and Reuse Follow-Ups`. At minimum, separate manual browser smoke verification from product-design follow-ups so validation debt is not buried with future feature work.
