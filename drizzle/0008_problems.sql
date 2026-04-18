-- Migration: Problems, TestCases, Attempts (spec 006)

CREATE TABLE "problems" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "topic_id" uuid NOT NULL REFERENCES "topics"("id") ON DELETE CASCADE,
  "title" varchar(255) NOT NULL,
  "description" text NOT NULL DEFAULT '',
  "starter_code" text,
  "language" varchar(32) NOT NULL,
  "order" integer NOT NULL DEFAULT 0,
  "created_by" uuid NOT NULL REFERENCES "users"("id"),
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX "problems_topic_order_idx" ON "problems"("topic_id", "order");

CREATE TABLE "test_cases" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "problem_id" uuid NOT NULL REFERENCES "problems"("id") ON DELETE CASCADE,
  "owner_id" uuid REFERENCES "users"("id") ON DELETE CASCADE,
  "name" varchar(120) NOT NULL DEFAULT '',
  "stdin" text NOT NULL DEFAULT '',
  "expected_stdout" text,
  "is_example" boolean NOT NULL DEFAULT false,
  "order" integer NOT NULL DEFAULT 0,
  "created_at" timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX "test_cases_problem_owner_idx" ON "test_cases"("problem_id", "owner_id");

CREATE TABLE "attempts" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "problem_id" uuid NOT NULL REFERENCES "problems"("id") ON DELETE CASCADE,
  "user_id" uuid NOT NULL REFERENCES "users"("id") ON DELETE CASCADE,
  "title" varchar(120) NOT NULL DEFAULT 'Untitled',
  "language" varchar(32) NOT NULL,
  "plain_text" text NOT NULL DEFAULT '',
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX "attempts_problem_user_updated_idx"
  ON "attempts"("problem_id", "user_id", "updated_at" DESC);
