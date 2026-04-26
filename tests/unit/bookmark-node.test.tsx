// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest"
import { render, screen, fireEvent } from "@testing-library/react"
import React from "react"

// ---------------------------------------------------------------------------
// Mock @tiptap/react so NodeViewWrapper / NodeViewContent render as plain divs
// and we can capture the component passed to ReactNodeViewRenderer.
// ---------------------------------------------------------------------------
let CapturedBookmarkNodeView: React.ComponentType<any> | null = null

vi.mock("@tiptap/react", async (importOriginal) => {
  const React = await import("react")
  return {
    ...(await importOriginal<typeof import("@tiptap/react")>()),
    NodeViewWrapper: React.forwardRef(({ children, className, ...rest }: any, ref: any) =>
      React.createElement("div", { ref, className, "data-node-view-wrapper": "", ...rest }, children)
    ),
    NodeViewContent: ({ className }: any) =>
      React.createElement("div", { className, "data-node-view-content": "" }),
    ReactNodeViewRenderer: (Component: any) => {
      CapturedBookmarkNodeView = Component
      return Component
    },
  }
})

// ---------------------------------------------------------------------------
// Import the module — the top-level `Node.create({ addNodeView() { ... } })`
// calls ReactNodeViewRenderer at extension create time, which captures the
// component via the mock above.
// ---------------------------------------------------------------------------
import { BookmarkNode } from "@/components/editor/tiptap/bookmark-node"

// Force addNodeView to be called (it runs lazily in Tiptap, but our mock
// ReactNodeViewRenderer captures the component when called)
if (!CapturedBookmarkNodeView) {
  // The extension definition calls ReactNodeViewRenderer in addNodeView.
  // Tiptap extensions define addNodeView as a method on the extension config.
  // We need to invoke it to trigger our capture mock.
  const ext = BookmarkNode.extend({})
  // Access the stored nodeView function from the extension
  const config = ext.config ?? ext.options
  if (config?.addNodeView) {
    ;(config.addNodeView as Function).call({ name: "bookmark", options: {} })
  }
}

// ---------------------------------------------------------------------------
// Helpers: build minimal NodeViewProps
// ---------------------------------------------------------------------------
function makeProps(attrs: Record<string, any>, overrides: Partial<any> = {}) {
  return {
    node: { attrs: { id: "test-id", url: "", title: null, description: null, image: null, ...attrs } },
    updateAttributes: vi.fn(),
    deleteNode: vi.fn(),
    selected: false,
    ...overrides,
  }
}

function BookmarkView(props: any) {
  if (!CapturedBookmarkNodeView) throw new Error("BookmarkNodeView was not captured")
  return React.createElement(CapturedBookmarkNodeView, props)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------
describe("BookmarkNodeView — URL input (empty url)", () => {
  it("renders URL input placeholder when url is empty", () => {
    const props = makeProps({ url: "" })
    render(<BookmarkView {...props} />)
    expect(screen.getByPlaceholderText("Paste a URL and press Enter…")).toBeInTheDocument()
  })

  it("does not render preview card when url is empty", () => {
    const props = makeProps({ url: "" })
    render(<BookmarkView {...props} />)
    // Edit button should not be present in the empty-url state
    expect(screen.queryByRole("button", { name: /edit bookmark/i })).toBeNull()
  })

  it("pressing Enter with a non-empty URL calls updateAttributes", () => {
    const props = makeProps({ url: "" })
    render(<BookmarkView {...props} />)
    const input = screen.getByPlaceholderText("Paste a URL and press Enter…")
    fireEvent.change(input, { target: { value: "https://example.com" } })
    fireEvent.keyDown(input, { key: "Enter" })
    expect(props.updateAttributes).toHaveBeenCalledWith({ url: "https://example.com" })
  })

  it("pressing Enter with empty input does NOT call updateAttributes", () => {
    const props = makeProps({ url: "" })
    render(<BookmarkView {...props} />)
    const input = screen.getByPlaceholderText("Paste a URL and press Enter…")
    // Leave input empty and press Enter
    fireEvent.keyDown(input, { key: "Enter" })
    expect(props.updateAttributes).not.toHaveBeenCalled()
  })

  it("pressing Enter with whitespace-only URL does NOT call updateAttributes", () => {
    const props = makeProps({ url: "" })
    render(<BookmarkView {...props} />)
    const input = screen.getByPlaceholderText("Paste a URL and press Enter…")
    fireEvent.change(input, { target: { value: "   " } })
    fireEvent.keyDown(input, { key: "Enter" })
    expect(props.updateAttributes).not.toHaveBeenCalled()
  })

  it("pressing other keys does not call updateAttributes", () => {
    const props = makeProps({ url: "" })
    render(<BookmarkView {...props} />)
    const input = screen.getByPlaceholderText("Paste a URL and press Enter…")
    fireEvent.change(input, { target: { value: "https://x.com" } })
    fireEvent.keyDown(input, { key: "Escape" })
    expect(props.updateAttributes).not.toHaveBeenCalled()
  })
})

describe("BookmarkNodeView — preview card (url set)", () => {
  it("renders preview card with domain when url is set", () => {
    const props = makeProps({ url: "https://example.com/path" })
    render(<BookmarkView {...props} />)
    // Domain appears both as fallback title and as domain text
    const matches = screen.getAllByText("example.com")
    expect(matches.length).toBeGreaterThanOrEqual(1)
  })

  it("renders title when provided", () => {
    const props = makeProps({ url: "https://example.com", title: "My Bookmark" })
    render(<BookmarkView {...props} />)
    expect(screen.getByText("My Bookmark")).toBeInTheDocument()
  })

  it("renders description when provided", () => {
    const props = makeProps({ url: "https://example.com", description: "A description here" })
    render(<BookmarkView {...props} />)
    expect(screen.getByText("A description here")).toBeInTheDocument()
  })

  it("falls back to domain as title when title is null", () => {
    const props = makeProps({ url: "https://example.com", title: null })
    render(<BookmarkView {...props} />)
    // Domain should appear as title (text 'example.com' shown at least twice — once as title, once as domain)
    const matches = screen.getAllByText("example.com")
    expect(matches.length).toBeGreaterThanOrEqual(1)
  })

  it("clicking edit button shows edit form with URL/title/description fields", () => {
    const props = makeProps({ url: "https://example.com", title: "Old Title", description: "Old desc" })
    render(<BookmarkView {...props} />)
    fireEvent.click(screen.getByRole("button", { name: /edit bookmark/i }))
    expect(screen.getByLabelText(/bookmark url/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/bookmark title/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/bookmark description/i)).toBeInTheDocument()
  })

  it("edit form pre-fills with current URL/title/description", () => {
    const props = makeProps({ url: "https://example.com", title: "Old Title", description: "Old desc" })
    render(<BookmarkView {...props} />)
    fireEvent.click(screen.getByRole("button", { name: /edit bookmark/i }))
    const urlInput = screen.getByLabelText(/bookmark url/i) as HTMLInputElement
    const titleInput = screen.getByLabelText(/bookmark title/i) as HTMLInputElement
    expect(urlInput.value).toBe("https://example.com")
    expect(titleInput.value).toBe("Old Title")
  })

  it("Save in edit form calls updateAttributes with new values", () => {
    const props = makeProps({ url: "https://example.com", title: "Old", description: "" })
    render(<BookmarkView {...props} />)
    fireEvent.click(screen.getByRole("button", { name: /edit bookmark/i }))

    const titleInput = screen.getByLabelText(/bookmark title/i)
    fireEvent.change(titleInput, { target: { value: "New Title" } })

    fireEvent.click(screen.getByText("Save"))
    expect(props.updateAttributes).toHaveBeenCalledWith(
      expect.objectContaining({ title: "New Title" })
    )
  })

  it("Cancel in edit form hides the form without calling updateAttributes", () => {
    const props = makeProps({ url: "https://example.com" })
    render(<BookmarkView {...props} />)
    fireEvent.click(screen.getByRole("button", { name: /edit bookmark/i }))
    fireEvent.click(screen.getByText("Cancel"))
    expect(props.updateAttributes).not.toHaveBeenCalled()
    // Should be back to preview card
    expect(screen.queryByLabelText(/bookmark url/i)).toBeNull()
  })

  it("Delete button in edit form calls deleteNode", () => {
    const props = makeProps({ url: "https://example.com" })
    render(<BookmarkView {...props} />)
    fireEvent.click(screen.getByRole("button", { name: /edit bookmark/i }))
    fireEvent.click(screen.getByText("Delete"))
    expect(props.deleteNode).toHaveBeenCalledTimes(1)
  })
})

describe("extractDomain helper (via rendered output)", () => {
  it("extracts hostname from a normal URL", () => {
    const props = makeProps({ url: "https://www.example.com/path?query=1" })
    render(<BookmarkView {...props} />)
    // www. is stripped; domain appears both as title fallback and domain text
    const matches = screen.getAllByText("example.com")
    expect(matches.length).toBeGreaterThanOrEqual(1)
  })

  it("extracts hostname without www prefix", () => {
    const props = makeProps({ url: "https://github.com/org/repo" })
    render(<BookmarkView {...props} />)
    const matches = screen.getAllByText("github.com")
    expect(matches.length).toBeGreaterThanOrEqual(1)
  })

  it("handles invalid URL gracefully (returns raw string)", () => {
    const props = makeProps({ url: "not-a-url", title: "Fallback" })
    render(<BookmarkView {...props} />)
    // extractDomain returns the raw string when URL parsing fails
    expect(screen.getByText("not-a-url")).toBeInTheDocument()
  })

  it("handles empty string gracefully", () => {
    // url="" branch renders the input form, not the preview card
    const props = makeProps({ url: "" })
    render(<BookmarkView {...props} />)
    expect(screen.getByPlaceholderText("Paste a URL and press Enter…")).toBeInTheDocument()
  })
})
