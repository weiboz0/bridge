BEGIN;

CREATE TYPE "public"."book_scope" AS ENUM ('platform', 'org');

CREATE TABLE "books" (
  "id"          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "title"       varchar(255) NOT NULL,
  "description" text NOT NULL DEFAULT '',
  "scope"       book_scope NOT NULL,
  "scope_id"    uuid NULL,
  "created_by"  uuid NOT NULL REFERENCES users(id),
  "created_at"  timestamptz NOT NULL DEFAULT now(),
  "updated_at"  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT books_scope_id_required CHECK (
    (scope = 'platform' AND scope_id IS NULL) OR
    (scope = 'org'      AND scope_id IS NOT NULL)
  )
);

CREATE INDEX books_scope_idx ON books(scope, scope_id);
CREATE INDEX books_created_by_idx ON books(created_by);

ALTER TABLE teaching_units RENAME TO chapters;
ALTER INDEX teaching_units_pkey RENAME TO chapters_pkey;
ALTER INDEX teaching_units_created_by_idx RENAME TO chapters_created_by_idx;
ALTER INDEX teaching_units_scope_scope_id_status_idx RENAME TO chapters_scope_scope_id_status_idx;
ALTER INDEX teaching_units_scope_slug_uniq RENAME TO chapters_scope_slug_uniq;
ALTER INDEX teaching_units_search_idx RENAME TO chapters_search_idx;
ALTER INDEX teaching_units_standards_tags_gin_idx RENAME TO chapters_standards_tags_gin_idx;
ALTER INDEX teaching_units_subject_tags_gin_idx RENAME TO chapters_subject_tags_gin_idx;
ALTER INDEX teaching_units_topic_id_uniq RENAME TO chapters_topic_id_uniq;
ALTER TABLE chapters RENAME CONSTRAINT teaching_units_scope_scope_id_chk TO chapters_scope_scope_id_chk;
ALTER TABLE chapters RENAME CONSTRAINT teaching_units_status_chk TO chapters_status_chk;
ALTER TABLE chapters RENAME CONSTRAINT teaching_units_created_by_fkey TO chapters_created_by_fkey;
ALTER TABLE chapters RENAME CONSTRAINT teaching_units_topic_id_fkey TO chapters_topic_id_fkey;

ALTER TABLE unit_documents RENAME TO chapter_documents;
ALTER TABLE unit_revisions RENAME TO chapter_revisions;
ALTER TABLE unit_overlays RENAME TO chapter_overlays;
ALTER TABLE unit_collection_items RENAME TO chapter_collection_items;
ALTER TABLE unit_collections RENAME TO chapter_collections;

ALTER TABLE chapter_documents RENAME COLUMN unit_id TO chapter_id;
ALTER TABLE chapter_revisions RENAME COLUMN unit_id TO chapter_id;
ALTER TABLE chapter_overlays RENAME COLUMN parent_unit_id TO parent_chapter_id;
ALTER TABLE chapter_overlays RENAME COLUMN child_unit_id TO child_chapter_id;
ALTER TABLE chapter_collection_items RENAME COLUMN unit_id TO chapter_id;

ALTER INDEX unit_documents_pkey RENAME TO chapter_documents_pkey;
ALTER INDEX unit_revisions_pkey RENAME TO chapter_revisions_pkey;
ALTER INDEX unit_revisions_unit_created_idx RENAME TO chapter_revisions_chapter_created_idx;
ALTER INDEX unit_overlays_pkey RENAME TO chapter_overlays_pkey;
ALTER INDEX unit_overlays_parent_idx RENAME TO chapter_overlays_parent_idx;
ALTER INDEX unit_collections_pkey RENAME TO chapter_collections_pkey;
ALTER INDEX unit_collections_scope_idx RENAME TO chapter_collections_scope_idx;
ALTER INDEX unit_collection_items_pkey RENAME TO chapter_collection_items_pkey;
ALTER TABLE chapter_collections RENAME CONSTRAINT unit_collections_scope_chk TO chapter_collections_scope_chk;

ALTER TABLE chapters ADD COLUMN "book_id" uuid NULL REFERENCES books(id) ON DELETE SET NULL;
CREATE INDEX chapters_book_idx ON chapters(book_id);

COMMIT;
