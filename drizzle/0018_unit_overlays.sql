BEGIN;

CREATE TABLE IF NOT EXISTS unit_overlays (
  child_unit_id      uuid PRIMARY KEY REFERENCES teaching_units(id) ON DELETE CASCADE,
  parent_unit_id     uuid NOT NULL REFERENCES teaching_units(id) ON DELETE CASCADE,
  parent_revision_id uuid REFERENCES unit_revisions(id) ON DELETE SET NULL,
  block_overrides    jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS unit_overlays_parent_idx ON unit_overlays(parent_unit_id);

COMMIT;
