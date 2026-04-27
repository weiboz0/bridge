import { describe, it, expect } from "vitest";
import { createTestUser } from "../helpers";
import { createRequest, parseResponse } from "../api-helpers";
import { POST } from "@/app/api/auth/register/route";

describe("POST /api/auth/register", () => {
  it("registers a new user (no role)", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Alice User",
        email: "alice@school.edu",
        password: "password123",
      },
    });

    const { status, body } = await parseResponse(await POST(req));

    expect(status).toBe(201);
    expect(body).toHaveProperty("id");
    expect(body).toHaveProperty("name", "Alice User");
    expect(body).toHaveProperty("email", "alice@school.edu");
    expect(body).not.toHaveProperty("role");
    expect(body).not.toHaveProperty("passwordHash");
  });

  it("rejects duplicate email", async () => {
    await createTestUser({ email: "taken@school.edu" });

    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Duplicate",
        email: "taken@school.edu",
        password: "password123",
      },
    });

    const { status, body } = await parseResponse(await POST(req));
    expect(status).toBe(409);
    expect(body).toHaveProperty("error", "Email already registered");
  });

  it("rejects invalid email format", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Bad Email",
        email: "not-an-email",
        password: "password123",
      },
    });

    const { status } = await parseResponse(await POST(req));
    expect(status).toBe(400);
  });

  it("rejects short password", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Short Pass",
        email: "short@school.edu",
        password: "abc",
      },
    });

    const { status } = await parseResponse(await POST(req));
    expect(status).toBe(400);
  });

  it("rejects missing fields", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: { name: "No Email" },
    });

    const { status } = await parseResponse(await POST(req));
    expect(status).toBe(400);
  });

  it("persists role: 'teacher' as intendedRole", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Teacher Tina",
        email: "tina@school.edu",
        password: "password123",
        role: "teacher",
      },
    });

    const { status, body } = await parseResponse(await POST(req));
    expect(status).toBe(201);
    expect(body).toHaveProperty("intendedRole", "teacher");
  });

  it("persists role: 'student' as intendedRole", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Student Sam",
        email: "sam@school.edu",
        password: "password123",
        role: "student",
      },
    });

    const { status, body } = await parseResponse(await POST(req));
    expect(status).toBe(201);
    expect(body).toHaveProperty("intendedRole", "student");
  });

  it("treats absent role as null intendedRole (BC for OAuth signups)", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Oauth Olivia",
        email: "olivia@school.edu",
        password: "password123",
      },
    });

    const { status, body } = await parseResponse(await POST(req));
    expect(status).toBe(201);
    expect(body.intendedRole).toBeNull();
  });

  it("rejects unknown role values", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Unknown",
        email: "unknown@school.edu",
        password: "password123",
        role: "admin",
      },
    });

    const { status } = await parseResponse(await POST(req));
    expect(status).toBe(400);
  });
});
