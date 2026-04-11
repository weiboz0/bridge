import { describe, it, expect } from "vitest";
import { testDb, createTestUser } from "../helpers";
import { listUsers, countUsers, getUserByEmail } from "@/lib/users";

describe("user operations", () => {
  it("lists all users", async () => {
    await createTestUser({ email: "a@test.edu" });
    await createTestUser({ email: "b@test.edu" });
    const users = await listUsers(testDb);
    expect(users.length).toBeGreaterThanOrEqual(2);
  });

  it("counts users", async () => {
    await createTestUser({ email: "count@test.edu" });
    const count = await countUsers(testDb);
    expect(count).toBeGreaterThanOrEqual(1);
  });

  it("gets user by email", async () => {
    await createTestUser({ name: "FindMe", email: "findme@test.edu" });
    const user = await getUserByEmail(testDb, "findme@test.edu");
    expect(user).not.toBeNull();
    expect(user!.name).toBe("FindMe");
  });

  it("returns null for non-existent email", async () => {
    const user = await getUserByEmail(testDb, "nope@test.edu");
    expect(user).toBeNull();
  });
});
