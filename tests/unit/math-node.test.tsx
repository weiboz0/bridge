// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest"
import { render, screen, fireEvent } from "@testing-library/react"
import React from "react"

// ---------------------------------------------------------------------------
// Capture the MathNodeView component via ReactNodeViewRenderer mock
// ---------------------------------------------------------------------------
let CapturedMathNodeView: React.ComponentType<any> | null = null

vi.mock("@tiptap/react", async (importOriginal) => {
  const React = await import("react")
  return {
    ...(await importOriginal<typeof import("@tiptap/react")>()),
    NodeViewWrapper: React.forwardRef(
      ({ children, className, as: Tag = "div", contentEditable, ...rest }: any, ref: any) =>
        React.createElement(Tag as string, { ref, className, "data-node-view-wrapper": "", ...rest }, children)
    ),
    NodeViewContent: ({ className }: any) =>
      React.createElement("div", { className, "data-node-view-content": "" }),
    ReactNodeViewRenderer: (Component: any, _opts?: any) => {
      // Last registration wins — both MathBlockNode and MathInlineNode use the
      // same MathNodeView component, so this is fine.
      CapturedMathNodeView = Component
      return Component
    },
  }
})

import { MathBlockNode, MathInlineNode } from "@/components/editor/tiptap/math-node"

// Force addNodeView to trigger capture if not already done
if (!CapturedMathNodeView) {
  const ext = MathBlockNode.extend({})
  const config = ext.config ?? ext.options
  if (config?.addNodeView) {
    ;(config.addNodeView as Function).call({ name: "math-block", options: {} })
  }
}

function MathView(props: any) {
  if (!CapturedMathNodeView) throw new Error("MathNodeView was not captured")
  return React.createElement(CapturedMathNodeView, props)
}

// ---------------------------------------------------------------------------
// Helpers: build NodeViewProps
// ---------------------------------------------------------------------------
function makeBlockProps(latex: string, overrides: Partial<any> = {}) {
  return {
    node: {
      type: { name: "math-block" },
      attrs: { id: "mid", latex },
    },
    updateAttributes: vi.fn(),
    deleteNode: vi.fn(),
    selected: false,
    ...overrides,
  }
}

function makeInlineProps(latex: string, overrides: Partial<any> = {}) {
  return {
    node: {
      type: { name: "math-inline" },
      attrs: { id: "mid", latex },
    },
    updateAttributes: vi.fn(),
    deleteNode: vi.fn(),
    selected: false,
    ...overrides,
  }
}

// ---------------------------------------------------------------------------
// Tests: MathBlockNode view
// ---------------------------------------------------------------------------
describe("MathBlockNodeView — render mode", () => {
  it("renders KaTeX HTML for valid LaTeX", () => {
    const { container } = render(<MathView {...makeBlockProps("x^2 + y^2 = z^2")} />)
    // KaTeX output contains class="katex"
    expect(container.querySelector(".katex")).toBeInTheDocument()
  })

  it("does NOT show error for valid LaTeX", () => {
    render(<MathView {...makeBlockProps("\\frac{1}{2}")} />)
    expect(screen.queryByText(/error/i)).toBeNull()
  })

  it("renders textarea when latex is empty (auto edit mode)", () => {
    render(<MathView {...makeBlockProps("")} />)
    // With empty latex, editing mode is shown (textarea present) because
    // the component initialises editing=true when latex is empty
    expect(screen.getByRole("textbox")).toBeInTheDocument()
  })

  it("shows placeholder text in textarea when in edit mode with empty latex", () => {
    render(<MathView {...makeBlockProps("")} />)
    const textarea = screen.getByRole("textbox")
    expect((textarea as HTMLTextAreaElement).placeholder).toMatch(/display math/i)
  })

  it("clicking KaTeX output enters edit mode", () => {
    render(<MathView {...makeBlockProps("x^2")} />)
    // Render mode: no textarea visible initially
    expect(screen.queryByRole("textbox")).toBeNull()
    // Click the rendered KaTeX
    const span = document.querySelector("[data-node-view-wrapper] span")!
    fireEvent.click(span)
    expect(screen.getByRole("textbox")).toBeInTheDocument()
  })
})

describe("MathBlockNodeView — edit mode", () => {
  it("starts in edit mode when latex is empty", () => {
    render(<MathView {...makeBlockProps("")} />)
    expect(screen.getByRole("textbox")).toBeInTheDocument()
  })

  it("blur calls updateAttributes with new latex value", () => {
    const props = makeBlockProps("")
    render(<MathView {...props} />)
    const textarea = screen.getByRole("textbox")
    fireEvent.change(textarea, { target: { value: "y = mx + b" } })
    fireEvent.blur(textarea)
    expect(props.updateAttributes).toHaveBeenCalledWith({ latex: "y = mx + b" })
  })

  it("Enter key commits changes via updateAttributes", () => {
    const props = makeBlockProps("")
    render(<MathView {...props} />)
    const textarea = screen.getByRole("textbox")
    fireEvent.change(textarea, { target: { value: "E = mc^2" } })
    fireEvent.keyDown(textarea, { key: "Enter" })
    expect(props.updateAttributes).toHaveBeenCalledWith({ latex: "E = mc^2" })
  })

  it("Shift+Enter does NOT commit (allows multi-line editing)", () => {
    const props = makeBlockProps("")
    render(<MathView {...props} />)
    const textarea = screen.getByRole("textbox")
    fireEvent.change(textarea, { target: { value: "a + b" } })
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: true })
    // updateAttributes should NOT be called on Shift+Enter
    expect(props.updateAttributes).not.toHaveBeenCalled()
  })

  it("Escape key reverts to original latex without calling updateAttributes", () => {
    const props = makeBlockProps("x^2")
    render(<MathView {...props} />)
    // Enter edit mode
    const katexSpan = document.querySelector("[data-node-view-wrapper] span")!
    fireEvent.click(katexSpan)
    const textarea = screen.getByRole("textbox")
    fireEvent.change(textarea, { target: { value: "something else" } })
    fireEvent.keyDown(textarea, { key: "Escape" })
    expect(props.updateAttributes).not.toHaveBeenCalled()
    // Edit mode should be closed
    expect(screen.queryByRole("textbox")).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// Tests: MathInlineNode view
// ---------------------------------------------------------------------------
describe("MathInlineNodeView", () => {
  it("renders KaTeX for valid inline LaTeX", () => {
    const { container } = render(<MathView {...makeInlineProps("a^2")} />)
    expect(container.querySelector(".katex")).toBeInTheDocument()
  })

  it("starts in edit mode when inline latex is empty", () => {
    render(<MathView {...makeInlineProps("")} />)
    const textarea = screen.getByRole("textbox")
    expect((textarea as HTMLTextAreaElement).placeholder).toMatch(/inline math/i)
  })

  it("blur commits inline latex via updateAttributes", () => {
    const props = makeInlineProps("")
    render(<MathView {...props} />)
    const textarea = screen.getByRole("textbox")
    fireEvent.change(textarea, { target: { value: "\\sqrt{2}" } })
    fireEvent.blur(textarea)
    expect(props.updateAttributes).toHaveBeenCalledWith({ latex: "\\sqrt{2}" })
  })
})

// ---------------------------------------------------------------------------
// Tests: placeholder display
// ---------------------------------------------------------------------------
describe("MathNodeView — placeholder display", () => {
  it("shows 'Click to add display math' placeholder when committing empty string", () => {
    const props = makeBlockProps("")
    render(<MathView {...props} />)
    const textarea = screen.getByRole("textbox")
    // Commit empty string -> updateAttributes({ latex: "" }) -> editing=false, html=""
    fireEvent.keyDown(textarea, { key: "Enter" })
    // After commit with empty string, the placeholder text should appear
    expect(screen.getByText(/click to add display math/i)).toBeInTheDocument()
  })

  it("shows 'Click to add inline math' placeholder for empty inline latex after commit", () => {
    const props = makeInlineProps("")
    render(<MathView {...props} />)
    const textarea = screen.getByRole("textbox")
    fireEvent.keyDown(textarea, { key: "Enter" })
    expect(screen.getByText(/click to add inline math/i)).toBeInTheDocument()
  })
})
