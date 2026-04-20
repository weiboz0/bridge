-- Plan 027: normalize legacy classroom references, then drop the tables.
--
-- Migration chain gap: `drizzle/0001_eminent_loa.sql:5-21` creates
-- live_sessions.classroom_id with a FK to classrooms, and the column
-- was renamed to class_id directly in dev during plan 022's session
-- work — but no Drizzle migration captured the rename. Similarly,
-- `drizzle/0005_code-persistence.sql:7` adds documents.classroom_id,
-- which is unused now. So a fresh DB built from migrations would
-- enter plan 027's drop of `classrooms` with dangling FKs.
--
-- This migration:
--   1. Repoints live_sessions from classrooms to classes (rename column +
--      reparent FK + rename indexes).
--   2. Drops the vestigial documents.classroom_id column.
--   3. Drops the legacy classrooms + classroom_members tables.
--
-- Every step is idempotent: dev / test DBs that already completed the
-- rename at runtime see no-op branches via IF EXISTS guards.

BEGIN;

-- 1. live_sessions: classroom_id → class_id, reparent FK
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'live_sessions' AND column_name = 'classroom_id'
  ) THEN
    ALTER TABLE live_sessions
      DROP CONSTRAINT IF EXISTS live_sessions_classroom_id_classrooms_id_fk;
    DROP INDEX IF EXISTS live_sessions_classroom_idx;
    DROP INDEX IF EXISTS live_sessions_status_idx;

    ALTER TABLE live_sessions RENAME COLUMN classroom_id TO class_id;

    ALTER TABLE live_sessions
      ADD CONSTRAINT live_sessions_class_id_classes_id_fk
      FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS live_sessions_class_idx
  ON live_sessions(class_id);
CREATE INDEX IF NOT EXISTS live_sessions_class_status_idx
  ON live_sessions(class_id, status);

-- 2. documents.classroom_id: unused; drop the column + its index
DROP INDEX IF EXISTS documents_classroom_idx;
ALTER TABLE documents DROP COLUMN IF EXISTS classroom_id;

-- 3. Drop the legacy pre-course-hierarchy tables
DROP TABLE IF EXISTS classroom_members CASCADE;
DROP TABLE IF EXISTS classrooms CASCADE;

COMMIT;
