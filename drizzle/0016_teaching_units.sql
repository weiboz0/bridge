-- Plan 031 / spec 012: teaching units core schema.
-- Introduces teaching_units, unit_documents, unit_revisions.
-- No lifecycle transitions in this plan — status is a stored field
-- and no code writes to unit_revisions yet.
-- Idempotent: IF EXISTS / IF NOT EXISTS guards throughout so dev
-- DBs mid-state apply cleanly.

BEGIN;

CREATE TABLE IF NOT EXISTS teaching_units (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  scope               varchar(16) NOT NULL,
  scope_id            uuid,
  title               varchar(255) NOT NULL,
  slug                varchar(255),
  summary             text NOT NULL DEFAULT '',
  grade_level         varchar(8),
  subject_tags        text[] NOT NULL DEFAULT '{}',
  standards_tags      text[] NOT NULL DEFAULT '{}',
  estimated_minutes   int,
  status              varchar(24) NOT NULL DEFAULT 'draft',
  created_by          uuid NOT NULL REFERENCES users(id),
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT teaching_units_scope_scope_id_chk CHECK (
    (scope = 'platform' AND scope_id IS NULL) OR
    (scope IN ('org', 'personal') AND scope_id IS NOT NULL)
  ),
  CONSTRAINT teaching_units_status_chk CHECK (
    status IN ('draft', 'reviewed', 'classroom_ready', 'coach_ready', 'archived')
  )
);

CREATE INDEX IF NOT EXISTS teaching_units_scope_scope_id_status_idx
  ON teaching_units(scope, scope_id, status);
CREATE INDEX IF NOT EXISTS teaching_units_created_by_idx
  ON teaching_units(created_by);
CREATE UNIQUE INDEX IF NOT EXISTS teaching_units_scope_slug_uniq
  ON teaching_units(scope, COALESCE(scope_id::text, ''), slug) WHERE slug IS NOT NULL;
CREATE INDEX IF NOT EXISTS teaching_units_subject_tags_gin_idx
  ON teaching_units USING GIN (subject_tags);
CREATE INDEX IF NOT EXISTS teaching_units_standards_tags_gin_idx
  ON teaching_units USING GIN (standards_tags);

CREATE TABLE IF NOT EXISTS unit_documents (
  unit_id    uuid PRIMARY KEY REFERENCES teaching_units(id) ON DELETE CASCADE,
  blocks     jsonb NOT NULL DEFAULT '{"type":"doc","content":[]}'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS unit_revisions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  unit_id     uuid NOT NULL REFERENCES teaching_units(id) ON DELETE CASCADE,
  blocks      jsonb NOT NULL,
  reason      varchar(255),
  created_by  uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS unit_revisions_unit_created_idx
  ON unit_revisions(unit_id, created_at DESC);

COMMIT;
