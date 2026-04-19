-- Migration: Yjs CRDT state column for attempts (spec 007 / plan 025b)

ALTER TABLE attempts ADD COLUMN yjs_state text;
