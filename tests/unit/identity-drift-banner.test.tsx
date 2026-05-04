// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";

import { IdentityDriftBanner } from "@/components/portal/identity-drift-banner";

beforeEach(() => {
  vi.restoreAllMocks();
});

afterEach(() => {
  vi.restoreAllMocks();
});

function mockFetch(response: { status: number; body?: unknown }) {
  globalThis.fetch = vi.fn(async () => {
    const headers = new Headers({ "Content-Type": "application/json" });
    return new Response(
      response.body !== undefined ? JSON.stringify(response.body) : null,
      { status: response.status, headers }
    );
  }) as unknown as typeof fetch;
}

describe("IdentityDriftBanner (plan 068 phase 2)", () => {
  it("renders nothing in production (debug endpoint returns 404)", async () => {
    mockFetch({ status: 404 });
    const { container } = render(<IdentityDriftBanner />);
    // Wait a tick for the effect to run.
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when match=true (no drift)", async () => {
    mockFetch({
      status: 200,
      body: {
        match: true,
        nextAuthUserId: "user-1",
        nextAuthEmail: "u@example.com",
        goClaimsUserId: "user-1",
        goClaimsEmail: "u@example.com",
        goError: null,
      },
    });
    const { container } = render(<IdentityDriftBanner />);
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(container).toBeEmptyDOMElement();
  });

  it("renders banner with both emails when DEV_SKIP_AUTH placeholder detected", async () => {
    mockFetch({
      status: 200,
      body: {
        match: false,
        nextAuthUserId: "real-uid",
        nextAuthEmail: "real@example.com",
        goClaimsUserId: "00000000-0000-0000-0000-000000000001",
        goClaimsEmail: "dev@localhost",
        goImpersonatedBy: null,
        goError: null,
      },
    });
    render(<IdentityDriftBanner />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByText(/Auth identity mismatch detected/)).toBeInTheDocument();
    expect(screen.getByText("real@example.com")).toBeInTheDocument();
    expect(screen.getByText("dev@localhost")).toBeInTheDocument();
  });

  it("does NOT render when impersonating (legit drift between JWT and live claims)", async () => {
    mockFetch({
      status: 200,
      body: {
        match: false,
        nextAuthUserId: "admin-uid",
        nextAuthEmail: "admin@example.com",
        goClaimsUserId: "target-uid",
        goClaimsEmail: "target@example.com",
        goImpersonatedBy: "admin-uid", // ← impersonation in flight
        goError: null,
      },
    });
    const { container } = render(<IdentityDriftBanner />);
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(container).toBeEmptyDOMElement();
  });

  it("does NOT render when goClaimsUserId is some other UUID (not the dev placeholder)", async () => {
    // Real user-id mismatch (e.g., stale cookie, real bug) — banner stays
    // silent because it's specifically scoped to DEV_SKIP_AUTH, not
    // general drift. Operators check /api/auth/debug directly for those.
    mockFetch({
      status: 200,
      body: {
        match: false,
        nextAuthUserId: "user-a",
        nextAuthEmail: "a@example.com",
        goClaimsUserId: "user-b",
        goClaimsEmail: "b@example.com",
        goImpersonatedBy: null,
        goError: null,
      },
    });
    const { container } = render(<IdentityDriftBanner />);
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when both sides are null (clean unauth, not drift)", async () => {
    mockFetch({
      status: 200,
      body: {
        match: true,
        nextAuthUserId: null,
        nextAuthEmail: null,
        goClaimsUserId: null,
        goClaimsEmail: null,
        goError: null,
      },
    });
    const { container } = render(<IdentityDriftBanner />);
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when nextAuthUserId is null even if match=false (no real session to drift from)", async () => {
    mockFetch({
      status: 200,
      body: {
        match: false,
        nextAuthUserId: null,
        nextAuthEmail: null,
        goClaimsUserId: "some-id",
        goClaimsEmail: "x@example.com",
        goError: null,
      },
    });
    const { container } = render(<IdentityDriftBanner />);
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(container).toBeEmptyDOMElement();
  });

  it("falls back to userId when email is missing", async () => {
    mockFetch({
      status: 200,
      body: {
        match: false,
        nextAuthUserId: "real-uid",
        nextAuthEmail: null,
        goClaimsUserId: "00000000-0000-0000-0000-000000000001",
        goClaimsEmail: null,
        goImpersonatedBy: null,
        goError: null,
      },
    });
    render(<IdentityDriftBanner />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByText("real-uid")).toBeInTheDocument();
    expect(screen.getByText("00000000-0000-0000-0000-000000000001")).toBeInTheDocument();
  });

  it("renders nothing when fetch throws (network error)", async () => {
    globalThis.fetch = vi.fn(async () => {
      throw new Error("ECONNREFUSED");
    }) as unknown as typeof fetch;
    const { container } = render(<IdentityDriftBanner />);
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(container).toBeEmptyDOMElement();
  });

  it("does NOT render when Go is unreachable (goClaimsUserId null, goError set)", async () => {
    // The original Phase-2 implementation rendered the banner with
    // "error: <goError>" copy; Codex pass-1 caught that the banner copy
    // says "DEV_SKIP_AUTH" specifically, which would mislead operators
    // when Go is just down. Banner now stays silent on this path.
    mockFetch({
      status: 200,
      body: {
        match: false,
        nextAuthUserId: "real-uid",
        nextAuthEmail: "real@example.com",
        goClaimsUserId: null,
        goClaimsEmail: null,
        goImpersonatedBy: null,
        goError: "500: Server Error",
      },
    });
    const { container } = render(<IdentityDriftBanner />);
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(container).toBeEmptyDOMElement();
  });
});
