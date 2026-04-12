-- Migration: Parent Reports

CREATE TABLE "parent_reports" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "student_id" uuid NOT NULL REFERENCES "users"("id") ON DELETE CASCADE,
  "generated_by" uuid NOT NULL REFERENCES "users"("id"),
  "period_start" timestamp NOT NULL,
  "period_end" timestamp NOT NULL,
  "content" text NOT NULL,
  "summary" jsonb DEFAULT '{}'::jsonb,
  "created_at" timestamp DEFAULT now() NOT NULL
);

CREATE INDEX "parent_reports_student_idx" ON "parent_reports" USING btree ("student_id");
CREATE INDEX "parent_reports_period_idx" ON "parent_reports" USING btree ("student_id", "period_start");
