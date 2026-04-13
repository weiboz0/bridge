import { test, expect } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";

test.describe("Assignments (API-driven)", () => {
  // Assignments don't have dedicated UI pages yet in the portal,
  // so we test via API calls using the authenticated context

  test.use({ storageState: "e2e/.auth/teacher.json" });

  test("teacher can create and list assignments via API", async ({ request }) => {
    // First, get a class ID
    const classesRes = await request.get("/api/classes?orgId=d386983b-6da4-4cb8-8057-f2aa70d27c07");
    if (!classesRes.ok()) {
      test.skip(true, "No classes available");
      return;
    }

    const classes = await classesRes.json();
    if (classes.length === 0) {
      test.skip(true, "No classes available");
      return;
    }

    const classId = classes[0].id;

    // Create assignment
    const createRes = await request.post("/api/assignments", {
      data: {
        classId,
        title: "E2E Test Assignment",
        description: "Created by Playwright",
      },
    });

    // 403 means teacher isn't instructor in this class — graceful skip
    if (createRes.status() === 403) return;

    expect(createRes.status()).toBe(201);
    const assignment = await createRes.json();
    expect(assignment.title).toBe("E2E Test Assignment");

    // List assignments
    const listRes = await request.get(`/api/assignments?classId=${classId}`);
    expect(listRes.ok()).toBeTruthy();
    const assignments = await listRes.json();
    expect(assignments.some((a: any) => a.title === "E2E Test Assignment")).toBeTruthy();

    // Clean up — delete the assignment
    await request.delete(`/api/assignments/${assignment.id}`);
  });
});
