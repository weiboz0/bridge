// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MyTestCaseEditor } from "@/components/problem/my-test-case-editor";
import type { TestCase } from "@/app/(portal)/student/classes/[id]/problems/[problemId]/page";

function makeCase(overrides: Partial<TestCase> = {}): TestCase {
  return {
    id: "c1",
    problemId: "p1",
    ownerId: "u1",
    name: "Existing",
    stdin: "1 2",
    expectedStdout: "3",
    isExample: false,
    order: 0,
    ...overrides,
  };
}

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn());
});
afterEach(() => {
  vi.restoreAllMocks();
});

describe("MyTestCaseEditor — create mode", () => {
  it("POSTs a private case with isCanonical=false", async () => {
    const fetchMock = vi.mocked(fetch);
    const saved = makeCase({ name: "Neg", stdin: "-1", expectedStdout: "-2" });
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(saved), { status: 201 }));
    const onSaved = vi.fn();

    render(
      <MyTestCaseEditor problemId="p1" onSaved={onSaved} onCancel={() => {}} />
    );

    fireEvent.change(screen.getByPlaceholderText("e.g., Negatives"), {
      target: { value: "Neg" },
    });
    fireEvent.change(screen.getByPlaceholderText("e.g., -1 -2"), {
      target: { value: "-1" },
    });
    fireEvent.change(screen.getByPlaceholderText("Leave empty for no check"), {
      target: { value: "-2" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(saved));
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/problems/p1/test-cases");
    const body = JSON.parse(((init as RequestInit).body as string)!);
    expect(body).toMatchObject({
      name: "Neg",
      stdin: "-1",
      expectedStdout: "-2",
      isExample: false,
      isCanonical: false,
    });
  });

  it("sends expectedStdout: null when field is empty", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(makeCase()), { status: 201 }));

    render(
      <MyTestCaseEditor problemId="p1" onSaved={() => {}} onCancel={() => {}} />
    );
    fireEvent.change(screen.getByPlaceholderText("e.g., -1 -2"), {
      target: { value: "1" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const body = JSON.parse(((fetchMock.mock.calls[0]![1] as RequestInit).body as string)!);
    expect(body.expectedStdout).toBeNull();
  });
});

describe("MyTestCaseEditor — edit mode", () => {
  it("PATCHes /api/test-cases/{id} with the case id", async () => {
    const fetchMock = vi.mocked(fetch);
    const updated = makeCase({ name: "Renamed" });
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(updated), { status: 200 }));
    const onSaved = vi.fn();

    render(
      <MyTestCaseEditor
        problemId="p1"
        initial={makeCase()}
        onSaved={onSaved}
        onCancel={() => {}}
      />
    );

    const nameInput = screen.getByDisplayValue("Existing");
    fireEvent.change(nameInput, { target: { value: "Renamed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(updated));
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/test-cases/c1");
    expect((init as RequestInit).method).toBe("PATCH");
  });
});

describe("MyTestCaseEditor — error handling", () => {
  it("shows server error message on non-ok response", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "stdin required" }), { status: 400 })
    );
    render(
      <MyTestCaseEditor problemId="p1" onSaved={() => {}} onCancel={() => {}} />
    );
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    await waitFor(() => expect(screen.getByText("stdin required")).toBeInTheDocument());
  });
});
