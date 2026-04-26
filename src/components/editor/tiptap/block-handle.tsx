"use client"

import {
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react"
import type { Editor } from "@tiptap/react"
import { TextSelection } from "@tiptap/pm/state"
import {
  GripVertical,
  Trash2,
  Copy,
  MoveUp,
  MoveDown,
  ChevronRight,
  Plus,
} from "lucide-react"
import {
  moveBlockUp,
  moveBlockDown,
  duplicateBlock,
  deleteBlock,
} from "./keyboard-shortcuts"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface HoveredBlock {
  /** ProseMirror position of the top-level node. */
  pos: number
  /** Bounding rect of the block's DOM element, relative to viewport. */
  rect: DOMRect
  /** Whether this block is an empty paragraph (for showing "+" instead). */
  isEmpty: boolean
}

// ---------------------------------------------------------------------------
// "Turn into" block type definitions
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
// Helpers
// ---------------------------------------------------------------------------

/**
 * Given a mouse event inside the editor, resolve the hovered top-level block.
 * Returns null if the pointer is not over any block content.
 */
function resolveHoveredBlock(
  editor: Editor,
  clientX: number,
  clientY: number,
): HoveredBlock | null {
  const coords = editor.view.posAtCoords({ left: clientX, top: clientY })
  if (!coords) return null

  const resolved = editor.state.doc.resolve(coords.pos)
  // depth 0 = doc, depth 1 = top-level block
  if (resolved.depth < 1) return null

  const topPos = resolved.before(1)
  const topNode = editor.state.doc.nodeAt(topPos)
  if (!topNode) return null

  // Get the DOM element for the block
  const dom = editor.view.nodeDOM(topPos)
  if (!dom || !(dom instanceof HTMLElement)) return null

  const rect = dom.getBoundingClientRect()

  // Check if the block is an "empty" paragraph (no text content)
  const isEmpty =
    topNode.type.name === "paragraph" &&
    topNode.content.size === 0

  return { pos: topPos, rect, isEmpty }
}

// ---------------------------------------------------------------------------
// BlockHandle component
// ---------------------------------------------------------------------------

interface BlockHandleProps {
  editor: Editor
}

export function BlockHandle({ editor }: BlockHandleProps) {
  const [hovered, setHovered] = useState<HoveredBlock | null>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const [turnIntoOpen, setTurnIntoOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const handleRef = useRef<HTMLDivElement>(null)
  const editorContainerRef = useRef<HTMLElement | null>(null)
  // Track the last hovered pos so we can keep handle stable when menu is open
  const lockedPosRef = useRef<number | null>(null)

  // Resolve the editor container once
  useEffect(() => {
    editorContainerRef.current = editor.view.dom.closest(
      ".ProseMirror"
    ) as HTMLElement | null
  }, [editor])

  // Track mouse position over the editor to find hovered blocks
  useEffect(() => {
    const editorEl = editor.view.dom

    const handleMouseMove = (e: MouseEvent) => {
      // Don't update hover while the actions menu is open
      if (menuOpen) return

      const block = resolveHoveredBlock(editor, e.clientX, e.clientY)
      if (!block) {
        // Only clear if we've moved far enough away from the handle itself
        if (handleRef.current) {
          const handleRect = handleRef.current.getBoundingClientRect()
          const isOverHandle =
            e.clientX >= handleRect.left - 8 &&
            e.clientX <= handleRect.right + 8 &&
            e.clientY >= handleRect.top - 4 &&
            e.clientY <= handleRect.bottom + 4
          if (isOverHandle) return
        }
        setHovered(null)
        return
      }

      setHovered(block)
    }

    const handleMouseLeave = (e: MouseEvent) => {
      if (menuOpen) return
      // Check if we moved to the handle itself
      if (handleRef.current) {
        const related = e.relatedTarget as Node | null
        if (related && handleRef.current.contains(related)) return
      }
      setHovered(null)
    }

    editorEl.addEventListener("mousemove", handleMouseMove)
    editorEl.addEventListener("mouseleave", handleMouseLeave)
    return () => {
      editorEl.removeEventListener("mousemove", handleMouseMove)
      editorEl.removeEventListener("mouseleave", handleMouseLeave)
    }
  }, [editor, menuOpen])

  // Close menu when clicking outside
  useEffect(() => {
    if (!menuOpen) return
    const handleClickOutside = (e: MouseEvent) => {
      if (
        menuRef.current &&
        !menuRef.current.contains(e.target as Node) &&
        handleRef.current &&
        !handleRef.current.contains(e.target as Node)
      ) {
        setMenuOpen(false)
        setTurnIntoOpen(false)
        lockedPosRef.current = null
      }
    }
    document.addEventListener("mousedown", handleClickOutside)
    return () => document.removeEventListener("mousedown", handleClickOutside)
  }, [menuOpen])

  // Close menu on Escape
  useEffect(() => {
    if (!menuOpen) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setMenuOpen(false)
        setTurnIntoOpen(false)
        lockedPosRef.current = null
      }
    }
    document.addEventListener("keydown", handleKeyDown)
    return () => document.removeEventListener("keydown", handleKeyDown)
  }, [menuOpen])

  // Focus the block at `pos` in the editor
  const focusBlock = useCallback(
    (pos: number) => {
      try {
        const resolvedPos = editor.state.doc.resolve(pos + 1)
        editor.view.dispatch(
          editor.state.tr.setSelection(
            TextSelection.near(resolvedPos)
          )
        )
        editor.view.focus()
      } catch {
        // If pos is invalid, just focus the editor
        editor.view.focus()
      }
    },
    [editor],
  )

  // Actions
  const handleDelete = useCallback(() => {
    const pos = lockedPosRef.current ?? hovered?.pos
    if (pos == null) return
    focusBlock(pos)
    deleteBlock(editor)
    setMenuOpen(false)
    setTurnIntoOpen(false)
    lockedPosRef.current = null
    setHovered(null)
  }, [editor, hovered, focusBlock])

  const handleDuplicate = useCallback(() => {
    const pos = lockedPosRef.current ?? hovered?.pos
    if (pos == null) return
    focusBlock(pos)
    duplicateBlock(editor)
    setMenuOpen(false)
    setTurnIntoOpen(false)
    lockedPosRef.current = null
  }, [editor, hovered, focusBlock])

  const handleMoveUp = useCallback(() => {
    const pos = lockedPosRef.current ?? hovered?.pos
    if (pos == null) return
    focusBlock(pos)
    moveBlockUp(editor)
    setMenuOpen(false)
    setTurnIntoOpen(false)
    lockedPosRef.current = null
    setHovered(null)
  }, [editor, hovered, focusBlock])

  const handleMoveDown = useCallback(() => {
    const pos = lockedPosRef.current ?? hovered?.pos
    if (pos == null) return
    focusBlock(pos)
    moveBlockDown(editor)
    setMenuOpen(false)
    setTurnIntoOpen(false)
    lockedPosRef.current = null
    setHovered(null)
  }, [editor, hovered, focusBlock])

  const handleTurnInto = useCallback(
    (item: TurnIntoItem) => {
      const pos = lockedPosRef.current ?? hovered?.pos
      if (pos == null) return
      focusBlock(pos)
      item.action(editor)
      setMenuOpen(false)
      setTurnIntoOpen(false)
      lockedPosRef.current = null
    },
    [editor, hovered, focusBlock],
  )

  // Handle "+" button on empty paragraphs — trigger slash menu
  const handlePlusClick = useCallback(() => {
    if (!hovered) return
    // Focus the empty paragraph and type "/" to trigger the slash menu
    focusBlock(hovered.pos)
    // Insert a "/" character to trigger the slash menu extension
    editor.chain().focus().insertContent("/").run()
  }, [editor, hovered, focusBlock])

  // Handle click on the grip to open the actions menu
  const handleGripClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      if (hovered) {
        lockedPosRef.current = hovered.pos
      }
      setMenuOpen((prev) => {
        if (prev) {
          setTurnIntoOpen(false)
          lockedPosRef.current = null
        }
        return !prev
      })
    },
    [hovered],
  )

  // Drag-and-drop support
  const handleDragStart = useCallback(
    (e: React.DragEvent) => {
      if (!hovered) return

      const pos = hovered.pos
      const node = editor.state.doc.nodeAt(pos)
      if (!node) return

      // Store the position in the drag data
      e.dataTransfer.setData("application/x-tiptap-block-pos", String(pos))
      e.dataTransfer.effectAllowed = "move"

      // Create a drag image from the block's DOM element
      const dom = editor.view.nodeDOM(pos) as HTMLElement | null
      if (dom) {
        e.dataTransfer.setDragImage(dom, 0, 0)
      }
    },
    [editor, hovered],
  )

  // Drop handler — registered on the editor container
  useEffect(() => {
    const editorEl = editor.view.dom

    const handleDragOver = (e: DragEvent) => {
      if (e.dataTransfer?.types.includes("application/x-tiptap-block-pos")) {
        e.preventDefault()
        e.dataTransfer.dropEffect = "move"
      }
    }

    const handleDrop = (e: DragEvent) => {
      const posStr = e.dataTransfer?.getData("application/x-tiptap-block-pos")
      if (!posStr) return

      e.preventDefault()
      const fromPos = parseInt(posStr, 10)
      if (isNaN(fromPos)) return

      // Find where the drop target is
      const coords = editor.view.posAtCoords({
        left: e.clientX,
        top: e.clientY,
      })
      if (!coords) return

      const resolved = editor.state.doc.resolve(coords.pos)
      if (resolved.depth < 1) return

      const toPos = resolved.before(1)
      if (toPos === fromPos) return // dropped on self

      const fromNode = editor.state.doc.nodeAt(fromPos)
      if (!fromNode) return

      const { tr } = editor.state

      if (toPos > fromPos) {
        // Moving down: insert after target, then delete source
        const toNode = editor.state.doc.nodeAt(toPos)
        if (!toNode) return
        const insertAt = toPos + toNode.nodeSize
        tr.insert(insertAt, fromNode)
        tr.delete(fromPos, fromPos + fromNode.nodeSize)
      } else {
        // Moving up: delete source first, then insert at target
        tr.delete(fromPos, fromPos + fromNode.nodeSize)
        tr.insert(toPos, fromNode)
      }

      editor.view.dispatch(tr)
      setHovered(null)
    }

    editorEl.addEventListener("dragover", handleDragOver)
    editorEl.addEventListener("drop", handleDrop)
    return () => {
      editorEl.removeEventListener("dragover", handleDragOver)
      editorEl.removeEventListener("drop", handleDrop)
    }
  }, [editor])

  if (!hovered) return null

  // Calculate handle position relative to the editor's scroll container.
  // The handle sits to the left of the block.
  const editorRect = editor.view.dom.getBoundingClientRect()
  const handleTop = hovered.rect.top - editorRect.top
  const handleLeft = -28 // 24px to the left of the block + 4px padding

  return (
    <div
      ref={handleRef}
      className="block-handle-container"
      style={{
        position: "absolute",
        top: handleTop,
        left: handleLeft,
        zIndex: 20,
      }}
      // Keep handle visible when mouse moves over it
      onMouseEnter={() => {
        /* prevent clearing hovered */
      }}
    >
      {hovered.isEmpty ? (
        // Plus button for empty paragraphs
        <button
          type="button"
          onClick={handlePlusClick}
          className={
            "flex h-6 w-6 items-center justify-center rounded-md " +
            "text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-600 " +
            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400"
          }
          title="Click to add a block"
          aria-label="Add block"
        >
          <Plus className="h-4 w-4" />
        </button>
      ) : (
        // Drag handle for non-empty blocks
        <button
          type="button"
          draggable
          onClick={handleGripClick}
          onDragStart={handleDragStart}
          className={
            "flex h-6 w-6 cursor-grab items-center justify-center rounded-md " +
            "text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-600 " +
            "active:cursor-grabbing " +
            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400 " +
            (menuOpen ? "bg-zinc-100 text-zinc-600" : "")
          }
          title="Drag to move. Click for actions."
          aria-label="Block actions"
        >
          <GripVertical className="h-4 w-4" />
        </button>
      )}

      {/* Actions dropdown menu */}
      {menuOpen && (
        <div
          ref={menuRef}
          className={
            "absolute left-0 top-full z-50 mt-1 w-48 rounded-md border border-zinc-200 " +
            "bg-white py-1 shadow-lg"
          }
        >
          <MenuItem
            icon={<Trash2 className="h-3.5 w-3.5" />}
            label="Delete"
            shortcut=""
            onClick={handleDelete}
          />
          <MenuItem
            icon={<Copy className="h-3.5 w-3.5" />}
            label="Duplicate"
            shortcut=""
            onClick={handleDuplicate}
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
                <span>Turn into...</span>
              </span>
              <ChevronRight className="h-3.5 w-3.5 text-zinc-400" />
            </button>
            {turnIntoOpen && (
              <div
                className={
                  "absolute left-full top-0 z-50 ml-1 w-40 rounded-md border border-zinc-200 " +
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
      )}
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
      {shortcut && (
        <span className="text-[10px] text-zinc-400">{shortcut}</span>
      )}
    </button>
  )
}

function MenuSeparator() {
  return <div className="my-1 h-px bg-zinc-100" />
}
