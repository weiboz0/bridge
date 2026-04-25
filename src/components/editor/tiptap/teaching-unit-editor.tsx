"use client"

import { useCallback, useImperativeHandle, useState, forwardRef } from "react"
import { nanoid } from "nanoid"
import { EditorContent, type Editor, type JSONContent, useEditor } from "@tiptap/react"
import { InputRule } from "@tiptap/core"
import { Button } from "@/components/ui/button"
import { teachingUnitExtensions } from "./extensions"
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

/**
 * Slash command input rules.
 * When the user types /note, /code, /media, or /problem at the start of a
 * paragraph (preceded only by optional whitespace), the typed trigger text is
 * replaced with the corresponding custom block node.
 *
 * Tiptap InputRule receives the full match and returns a replacement via the
 * `handler` callback.  We use a plain text regex pattern so the rule fires
 * exactly once per trigger word.
 */
function makeSlashCommandExtension() {
  // We cannot add InputRules to an existing node type here, so we return a
  // lightweight anonymous Tiptap extension that only contributes input rules.
  const { Extension } = require("@tiptap/core") as typeof import("@tiptap/core")
  return Extension.create({
    name: "slash-commands",
    addInputRules() {
      const makeRule = (
        trigger: string,
        nodeType: string,
        extraAttrs: Record<string, unknown> = {}
      ) =>
        new InputRule({
          // Match the slash command only at the start of a text block
          // (after whitespace or at position 0) to avoid false triggers
          // on inline text like "url/code".
          find: new RegExp(`(?:^|\\s)/${trigger}$`),
          handler: ({ state, range, chain }) => {
            const type = state.schema.nodes[nodeType]
            if (!type) return null

            chain()
              .deleteRange(range)
              .insertContent({
                type: nodeType,
                attrs: { id: nanoid(), ...extraAttrs },
              })
              .run()

            return null
          },
        })

      return [
        makeRule("note", "teacher-note"),
        makeRule("code", "code-snippet", { language: "python", code: "" }),
        makeRule("media", "media-embed", { url: "", alt: "", mediaType: "image" }),
        makeRule("problem", "problem-ref", {
          problemId: "",
          pinnedRevision: null,
          visibility: "always",
          overrideStarter: null,
        }),
        makeRule("solution", "solution-ref", { solutionId: "", reveal: "always" }),
        makeRule("testcase", "test-case-ref", {
          testCaseId: "",
          problemRefId: "",
          showToStudent: true,
        }),
        makeRule("cue", "live-cue"),
        makeRule("assignment", "assignment-variant", { title: "", timeLimitMinutes: null }),
      ]
    },
  })
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
    extensions: [
      ...teachingUnitExtensions(),
      makeSlashCommandExtension(),
      ...(isCollaborative ? yjsExtensions : []),
    ],
    // When collaborative, omit `content` so Yjs drives the initial state.
    // We still pass initialDoc as a content seed when not yet connected so
    // the editor isn't blank during the brief connection window.
    content: isCollaborative ? undefined : (initialDoc ?? { type: "doc", content: [] }),
    onCreate({ editor: e }: any) {
      assignMissingTopLevelNodeIds(e as Editor)
    },
    onUpdate({ editor: e }: any) {
      assignMissingTopLevelNodeIds(e as Editor)
      onDirty?.(true)
    },
  })

  const handleInsertProblem = useCallback(() => {
    if (!editor) {
      return
    }

    const problemId = window.prompt("Enter problem ID")
    if (!problemId) {
      return
    }

    editor
      .chain()
      .focus()
      .insertContent({
        type: "problem-ref",
        attrs: {
          id: nanoid(),
          problemId: problemId.trim(),
          pinnedRevision: null,
          visibility: "always",
          overrideStarter: null,
        },
      })
      .run()
  }, [editor])

  const handleInsertTeacherNote = useCallback(() => {
    if (!editor) return
    editor
      .chain()
      .focus()
      .insertContent({
        type: "teacher-note",
        attrs: { id: nanoid() },
        content: [{ type: "paragraph" }],
      })
      .run()
  }, [editor])

  const handleInsertCodeSnippet = useCallback(() => {
    if (!editor) return
    editor
      .chain()
      .focus()
      .insertContent({
        type: "code-snippet",
        attrs: { id: nanoid(), language: "python", code: "" },
      })
      .run()
  }, [editor])

  const handleInsertMediaEmbed = useCallback(() => {
    if (!editor) return
    editor
      .chain()
      .focus()
      .insertContent({
        type: "media-embed",
        attrs: { id: nanoid(), url: "", alt: "", mediaType: "image" },
      })
      .run()
  }, [editor])

  const handleInsertAIBlocks = useCallback((blocks: unknown[]) => {
    if (!editor) return
    // Insert each block at the current cursor position (or end of document).
    for (const block of blocks) {
      editor.chain().focus().insertContent(block as JSONContent).run()
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
        <Button onClick={handleInsertProblem} variant="outline" size="sm">
          + Problem
        </Button>
        <Button onClick={handleInsertTeacherNote} variant="outline" size="sm">
          + Teacher Note
        </Button>
        <Button onClick={handleInsertCodeSnippet} variant="outline" size="sm">
          + Code
        </Button>
        <Button onClick={handleInsertMediaEmbed} variant="outline" size="sm">
          + Media
        </Button>
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
        Tip: type{" "}
        {["/note", "/code", "/media", "/problem", "/solution", "/testcase", "/cue", "/assignment"].map((cmd, i, arr) => (
          <span key={cmd}>
            <code className="rounded bg-zinc-100 px-1">{cmd}</code>
            {i < arr.length - 1 ? ", " : ""}
          </span>
        ))}{" "}
        to insert a block.
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
