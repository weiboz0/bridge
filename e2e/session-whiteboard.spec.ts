import { test, expect, type BrowserContext, type Page } from "@playwright/test";
import { ACCOUNTS, loginWithCredentials } from "./helpers";
import { getFixtureState } from "./helpers/fixture-state";

// This spec intentionally has no webServer. It is allowed to execute only
// against a user-provisioned stack selected by E2E_BASE_URL; the production
// config defaults to a different local service and must never be guessed.
const liveStackURL = process.env.E2E_BASE_URL;
const injectedControlFailure = process.env.BRIDGE_E2E_CANVAS_CONTROL_FAILURE === "1";

async function openWhiteboard(page: Page) {
  await page.getByRole("button", { name: "Whiteboard", exact: true }).last().click();
  await expect(page.getByRole("region", { name: "Whiteboards" })).toBeVisible();
}

async function sceneInk(page: Page): Promise<number> {
  const canvas = page.getByTestId("excalidraw-board").locator("canvas").last();
  await expect(canvas).toBeVisible();
  return canvas.evaluate((node) => {
    const surface = node as HTMLCanvasElement;
    const context = surface.getContext("2d", { willReadFrequently: true });
    if (!context) return 0;
    const { width, height } = surface;
    // Ignore toolbars at the edges. A drawn rectangle is deliberately placed
    // in this center area, so a remote update increases this count.
    const left = Math.floor(width * 0.2);
    const top = Math.floor(height * 0.2);
    const sampleWidth = Math.max(1, Math.floor(width * 0.6));
    const sampleHeight = Math.max(1, Math.floor(height * 0.6));
    const pixels = context.getImageData(left, top, sampleWidth, sampleHeight).data;
    let count = 0;
    for (let i = 0; i < pixels.length; i += 4) {
      if (pixels[i] < 220 || pixels[i + 1] < 220 || pixels[i + 2] < 220) count++;
    }
    return count;
  });
}

test.describe.serial("session whiteboard live stack", () => {
  test.skip(!liveStackURL, "requires a separately started Bridge stack and explicit E2E_BASE_URL");

  let teacherContext: BrowserContext;
  let participantContext: BrowserContext;
  let outsiderContext: BrowserContext;
  let teacher: Page;
  let participant: Page;
  let outsider: Page;
  let classId: string;
  let sessionId: string;
  let boardTitle: string;

  test.beforeAll(async ({ browser }) => {
    ({ classId } = getFixtureState());
    boardTitle = `E2E whiteboard ${Date.now()}`;
    teacherContext = await browser.newContext();
    participantContext = await browser.newContext();
    outsiderContext = await browser.newContext();
    teacher = await teacherContext.newPage();
    participant = await participantContext.newPage();
    outsider = await outsiderContext.newPage();
    await loginWithCredentials(teacher, ACCOUNTS.teacher.email, ACCOUNTS.teacher.password);
    await loginWithCredentials(participant, ACCOUNTS.student.email, ACCOUNTS.student.password);
    await loginWithCredentials(outsider, ACCOUNTS.student2.email, ACCOUNTS.student2.password);
  });

  test.afterAll(async () => {
    await teacherContext?.close();
    await participantContext?.close();
    await outsiderContext?.close();
  });

  test("teacher creates a board, confirms a visibility raise, and raises the floor", async () => {
    await teacher.goto(`/teacher/classes/${classId}`);
    await teacher.getByRole("button", { name: "Start Live Session" }).click();
    await teacher.waitForURL(/\/teacher\/sessions\/[0-9a-f-]{36}/);
    const match = teacher.url().match(/\/teacher\/sessions\/([0-9a-f-]{36})/);
    expect(match?.[1]).toBeTruthy();
    sessionId = match![1];

    // The public-listing toggle makes the outsider case a real public-session
    // attempt; canvas creation still requires represented teacher/presence.
    await teacher.getByTestId("visibility-toggle").click();
    await expect(teacher.getByTestId("visibility-toggle")).toContainText(/public/i);

    await openWhiteboard(teacher);
    await teacher.getByLabel("Whiteboard title").fill(boardTitle);
    await teacher.getByRole("button", { name: "New whiteboard" }).click();
    await expect(teacher.getByRole("button", { name: boardTitle })).toBeVisible();

    const visibility = teacher.getByRole("combobox", { name: "Visibility", exact: true });
    await visibility.selectOption("participants");
    await expect(teacher.getByRole("dialog", { name: "Raise whiteboard visibility?" })).toBeVisible();
    await teacher.getByRole("button", { name: "Raise visibility" }).click();
    await expect(visibility).toHaveValue("participants");

    await teacher.getByLabel("Canvas floor", { exact: true }).selectOption("participants");
    await expect(teacher.getByLabel("Canvas floor", { exact: true })).toHaveValue("participants");
  });

  test("participant views the raised board while a public outsider cannot create", async () => {
    await participant.goto(`/student/classes/${classId}`);
    await participant.getByText("Live Session — Join Now").click();
    await participant.waitForURL(`/student/sessions/${sessionId}`);
    await openWhiteboard(participant);
    await participant.getByRole("button", { name: boardTitle }).click();
    await expect(participant.getByText("View only", { exact: true })).toBeVisible();

    // Bob is the public-outsider fixture: authenticated but neither a class
    // member nor a participant. This is deliberately a browser fetch, not a
    // privileged test client, so the actual session authorization decides it.
    const status = await outsider.evaluate(async ({ id, title }) => {
      const response = await fetch(`/api/sessions/${id}/canvases`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ title: `${title} outsider`, visibility: "private" }),
      });
      return response.status;
    }, { id: sessionId, title: boardTitle });
    expect(status).toBe(403);
  });

  test("scene synchronizes from teacher to the participant board", async () => {
    const before = await sceneInk(participant);
    const board = teacher.getByTestId("excalidraw-board");
    await expect(board).toBeVisible();
    const box = await board.boundingBox();
    expect(box).not.toBeNull();
    await teacher.mouse.click(box!.x + box!.width * 0.45, box!.y + box!.height * 0.45);
    await teacher.keyboard.press("r");
    await teacher.mouse.move(box!.x + box!.width * 0.40, box!.y + box!.height * 0.40);
    await teacher.mouse.down();
    await teacher.mouse.move(box!.x + box!.width * 0.60, box!.y + box!.height * 0.58);
    await teacher.mouse.up();

    await expect.poll(() => sceneInk(participant), { timeout: 10_000 }).toBeGreaterThan(before + 30);
  });

  test("explicit end redirects to the archive", async () => {
    await teacher.getByRole("button", { name: "End Session" }).click();
    await teacher.waitForURL(new RegExp(`/sessions/${sessionId}/whiteboards$`));
    await expect(teacher.getByRole("region", { name: "Whiteboard archive" })).toBeVisible();
  });

  test("read-only archive suppresses live controls after explicit end", async () => {
    await teacher.getByRole("button", { name: boardTitle }).click();
    await expect(teacher.getByText("Read-only session archive", { exact: true })).toBeVisible();
    await expect(teacher.getByText("View only", { exact: true })).toBeVisible();
    await expect(teacher.getByRole("button", { name: "New whiteboard" })).toHaveCount(0);
    await expect(teacher.getByLabel("Canvas floor", { exact: true })).toHaveCount(0);
    await expect(teacher.getByRole("combobox", { name: "Visibility", exact: true })).toHaveCount(0);
  });

  test("named E2E control-client failure injection reports an incomplete archive", async () => {
    test.skip(!injectedControlFailure, "requires BRIDGE_E2E_CANVAS_CONTROL_FAILURE=1 with both database proofs accepted at startup");
    await expect(teacher.getByText("Latest whiteboard changes may not have been archived", { exact: true })).toBeVisible();
  });
});
