import { describe, it, expect } from "vitest";
import {
  parseOrgIdFromSearchParams,
  appendOrgId,
} from "@/lib/portal/org-context";

const VALID_UUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479";

describe("parseOrgIdFromSearchParams", () => {
  it("returns the value when valid", () => {
    expect(parseOrgIdFromSearchParams({ orgId: VALID_UUID })).toBe(VALID_UUID);
  });

  it("returns undefined for missing param", () => {
    expect(parseOrgIdFromSearchParams({})).toBeUndefined();
    expect(parseOrgIdFromSearchParams(undefined)).toBeUndefined();
  });

  it("rejects non-UUID strings", () => {
    expect(parseOrgIdFromSearchParams({ orgId: "not-a-uuid" })).toBeUndefined();
    expect(parseOrgIdFromSearchParams({ orgId: "12345" })).toBeUndefined();
  });

  it("picks the first value when given an array", () => {
    expect(parseOrgIdFromSearchParams({ orgId: [VALID_UUID, "other"] })).toBe(VALID_UUID);
  });
});

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
