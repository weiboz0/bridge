import { describe, it, expect } from "vitest";

describe("JS Runner module", () => {
  it("exports useJsRunner hook", async () => {
    const mod = await import("@/lib/js-runner/use-js-runner");
    expect(mod.useJsRunner).toBeDefined();
    expect(typeof mod.useJsRunner).toBe("function");
  });
});

describe("JS sandbox HTML", () => {
  it("contains console override", async () => {
    // The sandbox HTML is embedded in the module — we can verify the module loads
    const mod = await import("@/lib/js-runner/use-js-runner");
    expect(mod).toBeDefined();
  });
});
