import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/lib/api-client", () => ({
  api: vi.fn(),
  ApiError: class ApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
      this.name = "ApiError";
    }
  },
}));

import {
  appendOrgId,
  resolveOrgContext,
} from "@/lib/portal/org-context";
import { api, ApiError } from "@/lib/api-client";

const VALID_UUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479";
const OTHER_UUID = "12345678-1234-1234-1234-123456789012";

const mockedApi = vi.mocked(api);

describe("appendOrgId", () => {
  it("appends ?orgId= when the path has no query", () => {
    expect(appendOrgId("/api/org/teachers", VALID_UUID)).toBe(
      `/api/org/teachers?orgId=${VALID_UUID}`
    );
  });

  it("appends &orgId= when the path already has a query", () => {
    expect(appendOrgId("/api/org/teachers?foo=bar", VALID_UUID)).toBe(
      `/api/org/teachers?foo=bar&orgId=${VALID_UUID}`
    );
  });

  it("returns the original path when orgId is undefined", () => {
    expect(appendOrgId("/api/org/teachers", undefined)).toBe("/api/org/teachers");
  });

  it("URL-encodes the value", () => {
    // UUID should never need encoding, but defensive nonetheless.
    expect(appendOrgId("/api/x", "a b")).toBe("/api/x?orgId=a%20b");
  });
});

describe("resolveOrgContext", () => {
  beforeEach(() => {
    mockedApi.mockReset();
  });

  it("returns ok with active org_admin when ?orgId= matches an admin membership", async () => {
    mockedApi.mockResolvedValueOnce([
      {
        orgId: VALID_UUID,
        orgName: "Acme School",
        role: "org_admin",
        status: "active",
        orgStatus: "active",
      },
    ]);
    const ctx = await resolveOrgContext({ orgId: VALID_UUID });
    expect(ctx).toEqual({ kind: "ok", orgId: VALID_UUID, orgName: "Acme School" });
  });

  it("returns ok with the first active org_admin when no ?orgId= is provided", async () => {
    mockedApi.mockResolvedValueOnce([
      {
        orgId: OTHER_UUID,
        orgName: "Suspended Org",
        role: "org_admin",
        status: "suspended",
        orgStatus: "active",
      },
      {
        orgId: VALID_UUID,
        orgName: "Acme School",
        role: "org_admin",
        status: "active",
        orgStatus: "active",
      },
    ]);
    const ctx = await resolveOrgContext({});
    expect(ctx).toEqual({ kind: "ok", orgId: VALID_UUID, orgName: "Acme School" });
  });

  it("returns no-org/not-a-member when ?orgId= doesn't match any membership", async () => {
    mockedApi.mockResolvedValueOnce([
      {
        orgId: OTHER_UUID,
        orgName: "Acme School",
        role: "org_admin",
        status: "active",
        orgStatus: "active",
      },
    ]);
    const ctx = await resolveOrgContext({ orgId: VALID_UUID });
    expect(ctx).toEqual({ kind: "no-org", reason: "not-a-member" });
  });

  it("returns no-org/not-org-admin-at-this-org when operator is teacher (not admin) at queried org", async () => {
    mockedApi.mockResolvedValueOnce([
      {
        orgId: VALID_UUID,
        orgName: "Acme School",
        role: "teacher",
        status: "active",
        orgStatus: "active",
      },
    ]);
    const ctx = await resolveOrgContext({ orgId: VALID_UUID });
    expect(ctx).toEqual({ kind: "no-org", reason: "not-org-admin-at-this-org" });
  });

  it("returns no-org/not-org-admin-at-this-org when operator's admin membership at queried org is suspended", async () => {
    mockedApi.mockResolvedValueOnce([
      {
        orgId: VALID_UUID,
        orgName: "Acme School",
        role: "org_admin",
        status: "suspended",
        orgStatus: "active",
      },
    ]);
    const ctx = await resolveOrgContext({ orgId: VALID_UUID });
    expect(ctx).toEqual({ kind: "no-org", reason: "not-org-admin-at-this-org" });
  });

  it("returns no-org/no-active-admin-membership when no ?orgId= and no active admin anywhere", async () => {
    mockedApi.mockResolvedValueOnce([
      {
        orgId: VALID_UUID,
        orgName: "Some Org",
        role: "teacher",
        status: "active",
        orgStatus: "active",
      },
    ]);
    const ctx = await resolveOrgContext({});
    expect(ctx).toEqual({ kind: "no-org", reason: "no-active-admin-membership" });
  });

  it("picks the first value when ?orgId= is delivered as an array (Next.js duplicate-query case)", async () => {
    mockedApi.mockResolvedValueOnce([
      {
        orgId: VALID_UUID,
        orgName: "Acme School",
        role: "org_admin",
        status: "active",
        orgStatus: "active",
      },
    ]);
    const ctx = await resolveOrgContext({ orgId: [VALID_UUID, "other"] });
    expect(ctx).toEqual({ kind: "ok", orgId: VALID_UUID, orgName: "Acme School" });
  });

  it("falls back to first-admin when ?orgId= is malformed (treats invalid as missing)", async () => {
    mockedApi.mockResolvedValueOnce([
      {
        orgId: VALID_UUID,
        orgName: "Acme School",
        role: "org_admin",
        status: "active",
        orgStatus: "active",
      },
    ]);
    const ctx = await resolveOrgContext({ orgId: "not-a-uuid" });
    expect(ctx).toEqual({ kind: "ok", orgId: VALID_UUID, orgName: "Acme School" });
  });

  it("returns error with status 401 when /api/orgs returns 401 ApiError", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(401, "Unauthorized"));
    const ctx = await resolveOrgContext({});
    expect(ctx).toEqual({ kind: "error", status: 401, message: "Unauthorized" });
  });

  it("returns error with status 500 when /api/orgs returns 500 ApiError", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(500, "Internal Server Error"));
    const ctx = await resolveOrgContext({ orgId: VALID_UUID });
    expect(ctx).toEqual({
      kind: "error",
      status: 500,
      message: "Internal Server Error",
    });
  });

  it("returns error with status 0 when /api/orgs throws non-ApiError", async () => {
    mockedApi.mockRejectedValueOnce(new Error("Network down"));
    const ctx = await resolveOrgContext({});
    expect(ctx).toEqual({ kind: "error", status: 0, message: "Network down" });
  });
});
