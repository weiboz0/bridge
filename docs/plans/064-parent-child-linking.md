# Plan 064 — Parent-child linking schema and authorization (P1)

## Status

- **Date:** 2026-05-02
- **Severity:** P1 (parent reports endpoints permanently 501; parent-viewer uses unverified legacy token; the implicit `class_memberships role="parent"` model leaks one parent's view to ALL students in the shared class)
- **Origin:** Original "plan 049" intent (see `parent.go:26` comment) was renamed to Python 101 curriculum. This plan resurrects the parent-child linking work under a fresh number. Hard prerequisite for plan 053b's parent-viewer half.

## Problem

Three things are broken because Bridge has no real parent ↔ child link in the DB:

1. **`/api/parent/children/{childId}/reports` returns 501.** `parent.go:43-52` and `:54-63` are intentionally disabled because there's no auth model — anyone could read any student's reports otherwise. Plan 047 P0 fix.
2. **Parent-viewer uses an unverified legacy token.** `live-session-viewer.tsx:32` constructs `${parentId}:parent`; `server/hocuspocus.ts:46-52` accepts `role==="parent"` for any session doc with no further checks. This is the security hole plan 053 closes — but plan 053b can't migrate the parent-viewer to JWT mint until a real parent-child gate exists.
3. **The current implicit model leaks across siblings.** `src/lib/parent-links.ts::getLinkedChildren` derives "your children" from `class_memberships role="parent"` — any student in a class where the parent has the `parent` role becomes "their child." A parent in `Period 3` can see the live activity, code, and (eventually) reports for every other student in `Period 3`.

## Out of scope

- Self-claim / invite-code workflow for parents (Phase 2 of a future plan once the platform/org-admin paths exist).
- Email-domain matching as a heuristic auto-link (privacy-sensitive; needs product call).
- Migrating EXISTING `class_memberships role="parent"` rows into the new table — Bridge has no production parents yet; the current rows are seed/test fixtures and can be cleaned up by the importer.
- The parent-viewer client refactor itself — that's plan 053b §item 3, which CONSUMES this plan's `IsParentOf` helper.

## Approach

Add a real `parent_links` table with a small admin-only CRUD surface. Wire the `IsParentOf` gate into the two `parent.go` endpoints (re-enable them) and surface the new helper for plan 053b to consume.

**Schema** — new table `parent_links`:

| Column | Type | Notes |
|---|---|---|
| `id` | uuid PK | `defaultRandom()` |
| `parent_user_id` | uuid NOT NULL | FK → `users.id` ON DELETE CASCADE |
| `child_user_id` | uuid NOT NULL | FK → `users.id` ON DELETE CASCADE |
| `status` | varchar(16) NOT NULL DEFAULT 'active' | `active` / `revoked` |
| `created_by` | uuid NOT NULL | FK → `users.id`; the platform admin who created the link |
| `created_at` | timestamptz NOT NULL DEFAULT now() | |
| `revoked_at` | timestamptz NULL | set when status flips to revoked |

Indexes:
- `parent_links_parent_idx` on `(parent_user_id)` — `getLinkedChildren` queries by parent
- `parent_links_child_idx` on `(child_user_id)` — admin "who's linked to this child" listing
- `parent_links_active_uniq` PARTIAL UNIQUE on `(parent_user_id, child_user_id) WHERE status = 'active'` — at most one ACTIVE link per (parent, child) pair, but revoked rows can coexist for audit AND a revoked pair can be re-linked (re-link inserts a new row; the old revoked row stays).

Status filter: only `active` links pass `IsParentOf`. Revoked links remain in the table for audit but don't grant access. Re-linking a previously-revoked pair inserts a NEW row (the partial unique allows this).

**Authorization helper** (Go):

```go
// IsParentOf reports whether `parentID` has an ACTIVE parent_link
// to `childID`. Existence-only on the matching row with status=active.
func (s *ParentLinkStore) IsParentOf(ctx context.Context, parentID, childID string) (bool, error)
```

**Endpoints**:

Phase 2 — re-enable existing parent endpoints:
- `GET /api/parent/children/{childId}/reports` — gate with `IsParentOf(claims.UserID, childId)`. 403 on deny.
- `POST /api/parent/children/{childId}/reports` — same gate.

Phase 3 — admin CRUD:
- `GET /api/admin/parent-links?parent={uid}|child={uid}` — platform admin only. Filter by parent or child.
- `POST /api/admin/parent-links` — body `{parentUserId, childUserId}`. Conflict (409) if active link already exists.
- `DELETE /api/admin/parent-links/{id}` — flips status to revoked + sets revoked_at. Hard delete only on cascade from `users` deletion.

Org-admin CRUD is **deferred** — the simplest model is "platform admin links parents to children." Org-admin can come later if/when the org-onboarding workflow needs it.

**TS-side parent landing page**:
- `src/lib/parent-links.ts::getLinkedChildren` rewritten to query `parent_links` directly: `SELECT child.* FROM parent_links WHERE parent_user_id = $1 AND status = 'active' JOIN users ON ...`. The implicit class-membership-based logic gets removed (and any seed `class_memberships` rows with `role="parent"` get ignored — they're not the source of truth anymore).
- 4 consumers of `getLinkedChildren` need to keep working unchanged after the rewrite:
  - `src/app/(portal)/parent/page.tsx:11`
  - `src/app/(portal)/parent/children/[id]/page.tsx:20`
  - `src/app/(portal)/parent/children/[id]/live/page.tsx:19`
  - `src/app/api/parent/children/[id]/live/route.ts:19`
- `tests/unit/parent-links.test.ts` currently uses `class_memberships role="parent"` fixtures; rewrite to use `parent_links` rows.
- `parent` role on `class_memberships` becomes vestigial. Documented but not removed (existing seed fixtures may use it; no harm leaving the column value).

**Sibling-leak cleanup — `/api/sessions/{id}/topics`**:

Plan 053b can't migrate the parent-viewer until `/api/sessions/{id}/topics` (the only HTTP endpoint live-session-viewer.tsx hits) is parent-aware. Two facts:

1. The Next.js handler at `src/app/api/sessions/[id]/topics/route.ts:8-20` only checks `auth() != null` — anyone logged in gets the agenda for any session. Same shadow-route pattern as plans 055/056.
2. Go's `SessionHandler.GetSessionTopics` at `sessions.go:813-851` already has plan 043's class-membership gate (`canAccessClass`). But that gate doesn't include parents — a parent of a student in the class would be denied.

This plan fixes both:
- **Delete the Next shadow** (`src/app/api/sessions/[id]/topics/route.ts` GET; keep the POST/DELETE if they're teacher-only — verify or split). `next.config.ts` already proxies `/api/sessions/:path*` to Go.
- **Extend `GetSessionTopics`** to allow `IsParentOf(claims.UserID, <any session participant who is the user's child>)`. The check: load session → load participants → if claims.UserID has an active `parent_links` row to ANY participant (which is more efficient as a single SQL EXISTS join), allow.
- New helper or inline SQL — to be designed during implementation; cleanest is a `SessionStore.IsParentOfAnyParticipant(ctx, sessionID, parentID) bool` so the auth path is one round-trip.

## Files

**Schema + store**:
- New: `drizzle/0024_parent_links.sql` — table + indexes + status enum (or varchar with check constraint).
- Modified: `src/lib/db/schema.ts` — `parentLinks` Drizzle table + index declarations.
- New: `platform/internal/store/parent_links.go` — `ParentLinkStore` with `IsParentOf`, `CreateLink`, `RevokeLink`, `ListByParent`, `ListByChild`, `GetLink`.
- New: `platform/internal/store/parent_links_test.go` — store integration tests (create, revoke, IsParentOf for active/revoked/missing).

**Schema parity test**:
- New: `tests/integration/schema-parent-links.test.ts` — confirms live DB matches Drizzle (same pattern as `schema-drift.test.ts` from plan 054).

**Re-enable parent.go**:
- Modified: `platform/internal/handlers/parent.go` — drop the 501 stub, wire `IsParentOf` into both endpoints; require `ReportStore + ParentLinks` deps.
- Modified: `platform/internal/handlers/parent_test.go` — flip "expects 501" tests to the proper auth matrix (parent-of OK, parent-not-of 403, no-claims 401, unknown childId 403).
- Modified: `platform/cmd/api/main.go` — wire `ParentLinks` store into `ParentHandler`.

**Admin CRUD**:
- New: `platform/internal/handlers/admin_parent_links.go` — `AdminParentLinksHandler` with the four endpoints above.
- New: `platform/internal/handlers/admin_parent_links_test.go` — auth matrix + happy paths + 409 on duplicate.
- Modified: `platform/cmd/api/main.go` — wire under the existing `AdminHandler` group (or as a sibling handler).

**TS side**:
- Modified: `src/lib/parent-links.ts::getLinkedChildren` — query `parent_links` directly. Drop the class-membership scan.
- Modified: `tests/unit/parent-links.test.ts` (if it exists) — update fixtures to use the new table.

**Plan-053b unblock**:
- Plan 053b's parent-viewer phase can now consume `ParentLinkStore.IsParentOf` for the Hocuspocus / realtime-token mint side, AND the `/api/sessions/{id}/topics` HTTP gate is parent-aware. This plan does NOT migrate `live-session-viewer.tsx` itself — plan 053b owns that.
- 053b will need TWO checks for `authorizeSessionDoc`: (a) `IsParentOf(claims.UserID, sessionDocOwnerID)` AND (b) the session-doc owner is a participant in the session being requested. `IsParentOf` alone isn't sufficient — a parent of one student shouldn't get tokens for an unrelated session that happens to mention their child's docId in URL params. (Codex pass-1 catch.)

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Existing `class_memberships role="parent"` rows in dev/test DBs still affect tests | low | The new `getLinkedChildren` ignores them. Seed scripts that create them get a one-line update. |
| Admin UI to create parent_links doesn't exist | medium | Out of scope for this plan — the API endpoints exist; admin UI is a follow-up. Until then, links can be created via SQL or curl. |
| Parent-viewer remains broken until plan 053b ships | low | Documented in plan 053b §item 3. This plan unblocks it but doesn't ship the migration. |
| The 1:N model (one child can have multiple linked parents) doesn't account for guardian relationships, custody splits, etc. | medium | Same shape as Google Classroom's parent linking. Adequate for K-12 demo / early adopter contexts. |
| Cascade-on-user-delete is correct vs anonymizing | low | Cascade matches existing user-related FKs (`auth_providers`, `org_memberships`, `class_memberships`). Anonymization is a future GDPR concern, separate plan. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch on this plan + the existing `parent.go`/`parent-links.ts` shape. Verify the schema design (indexes, status enum, FK cascade), the authorization shape (existence-only on status=active), and that the `class_memberships role="parent"` deprecation path is sound.

### Phase 1: schema + store + IsParentOf helper

- New migration + Drizzle declaration.
- `ParentLinkStore` with all five methods.
- Store integration tests.
- Schema parity test.
- Single commit.

### Phase 2: re-enable parent.go endpoints

- Wire `IsParentOf` into `ListReports` / `CreateReport`.
- Update `parent_test.go` matrix.
- Single commit.

### Phase 3: admin CRUD

- New `AdminParentLinksHandler` + tests.
- Wire into router.
- Single commit.

### Phase 4: TS-side `getLinkedChildren` rewrite

- Query `parent_links` directly.
- Update `tests/unit/parent-links.test.ts` fixtures to use `parent_links` rows.
- Verify the 4 known consumers still work unchanged.
- Single commit.

### Phase 5: extend session-topics gate + delete Next shadow

- Add `SessionStore.IsParentOfAnyParticipant(ctx, sessionID, parentID) bool`.
- Wire into `GetSessionTopics` as a fall-back after `canAccessClass` denies.
- Delete `src/app/api/sessions/[id]/topics/route.ts` GET (Go handles it via proxy). Verify POST/DELETE on the same route are teacher-only and either keep them in Next or migrate too.
- Update `tests/unit/session-topics.test.ts` (if it tests the deleted Next route).
- Single commit.

### Phase 6: post-impl Codex review

Dispatch on the full diff. Confirm all five phases hang together.

## Codex Review of This Plan

### Pass 1 — 2026-05-02: BLOCKED → fixes folded in

Codex found 2 blockers + 4 useful notes:

1. **Full unique on `(parent_user_id, child_user_id)` blocks
   re-linking after revoke.** Plan said revoked rows stay for audit
   but the unique constraint would prevent inserting a fresh active
   row for the same pair. **Fix**: switched to `PARTIAL UNIQUE WHERE
   status = 'active'` so revoked rows coexist with new active ones.

2. **`/api/sessions/{id}/topics` is the OTHER unguarded endpoint
   parent-viewer hits.** `IsParentOf` alone doesn't fix it because
   the Next shadow at `route.ts:8-20` only checks `auth() != null`,
   and Go's `GetSessionTopics` gate doesn't include parents. **Fix**:
   added Phase 5 that deletes the Next shadow + extends the Go gate
   with a parent-of-participant check.

Non-blocking notes folded in:
- 4 `getLinkedChildren` consumers (parent dashboard + 3 routes) need
  to keep working after the rewrite — listed in the Files section.
- `tests/unit/parent-links.test.ts` has class-membership-based
  fixtures — listed for rewrite.
- Admin handler pattern: fits under existing `/api/admin` group with
  `auth.RequireAdmin` middleware — confirmed.
- 053b's `authorizeSessionDoc` needs `IsParentOf` AND child-in-session
  check (not just `IsParentOf` alone) — documented in the
  "Plan-053b unblock" note.

Schema conventions: `org_memberships` and `class_memberships` only
have `created_at` (no `updated_at`); plan 064 mirrors that.

### Pass 2 — 2026-05-02: **CONCUR**

Codex verified both fixes applied correctly + did a privacy endpoint
audit:
- Partial unique index handles re-link correctly.
- Phase 5 deletes only the GET shadow (POST/DELETE in both Next and
  Go are already teacher-only).
- Parent-viewer only fetches `/api/sessions/{id}/topics` (covered by
  Phase 5) and parent reports (covered by Phase 2). No other parent
  UI fetches reach submissions, code snapshots, or private profile
  fields.
- `IsParentOfAnyParticipant` join shape (parent_links.child_user_id
  → session_participants.user_id WHERE status='active') is coherent.
- 053b's `authorizeSessionDoc` scope correctly bounded — parents get
  read-only access via the user role, never write.

Plan ready for implementation. Branch off main when starting Phase 1.
