// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import type { JSONContent } from "@tiptap/react";

// Mock fetch globally so ProblemRefNodeView's useEffect doesn't throw
beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({ title: "Test Problem", difficulty: "easy", tags: [] }),
      { status: 200 }
    )
  ));
  vi.stubGlobal("prompt", vi.fn());
});

afterEach(() => {
  vi.restoreAllMocks();
});

import { TeachingUnitEditor } from "@/components/editor/tiptap/teaching-unit-editor";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function emptyDoc(): JSONContent {
  return { type: "doc", content: [] };
}

function proseParagraphDoc(): JSONContent {
  return {
    type: "doc",
    content: [
      {
        type: "paragraph",
        content: [{ type: "text", text: "Hello world" }],
      },
    ],
  };
}

function problemRefDoc(): JSONContent {
  return {
    type: "doc",
    content: [
      {
        type: "problem-ref",
        attrs: {
          id: "test-1",
          problemId: "fake-uuid",
          pinnedRevision: null,
          visibility: "always",
          overrideStarter: null,
        },
      },
    ],
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("TeachingUnitEditor", () => {
  it("1. renders without crashing when mounted with an empty initialDoc", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<TeachingUnitEditor initialDoc={emptyDoc()} onSave={onSave} />);

    // The toolbar and editor container must be present
    expect(screen.getByRole("button", { name: /insert problem/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save/i })).toBeInTheDocument();
  });

  it("2. renders with provided content (prose block appears)", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<TeachingUnitEditor initialDoc={proseParagraphDoc()} onSave={onSave} />);

    // The editor should render the paragraph text into the DOM
    await waitFor(() => {
      expect(screen.getByText("Hello world")).toBeInTheDocument();
    });
  });

  it("3. Save button calls onSave with the editor JSON", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<TeachingUnitEditor initialDoc={proseParagraphDoc()} onSave={onSave} />);

    const saveBtn = screen.getByRole("button", { name: /save/i });
    await act(async () => {
      fireEvent.click(saveBtn);
    });

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));

    const [savedDoc] = onSave.mock.calls[0] as [JSONContent];
    // The JSON must be a valid Tiptap doc object
    expect(savedDoc).toMatchObject({ type: "doc" });
    expect(Array.isArray(savedDoc.content)).toBe(true);
  });

  it("4. problem-ref node renders a loading/card placeholder in the editor", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<TeachingUnitEditor initialDoc={problemRefDoc()} onSave={onSave} />);

    // The ProblemRefNodeView starts in a "loading" state and then (on fetch
    // success) shows a card.  Either state produces a `.problem-ref-node`
    // wrapper element.
    await waitFor(() => {
      const node = document.querySelector(".problem-ref-node");
      expect(node).toBeInTheDocument();
    });
  });

  it("5. problem-ref blocks with an empty id are assigned a nanoid on creation", async () => {
    // A problem-ref whose attrs.id is an empty string should have an id
    // injected by assignMissingTopLevelNodeIds during the editor's onCreate.
    const docWithEmptyId: JSONContent = {
      type: "doc",
      content: [
        {
          type: "problem-ref",
          attrs: {
            id: "",
            problemId: "fake-uuid",
            pinnedRevision: null,
            visibility: "always",
            overrideStarter: null,
          },
        },
      ],
    };

    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<TeachingUnitEditor initialDoc={docWithEmptyId} onSave={onSave} />);

    // Click save to get the processed JSON back from the editor.
    // Wait for the editor to mount before clicking.
    const saveBtn = await screen.findByRole("button", { name: /save/i });
    await act(async () => {
      fireEvent.click(saveBtn);
    });

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));

    const [savedDoc] = onSave.mock.calls[0] as [JSONContent];
    const firstBlock = savedDoc.content?.[0];
    expect(firstBlock).toBeDefined();
    expect(firstBlock?.type).toBe("problem-ref");

    // After onCreate, the id must be a non-empty string injected by nanoid
    const id = (firstBlock as any)?.attrs?.id;
    expect(typeof id).toBe("string");
    expect(id.length).toBeGreaterThan(0);
  });
});
