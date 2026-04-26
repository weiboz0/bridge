"use client"

import { useState } from "react"
import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import {
  NodeViewWrapper,
  NodeViewContent,
  ReactNodeViewRenderer,
  type NodeViewProps,
} from "@tiptap/react"

// ---------------------------------------------------------------------------
// Node view
// ---------------------------------------------------------------------------

function ToggleNodeView({ node, updateAttributes }: NodeViewProps) {
  const { summary } = node.attrs as { id: string; summary: string }
  const [open, setOpen] = useState(false)

  return (
    <NodeViewWrapper className="toggle-node my-2">
      <div className="rounded-md border border-zinc-200 bg-white">
        {/* Summary row */}
        <div className="flex items-center gap-2 px-3 py-2">
          {/* Triangle toggle */}
          <button
            type="button"
            contentEditable={false}
            aria-label={open ? "Collapse block" : "Expand block"}
            aria-expanded={open}
            onClick={() => setOpen((prev) => !prev)}
            className="shrink-0 select-none text-zinc-500 transition-transform duration-150"
            style={{ transform: open ? "rotate(90deg)" : "rotate(0deg)" }}
          >
            ▶
          </button>
          {/* Editable summary */}
          <input
            type="text"
            contentEditable={false}
            value={summary}
            placeholder="Toggle summary…"
            aria-label="Toggle block summary"
            onChange={(e) => updateAttributes({ summary: e.target.value })}
            className="flex-1 bg-transparent text-sm font-medium text-zinc-800 placeholder:text-zinc-400 focus:outline-none"
          />
        </div>

        {/* Collapsible content */}
        {open && (
          <div className="border-t border-zinc-100 px-4 py-3">
            <NodeViewContent className="text-sm text-zinc-800 [&>*:first-child]:mt-0 [&>*:last-child]:mb-0" />
          </div>
        )}
      </div>
    </NodeViewWrapper>
  )
}

// ---------------------------------------------------------------------------
// Tiptap node definition
// ---------------------------------------------------------------------------

export const ToggleNode = Node.create({
  name: "toggle-block",
  group: "block",
  content: "block+",

  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
        parseHTML: (element: Element) => element.getAttribute("data-toggle-id") ?? nanoid(),
      },
      summary: {
        default: "",
        parseHTML: (element: Element) =>
          element.getAttribute("data-toggle-summary") ?? "",
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="toggle-block"]' }]
  },

  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "toggle-block",
        "data-toggle-id": node.attrs.id,
        "data-toggle-summary": node.attrs.summary,
      }),
      0, // content hole
    ]
  },

  addNodeView() {
    return ReactNodeViewRenderer(ToggleNodeView)
  },
})
