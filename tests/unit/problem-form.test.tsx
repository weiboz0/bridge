// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const pushMock = vi.fn();
const refreshMock = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock, refresh: refreshMock, back: vi.fn() }),
}));

// Stub out Monaco — its dynamic import doesn't matter for the slug
// 409 test path and tries to touch the DOM in ways jsdom doesn't
// like.
vi.mock("@/components/editor/code-editor", () => ({
  CodeEditor: () => null,
}));

import { ProblemForm } from "@/components/problem/problem-form";

beforeEach(() => {
  pushMock.mockReset();
  refreshMock.mockReset();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

const IDENTITY = { userId: "user-1", isPlatformAdmin: false };

function setFetchMock(impl: typeof fetch) {
  vi.stubGlobal("fetch", vi.fn(impl));
}

describe("ProblemForm — plan 071 inline slug error on 409", () => {
  it("pins slug-409 inline next to the slug input rather than the generic banner", async () => {
    // The /api/orgs fetch resolves immediately to no orgs so we
    // stay in personal scope; the /api/problems POST returns 409
    // with the field-pinned body.
    setFetchMock(async (input: Request | string | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.endsWith("/api/orgs")) {
        return new Response("[]", {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      if (url.endsWith("/api/problems")) {
        return new Response(
          JSON.stringify({
            error: "Slug already taken in this scope",
            field: "slug",
          }),
          { status: 409, headers: { "content-type": "application/json" } },
        );
      }
      throw new Error(`Unexpected fetch: ${url}`);
    });

    render(<ProblemForm mode="create" identity={IDENTITY} />);

    fireEvent.change(screen.getByLabelText(/Title/i), {
      target: { value: "Two Sum" },
    });
    fireEvent.change(screen.getByLabelText(/Slug/i), {
      target: { value: "two-sum" },
    });

    const submit = await screen.findByRole("button", { name: /Create problem/i });
    fireEvent.click(submit);

    // Slug message lives next to the slug input (the form's
    // field-error <p> uses text-destructive styling).
    await waitFor(() => {
      expect(
        screen.getByText(/Slug already taken in this scope/i),
      ).toBeInTheDocument();
    });

    // The slug input should be marked invalid for AT-tools.
    const slugInput = screen.getByLabelText(/Slug/i);
    expect(slugInput.getAttribute("aria-invalid")).toBe("true");

    // The generic error banner (role=alert) must NOT also fire — the
    // form clears the banner state when the field-pinned 409 arrives.
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("clears the slug error when the user edits the slug field", async () => {
    setFetchMock(async (input: Request | string | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.endsWith("/api/orgs")) {
        return new Response("[]", { status: 200 });
      }
      if (url.endsWith("/api/problems")) {
        return new Response(
          JSON.stringify({ error: "Slug already taken", field: "slug" }),
          { status: 409, headers: { "content-type": "application/json" } },
        );
      }
      throw new Error(`Unexpected fetch: ${url}`);
    });

    render(<ProblemForm mode="create" identity={IDENTITY} />);

    fireEvent.change(screen.getByLabelText(/Title/i), {
      target: { value: "Two Sum" },
    });
    fireEvent.change(screen.getByLabelText(/Slug/i), {
      target: { value: "two-sum" },
    });
    fireEvent.click(await screen.findByRole("button", { name: /Create problem/i }));

    await waitFor(() => {
      expect(screen.getByText(/Slug already taken/i)).toBeInTheDocument();
    });

    // Edit the slug — the inline error should clear.
    fireEvent.change(screen.getByLabelText(/Slug/i), {
      target: { value: "two-sum-v2" },
    });
    expect(screen.queryByText(/Slug already taken/i)).toBeNull();
  });

  it("non-409 errors still surface in the generic banner", async () => {
    setFetchMock(async (input: Request | string | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.endsWith("/api/orgs")) {
        return new Response("[]", { status: 200 });
      }
      if (url.endsWith("/api/problems")) {
        return new Response(JSON.stringify({ error: "Server unhappy" }), {
          status: 500,
          headers: { "content-type": "application/json" },
        });
      }
      throw new Error(`Unexpected fetch: ${url}`);
    });

    render(<ProblemForm mode="create" identity={IDENTITY} />);
    fireEvent.change(screen.getByLabelText(/Title/i), {
      target: { value: "X" },
    });
    fireEvent.click(await screen.findByRole("button", { name: /Create problem/i }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert").textContent).toMatch(/Server unhappy/);
  });
});
