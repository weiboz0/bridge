CREATE TYPE "public"."user_status" AS ENUM('active', 'suspended');
ALTER TABLE "users" ADD COLUMN "status" "user_status" DEFAULT 'active' NOT NULL;
CREATE INDEX "users_status_idx" ON "users" USING btree ("status");
CREATE INDEX "org_memberships_user_status_created_idx" ON "org_memberships" USING btree ("user_id", "status", "created_at");
