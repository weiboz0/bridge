// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// Mock @monaco-editor/react since Monaco requires a real browser DOM
vi.mock("@monaco-editor/react", () => ({
  Editor: (props: any) => (
    <div
      data-testid="monaco-editor"
      data-language={props.language}
      data-readonly={props.options?.readOnly}
      data-value={props.defaultValue}
    />
  ),
  loader: { config: vi.fn(), init: vi.fn().mockResolvedValue({}) },
}));

vi.mock("y-monaco", () => ({
  MonacoBinding: vi.fn().mockImplementation(() => ({
    destroy: vi.fn(),
  })),
}));

vi.mock("@/lib/monaco/setup", () => ({
  setupMonaco: vi.fn(),
}));

vi.mock("@/lib/hooks/use-theme", () => ({
  useTheme: () => ({ theme: "light" }),
}));

import { CodeEditor } from "@/components/editor/code-editor";

describe("CodeEditor", () => {
  it("renders Monaco editor", () => {
    render(<CodeEditor />);
    expect(screen.getByTestId("monaco-editor")).toBeInTheDocument();
  });

  it("passes initialCode as defaultValue", () => {
    render(<CodeEditor initialCode='print("hi")' />);
    const editor = screen.getByTestId("monaco-editor");
    expect(editor.dataset.value).toBe('print("hi")');
  });

  it("sets python as the language", () => {
    render(<CodeEditor />);
    const editor = screen.getByTestId("monaco-editor");
    expect(editor.dataset.language).toBe("python");
  });

  it("sets readOnly when prop is true", () => {
    render(<CodeEditor readOnly />);
    const editor = screen.getByTestId("monaco-editor");
    expect(editor.dataset.readonly).toBe("true");
  });

  it("is editable by default", () => {
    render(<CodeEditor />);
    const editor = screen.getByTestId("monaco-editor");
    expect(editor.dataset.readonly).toBe("false");
  });

  it("renders container with border and rounded styling", () => {
    const { container } = render(<CodeEditor />);
    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper.className).toContain("border");
    expect(wrapper.className).toContain("rounded-lg");
  });
});
