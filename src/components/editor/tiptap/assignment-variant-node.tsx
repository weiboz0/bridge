"use client"

import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import { NodeViewWrapper, NodeViewContent, ReactNodeViewRenderer, type NodeViewProps } from "@tiptap/react"

type AssignmentVariantNodeAttrs = {
  id: string
  title: string
  timeLimitMinutes?: number | null
}

function AssignmentVariantNodeView({ node }: NodeViewProps) {
  const { title, timeLimitMinutes } = node.attrs as AssignmentVariantNodeAttrs

  return (
    <NodeViewWrapper className="assignment-variant-node my-3">
      <div className="rounded-r-md border-l-4 border-purple-400 bg-purple-50 px-4 py-3">
        <div className="flex items-center gap-3 mb-2">
          <p className="text-xs font-semibold uppercase tracking-wide text-purple-700">
            Assignment
          </p>
          {title && (
            <span className="text-sm font-medium text-zinc-800">{title}</span>
          )}
          {timeLimitMinutes != null && timeLimitMinutes > 0 && (
            <span className="inline-flex items-center rounded-full border border-purple-200 bg-purple-100 px-2 py-0.5 text-xs font-medium text-purple-700">
              {timeLimitMinutes} min
            </span>
          )}
        </div>
        <NodeViewContent className="text-sm text-zinc-800 [&>*:first-child]:mt-0 [&>*:last-child]:mb-0" />
      </div>
    </NodeViewWrapper>
  )
}

export const AssignmentVariantNode = Node.create({
  name: "assignment-variant",
  group: "block",
  content: "block+",
  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
      },
      title: {
        default: "",
        parseHTML: (element: Element) => element.getAttribute("data-assignment-variant-title") ?? "",
      },
      timeLimitMinutes: {
        default: null,
        parseHTML: (element: Element) => {
          const v = element.getAttribute("data-assignment-variant-time-limit-minutes")
          if (!v) return null
          const n = parseInt(v, 10)
          return Number.isFinite(n) ? n : null
        },
      },
    }
  },
  parseHTML() {
    return [{ tag: 'div[data-type="assignment-variant"]' }]
  },
  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "assignment-variant",
        "data-assignment-variant-id": node.attrs.id,
        "data-assignment-variant-title": node.attrs.title,
        "data-assignment-variant-time-limit-minutes":
          node.attrs.timeLimitMinutes != null ? String(node.attrs.timeLimitMinutes) : "",
      }),
      0, // content hole
    ]
  },
  addNodeView() {
    return ReactNodeViewRenderer(AssignmentVariantNodeView)
  },
})
