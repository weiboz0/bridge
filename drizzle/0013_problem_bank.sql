-- Plan 028 / spec 009: reshape `problems` into a scoped bank with many-to-many
-- topic attachment, and introduce `problem_solutions` for reference answers.
--
-- Existing rows (today: topic_bound, language-specific) are backfilled as
-- org-scoped problems attached to the same topic they came from, with the
-- starter_code migrated into a jsonb map keyed by the old language column.
-- Single transaction; idempotent where practical so dev DBs mid-state apply
-- cleanly.

BEGIN;

-- 1. Add new columns (nullable initially for backfill safety).
ALTER TABLE problems
  ADD COLUMN IF NOT EXISTS scope           varchar(16),
  ADD COLUMN IF NOT EXISTS scope_id        uuid,
  ADD COLUMN IF NOT EXISTS slug            varchar(255),
  ADD COLUMN IF NOT EXISTS difficulty      varchar(16),
  ADD COLUMN IF NOT EXISTS grade_level     varchar(8),
  ADD COLUMN IF NOT EXISTS tags            text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS status          varchar(16),
  ADD COLUMN IF NOT EXISTS forked_from     uuid REFERENCES problems(id),
  ADD COLUMN IF NOT EXISTS time_limit_ms   int,
  ADD COLUMN IF NOT EXISTS memory_limit_mb int;

-- starter_code_json only needed when migration is fresh (i.e., starter_code is still text).
-- After rename, starter_code is jsonb — skip adding the temp column to avoid clobbering it.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'problems' AND column_name = 'starter_code' AND data_type = 'jsonb'
  ) THEN
    ALTER TABLE problems
      ADD COLUMN IF NOT EXISTS starter_code_json jsonb NOT NULL DEFAULT '{}'::jsonb;
  END IF;
END $$;

-- 2. Backfill scope / scope_id / status / difficulty / grade_level / starter_code_json.
-- Guarded: skip if topic_id is already dropped (re-run safety).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'problems' AND column_name = 'topic_id'
  ) THEN
    UPDATE problems p
    SET
      scope = 'org',
      scope_id = c.org_id,
      status = 'published',
      difficulty = COALESCE(p.difficulty, 'easy'),
      grade_level = c.grade_level,
      starter_code_json = CASE
        WHEN p.starter_code IS NULL OR p.starter_code = ''
          THEN '{}'::jsonb
        ELSE jsonb_build_object(p.language, p.starter_code)
      END
    FROM topics t, courses c
    WHERE p.topic_id = t.id AND t.course_id = c.id;
  END IF;
END $$;

-- 3. Tighten constraints now that every row has values.
-- scope / difficulty / status: SET NOT NULL is a no-op if already NOT NULL (safe to repeat).
ALTER TABLE problems
  ALTER COLUMN scope      SET NOT NULL,
  ALTER COLUMN difficulty SET NOT NULL,
  ALTER COLUMN status     SET NOT NULL;
-- starter_code_json: only enforce NOT NULL before rename (column may already be 'starter_code').
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'problems' AND column_name = 'starter_code_json'
  ) THEN
    ALTER TABLE problems ALTER COLUMN starter_code_json SET NOT NULL;
  END IF;
END $$;

-- scope_id stays nullable (platform scope uses NULL scope_id).
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'problems_scope_scope_id_chk'
      AND table_name = 'problems'
  ) THEN
    ALTER TABLE problems
      ADD CONSTRAINT problems_scope_scope_id_chk CHECK (
        (scope = 'platform' AND scope_id IS NULL) OR
        (scope IN ('org', 'personal') AND scope_id IS NOT NULL)
      );
  END IF;
END $$;

-- 4. Create topic_problems and backfill from the current problems.topic_id.
CREATE TABLE IF NOT EXISTS topic_problems (
  topic_id    uuid NOT NULL REFERENCES topics(id)   ON DELETE CASCADE,
  problem_id  uuid NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
  sort_order  int  NOT NULL DEFAULT 0,
  attached_by uuid NOT NULL REFERENCES users(id),
  attached_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (topic_id, problem_id)
);
CREATE INDEX IF NOT EXISTS topic_problems_problem_idx ON topic_problems(problem_id);

-- Backfill topic_problems from problems.topic_id (guarded: skip if column already dropped).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'problems' AND column_name = 'topic_id'
  ) THEN
    INSERT INTO topic_problems (topic_id, problem_id, sort_order, attached_by, attached_at)
    SELECT p.topic_id, p.id, p."order", p.created_by, p.created_at
    FROM problems p
    ON CONFLICT DO NOTHING;
  END IF;
END $$;

-- 5. Drop the now-redundant columns on problems.
ALTER TABLE problems
  DROP COLUMN IF EXISTS topic_id,
  DROP COLUMN IF EXISTS language,
  DROP COLUMN IF EXISTS "order";

-- Drop the old text starter_code column only — never drop the jsonb one (which is the final form).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'problems' AND column_name = 'starter_code' AND data_type = 'text'
  ) THEN
    ALTER TABLE problems DROP COLUMN starter_code;
  END IF;
END $$;

-- Rename the new jsonb column to its final name (guarded: skip if already renamed).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'problems' AND column_name = 'starter_code_json'
  ) THEN
    ALTER TABLE problems RENAME COLUMN starter_code_json TO starter_code;
  END IF;
END $$;

-- 6. Create problem_solutions (empty at migration).
CREATE TABLE IF NOT EXISTS problem_solutions (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  problem_id     uuid NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
  language       varchar(32) NOT NULL,
  title          varchar(120),
  code           text NOT NULL,
  notes          text,
  approach_tags  text[] NOT NULL DEFAULT '{}',
  is_published   boolean NOT NULL DEFAULT false,
  created_by     uuid NOT NULL REFERENCES users(id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS problem_solutions_problem_language_idx
  ON problem_solutions(problem_id, language);

-- 7. Indexes on the reshaped problems table.
DROP INDEX IF EXISTS problems_topic_order_idx;  -- column gone
CREATE INDEX IF NOT EXISTS problems_scope_scope_id_status_idx
  ON problems(scope, scope_id, status);
CREATE INDEX IF NOT EXISTS problems_created_by_idx ON problems(created_by);
CREATE UNIQUE INDEX IF NOT EXISTS problems_scope_slug_uniq
  ON problems(scope, COALESCE(scope_id::text, ''), slug) WHERE slug IS NOT NULL;
CREATE INDEX IF NOT EXISTS problems_tags_gin_idx ON problems USING GIN (tags);

COMMIT;
