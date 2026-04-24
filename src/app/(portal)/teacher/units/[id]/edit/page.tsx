"use client"

import { useEffect, useState, useCallback } from "react"
import { useParams } from "next/navigation"
import Link from "next/link"
import type { JSONContent } from "@tiptap/react"
import { Button, buttonVariants } from "@/components/ui/button"
import { TeachingUnitEditor } from "@/components/editor/tiptap/teaching-unit-editor"
import { fetchUnit, fetchUnitDocument, saveUnitDocument, type TeachingUnit } from "@/lib/teaching-units"

type LoadState =
  | { status: "loading" }
  | { status: "ready"; unit: TeachingUnit; doc: JSONContent | null }
  | { status: "error"; message: string }

export default function EditUnitPage() {
  const { id } = useParams<{ id: string }>()
  const [state, setState] = useState<LoadState>({ status: "loading" })
  const [saveMessage, setSaveMessage] = useState<string | null>(null)

  useEffect(() => {
    async function load() {
      try {
        const [unit, docData] = await Promise.all([
          fetchUnit(id),
          fetchUnitDocument(id),
        ])
        if (!unit) {
          setState({ status: "error", message: "Unit not found." })
          return
        }
        const doc = docData?.blocks != null ? (docData.blocks as JSONContent) : null
        setState({ status: "ready", unit, doc })
      } catch {
        setState({ status: "error", message: "Failed to load unit." })
      }
    }
    load()
  }, [id])

  const handleSave = useCallback(
    async (doc: JSONContent) => {
      try {
        await saveUnitDocument(id, doc)
        setSaveMessage("Saved")
        setTimeout(() => setSaveMessage(null), 2500)
      } catch {
        setSaveMessage("Save failed")
        setTimeout(() => setSaveMessage(null), 3000)
      }
    },
    [id]
  )

  if (state.status === "loading") {
    return (
      <div className="p-6">
        <p className="text-sm text-muted-foreground">Loading...</p>
      </div>
    )
  }

  if (state.status === "error") {
    return (
      <div className="p-6">
        <p className="text-sm text-destructive">{state.message}</p>
        <Link href="/teacher" className="mt-4 inline-block text-sm underline">
          Back to teacher home
        </Link>
      </div>
    )
  }

  const { unit, doc } = state

  return (
    <div className="p-6 space-y-4">
      {/* Header bar */}
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <Link
            href="/teacher/units"
            className="text-sm text-muted-foreground hover:text-foreground shrink-0"
          >
            Units
          </Link>
          <span className="text-muted-foreground shrink-0">/</span>
          <h1 className="text-lg font-semibold truncate">{unit.title}</h1>
        </div>
        <div className="flex items-center gap-3 shrink-0">
          {saveMessage && (
            <span
              className={
                saveMessage === "Saved"
                  ? "text-sm text-green-600"
                  : "text-sm text-destructive"
              }
            >
              {saveMessage}
            </span>
          )}
          <Link
            href={`/teacher/units/${id}`}
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            View
          </Link>
        </div>
      </div>

      {/* Editor */}
      <TeachingUnitEditor
        initialDoc={doc ?? undefined}
        onSave={handleSave}
      />
    </div>
  )
}
