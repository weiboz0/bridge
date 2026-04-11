// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// Mock @monaco-editor/react DiffEditor
vi.mock("@monaco-editor/react", () => ({
  DiffEditor: (props: any) => (
    <div
      data-testid="monaco-diff-editor"
      data-original={props.original}
      data-modified={props.modified}
      data-readonly={props.options?.readOnly}
    />
  ),
  loader: { config: vi.fn(), init: vi.fn().mockResolvedValue({}) },
}));

vi.mock("@/lib/monaco/setup", () => ({
  setupMonaco: vi.fn(),
}));

vi.mock("@/lib/hooks/use-theme", () => ({
  useTheme: () => ({ theme: "light" }),
}));

import { DiffViewer } from "@/components/editor/diff-viewer";

describe("DiffViewer", () => {
  it("renders Monaco DiffEditor", () => {
    render(<DiffViewer original="" modified="" />);
    expect(screen.getByTestId("monaco-diff-editor")).toBeInTheDocument();
  });

  it("passes original and modified text", () => {
    render(
      <DiffViewer
        original='print("before")'
        modified='print("after")'
      />
    );
    const editor = screen.getByTestId("monaco-diff-editor");
    expect(editor.dataset.original).toBe('print("before")');
    expect(editor.dataset.modified).toBe('print("after")');
  });

  it("sets readOnly when prop is true", () => {
    render(<DiffViewer original="" modified="" readOnly />);
    const editor = screen.getByTestId("monaco-diff-editor");
    expect(editor.dataset.readonly).toBe("true");
  });

  it("is editable by default", () => {
    render(<DiffViewer original="" modified="" />);
    const editor = screen.getByTestId("monaco-diff-editor");
    expect(editor.dataset.readonly).toBe("false");
  });
});
