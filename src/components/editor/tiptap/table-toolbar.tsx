"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import type { Editor } from "@tiptap/react"
import {
  Minus,
  ArrowUp,
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  Merge,
  Grid2x2,
  Trash2,
} from "lucide-react"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface TableToolbarProps {
  editor: Editor
}

// ---------------------------------------------------------------------------
// Toolbar button
// ---------------------------------------------------------------------------

function TBtn({
  onClick,
  title,
  disabled,
  children,
}: {
  onClick: () => void
  title: string
  disabled?: boolean
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      aria-label={title}
      className={
        "flex h-7 w-7 items-center justify-center rounded text-zinc-600 transition-colors " +
        "hover:bg-zinc-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400 " +
        "disabled:cursor-not-allowed disabled:opacity-40"
      }
    >
      {children}
    </button>
  )
}

// ---------------------------------------------------------------------------
// TableToolbar component
// ---------------------------------------------------------------------------

export function TableToolbar({ editor }: TableToolbarProps) {
  const [visible, setVisible] = useState(false)
  const [position, setPosition] = useState<{ top: number; left: number }>({
    top: 0,
    left: 0,
  })
  const toolbarRef = useRef<HTMLDivElement>(null)

  // Check if cursor is inside a table, and if so position the toolbar above it.
  useEffect(() => {
    const updatePosition = () => {
      const isInTable = editor.isActive("table")
      if (!isInTable) {
        setVisible(false)
        return
      }

      // Find the table DOM element
      const { $from } = editor.state.selection
      let depth = $from.depth
      while (depth > 0) {
        const node = $from.node(depth)
        if (node.type.name === "table") break
        depth--
      }
      if (depth === 0) {
        setVisible(false)
        return
      }

      const tablePos = $from.before(depth)
      const dom = editor.view.nodeDOM(tablePos)
      if (!dom || !(dom instanceof HTMLElement)) {
        setVisible(false)
        return
      }

      const editorRect = editor.view.dom.getBoundingClientRect()
      const tableRect = dom.getBoundingClientRect()

      setPosition({
        top: tableRect.top - editorRect.top - 36,
        left: tableRect.left - editorRect.left,
      })
      setVisible(true)
    }

    editor.on("selectionUpdate", updatePosition)
    editor.on("update", updatePosition)
    return () => {
      editor.off("selectionUpdate", updatePosition)
      editor.off("update", updatePosition)
    }
  }, [editor])

  const addRowBefore = useCallback(
    () => editor.chain().focus().addRowBefore().run(),
    [editor],
  )
  const addRowAfter = useCallback(
    () => editor.chain().focus().addRowAfter().run(),
    [editor],
  )
  const addColBefore = useCallback(
    () => editor.chain().focus().addColumnBefore().run(),
    [editor],
  )
  const addColAfter = useCallback(
    () => editor.chain().focus().addColumnAfter().run(),
    [editor],
  )
  const deleteRow = useCallback(
    () => editor.chain().focus().deleteRow().run(),
    [editor],
  )
  const deleteCol = useCallback(
    () => editor.chain().focus().deleteColumn().run(),
    [editor],
  )
  const mergeCells = useCallback(
    () => editor.chain().focus().mergeCells().run(),
    [editor],
  )
  const toggleHeader = useCallback(
    () => editor.chain().focus().toggleHeaderRow().run(),
    [editor],
  )
  const deleteTable = useCallback(
    () => editor.chain().focus().deleteTable().run(),
    [editor],
  )

  if (!visible) return null

  return (
    <div
      ref={toolbarRef}
      className="absolute z-30 flex items-center gap-0.5 rounded-md border border-zinc-200 bg-white px-1 py-0.5 shadow-sm"
      style={{
        top: position.top,
        left: position.left,
      }}
    >
      <TBtn onClick={addRowBefore} title="Add row above">
        <ArrowUp className="h-3.5 w-3.5" />
      </TBtn>
      <TBtn onClick={addRowAfter} title="Add row below">
        <ArrowDown className="h-3.5 w-3.5" />
      </TBtn>

      <div className="mx-0.5 h-5 w-px bg-zinc-200" />

      <TBtn onClick={addColBefore} title="Add column left">
        <ArrowLeft className="h-3.5 w-3.5" />
      </TBtn>
      <TBtn onClick={addColAfter} title="Add column right">
        <ArrowRight className="h-3.5 w-3.5" />
      </TBtn>

      <div className="mx-0.5 h-5 w-px bg-zinc-200" />

      <TBtn onClick={deleteRow} title="Delete row">
        <Minus className="h-3.5 w-3.5" />
      </TBtn>
      <TBtn onClick={deleteCol} title="Delete column">
        <Minus className="h-3.5 w-3.5 rotate-90" />
      </TBtn>

      <div className="mx-0.5 h-5 w-px bg-zinc-200" />

      <TBtn onClick={mergeCells} title="Merge cells">
        <Merge className="h-3.5 w-3.5" />
      </TBtn>
      <TBtn onClick={toggleHeader} title="Toggle header row">
        <Grid2x2 className="h-3.5 w-3.5" />
      </TBtn>

      <div className="mx-0.5 h-5 w-px bg-zinc-200" />

      <TBtn onClick={deleteTable} title="Delete table">
        <Trash2 className="h-3.5 w-3.5 text-red-500" />
      </TBtn>
    </div>
  )
}
