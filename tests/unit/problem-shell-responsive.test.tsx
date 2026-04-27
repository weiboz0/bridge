// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { useState } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ResponsiveTabs, type NarrowTabId } from "@/components/problem/responsive-tabs";

/**
 * Tests the actual ResponsiveTabs component used by problem-shell.tsx
 * — Codex post-impl review #7 flagged that an in-test reimplementation
 * would let the real component drift silently. By importing the real
 * component, this test locks the contract.
 *
 * The wrapper provides the active-tab state (which lives in
 * problem-shell.tsx in production); the component itself owns the
 * tab markup, ARIA attributes, and keyboard interaction.
 */
function TabsHarness({ initial = "code" as NarrowTabId }: { initial?: NarrowTabId }) {
  const [active, setActive] = useState<NarrowTabId>(initial);
  return (
    <>
      <ResponsiveTabs active={active} onChange={setActive} />
      <div role="tabpanel" id="problem-pane-problem" aria-labelledby="problem-tab-problem" />
      <div role="tabpanel" id="problem-pane-code" aria-labelledby="problem-tab-code" />
      <div role="tabpanel" id="problem-pane-io" aria-labelledby="problem-tab-io" />
    </>
  );
}

describe("ResponsiveTabs", () => {
  it("starts with the initial active tab selected", () => {
    render(<TabsHarness initial="code" />);
    expect(screen.getByRole("tab", { name: "Code" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Problem" })).toHaveAttribute("aria-selected", "false");
    expect(screen.getByRole("tab", { name: "I/O" })).toHaveAttribute("aria-selected", "false");
  });

  it("clicking a tab switches the active selection", () => {
    render(<TabsHarness />);
    fireEvent.click(screen.getByRole("tab", { name: "Problem" }));
    expect(screen.getByRole("tab", { name: "Problem" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Code" })).toHaveAttribute("aria-selected", "false");
  });

  it("each tab's aria-controls points at a tabpanel with the matching id", () => {
    render(<TabsHarness />);
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

  it("only the active tab is in the document tab order (roving tabindex)", () => {
    render(<TabsHarness initial="code" />);
    expect(screen.getByRole("tab", { name: "Code" })).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("tab", { name: "Problem" })).toHaveAttribute("tabindex", "-1");
    expect(screen.getByRole("tab", { name: "I/O" })).toHaveAttribute("tabindex", "-1");
  });

  it("ArrowRight moves focus and selection to the next tab; wraps at end", () => {
    render(<TabsHarness initial="code" />);
    const codeTab = screen.getByRole("tab", { name: "Code" });
    codeTab.focus();
    fireEvent.keyDown(codeTab, { key: "ArrowRight" });
    expect(screen.getByRole("tab", { name: "I/O" })).toHaveAttribute("aria-selected", "true");

    const ioTab = screen.getByRole("tab", { name: "I/O" });
    fireEvent.keyDown(ioTab, { key: "ArrowRight" });
    // Wraps from I/O → Problem
    expect(screen.getByRole("tab", { name: "Problem" })).toHaveAttribute("aria-selected", "true");
  });

  it("ArrowLeft moves focus and selection to the previous tab; wraps at start", () => {
    render(<TabsHarness initial="problem" />);
    const problemTab = screen.getByRole("tab", { name: "Problem" });
    problemTab.focus();
    fireEvent.keyDown(problemTab, { key: "ArrowLeft" });
    // Wraps from Problem → I/O
    expect(screen.getByRole("tab", { name: "I/O" })).toHaveAttribute("aria-selected", "true");
  });

  it("Home and End jump to the first/last tab", () => {
    render(<TabsHarness initial="code" />);
    const codeTab = screen.getByRole("tab", { name: "Code" });
    codeTab.focus();
    fireEvent.keyDown(codeTab, { key: "End" });
    expect(screen.getByRole("tab", { name: "I/O" })).toHaveAttribute("aria-selected", "true");

    const ioTab = screen.getByRole("tab", { name: "I/O" });
    fireEvent.keyDown(ioTab, { key: "Home" });
    expect(screen.getByRole("tab", { name: "Problem" })).toHaveAttribute("aria-selected", "true");
  });

  it("calls onChange with the new tab id on click", () => {
    const onChange = vi.fn();
    render(<ResponsiveTabs active="code" onChange={onChange} />);
    fireEvent.click(screen.getByRole("tab", { name: "I/O" }));
    expect(onChange).toHaveBeenCalledWith("io");
  });
});
