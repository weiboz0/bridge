"use client"

import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import {
  NodeViewWrapper,
  NodeViewContent,
  ReactNodeViewRenderer,
} from "@tiptap/react"

// ---------------------------------------------------------------------------
// Column (child node)
// ---------------------------------------------------------------------------

function ColumnNodeView() {
  return (
    <NodeViewWrapper
      as="div"
      className="column-node min-w-0 flex-1 rounded-sm border border-transparent px-2 focus-within:border-zinc-200"
    >
      <NodeViewContent className="text-sm text-zinc-800 [&>*:first-child]:mt-0 [&>*:last-child]:mb-0" />
    </NodeViewWrapper>
  )
}

export const ColumnNode = Node.create({
  name: "column",
  group: "block",
  content: "block+",
  isolating: true,

  parseHTML() {
    return [{ tag: 'div[data-type="column"]' }]
  },

  renderHTML({ HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, { "data-type": "column" }),
      0, // content hole
    ]
  },

  addNodeView() {
    return ReactNodeViewRenderer(ColumnNodeView)
  },
})

// ---------------------------------------------------------------------------
// Columns (parent node)
// ---------------------------------------------------------------------------

function ColumnsNodeView() {
  return (
    <NodeViewWrapper className="columns-node my-3">
      <div className="flex gap-4 rounded-md border border-zinc-200 bg-white p-3">
        <NodeViewContent className="flex flex-row gap-4 w-full" />
      </div>
    </NodeViewWrapper>
  )
}

export const ColumnsNode = Node.create({
  name: "columns",
  group: "block",
  content: "column+",

  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
        parseHTML: (element: Element) => element.getAttribute("data-columns-id") ?? nanoid(),
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="columns"]' }]
  },

  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "columns",
        "data-columns-id": node.attrs.id,
      }),
      0, // content hole
    ]
  },

  addNodeView() {
    return ReactNodeViewRenderer(ColumnsNodeView)
  },
})
