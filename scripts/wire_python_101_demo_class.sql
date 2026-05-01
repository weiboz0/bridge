-- Plan 049 Phase 6 — wire the demo class to Bridge HQ Python 101.
--
-- Idempotent. Re-runs are no-ops.
--
-- The demo class (well-known UUID 00000000-0000-0000-0000-000000400101)
-- in Bridge Demo School points at Bridge HQ's canonical Python 101
-- course (id 8935aea2-e208-48d6-b5fa-56e54d1dc451).
--
-- Why no clone: plan 049 originally called for `Courses.CloneCourse`
-- followed by a course_id swap. CloneCourse intentionally drops the
-- topic→unit links during the clone (per plan 044's 1:1 invariant —
-- only one course can claim a given platform-scope unit), so the
-- cloned course's topics would render without teaching content.
-- Pointing the demo class directly at Bridge HQ's course is the
-- simplest correct behavior: there's no DB constraint requiring
-- classes.org_id == courses.org_id, and the Bridge HQ course is the
-- intended canonical publisher of Python 101. Cross-org "subscribe"
-- semantics for course reuse is a future plan (050+).
--
-- Apply:
--   psql postgresql://work@127.0.0.1:5432/bridge -f scripts/wire_python_101_demo_class.sql

-- Sanity: refuse to run if Bridge HQ Python 101 hasn't been imported.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM courses WHERE id = '8935aea2-e208-48d6-b5fa-56e54d1dc451') THEN
    RAISE EXCEPTION 'Bridge HQ Python 101 course not found. Run `bun run scripts/python-101/import.ts --apply` first.';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM classes WHERE id = '00000000-0000-0000-0000-000000400101') THEN
    RAISE NOTICE 'Demo class 00000000-0000-0000-0000-000000400101 does not exist; nothing to wire.';
    RETURN;
  END IF;
END$$;

UPDATE classes
SET course_id = '8935aea2-e208-48d6-b5fa-56e54d1dc451',
    updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000400101'
  AND course_id IS DISTINCT FROM '8935aea2-e208-48d6-b5fa-56e54d1dc451';

-- Verify.
SELECT
  c.id AS class_id,
  c.title AS class_title,
  co.id AS course_id,
  co.title AS course_title
FROM classes c
JOIN courses co ON co.id = c.course_id
WHERE c.id = '00000000-0000-0000-0000-000000400101';
