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
import { Extension } from "@tiptap/core"
import Suggestion, { type SuggestionOptions, type SuggestionProps, type SuggestionKeyDownProps } from "@tiptap/suggestion"
import { nanoid } from "nanoid"
import type { Editor, Range } from "@tiptap/core"
import { PluginKey } from "@tiptap/pm/state"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SlashMenuItem {
  id: string
  label: string
  description: string
  /** A short text badge shown to the left of the label (e.g. "H1", "P"). */
  badge: string
  category: "text" | "teaching" | "ai"
  /** Execute the command on the editor, replacing the slash trigger range. */
  command: (props: { editor: Editor; range: Range }) => void
}

// ---------------------------------------------------------------------------
// Item definitions
// ---------------------------------------------------------------------------

const TEXT_ITEMS: SlashMenuItem[] = [
  {
    id: "heading1",
    label: "Heading 1",
    description: "Large section heading",
    badge: "H1",
    category: "text",
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setHeading({ level: 1 }).run()
    },
  },
  {
    id: "heading2",
    label: "Heading 2",
    description: "Medium section heading",
    badge: "H2",
    category: "text",
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setHeading({ level: 2 }).run()
    },
  },
  {
    id: "heading3",
    label: "Heading 3",
    description: "Small section heading",
    badge: "H3",
    category: "text",
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setHeading({ level: 3 }).run()
    },
  },
  {
    id: "bulletList",
    label: "Bullet List",
    description: "Unordered list with bullets",
    badge: "UL",
    category: "text",
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).toggleBulletList().run()
    },
  },
  {
    id: "orderedList",
    label: "Numbered List",
    description: "Ordered list with numbers",
    badge: "OL",
    category: "text",
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).toggleOrderedList().run()
    },
  },
  {
    id: "blockquote",
    label: "Blockquote",
    description: "Indented quote block",
    badge: "BQ",
    category: "text",
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).toggleBlockquote().run()
    },
  },
  {
    id: "codeBlock",
    label: "Code Block",
    description: "Syntax-highlighted code",
    badge: "<>",
    category: "text",
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).toggleCodeBlock().run()
    },
  },
  {
    id: "divider",
    label: "Divider",
    description: "Horizontal rule separator",
    badge: "--",
    category: "text",
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setHorizontalRule().run()
    },
  },
]

const TEACHING_ITEMS: SlashMenuItem[] = [
  {
    id: "problem",
    label: "Problem",
    description: "Embed a problem reference",
    badge: "PR",
    category: "teaching",
    command: ({ editor, range }) => {
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: "problem-ref",
          attrs: {
            id: nanoid(),
            problemId: "",
            pinnedRevision: null,
            visibility: "always",
            overrideStarter: null,
          },
        })
        .run()
    },
  },
  {
    id: "teacherNote",
    label: "Teacher Note",
    description: "Note visible only to teachers",
    badge: "TN",
    category: "teaching",
    command: ({ editor, range }) => {
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: "teacher-note",
          attrs: { id: nanoid() },
          content: [{ type: "paragraph" }],
        })
        .run()
    },
  },
  {
    id: "codeSnippet",
    label: "Code Snippet",
    description: "Editable code block with language",
    badge: "CS",
    category: "teaching",
    command: ({ editor, range }) => {
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: "code-snippet",
          attrs: { id: nanoid(), language: "python", code: "" },
        })
        .run()
    },
  },
  {
    id: "media",
    label: "Media",
    description: "Image, video, or file embed",
    badge: "ME",
    category: "teaching",
    command: ({ editor, range }) => {
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: "media-embed",
          attrs: { id: nanoid(), url: "", alt: "", mediaType: "image" },
        })
        .run()
    },
  },
  {
    id: "solution",
    label: "Solution",
    description: "Reference to a solution",
    badge: "SL",
    category: "teaching",
    command: ({ editor, range }) => {
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: "solution-ref",
          attrs: { id: nanoid(), solutionId: "", reveal: "always" },
        })
        .run()
    },
  },
  {
    id: "liveCue",
    label: "Live Cue",
    description: "Timed instructor cue",
    badge: "LC",
    category: "teaching",
    command: ({ editor, range }) => {
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: "live-cue",
          attrs: { id: nanoid() },
          content: [{ type: "paragraph" }],
        })
        .run()
    },
  },
  {
    id: "assignment",
    label: "Assignment",
    description: "Assignment variant block",
    badge: "AS",
    category: "teaching",
    command: ({ editor, range }) => {
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: "assignment-variant",
          attrs: { id: nanoid(), title: "", timeLimitMinutes: null },
          content: [{ type: "paragraph" }],
        })
        .run()
    },
  },
]

const AI_ITEMS: SlashMenuItem[] = [
  {
    id: "aiWriter",
    label: "AI Writer",
    description: "Generate content with AI inline",
    badge: "AI",
    category: "ai",
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).run()
      // Signal the editor to show the inline AI prompt at the current cursor position
      window.dispatchEvent(new CustomEvent("tiptap:ai-write-inline"))
    },
  },
]

export const ALL_ITEMS: SlashMenuItem[] = [...AI_ITEMS, ...TEXT_ITEMS, ...TEACHING_ITEMS]

// ---------------------------------------------------------------------------
// Filter logic
// ---------------------------------------------------------------------------

function filterItems(query: string): SlashMenuItem[] {
  if (!query) return ALL_ITEMS
  const q = query.toLowerCase()
  return ALL_ITEMS.filter(
    (item) =>
      item.label.toLowerCase().includes(q) ||
      item.description.toLowerCase().includes(q) ||
      item.id.toLowerCase().includes(q),
  )
}

// ---------------------------------------------------------------------------
// Floating menu React component
// ---------------------------------------------------------------------------

interface SlashMenuListProps {
  items: SlashMenuItem[]
  command: (item: SlashMenuItem) => void
}

export interface SlashMenuListHandle {
  onKeyDown: (event: KeyboardEvent) => boolean
}

export const SlashMenuList = forwardRef<SlashMenuListHandle, SlashMenuListProps>(
  function SlashMenuList({ items, command }, ref) {
    const [selectedIndex, setSelectedIndex] = useState(0)
    const containerRef = useRef<HTMLDivElement>(null)

    // Reset selection when items change.
    useEffect(() => {
      setSelectedIndex(0)
    }, [items])

    // Scroll the selected item into view.
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
        if (item) {
          command(item)
        }
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
        <div className="rounded-lg border border-zinc-200 bg-white px-3 py-4 text-center text-sm text-zinc-400 shadow-lg">
          No matching commands
        </div>
      )
    }

    // Group items by category for rendering.
    const aiItems = items.filter((i) => i.category === "ai")
    const textItems = items.filter((i) => i.category === "text")
    const teachingItems = items.filter((i) => i.category === "teaching")

    // Build a flat index map so arrow keys navigate across categories.
    let flatIndex = 0
    const renderCategory = (label: string, categoryItems: SlashMenuItem[]) => {
      if (categoryItems.length === 0) return null
      const rows = categoryItems.map((item) => {
        const idx = flatIndex++
        return (
          <button
            key={item.id}
            type="button"
            data-selected={idx === selectedIndex}
            className={
              "flex w-full items-center gap-3 px-3 py-2 text-left transition-colors " +
              (idx === selectedIndex ? "bg-zinc-100" : "hover:bg-zinc-50")
            }
            onClick={() => selectItem(idx)}
            onMouseEnter={() => setSelectedIndex(idx)}
          >
            <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded border border-zinc-200 bg-zinc-50 text-[10px] font-semibold uppercase text-zinc-500">
              {item.badge}
            </span>
            <div className="min-w-0">
              <p className="text-sm font-medium text-zinc-900">{item.label}</p>
              <p className="text-xs text-zinc-500 truncate">{item.description}</p>
            </div>
          </button>
        )
      })
      return (
        <div key={label}>
          <p className="px-3 pb-1 pt-3 text-xs font-medium uppercase tracking-wider text-zinc-500">
            {label}
          </p>
          {rows}
        </div>
      )
    }

    return (
      <div
        ref={containerRef}
        className="max-h-80 w-64 overflow-y-auto rounded-lg border border-zinc-200 bg-white shadow-lg"
      >
        {renderCategory("AI", aiItems)}
        {renderCategory("Text formatting", textItems)}
        {renderCategory("Teaching blocks", teachingItems)}
      </div>
    )
  },
)
SlashMenuList.displayName = "SlashMenuList"

// ---------------------------------------------------------------------------
// Floating wrapper — positions a portal at the cursor
// ---------------------------------------------------------------------------

interface FloatingMenuWrapperProps {
  clientRect: (() => DOMRect | null) | null
  children: React.ReactNode
}

function FloatingMenuWrapper({ clientRect, children }: FloatingMenuWrapperProps) {
  const wrapperRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const el = wrapperRef.current
    if (!el || !clientRect) return

    const rect = clientRect()
    if (!rect) return

    // Position below the cursor, aligned left.
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
// Suggestion render adapter — bridges @tiptap/suggestion to React
// ---------------------------------------------------------------------------

/**
 * Creates the `render` callback for `@tiptap/suggestion`. This function
 * manages a root rendered into a portal via React 18 `createRoot`.
 */
function createSuggestionRenderer() {
  let component: {
    ref: SlashMenuListHandle | null
    updateProps: (props: SuggestionProps<SlashMenuItem, SlashMenuItem>) => void
    destroy: () => void
  } | null = null

  return {
    onStart(props: SuggestionProps<SlashMenuItem, SlashMenuItem>) {
      // We need to render a React tree into a portal. Since we're inside
      // Tiptap (not a React render cycle), we use a small wrapper rendered
      // into a detached DOM node via ReactDOM.createRoot.
      const container = document.createElement("div")
      document.body.appendChild(container)

      const root = createRoot(container)

      let currentRef: SlashMenuListHandle | null = null

      const renderWith = (p: SuggestionProps<SlashMenuItem, SlashMenuItem>) => {
        root.render(
          <FloatingMenuWrapper clientRect={p.clientRect ?? null}>
            <SlashMenuList
              ref={(r) => {
                currentRef = r
              }}
              items={p.items}
              command={p.command}
            />
          </FloatingMenuWrapper>,
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

    onUpdate(props: SuggestionProps<SlashMenuItem, SlashMenuItem>) {
      component?.updateProps(props)
    },

    onKeyDown(props: SuggestionKeyDownProps) {
      if (props.event.key === "Escape") {
        return true
      }
      return component?.ref?.onKeyDown(props.event) ?? false
    },

    onExit() {
      component?.destroy()
      component = null
    },
  }
}

// ---------------------------------------------------------------------------
// Tiptap Extension
// ---------------------------------------------------------------------------

const slashCommandPluginKey = new PluginKey("slashCommand")

export const SlashCommandExtension = Extension.create({
  name: "slashCommand",

  addOptions() {
    return {
      suggestion: {
        char: "/",
        startOfLine: false,
        allowSpaces: false,
        allowedPrefixes: null,
        pluginKey: slashCommandPluginKey,
        command: ({
          editor,
          range,
          props: item,
        }: {
          editor: Editor
          range: Range
          props: SlashMenuItem
        }) => {
          item.command({ editor, range })
        },
        items: ({ query }: { query: string }) => filterItems(query),
        render: () => createSuggestionRenderer(),
      } satisfies Omit<SuggestionOptions<SlashMenuItem, SlashMenuItem>, "editor">,
    }
  },

  addProseMirrorPlugins() {
    return [
      Suggestion<SlashMenuItem, SlashMenuItem>({
        editor: this.editor,
        ...this.options.suggestion,
      }),
    ]
  },
})
