-- Plan 064 — Parent-child linking schema.
--
-- Replaces the implicit `class_memberships role="parent"` model
-- (which leaked: any parent in a class could see all students in
-- the class, not just their child).
--
-- Status enum: 'active' grants IsParentOf; 'revoked' stays in the
-- table for audit but doesn't grant access. Re-linking after revoke
-- is supported via the partial unique below: a revoked row coexists
-- with a fresh active row for the same (parent, child) pair.

CREATE TABLE IF NOT EXISTS parent_links (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    child_user_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          varchar(16) NOT NULL DEFAULT 'active',
    created_by      uuid NOT NULL REFERENCES users(id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    revoked_at      timestamptz,
    CONSTRAINT parent_links_status_check
        CHECK (status IN ('active', 'revoked')),
    CONSTRAINT parent_links_no_self_link
        CHECK (parent_user_id <> child_user_id)
);

CREATE INDEX IF NOT EXISTS parent_links_parent_idx
    ON parent_links (parent_user_id);

CREATE INDEX IF NOT EXISTS parent_links_child_idx
    ON parent_links (child_user_id);

-- Partial unique: at most one ACTIVE link per (parent, child).
-- Revoked rows can coexist; re-linking after revoke inserts a new
-- row.
CREATE UNIQUE INDEX IF NOT EXISTS parent_links_active_uniq
    ON parent_links (parent_user_id, child_user_id)
    WHERE status = 'active';
