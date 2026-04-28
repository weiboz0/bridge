-- Plan 046: drop the deprecated topic columns one release after plan 044
-- locked the writes and removed all reads. Pre-plan Phase 0 discovery
-- confirmed no topic has non-empty lesson_content / starter_code, every
-- topic with teaching material has a linked teaching_unit (via topic_id),
-- and no view / trigger / index depends on these columns.
--
-- Plain DROP COLUMN (not IF EXISTS): if the column doesn't exist, that's
-- a state we want to be loud about — it means a previous run partially
-- succeeded or the schema drifted, both of which deserve manual review.
ALTER TABLE topics DROP COLUMN lesson_content;
ALTER TABLE topics DROP COLUMN starter_code;
