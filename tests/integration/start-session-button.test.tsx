// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

// Mock next/navigation's useRouter so the component can call .push()
// without a real Next runtime.
const pushMock = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}));

import { StartSessionButton } from "@/components/teacher/start-session-button";

beforeEach(() => {
  pushMock.mockReset();
  // jsdom's fetch is undefined by default — the test sets a per-test mock.
  // Reset between tests so a leftover mock doesn't leak.
  (globalThis as any).fetch = undefined;
});

function mockFetch(...responses: { status: number; body: any }[]) {
  let i = 0;
  (globalThis as any).fetch = vi.fn(async () => {
    const r = responses[i++];
    return new Response(JSON.stringify(r.body), {
      status: r.status,
      headers: { "Content-Type": "application/json" },
    });
  });
  return (globalThis as any).fetch as ReturnType<typeof vi.fn>;
}

describe("StartSessionButton — Plan 047 unlinked-topics guard", () => {
  it("creates session normally on 201 (no guard, no dialog)", async () => {
    const fetchMock = mockFetch({ status: 201, body: { id: "s-1" } });
    render(<StartSessionButton classId="c-1" />);

    fireEvent.click(screen.getByRole("button", { name: /start live session/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/teacher/sessions/s-1"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("renders strong dialog on 422 code=all_topics_unlinked, then re-POSTs with confirm flag on Start anyway", async () => {
    const fetchMock = mockFetch(
      {
        status: 422,
        body: {
          error: "everything unlinked",
          code: "all_topics_unlinked",
          unlinkedTopicTitles: ["A", "B"],
        },
      },
      { status: 201, body: { id: "s-2" } }
    );
    render(<StartSessionButton classId="c-1" />);

    fireEvent.click(screen.getByRole("button", { name: /start live session/i }));

    await waitFor(() =>
      expect(screen.getByRole("dialog", { name: /no teaching units linked/i })).toBeInTheDocument()
    );
    // Both unlinked topic titles render in the list.
    expect(screen.getByText("A")).toBeInTheDocument();
    expect(screen.getByText("B")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /start anyway/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    const secondCallBody = JSON.parse(fetchMock.mock.calls[1][1].body);
    expect(secondCallBody.confirmUnlinkedTopics).toBe(true);
    expect(secondCallBody.classId).toBe("c-1");
    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/teacher/sessions/s-2"));
  });

  it("renders softer dialog on 422 code=some_topics_unlinked", async () => {
    mockFetch({
      status: 422,
      body: {
        error: "some unlinked",
        code: "some_topics_unlinked",
        unlinkedTopicTitles: ["Only One"],
      },
    });
    render(<StartSessionButton classId="c-1" />);
    fireEvent.click(screen.getByRole("button", { name: /start live session/i }));

    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: /some focus areas have no material/i })
      ).toBeInTheDocument()
    );
    expect(screen.getByText("Only One")).toBeInTheDocument();
  });

  it("Cancel dismisses the dialog without re-POSTing", async () => {
    const fetchMock = mockFetch({
      status: 422,
      body: {
        error: "some",
        code: "some_topics_unlinked",
        unlinkedTopicTitles: ["X"],
      },
    });
    render(<StartSessionButton classId="c-1" />);
    fireEvent.click(screen.getByRole("button", { name: /start live session/i }));

    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /^cancel$/i }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(fetchMock).toHaveBeenCalledTimes(1); // no re-POST
    expect(pushMock).not.toHaveBeenCalled();
  });

  it("treats non-422 errors as regular error path (no dialog)", async () => {
    mockFetch({ status: 500, body: { error: "server died" } });
    render(<StartSessionButton classId="c-1" />);
    fireEvent.click(screen.getByRole("button", { name: /start live session/i }));

    await waitFor(() => expect(screen.getByText("server died")).toBeInTheDocument());
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("422 without a recognized guard code falls through to error path", async () => {
    mockFetch({
      status: 422,
      body: { error: "validation failed", code: "something_else" },
    });
    render(<StartSessionButton classId="c-1" />);
    fireEvent.click(screen.getByRole("button", { name: /start live session/i }));

    await waitFor(() => expect(screen.getByText("validation failed")).toBeInTheDocument());
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
