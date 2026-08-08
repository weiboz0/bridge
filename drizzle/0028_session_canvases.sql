BEGIN;

CREATE TYPE canvas_visibility AS ENUM ('private', 'host', 'participants', 'session');

ALTER TABLE sessions
  ADD COLUMN canvas_floor canvas_visibility;

UPDATE sessions
SET canvas_floor = 'private'
WHERE canvas_floor IS NULL;

ALTER TABLE sessions
  ALTER COLUMN canvas_floor SET DEFAULT 'private',
  ALTER COLUMN canvas_floor SET NOT NULL;

CREATE TABLE session_canvases (
  id uuid PRIMARY KEY,
  session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL REFERENCES users(id),
  title varchar(255) NOT NULL,
  visibility canvas_visibility NOT NULL,
  yjs_state text,
  plain_text text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX session_canvases_session_idx ON session_canvases (session_id);
CREATE INDEX session_canvases_session_owner_idx ON session_canvases (session_id, owner_id);

COMMIT;
