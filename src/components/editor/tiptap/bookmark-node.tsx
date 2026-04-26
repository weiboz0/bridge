"use client"

import { useState } from "react"
import { nanoid } from "nanoid"
import { Node, mergeAttributes } from "@tiptap/core"
import {
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type NodeViewProps,
} from "@tiptap/react"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function extractDomain(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "")
  } catch {
    return url
  }
}

// ---------------------------------------------------------------------------
// Node view
// ---------------------------------------------------------------------------

function BookmarkNodeView({ node, updateAttributes, deleteNode, selected }: NodeViewProps) {
  const { url, title, description } = node.attrs as {
    id: string
    url: string
    title?: string
    description?: string
    image?: string
  }

  const [inputValue, setInputValue] = useState(url ?? "")
  const [editingMeta, setEditingMeta] = useState(false)
  const [localTitle, setLocalTitle] = useState(title ?? "")
  const [localDesc, setLocalDesc] = useState(description ?? "")
  const [localUrl, setLocalUrl] = useState(url ?? "")

  // If no URL set yet, show the URL input form.
  if (!url) {
    return (
      <NodeViewWrapper className="bookmark-node my-3">
        <div
          contentEditable={false}
          className="flex items-center gap-2 rounded-md border border-zinc-200 bg-zinc-50 px-4 py-3"
        >
          <span className="text-zinc-400 text-sm select-none">🔗</span>
          <input
            type="url"
            value={inputValue}
            placeholder="Paste a URL and press Enter…"
            aria-label="Bookmark URL"
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault()
                const trimmed = inputValue.trim()
                if (trimmed) {
                  updateAttributes({ url: trimmed })
                }
              }
            }}
            className="flex-1 bg-transparent text-sm text-zinc-800 placeholder:text-zinc-400 focus:outline-none"
          />
        </div>
      </NodeViewWrapper>
    )
  }

  // Render the bookmark card.
  const domain = extractDomain(url)
  const displayTitle = title || domain
  const displayDesc = description || ""

  return (
    <NodeViewWrapper className="bookmark-node my-3">
      <div contentEditable={false}>
        {editingMeta ? (
          // Inline meta editor — includes URL editing
          <div className="rounded-md border border-zinc-300 bg-white px-4 py-3 space-y-2">
            <input
              type="url"
              value={localUrl}
              placeholder="URL"
              aria-label="Bookmark URL"
              onChange={(e) => setLocalUrl(e.target.value)}
              className="w-full rounded border border-zinc-200 px-2 py-1 text-sm font-mono focus:outline-none focus:ring-1 focus:ring-zinc-400"
            />
            <input
              type="text"
              value={localTitle}
              placeholder="Title (optional)"
              aria-label="Bookmark title"
              onChange={(e) => setLocalTitle(e.target.value)}
              className="w-full rounded border border-zinc-200 px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-zinc-400"
            />
            <textarea
              value={localDesc}
              placeholder="Description (optional)"
              aria-label="Bookmark description"
              rows={2}
              onChange={(e) => setLocalDesc(e.target.value)}
              className="w-full rounded border border-zinc-200 px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-zinc-400 resize-none"
            />
            <div className="flex gap-2 justify-end">
              <button
                type="button"
                onClick={() => { deleteNode() }}
                className="mr-auto text-xs text-red-500 hover:text-red-700"
              >
                Delete
              </button>
              <button
                type="button"
                onClick={() => setEditingMeta(false)}
                className="text-xs text-zinc-500 hover:text-zinc-700"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={() => {
                  updateAttributes({ url: localUrl.trim(), title: localTitle, description: localDesc })
                  setInputValue(localUrl.trim())
                  setEditingMeta(false)
                }}
                className="rounded bg-zinc-900 px-2 py-1 text-xs text-white hover:bg-zinc-700"
              >
                Save
              </button>
            </div>
          </div>
        ) : (
          // Preview card — use div instead of <a> to prevent browser drag creating duplicates
          <div
            className={`group flex flex-col gap-1 rounded-md border bg-white px-4 py-3 cursor-pointer transition-colors ${selected ? "border-blue-400 ring-1 ring-blue-200" : "border-zinc-200 hover:border-zinc-400"}`}
            onClick={() => window.open(url, "_blank", "noopener,noreferrer")}
            onDragStart={(e) => e.preventDefault()}
            role="link"
            tabIndex={0}
          >
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <p className="text-sm font-semibold text-zinc-900 truncate">{displayTitle}</p>
                {displayDesc && (
                  <p className="text-xs text-zinc-500 line-clamp-2 mt-0.5">{displayDesc}</p>
                )}
                <p className="mt-1 text-xs text-zinc-400">{domain}</p>
              </div>
              <button
                type="button"
                onClick={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  setLocalTitle(title ?? "")
                  setLocalDesc(description ?? "")
                  setEditingMeta(true)
                }}
                aria-label="Edit bookmark"
                className="shrink-0 rounded p-1 text-zinc-400 opacity-0 group-hover:opacity-100 hover:bg-zinc-100 hover:text-zinc-700 transition-opacity"
              >
                ✎
              </button>
            </div>
          </div>
        )}
      </div>
    </NodeViewWrapper>
  )
}

// ---------------------------------------------------------------------------
// Tiptap node definition
// ---------------------------------------------------------------------------

export const BookmarkNode = Node.create({
  name: "bookmark",
  group: "block",
  atom: true,
  selectable: true,
  draggable: true,

  addAttributes() {
    return {
      id: {
        default: () => nanoid(),
        parseHTML: (element: Element) => element.getAttribute("data-bookmark-id") ?? nanoid(),
      },
      url: {
        default: "",
        parseHTML: (element: Element) => element.getAttribute("data-bookmark-url") ?? "",
      },
      title: {
        default: null,
        parseHTML: (element: Element) => element.getAttribute("data-bookmark-title") ?? null,
      },
      description: {
        default: null,
        parseHTML: (element: Element) => element.getAttribute("data-bookmark-description") ?? null,
      },
      image: {
        default: null,
        parseHTML: (element: Element) => element.getAttribute("data-bookmark-image") ?? null,
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="bookmark"]' }]
  },

  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "bookmark",
        "data-bookmark-id": node.attrs.id,
        "data-bookmark-url": node.attrs.url ?? "",
        "data-bookmark-title": node.attrs.title ?? "",
        "data-bookmark-description": node.attrs.description ?? "",
        "data-bookmark-image": node.attrs.image ?? "",
      }),
    ]
  },

  addNodeView() {
    return ReactNodeViewRenderer(BookmarkNodeView)
  },
})
