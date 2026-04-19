// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import {
  TestResultsCard,
  type TestRunSummary,
} from "@/components/problem/test-results-card";

function summary(overrides: Partial<TestRunSummary>): TestRunSummary {
  return {
    ranAt: new Date().toISOString(),
    summary: { passed: 0, failed: 0, skipped: 0, total: 0 },
    cases: [],
    ...overrides,
  };
}

describe("TestResultsCard", () => {
  it("renders pass/total counter", () => {
    render(
      <TestResultsCard
        attemptId="a1"
        exampleLabels={{ ex1: "Example 1" }}
        result={summary({
          summary: { passed: 2, failed: 0, skipped: 0, total: 2 },
          cases: [
            { caseId: "ex1", isExample: true, status: "pass", durationMs: 11 },
            { caseId: "h1", isExample: false, status: "pass", durationMs: 12 },
          ],
        })}
      />
    );
    expect(screen.getByText("2/2")).toBeInTheDocument();
  });

  it("labels hidden cases by ordinal, never by id", () => {
    render(
      <TestResultsCard
        attemptId="a1"
        exampleLabels={{ ex1: "Example 1" }}
        result={summary({
          summary: { passed: 1, failed: 1, skipped: 0, total: 3 },
          cases: [
            { caseId: "ex1", isExample: true, status: "pass", durationMs: 9 },
            { caseId: "uuid-of-hidden-1", isExample: false, status: "fail", durationMs: 12, reason: "wrong_output" },
            { caseId: "uuid-of-hidden-2", isExample: false, status: "pass", durationMs: 10 },
          ],
        })}
      />
    );
    expect(screen.getByText("Example 1")).toBeInTheDocument();
    expect(screen.getByText("Hidden #1")).toBeInTheDocument();
    expect(screen.getByText("Hidden #2")).toBeInTheDocument();
    // Never the id
    expect(screen.queryByText("uuid-of-hidden-1")).toBeNull();
  });

  it("shows 'show diff' only on failed example cases", () => {
    render(
      <TestResultsCard
        attemptId="a1"
        exampleLabels={{ ex1: "Example 1" }}
        result={summary({
          summary: { passed: 1, failed: 2, skipped: 0, total: 3 },
          cases: [
            { caseId: "ex1", isExample: true, status: "fail", durationMs: 11, reason: "wrong_output" },
            { caseId: "ex2", isExample: true, status: "pass", durationMs: 9 },
            { caseId: "h1", isExample: false, status: "fail", durationMs: 12, reason: "wrong_output" },
          ],
        })}
      />
    );
    // One "show diff" button — only on the failed example
    expect(screen.getAllByText("show diff")).toHaveLength(1);
  });

  it("renders timeout status with amber-friendly label", () => {
    render(
      <TestResultsCard
        attemptId="a1"
        exampleLabels={{}}
        result={summary({
          summary: { passed: 0, failed: 1, skipped: 0, total: 1 },
          cases: [{ caseId: "h1", isExample: false, status: "timeout", durationMs: 3000 }],
        })}
      />
    );
    expect(screen.getByText("timeout")).toBeInTheDocument();
  });

  it("calls onClose when × is clicked", () => {
    const onClose = vi.fn();
    render(
      <TestResultsCard
        attemptId="a1"
        exampleLabels={{}}
        result={summary({})}
        onClose={onClose}
      />
    );
    fireEvent.click(screen.getByLabelText("Close"));
    expect(onClose).toHaveBeenCalled();
  });
});
