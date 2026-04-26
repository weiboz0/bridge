"use client"

import { useCallback, useState, useRef, useEffect } from "react"
import type { Editor } from "@tiptap/react"
import {
  Bold,
  Italic,
  Underline,
  Strikethrough,
  Code,
  Link,
  Highlighter,
  Type,
  List,
  ListOrdered,
  ListTodo,
  AlignLeft,
  AlignCenter,
  AlignRight,
  Plus,
  Undo2,
  Redo2,
  Check,
  Unlink,
  X,
  ChevronDown,
  HelpCircle,
  Monitor,
} from "lucide-react"
import { ALL_ITEMS, type SlashMenuItem } from "./slash-menu"

// ---------------------------------------------------------------------------
// Preset colors
// ---------------------------------------------------------------------------

const PRESET_COLORS = [
  { label: "Default", value: "" },
  { label: "Red", value: "#dc2626" },
  { label: "Orange", value: "#ea580c" },
  { label: "Yellow", value: "#ca8a04" },
  { label: "Green", value: "#16a34a" },
  { label: "Blue", value: "#2563eb" },
  { label: "Purple", value: "#9333ea" },
  { label: "Pink", value: "#db2777" },
] as const

// ---------------------------------------------------------------------------
// Toolbar button
// ---------------------------------------------------------------------------

interface ToolbarButtonProps {
  onClick: () => void
  active?: boolean
  disabled?: boolean
  title: string
  children: React.ReactNode
}

function ToolbarButton({ onClick, active, disabled, title, children }: ToolbarButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      aria-label={title}
      className={
        "flex h-7 w-7 items-center justify-center rounded text-zinc-600 transition-colors " +
        "hover:bg-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400 " +
        "disabled:cursor-not-allowed disabled:opacity-40 " +
        (active ? "bg-zinc-200 text-zinc-900" : "")
      }
    >
      {children}
    </button>
  )
}

function Separator() {
  return <div className="mx-1 h-5 w-px bg-zinc-200" aria-hidden="true" />
}

// ---------------------------------------------------------------------------
// Block type dropdown
// ---------------------------------------------------------------------------

interface BlockTypeDropdownProps {
  editor: Editor
}

const BLOCK_TYPES = [
  { label: "Text", type: "paragraph", level: undefined },
  { label: "Heading 1", type: "heading", level: 1 },
  { label: "Heading 2", type: "heading", level: 2 },
  { label: "Heading 3", type: "heading", level: 3 },
] as const

function BlockTypeDropdown({ editor }: BlockTypeDropdownProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const currentType = BLOCK_TYPES.find((bt) => {
    if (bt.type === "heading" && bt.level !== undefined) {
      return editor.isActive("heading", { level: bt.level })
    }
    return bt.type === "paragraph" && editor.isActive("paragraph")
  })

  useEffect(() => {
    if (!open) return
    const handleClickOutside = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener("mousedown", handleClickOutside)
    return () => document.removeEventListener("mousedown", handleClickOutside)
  }, [open])

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        title="Block type"
        aria-label="Block type"
        className={
          "flex h-7 items-center gap-1 rounded px-2 text-xs font-medium text-zinc-600 transition-colors " +
          "hover:bg-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400"
        }
      >
        {currentType?.label ?? "Text"}
        <ChevronDown className="h-3 w-3" />
      </button>
      {open && (
        <div className="absolute left-0 top-full z-50 mt-1 w-36 rounded-lg border border-zinc-200 bg-white py-1 shadow-lg">
          {BLOCK_TYPES.map((bt) => (
            <button
              key={bt.label}
              type="button"
              className={
                "flex w-full items-center px-3 py-1.5 text-left text-sm transition-colors " +
                "hover:bg-zinc-100 " +
                (currentType?.label === bt.label ? "bg-zinc-50 font-medium text-zinc-900" : "text-zinc-600")
              }
              onClick={() => {
                if (bt.type === "heading" && bt.level !== undefined) {
                  editor.chain().focus().setHeading({ level: bt.level as 1 | 2 | 3 }).run()
                } else {
                  editor.chain().focus().setParagraph().run()
                }
                setOpen(false)
              }}
            >
              {bt.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Insert dropdown (slash menu items as a dropdown)
// ---------------------------------------------------------------------------

interface InsertDropdownProps {
  editor: Editor
}

function InsertDropdown({ editor }: InsertDropdownProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handleClickOutside = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener("mousedown", handleClickOutside)
    return () => document.removeEventListener("mousedown", handleClickOutside)
  }, [open])

  const handleSelect = useCallback(
    (item: SlashMenuItem) => {
      // Create a range at the end of the current selection
      const { to } = editor.state.selection
      item.command({ editor, range: { from: to, to } })
      setOpen(false)
    },
    [editor],
  )

  // Group items by category
  const aiItems = ALL_ITEMS.filter((i) => i.category === "ai")
  const textItems = ALL_ITEMS.filter((i) => i.category === "text")
  const teachingItems = ALL_ITEMS.filter((i) => i.category === "teaching")

  const renderCategory = (label: string, items: SlashMenuItem[]) => {
    if (items.length === 0) return null
    return (
      <div key={label}>
        <p className="px-3 pb-1 pt-2 text-[10px] font-medium uppercase tracking-wider text-zinc-400">
          {label}
        </p>
        {items.map((item) => (
          <button
            key={item.id}
            type="button"
            className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-zinc-600 transition-colors hover:bg-zinc-100"
            onClick={() => handleSelect(item)}
          >
            <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded border border-zinc-200 bg-zinc-50 text-[9px] font-semibold uppercase text-zinc-400">
              {item.badge}
            </span>
            <span>{item.label}</span>
          </button>
        ))}
      </div>
    )
  }

  return (
    <div ref={ref} className="relative">
      <ToolbarButton
        onClick={() => setOpen(!open)}
        title="Insert block"
      >
        <Plus className="h-3.5 w-3.5" />
      </ToolbarButton>
      {open && (
        <div className="absolute left-0 top-full z-50 mt-1 max-h-72 w-52 overflow-y-auto rounded-lg border border-zinc-200 bg-white py-1 shadow-lg">
          {renderCategory("AI", aiItems)}
          {renderCategory("Text", textItems)}
          {renderCategory("Teaching", teachingItems)}
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Link input (inline in toolbar)
// ---------------------------------------------------------------------------

interface ToolbarLinkInputProps {
  editor: Editor
  onClose: () => void
}

function ToolbarLinkInput({ editor, onClose }: ToolbarLinkInputProps) {
  const currentUrl = editor.getAttributes("link").href ?? ""
  const [url, setUrl] = useState(currentUrl)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    setTimeout(() => inputRef.current?.focus(), 0)
  }, [])

  const apply = useCallback(() => {
    if (!url.trim()) {
      editor.chain().focus().unsetLink().run()
    } else {
      editor.chain().focus().setLink({ href: url.trim() }).run()
    }
    onClose()
  }, [editor, url, onClose])

  return (
    <div className="flex items-center gap-1">
      <input
        ref={inputRef}
        type="url"
        placeholder="https://..."
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") { e.preventDefault(); apply() }
          if (e.key === "Escape") { e.preventDefault(); onClose() }
        }}
        className="h-6 w-44 rounded border border-zinc-200 bg-white px-2 text-xs outline-none focus:border-zinc-400"
      />
      <ToolbarButton onClick={apply} title="Apply link">
        <Check className="h-3.5 w-3.5" />
      </ToolbarButton>
      {currentUrl && (
        <ToolbarButton
          onClick={() => { editor.chain().focus().unsetLink().run(); onClose() }}
          title="Remove link"
        >
          <Unlink className="h-3.5 w-3.5" />
        </ToolbarButton>
      )}
      <ToolbarButton onClick={onClose} title="Cancel">
        <X className="h-3.5 w-3.5" />
      </ToolbarButton>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Color picker (inline in toolbar)
// ---------------------------------------------------------------------------

interface ToolbarColorPickerProps {
  editor: Editor
  onClose: () => void
}

function ToolbarColorPicker({ editor, onClose }: ToolbarColorPickerProps) {
  const currentColor = editor.getAttributes("textStyle").color ?? ""

  return (
    <div className="flex items-center gap-0.5">
      {PRESET_COLORS.map((c) => (
        <button
          key={c.label}
          type="button"
          title={c.label}
          aria-label={`Text color: ${c.label}`}
          onClick={() => {
            if (c.value) {
              editor.chain().focus().setColor(c.value).run()
            } else {
              editor.chain().focus().unsetColor().run()
            }
            onClose()
          }}
          className={
            "flex h-6 w-6 items-center justify-center rounded transition-colors " +
            "hover:bg-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400 " +
            (currentColor === c.value ? "ring-2 ring-zinc-400" : "")
          }
        >
          {c.value ? (
            <span
              className="h-3.5 w-3.5 rounded-full border border-zinc-200"
              style={{ backgroundColor: c.value }}
            />
          ) : (
            <span className="text-[10px] font-medium text-zinc-500">A</span>
          )}
        </button>
      ))}
      <ToolbarButton onClick={onClose} title="Close color picker">
        <X className="h-3.5 w-3.5" />
      </ToolbarButton>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Keyboard shortcuts modal
// ---------------------------------------------------------------------------

const SHORTCUT_GROUPS = [
  {
    label: "Formatting",
    items: [
      { keys: "Cmd+B", desc: "Bold" },
      { keys: "Cmd+I", desc: "Italic" },
      { keys: "Cmd+U", desc: "Underline" },
      { keys: "Cmd+Shift+X", desc: "Strikethrough" },
      { keys: "Cmd+E", desc: "Inline code" },
      { keys: "Cmd+K", desc: "Link" },
    ],
  },
  {
    label: "Blocks",
    items: [
      { keys: "Cmd+Shift+Up", desc: "Move block up" },
      { keys: "Cmd+Shift+Down", desc: "Move block down" },
      { keys: "Cmd+Shift+D", desc: "Duplicate block" },
      { keys: "Cmd+Shift+Delete", desc: "Delete block" },
    ],
  },
  {
    label: "Navigation",
    items: [
      { keys: "Cmd+Enter", desc: "Save" },
      { keys: "Cmd+Z", desc: "Undo" },
      { keys: "Cmd+Shift+Z", desc: "Redo" },
      { keys: "/", desc: "Slash menu" },
    ],
  },
  {
    label: "Lists",
    items: [
      { keys: "Tab", desc: "Indent" },
      { keys: "Shift+Tab", desc: "Outdent" },
    ],
  },
] as const

function ShortcutsModal({ onClose }: { onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose()
    }
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    document.addEventListener("keydown", handleKey)
    document.addEventListener("mousedown", handleClick)
    return () => {
      document.removeEventListener("keydown", handleKey)
      document.removeEventListener("mousedown", handleClick)
    }
  }, [onClose])

  // Detect Mac vs PC for shortcut display
  const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.userAgent)
  const modKey = isMac ? "⌘" : "Ctrl"

  const formatKeys = (keys: string) =>
    keys
      .replace(/Cmd/g, modKey)
      .replace(/Shift/g, isMac ? "⇧" : "Shift")
      .replace(/Delete/g, isMac ? "⌫" : "Del")
      .replace(/Up/g, "↑")
      .replace(/Down/g, "↓")
      .replace(/Enter/g, "↵")

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/20">
      <div
        ref={ref}
        className="w-80 rounded-lg border border-zinc-200 bg-white p-4 shadow-xl"
      >
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-sm font-semibold text-zinc-900">Keyboard Shortcuts</h3>
          <button
            type="button"
            onClick={onClose}
            className="flex h-6 w-6 items-center justify-center rounded text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600"
            aria-label="Close"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="space-y-3">
          {SHORTCUT_GROUPS.map((group) => (
            <div key={group.label}>
              <p className="mb-1 text-[10px] font-medium uppercase tracking-wider text-zinc-400">
                {group.label}
              </p>
              <div className="space-y-0.5">
                {group.items.map((item) => (
                  <div
                    key={item.keys}
                    className="flex items-center justify-between py-0.5 text-xs"
                  >
                    <span className="text-zinc-600">{item.desc}</span>
                    <kbd className="rounded bg-zinc-100 px-1.5 py-0.5 font-mono text-[10px] text-zinc-500">
                      {formatKeys(item.keys)}
                    </kbd>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main editor toolbar
// ---------------------------------------------------------------------------

interface EditorToolbarProps {
  editor: Editor
  /** Collaborative status: connected/disconnected/null (non-collaborative) */
  collaborative?: { connected: boolean }
  onSave: () => void
  onToggleAI?: () => void
  showAI?: boolean
  unitId?: string
  /** Show the first-time help overlay. */
  onShowHelp?: () => void
  /** Whether presentation mode is active. */
  presentationMode?: boolean
  /** Toggle presentation mode. */
  onTogglePresentation?: () => void
  /** Current page width: standard, wide, or full. */
  pageWidth?: "standard" | "wide" | "full"
  /** Set page width. */
  onSetPageWidth?: (w: "standard" | "wide" | "full") => void
}

export function EditorToolbar({
  editor,
  collaborative,
  onSave,
  onToggleAI,
  showAI,
  unitId,
  onShowHelp,
  presentationMode,
  onTogglePresentation,
  pageWidth = "standard",
  onSetPageWidth,
}: EditorToolbarProps) {
  const [showLinkInput, setShowLinkInput] = useState(false)
  const [showColorPicker, setShowColorPicker] = useState(false)
  const [showShortcuts, setShowShortcuts] = useState(false)

  return (
    <div className="flex flex-wrap items-center gap-0.5 rounded-t-lg border border-zinc-200 bg-white px-2 py-1">
      {/* Block type dropdown */}
      <BlockTypeDropdown editor={editor} />

      <Separator />

      {/* Inline formatting */}
      <ToolbarButton
        onClick={() => editor.chain().focus().toggleBold().run()}
        active={editor.isActive("bold")}
        title="Bold (Cmd+B)"
      >
        <Bold className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        onClick={() => editor.chain().focus().toggleItalic().run()}
        active={editor.isActive("italic")}
        title="Italic (Cmd+I)"
      >
        <Italic className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        onClick={() => editor.chain().focus().toggleUnderline().run()}
        active={editor.isActive("underline")}
        title="Underline (Cmd+U)"
      >
        <Underline className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        onClick={() => editor.chain().focus().toggleStrike().run()}
        active={editor.isActive("strike")}
        title="Strikethrough (Cmd+Shift+X)"
      >
        <Strikethrough className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        onClick={() => editor.chain().focus().toggleCode().run()}
        active={editor.isActive("code")}
        title="Inline code (Cmd+E)"
      >
        <Code className="h-3.5 w-3.5" />
      </ToolbarButton>

      <Separator />

      {/* Link */}
      {showLinkInput ? (
        <ToolbarLinkInput editor={editor} onClose={() => setShowLinkInput(false)} />
      ) : (
        <ToolbarButton
          onClick={() => setShowLinkInput(true)}
          active={editor.isActive("link")}
          title="Link (Cmd+K)"
        >
          <Link className="h-3.5 w-3.5" />
        </ToolbarButton>
      )}

      {/* Highlight */}
      <ToolbarButton
        onClick={() => editor.chain().focus().toggleHighlight().run()}
        active={editor.isActive("highlight")}
        title="Highlight"
      >
        <Highlighter className="h-3.5 w-3.5" />
      </ToolbarButton>

      {/* Color */}
      {showColorPicker ? (
        <ToolbarColorPicker editor={editor} onClose={() => setShowColorPicker(false)} />
      ) : (
        <ToolbarButton
          onClick={() => setShowColorPicker(true)}
          title="Text color"
        >
          <Type className="h-3.5 w-3.5" />
        </ToolbarButton>
      )}

      <Separator />

      {/* Lists */}
      <ToolbarButton
        onClick={() => editor.chain().focus().toggleBulletList().run()}
        active={editor.isActive("bulletList")}
        title="Bullet list"
      >
        <List className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        onClick={() => editor.chain().focus().toggleOrderedList().run()}
        active={editor.isActive("orderedList")}
        title="Numbered list"
      >
        <ListOrdered className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        onClick={() => editor.chain().focus().toggleTaskList().run()}
        active={editor.isActive("taskList")}
        title="Task list"
      >
        <ListTodo className="h-3.5 w-3.5" />
      </ToolbarButton>

      <Separator />

      {/* Text align */}
      <ToolbarButton
        onClick={() => editor.chain().focus().setTextAlign("left").run()}
        active={editor.isActive({ textAlign: "left" })}
        title="Align left"
      >
        <AlignLeft className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        onClick={() => editor.chain().focus().setTextAlign("center").run()}
        active={editor.isActive({ textAlign: "center" })}
        title="Align center"
      >
        <AlignCenter className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        onClick={() => editor.chain().focus().setTextAlign("right").run()}
        active={editor.isActive({ textAlign: "right" })}
        title="Align right"
      >
        <AlignRight className="h-3.5 w-3.5" />
      </ToolbarButton>

      <Separator />

      {/* Insert dropdown */}
      <InsertDropdown editor={editor} />

      <Separator />

      {/* Undo / Redo */}
      <ToolbarButton
        onClick={() => editor.chain().focus().undo().run()}
        disabled={!editor.can().undo()}
        title="Undo (Cmd+Z)"
      >
        <Undo2 className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        onClick={() => editor.chain().focus().redo().run()}
        disabled={!editor.can().redo()}
        title="Redo (Cmd+Shift+Z)"
      >
        <Redo2 className="h-3.5 w-3.5" />
      </ToolbarButton>

      {/* Shortcuts + Help */}
      <ToolbarButton
        onClick={() => setShowShortcuts(true)}
        title="Keyboard shortcuts"
      >
        <HelpCircle className="h-3.5 w-3.5" />
      </ToolbarButton>

      {showShortcuts && <ShortcutsModal onClose={() => setShowShortcuts(false)} />}

      {/* Right side: Dark mode, Present, AI, Help, Save, Collab status */}
      <div className="ml-auto flex items-center gap-2">
        {/* Page width selector */}
        {onSetPageWidth && (
          <div className="flex items-center rounded-md border border-zinc-200">
            {(["standard", "wide", "full"] as const).map((w) => (
              <button
                key={w}
                type="button"
                onClick={() => onSetPageWidth(w)}
                className={`px-2 py-1 text-[10px] font-medium transition-colors ${
                  pageWidth === w
                    ? "bg-zinc-200 text-zinc-900"
                    : "text-zinc-500 hover:text-zinc-700"
                } ${w === "standard" ? "rounded-l-md" : w === "full" ? "rounded-r-md" : ""}`}
                title={`${w.charAt(0).toUpperCase() + w.slice(1)} width`}
              >
                {w === "standard" ? "S" : w === "wide" ? "W" : "F"}
              </button>
            ))}
          </div>
        )}
        {onTogglePresentation && (
          <ToolbarButton
            onClick={onTogglePresentation}
            active={presentationMode}
            title="Presentation mode"
          >
            <Monitor className="h-3.5 w-3.5" />
          </ToolbarButton>
        )}
        {onShowHelp && (
          <button
            type="button"
            onClick={onShowHelp}
            className={
              "flex h-7 items-center gap-1 rounded px-2 text-xs font-medium text-zinc-600 transition-colors " +
              "hover:bg-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400"
            }
            title="Editor help"
            aria-label="Editor help"
          >
            ?
          </button>
        )}
        {unitId && onToggleAI && (
          <button
            type="button"
            onClick={onToggleAI}
            className={
              "flex h-7 items-center gap-1 rounded px-2 text-xs font-medium transition-colors " +
              "hover:bg-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400 " +
              (showAI ? "bg-zinc-200 text-zinc-900" : "text-zinc-600")
            }
            title="Draft with AI"
            aria-label="Draft with AI"
          >
            {showAI ? "Close AI" : "Draft with AI"}
          </button>
        )}
        <button
          type="button"
          onClick={onSave}
          className={
            "flex h-7 items-center rounded bg-zinc-900 px-3 text-xs font-medium text-white transition-colors " +
            "hover:bg-zinc-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400"
          }
          title="Save (Cmd+S)"
          aria-label="Save"
        >
          Save
        </button>
        {collaborative && (
          <span
            className={
              collaborative.connected
                ? "text-xs font-medium text-green-600"
                : "text-xs font-medium text-zinc-400"
            }
          >
            {collaborative.connected ? "Live" : "Connecting..."}
          </span>
        )}
      </div>
    </div>
  )
}
