// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ProblemDescription } from "@/components/problem/problem-description";
import type { Problem, TestCase } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";

const problem: Problem = {
  id: "p1",
  title: "Two Sum",
  description: "Given **nums** and a target, return indices of two numbers that add up.",
  starterCode: { python: "def solve(): pass" },
  createdBy: "u1",
};

function makeCase(overrides: Partial<TestCase>): TestCase {
  return {
    id: "c" + Math.random(),
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

describe("ProblemDescription", () => {
  it("renders the title and the language badge", () => {
    render(<ProblemDescription problem={problem} testCases={[]} language="python" />);
    expect(screen.getByRole("heading", { level: 1, name: "Two Sum" })).toBeInTheDocument();
    expect(screen.getByText("python")).toBeInTheDocument();
  });

  it("renders markdown (including bold)", () => {
    const { container } = render(<ProblemDescription problem={problem} testCases={[]} />);
    const strong = container.querySelector("strong");
    expect(strong?.textContent).toBe("nums");
  });

  it("renders example Input/Output blocks for canonical examples only", () => {
    const examples = [
      makeCase({ id: "ex1", stdin: "1 2", expectedStdout: "3", isExample: true, name: "Ex 1" }),
      makeCase({ id: "ex2", stdin: "4 5", expectedStdout: "9", isExample: true, name: "Ex 2" }),
    ];
    const hidden = makeCase({ id: "h1", stdin: "x", expectedStdout: "y", isExample: false });
    const priv = makeCase({ id: "p1", ownerId: "u1", stdin: "a", expectedStdout: "b", isExample: false });

    render(<ProblemDescription problem={problem} testCases={[...examples, hidden, priv]} />);

    expect(screen.getByText("Ex 1")).toBeInTheDocument();
    expect(screen.getByText("Ex 2")).toBeInTheDocument();
    // Stdin / stdout bodies should appear exactly once each (no echoing from private / hidden)
    expect(screen.getAllByText("1 2")).toHaveLength(1);
    expect(screen.getAllByText("4 5")).toHaveLength(1);
    expect(screen.queryByText("x")).toBeNull();
    expect(screen.queryByText("a")).toBeNull();
  });

  it("shows hidden-case count when canonical-hidden exists", () => {
    const testCases = [
      makeCase({ stdin: "x", expectedStdout: "y", isExample: false }),
      makeCase({ stdin: "p", expectedStdout: "q", isExample: false }),
      makeCase({ stdin: "m", expectedStdout: "n", isExample: false }),
    ];
    render(<ProblemDescription problem={problem} testCases={testCases} />);
    // "3" appears in the small count circle + the sentence
    expect(screen.getByText(/more hidden test cases run on/i)).toBeInTheDocument();
  });

  it("omits the hidden-count row when there are no hidden cases", () => {
    render(<ProblemDescription problem={problem} testCases={[]} />);
    expect(screen.queryByText(/more hidden test case/i)).toBeNull();
  });

  it("handles empty description gracefully", () => {
    const noDesc = { ...problem, description: "" };
    const { container } = render(<ProblemDescription problem={noDesc} testCases={[]} />);
    expect(container.querySelector(".prose")).toBeNull();
  });
});
