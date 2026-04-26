"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { nanoid } from "nanoid"
import { Node, mergeAttributes, InputRule } from "@tiptap/core"
import {
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type NodeViewProps,
} from "@tiptap/react"
import katex from "katex"

// ---------------------------------------------------------------------------
// Shared KaTeX renderer
// ---------------------------------------------------------------------------

function renderKaTeX(
  latex: string,
  displayMode: boolean,
): { html: string; error: string | null } {
  if (!latex.trim()) {
    return { html: "", error: null }
  }
  try {
    const html = katex.renderToString(latex, {
      displayMode,
      throwOnError: false,
      output: "htmlAndMathml",
      strict: false,
    })
    return { html, error: null }
  } catch (err) {
    return {
      html: "",
      error: err instanceof Error ? err.message : "KaTeX render error",
    }
  }
}

// ---------------------------------------------------------------------------
// Math node view (shared between block and inline)
// ---------------------------------------------------------------------------

function MathNodeView({ node, updateAttributes, selected }: NodeViewProps) {
  const isBlock = node.type.name === "math-block"
  const { latex } = node.attrs as { id: string; latex: string }
  const [editing, setEditing] = useState(!latex)
  const [draft, setDraft] = useState(latex)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // Sync draft when the attr changes externally (e.g., collab).
  useEffect(() => {
    setDraft(latex)
  }, [latex])

  // Auto-focus when entering edit mode.
  useEffect(() => {
    if (editing) {
      setTimeout(() => textareaRef.current?.focus(), 0)
    }
  }, [editing])

  const commit = useCallback(() => {
    updateAttributes({ latex: draft })
    setEditing(false)
  }, [draft, updateAttributes])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault()
        commit()
      }
      if (e.key === "Escape") {
        setDraft(latex) // revert
        setEditing(false)
      }
    },
    [commit, latex],
  )

  const { html, error } = renderKaTeX(draft || latex, isBlock)

  if (editing) {
    return (
      <NodeViewWrapper
        className={isBlock ? "math-block-node my-3" : "math-inline-node"}
        as={isBlock ? "div" : "span"}
        contentEditable={false}
      >
        <div
          className={
            "rounded border border-blue-300 bg-blue-50 p-2 " +
            (isBlock ? "text-center" : "inline-block")
          }
        >
          <textarea
            ref={textareaRef}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commit}
            onKeyDown={handleKeyDown}
            placeholder={isBlock ? "Display math (LaTeX)..." : "Inline math..."}
            rows={isBlock ? 3 : 1}
            className="w-full resize-none rounded border border-zinc-200 bg-white px-2 py-1 font-mono text-sm outline-none focus:border-blue-400"
          />
          {error && (
            <p className="mt-1 text-xs text-red-600">{error}</p>
          )}
        </div>
      </NodeViewWrapper>
    )
  }

  // Render mode: show KaTeX output, click to edit.
  if (!html) {
    return (
      <NodeViewWrapper
        className={isBlock ? "math-block-node my-3" : "math-inline-node"}
        as={isBlock ? "div" : "span"}
        contentEditable={false}
      >
        <span
          onClick={() => setEditing(true)}
          className={
            "cursor-pointer rounded border border-dashed border-zinc-300 px-2 py-1 text-xs text-zinc-400 " +
            (isBlock ? "block text-center" : "inline-block")
          }
        >
          Click to add {isBlock ? "display" : "inline"} math
        </span>
      </NodeViewWrapper>
    )
  }

  return (
    <NodeViewWrapper
      className={isBlock ? "math-block-node my-3" : "math-inline-node"}
      as={isBlock ? "div" : "span"}
      contentEditable={false}
    >
      <span
        onClick={() => setEditing(true)}
        className={
          "cursor-pointer rounded transition-colors " +
          (selected ? "ring-2 ring-blue-300" : "hover:bg-blue-50") +
          (isBlock ? " block text-center" : " inline-block")
        }
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </NodeViewWrapper>
  )
}

// ---------------------------------------------------------------------------
// Math Block Node (display mode, centered)
// ---------------------------------------------------------------------------

export const MathBlockNode = Node.create({
  name: "math-block",
  group: "block",
  atom: true,

  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
        parseHTML: (element: Element) =>
          element.getAttribute("data-math-id") ?? nanoid(),
      },
      latex: {
        default: "",
        parseHTML: (element: Element) =>
          element.getAttribute("data-math-latex") ?? "",
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="math-block"]' }]
  },

  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "math-block",
        "data-math-id": node.attrs.id,
        "data-math-latex": node.attrs.latex,
      }),
    ]
  },

  addNodeView() {
    return ReactNodeViewRenderer(MathNodeView)
  },

  addInputRules() {
    return [
      // $$...$$ → math-block
      new InputRule({
        find: /\$\$([^$]+)\$\$$/,
        handler: ({ state, range, match }) => {
          const latex = match[1] || ""
          const { tr } = state
          tr.replaceWith(
            range.from,
            range.to,
            this.type.create({ id: nanoid(), latex }),
          )
        },
      }),
    ]
  },
})

// ---------------------------------------------------------------------------
// Math Inline Node (inline, text mode)
// ---------------------------------------------------------------------------

export const MathInlineNode = Node.create({
  name: "math-inline",
  group: "inline",
  inline: true,
  atom: true,

  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
        parseHTML: (element: Element) =>
          element.getAttribute("data-math-id") ?? nanoid(),
      },
      latex: {
        default: "",
        parseHTML: (element: Element) =>
          element.getAttribute("data-math-latex") ?? "",
      },
    }
  },

  parseHTML() {
    return [{ tag: 'span[data-type="math-inline"]' }]
  },

  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "span",
      mergeAttributes(HTMLAttributes, {
        "data-type": "math-inline",
        "data-math-id": node.attrs.id,
        "data-math-latex": node.attrs.latex,
      }),
    ]
  },

  addNodeView() {
    return ReactNodeViewRenderer(MathNodeView, { as: "span" })
  },

  addInputRules() {
    return [
      // $...$ → math-inline (but not $$)
      new InputRule({
        find: /(?<!\$)\$([^$\n]+)\$$/,
        handler: ({ state, range, match }) => {
          const latex = match[1] || ""
          const { tr } = state
          tr.replaceWith(
            range.from,
            range.to,
            this.type.create({ id: nanoid(), latex }),
          )
        },
      }),
    ]
  },
})
