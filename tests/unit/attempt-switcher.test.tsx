// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { AttemptSwitcher } from "@/components/problem/attempt-switcher";
import type { Attempt } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";

function makeAttempt(id: string, title: string, updatedMinsAgo: number): Attempt {
  const t = new Date(Date.now() - updatedMinsAgo * 60_000).toISOString();
  return {
    id,
    problemId: "p1",
    userId: "u1",
    title,
    language: "python",
    plainText: "",
    createdAt: t,
    updatedAt: t,
  };
}

describe("AttemptSwitcher", () => {
  it("renders nothing when there are no attempts", () => {
    const { container } = render(
      <AttemptSwitcher attempts={[]} activeId={null} onSwitch={() => {}} />
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders the trigger when attempts exist", () => {
    const attempts = [makeAttempt("a1", "First", 0), makeAttempt("a2", "Second", 10)];
    render(<AttemptSwitcher attempts={attempts} activeId="a1" onSwitch={() => {}} />);
    expect(screen.getByText("switch")).toBeInTheDocument();
  });

  it("fires onSwitch with the clicked attempt", async () => {
    const attempts = [makeAttempt("a1", "First", 0), makeAttempt("a2", "Second", 10)];
    const onSwitch = vi.fn();
    render(<AttemptSwitcher attempts={attempts} activeId="a1" onSwitch={onSwitch} />);

    fireEvent.click(screen.getByText("switch"));
    // Menu items render in a portal — wait for them and click "Second"
    const item = await screen.findByText("Second");
    fireEvent.click(item);

    expect(onSwitch).toHaveBeenCalledTimes(1);
    expect(onSwitch.mock.calls[0]![0]).toMatchObject({ id: "a2" });
  });
});
