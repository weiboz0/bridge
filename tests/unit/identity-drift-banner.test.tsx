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

  it("renders banner with both emails when drift detected", async () => {
    mockFetch({
      status: 200,
      body: {
        match: false,
        nextAuthUserId: "real-uid",
        nextAuthEmail: "real@example.com",
        goClaimsUserId: "00000000-0000-0000-0000-000000000001",
        goClaimsEmail: "dev@localhost",
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

  it("surfaces goError when no goClaimsUserId/email available", async () => {
    mockFetch({
      status: 200,
      body: {
        match: false,
        nextAuthUserId: "real-uid",
        nextAuthEmail: "real@example.com",
        goClaimsUserId: null,
        goClaimsEmail: null,
        goError: "401: Unauthorized",
      },
    });
    render(<IdentityDriftBanner />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByText(/error: 401: Unauthorized/)).toBeInTheDocument();
  });
});
