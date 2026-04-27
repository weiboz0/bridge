// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { useState } from "react";
import { render, screen, fireEvent } from "@testing-library/react";

/**
 * The inline tab semantics from problem-shell.tsx — a self-contained
 * test of the active-tab state machine + the ARIA wiring. Re-renders
 * the same JSX shape the shell uses but without Monaco/Yjs/Pyodide
 * (which would each need a heavy mock just to assert tab behavior).
 *
 * Plan 042 phase 1.5: Vitest covers the state machine + ARIA contract;
 * Playwright covers actual viewport-driven layout.
 */
function ResponsiveTabHarness() {
  type Tab = "problem" | "code" | "io";
  const [narrowTab, setNarrowTab] = useState<Tab>("code");

  const paneClass = (id: Tab) =>
    narrowTab === id ? "flex" : "hidden lg:flex";

  return (
    <div>
      <div role="tablist" aria-label="Problem editor sections" className="lg:hidden">
        {(["problem", "code", "io"] as const).map((id) => (
          <button
            key={id}
            role="tab"
            id={`problem-tab-${id}`}
            aria-selected={narrowTab === id}
            aria-controls={`problem-pane-${id}`}
            onClick={() => setNarrowTab(id)}
          >
            {id}
          </button>
        ))}
      </div>
      <div
        role="tabpanel"
        id="problem-pane-problem"
        aria-labelledby="problem-tab-problem"
        className={paneClass("problem")}
      >
        problem-content
      </div>
      <div
        role="tabpanel"
        id="problem-pane-code"
        aria-labelledby="problem-tab-code"
        className={paneClass("code")}
      >
        code-content
      </div>
      <div
        role="tabpanel"
        id="problem-pane-io"
        aria-labelledby="problem-tab-io"
        className={paneClass("io")}
      >
        io-content
      </div>
    </div>
  );
}

describe("ProblemShell narrow-tab semantics", () => {
  it("starts on code (the editor is the load-bearing pane)", () => {
    render(<ResponsiveTabHarness />);
    expect(screen.getByRole("tab", { name: "code" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "problem" })).toHaveAttribute("aria-selected", "false");
    expect(screen.getByRole("tab", { name: "io" })).toHaveAttribute("aria-selected", "false");
  });

  it("clicking a tab switches the active selection", () => {
    render(<ResponsiveTabHarness />);
    fireEvent.click(screen.getByRole("tab", { name: "problem" }));
    expect(screen.getByRole("tab", { name: "problem" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "code" })).toHaveAttribute("aria-selected", "false");

    fireEvent.click(screen.getByRole("tab", { name: "io" }));
    expect(screen.getByRole("tab", { name: "io" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "problem" })).toHaveAttribute("aria-selected", "false");
  });

  it("each tab's aria-controls points at a tabpanel with the matching id", () => {
    render(<ResponsiveTabHarness />);
    const tabs = screen.getAllByRole("tab");
    for (const tab of tabs) {
      const controlsId = tab.getAttribute("aria-controls");
      expect(controlsId).toBeTruthy();
      const panel = document.getElementById(controlsId!);
      expect(panel).toBeTruthy();
      expect(panel).toHaveAttribute("role", "tabpanel");
      expect(panel).toHaveAttribute("aria-labelledby", tab.id);
    }
  });

  it("active pane gets `flex`; inactive panes get `hidden lg:flex` so they stay mounted at wide widths", () => {
    render(<ResponsiveTabHarness />);
    const codePane = document.getElementById("problem-pane-code")!;
    const problemPane = document.getElementById("problem-pane-problem")!;
    expect(codePane.className).toContain("flex");
    expect(codePane.className).not.toContain("hidden");
    expect(problemPane.className).toContain("hidden");
    expect(problemPane.className).toContain("lg:flex");

    fireEvent.click(screen.getByRole("tab", { name: "problem" }));
    expect(problemPane.className).toContain("flex");
    expect(problemPane.className).not.toContain("hidden");
    expect(codePane.className).toContain("hidden");
    expect(codePane.className).toContain("lg:flex");
  });
});
