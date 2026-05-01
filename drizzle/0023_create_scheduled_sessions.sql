-- Plan 058 — backfill the missing `scheduled_sessions` migration.
--
-- The table was created out-of-band during plan 030's session-model
-- work. `platform/internal/store/schedule.go` reads/writes it,
-- migration 0014 references it via FK with an `IF EXISTS` guard, and
-- migration 0015 cleans up its constraints — but no migration in
-- `drizzle/` ever creates it. A fresh DB cannot bootstrap the app.
--
-- This migration is a backfill: every clause is idempotent so the
-- dev DB and `bridge_test` (both hand-created) no-op cleanly, while
-- a fresh DB gets the type, table, indexes, and the FK that 0014
-- conditionally skipped.

BEGIN;

-- ---------- ENUM: schedule_status ----------
-- Pattern: DO BEGIN CREATE TYPE ... EXCEPTION WHEN duplicate_object
-- THEN NULL; END (idiom from drizzle/0021_user_intended_role.sql:4-9).
-- pg has no `CREATE TYPE ... IF NOT EXISTS`.

DO $$ BEGIN
  CREATE TYPE schedule_status AS ENUM (
    'planned',
    'in_progress',
    'completed',
    'cancelled'
  );
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

-- ---------- TABLE: scheduled_sessions ----------
-- Shape matches the live dev DB exactly (verified via `\d
-- scheduled_sessions` against the canonical hand-created table).

CREATE TABLE IF NOT EXISTS public.scheduled_sessions (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  class_id        uuid NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
  teacher_id      uuid NOT NULL REFERENCES users(id),
  title           varchar(255),
  scheduled_start timestamptz NOT NULL,
  scheduled_end   timestamptz NOT NULL,
  recurrence      jsonb,
  topic_ids       uuid[],
  status          schedule_status NOT NULL DEFAULT 'planned',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS scheduled_sessions_class_idx
  ON public.scheduled_sessions (class_id);

CREATE INDEX IF NOT EXISTS scheduled_sessions_start_idx
  ON public.scheduled_sessions (scheduled_start);

CREATE INDEX IF NOT EXISTS scheduled_sessions_status_idx
  ON public.scheduled_sessions (class_id, status);

-- ---------- FK: sessions.scheduled_session_id ----------
-- Migration 0014_session_model.sql:75-92 added this FK only when
-- scheduled_sessions already existed. On a fresh DB chain the table
-- doesn't exist at the time 0014 runs, so the FK gets skipped. Now
-- that 0023 has created the table, attach the FK if it isn't
-- present. Constraint name matches what 0014 used.

DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'sessions_scheduled_session_id_fkey'
  ) THEN
    ALTER TABLE sessions
      ADD CONSTRAINT sessions_scheduled_session_id_fkey
      FOREIGN KEY (scheduled_session_id)
      REFERENCES scheduled_sessions(id)
      ON DELETE SET NULL;
  END IF;
END $$;

COMMIT;
