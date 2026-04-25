"use client"

import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import { NodeViewWrapper, NodeViewContent, ReactNodeViewRenderer, type NodeViewProps } from "@tiptap/react"

type CueTrigger = "before-problem" | "after-problem" | "manual"

type LiveCueNodeAttrs = {
  id: string
  trigger: CueTrigger
  problemRefId?: string
}

const TRIGGER_LABELS: Record<CueTrigger, string> = {
  "before-problem": "Before problem",
  "after-problem": "After problem",
  "manual": "Manual",
}

function LiveCueNodeView({ node }: NodeViewProps) {
  const { trigger, problemRefId } = node.attrs as LiveCueNodeAttrs
  const triggerLabel = TRIGGER_LABELS[trigger] ?? trigger

  return (
    <NodeViewWrapper className="live-cue-node my-3">
      <div className="rounded-r-md border-l-4 border-blue-400 bg-blue-50 px-4 py-3">
        <div className="flex items-center gap-3 mb-2">
          <p className="text-xs font-semibold uppercase tracking-wide text-blue-700">
            Live Cue
          </p>
          <span className="inline-flex items-center rounded-full border border-blue-200 bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700">
            {triggerLabel}
          </span>
          {problemRefId && (
            <span className="text-xs text-zinc-400 font-mono">{problemRefId}</span>
          )}
        </div>
        <NodeViewContent className="text-sm text-zinc-800 [&>*:first-child]:mt-0 [&>*:last-child]:mb-0" />
      </div>
    </NodeViewWrapper>
  )
}

export const LiveCueNode = Node.create({
  name: "live-cue",
  group: "block",
  content: "block+",
  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
      },
      trigger: {
        default: "manual",
        parseHTML: (element: Element) => {
          const v = element.getAttribute("data-live-cue-trigger")
          if (v === "before-problem" || v === "after-problem" || v === "manual") return v
          return "manual"
        },
      },
      problemRefId: {
        default: null,
        parseHTML: (element: Element) => element.getAttribute("data-live-cue-problem-ref-id") ?? null,
      },
    }
  },
  parseHTML() {
    return [{ tag: 'div[data-type="live-cue"]' }]
  },
  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "live-cue",
        "data-live-cue-id": node.attrs.id,
        "data-live-cue-trigger": node.attrs.trigger,
        "data-live-cue-problem-ref-id": node.attrs.problemRefId ?? "",
      }),
      0, // content hole
    ]
  },
  addNodeView() {
    return ReactNodeViewRenderer(LiveCueNodeView)
  },
})
