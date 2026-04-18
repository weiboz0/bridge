// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { useAutosaveAttempt } from "@/lib/problem/use-autosave-attempt";
import type { Attempt } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";

function makeAttempt(overrides: Partial<Attempt> = {}): Attempt {
  const now = new Date().toISOString();
  return {
    id: "a1",
    problemId: "p1",
    userId: "u1",
    title: "Untitled",
    language: "python",
    plainText: "start",
    createdAt: now,
    updatedAt: now,
    ...overrides,
  };
}

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn());
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useAutosaveAttempt", () => {
  it("uses starter code when no attempt exists", () => {
    const { result } = renderHook(() =>
      useAutosaveAttempt({
        problemId: "p1",
        initialAttempt: null,
        starterCode: "print('hi')",
        language: "python",
      })
    );
    expect(result.current.code).toBe("print('hi')");
    expect(result.current.attempt).toBeNull();
  });

  it("uses the initial attempt's plainText when one exists", () => {
    const attempt = makeAttempt({ plainText: "v1" });
    const { result } = renderHook(() =>
      useAutosaveAttempt({
        problemId: "p1",
        initialAttempt: attempt,
        starterCode: "start",
        language: "python",
      })
    );
    expect(result.current.code).toBe("v1");
    expect(result.current.attempt?.id).toBe("a1");
  });

  it("does not POST when code equals starterCode", async () => {
    const fetchMock = vi.mocked(fetch);
    const { result } = renderHook(() =>
      useAutosaveAttempt({
        problemId: "p1",
        initialAttempt: null,
        starterCode: "same",
        language: "python",
      })
    );
    act(() => {
      result.current.setCode("same");
    });
    await act(async () => {
      vi.advanceTimersByTime(1000);
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("POSTs a new attempt on the first real edit", async () => {
    const fetchMock = vi.mocked(fetch);
    const created = makeAttempt({ plainText: "hello" });
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(created), { status: 201 }));

    const { result } = renderHook(() =>
      useAutosaveAttempt({
        problemId: "p1",
        initialAttempt: null,
        starterCode: "start",
        language: "python",
        debounceMs: 200,
      })
    );
    act(() => {
      result.current.setCode("hello");
    });
    expect(result.current.saveState).toBe("pending");

    await act(async () => {
      vi.advanceTimersByTime(200);
      await Promise.resolve();
    });

    await waitFor(() => expect(result.current.attempt?.id).toBe("a1"));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/problems/p1/attempts");
    expect((init as RequestInit).method).toBe("POST");
    const body = JSON.parse(((init as RequestInit).body as string)!);
    expect(body).toEqual({ plainText: "hello", language: "python" });
  });

  it("coalesces rapid keystrokes into a single save", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(makeAttempt({ plainText: "abc" })), { status: 201 })
    );

    const { result } = renderHook(() =>
      useAutosaveAttempt({
        problemId: "p1",
        initialAttempt: null,
        starterCode: "",
        language: "python",
        debounceMs: 100,
      })
    );
    act(() => {
      result.current.setCode("a");
      result.current.setCode("ab");
      result.current.setCode("abc");
    });

    await act(async () => {
      vi.advanceTimersByTime(100);
      await Promise.resolve();
    });

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  });

  it("PATCHes subsequent saves once an attempt exists", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(makeAttempt({ plainText: "v1" })), { status: 201 })
    );
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(makeAttempt({ plainText: "v2" })), { status: 200 })
    );

    const { result } = renderHook(() =>
      useAutosaveAttempt({
        problemId: "p1",
        initialAttempt: null,
        starterCode: "",
        language: "python",
        debounceMs: 100,
      })
    );
    act(() => {
      result.current.setCode("v1");
    });
    await act(async () => {
      vi.advanceTimersByTime(100);
      await Promise.resolve();
    });
    await waitFor(() => expect(result.current.attempt?.id).toBe("a1"));

    act(() => {
      result.current.setCode("v2");
    });
    await act(async () => {
      vi.advanceTimersByTime(100);
      await Promise.resolve();
    });

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    const [url2, init2] = fetchMock.mock.calls[1]!;
    expect(url2).toBe("/api/attempts/a1");
    expect((init2 as RequestInit).method).toBe("PATCH");
  });

  it("moves saveState to 'error' on failure, leaves attempt unchanged", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockResolvedValueOnce(new Response("boom", { status: 500 }));

    const { result } = renderHook(() =>
      useAutosaveAttempt({
        problemId: "p1",
        initialAttempt: null,
        starterCode: "",
        language: "python",
        debounceMs: 100,
      })
    );
    act(() => {
      result.current.setCode("x");
    });
    await act(async () => {
      vi.advanceTimersByTime(100);
      await Promise.resolve();
    });

    await waitFor(() => expect(result.current.saveState).toBe("error"));
    expect(result.current.attempt).toBeNull();
  });

  it("setAttempt updates active attempt and resets code to its plainText", async () => {
    const { result } = renderHook(() =>
      useAutosaveAttempt({
        problemId: "p1",
        initialAttempt: makeAttempt({ id: "a1", plainText: "first" }),
        starterCode: "",
        language: "python",
      })
    );
    expect(result.current.code).toBe("first");

    const a2 = makeAttempt({ id: "a2", plainText: "second" });
    act(() => {
      result.current.setAttempt(a2);
    });
    expect(result.current.attempt?.id).toBe("a2");
    expect(result.current.code).toBe("second");
  });

  it("does not stomp the active attempt when the user switches mid-save", async () => {
    // Regression for a race: if a PATCH is in flight and the user switches
    // attempts before the response arrives, the save's setAttemptState must
    // NOT overwrite the newly-active attempt.
    const fetchMock = vi.mocked(fetch);

    const a1 = makeAttempt({ id: "a1", plainText: "v1" });
    // Build a deferred response for the PATCH so we can control its timing.
    let resolvePatch!: (v: Response) => void;
    const patchPromise = new Promise<Response>((r) => { resolvePatch = r; });
    fetchMock.mockReturnValueOnce(patchPromise);

    const { result } = renderHook(() =>
      useAutosaveAttempt({
        problemId: "p1",
        initialAttempt: a1,
        starterCode: "",
        language: "python",
        debounceMs: 50,
      })
    );

    // Edit → queues save for a1.
    act(() => { result.current.setCode("v1-edited"); });
    await act(async () => {
      vi.advanceTimersByTime(50);
      await Promise.resolve();
    });
    // Save is in flight (awaiting our deferred response).

    // User switches to a different attempt while PATCH is in flight.
    const a2 = makeAttempt({
      id: "a2",
      plainText: "second attempt",
      updatedAt: new Date(Date.now() + 1000).toISOString(),
    });
    act(() => { result.current.setAttempt(a2); });
    expect(result.current.attempt?.id).toBe("a2");

    // PATCH response arrives (points at a1).
    await act(async () => {
      resolvePatch(
        new Response(JSON.stringify({ ...a1, plainText: "v1-edited" }), { status: 200 })
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    // The active attempt must still be a2. The PATCH landed in the DB (correct)
    // but did not stomp the local state.
    expect(result.current.attempt?.id).toBe("a2");
    expect(result.current.code).toBe("second attempt");
  });

  it("flush immediately saves any pending edit", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(makeAttempt({ plainText: "flushed" })), { status: 201 })
    );

    const { result } = renderHook(() =>
      useAutosaveAttempt({
        problemId: "p1",
        initialAttempt: null,
        starterCode: "",
        language: "python",
        debounceMs: 5000, // deliberately long
      })
    );
    act(() => {
      result.current.setCode("flushed");
    });
    expect(result.current.saveState).toBe("pending");

    await act(async () => {
      await result.current.flush();
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(result.current.attempt?.id).toBe("a1"));
  });
});
