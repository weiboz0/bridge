BEGIN;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'live_sessions'
  ) AND NOT EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'sessions'
  ) THEN
    ALTER TABLE live_sessions RENAME TO sessions;
  END IF;
END $$;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'live_sessions_pkey') THEN
    ALTER INDEX live_sessions_pkey RENAME TO sessions_pkey;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'live_sessions_class_idx') THEN
    ALTER INDEX live_sessions_class_idx RENAME TO sessions_class_idx;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'live_sessions_class_status_idx') THEN
    ALTER INDEX live_sessions_class_status_idx RENAME TO sessions_class_status_idx;
  END IF;
END $$;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_type t
    JOIN pg_enum e ON e.enumtypid = t.oid
    WHERE t.typname = 'session_status' AND e.enumlabel = 'active'
  ) AND NOT EXISTS (
    SELECT 1
    FROM pg_type t
    JOIN pg_enum e ON e.enumtypid = t.oid
    WHERE t.typname = 'session_status' AND e.enumlabel = 'live'
  ) THEN
    ALTER TYPE session_status RENAME VALUE 'active' TO 'live';
  END IF;
END $$;

ALTER TABLE sessions
  ALTER COLUMN class_id DROP NOT NULL;

ALTER TABLE sessions
  ADD COLUMN IF NOT EXISTS title varchar(255),
  ADD COLUMN IF NOT EXISTS invite_token varchar(24),
  ADD COLUMN IF NOT EXISTS invite_expires_at timestamptz,
  ADD COLUMN IF NOT EXISTS scheduled_session_id uuid,
  ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

UPDATE sessions s
SET title = COALESCE(c.title, 'Untitled session')
FROM classes c
WHERE c.id = s.class_id
  AND (s.title IS NULL OR s.title = '');

UPDATE sessions
SET title = 'Untitled session'
WHERE title IS NULL OR title = '';

ALTER TABLE sessions
  ALTER COLUMN title SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS sessions_invite_token_idx
  ON sessions(invite_token) WHERE invite_token IS NOT NULL;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'scheduled_sessions'
  ) AND NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'sessions_scheduled_session_id_fkey'
  ) THEN
    ALTER TABLE sessions
      ADD CONSTRAINT sessions_scheduled_session_id_fkey
      FOREIGN KEY (scheduled_session_id)
      REFERENCES scheduled_sessions(id)
      ON DELETE SET NULL;
  END IF;
END $$;

ALTER TABLE session_participants
  ADD COLUMN IF NOT EXISTS invited_by uuid,
  ADD COLUMN IF NOT EXISTS invited_at timestamptz,
  ADD COLUMN IF NOT EXISTS help_requested_at timestamptz;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'session_participants'
      AND column_name = 'student_id'
  ) THEN
    ALTER TABLE session_participants RENAME COLUMN student_id TO user_id;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'session_participants_invited_by_users_id_fk'
  ) THEN
    ALTER TABLE session_participants
      ADD CONSTRAINT session_participants_invited_by_users_id_fk
      FOREIGN KEY (invited_by)
      REFERENCES users(id)
      ON DELETE SET NULL;
  END IF;
END $$;

ALTER TABLE session_participants
  ALTER COLUMN joined_at DROP NOT NULL;

UPDATE session_participants
SET help_requested_at = COALESCE(help_requested_at, now())
WHERE status::text = 'needs_help';

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_type t
    JOIN pg_enum e ON e.enumtypid = t.oid
    WHERE t.typname = 'participant_status' AND e.enumlabel = 'needs_help'
  ) THEN
    ALTER TABLE session_participants ALTER COLUMN status DROP DEFAULT;

    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'participant_status_v2') THEN
      CREATE TYPE participant_status_v2 AS ENUM ('invited', 'present', 'left');
    END IF;

    ALTER TABLE session_participants
      ALTER COLUMN status TYPE participant_status_v2
      USING (
        CASE
          WHEN status::text IN ('active', 'idle', 'needs_help', 'present') THEN 'present'::participant_status_v2
          WHEN status::text = 'invited' THEN 'invited'::participant_status_v2
          WHEN status::text = 'left' THEN 'left'::participant_status_v2
          ELSE 'present'::participant_status_v2
        END
      );

    DROP TYPE participant_status;
    ALTER TYPE participant_status_v2 RENAME TO participant_status;
    ALTER TABLE session_participants ALTER COLUMN status SET DEFAULT 'present';
  END IF;
END $$;

COMMIT;
