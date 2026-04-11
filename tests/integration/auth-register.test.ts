import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser } from "../helpers";
import { createRequest, parseResponse } from "../api-helpers";
import { POST } from "@/app/api/auth/register/route";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

describe("POST /api/auth/register", () => {
  it("registers a new teacher", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Alice Teacher",
        email: "alice@school.edu",
        password: "password123",
        role: "teacher",
      },
    });

    const { status, body } = await parseResponse(await POST(req));

    expect(status).toBe(201);
    expect(body).toHaveProperty("id");
    expect(body).toHaveProperty("name", "Alice Teacher");
    expect(body).toHaveProperty("email", "alice@school.edu");
    expect(body).toHaveProperty("role", "teacher");
    // Should not return passwordHash
    expect(body).not.toHaveProperty("passwordHash");
  });

  it("registers a new student", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Bob Student",
        email: "bob@school.edu",
        password: "password123",
        role: "student",
      },
    });

    const { status, body } = await parseResponse(await POST(req));

    expect(status).toBe(201);
    expect(body).toHaveProperty("role", "student");
  });

  it("rejects duplicate email", async () => {
    await createTestUser({ email: "taken@school.edu" });

    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Duplicate",
        email: "taken@school.edu",
        password: "password123",
        role: "teacher",
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
        role: "teacher",
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
        role: "teacher",
      },
    });

    const { status } = await parseResponse(await POST(req));
    expect(status).toBe(400);
  });

  it("rejects invalid role", async () => {
    const req = createRequest("/api/auth/register", {
      method: "POST",
      body: {
        name: "Bad Role",
        email: "badrole@school.edu",
        password: "password123",
        role: "admin",
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
});
