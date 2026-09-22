/**
 * Plan 078 phase 1 — Deterministic e2e fixture seed
 *
 * Runs before auth.setup.ts (enforced via Playwright project dependency
 * chain: tests → auth-setup → seed).
 *
 * Contract:
 *  1. Re-attest the E2E stack (Plan 094 Phase 14) with a FRESH nonce before any
 *     mutating request — fail closed if the gate did not verify it first.
 *  2. Assert HOCUSPOCUS_TOKEN_SECRET is set — fail loudly if missing.
 *  3. Log in as eve@demo.edu (hard-fail if users not seeded).
 *  4. Idempotently ensure a fixture class exists (title starts with "e2e-fixture").
 *     - If found: reuse classId.
 *     - If not: create via POST /api/classes (uses demo seed courseId + orgId).
 *  5. Idempotently ensure alice@demo.edu is enrolled as student.
 *     - If not enrolled: get join code, log in as alice, join via API, log back in as eve.
 *  6. Active-session cleanup: end any live session on the fixture class.
 *  7. Resolve a canonical demo chapter for realtime-token tests.
 *  8. Write e2e/.fixture/state.json: { classId, chapterId }.
 */

import * as fs from "node:fs";
import * as path from "node:path";
import { test as setup, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials, logout } from "./helpers";
import { assertAttestedE2EStack } from "./helpers/e2e-stack";

// Fixed demo-seed identifiers. Eve's org and the Python-101 course are
// created by scripts/seed_problem_demo.sql and are stable across re-runs.
const DEMO_ORG_ID = "d386983b-6da4-4cb8-8057-f2aa70d27c07";
const DEMO_COURSE_ID = "00000000-0000-0000-0000-0000000aa001";

const FIXTURE_CLASS_PREFIX = "e2e-fixture";
const FIXTURE_CLASS_TITLE = "e2e-fixture-class";
const DEMO_CHAPTER_TITLE = "Warm-ups";

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

interface ChapterItem {
  id: string;
  title: string;
}

setup("seed fixture data", async ({ page }) => {
  // ── Step 1: Re-attest the stack before ANY mutation ────────────────────────
  // Must be the first statement: every later step writes to whatever database
  // this stack is connected to. A fresh nonce and the gate's own instance ids
  // are required, so a replayed gate answer or a stack restarted since the gate
  // ran cannot pass. Fails closed when the gate did not run the attestation.
  const attestation = await assertAttestedE2EStack({ env: process.env });
  console.log(
    `[seed] e2e stack attested: ${attestation.baseUrl} (realtime ${attestation.realtimeOrigin ?? "n/a"})`,
  );

  // ── Step 2: Assert HOCUSPOCUS_TOKEN_SECRET ─────────────────────────────────
  expect(
    process.env.HOCUSPOCUS_TOKEN_SECRET,
    "HOCUSPOCUS_TOKEN_SECRET is not set in the runner environment. " +
      "The e2e suite cannot run without it — set it before running 'bun run test:e2e'.",
  ).toBeTruthy();

  // ── Step 3: Log in as eve@demo.edu ─────────────────────────────────────────
  try {
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password, "/teacher");
  } catch {
    throw new Error(
      "e2e seed failed: demo.edu users not found. " +
        "Run scripts/seed_problem_demo.sql or " +
        "'bun run content:python-101:import --apply --wire-demo-class' from docs/setup.md.",
    );
  }

  // ── Step 4: Idempotently ensure fixture class exists ───────────────────────
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

  // ── Step 5: Idempotently ensure alice is enrolled ──────────────────────────
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
      await loginWithCredentials(page, ACCOUNTS.student.email, ACCOUNTS.student.password, "/student");
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
    await loginWithCredentials(page, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password, "/teacher");
  } else {
    console.log("[seed] alice already enrolled in fixture class");
  }

  // ── Step 6: Active-session cleanup ─────────────────────────────────────────
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

  // ── Step 7: Resolve the canonical demo chapter ────────────────────────────
  let chapterId: string | undefined;

  try {
    const chaptersRes = await page.request.get(
      `/api/chapters?scope=org&scopeId=${DEMO_ORG_ID}`,
    );
    if (chaptersRes.ok()) {
      const body = (await chaptersRes.json()) as { items?: ChapterItem[] };
      const existing = body.items?.find((chapter) => chapter.title === DEMO_CHAPTER_TITLE);
      if (existing) {
        chapterId = existing.id;
        console.log(`[seed] using canonical demo chapter: ${chapterId} (${existing.title})`);
      } else {
        throw new Error("e2e seed failed: canonical Warm-ups chapter is missing from the demo seed.");
      }
    } else {
      throw new Error(`e2e seed failed: GET /api/chapters returned ${chaptersRes.status()}.`);
    }
  } catch (err) {
    throw new Error(`e2e seed failed: canonical chapter lookup failed: ${err}`);
  }

  // ── Step 8: Write state.json ────────────────────────────────────────────────
  if (!fs.existsSync(FIXTURE_DIR)) {
    fs.mkdirSync(FIXTURE_DIR, { recursive: true });
  }
  const state = { classId, chapterId };
  fs.writeFileSync(FIXTURE_STATE_PATH, JSON.stringify(state, null, 2), "utf-8");
  console.log(`[seed] wrote fixture state to ${FIXTURE_STATE_PATH}:`, state);
});
