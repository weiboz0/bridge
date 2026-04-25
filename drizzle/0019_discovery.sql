BEGIN;

-- FTS index on teaching_units
ALTER TABLE teaching_units
  ADD COLUMN IF NOT EXISTS search_vector tsvector
    GENERATED ALWAYS AS (
      to_tsvector('english', coalesce(title, '') || ' ' || coalesce(summary, ''))
    ) STORED;

CREATE INDEX IF NOT EXISTS teaching_units_search_idx
  ON teaching_units USING GIN (search_vector);

-- Quality signal columns (schema only — no capture pipeline yet)
ALTER TABLE teaching_units
  ADD COLUMN IF NOT EXISTS usage_count int NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS avg_rating numeric(3,2);

-- Unit collections
CREATE TABLE IF NOT EXISTS unit_collections (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  scope       varchar(16) NOT NULL,
  scope_id    uuid,
  title       varchar(255) NOT NULL,
  description text NOT NULL DEFAULT '',
  created_by  uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT unit_collections_scope_chk CHECK (
    (scope = 'platform' AND scope_id IS NULL) OR
    (scope IN ('org', 'personal') AND scope_id IS NOT NULL)
  )
);

CREATE INDEX IF NOT EXISTS unit_collections_scope_idx
  ON unit_collections(scope, scope_id);

CREATE TABLE IF NOT EXISTS unit_collection_items (
  collection_id uuid NOT NULL REFERENCES unit_collections(id) ON DELETE CASCADE,
  unit_id       uuid NOT NULL REFERENCES teaching_units(id) ON DELETE CASCADE,
  sort_order    int NOT NULL DEFAULT 0,
  PRIMARY KEY (collection_id, unit_id)
);

COMMIT;
