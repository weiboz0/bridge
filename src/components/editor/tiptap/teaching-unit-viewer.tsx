"use client"

import { EditorContent, type JSONContent, useEditor } from "@tiptap/react"
import { teachingUnitExtensions } from "./extensions"

export interface TeachingUnitViewerProps {
  doc?: JSONContent
}

export function TeachingUnitViewer({ doc }: TeachingUnitViewerProps) {
  const editor = useEditor({
    extensions: teachingUnitExtensions(),
    content: doc ?? { type: "doc", content: [] },
    editable: false,
  })

  return (
    <div className="rounded-lg border border-zinc-200 bg-zinc-50">
      <EditorContent editor={editor} className="min-h-60 px-3 py-2" />
    </div>
  )
}

