"use client"

import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import { NodeViewWrapper, ReactNodeViewRenderer, type NodeViewProps } from "@tiptap/react"

type RevealMode = "always" | "after-submit"

type SolutionRefNodeAttrs = {
  id: string
  solutionId: string
  reveal: RevealMode
}

function SolutionRefNodeView({ node }: NodeViewProps) {
  const { solutionId, reveal } = node.attrs as SolutionRefNodeAttrs

  return (
    <NodeViewWrapper className="solution-ref-node my-3">
      <div className="rounded-r-md border-l-4 border-green-500 bg-green-50 px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-green-700">
            Solution
          </p>
          <span
            className={
              reveal === "always"
                ? "inline-flex items-center rounded-full border border-green-200 bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700"
                : "inline-flex items-center rounded-full border border-amber-200 bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700"
            }
          >
            {reveal === "always" ? "Always visible" : "Revealed after submission"}
          </span>
        </div>
        <p className="mt-1 text-sm text-zinc-600">
          Solution ID: <span className="font-mono">{solutionId || "(not set)"}</span>
        </p>
      </div>
    </NodeViewWrapper>
  )
}

export const SolutionRefNode = Node.create({
  name: "solution-ref",
  group: "block",
  atom: true,
  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
      },
      solutionId: {
        default: "",
        parseHTML: (element: Element) => element.getAttribute("data-solution-ref-solution-id") ?? "",
      },
      reveal: {
        default: "always",
        parseHTML: (element: Element) => {
          const v = element.getAttribute("data-solution-ref-reveal")
          return v === "after-submit" ? "after-submit" : "always"
        },
      },
    }
  },
  parseHTML() {
    return [{ tag: 'div[data-type="solution-ref"]' }]
  },
  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "solution-ref",
        "data-solution-ref-id": node.attrs.id,
        "data-solution-ref-solution-id": node.attrs.solutionId,
        "data-solution-ref-reveal": node.attrs.reveal,
      }),
    ]
  },
  addNodeView() {
    return ReactNodeViewRenderer(SolutionRefNodeView)
  },
})
