import { describe, expect, it } from "vitest";
import { sql } from "drizzle-orm";
import { testDb } from "../helpers";

/**
 * Plan 064 regression test — parent_links schema parity.
 *
 * Same shape as schema-drift.test.ts (plan 054) but for the new
 * parent_links table. Asserts the partial-unique index is in place
 * (load-bearing for re-link-after-revoke), the FK cascades match
 * Drizzle, and the status check constraint exists.
 */
describe("parent_links schema parity (plan 064)", () => {
  it("table exists with the expected columns", async () => {
    const rows = (await testDb.execute(sql`
      SELECT column_name, data_type, is_nullable, column_default
      FROM information_schema.columns
      WHERE table_name = 'parent_links'
      ORDER BY ordinal_position
    `)) as unknown as Array<{
      column_name: string;
      data_type: string;
      is_nullable: string;
      column_default: string | null;
    }>;
    const cols = rows.map((r) => r.column_name);
    expect(cols).toEqual([
      "id",
      "parent_user_id",
      "child_user_id",
      "status",
      "created_by",
      "created_at",
      "revoked_at",
    ]);
  });

  async function indexDef(name: string): Promise<string | null> {
    const rows = (await testDb.execute(sql`
      SELECT pg_get_indexdef(c.oid) AS def
      FROM pg_class c
      JOIN pg_namespace n ON n.oid = c.relnamespace
      WHERE n.nspname = 'public' AND c.relname = ${name}
    `)) as unknown as Array<{ def: string }>;
    return rows[0]?.def ?? null;
  }

  it("parent_links_parent_idx exists on parent_user_id", async () => {
    const def = await indexDef("parent_links_parent_idx");
    expect(def).not.toBeNull();
    expect(def).toMatch(/parent_user_id/);
  });

  it("parent_links_child_idx exists on child_user_id", async () => {
    const def = await indexDef("parent_links_child_idx");
    expect(def).not.toBeNull();
    expect(def).toMatch(/child_user_id/);
  });

  it("parent_links_active_uniq is a PARTIAL UNIQUE on (parent, child) WHERE status='active'", async () => {
    // Load-bearing for re-link-after-revoke. A non-partial unique
    // would block creating a fresh active row after revoking the
    // old one.
    const def = await indexDef("parent_links_active_uniq");
    expect(def, "parent_links_active_uniq must exist").not.toBeNull();
    expect(def).toMatch(/UNIQUE/i);
    expect(def).toMatch(/parent_user_id/);
    expect(def).toMatch(/child_user_id/);
    // PostgreSQL renders the WHERE as `WHERE ((status)::text = 'active'::text)`
    // — match the loose shape: WHERE … status … = … 'active'.
    expect(def).toMatch(/WHERE.*status.*'active'/i);
  });

  it("parent_links_status_check enforces the status enum", async () => {
    const rows = (await testDb.execute(sql`
      SELECT pg_get_constraintdef(c.oid) AS def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
      WHERE t.relname = 'parent_links' AND c.conname = 'parent_links_status_check'
    `)) as unknown as Array<{ def: string }>;
    expect(rows.length).toBe(1);
    expect(rows[0].def).toMatch(/active/);
    expect(rows[0].def).toMatch(/revoked/);
  });

  it("parent_links_no_self_link rejects parent == child", async () => {
    const rows = (await testDb.execute(sql`
      SELECT pg_get_constraintdef(c.oid) AS def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
      WHERE t.relname = 'parent_links' AND c.conname = 'parent_links_no_self_link'
    `)) as unknown as Array<{ def: string }>;
    expect(rows.length).toBe(1);
    expect(rows[0].def).toMatch(/parent_user_id <> child_user_id/);
  });

  it("FK cascades on user delete for parent_user_id and child_user_id", async () => {
    const rows = (await testDb.execute(sql`
      SELECT conname, confdeltype, pg_get_constraintdef(c.oid) AS def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
      WHERE t.relname = 'parent_links' AND c.contype = 'f'
      ORDER BY conname
    `)) as unknown as Array<{ conname: string; confdeltype: string; def: string }>;
    // 3 FKs: parent_user_id, child_user_id, created_by.
    expect(rows.length).toBe(3);
    // parent + child cascade ('c' = CASCADE in pg_constraint.confdeltype).
    const parentFk = rows.find((r) => r.def.includes("(parent_user_id)"));
    const childFk = rows.find((r) => r.def.includes("(child_user_id)"));
    expect(parentFk?.confdeltype).toBe("c");
    expect(childFk?.confdeltype).toBe("c");
  });
});
