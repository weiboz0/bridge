-- Migration: persist last test run summary on each attempt (spec 008)

ALTER TABLE attempts ADD COLUMN last_test_result jsonb;
