// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { SessionRow, type SessionItem } from "@/app/(portal)/teacher/sessions/page";

const liveSession: SessionItem = {
  id: "live-1",
  title: "Period 3",
  classId: "class-1",
  status: "live",
  startedAt: new Date("2026-04-27T10:00:00Z").toISOString(),
  endedAt: null,
};

const endedSession: SessionItem = {
  ...liveSession,
  id: "ended-1",
  title: "Period 1 (yesterday)",
  status: "ended",
  endedAt: new Date("2026-04-26T11:00:00Z").toISOString(),
};

describe("SessionRow (plan 043 phase 2.1)", () => {
  it("live session renders as a Link to the workspace", () => {
    render(<SessionRow session={liveSession} />);
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/teacher/sessions/live-1");
    expect(screen.getByText("Period 3")).toBeInTheDocument();
    expect(screen.getByText("live")).toBeInTheDocument();
  });

  it("ended session renders as plain text (no link to live workspace)", () => {
    render(<SessionRow session={endedSession} />);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByText("Period 1 (yesterday)")).toBeInTheDocument();
    expect(screen.getByText("ended")).toBeInTheDocument();
  });
});
