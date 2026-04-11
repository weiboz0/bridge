-- Migration: Assignments and Submissions

CREATE TABLE "assignments" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "topic_id" uuid REFERENCES "topics"("id") ON DELETE CASCADE,
  "class_id" uuid NOT NULL REFERENCES "classes"("id") ON DELETE CASCADE,
  "title" varchar(255) NOT NULL,
  "description" text DEFAULT '',
  "starter_code" text,
  "due_date" timestamp,
  "rubric" jsonb DEFAULT '{}'::jsonb,
  "created_at" timestamp DEFAULT now() NOT NULL
);

CREATE INDEX "assignments_class_idx" ON "assignments" USING btree ("class_id");
CREATE INDEX "assignments_topic_idx" ON "assignments" USING btree ("topic_id");

CREATE TABLE "submissions" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "assignment_id" uuid NOT NULL REFERENCES "assignments"("id") ON DELETE CASCADE,
  "student_id" uuid NOT NULL REFERENCES "users"("id") ON DELETE CASCADE,
  "document_id" uuid,
  "grade" double precision,
  "feedback" text,
  "submitted_at" timestamp DEFAULT now() NOT NULL
);

CREATE UNIQUE INDEX "submission_unique_idx" ON "submissions" USING btree ("assignment_id", "student_id");
CREATE INDEX "submissions_assignment_idx" ON "submissions" USING btree ("assignment_id");
CREATE INDEX "submissions_student_idx" ON "submissions" USING btree ("student_id");
