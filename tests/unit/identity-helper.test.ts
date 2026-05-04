import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The identity helper is React.cache-wrapped. We need to reset the
// module between tests so the cache doesn't bleed across cases.
// `vi.resetModules` plus a dynamic import is the canonical pattern.

describe("getIdentity", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns the identity payload on a 200 from /api/me/identity", async () => {
    vi.doMock("@/lib/api-client", () => ({
      api: vi.fn(async () => ({
        userId: "user-1",
        email: "u@example.com",
        name: "User",
        isPlatformAdmin: true,
        impersonatedBy: "",
      })),
      ApiError: class ApiError extends Error {
        constructor(
          public status: number,
          message: string,
          public body?: unknown
        ) {
          super(message);
        }
      },
    }));

    const { getIdentity } = await import("@/lib/identity");
    const result = await getIdentity();
    expect(result).toEqual({
      userId: "user-1",
      email: "u@example.com",
      name: "User",
      isPlatformAdmin: true,
      impersonatedBy: "",
    });
  });

  it("returns null on a 401 (unauthenticated)", async () => {
    class ApiError extends Error {
      constructor(
        public status: number,
        message: string,
        public body?: unknown
      ) {
        super(message);
      }
    }
    vi.doMock("@/lib/api-client", () => ({
      api: vi.fn(async () => {
        throw new ApiError(401, "Unauthorized");
      }),
      ApiError,
    }));

    const { getIdentity } = await import("@/lib/identity");
    expect(await getIdentity()).toBeNull();
  });

  it("returns null on a 500 (Go down) and logs a warning", async () => {
    class ApiError extends Error {
      constructor(
        public status: number,
        message: string,
        public body?: unknown
      ) {
        super(message);
      }
    }
    vi.doMock("@/lib/api-client", () => ({
      api: vi.fn(async () => {
        throw new ApiError(500, "Internal Server Error", { error: "boom" });
      }),
      ApiError,
    }));
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

    const { getIdentity } = await import("@/lib/identity");
    expect(await getIdentity()).toBeNull();
    expect(warnSpy).toHaveBeenCalled();
  });

  it("returns null on a network error (non-ApiError throw)", async () => {
    vi.doMock("@/lib/api-client", () => ({
      api: vi.fn(async () => {
        throw new Error("ECONNREFUSED");
      }),
      ApiError: class ApiError extends Error {
        constructor(
          public status: number,
          message: string,
          public body?: unknown
        ) {
          super(message);
        }
      },
    }));
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

    const { getIdentity } = await import("@/lib/identity");
    expect(await getIdentity()).toBeNull();
    expect(warnSpy).toHaveBeenCalled();
  });
});

describe("requireAdmin", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.clearAllMocks();
  });

  it("returns the identity when isPlatformAdmin=true", async () => {
    vi.doMock("@/lib/api-client", () => ({
      api: vi.fn(async () => ({
        userId: "admin-1",
        email: "a@example.com",
        name: "Admin",
        isPlatformAdmin: true,
        impersonatedBy: "",
      })),
      ApiError: class ApiError extends Error {
        constructor(
          public status: number,
          message: string,
          public body?: unknown
        ) {
          super(message);
        }
      },
    }));

    const { requireAdmin } = await import("@/lib/identity");
    const result = await requireAdmin();
    expect(result?.userId).toBe("admin-1");
    expect(result?.isPlatformAdmin).toBe(true);
  });

  it("returns null when isPlatformAdmin=false", async () => {
    vi.doMock("@/lib/api-client", () => ({
      api: vi.fn(async () => ({
        userId: "user-1",
        email: "u@example.com",
        name: "User",
        isPlatformAdmin: false,
        impersonatedBy: "",
      })),
      ApiError: class ApiError extends Error {
        constructor(
          public status: number,
          message: string,
          public body?: unknown
        ) {
          super(message);
        }
      },
    }));

    const { requireAdmin } = await import("@/lib/identity");
    expect(await requireAdmin()).toBeNull();
  });

  it("returns null when the user is unauthenticated (getIdentity null)", async () => {
    class ApiError extends Error {
      constructor(
        public status: number,
        message: string,
        public body?: unknown
      ) {
        super(message);
      }
    }
    vi.doMock("@/lib/api-client", () => ({
      api: vi.fn(async () => {
        throw new ApiError(401, "Unauthorized");
      }),
      ApiError,
    }));

    const { requireAdmin } = await import("@/lib/identity");
    expect(await requireAdmin()).toBeNull();
  });

  it("returns null when userId is empty (defense in depth)", async () => {
    vi.doMock("@/lib/api-client", () => ({
      api: vi.fn(async () => ({
        userId: "",
        email: "?",
        name: "?",
        isPlatformAdmin: true,
        impersonatedBy: "",
      })),
      ApiError: class ApiError extends Error {
        constructor(
          public status: number,
          message: string,
          public body?: unknown
        ) {
          super(message);
        }
      },
    }));

    const { requireAdmin } = await import("@/lib/identity");
    expect(await requireAdmin()).toBeNull();
  });
});
