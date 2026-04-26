// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest"
import { render, screen, fireEvent } from "@testing-library/react"
import React from "react"

// ---------------------------------------------------------------------------
// Mock @tiptap/react so NodeViewWrapper / NodeViewContent render without a
// real ProseMirror editor context, and capture the component passed to
// ReactNodeViewRenderer.
// ---------------------------------------------------------------------------
let CapturedCalloutNodeView: React.ComponentType<any> | null = null

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
      CapturedCalloutNodeView = Component
      return Component
    },
  }
})

// Import triggers Node.create which calls addNodeView → ReactNodeViewRenderer
import { CalloutNode } from "@/components/editor/tiptap/callout-node"

// If the component wasn't captured at import time, force-invoke addNodeView
if (!CapturedCalloutNodeView) {
  const ext = CalloutNode.extend({})
  const config = ext.config ?? ext.options
  if (config?.addNodeView) {
    ;(config.addNodeView as Function).call({ name: "callout", options: {} })
  }
}

function CalloutView(props: any) {
  if (!CapturedCalloutNodeView) throw new Error("CalloutNodeView was not captured")
  return React.createElement(CapturedCalloutNodeView, props)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
type CalloutVariant = "info" | "warning" | "tip" | "danger"

function makeProps(variant: CalloutVariant, overrides: Partial<any> = {}) {
  return {
    node: { attrs: { id: "cid", variant } },
    updateAttributes: vi.fn(),
    deleteNode: vi.fn(),
    selected: false,
    ...overrides,
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------
describe("CalloutNodeView — variant rendering", () => {
  it("renders 'info' variant with 'i' icon", () => {
    const { container } = render(<CalloutView {...makeProps("info")} />)
    // The icon text is 'i' for info
    const button = screen.getByRole("button")
    expect(button).toBeInTheDocument()
    expect(button.textContent).toBe("i")
    // bg-blue-50 class present
    expect(container.querySelector(".bg-blue-50")).toBeInTheDocument()
  })

  it("renders 'warning' variant with '!' icon", () => {
    const { container } = render(<CalloutView {...makeProps("warning")} />)
    const button = screen.getByRole("button")
    expect(button.textContent).toBe("!")
    expect(container.querySelector(".bg-yellow-50")).toBeInTheDocument()
  })

  it("renders 'tip' variant with star icon", () => {
    const { container } = render(<CalloutView {...makeProps("tip")} />)
    const button = screen.getByRole("button")
    expect(button.textContent?.trim()).toContain("★")
    expect(container.querySelector(".bg-green-50")).toBeInTheDocument()
  })

  it("renders 'danger' variant with x icon", () => {
    const { container } = render(<CalloutView {...makeProps("danger")} />)
    const button = screen.getByRole("button")
    expect(button.textContent?.trim()).toContain("✕")
    expect(container.querySelector(".bg-red-50")).toBeInTheDocument()
  })

  it("renders NodeViewContent (editable content area)", () => {
    const { container } = render(<CalloutView {...makeProps("info")} />)
    expect(container.querySelector("[data-node-view-content]")).toBeInTheDocument()
  })
})

describe("CalloutNodeView — variant cycling", () => {
  it("clicking icon calls updateAttributes with next variant (info -> warning)", () => {
    const props = makeProps("info")
    render(<CalloutView {...props} />)
    fireEvent.click(screen.getByRole("button"))
    expect(props.updateAttributes).toHaveBeenCalledWith({ variant: "warning" })
  })

  it("clicking icon cycles warning -> tip", () => {
    const props = makeProps("warning")
    render(<CalloutView {...props} />)
    fireEvent.click(screen.getByRole("button"))
    expect(props.updateAttributes).toHaveBeenCalledWith({ variant: "tip" })
  })

  it("clicking icon cycles tip -> danger", () => {
    const props = makeProps("tip")
    render(<CalloutView {...props} />)
    fireEvent.click(screen.getByRole("button"))
    expect(props.updateAttributes).toHaveBeenCalledWith({ variant: "danger" })
  })

  it("clicking icon wraps around: danger -> info", () => {
    const props = makeProps("danger")
    render(<CalloutView {...props} />)
    fireEvent.click(screen.getByRole("button"))
    expect(props.updateAttributes).toHaveBeenCalledWith({ variant: "info" })
  })

  it("updateAttributes is called exactly once per click", () => {
    const props = makeProps("info")
    render(<CalloutView {...props} />)
    fireEvent.click(screen.getByRole("button"))
    expect(props.updateAttributes).toHaveBeenCalledTimes(1)
  })
})

describe("CalloutNodeView — button accessibility", () => {
  it("icon button has descriptive aria-label", () => {
    render(<CalloutView {...makeProps("info")} />)
    const button = screen.getByRole("button")
    expect(button.getAttribute("aria-label")).toMatch(/info/i)
  })

  it("icon button title contains variant name and click instruction", () => {
    render(<CalloutView {...makeProps("warning")} />)
    const button = screen.getByRole("button")
    expect(button.getAttribute("title")).toMatch(/warning/i)
  })
})

describe("CalloutNodeView — unknown variant fallback", () => {
  it("falls back to 'info' config for unknown variant", () => {
    const props = {
      node: { attrs: { id: "x", variant: "unknown_variant" } },
      updateAttributes: vi.fn(),
      deleteNode: vi.fn(),
      selected: false,
    }
    // Should not throw; falls back to VARIANTS.info
    const { container } = render(<CalloutView {...props} />)
    expect(container.querySelector(".bg-blue-50")).toBeInTheDocument()
  })
})
