"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import type { Editor } from "@tiptap/react"
import { TextSelection } from "@tiptap/pm/state"
import {
  Scissors,
  Copy,
  ClipboardPaste,
  Trash2,
  CopyPlus,
  MoveUp,
  MoveDown,
  ChevronRight,
} from "lucide-react"
import {
  moveBlockUp,
  moveBlockDown,
  duplicateBlock,
  deleteBlock,
} from "./keyboard-shortcuts"

// ---------------------------------------------------------------------------
// "Turn into" block type definitions (same as block-handle.tsx)
// ---------------------------------------------------------------------------

interface TurnIntoItem {
  label: string
  action: (editor: Editor) => void
}

const TURN_INTO_ITEMS: TurnIntoItem[] = [
  {
    label: "Paragraph",
    action: (editor) => editor.chain().focus().setParagraph().run(),
  },
  {
    label: "Heading 1",
    action: (editor) =>
      editor.chain().focus().setHeading({ level: 1 }).run(),
  },
  {
    label: "Heading 2",
    action: (editor) =>
      editor.chain().focus().setHeading({ level: 2 }).run(),
  },
  {
    label: "Heading 3",
    action: (editor) =>
      editor.chain().focus().setHeading({ level: 3 }).run(),
  },
  {
    label: "Bullet List",
    action: (editor) => editor.chain().focus().toggleBulletList().run(),
  },
  {
    label: "Blockquote",
    action: (editor) => editor.chain().focus().toggleBlockquote().run(),
  },
  {
    label: "Code Block",
    action: (editor) => editor.chain().focus().toggleCodeBlock().run(),
  },
]

// ---------------------------------------------------------------------------
// Context menu state
// ---------------------------------------------------------------------------

interface MenuPosition {
  x: number
  y: number
}

// ---------------------------------------------------------------------------
// ContextMenu component
// ---------------------------------------------------------------------------

interface ContextMenuProps {
  editor: Editor
}

export function ContextMenu({ editor }: ContextMenuProps) {
  const [position, setPosition] = useState<MenuPosition | null>(null)
  const [turnIntoOpen, setTurnIntoOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  // Intercept right-click on the ProseMirror editor
  useEffect(() => {
    const editorEl = editor.view.dom

    const handleContextMenu = (e: MouseEvent) => {
      e.preventDefault()

      // Focus the cursor at the click position so block ops work on the right block
      const coords = editor.view.posAtCoords({ left: e.clientX, top: e.clientY })
      if (coords) {
        const resolved = editor.state.doc.resolve(coords.pos)
        const selection = TextSelection.near(resolved)
        editor.view.dispatch(editor.state.tr.setSelection(selection))
      }

      setPosition({ x: e.clientX, y: e.clientY })
      setTurnIntoOpen(false)
    }

    editorEl.addEventListener("contextmenu", handleContextMenu)
    return () => {
      editorEl.removeEventListener("contextmenu", handleContextMenu)
    }
  }, [editor])

  // Close on click outside
  useEffect(() => {
    if (!position) return
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setPosition(null)
        setTurnIntoOpen(false)
      }
    }
    document.addEventListener("mousedown", handleClick)
    return () => document.removeEventListener("mousedown", handleClick)
  }, [position])

  // Close on Escape
  useEffect(() => {
    if (!position) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setPosition(null)
        setTurnIntoOpen(false)
      }
    }
    document.addEventListener("keydown", handleKeyDown)
    return () => document.removeEventListener("keydown", handleKeyDown)
  }, [position])

  const close = useCallback(() => {
    setPosition(null)
    setTurnIntoOpen(false)
  }, [])

  const handleCut = useCallback(() => {
    document.execCommand("cut")
    close()
  }, [close])

  const handleCopy = useCallback(() => {
    document.execCommand("copy")
    close()
  }, [close])

  const handlePaste = useCallback(async () => {
    try {
      const text = await navigator.clipboard.readText()
      editor.chain().focus().insertContent(text).run()
    } catch {
      // Fallback for older browsers
      document.execCommand("paste")
    }
    close()
  }, [editor, close])

  const handleDeleteBlock = useCallback(() => {
    deleteBlock(editor)
    close()
  }, [editor, close])

  const handleDuplicateBlock = useCallback(() => {
    duplicateBlock(editor)
    close()
  }, [editor, close])

  const handleMoveUp = useCallback(() => {
    moveBlockUp(editor)
    close()
  }, [editor, close])

  const handleMoveDown = useCallback(() => {
    moveBlockDown(editor)
    close()
  }, [editor, close])

  const handleTurnInto = useCallback(
    (item: TurnIntoItem) => {
      item.action(editor)
      close()
    },
    [editor, close],
  )

  if (!position) return null

  return (
    <div
      ref={menuRef}
      className="fixed z-[100] w-52 rounded-md border border-zinc-200 bg-white py-1 shadow-lg"
      style={{ left: position.x, top: position.y }}
    >
      {/* Clipboard actions */}
      <MenuItem
        icon={<Scissors className="h-3.5 w-3.5" />}
        label="Cut"
        shortcut="Cmd+X"
        onClick={handleCut}
      />
      <MenuItem
        icon={<Copy className="h-3.5 w-3.5" />}
        label="Copy"
        shortcut="Cmd+C"
        onClick={handleCopy}
      />
      <MenuItem
        icon={<ClipboardPaste className="h-3.5 w-3.5" />}
        label="Paste"
        shortcut="Cmd+V"
        onClick={handlePaste}
      />

      <MenuSeparator />

      {/* Block actions */}
      <MenuItem
        icon={<Trash2 className="h-3.5 w-3.5" />}
        label="Delete Block"
        shortcut=""
        onClick={handleDeleteBlock}
      />
      <MenuItem
        icon={<CopyPlus className="h-3.5 w-3.5" />}
        label="Duplicate Block"
        shortcut=""
        onClick={handleDuplicateBlock}
      />

      <MenuSeparator />

      <MenuItem
        icon={<MoveUp className="h-3.5 w-3.5" />}
        label="Move Up"
        shortcut=""
        onClick={handleMoveUp}
      />
      <MenuItem
        icon={<MoveDown className="h-3.5 w-3.5" />}
        label="Move Down"
        shortcut=""
        onClick={handleMoveDown}
      />

      <MenuSeparator />

      {/* Turn into submenu */}
      <div className="relative">
        <button
          type="button"
          className={
            "flex w-full items-center justify-between px-3 py-1.5 text-left text-sm " +
            "text-zinc-700 transition-colors hover:bg-zinc-100 " +
            (turnIntoOpen ? "bg-zinc-50" : "")
          }
          onClick={() => setTurnIntoOpen(!turnIntoOpen)}
          onMouseEnter={() => setTurnIntoOpen(true)}
        >
          <span className="flex items-center gap-2">
            <span className="flex h-3.5 w-3.5 items-center justify-center text-[10px] font-bold text-zinc-500">
              T
            </span>
            <span>Turn Into...</span>
          </span>
          <ChevronRight className="h-3.5 w-3.5 text-zinc-400" />
        </button>
        {turnIntoOpen && (
          <div
            className={
              "absolute left-full top-0 z-[101] ml-1 w-40 rounded-md border border-zinc-200 " +
              "bg-white py-1 shadow-lg"
            }
          >
            {TURN_INTO_ITEMS.map((item) => (
              <button
                key={item.label}
                type="button"
                className={
                  "flex w-full items-center px-3 py-1.5 text-left text-sm " +
                  "text-zinc-700 transition-colors hover:bg-zinc-100"
                }
                onClick={() => handleTurnInto(item)}
              >
                {item.label}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Menu sub-components
// ---------------------------------------------------------------------------

interface MenuItemProps {
  icon: React.ReactNode
  label: string
  shortcut: string
  onClick: () => void
}

function MenuItem({ icon, label, shortcut, onClick }: MenuItemProps) {
  // Detect Mac vs PC for shortcut display
  const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.userAgent)
  const displayShortcut = shortcut
    ? shortcut.replace(/Cmd/g, isMac ? "⌘" : "Ctrl")
    : ""

  return (
    <button
      type="button"
      className={
        "flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm " +
        "text-zinc-700 transition-colors hover:bg-zinc-100"
      }
      onClick={onClick}
    >
      <span className="flex-shrink-0 text-zinc-500">{icon}</span>
      <span className="flex-1">{label}</span>
      {displayShortcut && (
        <span className="text-[10px] text-zinc-400">{displayShortcut}</span>
      )}
    </button>
  )
}

function MenuSeparator() {
  return <div className="my-1 h-px bg-zinc-100" />
}
