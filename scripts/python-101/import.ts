#!/usr/bin/env bun
/**
 * Python 101 importer — plan 049 phase 2.
 *
 * Reads `content/python-101/{course.yaml, units/*.yaml}`, validates
 * the tree, runs every reference solution against its test cases via
 * Piston, and (when --apply is passed) writes the result to the DB
 * inside ONE transaction.
 *
 * Three insert passes inside the transaction:
 *
 *   Pass 1 — library content (platform-scope):
 *     teaching_units (topic_id NULL), unit_documents,
 *     problems, problem_solutions, test_cases (canonical).
 *
 *   Pass 2 — course arrangement (Bridge HQ org):
 *     courses, topics, topic_problems.
 *
 *   Pass 3 — link units to topics (1:1 invariant):
 *     For each (unit, topic) pair: skip if already linked, else
 *     UPDATE teaching_units SET topic_id = $topicId. The partial-
 *     unique index `teaching_units_topic_id_uniq` keeps us safe
 *     against races; we surface a clean error on conflict.
 *
 * After the three passes, runs a post-insert orphan check (every
 * topic for the imported course must have a linked unit). If the
 * check fails, the transaction is rolled back.
 *
 * Flags:
 *   --root <dir>          Content directory (default content/python-101)
 *   --apply               Write to the DB. Required; default is a
 *                         dry-run that validates + sandbox-checks but
 *                         does NOT touch the DB.
 *   --library-only        Stop after Pass 1 (no course / topics /
 *                         topic_problems / link). Used by Phase 3
 *                         picker-discovery test.
 *   --skip-sandbox        Skip the Piston pre-flight. Intended for
 *                         CI / test runs where solutions are known
 *                         good. Logs a warning.
 *   --allow-rename        Permit slug rename when id matches an
 *                         existing row. Without it, mismatched slugs
 *                         abort.
 *   --target-db <conn>    Override DATABASE_URL.
 *   --piston-url <url>    Override PISTON_URL for this invocation.
 */

import { readdir, readFile, stat } from "node:fs/promises";
import { join, basename } from "node:path";
import { sql, eq, and, isNull, inArray } from "drizzle-orm";
import { drizzle } from "drizzle-orm/postgres-js";
import { nanoid } from "nanoid";
import postgres, { type Sql } from "postgres";
import {
  classes,
  courses,
  problems,
  problemSolutions,
  teachingUnits,
  topicProblems,
  topics,
  unitDocuments,
} from "../../src/lib/db/schema";
import {
  courseManifestSchema,
  parseAuthoringYaml,
  unitFileSchema,
  validateContentTree,
  type CourseManifest,
  type UnitFile,
  type ContentTree,
} from "./schema";
import {
  compareOutputs,
  runInSandbox,
  SandboxError,
  type SandboxResult,
} from "./sandbox-runner";

// =========================================================
// Bridge HQ constants (see scripts/seed_bridge_hq.sql)
// =========================================================

const BRIDGE_HQ_ORG_ID = "00000000-0000-0000-0000-bbbbbbbbb002";
const BRIDGE_HQ_SYSTEM_USER_ID = "00000000-0000-0000-0000-bbbbbbbbb001";

// =========================================================
// CLI args
// =========================================================

interface CliArgs {
  root: string;
  apply: boolean;
  libraryOnly: boolean;
  skipSandbox: boolean;
  allowRename: boolean;
  wireDemoClass?: boolean;
  targetDb?: string;
  pistonUrl?: string;
}

function parseArgs(argv: string[]): CliArgs {
  const args: CliArgs = {
    root: "content/python-101",
    apply: false,
    libraryOnly: false,
    skipSandbox: false,
    allowRename: false,
    wireDemoClass: false,
  };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--apply") args.apply = true;
    else if (a === "--library-only" || a === "--stop-after=library")
      args.libraryOnly = true;
    else if (a === "--skip-sandbox") args.skipSandbox = true;
    else if (a === "--allow-rename") args.allowRename = true;
    else if (a === "--wire-demo-class") args.wireDemoClass = true;
    else if (a === "--root" && argv[i + 1]) args.root = argv[++i];
    else if (a.startsWith("--root=")) args.root = a.slice("--root=".length);
    else if (a === "--target-db" && argv[i + 1]) args.targetDb = argv[++i];
    else if (a.startsWith("--target-db="))
      args.targetDb = a.slice("--target-db=".length);
    else if (a === "--piston-url" && argv[i + 1]) args.pistonUrl = argv[++i];
    else if (a.startsWith("--piston-url="))
      args.pistonUrl = a.slice("--piston-url=".length);
    else if (a === "-h" || a === "--help") {
      printHelp();
      process.exit(0);
    } else {
      console.error(`unknown argument: ${a}`);
      process.exit(2);
    }
  }
  return args;
}

function printHelp(): void {
  process.stdout.write(`Usage: bun run scripts/python-101/import.ts [options]

  --apply                  Write to the DB (default is dry-run).
  --root <dir>             Content directory (default content/python-101).
  --library-only           Stop after the library pass.
  --skip-sandbox           Skip Piston pre-flight (CI/test only).
  --allow-rename           Allow slug renames where ids match.
  --wire-demo-class        After import, clone the Bridge HQ Python 101
                           course into Bridge Demo School org (owned
                           by eve@demo.edu), clone its units into the
                           same org, and point the demo class at the
                           cloned course. Idempotent — re-runs detect
                           the existing clone and refresh the demo
                           class link.
  --target-db <conn>       Override DATABASE_URL.
  --piston-url <url>       Override PISTON_URL for this run.

Piston must be reachable at PISTON_URL (default http://localhost:2000)
unless --skip-sandbox is set. See content/python-101/README.md for
the docker run incantation.
`);
}

// =========================================================
// Tree loader (mirrors validate.ts; intentionally local to keep
// the importer self-contained for embedding in tests)
// =========================================================

async function loadContentTree(root: string): Promise<ContentTree> {
  const courseRaw = await readFile(join(root, "course.yaml"), "utf8");
  const courseParsed = parseAuthoringYaml(courseRaw);
  const courseResult = courseManifestSchema.safeParse(courseParsed);
  if (!courseResult.success) {
    throw new Error(
      `course.yaml: ${formatZodFailure(courseResult.error)}`,
    );
  }

  const unitsDir = join(root, "units");
  const entries = await readdir(unitsDir);
  const units = new Map<string, UnitFile>();
  for (const entry of entries) {
    if (!entry.endsWith(".yaml") && !entry.endsWith(".yml")) continue;
    const file = join(unitsDir, entry);
    const fileStat = await stat(file);
    if (!fileStat.isFile()) continue;
    const raw = await readFile(file, "utf8");
    const parsed = parseAuthoringYaml(raw);
    const unitResult = unitFileSchema.safeParse(parsed);
    if (!unitResult.success) {
      throw new Error(`${file}: ${formatZodFailure(unitResult.error)}`);
    }
    const expectedSlug = basename(
      entry,
      entry.endsWith(".yaml") ? ".yaml" : ".yml",
    );
    if (unitResult.data.slug !== expectedSlug) {
      throw new Error(
        `${file}: slug "${unitResult.data.slug}" must match filename "${expectedSlug}"`,
      );
    }
    if (units.has(unitResult.data.slug)) {
      throw new Error(
        `${file}: slug "${unitResult.data.slug}" already used by another unit file`,
      );
    }
    units.set(unitResult.data.slug, unitResult.data);
  }

  const tree: ContentTree = { course: courseResult.data, units };
  const issues = validateContentTree(tree);
  if (issues.length > 0) {
    const lines = issues.map((i) => `  ${i.file}: ${i.message}`).join("\n");
    throw new Error(`cross-file validation failed:\n${lines}`);
  }
  return tree;
}

function formatZodFailure(error: unknown): string {
  if (
    error &&
    typeof error === "object" &&
    "issues" in error &&
    Array.isArray((error as { issues: unknown[] }).issues)
  ) {
    type ZIssue = { path: (string | number)[]; message: string };
    const issues = (error as { issues: ZIssue[] }).issues;
    return issues
      .map((i) => `${i.path.join(".") || "<root>"}: ${i.message}`)
      .join("; ");
  }
  return error instanceof Error ? error.message : String(error);
}

// =========================================================
// Sandbox pre-flight: every problem's reference solution × every
// test case must produce the expected stdout (after normalization).
// =========================================================

interface SandboxFailure {
  unitSlug: string;
  problemSlug: string;
  caseName: string;
  expected: string;
  actual: string;
  exitCode: number;
  stderr: string;
}

async function preflightSandbox(
  tree: ContentTree,
): Promise<{ ok: true } | { ok: false; failures: SandboxFailure[] }> {
  const failures: SandboxFailure[] = [];
  for (const [unitSlug, unit] of tree.units) {
    for (const problem of unit.problems) {
      for (const tc of problem.testCases) {
        let result: SandboxResult;
        try {
          result = await runInSandbox({
            language: problem.solution.language,
            source: problem.solution.code,
            stdin: tc.stdin,
          });
        } catch (e) {
          if (e instanceof SandboxError) {
            failures.push({
              unitSlug,
              problemSlug: problem.slug,
              caseName: tc.name,
              expected: tc.expectedStdout,
              actual: "",
              exitCode: e.exitCode ?? -1,
              stderr: `[sandbox-${e.stage}] ${e.message}`,
            });
            continue;
          }
          throw e;
        }
        const cmp = compareOutputs(tc.expectedStdout, result.stdout);
        if (!cmp.matched || result.exitCode !== 0) {
          failures.push({
            unitSlug,
            problemSlug: problem.slug,
            caseName: tc.name,
            expected: cmp.expectedNormalized,
            actual: cmp.actualNormalized,
            exitCode: result.exitCode,
            stderr: result.stderr.slice(0, 1000),
          });
        }
      }
    }
  }
  return failures.length > 0 ? { ok: false, failures } : { ok: true };
}

function reportSandboxFailures(failures: SandboxFailure[]): void {
  process.stderr.write(`\n${failures.length} sandbox failure(s):\n\n`);
  for (const f of failures) {
    process.stderr.write(
      `  [${f.unitSlug}/${f.problemSlug}/${f.caseName}] exit=${f.exitCode}\n`,
    );
    process.stderr.write(`    expected: ${JSON.stringify(f.expected)}\n`);
    process.stderr.write(`    actual:   ${JSON.stringify(f.actual)}\n`);
    if (f.stderr) {
      process.stderr.write(`    stderr:   ${f.stderr.slice(0, 200)}\n`);
    }
  }
  process.stderr.write(
    "\nA reference solution failed its own test cases. Fix the YAML and re-run.\n",
  );
}

// =========================================================
// Identity check (slug-rename guard, --allow-rename gate)
// =========================================================

type Tx = Parameters<Parameters<ReturnType<typeof drizzle>["transaction"]>[0]>[0];

interface IdentityIssue {
  file: string;
  message: string;
}

async function checkIdentities(
  tx: Tx,
  tree: ContentTree,
  allowRename: boolean,
): Promise<IdentityIssue[]> {
  const issues: IdentityIssue[] = [];

  // Collect (id -> expected slug) for problems and units. The course
  // and topics rows don't carry a `slug` column, so they're checked
  // by id only — the title may freely change.
  const expectedProblemSlugs = new Map<string, string>();
  const expectedUnitSlugs = new Map<string, string>();
  for (const [unitSlug, unit] of tree.units) {
    expectedUnitSlugs.set(unit.id, unitSlug);
    for (const p of unit.problems) {
      expectedProblemSlugs.set(p.id, p.slug);
    }
  }

  // Look up actual slugs by id.
  const unitIds = Array.from(expectedUnitSlugs.keys());
  if (unitIds.length > 0) {
    const rows = await tx
      .select({ id: teachingUnits.id, slug: teachingUnits.slug })
      .from(teachingUnits)
      .where(inArray(teachingUnits.id, unitIds));
    for (const row of rows) {
      const want = expectedUnitSlugs.get(row.id);
      if (want === undefined) continue;
      if (row.slug !== null && row.slug !== want && !allowRename) {
        issues.push({
          file: `units/${want}.yaml`,
          message: `unit id ${row.id} exists with slug "${row.slug}"; YAML has "${want}". Rerun with --allow-rename to update the slug in place.`,
        });
      }
    }
  }

  const problemIds = Array.from(expectedProblemSlugs.keys());
  if (problemIds.length > 0) {
    const rows = await tx
      .select({ id: problems.id, slug: problems.slug })
      .from(problems)
      .where(inArray(problems.id, problemIds));
    for (const row of rows) {
      const want = expectedProblemSlugs.get(row.id);
      if (want === undefined) continue;
      if (row.slug !== null && row.slug !== want && !allowRename) {
        issues.push({
          file: "(problem)",
          message: `problem id ${row.id} exists with slug "${row.slug}"; YAML has "${want}". Rerun with --allow-rename.`,
        });
      }
    }
  }

  return issues;
}

// =========================================================
// Block translation: YAML blocks -> unit_documents.blocks JSONB
// =========================================================

/**
 * Emits the editor's expected document shape:
 *
 *   { type: "doc", content: [<block>, ...] }
 *
 * Maps each authoring block to a Tiptap-compatible node so the
 * unit renders with its prose intact. Mapping (see KNOWN_NODE_TYPES
 * in src/components/editor/tiptap/teaching-unit-editor.tsx for the
 * authoritative whitelist):
 *
 *   heading      -> heading (level 2)
 *   paragraph    -> paragraph
 *   list         -> bulletList of listItem(paragraph) per non-empty line
 *   callout      -> teacher-note wrapping a paragraph (closest match;
 *                    the editor has no native callout type)
 *   code         -> codeBlock with language attr
 *   problem-ref  -> problem-ref custom node
 */
function buildUnitDocumentBlocks(
  unit: UnitFile,
  problemIdBySlug: Map<string, string>,
): Record<string, unknown> {
  const blocks: Record<string, unknown>[] = [];
  let problemRefIdx = 0;
  for (const block of unit.blocks) {
    if (block.type === "problem-ref") {
      const problemId = problemIdBySlug.get(block.problemSlug);
      if (!problemId) {
        throw new Error(
          `unit ${unit.slug}: problem-ref to "${block.problemSlug}" but no problem id was registered`,
        );
      }
      blocks.push({
        type: "problem-ref",
        attrs: {
          id: `b${String(problemRefIdx).padStart(3, "0")}`,
          problemId,
          pinnedRevision: null,
          visibility: block.visibility,
          overrideStarter: null,
        },
      });
      problemRefIdx++;
      continue;
    }

    // Prose blocks. The text comes from the YAML's `text` field, which
    // arrives as a single string (`|`-block scalars preserve the
    // newlines). Some block types want it split into list items; the
    // rest treat the whole string as a single block.
    const text = block.text;
    switch (block.type) {
      case "heading":
        blocks.push({
          type: "heading",
          attrs: { level: 2 },
          content: text.length > 0 ? [{ type: "text", text }] : [],
        });
        break;

      case "paragraph":
        // Authors may put multi-paragraph content in one YAML block via
        // blank-line splits. Emit one `paragraph` node per chunk so
        // the editor renders them with proper spacing.
        for (const chunk of text.split(/\n{2,}/)) {
          const trimmed = chunk.trim();
          if (!trimmed) continue;
          blocks.push({
            type: "paragraph",
            content: [{ type: "text", text: trimmed }],
          });
        }
        break;

      case "list": {
        const items = text
          .split(/\r?\n/)
          .map((line) => line.trim())
          .filter((line) => line.length > 0)
          .map((line) => ({
            type: "listItem",
            content: [
              {
                type: "paragraph",
                content: [{ type: "text", text: line }],
              },
            ],
          }));
        if (items.length > 0) {
          blocks.push({ type: "bulletList", content: items });
        }
        break;
      }

      case "callout":
        blocks.push({
          type: "teacher-note",
          attrs: { id: nanoid() },
          content: [
            {
              type: "paragraph",
              content: text.length > 0 ? [{ type: "text", text }] : [],
            },
          ],
        });
        break;

      case "code":
        blocks.push({
          type: "codeBlock",
          attrs: { language: "python" },
          content: text.length > 0 ? [{ type: "text", text }] : [],
        });
        break;
    }
  }
  return { type: "doc", content: blocks };
}

// =========================================================
// Pass 1: library content
// =========================================================

async function runLibraryPass(tx: Tx, tree: ContentTree): Promise<void> {
  const problemIdBySlug = new Map<string, string>();

  for (const [, unit] of tree.units) {
    // teaching_units (topic_id NULL — Pass 3 sets it).
    await tx
      .insert(teachingUnits)
      .values({
        id: unit.id,
        scope: "platform",
        scopeId: null,
        title: unit.title,
        slug: unit.slug,
        summary: unit.description,
        gradeLevel: unit.gradeLevel,
        subjectTags: unit.subjectTags,
        standardsTags: unit.standardsTags,
        estimatedMinutes: unit.estimatedMinutes ?? null,
        materialType: unit.materialType,
        status: "classroom_ready",
        topicId: null,
        createdBy: BRIDGE_HQ_SYSTEM_USER_ID,
      })
      .onConflictDoUpdate({
        target: teachingUnits.id,
        set: {
          title: unit.title,
          slug: unit.slug,
          summary: unit.description,
          gradeLevel: unit.gradeLevel,
          subjectTags: unit.subjectTags,
          standardsTags: unit.standardsTags,
          estimatedMinutes: unit.estimatedMinutes ?? null,
          materialType: unit.materialType,
          status: "classroom_ready",
          updatedAt: new Date(),
        },
      });

    // problems (canonical, scope=platform).
    for (const p of unit.problems) {
      await tx
        .insert(problems)
        .values({
          id: p.id,
          scope: "platform",
          scopeId: null,
          title: p.title,
          slug: p.slug,
          description: p.description,
          starterCode: p.starterCode,
          difficulty: p.difficulty,
          gradeLevel: p.gradeLevel,
          tags: p.tags,
          status: "published",
          forkedFrom: null,
          timeLimitMs: p.timeLimitMs ?? null,
          memoryLimitMb: p.memoryLimitMb ?? null,
          createdBy: BRIDGE_HQ_SYSTEM_USER_ID,
        })
        .onConflictDoUpdate({
          target: problems.id,
          set: {
            title: p.title,
            slug: p.slug,
            description: p.description,
            starterCode: p.starterCode,
            difficulty: p.difficulty,
            gradeLevel: p.gradeLevel,
            tags: p.tags,
            status: "published",
            timeLimitMs: p.timeLimitMs ?? null,
            memoryLimitMb: p.memoryLimitMb ?? null,
            updatedAt: new Date(),
          },
        });
      problemIdBySlug.set(p.slug, p.id);

      // problem_solutions: ensure exactly one canonical published row
      // per (problem, language). Keyed by (problem_id, language) which
      // doesn't have a unique index in DB schema — the importer
      // deletes all platform-scope solutions for the problem first,
      // then re-inserts. Safe because canonical solutions are
      // importer-owned; user solutions live elsewhere (none today).
      await tx.delete(problemSolutions).where(eq(problemSolutions.problemId, p.id));
      await tx.insert(problemSolutions).values({
        problemId: p.id,
        language: p.solution.language,
        title: "Canonical solution",
        code: p.solution.code,
        notes: null,
        approachTags: [],
        isPublished: true,
        createdBy: BRIDGE_HQ_SYSTEM_USER_ID,
      });

      // test_cases: re-emit canonical (owner_id IS NULL) cases for
      // this problem. owner_id IS NOT NULL rows survive (student/
      // teacher private cases). Order matches YAML order.
      await tx.execute(
        sql`DELETE FROM test_cases WHERE problem_id = ${p.id} AND owner_id IS NULL`,
      );
      let order = 0;
      for (const tc of p.testCases) {
        await tx.execute(sql`
          INSERT INTO test_cases (
            problem_id, owner_id, name, stdin, expected_stdout,
            is_example, "order"
          ) VALUES (
            ${p.id}, NULL, ${tc.name}, ${tc.stdin}, ${tc.expectedStdout},
            ${tc.isExample}, ${order}
          )
        `);
        order++;
      }
    }

    // unit_documents: rebuild from YAML blocks (problem-ref only for
    // v1). UPSERT on unit_id (unique).
    const docBlocks = buildUnitDocumentBlocks(unit, problemIdBySlug);
    await tx
      .insert(unitDocuments)
      .values({
        unitId: unit.id,
        blocks: docBlocks,
      })
      .onConflictDoUpdate({
        target: unitDocuments.unitId,
        set: {
          blocks: docBlocks,
          updatedAt: new Date(),
        },
      });
  }
}

// =========================================================
// Pass 2: course arrangement
// =========================================================

async function runCoursePass(tx: Tx, tree: ContentTree): Promise<void> {
  // courses
  await tx
    .insert(courses)
    .values({
      id: tree.course.id,
      orgId: BRIDGE_HQ_ORG_ID,
      createdBy: BRIDGE_HQ_SYSTEM_USER_ID,
      title: tree.course.title,
      description: tree.course.description,
      gradeLevel: tree.course.gradeLevel,
      language: tree.course.language,
      isPublished: true,
    })
    .onConflictDoUpdate({
      target: courses.id,
      set: {
        title: tree.course.title,
        description: tree.course.description,
        gradeLevel: tree.course.gradeLevel,
        language: tree.course.language,
        isPublished: true,
        updatedAt: new Date(),
      },
    });

  // topics + topic_problems
  let sortOrder = 0;
  for (const topicEntry of tree.course.topics) {
    const unit = tree.units.get(topicEntry.unitSlug);
    if (!unit) {
      throw new Error(
        `course.yaml topic references missing unit slug "${topicEntry.unitSlug}"`,
      );
    }
    await tx
      .insert(topics)
      .values({
        id: topicEntry.id,
        courseId: tree.course.id,
        title: unit.title,
        description: unit.description,
        sortOrder,
      })
      .onConflictDoUpdate({
        target: topics.id,
        set: {
          courseId: tree.course.id,
          title: unit.title,
          description: unit.description,
          sortOrder,
          updatedAt: new Date(),
        },
      });

    // Wipe & re-insert canonical topic_problems for this topic. The
    // composite primary key (topic_id, problem_id) doesn't support a
    // simple UPSERT-with-ordering, so delete-then-insert keeps things
    // simple and idempotent.
    await tx.delete(topicProblems).where(eq(topicProblems.topicId, topicEntry.id));
    let problemOrder = 0;
    for (const p of unit.problems) {
      await tx.insert(topicProblems).values({
        topicId: topicEntry.id,
        problemId: p.id,
        sortOrder: problemOrder,
        attachedBy: BRIDGE_HQ_SYSTEM_USER_ID,
      });
      problemOrder++;
    }
    sortOrder++;
  }
}

// =========================================================
// Pass 3: link units to topics (mirrors LinkUnitToTopic Go store)
// =========================================================

async function runLinkPass(tx: Tx, tree: ContentTree): Promise<void> {
  for (const topicEntry of tree.course.topics) {
    const unit = tree.units.get(topicEntry.unitSlug);
    if (!unit) continue; // already validated upstream

    // Pre-check: skip the UPDATE if topic_id already matches (avoid
    // bumping updated_at on idempotent re-runs). Matches the Go
    // LinkUnitToTopic semantics.
    const [current] = await tx
      .select({ topicId: teachingUnits.topicId })
      .from(teachingUnits)
      .where(eq(teachingUnits.id, unit.id));
    if (!current) {
      throw new Error(
        `link pass: unit ${unit.id} not found (Pass 1 should have inserted it)`,
      );
    }
    if (current.topicId === topicEntry.id) continue;
    if (current.topicId !== null && current.topicId !== topicEntry.id) {
      throw new Error(
        `link pass: unit ${unit.id} (${unit.slug}) is already linked to a different topic ${current.topicId}; expected ${topicEntry.id}`,
      );
    }

    try {
      await tx
        .update(teachingUnits)
        .set({ topicId: topicEntry.id, updatedAt: new Date() })
        .where(eq(teachingUnits.id, unit.id));
    } catch (e) {
      // Drizzle wraps the postgres-js error as "Failed query: ...";
      // the constraint name lives on the cause chain. Walk it.
      let cur: unknown = e;
      let needle = "";
      for (let depth = 0; depth < 5 && cur; depth++) {
        if (cur instanceof Error) {
          needle += " " + cur.message;
          const c = (cur as Error & { cause?: unknown }).cause;
          cur = c;
        } else {
          break;
        }
      }
      if (needle.includes("teaching_units_topic_id_uniq")) {
        throw new Error(
          `link pass: topic ${topicEntry.id} is already claimed by another unit (uniq violation)`,
        );
      }
      throw e;
    }
  }
}

// =========================================================
// Post-insert verification
// =========================================================

async function postInsertVerification(
  tx: Tx,
  tree: ContentTree,
): Promise<void> {
  const orphanRows = await tx
    .select({ topicId: topics.id, title: topics.title })
    .from(topics)
    .leftJoin(teachingUnits, eq(teachingUnits.topicId, topics.id))
    .where(and(eq(topics.courseId, tree.course.id), isNull(teachingUnits.id)));
  if (orphanRows.length > 0) {
    const list = orphanRows
      .map((r) => `${r.topicId} "${r.title}"`)
      .join("; ");
    throw new Error(`post-insert: topics with no linked unit: ${list}`);
  }

  // Course title sanity (Codex dispatch-3 IMPORTANT): just confirm
  // the course is still there with the YAML title. Cheap; catches
  // the case where someone manually deleted the course mid-import.
  const [c] = await tx
    .select({ title: courses.title })
    .from(courses)
    .where(eq(courses.id, tree.course.id));
  if (!c) {
    throw new Error(
      `post-insert: course ${tree.course.id} disappeared mid-transaction`,
    );
  }
  if (c.title !== tree.course.title) {
    throw new Error(
      `post-insert: course title mismatch (DB=${JSON.stringify(c.title)}, YAML=${JSON.stringify(tree.course.title)})`,
    );
  }
}

// =========================================================
// Demo class wire-up
// =========================================================
//
// Bridge HQ owns the canonical Python 101 (platform-scope library).
// Plain teachers can READ platform units (via canViewUnit's
// classroom_ready rule) but CANNOT edit them — canEditUnit requires
// platform_admin for scope='platform'. So if the demo class points
// directly at Bridge HQ's course, eve@demo.edu can open the units
// but every save fails with "Not authorized to edit document".
//
// Fix: clone the course tree into Bridge Demo School org, owned by
// eve. Cloned units are scope='org', scope_id=Bridge Demo School →
// canEditUnit lets active-teacher members of that org edit. Problems
// stay platform-scope and are referenced (not cloned) by both the
// canonical and cloned topic_problems rows.
//
// Idempotent: detects an existing clone by its well-known UUID and
// only performs the clone once.

const BRIDGE_DEMO_SCHOOL_ORG_ID = "d386983b-6da4-4cb8-8057-f2aa70d27c07";
const EVE_DEMO_USER_ID = "d0d3b031-a483-4214-97fb-48c9584f4dcb";
const DEMO_CLASS_ID = "00000000-0000-0000-0000-000000400101";

// Well-known UUID for the demo-class clone of Bridge HQ Python 101.
// Hard-coded so the wire-up is idempotent without needing a separate
// lookup table.
const DEMO_CLONE_COURSE_ID = "00000000-0000-0000-0000-de7000000001";

interface DemoWireSummary {
  cloned: boolean; // true if a fresh clone was made; false if the existing one was reused
  cloneCourseId: string;
  unitCount: number;
}

async function wireDemoClass(
  tx: Tx,
  sourceCourseId: string,
): Promise<DemoWireSummary> {
  // Refuse to run if the prerequisites aren't there. Each is a
  // separately-installable seed; failure modes should be loud.
  const [demoOrg] = await tx.execute(
    sql`SELECT 1 FROM organizations WHERE id = ${BRIDGE_DEMO_SCHOOL_ORG_ID}`,
  );
  if (!demoOrg) {
    throw new Error(
      `wire-demo-class: Bridge Demo School org (${BRIDGE_DEMO_SCHOOL_ORG_ID}) does not exist; run scripts/seed_problem_demo.sql first`,
    );
  }
  const [eve] = await tx.execute(
    sql`SELECT 1 FROM users WHERE id = ${EVE_DEMO_USER_ID}`,
  );
  if (!eve) {
    throw new Error(
      `wire-demo-class: eve@demo.edu (${EVE_DEMO_USER_ID}) does not exist; run scripts/seed_problem_demo.sql first`,
    );
  }
  const [demoClass] = await tx
    .select({ id: classes.id, courseId: classes.courseId })
    .from(classes)
    .where(eq(classes.id, DEMO_CLASS_ID));
  if (!demoClass) {
    throw new Error(
      `wire-demo-class: demo class (${DEMO_CLASS_ID}) does not exist; run scripts/seed_problem_demo.sql first`,
    );
  }

  // Detect an existing clone by its well-known UUID.
  const [existingClone] = await tx
    .select({ id: courses.id, title: courses.title })
    .from(courses)
    .where(eq(courses.id, DEMO_CLONE_COURSE_ID));

  if (existingClone) {
    // Already cloned. Just ensure the demo class points at it.
    if (demoClass.courseId !== DEMO_CLONE_COURSE_ID) {
      await tx
        .update(classes)
        .set({ courseId: DEMO_CLONE_COURSE_ID, updatedAt: new Date() })
        .where(eq(classes.id, DEMO_CLASS_ID));
    }
    const cloneTopics = await tx
      .select({ id: topics.id })
      .from(topics)
      .where(eq(topics.courseId, DEMO_CLONE_COURSE_ID));
    return {
      cloned: false,
      cloneCourseId: DEMO_CLONE_COURSE_ID,
      unitCount: cloneTopics.length,
    };
  }

  // Fresh clone. Get the source course details.
  const [sourceCourse] = await tx
    .select()
    .from(courses)
    .where(eq(courses.id, sourceCourseId));
  if (!sourceCourse) {
    throw new Error(
      `wire-demo-class: source course ${sourceCourseId} not found; run the importer's main pass first`,
    );
  }

  // Clone the course into Bridge Demo School, owned by eve. Title
  // stays the same (no " (Copy)" suffix — this IS the demo class's
  // version).
  await tx.insert(courses).values({
    id: DEMO_CLONE_COURSE_ID,
    orgId: BRIDGE_DEMO_SCHOOL_ORG_ID,
    createdBy: EVE_DEMO_USER_ID,
    title: sourceCourse.title,
    description: sourceCourse.description,
    gradeLevel: sourceCourse.gradeLevel,
    language: sourceCourse.language,
    isPublished: true,
  });

  // Clone topics, units, unit_documents, and topic_problems in
  // source order. For each source topic:
  //   - Clone the topic under the new course (gen_random_uuid).
  //   - Find the unit linked to the source topic (1:1 invariant).
  //   - Clone the unit into Bridge Demo School org.
  //   - Clone the unit_document (problem-ref attrs reference platform
  //     problems and don't need to change).
  //   - Link the cloned topic to the cloned unit.
  //   - Copy topic_problems entries to the cloned topic.
  const sourceTopicsRows = await tx
    .select({
      id: topics.id,
      title: topics.title,
      description: topics.description,
      sortOrder: topics.sortOrder,
    })
    .from(topics)
    .where(eq(topics.courseId, sourceCourseId))
    .orderBy(topics.sortOrder);

  let unitCount = 0;
  for (const st of sourceTopicsRows) {
    // Clone topic.
    const [clonedTopic] = await tx
      .insert(topics)
      .values({
        courseId: DEMO_CLONE_COURSE_ID,
        title: st.title,
        description: st.description,
        sortOrder: st.sortOrder,
      })
      .returning({ id: topics.id });

    // Find the source unit linked to this topic.
    const [sourceUnit] = await tx
      .select()
      .from(teachingUnits)
      .where(eq(teachingUnits.topicId, st.id));
    if (!sourceUnit) {
      throw new Error(
        `wire-demo-class: source topic ${st.id} has no linked unit (1:1 invariant violated)`,
      );
    }

    // Clone unit into Bridge Demo School org. Slug must be unique
    // per (scope, scope_id) so suffix it with "-demo" to avoid
    // colliding with any existing org-scope unit. status starts at
    // classroom_ready so eve doesn't have to transition each one.
    const [clonedUnit] = await tx
      .insert(teachingUnits)
      .values({
        scope: "org",
        scopeId: BRIDGE_DEMO_SCHOOL_ORG_ID,
        title: sourceUnit.title,
        slug: sourceUnit.slug ? `${sourceUnit.slug}-demo` : null,
        summary: sourceUnit.summary,
        gradeLevel: sourceUnit.gradeLevel,
        subjectTags: sourceUnit.subjectTags,
        standardsTags: sourceUnit.standardsTags,
        estimatedMinutes: sourceUnit.estimatedMinutes,
        materialType: sourceUnit.materialType,
        status: "classroom_ready",
        topicId: null,
        createdBy: EVE_DEMO_USER_ID,
      })
      .returning({ id: teachingUnits.id });

    // Clone unit_document (the blocks JSON references platform
    // problems via problemId attrs — those stay valid).
    const [sourceDoc] = await tx
      .select({ blocks: unitDocuments.blocks })
      .from(unitDocuments)
      .where(eq(unitDocuments.unitId, sourceUnit.id));
    if (sourceDoc) {
      await tx.insert(unitDocuments).values({
        unitId: clonedUnit.id,
        blocks: sourceDoc.blocks,
      });
    }

    // Link cloned topic ↔ cloned unit (1:1).
    await tx
      .update(teachingUnits)
      .set({ topicId: clonedTopic.id })
      .where(eq(teachingUnits.id, clonedUnit.id));

    // Copy topic_problems for the cloned topic. Problems stay
    // platform-scope and shared.
    const sourceTPs = await tx
      .select()
      .from(topicProblems)
      .where(eq(topicProblems.topicId, st.id));
    for (const tp of sourceTPs) {
      await tx.insert(topicProblems).values({
        topicId: clonedTopic.id,
        problemId: tp.problemId,
        sortOrder: tp.sortOrder,
        attachedBy: EVE_DEMO_USER_ID,
      });
    }

    unitCount++;
  }

  // Update the demo class's course_id to point at the clone.
  await tx
    .update(classes)
    .set({ courseId: DEMO_CLONE_COURSE_ID, updatedAt: new Date() })
    .where(eq(classes.id, DEMO_CLASS_ID));

  return {
    cloned: true,
    cloneCourseId: DEMO_CLONE_COURSE_ID,
    unitCount,
  };
}

// =========================================================
// Main
// =========================================================

interface ImportSummary {
  courseId: string;
  unitCount: number;
  problemCount: number;
  testCaseCount: number;
  applied: boolean;
  libraryOnly: boolean;
  demoWire?: DemoWireSummary;
}

export async function runImporter(args: CliArgs): Promise<ImportSummary> {
  const tree = await loadContentTree(args.root);

  if (!args.skipSandbox) {
    const result = await preflightSandbox(tree);
    if (!result.ok) {
      reportSandboxFailures(result.failures);
      throw new Error(`${result.failures.length} sandbox pre-flight failure(s)`);
    }
  } else {
    process.stderr.write(
      "WARNING: --skip-sandbox is set; reference solutions are NOT verified.\n",
    );
  }

  const problemCount = [...tree.units.values()].reduce(
    (n, u) => n + u.problems.length,
    0,
  );
  const testCaseCount = [...tree.units.values()].reduce(
    (n, u) => n + u.problems.reduce((m, p) => m + p.testCases.length, 0),
    0,
  );

  if (!args.apply) {
    return {
      courseId: tree.course.id,
      unitCount: tree.units.size,
      problemCount,
      testCaseCount,
      applied: false,
      libraryOnly: args.libraryOnly,
    };
  }

  const connectionString = args.targetDb ?? process.env.DATABASE_URL;
  if (!connectionString) {
    throw new Error("DATABASE_URL not set and --target-db not provided");
  }
  const client: Sql = postgres(connectionString, { max: 1 });
  const db = drizzle(client);

  let demoWire: DemoWireSummary | undefined;
  try {
    await db.transaction(async (tx) => {
      const idIssues = await checkIdentities(tx, tree, args.allowRename);
      if (idIssues.length > 0) {
        const lines = idIssues
          .map((i) => `  ${i.file}: ${i.message}`)
          .join("\n");
        throw new Error(`identity check failed:\n${lines}`);
      }

      await runLibraryPass(tx, tree);

      if (args.libraryOnly) {
        // Skip course / link / verification passes.
        return;
      }

      await runCoursePass(tx, tree);
      await runLinkPass(tx, tree);
      await postInsertVerification(tx, tree);

      if (args.wireDemoClass) {
        demoWire = await wireDemoClass(tx, tree.course.id);
      }
    });
  } finally {
    await client.end();
  }

  return {
    courseId: tree.course.id,
    unitCount: tree.units.size,
    problemCount,
    testCaseCount,
    applied: true,
    libraryOnly: args.libraryOnly,
    demoWire,
  };
}

function isMainModule(): boolean {
  const argv1 = process.argv[1] ?? "";
  if (!argv1) return false;
  // Run via `bun run scripts/python-101/import.ts` ends up with
  // process.argv[1] equal to the absolute path of this file. The
  // ../scripts/python-101/import.ts suffix is sufficient to match
  // both forward-slash and backslash environments.
  return argv1.endsWith("/import.ts") || argv1.endsWith("\\import.ts");
}

if (isMainModule()) {
  const args = parseArgs(process.argv.slice(2));
  if (args.pistonUrl) process.env.PISTON_URL = args.pistonUrl;
  runImporter(args)
    .then((summary) => {
      if (summary.applied) {
        process.stdout.write(
          `OK: applied ${summary.libraryOnly ? "library-only" : "full"} import — course ${summary.courseId}, ${summary.unitCount} unit(s), ${summary.problemCount} problem(s), ${summary.testCaseCount} test case(s)\n`,
        );
        if (summary.demoWire) {
          const verb = summary.demoWire.cloned ? "cloned" : "verified existing";
          process.stdout.write(
            `OK: demo class ${verb} — course ${summary.demoWire.cloneCourseId} owned by Bridge Demo School, ${summary.demoWire.unitCount} unit(s)\n`,
          );
        }
      } else {
        process.stdout.write(
          `OK (dry-run): course ${summary.courseId}, ${summary.unitCount} unit(s), ${summary.problemCount} problem(s), ${summary.testCaseCount} test case(s). Pass --apply to write to the DB.\n`,
        );
      }
    })
    .catch((err) => {
      process.stderr.write(
        `\nFAIL: ${err instanceof Error ? err.message : String(err)}\n`,
      );
      process.exit(1);
    });
}
