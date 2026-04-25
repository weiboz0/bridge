"use client"

import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import { NodeViewWrapper, ReactNodeViewRenderer, type NodeViewProps } from "@tiptap/react"

type TestCaseRefNodeAttrs = {
  id: string
  testCaseId: string
  problemRefId: string
  showToStudent: boolean
}

function TestCaseRefNodeView({ node }: NodeViewProps) {
  const { testCaseId, problemRefId, showToStudent } = node.attrs as TestCaseRefNodeAttrs

  return (
    <NodeViewWrapper className="test-case-ref-node my-3">
      <div className="rounded-r-md border-l-4 border-orange-400 bg-orange-50 px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-orange-700">
            Test Case
          </p>
          <span
            className={
              showToStudent
                ? "inline-flex items-center rounded-full border border-green-200 bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700"
                : "inline-flex items-center rounded-full border border-zinc-200 bg-zinc-100 px-2 py-0.5 text-xs font-medium text-zinc-600"
            }
          >
            {showToStudent ? "Shown to student" : "Hidden from student"}
          </span>
        </div>
        <p className="mt-1 text-sm text-zinc-600">
          Test case ID: <span className="font-mono">{testCaseId || "(not set)"}</span>
        </p>
        {problemRefId && (
          <p className="text-xs text-zinc-400 mt-0.5">
            Problem ref: <span className="font-mono">{problemRefId}</span>
          </p>
        )}
      </div>
    </NodeViewWrapper>
  )
}

export const TestCaseRefNode = Node.create({
  name: "test-case-ref",
  group: "block",
  atom: true,
  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
      },
      testCaseId: {
        default: "",
        parseHTML: (element: Element) => element.getAttribute("data-test-case-ref-test-case-id") ?? "",
      },
      problemRefId: {
        default: "",
        parseHTML: (element: Element) => element.getAttribute("data-test-case-ref-problem-ref-id") ?? "",
      },
      showToStudent: {
        default: true,
        parseHTML: (element: Element) => {
          const v = element.getAttribute("data-test-case-ref-show-to-student")
          return v !== "false"
        },
      },
    }
  },
  parseHTML() {
    return [{ tag: 'div[data-type="test-case-ref"]' }]
  },
  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "test-case-ref",
        "data-test-case-ref-id": node.attrs.id,
        "data-test-case-ref-test-case-id": node.attrs.testCaseId,
        "data-test-case-ref-problem-ref-id": node.attrs.problemRefId,
        "data-test-case-ref-show-to-student": String(node.attrs.showToStudent),
      }),
    ]
  },
  addNodeView() {
    return ReactNodeViewRenderer(TestCaseRefNodeView)
  },
})
