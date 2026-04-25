"use client"

import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import { NodeViewWrapper, NodeViewContent, ReactNodeViewRenderer } from "@tiptap/react"

type TeacherNoteNodeAttrs = {
  id: string
}

function TeacherNoteNodeView() {
  return (
    <NodeViewWrapper className="teacher-note-node my-3">
      <div className="rounded-r-md border-l-4 border-amber-400 bg-amber-50 px-4 py-3">
        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-amber-700">
          Teacher Note
        </p>
        {/* NodeViewContent renders the editable ProseMirror content area */}
        <NodeViewContent className="text-sm text-zinc-800 [&>*:first-child]:mt-0 [&>*:last-child]:mb-0" />
      </div>
    </NodeViewWrapper>
  )
}

export const TeacherNoteNode = Node.create({
  name: "teacher-note",
  group: "block",
  content: "block+",
  // Not atom — teachers type inside it
  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
      },
    }
  },
  parseHTML() {
    return [
      {
        tag: 'div[data-type="teacher-note"]',
      },
    ]
  },
  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "teacher-note",
        "data-teacher-note-id": node.attrs.id,
      }),
      0, // content hole
    ]
  },
  addNodeView() {
    return ReactNodeViewRenderer(TeacherNoteNodeView)
  },
})
