import { afterAll, beforeEach, describe, expect, it } from "vitest";
import { eq } from "drizzle-orm";
import { refreshJwtFromDb } from "@/lib/auth-jwt-callback";
import { users } from "@/lib/db/schema";
import { cleanupDatabase, createTestUser, testDb } from "../helpers";

// Plan 050 phase 1: `refreshJwtFromDb` reconciles `id` and
// `isPlatformAdmin` from the DB. The NextAuth `callbacks.jwt` hook
// in `src/lib/auth.ts` calls it on every request. These tests drive
// the helper directly, mutate the `users` row between calls, and
// assert the returned token reflects the new DB state.

describe("refreshJwtFromDb — Plan 050 always-refresh", () => {
  beforeEach(async () => {
    await cleanupDatabase();
  });
  afterAll(async () => {
    await cleanupDatabase();
  });

  it("PROMOTION: non-admin → DB sets is_platform_admin=true → next call returns admin claim", async () => {
    const user = await createTestUser({ isPlatformAdmin: false });

    // Initial token (simulates state right after first sign-in).
    let token = await refreshJwtFromDb({ email: user.email, name: user.name });
    expect(token.id).toBe(user.id);
    expect(token.isPlatformAdmin).toBe(false);

    await testDb
      .update(users)
      .set({ isPlatformAdmin: true })
      .where(eq(users.id, user.id));

    token = await refreshJwtFromDb(token);
    expect(token.id).toBe(user.id);
    expect(token.isPlatformAdmin).toBe(true);
  });

  it("DEMOTION: admin → DB clears is_platform_admin → next call returns non-admin claim", async () => {
    const user = await createTestUser({ isPlatformAdmin: true });
    let token = await refreshJwtFromDb({ email: user.email, name: user.name });
    expect(token.isPlatformAdmin).toBe(true);

    await testDb
      .update(users)
      .set({ isPlatformAdmin: false })
      .where(eq(users.id, user.id));

    token = await refreshJwtFromDb(token);
    expect(token.isPlatformAdmin).toBe(false);
  });

  it("ACCOUNT DELETION: users row removed → callback nulls id and isPlatformAdmin", async () => {
    const user = await createTestUser({ isPlatformAdmin: true });
    let token = await refreshJwtFromDb({ email: user.email, name: user.name });
    expect(token.id).toBe(user.id);

    await testDb.delete(users).where(eq(users.id, user.id));

    token = await refreshJwtFromDb(token);
    expect(token.id).toBeUndefined();
    expect(token.isPlatformAdmin).toBe(false);
  });

  it("NO EMAIL: empty email returns the token unchanged (sanity)", async () => {
    const inputToken = { name: "anon" };
    const result = await refreshJwtFromDb(inputToken);
    expect(result).toBe(inputToken);
  });

  // Edge runtime detection — when EdgeRuntime is a global, we can't
  // open TCP to Postgres, so refreshJwtFromDb skips the lookup and
  // returns the token unchanged. The middleware/Edge path keeps
  // working with whatever was minted at last Node-side refresh.
  it("EDGE RUNTIME: globalThis.EdgeRuntime defined → returns token unchanged (no DB hit)", async () => {
    const user = await createTestUser({ isPlatformAdmin: false });
    // Promote in DB but Edge can't see it.
    await testDb.update(users).set({ isPlatformAdmin: true }).where(eq(users.id, user.id));

    const stale = { email: user.email, id: user.id, isPlatformAdmin: false };

    // Simulate Edge by setting the global the runtime exposes.
    const g = globalThis as { EdgeRuntime?: unknown };
    const orig = g.EdgeRuntime;
    g.EdgeRuntime = "edge-runtime";
    try {
      const token = await refreshJwtFromDb(stale);
      expect(token.isPlatformAdmin).toBe(false);
      expect(token.id).toBe(user.id);
    } finally {
      g.EdgeRuntime = orig;
    }
  });
});
