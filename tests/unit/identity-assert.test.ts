import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { logIdentityMismatch } from "@/lib/identity-assert";

describe("logIdentityMismatch", () => {
  let consoleSpy: ReturnType<typeof vi.spyOn>;
  const originalEnv = process.env.NODE_ENV;

  beforeEach(() => {
    consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    consoleSpy.mockRestore();
    process.env.NODE_ENV = originalEnv;
  });

  it("logs when IDs differ in development", () => {
    process.env.NODE_ENV = "development";
    logIdentityMismatch("teacher page", "user-A", "user-B");
    expect(consoleSpy).toHaveBeenCalledTimes(1);
    expect(consoleSpy.mock.calls[0][0]).toMatch(/identity-mismatch.*teacher page/);
    expect(consoleSpy.mock.calls[0][0]).toMatch(/user-A/);
    expect(consoleSpy.mock.calls[0][0]).toMatch(/user-B/);
  });

  it("does not log when IDs match", () => {
    process.env.NODE_ENV = "development";
    logIdentityMismatch("ctx", "user-A", "user-A");
    expect(consoleSpy).not.toHaveBeenCalled();
  });

  it("does not log in production even on mismatch", () => {
    process.env.NODE_ENV = "production";
    logIdentityMismatch("ctx", "user-A", "user-B");
    expect(consoleSpy).not.toHaveBeenCalled();
  });

  it("includes extra context when provided", () => {
    process.env.NODE_ENV = "development";
    logIdentityMismatch("ctx", "user-A", "user-B", { sessionId: "session-1" });
    expect(consoleSpy.mock.calls[0][0]).toMatch(/session-1/);
  });

  it("treats both null as match (no log)", () => {
    process.env.NODE_ENV = "development";
    logIdentityMismatch("ctx", null, null);
    expect(consoleSpy).not.toHaveBeenCalled();
  });
});
