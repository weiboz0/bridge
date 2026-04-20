-- Rename new_classrooms → class_settings (Plan 027).
--
-- Same rows, clearer name. The "new_" prefix dates back to the
-- pre-course-hierarchy migration when this table coexisted with
-- the legacy `classrooms`. Now that `classrooms` is being dropped
-- (0012_drop_legacy_classrooms.sql), the prefix is dead weight.

ALTER TABLE new_classrooms RENAME TO class_settings;

-- Postgres renames the table but keeps the original index/constraint
-- names. Rename for clarity so future grep-and-find works.
ALTER INDEX new_classrooms_pkey       RENAME TO class_settings_pkey;
ALTER INDEX new_classrooms_class_idx  RENAME TO class_settings_class_idx;
ALTER INDEX new_classrooms_class_id_key RENAME TO class_settings_class_id_key;
ALTER TABLE class_settings
  RENAME CONSTRAINT new_classrooms_class_id_fkey TO class_settings_class_id_fkey;
