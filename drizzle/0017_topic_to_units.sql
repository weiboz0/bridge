-- Plan 032 / spec 012: Convert existing topics into teaching units.
--
-- 1. Add teaching_units.topic_id (nullable FK, unique) for the compatibility shim.
-- 2. For each topic without a corresponding unit, create a teaching_unit + unit_document
--    with prose block (from lesson_content) and problem-ref blocks (from topic_problems).
--
-- Idempotent: re-running skips topics that already have units.

BEGIN;

-- 1. Add topic_id column if not present.
ALTER TABLE teaching_units
  ADD COLUMN IF NOT EXISTS topic_id uuid REFERENCES topics(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS teaching_units_topic_id_uniq
  ON teaching_units(topic_id) WHERE topic_id IS NOT NULL;

-- 2. Convert each topic into a teaching unit + unit document.
-- Uses a DO block so we can loop and build per-topic block JSON.
DO $$
DECLARE
  t RECORD;
  new_unit_id uuid;
  blocks jsonb;
  prob RECORD;
  block_idx int;
BEGIN
  FOR t IN
    SELECT
      top.id AS topic_id,
      top.title,
      top.description,
      top.lesson_content,
      top.sort_order,
      top.created_at,
      c.org_id,
      c.grade_level,
      c.created_by
    FROM topics top
    JOIN courses c ON c.id = top.course_id
    WHERE NOT EXISTS (
      SELECT 1 FROM teaching_units tu WHERE tu.topic_id = top.id
    )
  LOOP
    new_unit_id := gen_random_uuid();

    -- Start with empty doc
    blocks := '[]'::jsonb;
    block_idx := 0;

    -- If lesson_content is non-empty (not '{}' or null), add a prose block
    IF t.lesson_content IS NOT NULL
       AND t.lesson_content::text != '{}'
       AND t.lesson_content::text != 'null'
       AND length(t.lesson_content::text) > 2
    THEN
      blocks := blocks || jsonb_build_array(
        jsonb_build_object(
          'type', 'paragraph',
          'attrs', jsonb_build_object('id', 'b' || lpad(block_idx::text, 3, '0')),
          'content', jsonb_build_array(
            jsonb_build_object('type', 'text', 'text', t.lesson_content::text)
          )
        )
      );
      block_idx := block_idx + 1;
    END IF;

    -- Add a problem-ref block for each topic_problem
    FOR prob IN
      SELECT tp.problem_id, tp.sort_order
      FROM topic_problems tp
      WHERE tp.topic_id = t.topic_id
      ORDER BY tp.sort_order ASC
    LOOP
      blocks := blocks || jsonb_build_array(
        jsonb_build_object(
          'type', 'problem-ref',
          'attrs', jsonb_build_object(
            'id', 'b' || lpad(block_idx::text, 3, '0'),
            'problemId', prob.problem_id,
            'pinnedRevision', null,
            'visibility', 'always',
            'overrideStarter', null
          )
        )
      );
      block_idx := block_idx + 1;
    END LOOP;

    -- Insert teaching_unit
    INSERT INTO teaching_units (
      id, scope, scope_id, title, slug, summary, grade_level,
      subject_tags, standards_tags, estimated_minutes,
      status, created_by, topic_id, created_at, updated_at
    ) VALUES (
      new_unit_id, 'org', t.org_id, t.title, NULL,
      COALESCE(t.description, ''), t.grade_level,
      '{}', '{}', NULL,
      'classroom_ready', t.created_by, t.topic_id,
      t.created_at, t.created_at
    );

    -- Insert unit_document with the assembled blocks
    INSERT INTO unit_documents (unit_id, blocks, updated_at)
    VALUES (
      new_unit_id,
      jsonb_build_object('type', 'doc', 'content', blocks),
      t.created_at
    );
  END LOOP;
END $$;

COMMIT;
