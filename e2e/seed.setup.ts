/**
 * Plan 078 phase 1 — Deterministic e2e fixture seed
 *
 * Runs before auth.setup.ts (enforced via Playwright project dependency
 * chain: tests → auth-setup → seed).
 *
 * Contract:
 *  1. Assert HOCUSPOCUS_TOKEN_SECRET is set — fail loudly if missing.
 *  2. Log in as eve@demo.edu (hard-fail if users not seeded).
 *  3. Idempotently ensure a fixture class exists (title starts with "e2e-fixture").
 *     - If found: reuse classId.
 *     - If not: create via POST /api/classes (uses demo seed courseId + orgId).
 *  4. Idempotently ensure alice@demo.edu is enrolled as student.
 *     - If not enrolled: get join code, log in as alice, join via API, log back in as eve.
 *  5. Active-session cleanup: end any live session on the fixture class.
 *  6. Idempotently ensure a fixture unit exists (title starts with "e2e-fixture").
 *     - SKIP (log TODO) if not found — unit creation via UI is complex and
 *       not blocking the WS auth tests.
 *  7. Write e2e/.fixture/state.json: { classId, unitId? }.
 */

import * as fs from "node:fs";
import * as path from "node:path";
import { test as setup, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials, logout } from "./helpers";

// Fixed demo-seed identifiers. Eve's org and the Python-101 course are
// created by scripts/seed_problem_demo.sql and are stable across re-runs.
const DEMO_ORG_ID = "d386983b-6da4-4cb8-8057-f2aa70d27c07";
const DEMO_COURSE_ID = "00000000-0000-0000-0000-0000000aa001";

const FIXTURE_CLASS_PREFIX = "e2e-fixture";
const FIXTURE_UNIT_PREFIX = "e2e-fixture";
const FIXTURE_CLASS_TITLE = "e2e-fixture-class";

const FIXTURE_DIR = path.resolve(__dirname, ".fixture");
const FIXTURE_STATE_PATH = path.join(FIXTURE_DIR, "state.json");

interface ClassItem {
  id: string;
  title: string;
  status: string;
  memberRole: string;
}

interface ClassMember {
  id: string;
  userId: string;
  role: string;
  name: string;
  email: string;
}

interface ClassDetail {
  id: string;
  title: string;
  joinCode: string;
}

interface ActiveSession {
  id: string;
  status: string;
}

interface UnitItem {
  id: string;
  title: string;
}

setup("seed fixture data", async ({ page }) => {
  // ── Step 1: Assert HOCUSPOCUS_TOKEN_SECRET ─────────────────────────────────
  expect(
    process.env.HOCUSPOCUS_TOKEN_SECRET,
    "HOCUSPOCUS_TOKEN_SECRET is not set in the runner environment. " +
      "The e2e suite cannot run without it — set it before running 'bun run test:e2e'.",
  ).toBeTruthy();

  // ── Step 2: Log in as eve@demo.edu ─────────────────────────────────────────
  try {
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
  } catch {
    throw new Error(
      "e2e seed failed: demo.edu users not found. " +
        "Run scripts/seed_problem_demo.sql or " +
        "'bun run content:python-101:import --apply --wire-demo-class' from docs/setup.md.",
    );
  }

  // ── Step 3: Idempotently ensure fixture class exists ───────────────────────
  let classId: string | undefined;

  const classesRes = await page.request.get("/api/classes/mine");
  if (classesRes.ok()) {
    const classes = (await classesRes.json()) as ClassItem[];
    const existing = classes.find((c) => c.title.startsWith(FIXTURE_CLASS_PREFIX));
    if (existing) {
      classId = existing.id;
      console.log(`[seed] reusing existing fixture class: ${classId} (${existing.title})`);
    }
  }

  if (!classId) {
    console.log(`[seed] creating fixture class: ${FIXTURE_CLASS_TITLE}`);
    const createRes = await page.request.post("/api/classes", {
      data: {
        courseId: DEMO_COURSE_ID,
        orgId: DEMO_ORG_ID,
        title: FIXTURE_CLASS_TITLE,
        term: "e2e",
      },
      headers: { "Content-Type": "application/json" },
    });
    if (!createRes.ok()) {
      const body = await createRes.text();
      throw new Error(
        `e2e seed failed: could not create fixture class. Status ${createRes.status()}: ${body}`,
      );
    }
    const created = (await createRes.json()) as ClassDetail;
    classId = created.id;
    console.log(`[seed] fixture class created: ${classId}`);
  }

  // ── Step 4: Idempotently ensure alice is enrolled ──────────────────────────
  const membersRes = await page.request.get(`/api/classes/${classId}/members`);
  let aliceEnrolled = false;
  let joinCode: string | undefined;

  if (membersRes.ok()) {
    const members = (await membersRes.json()) as ClassMember[];
    aliceEnrolled = members.some(
      (m) => m.email === ACCOUNTS.student.email && m.role === "student",
    );
  }

  if (!aliceEnrolled) {
    // Get the join code from class detail
    const detailRes = await page.request.get(`/api/classes/${classId}`);
    if (!detailRes.ok()) {
      throw new Error(
        `e2e seed failed: could not fetch class detail for ${classId} (status ${detailRes.status()})`,
      );
    }
    const detail = (await detailRes.json()) as ClassDetail;
    joinCode = detail.joinCode;
    console.log(`[seed] alice not enrolled — joining with code ${joinCode}`);

    // Log out as eve, log in as alice, join the class, log back in as eve
    await logout(page);
    try {
      await loginWithCredentials(page, ACCOUNTS.student.email, ACCOUNTS.student.password);
    } catch {
      throw new Error(
        "e2e seed failed: could not log in as alice@demo.edu. " +
          "Run scripts/seed_problem_demo.sql or " +
          "'bun run content:python-101:import --apply --wire-demo-class' from docs/setup.md.",
      );
    }

    const joinRes = await page.request.post("/api/classes/join", {
      data: { joinCode },
      headers: { "Content-Type": "application/json" },
    });
    if (!joinRes.ok()) {
      const body = await joinRes.text();
      // 409 / duplicate means alice is already enrolled — treat as success
      if (joinRes.status() !== 409) {
        console.warn(`[seed] WARNING: alice join returned ${joinRes.status()}: ${body}`);
      }
    } else {
      console.log("[seed] alice enrolled successfully");
    }

    // Log back in as eve
    await logout(page);
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
  } else {
    console.log("[seed] alice already enrolled in fixture class");
  }

  // ── Step 5: Active-session cleanup ─────────────────────────────────────────
  try {
    const activeRes = await page.request.get(`/api/sessions/active/${classId}`);
    if (activeRes.ok()) {
      const activeSession = (await activeRes.json()) as ActiveSession | null;
      if (activeSession?.id) {
        console.log(`[seed] ending active session ${activeSession.id}`);
        const endRes = await page.request.post(`/api/sessions/${activeSession.id}/end`);
        if (!endRes.ok()) {
          const body = await endRes.text();
          console.warn(
            `[seed] WARNING: could not end active session ${activeSession.id} — ` +
              `status ${endRes.status()}: ${body}. Proceeding anyway.`,
          );
        } else {
          console.log(`[seed] active session ${activeSession.id} ended`);
        }
      } else {
        console.log("[seed] no active session on fixture class");
      }
    } else {
      console.warn(
        `[seed] WARNING: GET /api/sessions/active/${classId} returned ${activeRes.status()} — ` +
          "skipping session cleanup.",
      );
    }
  } catch (err) {
    console.warn(`[seed] WARNING: active-session cleanup failed: ${err}. Proceeding anyway.`);
  }

  // ── Step 6: Idempotently ensure a fixture unit exists ──────────────────────
  // Query /api/units (which the seed accesses as an authenticated teacher).
  // The hocuspocus-auth tests use the FIRST available unit from /api/me/units,
  // which is the same endpoint backed by /api/units. If no e2e-fixture unit
  // exists, log a TODO and proceed — unit creation requires a published course
  // + topic linkage that is too complex for this phase. Phase 2 skip-to-fail
  // conversion will read unitId from the fixture and assert it is defined.
  let unitId: string | undefined;

  try {
    const unitsRes = await page.request.get("/api/units");
    if (unitsRes.ok()) {
      const body = (await unitsRes.json()) as
        | UnitItem[]
        | { units?: UnitItem[]; items?: UnitItem[] };
      const unitList = Array.isArray(body)
        ? body
        : (body.units ?? body.items ?? []);
      const existing = unitList.find((u) => u.title.startsWith(FIXTURE_UNIT_PREFIX));
      if (existing) {
        unitId = existing.id;
        console.log(`[seed] reusing existing fixture unit: ${unitId} (${existing.title})`);
      } else {
        console.log(
          "[seed] TODO: no e2e-fixture unit found. Unit creation via the API requires a " +
            "published course + topic linkage — deferred to a future phase. " +
            "Phase-2 hocuspocus-auth skip-to-fail conversion will assert unitId is defined " +
            "once unit seeding is added.",
        );
      }
    } else {
      console.warn(
        `[seed] WARNING: GET /api/units returned ${unitsRes.status()} — ` +
          "skipping unit check.",
      );
    }
  } catch (err) {
    console.warn(`[seed] WARNING: unit lookup failed: ${err}. Proceeding.`);
  }

  // ── Step 7: Write state.json ────────────────────────────────────────────────
  if (!fs.existsSync(FIXTURE_DIR)) {
    fs.mkdirSync(FIXTURE_DIR, { recursive: true });
  }
  const state = { classId, ...(unitId ? { unitId } : {}) };
  fs.writeFileSync(FIXTURE_STATE_PATH, JSON.stringify(state, null, 2), "utf-8");
  console.log(`[seed] wrote fixture state to ${FIXTURE_STATE_PATH}:`, state);
});
