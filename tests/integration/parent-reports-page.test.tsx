// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";

// Mock useParams so the page can read its dynamic route segment
// without a real Next runtime.
vi.mock("next/navigation", () => ({
  useParams: () => ({ id: "child-1" }),
}));

import ParentReportsPage from "@/app/(portal)/parent/children/[id]/reports/page";

beforeEach(() => {
  (globalThis as unknown as { fetch: undefined }).fetch = undefined;
});

function mockFetch(status: number, body: unknown) {
  (globalThis as unknown as { fetch: typeof fetch }).fetch = vi.fn(async () =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    })
  ) as unknown as typeof fetch;
  return (globalThis as unknown as { fetch: ReturnType<typeof vi.fn> }).fetch;
}

describe("ParentReportsPage", () => {
  it("renders the empty state with accurate copy when 200 with []", async () => {
    mockFetch(200, []);

    render(<ParentReportsPage />);

    await waitFor(() =>
      expect(screen.getByText(/no reports yet/i)).toBeInTheDocument()
    );
    expect(
      screen.getByText(/Progress reports will appear here once they're generated/i)
    ).toBeInTheDocument();
    // Plan 080: Generate button removed (POSTed empty body to a Go
    // endpoint that requires content). No-button is the v1 contract.
    expect(
      screen.queryByRole("button", { name: /generate weekly report/i })
    ).toBeNull();
    // Plan 080: stale "linking still being built" copy removed.
    expect(screen.queryByText(/parent-child account linking/i)).toBeNull();
  });

  it("renders the report list when GET returns a non-empty array", async () => {
    // Mock data matches the Report interface shape (Kimi K2.6 plan-review NIT Q5).
    const reports = [
      {
        id: "report-1",
        periodStart: "2026-04-01T00:00:00Z",
        periodEnd: "2026-04-07T00:00:00Z",
        content: "Week 1 progress: completed 3 problems.",
        createdAt: "2026-04-08T10:00:00Z",
      },
      {
        id: "report-2",
        periodStart: "2026-04-08T00:00:00Z",
        periodEnd: "2026-04-14T00:00:00Z",
        content: "Week 2 progress: shipped first project.",
        createdAt: "2026-04-15T10:00:00Z",
      },
    ];
    mockFetch(200, reports);

    render(<ParentReportsPage />);

    await waitFor(() =>
      expect(screen.getByText(/Week 1 progress/)).toBeInTheDocument()
    );
    expect(screen.getByText(/Week 2 progress/)).toBeInTheDocument();
    // Empty-state copy should be ABSENT when reports exist.
    expect(screen.queryByText(/no reports yet/i)).toBeNull();
    // Generate button still absent (plan 080 removed entirely, not just
    // hidden when reports exist).
    expect(
      screen.queryByRole("button", { name: /generate weekly report/i })
    ).toBeNull();
  });

  it("surfaces a fetch error in a destructive style", async () => {
    mockFetch(500, { error: "Database error" });

    render(<ParentReportsPage />);

    await waitFor(() =>
      expect(screen.getByText(/Database error/)).toBeInTheDocument()
    );
  });
});
