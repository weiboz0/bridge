import { describe, it, expect } from "vitest";
import { generateJoinCode } from "@/lib/utils";

describe("generateJoinCode", () => {
  it("returns a string of 8 characters", () => {
    const code = generateJoinCode();
    expect(code).toHaveLength(8);
  });

  it("contains only uppercase alphanumeric characters", () => {
    const code = generateJoinCode();
    expect(code).toMatch(/^[A-Z0-9]{8}$/);
  });

  it("generates unique codes", () => {
    const codes = new Set(Array.from({ length: 100 }, () => generateJoinCode()));
    expect(codes.size).toBe(100);
  });
});
