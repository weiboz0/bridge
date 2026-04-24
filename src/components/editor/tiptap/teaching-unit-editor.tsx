"use client"

import { useCallback } from "react"
import { nanoid } from "nanoid"
import { EditorContent, type Editor, type JSONContent, useEditor } from "@tiptap/react"
import { Button } from "@/components/ui/button"
import { teachingUnitExtensions } from "./extensions"

export interface TeachingUnitEditorProps {
  initialDoc?: JSONContent
  onSave: (doc: JSONContent) => Promise<void>
  onDirty?: (dirty: boolean) => void
}

function assignMissingTopLevelNodeIds(editor: Editor) {
  let transaction = editor.state.tr
  let needsUpdate = false

  editor.state.doc.forEach((node: { attrs?: Record<string, unknown> }, offset: number) => {
    const attrs = node.attrs as Record<string, unknown> | null
    if (typeof attrs?.id === "string" && attrs.id.length > 0) {
      return
    }

    const nextAttrs = {
      ...attrs,
      id: nanoid(),
    }

    transaction = transaction.setNodeMarkup(offset + 1, undefined, nextAttrs)
    needsUpdate = true
  })

  if (needsUpdate) {
    editor.view.dispatch(transaction)
  }
}

export function TeachingUnitEditor({ initialDoc, onSave, onDirty }: TeachingUnitEditorProps) {
  const editor = useEditor({
    extensions: teachingUnitExtensions(),
    content: initialDoc ?? { type: "doc", content: [] },
    onCreate({ editor }: { editor: Editor }) {
      assignMissingTopLevelNodeIds(editor)
    },
    onUpdate({ editor }: { editor: Editor }) {
      assignMissingTopLevelNodeIds(editor)
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

  const handleSave = useCallback(async () => {
    if (!editor) {
      return
    }

    const doc = editor.getJSON()
    await onSave(doc)
    onDirty?.(false)
  }, [editor, onDirty, onSave])

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2">
        <Button onClick={handleInsertProblem}>Insert Problem</Button>
        <Button variant="outline" onClick={handleSave}>
          Save
        </Button>
      </div>
      <div className="rounded-lg border border-zinc-200 bg-zinc-50">
        <EditorContent editor={editor} className="min-h-60 px-3 py-2" />
      </div>
    </div>
  )
}
