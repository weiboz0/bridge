-- Records "what the user said when they signed up" so onboarding can route
-- without re-asking. Distinct from any role the user holds in an org
-- (orgMemberRoleEnum). Nullable: pre-existing rows + OAuth-only signups
-- with no explicit answer remain valid.

DO $$ BEGIN
  CREATE TYPE signup_intent AS ENUM ('teacher', 'student');
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS intended_role signup_intent;
