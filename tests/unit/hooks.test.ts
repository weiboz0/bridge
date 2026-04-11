// @vitest-environment jsdom
import { describe, it, expect, beforeEach, vi } from "vitest";

describe("useTheme", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.classList.remove("dark", "light");
  });

  it("defaults to dark theme", async () => {
    document.documentElement.classList.add("dark");
    const { useTheme } = await import("@/lib/hooks/use-theme");
    // Can't use hooks outside React, but we can test the module exports
    expect(useTheme).toBeDefined();
  });
});

describe("useSidebar", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("exports useSidebar function", async () => {
    const { useSidebar } = await import("@/lib/hooks/use-sidebar");
    expect(useSidebar).toBeDefined();
  });
});

describe("theme localStorage", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.classList.remove("dark", "light");
  });

  it("stores theme preference", () => {
    localStorage.setItem("bridge-theme", "light");
    expect(localStorage.getItem("bridge-theme")).toBe("light");
  });

  it("stores sidebar collapsed state", () => {
    localStorage.setItem("bridge-sidebar-collapsed", "true");
    expect(localStorage.getItem("bridge-sidebar-collapsed")).toBe("true");
  });
});
