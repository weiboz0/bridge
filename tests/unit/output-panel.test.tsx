// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { OutputPanel } from "@/components/editor/output-panel";

describe("OutputPanel", () => {
  it("renders empty state when no output", () => {
    render(<OutputPanel output={[]} running={false} />);
    expect(screen.getByTestId("output-panel")).toBeInTheDocument();
    expect(screen.getByTestId("output-panel")).toHaveTextContent("");
  });

  it("renders stdout lines", () => {
    const output = [
      { type: "stdout" as const, text: "Hello, world!" },
      { type: "stdout" as const, text: "Line 2" },
    ];
    render(<OutputPanel output={output} running={false} />);
    expect(screen.getByText("Hello, world!")).toBeInTheDocument();
    expect(screen.getByText("Line 2")).toBeInTheDocument();
  });

  it("renders stderr lines with error styling", () => {
    const output = [
      { type: "stderr" as const, text: "NameError: name 'x' is not defined" },
    ];
    render(<OutputPanel output={output} running={false} />);
    const errorLine = screen.getByText("NameError: name 'x' is not defined");
    expect(errorLine).toBeInTheDocument();
    expect(errorLine.className).toContain("text-red");
  });

  it("shows running indicator when running", () => {
    render(<OutputPanel output={[]} running={true} />);
    expect(screen.getByText("Running...")).toBeInTheDocument();
  });

  it("hides Running indicator while awaiting stdin", () => {
    render(
      <OutputPanel
        output={[{ type: "stdout", text: "What is your name? " }]}
        running={true}
        awaitingInput={true}
        onStdin={() => {}}
      />
    );
    expect(screen.queryByText("Running...")).toBeNull();
  });

  it("renders stdin input when awaitingInput", () => {
    render(
      <OutputPanel
        output={[]}
        running={true}
        awaitingInput={true}
        onStdin={() => {}}
      />
    );
    expect(screen.getByLabelText("stdin")).toBeInTheDocument();
  });

  it("calls onStdin with the typed line when Enter is pressed", () => {
    const onStdin = vi.fn();
    render(
      <OutputPanel
        output={[]}
        running={true}
        awaitingInput={true}
        onStdin={onStdin}
      />
    );
    const input = screen.getByLabelText("stdin");
    fireEvent.change(input, { target: { value: "Ada" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onStdin).toHaveBeenCalledWith("Ada");
    expect((input as HTMLInputElement).value).toBe(""); // cleared after submit
  });

  it("does not render stdin input when not awaitingInput", () => {
    render(
      <OutputPanel output={[]} running={true} awaitingInput={false} onStdin={() => {}} />
    );
    expect(screen.queryByLabelText("stdin")).toBeNull();
  });
});
