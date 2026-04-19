// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { AttemptCardsRow } from "@/components/problem/attempt-cards-row";
import type { Attempt } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";

function makeAttempt(id: string, title: string, mins: number, code = ""): Attempt {
  const t = new Date(Date.now() - mins * 60_000).toISOString();
  return {
    id,
    problemId: "p1",
    userId: "s1",
    title,
    language: "python",
    plainText: code,
    createdAt: t,
    updatedAt: t,
  };
}

describe("AttemptCardsRow", () => {
  it("shows empty state when no attempts", () => {
    render(<AttemptCardsRow attempts={[]} activeId={null} onSelect={() => {}} />);
    expect(screen.getByText(/Not started yet/i)).toBeInTheDocument();
  });

  it("renders the top three attempts", () => {
    const attempts = [
      makeAttempt("a1", "Latest", 0),
      makeAttempt("a2", "Mid", 10),
      makeAttempt("a3", "Old", 60),
      makeAttempt("a4", "Older", 120),
    ];
    render(<AttemptCardsRow attempts={attempts} activeId="a1" onSelect={() => {}} />);
    expect(screen.getByText("Latest")).toBeInTheDocument();
    expect(screen.getByText("Mid")).toBeInTheDocument();
    expect(screen.getByText("Old")).toBeInTheDocument();
    expect(screen.queryByText("Older")).toBeNull();
  });

  it("marks the active attempt with the Active tag", () => {
    const attempts = [
      makeAttempt("a1", "First", 0),
      makeAttempt("a2", "Second", 10),
    ];
    render(<AttemptCardsRow attempts={attempts} activeId="a2" onSelect={() => {}} />);
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("calls onSelect with the clicked attempt", () => {
    const attempts = [
      makeAttempt("a1", "First", 0),
      makeAttempt("a2", "Second", 10),
    ];
    const onSelect = vi.fn();
    render(<AttemptCardsRow attempts={attempts} activeId="a1" onSelect={onSelect} />);
    fireEvent.click(screen.getByText("Second"));
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "a2" }));
  });

  it("shows preview of the first 3 lines of code", () => {
    const code = "line 1\nline 2\nline 3\nline 4";
    const attempts = [makeAttempt("a1", "Has code", 0, code)];
    const { container } = render(
      <AttemptCardsRow attempts={attempts} activeId="a1" onSelect={() => {}} />
    );
    const pre = container.querySelector("pre");
    expect(pre?.textContent).toBe("line 1\nline 2\nline 3");
  });
});
