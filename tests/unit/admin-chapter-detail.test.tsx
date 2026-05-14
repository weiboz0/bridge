// @vitest-environment jsdom

import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/lib/api-client", () => ({
  api: vi.fn(),
}));

import AdminChapterDetailPage from "@/app/(portal)/admin/chapters/[id]/page";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";

const VALID_UUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479";
const mockedApi = vi.mocked(api);

async function renderPage(id: string) {
  const element = await AdminChapterDetailPage({ params: Promise.resolve({ id }) });
  render(element as React.ReactElement);
}

describe("AdminChapterDetailPage", () => {
  beforeEach(() => {
    mockedApi.mockReset();
  });

  it("renders 404 panel when /api/chapters/:id returns 404", async () => {
    mockedApi.mockRejectedValueOnce(new ApiError(404, "Not Found"));
    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Chapter not found" })).toBeInTheDocument();
    expect(screen.getByText(/back to all chapters/i)).toBeInTheDocument();
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

  it("renders error panel with status 0 when api throws non-ApiError", async () => {
    mockedApi.mockRejectedValueOnce(new Error("Network down"));
    await renderPage(VALID_UUID);

    expect(
      screen.getByRole("heading", { name: /couldn.t load chapter/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/Network down/)).toBeInTheDocument();
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
      scope: "platform",
      scopeId: null,
      title: "Print & Comments",
      slug: "print-comments",
      summary: "First chapter",
      gradeLevel: "K-5",
      status: "classroom_ready",
      materialType: "lesson",
      createdAt: "2026-01-15T12:00:00Z",
      createdBy: "00000000-0000-0000-0000-0000000aa001",
    });
    await renderPage(VALID_UUID);

    expect(screen.getByRole("heading", { name: "Print & Comments" })).toBeInTheDocument();
    expect(screen.getByText("First chapter")).toBeInTheDocument();
    expect(screen.getByText("Classroom Ready")).toBeInTheDocument();
    expect(screen.getByText("Platform")).toBeInTheDocument();
    expect(screen.getByText("K-5")).toBeInTheDocument();
    expect(screen.getByText("lesson")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /back to all chapters/i })).toHaveAttribute(
      "href",
      "/admin/chapters",
    );
  });
});
