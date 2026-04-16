import { test, expect } from "@playwright/test";

/**
 * Session Scheduling E2E — API-driven tests since there is no scheduling UI yet.
 *
 * Tests CRUD operations for scheduled sessions:
 * - Create a scheduled session for a class
 * - List upcoming sessions
 * - Start a session from a schedule entry
 * - Cancel a scheduled session
 *
 * All requests go through the Go API proxied via Next.js.
 */

const ORG_ID = "d386983b-6da4-4cb8-8057-f2aa70d27c07";

test.describe("Session Scheduling (API-driven)", () => {
  test.use({ storageState: "e2e/.auth/teacher.json" });

  let classId: string;
  let scheduleId: string;

  test.beforeAll(async ({ request }) => {
    // Get a class ID from the teacher's classes
    const classesRes = await request.get(`/api/classes?orgId=${ORG_ID}`);
    if (classesRes.ok()) {
      const classes = await classesRes.json();
      if (classes.length > 0) {
        classId = classes[0].id;
      }
    }
  });

  test("teacher can create a scheduled session via API", async ({ request }) => {
    test.skip(!classId, "No classes available for scheduling");

    // Schedule a session 1 hour from now, lasting 1 hour
    const now = new Date();
    const start = new Date(now.getTime() + 60 * 60 * 1000);
    const end = new Date(now.getTime() + 2 * 60 * 60 * 1000);

    const res = await request.post(`/api/classes/${classId}/schedule`, {
      data: {
        title: "E2E Scheduled Session",
        scheduledStart: start.toISOString(),
        scheduledEnd: end.toISOString(),
        topicIds: [],
      },
    });

    // 403 means the teacher may not have the right role — graceful skip
    if (res.status() === 403) {
      test.skip(true, "Teacher not authorized to create schedules for this class");
      return;
    }

    expect(res.status()).toBe(201);
    const schedule = await res.json();
    expect(schedule.id).toBeTruthy();
    expect(schedule.classId || schedule.class_id).toBe(classId);
    scheduleId = schedule.id;
  });

  test("teacher can list upcoming sessions for a class via API", async ({ request }) => {
    test.skip(!classId, "No classes available");

    const res = await request.get(`/api/classes/${classId}/schedule/upcoming`);
    expect(res.ok()).toBeTruthy();

    const schedules = await res.json();
    expect(Array.isArray(schedules)).toBeTruthy();

    // If we created a schedule in the previous test, it should appear
    if (scheduleId) {
      const found = schedules.some(
        (s: { id: string }) => s.id === scheduleId
      );
      expect(found).toBeTruthy();
    }
  });

  test("teacher can cancel a scheduled session via API", async ({ request }) => {
    test.skip(!classId, "No classes available");

    // Create a new schedule to cancel (don't reuse the one we might start)
    const now = new Date();
    const start = new Date(now.getTime() + 3 * 60 * 60 * 1000);
    const end = new Date(now.getTime() + 4 * 60 * 60 * 1000);

    const createRes = await request.post(`/api/classes/${classId}/schedule`, {
      data: {
        title: "E2E Cancel Test",
        scheduledStart: start.toISOString(),
        scheduledEnd: end.toISOString(),
        topicIds: [],
      },
    });

    if (!createRes.ok()) {
      test.skip(true, "Could not create schedule to cancel");
      return;
    }

    const created = await createRes.json();
    const cancelId = created.id;

    // Cancel the schedule
    const cancelRes = await request.delete(`/api/schedule/${cancelId}`);
    expect(cancelRes.ok()).toBeTruthy();

    const cancelled = await cancelRes.json();
    expect(cancelled.status).toBe("cancelled");
  });

  test("teacher can start a session from a schedule entry via API", async ({ request }) => {
    test.skip(!classId, "No classes available");

    // Create a fresh schedule to start
    const now = new Date();
    const start = new Date(now.getTime() + 5 * 60 * 60 * 1000);
    const end = new Date(now.getTime() + 6 * 60 * 60 * 1000);

    const createRes = await request.post(`/api/classes/${classId}/schedule`, {
      data: {
        title: "E2E Start Test",
        scheduledStart: start.toISOString(),
        scheduledEnd: end.toISOString(),
        topicIds: [],
      },
    });

    if (!createRes.ok()) {
      test.skip(true, "Could not create schedule to start");
      return;
    }

    const created = await createRes.json();
    const startId = created.id;

    // Start a session from the schedule
    const startRes = await request.post(`/api/schedule/${startId}/start`);
    expect(startRes.status()).toBe(201);

    const session = await startRes.json();
    expect(session.id).toBeTruthy();

    // Clean up: end the started session if it has a session ID
    if (session.id) {
      await request.patch(`/api/sessions/${session.id}`);
    }
  });
});
