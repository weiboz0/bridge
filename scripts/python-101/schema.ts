/**
 * Zod schemas for the Python 101 authoring format (plan 049).
 *
 * Two top-level files:
 *   content/python-101/course.yaml    -> CourseManifest
 *   content/python-101/units/*.yaml    -> UnitFile (one per unit)
 *
 * The validator walks the tree, parses each file with the safe YAML
 * settings (no anchors, aliases, or merge keys), then runs the
 * appropriate Zod schema. Cross-file invariants (e.g., every slug in
 * course.yaml exists, every problemSlug references a problem in the
 * unit) are checked outside Zod by `validateContentTree` below.
 *
 * Stable identity (Codex CRITICAL pass-3): every unit, problem, topic,
 * and course carries an explicit `id` field — a uuidv4 generated ONCE
 * at file creation time. The importer treats `id` as the primary key
 * and slugs as renamable. This decouples DB identity from user-visible
 * naming.
 */

import { isAlias, isScalar, parseDocument, visit, type Document } from "yaml";
import { z } from "zod";

// =========================================================
// Reusable primitives
// =========================================================

const uuidV4 = z
  .string()
  .regex(
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
    "must be a uuidv4 (lowercase recommended)",
  );

const slug = z
  .string()
  .regex(
    /^[a-z0-9]+(?:-[a-z0-9]+)*$/,
    "lowercase letters, digits, and single hyphens only",
  )
  .min(2)
  .max(80);

const nonEmptyString = z.string().min(1);

const gradeLevel = z.enum(["K-2", "3-5", "6-8", "9-12"]);

const difficulty = z.enum(["easy", "medium", "hard"]);

const tagList = z.array(z.string().min(1).max(40)).default([]);

// =========================================================
// Test case
// =========================================================

const testCase = z
  .object({
    name: nonEmptyString.max(80),
    stdin: z.string(),
    // expected_stdout (table column) is exact-match. The grader
    // normalizes trailing whitespace; authors should write outputs
    // WITHOUT a trailing newline (it is added by `print`).
    expectedStdout: z.string(),
    isExample: z.boolean(),
  })
  .strict();

// =========================================================
// Problem
// =========================================================

const problem = z
  .object({
    id: uuidV4,
    slug,
    title: nonEmptyString.max(120),
    description: nonEmptyString,
    difficulty,
    gradeLevel,
    tags: tagList,
    // language -> starter source. Python 101 only ships python; the
    // schema accepts any non-empty key for forward compatibility.
    starterCode: z.record(z.string().min(1), z.string()),
    // Reference solution (one language only for plan 049).
    solution: z
      .object({
        language: z.literal("python"),
        code: nonEmptyString,
      })
      .strict(),
    timeLimitMs: z.number().int().positive().optional(),
    memoryLimitMb: z.number().int().positive().optional(),
    testCases: z.array(testCase).min(1),
  })
  .strict()
  .refine(
    (p) => Object.prototype.hasOwnProperty.call(p.starterCode, "python"),
    { message: "starterCode must include a 'python' entry" },
  )
  .refine(
    (p) => p.testCases.some((tc) => tc.isExample),
    { message: "at least one test case must have isExample: true" },
  );

// =========================================================
// Unit
// =========================================================

// A `problem-ref` block embeds a problem id into the unit document.
// Mirrors the runtime block shape stored in `unit_documents.blocks`.
const problemRefBlock = z
  .object({
    type: z.literal("problem-ref"),
    problemSlug: slug,
    visibility: z.enum(["always", "after-attempt"]).default("always"),
  })
  .strict();

const proseBlock = z
  .object({
    type: z.enum(["heading", "paragraph", "list", "callout", "code"]),
    text: nonEmptyString,
  })
  .strict();

const unitBlock = z.discriminatedUnion("type", [
  problemRefBlock,
  // The prose block validates the same shape for several types using a
  // permissive `text` field. The importer translates these into the
  // `unit_documents.blocks` JSON the editor renders.
  proseBlock.extend({ type: z.literal("heading") }).strict(),
  proseBlock.extend({ type: z.literal("paragraph") }).strict(),
  proseBlock.extend({ type: z.literal("list") }).strict(),
  proseBlock.extend({ type: z.literal("callout") }).strict(),
  proseBlock.extend({ type: z.literal("code") }).strict(),
]);

export const unitFileSchema = z
  .object({
    id: uuidV4,
    slug,
    title: nonEmptyString.max(120),
    description: nonEmptyString,
    gradeLevel,
    subjectTags: tagList,
    standardsTags: tagList,
    estimatedMinutes: z.number().int().positive().max(600).optional(),
    materialType: z.enum(["notes", "lesson", "worksheet"]).default("notes"),
    blocks: z.array(unitBlock).default([]),
    problems: z.array(problem).min(1),
  })
  .strict()
  .refine(
    (u) => {
      const ids = new Set<string>();
      for (const p of u.problems) {
        if (ids.has(p.id)) return false;
        ids.add(p.id);
      }
      return true;
    },
    { message: "duplicate problem id within unit" },
  )
  .refine(
    (u) => {
      const slugs = new Set<string>();
      for (const p of u.problems) {
        if (slugs.has(p.slug)) return false;
        slugs.add(p.slug);
      }
      return true;
    },
    { message: "duplicate problem slug within unit" },
  )
  .refine(
    (u) => {
      const known = new Set(u.problems.map((p) => p.slug));
      for (const block of u.blocks) {
        if (block.type === "problem-ref" && !known.has(block.problemSlug)) {
          return false;
        }
      }
      return true;
    },
    { message: "problem-ref block references unknown problem slug" },
  );

export type UnitFile = z.infer<typeof unitFileSchema>;

// =========================================================
// Course manifest
// =========================================================

const courseTopicEntry = z
  .object({
    id: uuidV4, // topics.id
    unitSlug: slug, // -> resolves to a unit file
  })
  .strict();

export const courseManifestSchema = z
  .object({
    id: uuidV4, // courses.id
    title: nonEmptyString.max(180),
    description: nonEmptyString,
    gradeLevel,
    language: z.literal("python"),
    topics: z.array(courseTopicEntry).min(1),
  })
  .strict()
  .refine(
    (c) => {
      const ids = new Set<string>();
      for (const t of c.topics) {
        if (ids.has(t.id)) return false;
        ids.add(t.id);
      }
      return true;
    },
    { message: "duplicate topic id in course manifest" },
  )
  .refine(
    (c) => {
      const slugs = new Set<string>();
      for (const t of c.topics) {
        if (slugs.has(t.unitSlug)) return false;
        slugs.add(t.unitSlug);
      }
      return true;
    },
    { message: "duplicate unitSlug in course manifest" },
  );

export type CourseManifest = z.infer<typeof courseManifestSchema>;

// =========================================================
// YAML parser
// =========================================================

/**
 * Parses YAML using the conservative settings required by the
 * authoring format: no anchors, no aliases, no merge keys. Multiline
 * strings must use `|`-block scalars (yaml lib enforces this when the
 * input doesn't use the disallowed features).
 */
export function parseAuthoringYaml(input: string): unknown {
  const doc: Document.Parsed = parseDocument(input, {
    merge: false,
    schema: "core",
  });
  if (doc.errors.length > 0) {
    throw new Error(
      `YAML parse error: ${doc.errors.map((e) => e.message).join("; ")}`,
    );
  }
  // Anchor / alias detection: walk the document and reject any
  // anchor or alias node. The yaml lib doesn't fail on them with
  // schema:'core', so we enforce the policy ourselves.
  let anchorOrAlias = false;
  visit(doc, (_key, node) => {
    if (isAlias(node)) {
      anchorOrAlias = true;
      return visit.BREAK;
    }
    if (
      (isScalar(node) ||
        (node && typeof node === "object" && "items" in node)) &&
      typeof (node as { anchor?: unknown }).anchor === "string" &&
      (node as { anchor: string }).anchor.length > 0
    ) {
      anchorOrAlias = true;
      return visit.BREAK;
    }
    return undefined;
  });
  if (anchorOrAlias) {
    throw new Error(
      "YAML anchors / aliases are not allowed in Python 101 content",
    );
  }
  return doc.toJS();
}

// =========================================================
// Cross-file validation
// =========================================================

export interface ContentTree {
  course: CourseManifest;
  units: Map<string, UnitFile>; // keyed by slug
}

export interface ValidationIssue {
  file: string;
  message: string;
  path?: (string | number)[];
}

/**
 * Cross-file invariants the per-file schemas can't express:
 *
 *   1. Every course.topics[].unitSlug resolves to a unit file.
 *   2. Every unit file referenced by the course manifest exists once.
 *   3. No duplicate ids across the tree (units, problems, topics).
 *   4. No duplicate slugs across units; no duplicate slugs across
 *      problems (within the platform-scope library).
 *   5. The course's `language: python` matches every problem solution.
 */
export function validateContentTree(tree: ContentTree): ValidationIssue[] {
  const issues: ValidationIssue[] = [];

  // (1) + (2): course unitSlug resolution.
  const expectedSlugs = new Set(tree.course.topics.map((t) => t.unitSlug));
  for (const slug of expectedSlugs) {
    if (!tree.units.has(slug)) {
      issues.push({
        file: "course.yaml",
        message: `topics references unitSlug "${slug}" but no units/${slug}.yaml file exists`,
      });
    }
  }
  for (const [unitSlug] of tree.units) {
    if (!expectedSlugs.has(unitSlug)) {
      issues.push({
        file: `units/${unitSlug}.yaml`,
        message: `unit is not referenced by course.yaml topics`,
      });
    }
  }

  // (3): id uniqueness across the entire tree.
  const idsSeen = new Map<string, string>(); // id -> first seen at
  function claim(id: string, where: string) {
    const prior = idsSeen.get(id);
    if (prior) {
      issues.push({
        file: where,
        message: `id ${id} is already used by ${prior}`,
      });
    } else {
      idsSeen.set(id, where);
    }
  }
  claim(tree.course.id, "course.yaml#id");
  for (const t of tree.course.topics) claim(t.id, `course.yaml#topics[${t.unitSlug}]`);
  for (const [slug, unit] of tree.units) {
    claim(unit.id, `units/${slug}.yaml#id`);
    for (const p of unit.problems) {
      claim(p.id, `units/${slug}.yaml#problems[${p.slug}]`);
    }
  }

  // (4): problem-slug uniqueness across units (problems are platform-
  // scope library content and share a `(scope, scope_id, slug)`
  // unique constraint).
  const problemSlugSeen = new Map<string, string>();
  for (const [unitSlug, unit] of tree.units) {
    for (const p of unit.problems) {
      const prior = problemSlugSeen.get(p.slug);
      if (prior) {
        issues.push({
          file: `units/${unitSlug}.yaml`,
          message: `problem slug "${p.slug}" already used by ${prior}`,
        });
      } else {
        problemSlugSeen.set(p.slug, `units/${unitSlug}.yaml`);
      }
    }
  }

  // (5): course.language consistency. Schema already pins to "python"
  // but defend against a mismatch if either schema relaxes later.
  if (tree.course.language !== "python") {
    issues.push({
      file: "course.yaml",
      message: `course language must be "python" (got "${tree.course.language}")`,
    });
  }

  return issues;
}
