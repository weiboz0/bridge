# Plan 058 — Create the missing `scheduled_sessions` migration (P0, release-blocker)

## Status

- **Date:** 2026-05-01
- **Severity:** P0 (release-blocker — fresh DB cannot bootstrap)
- **Origin:** `docs/reviews/009-deep-codebase-review-2026-04-30.md` §P0-2.

## Problem

`scheduled_sessions` is referenced everywhere — `platform/internal/store/schedule.go:54-291` does INSERT/SELECT/UPDATE/DELETE against it; migration `drizzle/0014_session_model.sql:75-92` adds an FK from `sessions.scheduled_session_id`; migration `drizzle/0015_scheduled_session_backref_cleanup.sql` cleans up its constraints; tests at `platform/internal/store/schedule_test.go:32-328` exercise it; the live dev DB has the table.

**No migration creates it.** `git grep -n "CREATE TABLE.*scheduled_sessions" drizzle/` returns zero. The dev DB was hand-created, probably during plan 030's session-model work, and someone forgot to capture the DDL.

Failure modes on a clean checkout:

1. `bun run db:migrate` against a fresh DB completes without error (the guard `DO $$ IF EXISTS ('scheduled_sessions')` skips the FK).
2. The Go API starts; the user creates a class.
3. The user opens the schedule UI / hits any `schedule.go` endpoint.
4. PostgreSQL: `ERROR: relation "scheduled_sessions" does not exist`.

This is reproducible from any clean clone. CI doesn't catch it because CI tests run against `bridge_test` which has been hand-created on the dev box.

## Out of scope

- Refactoring `schedule.go` itself.
- Removing the `IF EXISTS` guard in 0014 (it will become harmless once 0023 lands first in the chain — but reordering migrations is dangerous; we leave 0014's guard intact).
- Drizzle schema additions for `sessions` / `session_participants` / etc. that may also have drift; tracked separately.

## Approach

Add `drizzle/0023_create_scheduled_sessions.sql` that:

1. `CREATE TYPE schedule_status AS ENUM ('planned', 'in_progress', 'completed', 'cancelled')` (wrapped in a `DO $$ BEGIN … EXCEPTION WHEN duplicate_object THEN NULL; END $$` block, since pg `CREATE TYPE` has no `IF NOT EXISTS` syntax).
2. **DO-block pattern citation:** Postgres has two valid idioms for guarded `CREATE TYPE`:
   - `DO $$ BEGIN CREATE TYPE ... AS ENUM (...); EXCEPTION WHEN duplicate_object THEN NULL; END $$;` — precedent at `drizzle/0021_user_intended_role.sql:4-9`. **This is the pattern plan 058 will use.** Simpler and idiomatic for ENUMs.
   - `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = '...') THEN CREATE TYPE ... END IF; END $$;` — precedent at `drizzle/0014_session_model.sql:144-146`. Equivalent but more verbose; not used here.
   The earlier draft's `0014:33-49` reference pointed at an ENUM-value rename, a different operation; corrected.

3. `CREATE TABLE IF NOT EXISTS public.scheduled_sessions` matching the live shape verbatim:
   ```
   id uuid PK DEFAULT gen_random_uuid(),
   class_id uuid NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
   teacher_id uuid NOT NULL REFERENCES users(id),
   title varchar(255),
   scheduled_start timestamptz NOT NULL,
   scheduled_end timestamptz NOT NULL,
   recurrence jsonb,
   topic_ids uuid[],
   status schedule_status NOT NULL DEFAULT 'planned',
   created_at timestamptz NOT NULL DEFAULT now(),
   updated_at timestamptz NOT NULL DEFAULT now()
   ```
4. `CREATE INDEX IF NOT EXISTS` for:
   - `scheduled_sessions_class_idx` on `(class_id)`
   - `scheduled_sessions_start_idx` on `(scheduled_start)`
   - `scheduled_sessions_status_idx` on `(class_id, status)`
5. Re-attempt the `sessions.scheduled_session_id` FK that 0014 conditionally created. The constraint name is `sessions_scheduled_session_id_fkey` (per `0014_session_model.sql:81-90`). Use a `DO $$ ... IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '...')` guard so a clean DB gets it and a hand-created dev DB stays no-op.
6. **Idempotence:** every clause is guarded so the dev DB (hand-created) and bridge_test (also hand-created) both no-op cleanly. ENUM uses the `DO $$ ... EXCEPTION WHEN duplicate_object` pattern from step 1; table/indexes use `CREATE TABLE/INDEX IF NOT EXISTS`; FK uses a `pg_constraint` lookup guard.

**Migration ordering:** 0023 runs AFTER 0014/0015. On a fresh DB:
- 0014 hits the `IF EXISTS (scheduled_sessions)` guard at lines 75-81; table doesn't exist; FK skipped.
- 0015 uses `ALTER TABLE IF EXISTS` with catalog guards; no-op when target absent.
- 0023 creates the type, table, indexes, AND the FK that 0014 wanted to add. No hazard.

Plus parity in Drizzle:
- Add `scheduleStatusEnum` pgEnum to `src/lib/db/schema.ts`.
- Add `scheduledSessions` pgTable matching the live shape.
- Don't run `db:generate` after — that would emit a drift migration since the table already exists in the live DB. Instead, hand-author 0023 to match the live shape exactly.
- **Manual index-sync hazard (Codex IMPORTANT):** schema.ts → migration parity is hand-maintained. Existing tables created this way (`teaching_units` per `drizzle/0016`, `unit_overlays` per `drizzle/0018`) have already produced documented index drift (`docs/reviews/009-deep-codebase-review-2026-04-30.md` §P1-12: three indexes missing from the Drizzle declaration). This plan opens itself to the same risk for `scheduled_sessions`. The Phase 1 regression test (below) catches column drift; index/constraint drift is caught by the same test if it asserts the full set of indexes and FKs, not just columns. A broader drift-checker is a separate plan.

## Files

- Create: `drizzle/0023_create_scheduled_sessions.sql`
- Modify: `src/lib/db/schema.ts` — add the enum + table.
- Add: `tests/integration/schema-scheduled-sessions.test.ts` — assert against `bridge_test`:
  - The `schedule_status` ENUM exists with values `('planned', 'in_progress', 'completed', 'cancelled')` via `pg_enum`.
  - The `scheduled_sessions` table exists with all 11 columns (matching the plan's column list) via `information_schema.columns`. Match the precedent at `drizzle/0014_session_model.sql:101-106`.
  - The three indexes exist via `pg_index` (precedent: `drizzle/0015:33-45`).
  - The FK `sessions_scheduled_session_id_fkey` exists via `pg_constraint` (precedent: `drizzle/0015:7-18`).
  This is the regression test that would have caught the original gap. Codex IMPORTANT: the test must cover ALL FOUR (ENUM values, columns, indexes, FK constraint), not just columns, because manual schema.ts ↔ SQL parity is hand-maintained and the existing `teaching_units` / `unit_overlays` precedents already produced index drift.
- Verify: `cd platform && go test ./internal/store/schedule_test.go -count=1` passes.
- Verify on a **fresh DB**:
  ```bash
  createdb bridge_058_test
  DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_058_test bun run db:migrate
  DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_058_test psql -c "\d scheduled_sessions"
  ```
  The `\d` must show the table with all columns and indexes. Drop the test DB after.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Hand-authored migration drifts from live DB shape | medium | Plan 058's first task verifies live shape (via `psql \d`) and pins the migration to it character-for-character. The integration test in `tests/integration/schema-scheduled-sessions.test.ts` ratchets this. |
| `CREATE TYPE schedule_status` already exists in dev DB | high (would error) | Wrap in `DO $$ BEGIN CREATE TYPE ... EXCEPTION WHEN duplicate_object THEN NULL; END $$` block. Pattern verified at `drizzle/0021_user_intended_role.sql:4-9`. |
| FK addition errors if 0014 already added it on this DB | medium | Use `DO $$ ... IF NOT EXISTS (pg_constraint)` guard, mirroring the same defensive shape 0014 used. |
| Drizzle `db:generate` later regenerates 0023 with subtle differences | low | Document that 0023 is a hand-authored backfill and skip `db:generate` regression. The integration test compares schema-vs-live after every change. |
| Plan 014/015 history breaks if we re-order migrations | n/a | We're not reordering. 0023 lands at the end of the chain. The historical IF-EXISTS guards stay correct. |

## Phases

### Phase 0: pre-impl Codex review

Dispatch `codex:codex-rescue` on this plan focusing on:
- Live-DB shape capture (am I missing any columns/indexes/constraints?).
- The `DO $$ … EXCEPTION WHEN duplicate_object` pattern for `CREATE TYPE` (the `0021` precedent at lines 4-9).
- The FK re-attach idempotence — confirm the `pg_constraint` lookup is the canonical way to gate on FK existence.
- Whether the integration test's introspection query is the right shape.

Capture verdict in `## Codex Review of This Plan` below. Iterate until concur.

### Phase 1: write the migration + Drizzle parity + regression test

- Author `drizzle/0023_create_scheduled_sessions.sql`.
- Add `scheduleStatusEnum` and `scheduledSessions` to `src/lib/db/schema.ts`.
- Add `tests/integration/schema-scheduled-sessions.test.ts`.
- Run on dev DB (no-op expected).
- Run on bridge_test (no-op expected).
- Run on a fresh DB (`createdb bridge_058_test`) and verify the table comes up green.
- `bun run test` + `cd platform && go test ./...` end-to-end.
- Self-review.
- Commit + open PR.

### Phase 2: post-impl Codex review

Dispatch `codex:codex-rescue` on the diff before merge. Resolve findings. Drop the test DB.

## Codex Review of This Plan

### Pass 1 — 2026-05-01

Verdict: **No blockers.** All ordering safe. 2 [IMPORTANT] + 4 [MINOR] addressable inline.

- `[IMPORTANT]` Manual index-sync hazard. Existing hand-migrated tables (`teaching_units` per 0016, `unit_overlays` per 0018) have produced documented index drift (review 009 §P1-12). Plan must call this out and have the regression test cover indexes too. → **Resolved:** Approach section now flags the hazard and points at the broader drift-checker as a separate plan; Phase 1 test now asserts all three (columns, indexes, FK constraint).
- `[IMPORTANT]` Regression test scope was column-only. Should cover constraints + indexes since schema.ts ↔ SQL parity is hand-maintained. → **Resolved:** test description above expanded with the four assertions and cited precedents.
- `[MINOR]` DO-block precedent citation pointed at `0014:33-49` (an enum-value rename) instead of `0014:144-146` and `0021:6-10` (actual `CREATE TYPE` guard with `duplicate_object` handling). → **Resolved:** corrected citation in the Approach section.
- `[MINOR]` Column list is complete (11 columns); `linked_session_id` is computed from `sessions`, not stored. No gap.
- `[MINOR]` FK constraint name `sessions_scheduled_session_id_fkey` matches `0014:81-90`.
- `[MINOR]` Migration ordering is safe: 0014 IF-EXISTS guard, 0015 ALTER IF EXISTS, 0023 creates everything.

Pass-1 verdict: **APPROVE with the [IMPORTANT] fixes folded in.**

### Passes 2–5 — citation tightening

Pass 2 caught a stale 0014:33-49 citation in the Risks table. Pass 3 caught conflation of two distinct `CREATE TYPE` idioms (the IF-NOT-EXISTS pattern at `0014:144-146` vs the EXCEPTION pattern at `0021:4-9`). Pass 4 caught two prose lines still describing the chosen `EXCEPTION WHEN duplicate_object` pattern as `IF NOT EXISTS`. Pass 5 caught one residual "IF NOT EXISTS on every clause" sentence that misrepresented the ENUM step. Each iteration fixed inline; the rounds are documented in the commit history.

### Pass 6 — 2026-05-01: **CONCUR**

> "No remaining blockers found. Ordering is sound, idempotence now covers ENUM/table/index/FK paths, and test scope covers enum values, columns, indexes, and FK constraint."

**Status: ready for implementation.**
