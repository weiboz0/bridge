// @vitest-environment jsdom
//
// Plan 090 phase 5 — /sessions/[id] room dispatcher (SessionRoomPage).
// Fetches teacher-page first: 200 renders TeacherDashboard (the host, or an
// authorized instructor/admin); a 403/404 falls back to student-page and
// renders StudentSession. Neither authorized -> notFound(). Also covers the
// class-less returnPath override (Go hardcodes "/teacher"/"/student", which
// this route rewrites to "/sessions" so a class-less viewer isn't bounced by
// a portal role gate they may not hold).

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/lib/api-client", () => ({
  api: vi.fn(),
}));

class NotFoundError extends Error {
  constructor() {
    super("NEXT_NOT_FOUND");
    this.name = "NotFoundError";
  }
}
class RedirectError extends Error {
  constructor(destination: string) {
    super(`NEXT_REDIRECT:${destination}`);
    this.name = "RedirectError";
  }
}
vi.mock("next/navigation", () => ({
  notFound: vi.fn(() => {
    throw new NotFoundError();
  }),
  redirect: vi.fn((destination: string) => {
    throw new RedirectError(destination);
  }),
}));

vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}));

vi.mock("@/components/session/teacher/teacher-dashboard", () => ({
  TeacherDashboard: (props: { sessionId: string }) => (
    <div
      data-testid="teacher-dashboard-stub"
      data-session-id={props.sessionId}
    />
  ),
}));

vi.mock("@/components/session/student/student-session", () => ({
  StudentSession: (props: {
    sessionId: string;
    classId: string | null;
    returnPath?: string;
  }) => (
    <div
      data-testid="student-session-stub"
      data-session-id={props.sessionId}
      data-class-id={props.classId ?? ""}
      data-return-path={props.returnPath ?? ""}
    />
  ),
}));

import SessionRoomPage from "@/app/(portal)/sessions/[id]/page";
import { api } from "@/lib/api-client";
import { ApiError } from "@/lib/api-error";
import { notFound, redirect } from "next/navigation";

const mockedApi = vi.mocked(api);
const mockedNotFound = vi.mocked(notFound);
const mockedRedirect = vi.mocked(redirect);

const SESSION_ID = "11111111-1111-4111-8111-111111111111";

async function renderRoom() {
  const element = await SessionRoomPage({ params: Promise.resolve({ id: SESSION_ID }) });
  render(element as React.ReactElement);
}

beforeEach(() => {
  mockedApi.mockReset();
  mockedNotFound.mockClear();
  mockedRedirect.mockClear();
});

describe("SessionRoomPage — plan 090 phase 5", () => {
  it("renders TeacherDashboard when teacher-page returns 200 (host)", async () => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === `/api/sessions/${SESSION_ID}/teacher-page`) {
        return {
          session: {
            id: SESSION_ID,
            classId: null,
            teacherId: "host-1",
            status: "live",
            inviteToken: "tok-abc",
            inviteExpiresAt: null,
          },
          classId: null,
          returnPath: "/teacher",
          editorMode: "python",
          courseTopics: [],
        };
      }
      throw new Error(`Unexpected api() call: ${path}`);
    });

    await renderRoom();

    const stub = screen.getByTestId("teacher-dashboard-stub");
    expect(stub).toBeInTheDocument();
    expect(stub).toHaveAttribute("data-session-id", SESSION_ID);
    expect(screen.queryByTestId("student-session-stub")).not.toBeInTheDocument();
  });

  it("falls back to StudentSession when teacher-page 403s and student-page succeeds", async () => {
    mockedApi.mockImplementation(async (path: string, opts?: { method?: string }) => {
      if (path === `/api/sessions/${SESSION_ID}/teacher-page`) {
        throw new ApiError(403, "Not authorized to view this session as teacher");
      }
      if (path === `/api/sessions/${SESSION_ID}/student-page`) {
        return {
          session: { id: SESSION_ID, classId: null, status: "live" },
          classId: null,
          returnPath: "/student",
          editorMode: "python",
        };
      }
      if (path === `/api/sessions/${SESSION_ID}/join` && opts?.method === "POST") {
        return {};
      }
      throw new Error(`Unexpected api() call: ${path}`);
    });

    await renderRoom();

    const stub = screen.getByTestId("student-session-stub");
    expect(stub).toBeInTheDocument();
    expect(stub).toHaveAttribute("data-session-id", SESSION_ID);
    expect(stub).toHaveAttribute("data-return-path", "/sessions");
    expect(screen.queryByTestId("teacher-dashboard-stub")).not.toBeInTheDocument();

    // Side-effect join POST fired.
    expect(
      mockedApi.mock.calls.some(
        ([path, opts]) => path === `/api/sessions/${SESSION_ID}/join` && (opts as { method?: string } | undefined)?.method === "POST"
      )
    ).toBe(true);
  });

  it("calls notFound() when neither teacher-page nor student-page authorize the caller", async () => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === `/api/sessions/${SESSION_ID}/teacher-page`) {
        throw new ApiError(403, "Not authorized");
      }
      if (path === `/api/sessions/${SESSION_ID}/student-page`) {
        throw new ApiError(403, "Not enrolled in this session's class");
      }
      throw new Error(`Unexpected api() call: ${path}`);
    });

    await expect(renderRoom()).rejects.toThrow("NEXT_NOT_FOUND");
    expect(mockedNotFound).toHaveBeenCalledTimes(1);
  });

  it("redirects an ended former participant to the archive without joining the live room", async () => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === `/api/sessions/${SESSION_ID}/teacher-page`) {
        throw new ApiError(403, "Not a teacher");
      }
      if (path === `/api/sessions/${SESSION_ID}/student-page`) {
        throw new ApiError(404, "Session ended");
      }
      throw new Error(`Unexpected api() call: ${path}`);
    });

    await expect(renderRoom()).rejects.toThrow(`NEXT_REDIRECT:/sessions/${SESSION_ID}/whiteboards`);
    expect(mockedRedirect).toHaveBeenCalledWith(`/sessions/${SESSION_ID}/whiteboards`);
    expect(
      mockedApi.mock.calls.some(
        ([path]) => path === `/api/sessions/${SESSION_ID}/join`,
      ),
    ).toBe(false);
  });

  it("keeps a genuinely missing session as a 404 instead of redirecting to an archive that cannot exist", async () => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === `/api/sessions/${SESSION_ID}/teacher-page`) {
        throw new ApiError(403, "Not a teacher");
      }
      if (path === `/api/sessions/${SESSION_ID}/student-page`) {
        throw new ApiError(404, "Not found", { error: "Not found" });
      }
      throw new Error(`Unexpected api() call: ${path}`);
    });

    await expect(renderRoom()).rejects.toThrow("NEXT_NOT_FOUND");
    expect(mockedNotFound).toHaveBeenCalledTimes(1);
    expect(mockedRedirect).not.toHaveBeenCalled();
  });

  it("renders the 'Session ended' notice instead of TeacherDashboard for a non-live session", async () => {
    mockedApi.mockImplementation(async (path: string) => {
      if (path === `/api/sessions/${SESSION_ID}/teacher-page`) {
        return {
          session: {
            id: SESSION_ID,
            classId: null,
            teacherId: "host-1",
            status: "ended",
          },
          classId: null,
          returnPath: "/teacher",
          editorMode: "python",
          courseTopics: [],
        };
      }
      throw new Error(`Unexpected api() call: ${path}`);
    });

    await renderRoom();

    expect(screen.getByText("Session ended")).toBeInTheDocument();
    expect(screen.queryByTestId("teacher-dashboard-stub")).not.toBeInTheDocument();
  });
});
