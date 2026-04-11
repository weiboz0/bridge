import { describe, it, expect } from "vitest";
import { testDb, createTestUser, createTestOrg } from "../helpers";
import { users, organizations } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

describe("database connection", () => {
  it("can insert and query an organization", async () => {
    const org = await createTestOrg({ name: "Bridge Academy" });

    expect(org.id).toBeDefined();
    expect(org.name).toBe("Bridge Academy");

    const results = await testDb
      .select()
      .from(organizations)
      .where(eq(organizations.id, org.id));

    expect(results).toHaveLength(1);
    expect(results[0].name).toBe("Bridge Academy");
  });

  it("can insert and query a user", async () => {
    const user = await createTestUser({
      name: "Alice",
      email: "alice@school.edu",
    });

    expect(user.id).toBeDefined();
    expect(user.isPlatformAdmin).toBe(false);

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
