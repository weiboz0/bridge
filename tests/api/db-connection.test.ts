import { describe, it, expect } from "vitest";
import { testDb, createTestSchool, createTestUser } from "../helpers";
import { users, schools } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

describe("database connection", () => {
  it("can insert and query a school", async () => {
    const school = await createTestSchool({ name: "Bridge Academy" });

    expect(school.id).toBeDefined();
    expect(school.name).toBe("Bridge Academy");

    const results = await testDb
      .select()
      .from(schools)
      .where(eq(schools.id, school.id));

    expect(results).toHaveLength(1);
    expect(results[0].name).toBe("Bridge Academy");
  });

  it("can insert and query a user", async () => {
    const user = await createTestUser({
      name: "Alice",
      email: "alice@school.edu",
      role: "student",
    });

    expect(user.id).toBeDefined();
    expect(user.role).toBe("student");

    const results = await testDb
      .select()
      .from(users)
      .where(eq(users.id, user.id));

    expect(results).toHaveLength(1);
    expect(results[0].name).toBe("Alice");
    expect(results[0].email).toBe("alice@school.edu");
  });

  it("enforces unique email constraint", async () => {
    await createTestUser({ email: "duplicate@school.edu" });

    await expect(
      createTestUser({ email: "duplicate@school.edu" })
    ).rejects.toThrow();
  });
});
