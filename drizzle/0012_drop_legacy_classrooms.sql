-- Drop the pre-course-hierarchy classroom tables (Plan 027).
--
-- Superseded by classes + class_memberships + class_settings from
-- plan 004 onward. The 2 rows here are test artifacts from before
-- the portal shipped; no live code path still references them (audit
-- by grep for `classrooms` and `classroom_members` in src/, platform/,
-- and server/ returns only the references that were removed in
-- earlier 027 commits).

DROP TABLE IF EXISTS classroom_members CASCADE;
DROP TABLE IF EXISTS classrooms CASCADE;
