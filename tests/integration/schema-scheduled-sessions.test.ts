import { describe, expect, it } from "vitest";
import { sql } from "drizzle-orm";
import { testDb } from "../helpers";

/**
 * Plan 058 regression test.
 *
 * Verifies the live shape of `scheduled_sessions` — ENUM, columns,
 * indexes, and the cross-table FK that 0023 backfilled. If any of
 * these drift from the migration in `drizzle/0023_create_scheduled_sessions.sql`,
 * this test fails loudly. That's the safety net the original
 * "scheduled_sessions has no CREATE TABLE migration" gap lacked.
 *
 * Codex review of plan 058 was emphatic that all four facets
 * (ENUM/columns/indexes/FK) need coverage, since manual schema.ts
 * <-> SQL parity has produced documented drift before
 * (review 009 §P1-12 on teaching_units indexes).
 */
describe("scheduled_sessions schema parity (plan 058)", () => {
  it("schedule_status enum has the four expected values", async () => {
    const rows = (await testDb.execute(sql`
      SELECT enumlabel
      FROM pg_type t
      JOIN pg_enum e ON e.enumtypid = t.oid
      WHERE t.typname = 'schedule_status'
      ORDER BY e.enumsortorder
    `)) as unknown as Array<{ enumlabel: string }>;
    expect(rows.map((r) => r.enumlabel)).toEqual([
      "planned",
      "in_progress",
      "completed",
      "cancelled",
    ]);
  });

  it("scheduled_sessions table exists with the expected columns", async () => {
    const rows = (await testDb.execute(sql`
      SELECT column_name, data_type, is_nullable
      FROM information_schema.columns
      WHERE table_schema = 'public' AND table_name = 'scheduled_sessions'
      ORDER BY ordinal_position
    `)) as unknown as Array<{
      column_name: string;
      data_type: string;
      is_nullable: string;
    }>;
    const columns = rows.map((r) => r.column_name);
    expect(columns).toEqual([
      "id",
      "class_id",
      "teacher_id",
      "title",
      "scheduled_start",
      "scheduled_end",
      "recurrence",
      "topic_ids",
      "status",
      "created_at",
      "updated_at",
    ]);

    const byName = new Map(rows.map((r) => [r.column_name, r]));
    expect(byName.get("id")?.is_nullable).toBe("NO");
    expect(byName.get("class_id")?.is_nullable).toBe("NO");
    expect(byName.get("teacher_id")?.is_nullable).toBe("NO");
    expect(byName.get("title")?.is_nullable).toBe("YES");
    expect(byName.get("scheduled_start")?.is_nullable).toBe("NO");
    expect(byName.get("scheduled_end")?.is_nullable).toBe("NO");
    expect(byName.get("recurrence")?.is_nullable).toBe("YES");
    expect(byName.get("topic_ids")?.is_nullable).toBe("YES");
    expect(byName.get("status")?.is_nullable).toBe("NO");
  });

  it("scheduled_sessions indexes are all present", async () => {
    const rows = (await testDb.execute(sql`
      SELECT indexname
      FROM pg_indexes
      WHERE schemaname = 'public' AND tablename = 'scheduled_sessions'
      ORDER BY indexname
    `)) as unknown as Array<{ indexname: string }>;
    const names = new Set(rows.map((r) => r.indexname));
    // pkey is implicit; the three named indexes come from the
    // migration. If any of these go missing, the migration drifted
    // from the schema.
    expect(names.has("scheduled_sessions_pkey")).toBe(true);
    expect(names.has("scheduled_sessions_class_idx")).toBe(true);
    expect(names.has("scheduled_sessions_start_idx")).toBe(true);
    expect(names.has("scheduled_sessions_status_idx")).toBe(true);
  });

  it("sessions.scheduled_session_id FK targets scheduled_sessions(id)", async () => {
    // Constraint name matches what 0014_session_model.sql:81-90
    // declared. Migration 0023 re-attaches it on a fresh DB chain
    // where 0014's IF EXISTS guard would have skipped it.
    const rows = (await testDb.execute(sql`
      SELECT
        c.conname,
        c.confdeltype,
        ref.relname AS referenced_table
      FROM pg_constraint c
      JOIN pg_class src ON src.oid = c.conrelid
      LEFT JOIN pg_class ref ON ref.oid = c.confrelid
      WHERE c.conname = 'sessions_scheduled_session_id_fkey'
        AND src.relname = 'sessions'
    `)) as unknown as Array<{
      conname: string;
      confdeltype: string;
      referenced_table: string;
    }>;
    expect(rows).toHaveLength(1);
    expect(rows[0].referenced_table).toBe("scheduled_sessions");
    // confdeltype 'n' === SET NULL (per pg_constraint catalog).
    expect(rows[0].confdeltype).toBe("n");
  });
});
