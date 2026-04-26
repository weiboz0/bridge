"use client"

import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
} from "react"
import { createPortal } from "react-dom"
import { createRoot } from "react-dom/client"
import { Node, mergeAttributes, Extension } from "@tiptap/core"
import {
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type NodeViewProps,
} from "@tiptap/react"
import Suggestion, {
  type SuggestionOptions,
  type SuggestionProps,
  type SuggestionKeyDownProps,
} from "@tiptap/suggestion"
import type { Editor, Range } from "@tiptap/core"
import { PluginKey } from "@tiptap/pm/state"

// ---------------------------------------------------------------------------
// Demo user data (hardcoded for MVP — replace with API call when available)
// ---------------------------------------------------------------------------

interface MentionUser {
  id: string
  name: string
}

const DEMO_USERS: MentionUser[] = [
  { id: "user-1", name: "Alice Chen" },
  { id: "user-2", name: "Bob Martinez" },
  { id: "user-3", name: "Clara Johnson" },
  { id: "user-4", name: "David Kim" },
  { id: "user-5", name: "Emma Wilson" },
]

function filterUsers(query: string): MentionUser[] {
  if (!query) return DEMO_USERS
  const q = query.toLowerCase()
  return DEMO_USERS.filter((u) => u.name.toLowerCase().includes(q))
}

// ---------------------------------------------------------------------------
// Mention node view (inline pill)
// ---------------------------------------------------------------------------

function MentionNodeView({ node }: NodeViewProps) {
  const { label } = node.attrs as { id: string; label: string }
  return (
    <NodeViewWrapper as="span" className="mention-pill">
      <span
        className="inline-flex items-center rounded-md bg-blue-100 px-1.5 py-0.5 text-xs font-medium text-blue-700"
        contentEditable={false}
      >
        @{label}
      </span>
    </NodeViewWrapper>
  )
}

// ---------------------------------------------------------------------------
// Mention Tiptap Node
// ---------------------------------------------------------------------------

export const MentionNode = Node.create({
  name: "mention",
  group: "inline",
  inline: true,
  atom: true,

  addAttributes() {
    return {
      id: { default: "" },
      label: { default: "" },
    }
  },

  parseHTML() {
    return [{ tag: 'span[data-type="mention"]' }]
  },

  renderHTML({ node, HTMLAttributes }: any) {
    return [
      "span",
      mergeAttributes(HTMLAttributes, {
        "data-type": "mention",
        "data-mention-id": node.attrs.id,
        class: "mention-pill",
      }),
      `@${node.attrs.label}`,
    ]
  },

  addNodeView() {
    return ReactNodeViewRenderer(MentionNodeView)
  },
})

// ---------------------------------------------------------------------------
// Mention suggestion list
// ---------------------------------------------------------------------------

interface MentionListProps {
  items: MentionUser[]
  command: (item: MentionUser) => void
}

export interface MentionListHandle {
  onKeyDown: (event: KeyboardEvent) => boolean
}

export const MentionList = forwardRef<MentionListHandle, MentionListProps>(
  function MentionList({ items, command }, ref) {
    const [selectedIndex, setSelectedIndex] = useState(0)
    const containerRef = useRef<HTMLDivElement>(null)

    useEffect(() => {
      setSelectedIndex(0)
    }, [items])

    useEffect(() => {
      const container = containerRef.current
      if (!container) return
      const selected = container.querySelector("[data-selected=true]")
      if (selected) {
        selected.scrollIntoView({ block: "nearest" })
      }
    }, [selectedIndex])

    const selectItem = useCallback(
      (index: number) => {
        const item = items[index]
        if (item) command(item)
      },
      [items, command],
    )

    useImperativeHandle(ref, () => ({
      onKeyDown: (event: KeyboardEvent) => {
        if (event.key === "ArrowUp") {
          setSelectedIndex((prev) => (prev <= 0 ? items.length - 1 : prev - 1))
          return true
        }
        if (event.key === "ArrowDown") {
          setSelectedIndex((prev) => (prev >= items.length - 1 ? 0 : prev + 1))
          return true
        }
        if (event.key === "Enter") {
          selectItem(selectedIndex)
          return true
        }
        return false
      },
    }))

    if (items.length === 0) {
      return (
        <div className="rounded-lg border border-zinc-200 bg-white px-3 py-3 text-center text-sm text-zinc-400 shadow-lg">
          No matching users
        </div>
      )
    }

    return (
      <div
        ref={containerRef}
        className="max-h-48 w-52 overflow-y-auto rounded-lg border border-zinc-200 bg-white py-1 shadow-lg"
      >
        {items.map((item, idx) => (
          <button
            key={item.id}
            type="button"
            data-selected={idx === selectedIndex}
            className={
              "flex w-full items-center gap-2.5 px-3 py-1.5 text-left transition-colors " +
              (idx === selectedIndex ? "bg-zinc-100" : "hover:bg-zinc-50")
            }
            onClick={() => selectItem(idx)}
            onMouseEnter={() => setSelectedIndex(idx)}
          >
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-blue-100 text-xs font-semibold text-blue-700">
              {item.name.charAt(0)}
            </span>
            <span className="text-sm text-zinc-700">{item.name}</span>
          </button>
        ))}
      </div>
    )
  },
)
MentionList.displayName = "MentionList"

// ---------------------------------------------------------------------------
// Floating wrapper (portal-based positioning)
// ---------------------------------------------------------------------------

interface FloatingWrapperProps {
  clientRect: (() => DOMRect | null) | null
  children: React.ReactNode
}

function FloatingWrapper({ clientRect, children }: FloatingWrapperProps) {
  const wrapperRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const el = wrapperRef.current
    if (!el || !clientRect) return

    const rect = clientRect()
    if (!rect) return

    el.style.position = "fixed"
    el.style.left = `${rect.left}px`
    el.style.top = `${rect.bottom + 4}px`
    el.style.zIndex = "50"
  })

  return createPortal(
    <div ref={wrapperRef} style={{ position: "fixed" }}>
      {children}
    </div>,
    document.body,
  )
}

// ---------------------------------------------------------------------------
// Suggestion render adapter
// ---------------------------------------------------------------------------

function createMentionRenderer() {
  let component: {
    ref: MentionListHandle | null
    updateProps: (props: SuggestionProps<MentionUser, MentionUser>) => void
    destroy: () => void
  } | null = null

  return {
    onStart(props: SuggestionProps<MentionUser, MentionUser>) {
      const container = document.createElement("div")
      document.body.appendChild(container)
      const root = createRoot(container)

      let currentRef: MentionListHandle | null = null

      const renderWith = (p: SuggestionProps<MentionUser, MentionUser>) => {
        root.render(
          <FloatingWrapper clientRect={p.clientRect ?? null}>
            <MentionList
              ref={(r) => {
                currentRef = r
              }}
              items={p.items}
              command={p.command}
            />
          </FloatingWrapper>,
        )
      }

      renderWith(props)

      component = {
        get ref() {
          return currentRef
        },
        updateProps: renderWith,
        destroy: () => {
          root.unmount()
          container.remove()
        },
      }
    },

    onUpdate(props: SuggestionProps<MentionUser, MentionUser>) {
      component?.updateProps(props)
    },

    onKeyDown(props: SuggestionKeyDownProps) {
      if (props.event.key === "Escape") return true
      return component?.ref?.onKeyDown(props.event) ?? false
    },

    onExit() {
      component?.destroy()
      component = null
    },
  }
}

// ---------------------------------------------------------------------------
// Mention Suggestion Extension
// ---------------------------------------------------------------------------

const mentionPluginKey = new PluginKey("mentionSuggestion")

export const MentionSuggestionExtension = Extension.create({
  name: "mentionSuggestion",

  addOptions() {
    return {
      suggestion: {
        char: "@",
        startOfLine: false,
        allowSpaces: true,
        allowedPrefixes: null,
        pluginKey: mentionPluginKey,
        command: ({
          editor,
          range,
          props: item,
        }: {
          editor: Editor
          range: Range
          props: MentionUser
        }) => {
          editor
            .chain()
            .focus()
            .deleteRange(range)
            .insertContent({
              type: "mention",
              attrs: { id: item.id, label: item.name },
            })
            .run()
        },
        items: ({ query }: { query: string }) => filterUsers(query),
        render: () => createMentionRenderer(),
      } satisfies Omit<SuggestionOptions<MentionUser, MentionUser>, "editor">,
    }
  },

  addProseMirrorPlugins() {
    return [
      Suggestion<MentionUser, MentionUser>({
        editor: this.editor,
        ...this.options.suggestion,
      }),
    ]
  },
})
