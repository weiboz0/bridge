"use client"

import { useCallback, useImperativeHandle, useState, forwardRef } from "react"
import { nanoid } from "nanoid"
import { EditorContent, type Editor, type JSONContent, useEditor } from "@tiptap/react"
import { Button } from "@/components/ui/button"
import { teachingUnitExtensions } from "./extensions"
import { SlashCommandExtension } from "./slash-menu"
import { AIDraftPanel } from "./ai-draft-panel"
import { useYjsTiptap } from "@/lib/yjs/use-yjs-tiptap"

/** Options for enabling real-time collaborative editing via Yjs/Hocuspocus. */
export interface CollaborativeOptions {
  /** The unit ID — used as the Hocuspocus document name (`unit:{unitId}`). */
  unitId: string
  /** Authenticated user ID — used for the Hocuspocus auth token and cursor color. */
  userId: string
  /** Display name shown on the awareness cursor label. */
  userName: string
  /** Optional explicit cursor color. Derived from userId hash when omitted. */
  userColor?: string
}

export interface TeachingUnitEditorProps {
  initialDoc?: JSONContent
  onSave: (doc: JSONContent) => Promise<void>
  onDirty?: (dirty: boolean) => void
  /**
   * The teaching unit ID. Used for AI drafting and derived from collaborative
   * options when omitted.
   */
  unitId?: string
  /**
   * When provided, the editor runs in collaborative mode via Yjs + Hocuspocus.
   * Yjs becomes the source of truth; `initialDoc` is ignored once any remote
   * state has synced.  The `onSave` callback still works — it serializes the
   * current editor JSON and calls the save API.
   */
  collaborative?: CollaborativeOptions
}

export interface TeachingUnitEditorHandle {
  /** Programmatically trigger a save of the current document. */
  save: () => Promise<void>
}

function assignMissingTopLevelNodeIds(editor: Editor) {
  let transaction = editor.state.tr
  let needsUpdate = false

  editor.state.doc.forEach((node: { type: { spec: { attrs?: Record<string, unknown> } }; attrs?: Record<string, unknown> }, offset: number) => {
    // Only process node types that declare an `id` attribute in their schema.
    // Calling setNodeMarkup on nodes without a declared `id` (e.g. paragraph,
    // heading) would pass unknown attrs into ProseMirror's schema validation
    // and cause a "NodeType.create can't construct text nodes" error.
    if (!node.type.spec.attrs || !("id" in node.type.spec.attrs)) {
      return
    }

    const attrs = node.attrs as Record<string, unknown> | null
    if (typeof attrs?.id === "string" && attrs.id.length > 0) {
      return
    }

    const nextAttrs = {
      ...attrs,
      id: nanoid(),
    }

    transaction = transaction.setNodeMarkup(offset, undefined, nextAttrs)
    needsUpdate = true
  })

  if (needsUpdate) {
    editor.view.dispatch(transaction)
  }
}

// ---------------------------------------------------------------------------
// AI block type mapping
// ---------------------------------------------------------------------------

/**
 * Maps AI-generated block types (e.g. "prose", "teacher-note", "code-snippet")
 * to valid Tiptap JSONContent that the editor can insert. The AI endpoint uses
 * its own vocabulary for block types — this function bridges the gap.
 */
function mapAIBlockToTiptap(block: Record<string, unknown>): JSONContent {
  const blockType = typeof block.type === "string" ? block.type : ""
  const text =
    typeof block.text === "string"
      ? block.text
      : typeof block.content === "string"
        ? block.content
        : ""

  switch (blockType) {
    case "prose":
      // "prose" maps to one or more paragraphs. Split on double-newlines so
      // multi-paragraph prose becomes multiple paragraph nodes.
      if (!text) return { type: "paragraph" }
      return {
        type: "doc",
        content: text.split(/\n{2,}/).map((chunk: string) => ({
          type: "paragraph",
          content: chunk.trim()
            ? [{ type: "text", text: chunk.trim() }]
            : [],
        })),
      }

    case "heading": {
      const level =
        typeof block.level === "number" && block.level >= 1 && block.level <= 3
          ? block.level
          : 2
      return {
        type: "heading",
        attrs: { level },
        content: text ? [{ type: "text", text }] : [],
      }
    }

    case "teacher-note":
      return {
        type: "teacher-note",
        attrs: { id: nanoid() },
        content: [
          {
            type: "paragraph",
            content: text ? [{ type: "text", text }] : [],
          },
        ],
      }

    case "code-snippet": {
      const language =
        typeof block.language === "string" ? block.language : "python"
      const code =
        typeof block.code === "string"
          ? block.code
          : typeof block.text === "string"
            ? block.text
            : ""
      return {
        type: "code-snippet",
        attrs: { id: nanoid(), language, code },
      }
    }

    case "problem-ref":
    case "problem": {
      const problemId =
        typeof block.problemId === "string" ? block.problemId : ""
      return {
        type: "problem-ref",
        attrs: {
          id: nanoid(),
          problemId,
          pinnedRevision: null,
          visibility: "always",
          overrideStarter: null,
        },
      }
    }

    case "solution-ref":
    case "solution": {
      const solutionId =
        typeof block.solutionId === "string" ? block.solutionId : ""
      return {
        type: "solution-ref",
        attrs: { id: nanoid(), solutionId, reveal: "always" },
      }
    }

    case "media-embed":
    case "media": {
      const url = typeof block.url === "string" ? block.url : ""
      const alt = typeof block.alt === "string" ? block.alt : ""
      const mediaType =
        typeof block.mediaType === "string" ? block.mediaType : "image"
      return {
        type: "media-embed",
        attrs: { id: nanoid(), url, alt, mediaType },
      }
    }

    case "live-cue":
      return {
        type: "live-cue",
        attrs: { id: nanoid() },
        content: [
          {
            type: "paragraph",
            content: text ? [{ type: "text", text }] : [],
          },
        ],
      }

    case "assignment-variant":
    case "assignment": {
      const title = typeof block.title === "string" ? block.title : ""
      const timeLimitMinutes =
        typeof block.timeLimitMinutes === "number"
          ? block.timeLimitMinutes
          : null
      return {
        type: "assignment-variant",
        attrs: { id: nanoid(), title, timeLimitMinutes },
        content: [
          {
            type: "paragraph",
            content: text ? [{ type: "text", text }] : [],
          },
        ],
      }
    }

    default:
      // Fallback: wrap any text content in a paragraph.
      if (text) {
        return {
          type: "paragraph",
          content: [{ type: "text", text }],
        }
      }
      // Last resort: try to use the block as-is (it might already be valid
      // Tiptap JSON).
      return block as JSONContent
  }
}

export const TeachingUnitEditor = forwardRef<TeachingUnitEditorHandle, TeachingUnitEditorProps>(
function TeachingUnitEditor({ initialDoc, onSave, onDirty, unitId, collaborative }, ref) {
  const [showAIPanel, setShowAIPanel] = useState(false)

  // Resolve the unit ID from the explicit prop or the collaborative options.
  const resolvedUnitId = unitId ?? collaborative?.unitId ?? ""
  // Collaborative mode: set up Yjs binding when `collaborative` is provided.
  // The hook is always called (Rules of Hooks) but does nothing when
  // `unitId` or `userId` are empty strings.
  const { extensions: yjsExtensions, connected } = useYjsTiptap({
    unitId: collaborative?.unitId ?? "",
    userId: collaborative?.userId ?? "",
    userName: collaborative?.userName ?? "",
    userColor: collaborative?.userColor,
  })

  const isCollaborative = Boolean(collaborative) && yjsExtensions.length > 0

  const editor = useEditor({
    immediatelyRender: false,
    extensions: [
      ...teachingUnitExtensions(),
      SlashCommandExtension,
      ...(isCollaborative ? yjsExtensions : []),
    ],
    content: isCollaborative && connected
      ? undefined
      : (initialDoc ?? { type: "doc", content: [] }),
    onCreate({ editor: e }: { editor: Editor }) {
      assignMissingTopLevelNodeIds(e)
    },
    onUpdate({ editor: e }: { editor: Editor }) {
      assignMissingTopLevelNodeIds(e)
      onDirty?.(true)
    },
  })

  const handleInsertAIBlocks = useCallback((blocks: unknown[]) => {
    if (!editor) return
    // Map AI output types to valid Tiptap node types, then insert.
    for (const block of blocks) {
      if (typeof block !== "object" || block === null) continue
      const mapped = mapAIBlockToTiptap(block as Record<string, unknown>)
      // A "doc" wrapper means the mapper produced multiple nodes (e.g. prose
      // split into paragraphs). Insert each child individually.
      if (mapped.type === "doc" && Array.isArray(mapped.content)) {
        for (const child of mapped.content) {
          editor.chain().focus().insertContent(child).run()
        }
      } else {
        editor.chain().focus().insertContent(mapped).run()
      }
    }
    onDirty?.(true)
  }, [editor, onDirty])

  const handleSave = useCallback(async () => {
    if (!editor) {
      return
    }

    const doc = editor.getJSON()
    await onSave(doc)
    onDirty?.(false)
  }, [editor, onDirty, onSave])

  useImperativeHandle(ref, () => ({
    save: handleSave,
  }), [handleSave])

  return (
    <div className="space-y-3">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        {resolvedUnitId && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setShowAIPanel((prev) => !prev)}
          >
            {showAIPanel ? "Close AI" : "Draft with AI"}
          </Button>
        )}
        <Button variant="default" size="sm" onClick={handleSave}>
          Save
        </Button>
        {/* Collaborative status indicator — only shown in collaborative mode */}
        {collaborative && (
          <span
            className={
              connected
                ? "ml-auto text-xs font-medium text-green-600"
                : "ml-auto text-xs font-medium text-zinc-400"
            }
          >
            {connected ? "Live" : "Connecting..."}
          </span>
        )}
      </div>
      <p className="text-xs text-zinc-400">
        Tip: type <code className="rounded bg-zinc-100 px-1">/</code> to open the command menu and insert blocks.
      </p>
      {showAIPanel && resolvedUnitId && (
        <AIDraftPanel
          unitId={resolvedUnitId}
          onInsertBlocks={handleInsertAIBlocks}
          onClose={() => setShowAIPanel(false)}
        />
      )}
      <div className="rounded-lg border border-zinc-200 bg-zinc-50">
        <EditorContent editor={editor} className="min-h-60 px-3 py-2" />
      </div>
    </div>
  )
})
TeachingUnitEditor.displayName = "TeachingUnitEditor"
