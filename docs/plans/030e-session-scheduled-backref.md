# Session Model Phase 5: Scheduled Session Backref Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `sessions.scheduled_session_id` the canonical schedule linkage and remove the old `scheduled_sessions.live_session_id` backref once all callers have migrated.

**Architecture:** This is a cleanup phase with a small blast radius: update schedule creation/completion to write and read `sessions.scheduled_session_id`, keep compatibility checks during rollout, then drop the old schedule-side backref in a dedicated migration once no application code reads it. No user-visible behavior should change in this phase.

**Tech Stack:** PostgreSQL migration SQL, Go, testify

**Spec:** `docs/specs/010-session-model.md`

**Branch:** `feat/010-session-model`

**Depends On:** `030a-session-model-schema.md`, `030d-orphan-sessions.md`

**Unblocks:** None

---

## File Structure

| File | Responsibility |
|---|---|
| `drizzle/0015_scheduled_session_backref_cleanup.sql` | Drop `scheduled_sessions.live_session_id` after application callers stop using it. |
| `platform/internal/store/schedule.go` | Create scheduled sessions by writing `sessions.scheduled_session_id`; complete them by looking up sessions instead of the old schedule backref. |
| `platform/internal/store/schedule_test.go` | Verify scheduled start/end still updates schedule status correctly after the flip. |
| `platform/internal/handlers/schedule.go` | Keep any response payloads or status checks aligned with the new canonical linkage. |
| `platform/internal/handlers/schedule_test.go` | Cover handler behavior after the backref flip. |
| `docs/api.md` | Update any schedule/session linkage examples if they still mention `liveSessionId`. |

---

## Task 1: Application Caller Flip

**Files:**
- Modify: `platform/internal/store/schedule.go`
- Modify: `platform/internal/store/schedule_test.go`
- Modify: `platform/internal/handlers/schedule.go`
- Modify: `platform/internal/handlers/schedule_test.go`

- [ ] Change scheduled-session start to populate `sessions.scheduled_session_id`.
- [ ] Change scheduled-session completion to find the schedule through `sessions.scheduled_session_id`, not `scheduled_sessions.live_session_id`.
- [ ] Keep the old schedule column read-only during the transition if needed, but stop relying on it as the source of truth.

**Testing plan:**
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/store -run 'TestScheduleStore_' -count=1`
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./internal/handlers -run 'TestScheduleHandler_' -count=1`

## Task 2: Migration Cleanup

**Files:**
- Create: `drizzle/0015_scheduled_session_backref_cleanup.sql`
- Modify: `docs/api.md`

- [ ] Add the drop migration only after Task 1 is merged and verified.
- [ ] Remove the obsolete `live_session_id` column, index, and foreign key from `scheduled_sessions`.
- [ ] Update docs so `scheduledSessionId` is the only documented lineage field.

**Testing plan:**
- `psql postgresql://work@127.0.0.1:5432/bridge -f drizzle/0015_scheduled_session_backref_cleanup.sql`
- `psql postgresql://work@127.0.0.1:5432/bridge_test -f drizzle/0015_scheduled_session_backref_cleanup.sql`
- `psql postgresql://work@127.0.0.1:5432/bridge -c "\\d scheduled_sessions"`

## Task 3: Whole-Phase Verification

**Files:**
- Modify: `docs/plans/030e-session-scheduled-backref.md`

- [ ] Re-run the full schedule and session test suites after the cleanup migration.
- [ ] Confirm there are no remaining `live_session_id` reads outside migration history and plan/spec docs.

**Verification commands:**
- `rg -n 'live_session_id' platform src docs tests drizzle`
- `env GOCACHE=/tmp/magicburg-go-build-cache go test ./... -count=1 -timeout 120s`

---

## Code Review

Reviewers append findings here following `docs/code-review.md`. Author responds inline with `→ Response:` and updates status to `[FIXED]` or `[WONTFIX]`.

## Post-Execution Report

Populate during Step 6 of `docs/development-workflow.md` after this phase ships.
