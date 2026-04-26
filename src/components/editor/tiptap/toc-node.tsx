"use client"

import { useEffect, useState } from "react"
import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import {
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type NodeViewProps,
} from "@tiptap/react"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type HeadingEntry = {
  level: number
  text: string
  /** ProseMirror absolute position of the heading node start. */
  pos: number
}

// ---------------------------------------------------------------------------
// Node view
// ---------------------------------------------------------------------------

function TocNodeView({ editor }: NodeViewProps) {
  const [headings, setHeadings] = useState<HeadingEntry[]>([])

  useEffect(() => {
    if (!editor) return

    function collectHeadings() {
      const entries: HeadingEntry[] = []
      editor.state.doc.forEach((node, offset) => {
        if (node.type.name === "heading") {
          entries.push({
            level: node.attrs.level as number,
            text: node.textContent,
            pos: offset,
          })
        }
      })
      setHeadings(entries)
    }

    collectHeadings()
    editor.on("update", collectHeadings)
    return () => {
      editor.off("update", collectHeadings)
    }
  }, [editor])

  function scrollToHeading(pos: number) {
    // Move cursor to the heading so the editor scrolls it into view.
    editor
      .chain()
      .focus()
      .setTextSelection(pos + 1) // +1 to land inside the node
      .run()

    // Also attempt DOM scroll for visibility.
    try {
      const domPos = editor.view.domAtPos(pos + 1)
      const el = domPos.node as HTMLElement
      if (el) {
        const target = el.nodeType === 3 ? el.parentElement : (el as HTMLElement)
        target?.scrollIntoView({ behavior: "smooth", block: "start" })
      }
    } catch {
      // Ignore — cursor movement is sufficient
    }
  }

  return (
    <NodeViewWrapper className="toc-node my-4" contentEditable={false}>
      <div className="rounded-md border border-zinc-200 bg-zinc-50 px-4 py-3">
        <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-zinc-500 select-none">
          Table of Contents
        </p>
        {headings.length === 0 ? (
          <p className="text-xs text-zinc-400 italic">No headings found in document.</p>
        ) : (
          <nav aria-label="Table of contents">
            <ul className="space-y-0.5">
              {headings.map((h, i) => (
                <li
                  key={i}
                  style={{ paddingLeft: `${(h.level - 1) * 12}px` }}
                >
                  <button
                    type="button"
                    onClick={() => scrollToHeading(h.pos)}
                    className="text-left text-sm text-blue-600 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-400 focus-visible:ring-offset-1 truncate max-w-full"
                  >
                    {h.text || <span className="italic text-zinc-400">Untitled heading</span>}
                  </button>
                </li>
              ))}
            </ul>
          </nav>
        )}
      </div>
    </NodeViewWrapper>
  )
}

// ---------------------------------------------------------------------------
// Tiptap node definition
// ---------------------------------------------------------------------------

export const TocNode = Node.create({
  name: "toc",
  group: "block",
  atom: true,

  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
        parseHTML: (element: Element) => element.getAttribute("data-toc-id") ?? nanoid(),
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="toc"]' }]
  },

  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "toc",
        "data-toc-id": node.attrs.id,
      }),
    ]
  },

  addNodeView() {
    return ReactNodeViewRenderer(TocNodeView)
  },
})
