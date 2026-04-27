// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn() }),
}));

import { JoinClassDialog } from "@/components/student/join-class-dialog";

const JOINED_CLASS = { id: "class-123", title: "Algebra" };

function openDialog() {
  fireEvent.click(screen.getByRole("button", { name: /Join a Class/i }));
}

function typeCode(code: string) {
  const input = screen.getByLabelText(/Enter join code/i);
  fireEvent.change(input, { target: { value: code } });
}

function submit() {
  fireEvent.click(screen.getByRole("button", { name: /^Join$/i }));
}

describe("JoinClassDialog", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("happy path: closes dialog after join + verification", async () => {
    fetchMock
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ class: JOINED_CLASS }), { status: 200 })
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify([JOINED_CLASS]), { status: 200 })
      );

    render(<JoinClassDialog />);
    openDialog();
    typeCode("ABCD1234");
    submit();

    await waitFor(() => {
      expect(screen.queryByLabelText(/Enter join code/i)).not.toBeInTheDocument();
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/classes/join");
    expect(fetchMock.mock.calls[1][0]).toBe("/api/classes/mine");
  });

  // The review-002 bug: join "succeeds" but the class isn't visible to the
  // student (auth identity drift). Dialog must stay open with a clear error.
  it("keeps dialog open and shows an error when class is not in /mine after join", async () => {
    fetchMock
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ class: JOINED_CLASS }), { status: 200 })
      )
      .mockResolvedValueOnce(
        // The student's class list does NOT include the joined class.
        new Response(JSON.stringify([]), { status: 200 })
      );

    render(<JoinClassDialog />);
    openDialog();
    typeCode("ABCD1234");
    submit();

    await waitFor(() => {
      expect(screen.getByText(/isn't showing up/i)).toBeInTheDocument();
    });
    expect(screen.getByLabelText(/Enter join code/i)).toBeInTheDocument();
  });

  it("surfaces the server error when join itself fails", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "Invalid or inactive join code" }), {
        status: 404,
      })
    );

    render(<JoinClassDialog />);
    openDialog();
    typeCode("BADCODE0");
    submit();

    await waitFor(() => {
      expect(screen.getByText(/Invalid or inactive join code/i)).toBeInTheDocument();
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText(/Enter join code/i)).toBeInTheDocument();
  });

  it("handles a join response that omits the class id", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ class: null }), { status: 200 })
    );

    render(<JoinClassDialog />);
    openDialog();
    typeCode("ABCD1234");
    submit();

    await waitFor(() => {
      expect(screen.getByText(/didn't return a class/i)).toBeInTheDocument();
    });
    // Verification fetch was NOT made because join didn't give us an id to verify.
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("handles a verification fetch that errors", async () => {
    fetchMock
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ class: JOINED_CLASS }), { status: 200 })
      )
      .mockResolvedValueOnce(
        new Response("server error", { status: 500 })
      );

    render(<JoinClassDialog />);
    openDialog();
    typeCode("ABCD1234");
    submit();

    await waitFor(() => {
      expect(screen.getByText(/couldn't verify your class list/i)).toBeInTheDocument();
    });
    expect(screen.getByLabelText(/Enter join code/i)).toBeInTheDocument();
  });
});
