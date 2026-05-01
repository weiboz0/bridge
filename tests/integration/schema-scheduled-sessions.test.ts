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

  it("scheduled_sessions table has the expected columns, types, defaults, nullability", async () => {
    const rows = (await testDb.execute(sql`
      SELECT
        column_name,
        data_type,
        udt_name,
        is_nullable,
        column_default
      FROM information_schema.columns
      WHERE table_schema = 'public' AND table_name = 'scheduled_sessions'
      ORDER BY ordinal_position
    `)) as unknown as Array<{
      column_name: string;
      data_type: string;
      udt_name: string;
      is_nullable: string;
      column_default: string | null;
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

    // Types — protect against silently-changed column shapes.
    expect(byName.get("id")?.udt_name).toBe("uuid");
    expect(byName.get("class_id")?.udt_name).toBe("uuid");
    expect(byName.get("teacher_id")?.udt_name).toBe("uuid");
    expect(byName.get("title")?.udt_name).toBe("varchar");
    expect(byName.get("scheduled_start")?.udt_name).toBe("timestamptz");
    expect(byName.get("scheduled_end")?.udt_name).toBe("timestamptz");
    expect(byName.get("recurrence")?.udt_name).toBe("jsonb");
    // PostgreSQL stores arrays with a leading `_` in udt_name (`_uuid` for uuid[]).
    expect(byName.get("topic_ids")?.udt_name).toBe("_uuid");
    expect(byName.get("status")?.udt_name).toBe("schedule_status");
    expect(byName.get("created_at")?.udt_name).toBe("timestamptz");
    expect(byName.get("updated_at")?.udt_name).toBe("timestamptz");

    // Nullability.
    expect(byName.get("id")?.is_nullable).toBe("NO");
    expect(byName.get("class_id")?.is_nullable).toBe("NO");
    expect(byName.get("teacher_id")?.is_nullable).toBe("NO");
    expect(byName.get("title")?.is_nullable).toBe("YES");
    expect(byName.get("scheduled_start")?.is_nullable).toBe("NO");
    expect(byName.get("scheduled_end")?.is_nullable).toBe("NO");
    expect(byName.get("recurrence")?.is_nullable).toBe("YES");
    expect(byName.get("topic_ids")?.is_nullable).toBe("YES");
    expect(byName.get("status")?.is_nullable).toBe("NO");
    expect(byName.get("created_at")?.is_nullable).toBe("NO");
    expect(byName.get("updated_at")?.is_nullable).toBe("NO");

    // Defaults — gen_random_uuid() on id, 'planned' on status, now() on timestamps.
    expect(byName.get("id")?.column_default).toMatch(/^gen_random_uuid\(\)$/);
    expect(byName.get("status")?.column_default).toMatch(/^'planned'/);
    expect(byName.get("created_at")?.column_default).toMatch(/^now\(\)/);
    expect(byName.get("updated_at")?.column_default).toMatch(/^now\(\)/);
  });

  it("scheduled_sessions indexes have the expected names AND columns", async () => {
    // Codex post-impl review: name-only checks pass even if the
    // index points at the wrong column. Verify column lists too via
    // pg_index → pg_attribute.
    const rows = (await testDb.execute(sql`
      SELECT
        i.relname AS index_name,
        ARRAY(
          SELECT a.attname
          FROM unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ord)
          JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = k.attnum
          ORDER BY k.ord
        ) AS column_names
      FROM pg_index ix
      JOIN pg_class i ON i.oid = ix.indexrelid
      JOIN pg_class t ON t.oid = ix.indrelid
      WHERE t.relname = 'scheduled_sessions'
      ORDER BY i.relname
    `)) as unknown as Array<{
      index_name: string;
      column_names: string[];
    }>;
    const byName = new Map(rows.map((r) => [r.index_name, r.column_names]));
    expect(byName.get("scheduled_sessions_pkey")).toEqual(["id"]);
    expect(byName.get("scheduled_sessions_class_idx")).toEqual(["class_id"]);
    expect(byName.get("scheduled_sessions_start_idx")).toEqual(["scheduled_start"]);
    expect(byName.get("scheduled_sessions_status_idx")).toEqual([
      "class_id",
      "status",
    ]);
  });

  it("sessions.scheduled_session_id FK columns and behavior are correct", async () => {
    // Codex post-impl review: also verify the local + referenced
    // column names (conkey/confkey) so a swap goes caught.
    const rows = (await testDb.execute(sql`
      SELECT
        c.conname,
        c.confdeltype,
        ref.relname AS referenced_table,
        (
          SELECT array_agg(att.attname ORDER BY u.ord)
          FROM unnest(c.conkey) WITH ORDINALITY AS u(attnum, ord)
          JOIN pg_attribute att
            ON att.attrelid = c.conrelid AND att.attnum = u.attnum
        ) AS local_columns,
        (
          SELECT array_agg(att.attname ORDER BY u.ord)
          FROM unnest(c.confkey) WITH ORDINALITY AS u(attnum, ord)
          JOIN pg_attribute att
            ON att.attrelid = c.confrelid AND att.attnum = u.attnum
        ) AS referenced_columns
      FROM pg_constraint c
      JOIN pg_class src ON src.oid = c.conrelid
      LEFT JOIN pg_class ref ON ref.oid = c.confrelid
      WHERE c.conname = 'sessions_scheduled_session_id_fkey'
        AND src.relname = 'sessions'
    `)) as unknown as Array<{
      conname: string;
      confdeltype: string;
      referenced_table: string;
      local_columns: string[];
      referenced_columns: string[];
    }>;
    expect(rows).toHaveLength(1);
    expect(rows[0].referenced_table).toBe("scheduled_sessions");
    // confdeltype 'n' === SET NULL (per pg_constraint catalog).
    expect(rows[0].confdeltype).toBe("n");
    expect(rows[0].local_columns).toEqual(["scheduled_session_id"]);
    expect(rows[0].referenced_columns).toEqual(["id"]);
  });
});
