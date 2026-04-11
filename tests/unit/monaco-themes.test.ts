import { describe, it, expect } from "vitest";
import { bridgeLightTheme, bridgeDarkTheme } from "@/lib/monaco/themes";

describe("Monaco themes", () => {
  it("light theme has required base colors", () => {
    expect(bridgeLightTheme.base).toBe("vs");
    expect(bridgeLightTheme.colors["editor.background"]).toBeDefined();
    expect(bridgeLightTheme.colors["editor.foreground"]).toBeDefined();
    expect(bridgeLightTheme.colors["editor.lineHighlightBackground"]).toBeDefined();
    expect(bridgeLightTheme.colors["editorLineNumber.foreground"]).toBeDefined();
  });

  it("dark theme has required base colors", () => {
    expect(bridgeDarkTheme.base).toBe("vs-dark");
    expect(bridgeDarkTheme.colors["editor.background"]).toBeDefined();
    expect(bridgeDarkTheme.colors["editor.foreground"]).toBeDefined();
    expect(bridgeDarkTheme.colors["editor.lineHighlightBackground"]).toBeDefined();
    expect(bridgeDarkTheme.colors["editorLineNumber.foreground"]).toBeDefined();
  });

  it("light theme has token rules", () => {
    expect(bridgeLightTheme.rules.length).toBeGreaterThan(0);
  });

  it("dark theme has token rules", () => {
    expect(bridgeDarkTheme.rules.length).toBeGreaterThan(0);
  });

  it("themes have inherit set to true", () => {
    expect(bridgeLightTheme.inherit).toBe(true);
    expect(bridgeDarkTheme.inherit).toBe(true);
  });
});
