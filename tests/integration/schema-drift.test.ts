import { describe, expect, it } from "vitest";
import { sql } from "drizzle-orm";
import { testDb } from "../helpers";

/**
 * Plan 054 regression test.
 *
 * Detects code-vs-schema drift between Drizzle declarations and the
 * live DB shape, for tables where drift has historically caused
 * silent breakage:
 *
 * - `documents` — migration 0012 dropped `classroom_id` but the
 *   Drizzle schema kept declaring it (pre-054). A `bun run
 *   db:generate` would re-emit the column.
 * - `teaching_units` — migrations 0016/0017/0019 created indexes
 *   (including a partial-WHERE expression unique that's load-
 *   bearing for plan 044's 1:1 unit↔topic invariant) that the
 *   Drizzle schema didn't declare. A `drizzle-kit push` would
 *   silently drop them.
 *
 * The pattern uses `pg_get_indexdef` and `pg_get_expr` so partial
 * WHERE clauses and expression columns are visible — the existing
 * `schema-scheduled-sessions.test.ts` pattern only checks index
 * column NAMES via `indkey + pg_attribute`, which can't catch
 * either of those.
 */
describe("schema drift (plan 054)", () => {
  describe("documents", () => {
    it("classroom_id column is gone (migration 0012)", async () => {
      const rows = (await testDb.execute(sql`
        SELECT column_name
        FROM information_schema.columns
        WHERE table_name = 'documents' AND column_name = 'classroom_id'
      `)) as unknown as Array<{ column_name: string }>;
      expect(rows.length).toBe(0);
    });

    it("documents_classroom_idx index is gone", async () => {
      const rows = (await testDb.execute(sql`
        SELECT indexname FROM pg_indexes
        WHERE tablename = 'documents' AND indexname = 'documents_classroom_idx'
      `)) as unknown as Array<{ indexname: string }>;
      expect(rows.length).toBe(0);
    });
  });

  describe("teaching_units indexes (load-bearing partial-WHERE + expression)", () => {
    async function indexDef(name: string): Promise<string | null> {
      const rows = (await testDb.execute(sql`
        SELECT pg_get_indexdef(c.oid) AS def
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relname = ${name}
      `)) as unknown as Array<{ def: string }>;
      return rows[0]?.def ?? null;
    }

    it("teaching_units_scope_slug_uniq exists with COALESCE expression and partial WHERE", async () => {
      const def = await indexDef("teaching_units_scope_slug_uniq");
      expect(def, "teaching_units_scope_slug_uniq must exist in the live DB").not.toBeNull();
      // The expression index uses COALESCE(scope_id::text, '') so
      // platform-scope rows (scope_id NULL) don't all collide.
      expect(def).toMatch(/COALESCE/i);
      expect(def).toMatch(/scope_id/i);
      expect(def).toMatch(/WHERE \(?slug IS NOT NULL\)?/i);
      expect(def).toMatch(/UNIQUE/i);
    });

    it("teaching_units_topic_id_uniq exists with partial WHERE — load-bearing for plan 044", async () => {
      const def = await indexDef("teaching_units_topic_id_uniq");
      expect(def).not.toBeNull();
      expect(def).toMatch(/UNIQUE/i);
      expect(def).toMatch(/topic_id/i);
      expect(def).toMatch(/WHERE \(?topic_id IS NOT NULL\)?/i);
    });

    it("teaching_units_search_idx is a GIN index on search_vector", async () => {
      const def = await indexDef("teaching_units_search_idx");
      expect(def).not.toBeNull();
      expect(def).toMatch(/USING gin/i);
      expect(def).toMatch(/search_vector/);
    });

    it("teaching_units_subject_tags_gin_idx is a GIN index on subject_tags", async () => {
      const def = await indexDef("teaching_units_subject_tags_gin_idx");
      expect(def).not.toBeNull();
      expect(def).toMatch(/USING gin/i);
      expect(def).toMatch(/subject_tags/);
    });

    it("teaching_units_standards_tags_gin_idx is a GIN index on standards_tags", async () => {
      const def = await indexDef("teaching_units_standards_tags_gin_idx");
      expect(def).not.toBeNull();
      expect(def).toMatch(/USING gin/i);
      expect(def).toMatch(/standards_tags/);
    });
  });

  describe("teaching_units columns", () => {
    async function columnExists(name: string): Promise<boolean> {
      const rows = (await testDb.execute(sql`
        SELECT column_name FROM information_schema.columns
        WHERE table_name = 'teaching_units' AND column_name = ${name}
      `)) as unknown as Array<{ column_name: string }>;
      return rows.length > 0;
    }

    it("usage_count column exists (migration 0019)", async () => {
      expect(await columnExists("usage_count")).toBe(true);
    });

    it("avg_rating column exists (migration 0019)", async () => {
      expect(await columnExists("avg_rating")).toBe(true);
    });

    it("search_vector generated column exists", async () => {
      expect(await columnExists("search_vector")).toBe(true);
    });
  });
});
