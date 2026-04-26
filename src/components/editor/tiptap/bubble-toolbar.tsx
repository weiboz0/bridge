"use client"

import { useCallback, useState, useRef, useEffect } from "react"
import type { Editor } from "@tiptap/react"
import { BubbleMenu } from "@tiptap/react/menus"
import {
  Bold,
  Italic,
  Underline,
  Strikethrough,
  Code,
  Link,
  Highlighter,
  Type,
  Subscript,
  Superscript,
  RemoveFormatting,
  Sparkles,
  X,
  Check,
  Unlink,
} from "lucide-react"

// ---------------------------------------------------------------------------
// Preset colors for text color picker
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
// Toolbar button component
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

// ---------------------------------------------------------------------------
// Link input popover (inline within the bubble menu)
// ---------------------------------------------------------------------------

interface LinkInputProps {
  editor: Editor
  onClose: () => void
}

function LinkInput({ editor, onClose }: LinkInputProps) {
  const currentUrl = editor.getAttributes("link").href ?? ""
  const [url, setUrl] = useState(currentUrl)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    // Focus the input when it appears
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

  const unlink = useCallback(() => {
    editor.chain().focus().unsetLink().run()
    onClose()
  }, [editor, onClose])

  return (
    <div className="flex items-center gap-1 px-1">
      <input
        ref={inputRef}
        type="url"
        placeholder="https://..."
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault()
            apply()
          }
          if (e.key === "Escape") {
            e.preventDefault()
            onClose()
          }
        }}
        className="h-6 w-40 rounded border border-zinc-200 bg-white px-2 text-xs outline-none focus:border-zinc-400"
      />
      <ToolbarButton onClick={apply} title="Apply link">
        <Check className="h-3.5 w-3.5" />
      </ToolbarButton>
      {currentUrl && (
        <ToolbarButton onClick={unlink} title="Remove link">
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
// Color picker popover (inline within the bubble menu)
// ---------------------------------------------------------------------------

interface ColorPickerProps {
  editor: Editor
  onClose: () => void
}

function ColorPicker({ editor, onClose }: ColorPickerProps) {
  const currentColor = editor.getAttributes("textStyle").color ?? ""

  return (
    <div className="flex items-center gap-0.5 px-1">
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
              className="h-4 w-4 rounded-full border border-zinc-200"
              style={{ backgroundColor: c.value }}
            />
          ) : (
            <span className="text-[10px] font-medium text-zinc-500">A</span>
          )}
        </button>
      ))}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Bubble toolbar
// ---------------------------------------------------------------------------

interface BubbleToolbarProps {
  editor: Editor
}

export function BubbleToolbar({ editor }: BubbleToolbarProps) {
  const [showLinkInput, setShowLinkInput] = useState(false)
  const [showColorPicker, setShowColorPicker] = useState(false)

  // Close sub-panels whenever selection changes
  useEffect(() => {
    const handleSelectionUpdate = () => {
      // Keep link input open if the user is actively editing
    }
    editor.on("selectionUpdate", handleSelectionUpdate)
    return () => {
      editor.off("selectionUpdate", handleSelectionUpdate)
    }
  }, [editor])

  return (
    <BubbleMenu
      editor={editor}
      options={{
        placement: "top",
        onHide: () => {
          setShowLinkInput(false)
          setShowColorPicker(false)
        },
      }}
      className="flex items-center gap-0.5 rounded-lg border border-zinc-200 bg-white px-1 py-1 shadow-md"
    >
      {showLinkInput ? (
        <LinkInput editor={editor} onClose={() => setShowLinkInput(false)} />
      ) : showColorPicker ? (
        <ColorPicker editor={editor} onClose={() => setShowColorPicker(false)} />
      ) : (
        <>
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

          {/* Divider */}
          <div className="mx-0.5 h-5 w-px bg-zinc-200" aria-hidden="true" />

          {/* Link */}
          <ToolbarButton
            onClick={() => setShowLinkInput(true)}
            active={editor.isActive("link")}
            title="Link (Cmd+K)"
          >
            <Link className="h-3.5 w-3.5" />
          </ToolbarButton>

          {/* Highlight */}
          <ToolbarButton
            onClick={() => editor.chain().focus().toggleHighlight().run()}
            active={editor.isActive("highlight")}
            title="Highlight"
          >
            <Highlighter className="h-3.5 w-3.5" />
          </ToolbarButton>

          {/* Color */}
          <ToolbarButton
            onClick={() => setShowColorPicker(true)}
            title="Text color"
          >
            <Type className="h-3.5 w-3.5" />
          </ToolbarButton>

          {/* Divider */}
          <div className="mx-0.5 h-5 w-px bg-zinc-200" aria-hidden="true" />

          {/* Sub/Superscript */}
          <ToolbarButton
            onClick={() => editor.chain().focus().toggleSubscript().run()}
            active={editor.isActive("subscript")}
            title="Subscript"
          >
            <Subscript className="h-3.5 w-3.5" />
          </ToolbarButton>
          <ToolbarButton
            onClick={() => editor.chain().focus().toggleSuperscript().run()}
            active={editor.isActive("superscript")}
            title="Superscript"
          >
            <Superscript className="h-3.5 w-3.5" />
          </ToolbarButton>

          {/* Clear formatting */}
          <ToolbarButton
            onClick={() => editor.chain().focus().unsetAllMarks().clearNodes().run()}
            title="Clear formatting"
          >
            <RemoveFormatting className="h-3.5 w-3.5" />
          </ToolbarButton>

          {/* Divider */}
          <div className="mx-0.5 h-5 w-px bg-zinc-200" aria-hidden="true" />

          {/* AI placeholder (disabled, Phase 5 enables) */}
          <ToolbarButton onClick={() => {}} disabled title="AI actions (coming soon)">
            <Sparkles className="h-3.5 w-3.5" />
          </ToolbarButton>
        </>
      )}
    </BubbleMenu>
  )
}
