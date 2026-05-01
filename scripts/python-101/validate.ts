#!/usr/bin/env bun
/**
 * Validates the Python 101 content tree:
 *   content/python-101/course.yaml
 *   content/python-101/units/*.yaml
 *
 * Exit codes:
 *   0 — all files parse, both per-file Zod and cross-file checks pass
 *   1 — one or more files invalid (errors printed to stderr)
 *   2 — invocation error (missing root dir, unreadable file, etc.)
 *
 * Usage:
 *   bun run scripts/python-101/validate.ts [--root content/python-101]
 */

import { readdir, readFile, stat } from "node:fs/promises";
import { join, basename } from "node:path";
import {
  courseManifestSchema,
  parseAuthoringYaml,
  unitFileSchema,
  validateContentTree,
  type ContentTree,
  type CourseManifest,
  type UnitFile,
  type ValidationIssue,
} from "./schema";

interface CliArgs {
  root: string;
}

function parseArgs(argv: string[]): CliArgs {
  let root = "content/python-101";
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--root" && argv[i + 1]) {
      root = argv[++i];
    } else if (a.startsWith("--root=")) {
      root = a.slice("--root=".length);
    } else if (a === "-h" || a === "--help") {
      console.log(
        "Usage: bun run scripts/python-101/validate.ts [--root <dir>]",
      );
      process.exit(0);
    } else {
      console.error(`unknown argument: ${a}`);
      process.exit(2);
    }
  }
  return { root };
}

interface IssueWithFile {
  file: string;
  message: string;
}

function formatZodIssues(file: string, error: unknown): IssueWithFile[] {
  if (
    error &&
    typeof error === "object" &&
    "issues" in error &&
    Array.isArray((error as { issues: unknown[] }).issues)
  ) {
    type ZIssue = { path: (string | number)[]; message: string };
    const issues = (error as { issues: ZIssue[] }).issues;
    return issues.map((i) => ({
      file,
      message: `${i.path.join(".") || "<root>"}: ${i.message}`,
    }));
  }
  return [{ file, message: error instanceof Error ? error.message : String(error) }];
}

async function loadCourseManifest(
  root: string,
): Promise<{ data?: CourseManifest; issues: IssueWithFile[] }> {
  const file = join(root, "course.yaml");
  let raw: string;
  try {
    raw = await readFile(file, "utf8");
  } catch (e) {
    return { issues: [{ file, message: `could not read: ${(e as Error).message}` }] };
  }
  let parsed: unknown;
  try {
    parsed = parseAuthoringYaml(raw);
  } catch (e) {
    return { issues: [{ file, message: (e as Error).message }] };
  }
  const result = courseManifestSchema.safeParse(parsed);
  if (!result.success) {
    return { issues: formatZodIssues(file, result.error) };
  }
  return { data: result.data, issues: [] };
}

async function loadUnitFiles(
  root: string,
): Promise<{ units: Map<string, UnitFile>; issues: IssueWithFile[] }> {
  const unitsDir = join(root, "units");
  const issues: IssueWithFile[] = [];
  const units = new Map<string, UnitFile>();

  let entries: string[];
  try {
    entries = await readdir(unitsDir);
  } catch (e) {
    issues.push({
      file: unitsDir,
      message: `could not list units directory: ${(e as Error).message}`,
    });
    return { units, issues };
  }

  for (const entry of entries) {
    if (!entry.endsWith(".yaml") && !entry.endsWith(".yml")) continue;
    const file = join(unitsDir, entry);
    const stats = await stat(file);
    if (!stats.isFile()) continue;

    let raw: string;
    try {
      raw = await readFile(file, "utf8");
    } catch (e) {
      issues.push({ file, message: `could not read: ${(e as Error).message}` });
      continue;
    }
    let parsed: unknown;
    try {
      parsed = parseAuthoringYaml(raw);
    } catch (e) {
      issues.push({ file, message: (e as Error).message });
      continue;
    }
    const result = unitFileSchema.safeParse(parsed);
    if (!result.success) {
      issues.push(...formatZodIssues(file, result.error));
      continue;
    }
    const expectedSlug = basename(entry, entry.endsWith(".yaml") ? ".yaml" : ".yml");
    if (result.data.slug !== expectedSlug) {
      issues.push({
        file,
        message: `slug "${result.data.slug}" does not match filename "${expectedSlug}"`,
      });
      continue;
    }
    if (units.has(result.data.slug)) {
      issues.push({
        file,
        message: `unit slug "${result.data.slug}" duplicated in another file`,
      });
      continue;
    }
    units.set(result.data.slug, result.data);
  }

  return { units, issues };
}

function reportIssues(issues: IssueWithFile[] | ValidationIssue[]): void {
  for (const issue of issues) {
    const f = (issue as { file: string }).file;
    const m = (issue as { message: string }).message;
    process.stderr.write(`  ${f}: ${m}\n`);
  }
}

async function main(): Promise<void> {
  const args = parseArgs(process.argv.slice(2));
  const allIssues: IssueWithFile[] = [];

  const courseLoad = await loadCourseManifest(args.root);
  allIssues.push(...courseLoad.issues);
  const unitLoad = await loadUnitFiles(args.root);
  allIssues.push(...unitLoad.issues);

  if (!courseLoad.data || allIssues.length > 0) {
    process.stderr.write(`\n${allIssues.length} issue(s) before cross-file checks:\n`);
    reportIssues(allIssues);
    if (!courseLoad.data) {
      process.stderr.write("\ncannot run cross-file checks without a valid course.yaml\n");
      process.exit(1);
    }
  }

  const tree: ContentTree = {
    course: courseLoad.data,
    units: unitLoad.units,
  };
  const crossIssues = validateContentTree(tree);
  if (crossIssues.length > 0) {
    process.stderr.write(`\n${crossIssues.length} cross-file issue(s):\n`);
    reportIssues(crossIssues);
  }

  const total = allIssues.length + crossIssues.length;
  if (total > 0) {
    process.stderr.write(`\nFAIL: ${total} issue(s)\n`);
    process.exit(1);
  }
  process.stdout.write(
    `OK: course "${tree.course.id}", ${tree.units.size} unit(s), ` +
      `${[...tree.units.values()].reduce((n, u) => n + u.problems.length, 0)} problem(s)\n`,
  );
}

main().catch((err) => {
  process.stderr.write(`unexpected error: ${err instanceof Error ? err.stack : String(err)}\n`);
  process.exit(2);
});
