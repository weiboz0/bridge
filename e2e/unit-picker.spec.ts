import { test, expect, type APIRequestContext } from "@playwright/test";

/**
 * Plan 045 — UnitPickerDialog browser flow.
 *
 * Bootstraps a fresh course + topic + a couple of units via the API
 * (using the saved teacher auth state), then drives the UI: opens
 * the picker, searches, picks, replaces, unlinks. Each test cleans
 * up its own course/topic/units.
 */

test.describe("Unit picker dialog", () => {
  test.use({ storageState: "e2e/.auth/teacher.json" });

  // Helpers --------------------------------------------------------------

  async function fetchTeacherOrg(api: APIRequestContext): Promise<string> {
    const res = await api.get("/api/teacher/courses");
    expect(res.ok(), "GET /api/teacher/courses must succeed").toBeTruthy();
    const body = (await res.json()) as {
      teacherOrgs: Array<{ orgId: string; orgName: string }>;
    };
    const org = body.teacherOrgs?.[0];
    if (!org) throw new Error("teacher has no org; check demo seed");
    return org.orgId;
  }

  async function createCourse(
    api: APIRequestContext,
    orgId: string
  ): Promise<{ id: string }> {
    const res = await api.post("/api/courses", {
      data: {
        orgId,
        title: `Picker E2E ${Date.now()}`,
        gradeLevel: "K-5",
        language: "python",
      },
    });
    expect(res.status()).toBe(201);
    return (await res.json()) as { id: string };
  }

  async function createTopic(
    api: APIRequestContext,
    courseId: string,
    title = "Topic 1"
  ): Promise<{ id: string }> {
    const res = await api.post(`/api/courses/${courseId}/topics`, {
      data: { title },
    });
    expect(res.status()).toBe(201);
    return (await res.json()) as { id: string };
  }

  async function createOrgUnit(
    api: APIRequestContext,
    orgId: string,
    title: string
  ): Promise<{ id: string }> {
    const res = await api.post("/api/units", {
      data: {
        scope: "org",
        scopeId: orgId,
        title,
        materialType: "notes",
        status: "draft",
      },
    });
    expect(res.status()).toBe(201);
    return (await res.json()) as { id: string };
  }

  // -------------------------------------------------------------------- //

  test("teacher picks a unit, replaces it, then unlinks", async ({ page, request }) => {
    const orgId = await fetchTeacherOrg(request);
    const course = await createCourse(request, orgId);
    const topic = await createTopic(request, course.id);
    const firstUnit = await createOrgUnit(request, orgId, `Picker First ${Date.now()}`);
    const secondUnit = await createOrgUnit(request, orgId, `Picker Second ${Date.now()}`);

    await page.goto(`/teacher/courses/${course.id}/topics/${topic.id}`);
    await expect(page.getByRole("heading", { name: "Edit Topic" })).toBeVisible();

    // Open picker.
    await page.getByRole("button", { name: /pick a unit/i }).click();
    const dialog = page.getByRole("dialog", { name: /pick a teaching unit/i });
    await expect(dialog).toBeVisible();

    // Type the first unit's prefix; the picker debounces and re-fetches.
    await dialog.getByLabel("Search").fill("Picker First");
    await expect(dialog.getByText(firstUnit.id ? "Picker First" : "Picker", { exact: false })).toBeVisible({ timeout: 5000 });

    // Pick the first unit.
    await dialog
      .getByRole("listitem")
      .filter({ hasText: "Picker First" })
      .getByRole("button", { name: /^pick$/i })
      .click();

    // Dialog closes; the linked Unit appears in the topic editor's card.
    await expect(dialog).toBeHidden();
    await expect(page.getByText("Picker First", { exact: false })).toBeVisible();
    await expect(page.getByRole("button", { name: /replace/i })).toBeVisible();

    // Replace with the second Unit.
    await page.getByRole("button", { name: /replace/i }).click();
    await expect(dialog).toBeVisible();
    await dialog.getByLabel("Search").fill("Picker Second");
    await dialog
      .getByRole("listitem")
      .filter({ hasText: "Picker Second" })
      .getByRole("button", { name: /^pick$/i })
      .click({ timeout: 7000 });

    await expect(dialog).toBeHidden();
    await expect(page.getByText("Picker Second", { exact: false })).toBeVisible();
    await expect(page.getByText("Picker First", { exact: false })).toBeHidden();

    // Unlink.
    await page.getByRole("button", { name: /unlink/i }).click();
    await expect(page.getByRole("button", { name: /pick a unit/i })).toBeVisible();
    await expect(page.getByText("Picker Second", { exact: false })).toBeHidden();

    // Cleanup the world we created (best effort).
    await request.delete(`/api/courses/${course.id}/topics/${topic.id}`);
    await request.delete(`/api/courses/${course.id}`);
    await request.delete(`/api/units/${firstUnit.id}`);
    await request.delete(`/api/units/${secondUnit.id}`);
  });

  test("picker shows already-linked badge for a unit linked elsewhere", async ({
    page,
    request,
  }) => {
    const orgId = await fetchTeacherOrg(request);
    const course = await createCourse(request, orgId);
    const topicA = await createTopic(request, course.id, "Topic A");
    const topicB = await createTopic(request, course.id, "Topic B");
    const sharedUnit = await createOrgUnit(request, orgId, `Picker Shared ${Date.now()}`);

    // Link the unit to Topic A first via the API.
    const linkRes = await request.post(
      `/api/courses/${course.id}/topics/${topicA.id}/link-unit`,
      { data: { unitId: sharedUnit.id } }
    );
    expect(linkRes.ok()).toBeTruthy();

    // Open Topic B's editor and the picker.
    await page.goto(`/teacher/courses/${course.id}/topics/${topicB.id}`);
    await page.getByRole("button", { name: /pick a unit/i }).click();
    const dialog = page.getByRole("dialog", { name: /pick a teaching unit/i });

    await dialog.getByLabel("Search").fill("Picker Shared");

    const row = dialog
      .getByRole("listitem")
      .filter({ hasText: "Picker Shared" });
    await expect(row).toBeVisible({ timeout: 5000 });
    // Should show "Already linked" badge with Topic A's title.
    await expect(row.getByText(/already linked/i)).toBeVisible();
    await expect(row.getByText("Topic A")).toBeVisible();
    // No Pick button on this row.
    await expect(row.getByRole("button", { name: /^pick$/i })).toBeHidden();

    await dialog.getByRole("button", { name: /cancel/i }).click();
    await expect(dialog).toBeHidden();

    // Cleanup.
    await request.delete(
      `/api/courses/${course.id}/topics/${topicA.id}/link-unit`
    );
    await request.delete(`/api/units/${sharedUnit.id}`);
    await request.delete(`/api/courses/${course.id}/topics/${topicA.id}`);
    await request.delete(`/api/courses/${course.id}/topics/${topicB.id}`);
    await request.delete(`/api/courses/${course.id}`);
  });
});
