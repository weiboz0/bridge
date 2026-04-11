-- Migration: Organization & Role System
-- Adds organizations, org_memberships tables
-- Modifies users table (add is_platform_admin, drop role/school_id)
-- Drops schools table
-- Removes school_id from classrooms

-- New enums
CREATE TYPE "public"."org_type" AS ENUM('school', 'tutoring_center', 'bootcamp', 'other');
CREATE TYPE "public"."org_status" AS ENUM('pending', 'active', 'suspended');
CREATE TYPE "public"."org_member_role" AS ENUM('org_admin', 'teacher', 'student', 'parent');
CREATE TYPE "public"."org_member_status" AS ENUM('pending', 'active', 'suspended');

-- Organizations table
CREATE TABLE "organizations" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "name" varchar(255) NOT NULL,
  "slug" varchar(255) NOT NULL,
  "type" "org_type" NOT NULL,
  "status" "org_status" DEFAULT 'pending' NOT NULL,
  "contact_email" varchar(255) NOT NULL,
  "contact_name" varchar(255) NOT NULL,
  "domain" varchar(255),
  "settings" jsonb DEFAULT '{}'::jsonb,
  "verified_at" timestamp,
  "created_at" timestamp DEFAULT now() NOT NULL,
  "updated_at" timestamp DEFAULT now() NOT NULL
);

CREATE UNIQUE INDEX "organizations_slug_idx" ON "organizations" USING btree ("slug");
CREATE INDEX "organizations_status_idx" ON "organizations" USING btree ("status");

-- Org memberships table
CREATE TABLE "org_memberships" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "org_id" uuid NOT NULL REFERENCES "organizations"("id") ON DELETE CASCADE,
  "user_id" uuid NOT NULL REFERENCES "users"("id") ON DELETE CASCADE,
  "role" "org_member_role" NOT NULL,
  "status" "org_member_status" DEFAULT 'pending' NOT NULL,
  "invited_by" uuid REFERENCES "users"("id"),
  "created_at" timestamp DEFAULT now() NOT NULL
);

CREATE INDEX "org_memberships_org_idx" ON "org_memberships" USING btree ("org_id");
CREATE INDEX "org_memberships_user_idx" ON "org_memberships" USING btree ("user_id");
CREATE UNIQUE INDEX "org_memberships_org_user_role_idx" ON "org_memberships" USING btree ("org_id", "user_id", "role");

-- Add isPlatformAdmin to users
ALTER TABLE "users" ADD COLUMN "is_platform_admin" boolean DEFAULT false NOT NULL;

-- Drop school_id from classrooms
ALTER TABLE "classrooms" DROP CONSTRAINT IF EXISTS "classrooms_school_id_schools_id_fk";
ALTER TABLE "classrooms" DROP COLUMN IF EXISTS "school_id";

-- Drop school_id from users
ALTER TABLE "users" DROP CONSTRAINT IF EXISTS "users_school_id_schools_id_fk";
ALTER TABLE "users" DROP COLUMN IF EXISTS "school_id";

-- Drop role from users
ALTER TABLE "users" DROP COLUMN IF EXISTS "role";

-- Drop schools table
DROP TABLE IF EXISTS "schools";
