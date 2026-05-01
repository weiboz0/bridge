import { describe, expect, it } from "vitest";
import { v4 as uuidv4 } from "uuid";
import {
  courseManifestSchema,
  parseAuthoringYaml,
  unitFileSchema,
  validateContentTree,
  type CourseManifest,
  type UnitFile,
} from "../../scripts/python-101/schema";

// =========================================================
// Builders for valid examples
// =========================================================

function validProblem(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: uuidv4(),
    slug: "hello-world",
    title: "Hello, World",
    description: "Print Hello, World!",
    difficulty: "easy",
    gradeLevel: "9-12",
    tags: ["output"],
    starterCode: { python: 'print("Hello, World!")\n' },
    solution: { language: "python", code: 'print("Hello, World!")\n' },
    testCases: [
      { name: "Example", stdin: "", expectedStdout: "Hello, World!", isExample: true },
    ],
    ...overrides,
  };
}

function validUnit(overrides: Partial<Record<string, unknown>> = {}): unknown {
  return {
    id: uuidv4(),
    slug: "print-and-comments",
    title: "1. Print & Comments",
    description: "Your first program: printing output.",
    gradeLevel: "9-12",
    subjectTags: ["python", "intro"],
    standardsTags: [],
    materialType: "notes",
    blocks: [
      { type: "heading", text: "Big idea" },
      { type: "paragraph", text: "Programs print things." },
      { type: "problem-ref", problemSlug: "hello-world", visibility: "always" },
    ],
    problems: [validProblem()],
    ...overrides,
  };
}

function validCourse(overrides: Partial<Record<string, unknown>> = {}): unknown {
  return {
    id: uuidv4(),
    title: "Python 101 — Introduction to Programming",
    description: "A first course in Python.",
    gradeLevel: "9-12",
    language: "python",
    topics: [
      { id: uuidv4(), unitSlug: "print-and-comments" },
      { id: uuidv4(), unitSlug: "variables-and-types" },
    ],
    ...overrides,
  };
}

// =========================================================
// Per-file: unit
// =========================================================

describe("unitFileSchema", () => {
  it("accepts a minimal valid unit", () => {
    const result = unitFileSchema.safeParse(validUnit());
    expect(result.success).toBe(true);
  });

  it("rejects a missing problem id", () => {
    const bad = validUnit({
      problems: [{ ...(validProblem() as object), id: undefined }],
    });
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects a non-uuidv4 id", () => {
    const bad = validUnit({ id: "not-a-uuid" });
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects a problem id that is uuidv1 (wrong version)", () => {
    const v1Like = "12345678-1234-1234-8234-123456789abc";
    const bad = validUnit({
      problems: [validProblem({ id: v1Like })],
    });
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects a unit with zero problems", () => {
    const bad = validUnit({ problems: [] });
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects an upper-case slug", () => {
    const bad = validUnit({ slug: "Print-And-Comments" });
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects duplicate problem slugs", () => {
    const bad = validUnit({
      problems: [validProblem(), validProblem({ id: uuidv4() })],
    });
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects duplicate problem ids", () => {
    const id = uuidv4();
    const bad = validUnit({
      problems: [
        validProblem({ id, slug: "p1" }),
        validProblem({ id, slug: "p2" }),
      ],
    });
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects a problem-ref block referencing a missing problem slug", () => {
    const bad = validUnit({
      blocks: [
        { type: "problem-ref", problemSlug: "no-such-problem", visibility: "always" },
      ],
    });
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects a test case set with no examples", () => {
    const bad = validUnit({
      problems: [
        validProblem({
          testCases: [
            { name: "Hidden only", stdin: "", expectedStdout: "x", isExample: false },
          ],
        }),
      ],
    });
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects starterCode missing python", () => {
    const bad = validUnit({
      problems: [
        validProblem({ starterCode: { javascript: "console.log('hi')" } }),
      ],
    });
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects unknown top-level keys (strict)", () => {
    const bad = { ...(validUnit() as object), extraneous: "not allowed" };
    const result = unitFileSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });
});

// =========================================================
// Per-file: course manifest
// =========================================================

describe("courseManifestSchema", () => {
  it("accepts a minimal valid course manifest", () => {
    const result = courseManifestSchema.safeParse(validCourse());
    expect(result.success).toBe(true);
  });

  it("rejects duplicate topic ids", () => {
    const id = uuidv4();
    const bad = validCourse({
      topics: [
        { id, unitSlug: "print-and-comments" },
        { id, unitSlug: "variables-and-types" },
      ],
    });
    const result = courseManifestSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects duplicate unitSlug entries", () => {
    const bad = validCourse({
      topics: [
        { id: uuidv4(), unitSlug: "print-and-comments" },
        { id: uuidv4(), unitSlug: "print-and-comments" },
      ],
    });
    const result = courseManifestSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects empty topics list", () => {
    const bad = validCourse({ topics: [] });
    const result = courseManifestSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });

  it("rejects language other than python", () => {
    const bad = validCourse({ language: "javascript" });
    const result = courseManifestSchema.safeParse(bad);
    expect(result.success).toBe(false);
  });
});

// =========================================================
// Cross-file: validateContentTree
// =========================================================

describe("validateContentTree", () => {
  function tree(course: CourseManifest, units: UnitFile[]) {
    return {
      course,
      units: new Map(units.map((u) => [u.slug, u])),
    };
  }

  it("passes a valid tree", () => {
    const u1 = unitFileSchema.parse(
      validUnit({ slug: "print-and-comments" }),
    );
    const u2 = unitFileSchema.parse(
      validUnit({
        slug: "variables-and-types",
        title: "2. Variables",
        blocks: [
          { type: "heading", text: "Variables" },
          {
            type: "problem-ref",
            problemSlug: "greet-by-name",
            visibility: "always",
          },
        ],
        problems: [
          validProblem({
            slug: "greet-by-name",
            title: "Greet by Name",
            description: "Print Hello, {name}!",
            starterCode: { python: "name = input()\n" },
            solution: {
              language: "python",
              code: 'name = input()\nprint(f"Hello, {name}!")\n',
            },
            testCases: [
              { name: "Ex", stdin: "Ada", expectedStdout: "Hello, Ada!", isExample: true },
            ],
          }),
        ],
      }),
    );
    const c = courseManifestSchema.parse(
      validCourse({
        topics: [
          { id: uuidv4(), unitSlug: "print-and-comments" },
          { id: uuidv4(), unitSlug: "variables-and-types" },
        ],
      }),
    );
    expect(validateContentTree(tree(c, [u1, u2]))).toEqual([]);
  });

  it("flags a course unitSlug with no matching file", () => {
    const u1 = unitFileSchema.parse(validUnit({ slug: "print-and-comments" }));
    const c = courseManifestSchema.parse(
      validCourse({
        topics: [
          { id: uuidv4(), unitSlug: "print-and-comments" },
          { id: uuidv4(), unitSlug: "missing-unit" },
        ],
      }),
    );
    const issues = validateContentTree(tree(c, [u1]));
    expect(issues).toHaveLength(1);
    expect(issues[0].message).toContain("missing-unit");
  });

  it("flags an orphan unit file the course doesn't reference", () => {
    const u1 = unitFileSchema.parse(validUnit({ slug: "print-and-comments" }));
    const u2 = unitFileSchema.parse(
      validUnit({
        slug: "orphan-unit",
        title: "Orphan",
        problems: [validProblem({ slug: "orphan-problem" })],
        blocks: [
          {
            type: "problem-ref",
            problemSlug: "orphan-problem",
            visibility: "always",
          },
        ],
      }),
    );
    const c = courseManifestSchema.parse(
      validCourse({
        topics: [{ id: uuidv4(), unitSlug: "print-and-comments" }],
      }),
    );
    const issues = validateContentTree(tree(c, [u1, u2]));
    expect(issues.some((i) => i.message.includes("not referenced"))).toBe(true);
  });

  it("flags a problem id collision across units", () => {
    const sharedId = uuidv4();
    const u1 = unitFileSchema.parse(
      validUnit({
        slug: "print-and-comments",
        problems: [validProblem({ id: sharedId, slug: "p1" })],
        blocks: [{ type: "problem-ref", problemSlug: "p1", visibility: "always" }],
      }),
    );
    const u2 = unitFileSchema.parse(
      validUnit({
        slug: "variables-and-types",
        problems: [validProblem({ id: sharedId, slug: "p2" })],
        blocks: [{ type: "problem-ref", problemSlug: "p2", visibility: "always" }],
      }),
    );
    const c = courseManifestSchema.parse(
      validCourse({
        topics: [
          { id: uuidv4(), unitSlug: "print-and-comments" },
          { id: uuidv4(), unitSlug: "variables-and-types" },
        ],
      }),
    );
    const issues = validateContentTree(tree(c, [u1, u2]));
    expect(issues.some((i) => i.message.includes("already used"))).toBe(true);
  });

  it("flags a problem slug collision across units", () => {
    const u1 = unitFileSchema.parse(
      validUnit({
        slug: "print-and-comments",
        problems: [validProblem({ slug: "shared" })],
        blocks: [{ type: "problem-ref", problemSlug: "shared", visibility: "always" }],
      }),
    );
    const u2 = unitFileSchema.parse(
      validUnit({
        slug: "variables-and-types",
        problems: [validProblem({ id: uuidv4(), slug: "shared" })],
        blocks: [{ type: "problem-ref", problemSlug: "shared", visibility: "always" }],
      }),
    );
    const c = courseManifestSchema.parse(
      validCourse({
        topics: [
          { id: uuidv4(), unitSlug: "print-and-comments" },
          { id: uuidv4(), unitSlug: "variables-and-types" },
        ],
      }),
    );
    const issues = validateContentTree(tree(c, [u1, u2]));
    expect(
      issues.some((i) => i.message.includes('problem slug "shared" already used')),
    ).toBe(true);
  });
});

// =========================================================
// YAML parser policy
// =========================================================

describe("parseAuthoringYaml", () => {
  it("parses a plain yaml document", () => {
    const result = parseAuthoringYaml("title: hello\nblocks:\n  - foo\n  - bar\n");
    expect(result).toEqual({ title: "hello", blocks: ["foo", "bar"] });
  });

  it("rejects yaml that uses anchors and aliases", () => {
    const yaml = [
      "shared: &shared",
      "  - one",
      "  - two",
      "first: *shared",
      "second: *shared",
    ].join("\n");
    expect(() => parseAuthoringYaml(yaml)).toThrow(/anchor/i);
  });

  it("rejects yaml with merge keys", () => {
    const yaml = [
      "base: &base",
      "  a: 1",
      "  b: 2",
      "child:",
      "  <<: *base",
      "  c: 3",
    ].join("\n");
    expect(() => parseAuthoringYaml(yaml)).toThrow();
  });

  it("rejects malformed yaml", () => {
    expect(() => parseAuthoringYaml("foo: : :\n")).toThrow();
  });

  it("preserves block scalars", () => {
    const yaml = ['code: |', '  print("hi")', '  print("bye")'].join("\n");
    const out = parseAuthoringYaml(yaml) as { code: string };
    expect(out.code).toBe('print("hi")\nprint("bye")\n');
  });
});
