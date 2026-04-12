import { describe, it, expect } from "vitest";
import { k5Toolbox } from "@/components/editor/blockly/toolbox";

describe("K-5 Blockly toolbox", () => {
  it("has categoryToolbox kind", () => {
    expect(k5Toolbox.kind).toBe("categoryToolbox");
  });

  it("has 6 categories", () => {
    expect(k5Toolbox.contents).toHaveLength(6);
  });

  it("includes Logic, Loops, Math, Text, Variables, Functions", () => {
    const names = k5Toolbox.contents.map((c: any) => c.name);
    expect(names).toContain("Logic");
    expect(names).toContain("Loops");
    expect(names).toContain("Math");
    expect(names).toContain("Text");
    expect(names).toContain("Variables");
    expect(names).toContain("Functions");
  });

  it("Variables category uses custom VARIABLE type", () => {
    const vars = k5Toolbox.contents.find((c: any) => c.name === "Variables");
    expect(vars).toHaveProperty("custom", "VARIABLE");
  });

  it("Functions category uses custom PROCEDURE type", () => {
    const funcs = k5Toolbox.contents.find((c: any) => c.name === "Functions");
    expect(funcs).toHaveProperty("custom", "PROCEDURE");
  });

  it("each category has a colour", () => {
    for (const cat of k5Toolbox.contents) {
      expect((cat as any).colour).toBeDefined();
    }
  });
});
