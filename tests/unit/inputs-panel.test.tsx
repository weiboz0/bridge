// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { InputsPanel, selectionStdin } from "@/components/problem/inputs-panel";
import type { TestCase } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";

function c(id: string, overrides: Partial<TestCase>): TestCase {
  return {
    id,
    problemId: "p1",
    ownerId: null,
    name: "",
    stdin: "",
    expectedStdout: null,
    isExample: false,
    order: 0,
    ...overrides,
  };
}

describe("InputsPanel", () => {
  it("renders one chip per example + per private case + Custom", () => {
    const testCases = [
      c("ex1", { isExample: true, name: "Ex 1", stdin: "a" }),
      c("ex2", { isExample: true, name: "Ex 2", stdin: "b" }),
      c("pr1", { ownerId: "u1", name: "Priv", stdin: "p" }),
    ];
    render(
      <InputsPanel
        testCases={testCases}
        selection={{ kind: "custom" }} // pick Custom so chip labels don't duplicate into the preview header
        customStdin=""
        onSelectionChange={() => {}}
        onCustomStdinChange={() => {}}
      />
    );
    expect(screen.getByText("Ex 1")).toBeInTheDocument();
    expect(screen.getByText("Ex 2")).toBeInTheDocument();
    expect(screen.getByText("Mine · Priv")).toBeInTheDocument();
    expect(screen.getByText("Custom…")).toBeInTheDocument();
  });

  it("clicking a chip fires onSelectionChange with the case id", () => {
    const onSelectionChange = vi.fn();
    const testCases = [c("ex1", { isExample: true, name: "Ex 1", stdin: "a" })];
    render(
      <InputsPanel
        testCases={testCases}
        selection={{ kind: "custom" }}
        customStdin=""
        onSelectionChange={onSelectionChange}
        onCustomStdinChange={() => {}}
      />
    );
    fireEvent.click(screen.getByText("Ex 1"));
    expect(onSelectionChange).toHaveBeenCalledWith({ kind: "case", caseId: "ex1" });
  });

  it("shows the selected case's stdin preview", () => {
    const testCases = [c("ex1", { isExample: true, name: "Ex 1", stdin: "hello\nworld" })];
    const { container } = render(
      <InputsPanel
        testCases={testCases}
        selection={{ kind: "case", caseId: "ex1" }}
        customStdin=""
        onSelectionChange={() => {}}
        onCustomStdinChange={() => {}}
      />
    );
    // Query the <pre> directly so DOM whitespace normalization doesn't hide the \n.
    const pre = container.querySelector("pre");
    expect(pre?.textContent).toBe("hello\nworld");
    // 2 lines badge
    expect(screen.getByText("2 lines")).toBeInTheDocument();
  });

  it("custom mode shows the textarea and propagates edits", () => {
    const onCustomStdinChange = vi.fn();
    render(
      <InputsPanel
        testCases={[]}
        selection={{ kind: "custom" }}
        customStdin="abc"
        onSelectionChange={() => {}}
        onCustomStdinChange={onCustomStdinChange}
      />
    );
    const textarea = screen.getByPlaceholderText(/Type stdin here/i);
    expect(textarea).toHaveValue("abc");
    fireEvent.change(textarea, { target: { value: "xyz" } });
    expect(onCustomStdinChange).toHaveBeenCalledWith("xyz");
  });
});

describe("selectionStdin helper", () => {
  const testCases = [
    c("ex1", { isExample: true, stdin: "1 2" }),
    c("pr1", { ownerId: "u1", stdin: "private" }),
  ];

  it("returns the selected case's stdin", () => {
    expect(selectionStdin({ kind: "case", caseId: "ex1" }, testCases, "CUSTOM")).toBe("1 2");
    expect(selectionStdin({ kind: "case", caseId: "pr1" }, testCases, "CUSTOM")).toBe("private");
  });

  it("returns the custom stdin when custom is selected", () => {
    expect(selectionStdin({ kind: "custom" }, testCases, "custom content")).toBe("custom content");
  });

  it("returns empty string for a missing case id", () => {
    expect(selectionStdin({ kind: "case", caseId: "nope" }, testCases, "x")).toBe("");
  });
});
