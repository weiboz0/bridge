// @vitest-environment jsdom

import { readFileSync } from "node:fs";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/lib/api-client", () => ({
  api: vi.fn(),
}));

import OrgChapterDetailPage from "@/app/(portal)/org/chapters/[id]/page";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";

const VALID_UUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479";
const mockedApi = vi.mocked(api);

async function renderPage(id: string) {
  const element = await OrgChapterDetailPage({ params: Promise.resolve({ id }) });
  render(element as React.ReactElement);
}

describe("OrgChapterDetailPage", () => {
  beforeEach(() => {
    mockedApi.mockReset();
  });

  it("renders 404 panel when /api/chapters/:id returns 404", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(404, "Not Found"));
    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Chapter not found" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /back to org chapters/i })).toHaveAttribute(
      "href",
      "/org/chapters",
    );
  });

  it("renders error panel with HTTP status when /api/chapters/:id returns 500", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(500, "Internal Server Error"));
    await renderPage(VALID_UUID);

    expect(
      screen.getByRole("heading", { name: /couldn.t load chapter/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/HTTP 500/)).toBeInTheDocument();
    expect(screen.getByText(/Internal Server Error/)).toBeInTheDocument();
  });

  it("sanitizes unexpected API failures", async () => {
    mockedApi.mockRejectedValueOnce(new Error("connect ECONNREFUSED http://localhost:8002"));
    await renderPage(VALID_UUID);

    expect(
      screen.getByRole("heading", { name: /couldn.t load chapter/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/request failed/)).toBeInTheDocument();
    expect(screen.queryByText(/localhost:8002/)).not.toBeInTheDocument();
  });

  it("renders 'Chapter not found' for malformed UUID without calling api", async () => {
    await renderPage("not-a-uuid");

    expect(screen.getByRole("heading", { name: "Chapter not found" })).toBeInTheDocument();
    expect(screen.getByText(/Invalid chapter id format/)).toBeInTheDocument();
    expect(mockedApi).not.toHaveBeenCalled();
  });

  it("renders metadata fields on happy path", async () => {
    mockedApi.mockResolvedValueOnce({
      id: VALID_UUID,
      scope: "org",
      scopeId: "00000000-0000-0000-0000-0000000bb001",
      title: "Loops in Python",
      slug: "loops-python",
      summary: "Org-authored chapter",
      gradeLevel: "6-8",
      status: "reviewed",
      materialType: "lesson",
      createdAt: "2026-01-15T12:00:00Z",
      createdBy: "00000000-0000-0000-0000-0000000aa001",
    });
    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Loops in Python" })).toBeInTheDocument();
    expect(screen.getByText("Org-authored chapter")).toBeInTheDocument();
    expect(screen.getByText("Reviewed")).toBeInTheDocument();
    expect(screen.getByText("Org")).toBeInTheDocument();
    expect(screen.getByText("6-8")).toBeInTheDocument();
    expect(screen.getByText("lesson")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /back to org chapters/i })).toHaveAttribute(
      "href",
      "/org/chapters",
    );
  });
});

describe("Org chapters list links", () => {
  it("links chapter titles to the org read-only detail route", () => {
    const source = readFileSync("src/app/(portal)/org/chapters/page.tsx", "utf8");

    expect(source).toMatch(/href=\{`\/org\/chapters\/\$\{u\.id\}`\}/);
    expect(source).not.toMatch(/href=\{`\/teacher\/chapters\/\$\{u\.id\}\/edit`\}/);
  });
});
