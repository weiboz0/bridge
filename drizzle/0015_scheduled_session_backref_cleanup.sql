BEGIN;

DO $$
DECLARE
  fk_name text;
BEGIN
  SELECT con.conname
  INTO fk_name
  FROM pg_constraint con
  JOIN pg_class rel ON rel.oid = con.conrelid
  JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
  JOIN pg_attribute att
    ON att.attrelid = rel.oid
   AND att.attnum = ANY(con.conkey)
  WHERE nsp.nspname = 'public'
    AND rel.relname = 'scheduled_sessions'
    AND con.contype = 'f'
    AND att.attname = 'live_session_id'
  LIMIT 1;

  IF fk_name IS NOT NULL THEN
    EXECUTE format(
      'ALTER TABLE public.scheduled_sessions DROP CONSTRAINT IF EXISTS %I',
      fk_name
    );
  END IF;
END $$;

DO $$
DECLARE
  idx record;
BEGIN
  FOR idx IN
    SELECT DISTINCT idx_nsp.nspname AS schema_name, idx_cls.relname AS index_name
    FROM pg_index pg_idx
    JOIN pg_class idx_cls ON idx_cls.oid = pg_idx.indexrelid
    JOIN pg_namespace idx_nsp ON idx_nsp.oid = idx_cls.relnamespace
    JOIN pg_class tbl_cls ON tbl_cls.oid = pg_idx.indrelid
    JOIN pg_namespace tbl_nsp ON tbl_nsp.oid = tbl_cls.relnamespace
    JOIN pg_attribute att
      ON att.attrelid = tbl_cls.oid
     AND att.attnum = ANY(pg_idx.indkey)
    WHERE tbl_nsp.nspname = 'public'
      AND tbl_cls.relname = 'scheduled_sessions'
      AND att.attname = 'live_session_id'
  LOOP
    EXECUTE format(
      'DROP INDEX IF EXISTS %I.%I',
      idx.schema_name,
      idx.index_name
    );
  END LOOP;
END $$;

ALTER TABLE IF EXISTS public.scheduled_sessions
  DROP COLUMN IF EXISTS live_session_id;

COMMIT;
