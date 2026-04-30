// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import * as React from "react";

// Mock the api-client so we control the doc list per test. Hoisted
// because vi.mock factories are hoisted; a plain top-level const
// would not be initialized in time.
const { apiMock } = vi.hoisted(() => ({ apiMock: vi.fn() }));
vi.mock("@/lib/api-client", () => ({
  api: apiMock,
}));

import StudentWorkPage from "@/app/(portal)/student/code/page";

beforeEach(() => {
  apiMock.mockReset();
});

async function renderPage() {
  const tree = await StudentWorkPage();
  render(<>{tree}</>);
}

const baseDoc = {
  language: "python",
  plainText: "x = 1",
  updatedAt: "2026-04-30T12:00:00Z",
};

describe("My Work navigability — Plan 048 phase 7", () => {
  it("links live-session doc to /student/sessions/{sessionId}", async () => {
    apiMock.mockResolvedValueOnce([
      {
        id: "doc-live",
        ...baseDoc,
        sessionId: "sess-1",
        classId: "class-1",
        sessionStatus: "live",
      },
    ]);

    await renderPage();

    const link = screen.getByRole("link");
    expect(link.getAttribute("href")).toBe("/student/sessions/sess-1");
    expect(screen.getByText(/open live session/i)).toBeInTheDocument();
  });

  it("ended-session doc links to /student/classes/{classId} (not the broken sessions route)", async () => {
    apiMock.mockResolvedValueOnce([
      {
        id: "doc-ended",
        ...baseDoc,
        sessionId: "sess-2",
        classId: "class-2",
        sessionStatus: "ended",
      },
    ]);

    await renderPage();

    const link = screen.getByRole("link");
    expect(link.getAttribute("href")).toBe("/student/classes/class-2");
    expect(screen.getByText(/open class/i)).toBeInTheDocument();
  });

  it("class-only doc (no sessionId) links to /student/classes/{classId}", async () => {
    apiMock.mockResolvedValueOnce([
      {
        id: "doc-class-only",
        ...baseDoc,
        sessionId: null,
        classId: "class-3",
        sessionStatus: null,
      },
    ]);

    await renderPage();

    const link = screen.getByRole("link");
    expect(link.getAttribute("href")).toBe("/student/classes/class-3");
  });

  it("dangling sessionId + null classId → card NOT wrapped in <Link>", async () => {
    // The session row was hard-deleted; LEFT JOIN sessions returns null
    // for both class_id and status. The card must NOT be clickable
    // (would produce an empty /student/classes/ URL otherwise).
    apiMock.mockResolvedValueOnce([
      {
        id: "doc-dangling",
        ...baseDoc,
        sessionId: "sess-deleted",
        classId: null,
        sessionStatus: null,
      },
    ]);

    await renderPage();

    expect(screen.queryByRole("link")).toBeNull();
    // The plainText snippet still renders.
    expect(screen.getByText(/x = 1/)).toBeInTheDocument();
  });

  it("no metadata at all → card NOT wrapped in <Link>", async () => {
    apiMock.mockResolvedValueOnce([
      {
        id: "doc-orphan",
        ...baseDoc,
        sessionId: null,
        classId: null,
        sessionStatus: null,
      },
    ]);

    await renderPage();

    expect(screen.queryByRole("link")).toBeNull();
  });

  it("renders empty state when there are no documents", async () => {
    apiMock.mockResolvedValueOnce([]);
    await renderPage();
    expect(screen.getByText(/no code saved yet/i)).toBeInTheDocument();
    expect(screen.queryByRole("link")).toBeNull();
  });
});
