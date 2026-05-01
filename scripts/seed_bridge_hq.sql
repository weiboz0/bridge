-- Plan 049 — Bridge HQ org + system user bootstrap
--
-- Creates the canonical "Bridge HQ" organization and a system user
-- (`system@bridge.platform`) that owns platform-scope reference content
-- like Python 101. Both are inserted with hard-coded UUIDs so the
-- importer (`scripts/python-101/import.ts`) can reference them as
-- well-known constants.
--
-- Idempotent: ON CONFLICT (id) DO NOTHING on BOTH the org row AND the
-- user row. Re-runs are no-ops; a delete-recreate cycle (developer
-- manually deletes the org row but keeps the user, or vice versa) is
-- recovered cleanly.
--
-- Security: the system user is a SERVICE ACCOUNT, not a login account.
-- password_hash is NULL — the credentials-login path at
-- `src/lib/auth.ts:77` rejects users with null password_hash, so this
-- account cannot log in via the UI even with a guess. The user's UUID
-- is referenced for FK fields only (teaching_units.created_by,
-- problems.created_by, topic_problems.attached_by, etc.).
--
-- Apply:
--   psql postgresql://work@127.0.0.1:5432/bridge -f scripts/seed_bridge_hq.sql
--   (and against bridge_test in CI / dev verification)

-- =========================================================
-- Bridge HQ system user
-- =========================================================
-- UUID: 00000000-0000-0000-0000-bbbbbbbbb001 (mnemonic: bbb = "Bridge")
-- platform admin = true so it can author at all scopes
-- intended_role = NULL (service account doesn't onboard)
-- avatar_url = NULL
-- password_hash = NULL (login disabled)

INSERT INTO users (
  id, name, email, password_hash, is_platform_admin,
  intended_role, created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-bbbbbbbbb001',
  'Bridge HQ System',
  'system@bridge.platform',
  NULL,
  true,
  NULL,
  now(),
  now()
)
ON CONFLICT (id) DO NOTHING;

-- =========================================================
-- Bridge HQ organization
-- =========================================================
-- UUID: 00000000-0000-0000-0000-bbbbbbbbb002
-- type = 'school' (closest existing enum value; "platform publisher"
--                  isn't an enum value yet — out of scope for plan 049)
-- status = 'active' so the org passes membership / visibility checks

INSERT INTO organizations (
  id, name, slug, type, status,
  contact_email, contact_name,
  domain, settings, verified_at,
  created_at, updated_at
) VALUES (
  '00000000-0000-0000-0000-bbbbbbbbb002',
  'Bridge HQ',
  'bridge-hq',
  'school',
  'active',
  'system@bridge.platform',
  'Bridge HQ System',
  'bridge.platform',
  '{}'::jsonb,
  now(),
  now(),
  now()
)
ON CONFLICT (id) DO NOTHING;

-- =========================================================
-- Membership: system user is org_admin of Bridge HQ
-- =========================================================
-- UUID: 00000000-0000-0000-0000-bbbbbbbbb003
-- The system user needs an active org_admin membership in Bridge HQ
-- so authorize-by-membership checks pass when the importer runs as
-- this user (e.g., creating courses owned by Bridge HQ).

INSERT INTO org_memberships (
  id, org_id, user_id, role, status,
  invited_by, created_at
) VALUES (
  '00000000-0000-0000-0000-bbbbbbbbb003',
  '00000000-0000-0000-0000-bbbbbbbbb002',
  '00000000-0000-0000-0000-bbbbbbbbb001',
  'org_admin',
  'active',
  NULL,
  now()
)
ON CONFLICT (id) DO NOTHING;
