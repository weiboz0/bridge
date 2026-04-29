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
  (globalThis as any).fetch = undefined;
});

function mockFetch(status: number, body: any) {
  (globalThis as any).fetch = vi.fn(async () =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    })
  );
  return (globalThis as any).fetch as ReturnType<typeof vi.fn>;
}

describe("ParentReportsPage — Plan 047 phase 2 disabled state", () => {
  it("renders 'Reports coming soon' when GET returns 501", async () => {
    mockFetch(501, { error: "scheduled for plan 049", code: "not_implemented" });

    render(<ParentReportsPage />);

    await waitFor(() =>
      expect(screen.getByText(/reports coming soon/i)).toBeInTheDocument()
    );
    expect(screen.getByText(/parent-child account linking/i)).toBeInTheDocument();
  });

  it("hides the Generate button when 501 is returned", async () => {
    mockFetch(501, { error: "scheduled for plan 049", code: "not_implemented" });

    render(<ParentReportsPage />);

    await waitFor(() =>
      expect(screen.getByText(/reports coming soon/i)).toBeInTheDocument()
    );
    expect(
      screen.queryByRole("button", { name: /generate weekly report/i })
    ).toBeNull();
  });

  it("renders the empty state (not the disabled state) when 200 with []", async () => {
    mockFetch(200, []);

    render(<ParentReportsPage />);

    await waitFor(() =>
      expect(screen.getByText(/no reports yet/i)).toBeInTheDocument()
    );
    expect(screen.queryByText(/reports coming soon/i)).toBeNull();
    expect(
      screen.getByRole("button", { name: /generate weekly report/i })
    ).toBeInTheDocument();
  });
});
