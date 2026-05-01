import { afterAll, beforeEach, describe, expect, it } from "vitest";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { v4 as uuidv4 } from "uuid";
import { eq, sql } from "drizzle-orm";
import { testDb, cleanupDatabase } from "../helpers";
import {
  courses,
  organizations,
  orgMemberships,
  problems,
  problemSolutions,
  teachingUnits,
  topicProblems,
  topics,
  unitDocuments,
  users,
} from "@/lib/db/schema";
import { runImporter } from "../../scripts/python-101/import";

const BRIDGE_HQ_ORG_ID = "00000000-0000-0000-0000-bbbbbbbbb002";
const BRIDGE_HQ_SYSTEM_USER_ID = "00000000-0000-0000-0000-bbbbbbbbb001";
const BRIDGE_HQ_MEMBERSHIP_ID = "00000000-0000-0000-0000-bbbbbbbbb003";

const TARGET_DB =
  process.env.DATABASE_URL || "postgresql://work@127.0.0.1:5432/bridge_test";

const tmpDirs: string[] = [];

afterAll(async () => {
  for (const d of tmpDirs) {
    await rm(d, { recursive: true, force: true }).catch(() => {});
  }
});

async function ensureBridgeHq() {
  // Idempotent re-seed of Bridge HQ since cleanupDatabase wipes users
  // and organizations.
  await testDb.insert(users).values({
    id: BRIDGE_HQ_SYSTEM_USER_ID,
    name: "Bridge HQ System",
    email: "system@bridge.platform",
    passwordHash: null,
    isPlatformAdmin: true,
  }).onConflictDoNothing();
  await testDb.insert(organizations).values({
    id: BRIDGE_HQ_ORG_ID,
    name: "Bridge HQ",
    slug: "bridge-hq",
    type: "school",
    status: "active",
    contactEmail: "system@bridge.platform",
    contactName: "Bridge HQ System",
    domain: "bridge.platform",
    settings: {},
    verifiedAt: new Date(),
  }).onConflictDoNothing();
  await testDb.insert(orgMemberships).values({
    id: BRIDGE_HQ_MEMBERSHIP_ID,
    orgId: BRIDGE_HQ_ORG_ID,
    userId: BRIDGE_HQ_SYSTEM_USER_ID,
    role: "org_admin",
    status: "active",
    invitedBy: null,
  }).onConflictDoNothing();
}

interface Tree {
  root: string;
  courseId: string;
  topicIds: string[];
  unitIds: string[];
  problemIds: string[];
}

async function writeTree(
  spec: {
    units: Array<{
      slug: string;
      problems: Array<{
        slug: string;
        solution: string;
        cases: Array<{ name: string; stdin: string; expected: string; isExample: boolean }>;
      }>;
    }>;
    courseTitle?: string;
  },
): Promise<Tree> {
  const root = await mkdtemp(join(tmpdir(), "p101-"));
  tmpDirs.push(root);
  await mkdir(join(root, "units"));

  const courseId = uuidv4();
  const topicIds: string[] = [];
  const unitIds: string[] = [];
  const problemIds: string[] = [];

  const courseLines: string[] = [
    `id: ${courseId}`,
    `title: ${spec.courseTitle ?? "Python 101 — Test"}`,
    `description: Test course for the importer.`,
    `gradeLevel: 9-12`,
    `language: python`,
    `topics:`,
  ];

  for (const u of spec.units) {
    const topicId = uuidv4();
    const unitId = uuidv4();
    topicIds.push(topicId);
    unitIds.push(unitId);
    courseLines.push(`  - id: ${topicId}`);
    courseLines.push(`    unitSlug: ${u.slug}`);

    const unitLines: string[] = [
      `id: ${unitId}`,
      `slug: ${u.slug}`,
      `title: Unit ${u.slug}`,
      `description: Test unit ${u.slug}.`,
      `gradeLevel: 9-12`,
      `subjectTags: []`,
      `standardsTags: []`,
      `materialType: notes`,
      `blocks:`,
    ];
    for (const p of u.problems) {
      unitLines.push(`  - type: problem-ref`);
      unitLines.push(`    problemSlug: ${p.slug}`);
      unitLines.push(`    visibility: always`);
    }
    unitLines.push(`problems:`);
    for (const p of u.problems) {
      const pid = uuidv4();
      problemIds.push(pid);
      unitLines.push(`  - id: ${pid}`);
      unitLines.push(`    slug: ${p.slug}`);
      unitLines.push(`    title: Problem ${p.slug}`);
      unitLines.push(`    description: Test problem ${p.slug}.`);
      unitLines.push(`    difficulty: easy`);
      unitLines.push(`    gradeLevel: 9-12`);
      unitLines.push(`    tags: []`);
      unitLines.push(`    starterCode:`);
      unitLines.push(`      python: |`);
      unitLines.push(`        # starter`);
      unitLines.push(`    solution:`);
      unitLines.push(`      language: python`);
      unitLines.push(`      code: |`);
      for (const line of p.solution.split("\n")) {
        unitLines.push(`        ${line}`);
      }
      unitLines.push(`    testCases:`);
      for (const tc of p.cases) {
        unitLines.push(`      - name: ${JSON.stringify(tc.name)}`);
        unitLines.push(`        stdin: ${JSON.stringify(tc.stdin)}`);
        unitLines.push(`        expectedStdout: ${JSON.stringify(tc.expected)}`);
        unitLines.push(`        isExample: ${tc.isExample}`);
      }
    }

    await writeFile(join(root, "units", `${u.slug}.yaml`), unitLines.join("\n") + "\n");
  }

  await writeFile(join(root, "course.yaml"), courseLines.join("\n") + "\n");
  return { root, courseId, topicIds, unitIds, problemIds };
}

describe("python-101 importer", () => {
  beforeEach(async () => {
    await cleanupDatabase();
    await ensureBridgeHq();
  });

  it("dry-run does not write to the DB", async () => {
    const tree = await writeTree({
      units: [
        {
          slug: "u1",
          problems: [
            {
              slug: "p1",
              solution: 'print("hi")',
              cases: [{ name: "ex", stdin: "", expected: "hi", isExample: true }],
            },
          ],
        },
      ],
    });

    const summary = await runImporter({
      root: tree.root,
      apply: false,
      libraryOnly: false,
      skipSandbox: true,
      allowRename: false,
    });
    expect(summary.applied).toBe(false);
    expect(summary.unitCount).toBe(1);
    expect(summary.problemCount).toBe(1);

    const courseRows = await testDb
      .select()
      .from(courses)
      .where(eq(courses.id, tree.courseId));
    expect(courseRows).toHaveLength(0);
  });

  it("applies the full import in one transaction", async () => {
    const tree = await writeTree({
      courseTitle: "Python 101 — Apply Test",
      units: [
        {
          slug: "u1",
          problems: [
            {
              slug: "p1",
              solution: 'print("hi")',
              cases: [{ name: "ex", stdin: "", expected: "hi", isExample: true }],
            },
            {
              slug: "p2",
              solution: 'print(int(input()) * 2)',
              cases: [
                { name: "ex", stdin: "3", expected: "6", isExample: true },
                { name: "hidden", stdin: "5", expected: "10", isExample: false },
              ],
            },
          ],
        },
        {
          slug: "u2",
          problems: [
            {
              slug: "p3",
              solution: 'print("bye")',
              cases: [{ name: "ex", stdin: "", expected: "bye", isExample: true }],
            },
          ],
        },
      ],
    });

    const summary = await runImporter({
      root: tree.root,
      apply: true,
      libraryOnly: false,
      skipSandbox: true,
      allowRename: false,
      targetDb: TARGET_DB,
    });
    expect(summary.applied).toBe(true);
    expect(summary.unitCount).toBe(2);
    expect(summary.problemCount).toBe(3);
    expect(summary.testCaseCount).toBe(4);

    const courseRows = await testDb
      .select()
      .from(courses)
      .where(eq(courses.id, tree.courseId));
    expect(courseRows).toHaveLength(1);
    expect(courseRows[0].orgId).toBe(BRIDGE_HQ_ORG_ID);
    expect(courseRows[0].title).toBe("Python 101 — Apply Test");

    const topicRows = await testDb
      .select()
      .from(topics)
      .where(eq(topics.courseId, tree.courseId));
    expect(topicRows).toHaveLength(2);
    expect(topicRows.map((t) => t.sortOrder).sort()).toEqual([0, 1]);

    // Both units have topic_id set.
    const unitRows = await testDb
      .select({ id: teachingUnits.id, topicId: teachingUnits.topicId })
      .from(teachingUnits);
    const ourUnits = unitRows.filter((r) => tree.unitIds.includes(r.id));
    expect(ourUnits).toHaveLength(2);
    for (const u of ourUnits) expect(u.topicId).not.toBeNull();

    // unit_documents rebuilt as { type: doc, content: [problem-ref...] }
    const docRows = await testDb.select().from(unitDocuments);
    const ourDocs = docRows.filter((d) => tree.unitIds.includes(d.unitId));
    expect(ourDocs).toHaveLength(2);
    const u1Doc = ourDocs.find(
      (d) => (d.blocks as { content?: unknown[] }).content?.length === 2,
    );
    expect(u1Doc).toBeTruthy();

    // topic_problems
    const topicProblemRows = await testDb
      .select()
      .from(topicProblems);
    const ourTp = topicProblemRows.filter((tp) =>
      tree.topicIds.includes(tp.topicId),
    );
    expect(ourTp).toHaveLength(3);

    // problem_solutions: exactly one per problem
    const solRows = await testDb.select().from(problemSolutions);
    const ourSols = solRows.filter((s) => tree.problemIds.includes(s.problemId));
    expect(ourSols).toHaveLength(3);
    for (const s of ourSols) {
      expect(s.isPublished).toBe(true);
      expect(s.language).toBe("python");
    }

    // test_cases: 4 total, all canonical (owner_id NULL)
    const tcRows = (await testDb.execute(
      sql`SELECT problem_id, owner_id, name, expected_stdout, is_example, "order" FROM test_cases ORDER BY problem_id, "order"`,
    )) as unknown as Array<{
      problem_id: string;
      owner_id: string | null;
      name: string;
      expected_stdout: string;
      is_example: boolean;
      order: number;
    }>;
    const ourCases = tcRows.filter((r) => tree.problemIds.includes(r.problem_id));
    expect(ourCases).toHaveLength(4);
    for (const c of ourCases) expect(c.owner_id).toBeNull();
  });

  it("re-running the importer is idempotent", async () => {
    const tree = await writeTree({
      units: [
        {
          slug: "u1",
          problems: [
            {
              slug: "p1",
              solution: 'print("hi")',
              cases: [{ name: "ex", stdin: "", expected: "hi", isExample: true }],
            },
          ],
        },
      ],
    });
    const baseArgs = {
      root: tree.root,
      apply: true,
      libraryOnly: false,
      skipSandbox: true,
      allowRename: false,
      targetDb: TARGET_DB,
    };
    await runImporter(baseArgs);
    await runImporter(baseArgs);

    const courseRows = await testDb.select().from(courses).where(eq(courses.id, tree.courseId));
    expect(courseRows).toHaveLength(1);
    const tcRows = (await testDb.execute(
      sql`SELECT count(*)::int AS n FROM test_cases WHERE problem_id = ${tree.problemIds[0]}`,
    )) as unknown as Array<{ n: number }>;
    expect(tcRows[0].n).toBe(1);
  });

  it("library-only stops after Pass 1 (no course / topics)", async () => {
    const tree = await writeTree({
      units: [
        {
          slug: "u1",
          problems: [
            {
              slug: "p1",
              solution: 'print("hi")',
              cases: [{ name: "ex", stdin: "", expected: "hi", isExample: true }],
            },
          ],
        },
      ],
    });
    await runImporter({
      root: tree.root,
      apply: true,
      libraryOnly: true,
      skipSandbox: true,
      allowRename: false,
      targetDb: TARGET_DB,
    });

    // Library content present
    const unitRows = await testDb.select().from(teachingUnits);
    const ourUnits = unitRows.filter((r) => tree.unitIds.includes(r.id));
    expect(ourUnits).toHaveLength(1);
    expect(ourUnits[0].topicId).toBeNull();

    const probRows = await testDb.select().from(problems);
    const ourProbs = probRows.filter((p) => tree.problemIds.includes(p.id));
    expect(ourProbs).toHaveLength(1);

    // Course / topics absent
    const courseRows = await testDb.select().from(courses).where(eq(courses.id, tree.courseId));
    expect(courseRows).toHaveLength(0);
  });

  it("rejects a slug rename without --allow-rename", async () => {
    const tree = await writeTree({
      units: [
        {
          slug: "u1",
          problems: [
            {
              slug: "p1",
              solution: 'print("hi")',
              cases: [{ name: "ex", stdin: "", expected: "hi", isExample: true }],
            },
          ],
        },
      ],
    });
    await runImporter({
      root: tree.root,
      apply: true,
      libraryOnly: false,
      skipSandbox: true,
      allowRename: false,
      targetDb: TARGET_DB,
    });

    // Rename the problem slug in the YAML, keep the id the same. To
    // simulate this without an extra writeTree call, mutate the
    // problem row's slug AS IF the YAML id matched but the DB has a
    // stale slug.
    await testDb
      .update(problems)
      .set({ slug: "p1-old-slug" })
      .where(eq(problems.id, tree.problemIds[0]));

    await expect(
      runImporter({
        root: tree.root,
        apply: true,
        libraryOnly: false,
        skipSandbox: true,
        allowRename: false,
        targetDb: TARGET_DB,
      }),
    ).rejects.toThrow(/--allow-rename/);

    // --allow-rename succeeds.
    await runImporter({
      root: tree.root,
      apply: true,
      libraryOnly: false,
      skipSandbox: true,
      allowRename: true,
      targetDb: TARGET_DB,
    });
    const after = await testDb
      .select({ slug: problems.slug })
      .from(problems)
      .where(eq(problems.id, tree.problemIds[0]));
    expect(after[0].slug).toBe("p1");
  });

  it("rolls back the transaction when post-insert verification fails", async () => {
    const tree = await writeTree({
      units: [
        {
          slug: "u1",
          problems: [
            {
              slug: "p1",
              solution: 'print("hi")',
              cases: [{ name: "ex", stdin: "", expected: "hi", isExample: true }],
            },
          ],
        },
      ],
    });
    // Pre-claim the topic_id with a different unit so Pass 3 fails.
    const conflictUnitId = uuidv4();
    await testDb.insert(teachingUnits).values({
      id: conflictUnitId,
      scope: "platform",
      title: "Conflict Unit",
      slug: "conflict-unit",
      summary: "",
      gradeLevel: "9-12",
      subjectTags: [],
      standardsTags: [],
      materialType: "notes",
      status: "draft",
      createdBy: BRIDGE_HQ_SYSTEM_USER_ID,
    });
    // Insert a course + topic that the conflict unit will claim.
    const conflictCourseId = uuidv4();
    await testDb.insert(courses).values({
      id: conflictCourseId,
      orgId: BRIDGE_HQ_ORG_ID,
      createdBy: BRIDGE_HQ_SYSTEM_USER_ID,
      title: "Conflict Course",
      description: "",
      gradeLevel: "9-12",
      language: "python",
      isPublished: false,
    });
    await testDb.insert(topics).values({
      id: tree.topicIds[0], // SAME topic id the importer wants
      courseId: conflictCourseId,
      title: "Existing Topic",
      description: "",
      sortOrder: 0,
    });
    await testDb
      .update(teachingUnits)
      .set({ topicId: tree.topicIds[0] })
      .where(eq(teachingUnits.id, conflictUnitId));

    await expect(
      runImporter({
        root: tree.root,
        apply: true,
        libraryOnly: false,
        skipSandbox: true,
        allowRename: false,
        targetDb: TARGET_DB,
      }),
    ).rejects.toThrow(/already linked|already claimed/);

    // The course we tried to import must NOT exist (rolled back).
    const ours = await testDb
      .select()
      .from(courses)
      .where(eq(courses.id, tree.courseId));
    expect(ours).toHaveLength(0);
  });
});
