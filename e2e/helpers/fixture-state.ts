/**
 * Plan 078 phase 1 — Fixture state reader
 *
 * Synchronously reads e2e/.fixture/state.json written by seed.setup.ts.
 * Tests import this instead of re-querying the API on every run.
 *
 * Usage:
 *   import { getFixtureState } from "./helpers/fixture-state";
 *   const { classId, unitId } = getFixtureState();
 */

import * as fs from "node:fs";
import * as path from "node:path";

export interface FixtureState {
  classId: string;
  unitId?: string;
}

const STATE_PATH = path.resolve(__dirname, "../.fixture/state.json");

/**
 * Read the fixture state written by the seed project.
 *
 * Throws a developer-actionable error if the state file is missing.
 */
export function getFixtureState(): FixtureState {
  if (!fs.existsSync(STATE_PATH)) {
    throw new Error(
      `e2e/.fixture/state.json not found — run the e2e seed project first ` +
        `('bun run test:e2e --project=seed' or just 'bun run test:e2e' which ` +
        `auto-runs seed via Playwright project dependencies). ` +
        `The seed project has a hard dependency: HOCUSPOCUS_TOKEN_SECRET must be ` +
        `set in the runner's environment.`,
    );
  }

  const raw = fs.readFileSync(STATE_PATH, "utf-8");
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch (err) {
    throw new Error(
      `e2e/.fixture/state.json exists but could not be parsed as JSON: ${err}. ` +
        `Delete the file and re-run the seed project.`,
    );
  }

  const state = parsed as Record<string, unknown>;
  if (typeof state.classId !== "string" || !state.classId) {
    throw new Error(
      `e2e/.fixture/state.json is missing a valid 'classId' field. ` +
        `Delete the file and re-run the seed project.`,
    );
  }

  return {
    classId: state.classId,
    ...(typeof state.unitId === "string" && state.unitId ? { unitId: state.unitId } : {}),
  };
}
