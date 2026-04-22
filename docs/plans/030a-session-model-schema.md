# Session Model Phase 1: Schema + Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the legacy live-session schema to the spec 010 session model, normalize participant presence state, and update every compile-time caller so current class-based session flows keep working on the new tables.

**Architecture:** Ship one additive migration that renames `live_sessions` to `sessions`, renames `session_participants.student_id` to `user_id`, adds the new session columns, and moves help-queue state out of `session_participants.status` into its own nullable timestamp. Keep the current product behavior class-bound for now, but make Go, Drizzle, Hocuspocus-adjacent helpers, and server components all read the renamed schema and the new `live`/`ended` plus `invited`/`present`/`left` lifecycle vocabulary.

**Tech Stack:** PostgreSQL 15 migration SQL, Go (`database/sql`, Chi, testify), Next.js 16, Drizzle ORM, Vitest

**Spec:** `docs/specs/010-session-model.md`

**Branch:** `feat/010-session-model` (create before implementation; do not build this on `feat/009-problem-bank`)

**Depends On:** None

**Unblocks:** `030b-session-invite-tokens.md`, `030c-session-direct-add-and-access.md`, `030d-orphan-sessions.md`, `030e-scheduled-session-backref.md`

---

## Why This Phase Exists

Spec 010 assumes a schema the repo does not have yet. Today:

- the table is still `live_sessions`
- session status is `active|ended`, not `live|ended`
- participant identity is `student_id`, not `user_id`
- participant status is overloaded with help-queue state (`active|idle|needs_help`), which conflicts with the spec's roster lifecycle (`invited|present|left`)
- schedule linkage still points at `live_session_id`

This phase fixes the foundation first so later phases can change behavior without mixing runtime feature work and table surgery in the same PR.

## File Structure

| File | Responsibility |
|---|---|
| `drizzle/0014_session_model.sql` | Rename `live_sessions` to `sessions`, rename `student_id` to `user_id`, backfill `title`, add invite/schedule columns, add `help_requested_at`, convert existing status values to the spec vocabulary, keep old schedule backref temporarily. |
| `src/lib/db/schema.ts` | Rename Drizzle exports to `sessions`; expose new columns and enum literals; repoint all foreign keys. |
| `platform/internal/store/sessions.go` | Rename types to `Session` / `SessionParticipant`; query `sessions`; treat presence and help state separately. |
| `platform/internal/store/schedule.go` | Point scheduled-session creation and completion at `sessions`; keep `scheduled_sessions.live_session_id` as compatibility-only for now. |
| `platform/internal/store/interactions_test.go` | Update session fixture expectations after the status rename. |
| `platform/internal/store/sessions_test.go` | Cover migration-compatible create/get/end/join/leave/help flows on the renamed schema. |
| `platform/internal/store/schedule_test.go` | Verify start-from-schedule still works after the table rename. |
| `platform/internal/handlers/sessions.go` | Return the new status values and use help-specific fields instead of overloading participant status. |
| `platform/internal/handlers/sessions_test.go` | Update auth/unit coverage to the new request and response vocabulary. |
| `platform/cmd/api/main.go` | No route changes, but compile against the renamed session store types. |
| `src/lib/sessions.ts` | Rename schema references and keep Next server-component helpers working on `sessions`. |
| `src/lib/attendance.ts` | Swap `liveSessions` references to `sessions`; keep attendance/history queries compiling. |
| `src/lib/parent-reports.ts` | Same schema rename sweep for report aggregation. |
| `src/components/help-queue/help-queue-panel.tsx` | Read help state from the dedicated field rather than the participant status enum. |
| `src/components/help-queue/raise-hand-button.tsx` | Toggle help state without mutating the presence lifecycle field. |
| `src/components/session/student-tile.tsx` | Replace `active|idle|needs_help` rendering with `present|left` plus a separate hand-raised indicator. |
| `src/components/session/teacher/student-list-panel.tsx` | Sort and badge participants by presence plus help state. |
| `src/app/(portal)/teacher/classes/[id]/session/[sessionId]/dashboard/page.tsx` | Continue rendering teacher sessions from the renamed schema helper. |
| `src/app/(portal)/student/classes/[id]/session/[sessionId]/page.tsx` | Continue joining student sessions from the renamed schema helper. |
| `src/app/(portal)/parent/children/[id]/live/page.tsx` | Keep parent live-view lookups compiling after the rename. |
| `tests/helpers.ts` | Clean up `sessions` instead of `live_sessions` and update fixture helpers to the new names. |
| `tests/unit/schema.test.ts` | Assert the renamed Drizzle exports. |
| `tests/unit/student-tile.test.tsx` | Update status rendering expectations for the split presence/help model. |
| `docs/api.md` | Note the status vocabulary change where session payload examples appear. |

---

## Task 1: Migration + Drizzle Schema

**Files:**
- Create: `drizzle/0014_session_model.sql`
- Modify: `src/lib/db/schema.ts`
- Modify: `tests/helpers.ts`
- Modify: `tests/unit/schema.test.ts`

- [ ] Add an idempotent migration that does all of the following in one transaction:
  - renames `live_sessions` to `sessions`
  - renames `session_participants.student_id` to `user_id`
  - renames `ai_interactions.session_id` and other foreign keys to point at `sessions`
  - adds `title`, `invite_token`, `invite_expires_at`, `scheduled_session_id`
  - drops `sessions.class_id` `NOT NULL`
  - adds `help_requested_at timestamptz`
  - converts session status values `active -> live`
  - converts participant status values `active|idle|needs_help -> present`, while backfilling `help_requested_at` for the old `needs_help` rows
  - expands the participant status enum to include `invited`, `present`, `left`
  - leaves `scheduled_sessions.live_session_id` in place for compatibility
- [ ] Update `src/lib/db/schema.ts` to export `sessions` instead of `liveSessions`, `userId` instead of `studentId`, and the new status literals.
- [ ] Update test helpers and unit schema assertions to use the renamed table exports.
- [ ] Smoke-verify on both `bridge` and `bridge_test` before touching application code.

**Testing plan:**
- `psql postgresql://work@127.0.0.1:5432/bridge -f drizzle/0014_session_model.sql`
- `psql postgresql://work@127.0.0.1:5432/bridge_test -f drizzle/0014_session_model.sql`
- `psql postgresql://work@127.0.0.1:5432/bridge -c "\\d sessions"`
- `psql postgresql://work@127.0.0.1:5432/bridge -c "\\d session_participants"`
- `bun run test tests/unit/schema.test.ts`

## Task 2: Go Store + Handler Compatibility Sweep

**Files:**
- Modify: `platform/internal/store/sessions.go`
- Modify: `platform/internal/store/schedule.go`
- Modify: `platform/internal/store/sessions_test.go`
- Modify: `platform/internal/store/schedule_test.go`
- Modify: `platform/internal/store/interactions_test.go`
- Modify: `platform/internal/handlers/sessions.go`
- Modify: `platform/internal/handlers/sessions_test.go`
- Modify: `platform/cmd/api/main.go`

- [ ] Rename the Go session types and SQL column lists to the new table/column names.
- [ ] Change create/start/end flows to emit `status="live"` instead of `status="active"`.
- [ ] Replace participant help-queue reads and writes with `help_requested_at` helpers so `status` only represents the roster lifecycle.
- [ ] Preserve current public behavior for class-based session create/join/list/end routes; phase 1 is not where access control changes.
- [ ] Keep schedule start/complete behavior working while still writing the legacy `scheduled_sessions.live_session_id` compatibility column.

**Testing plan:**
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/store -run 'TestSessionStore_|TestScheduleStore_|TestInteractionStore_' -count=1`
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/handlers -run 'TestCreateSession|TestGetSession|TestJoinSession|TestGetParticipants|TestToggleHelp|TestToggleBroadcast' -count=1`

## Task 3: Frontend/Server Helper Compatibility Sweep

**Files:**
- Modify: `src/lib/sessions.ts`
- Modify: `src/lib/attendance.ts`
- Modify: `src/lib/parent-reports.ts`
- Modify: `src/components/help-queue/help-queue-panel.tsx`
- Modify: `src/components/help-queue/raise-hand-button.tsx`
- Modify: `src/components/session/student-tile.tsx`
- Modify: `src/components/session/teacher/student-list-panel.tsx`
- Modify: `src/app/(portal)/teacher/classes/[id]/session/[sessionId]/dashboard/page.tsx`
- Modify: `src/app/(portal)/student/classes/[id]/session/[sessionId]/page.tsx`
- Modify: `src/app/(portal)/parent/children/[id]/live/page.tsx`
- Modify: `tests/unit/student-tile.test.tsx`

- [ ] Update all Drizzle-backed helpers to the renamed exports and the new `live` / `present` vocabulary.
- [ ] Remove the last UI assumptions that `participant.status === "needs_help"` by reading help state explicitly.
- [ ] Keep teacher, student, and parent session views rendering unchanged apart from the status label vocabulary.

**Testing plan:**
- `bun run test tests/unit/student-tile.test.tsx`
- `bun run test tests/unit/schema.test.ts`
- `node_modules/.bin/tsc --noEmit`

## Task 4: Documentation + Verification Gate

**Files:**
- Modify: `docs/api.md`
- Modify: `docs/plans/030a-session-model-schema.md`

- [ ] Update `docs/api.md` anywhere session payload examples still say `active` or `studentId`.
- [ ] Re-read every file touched in this phase and confirm there are no remaining `live_sessions`, `liveSessions`, `student_id`, or `studentId` references except the explicitly temporary `scheduled_sessions.live_session_id`.
- [ ] Run broad verification before moving to 030b.

**Verification commands:**
- `rg -n 'live_sessions|liveSessions|student_id|studentId' platform src tests drizzle`
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./... -count=1 -timeout 120s`
- `bun run test`
- `node_modules/.bin/tsc --noEmit`

---

## Code Review

Reviewers append findings here following `docs/code-review.md`. Author responds inline with `→ Response:` and updates status to `[FIXED]` or `[WONTFIX]`.

## Post-Execution Report

Populate during Step 6 of `docs/development-workflow.md` after this phase ships.
