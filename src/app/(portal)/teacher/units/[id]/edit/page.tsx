"use client"

import { useEffect, useState, useCallback, useRef } from "react"
import { useParams } from "next/navigation"
import Link from "next/link"
import type { JSONContent } from "@tiptap/react"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  TeachingUnitEditor,
  type TeachingUnitEditorHandle,
} from "@/components/editor/tiptap/teaching-unit-editor"
import { TeachingUnitViewer } from "@/components/editor/tiptap/teaching-unit-viewer"
import {
  fetchUnit,
  fetchUnitDocument,
  fetchProjectedDocument,
  saveUnitDocument,
  transitionUnit,
  type TeachingUnit,
} from "@/lib/teaching-units"

type LoadState =
  | { status: "loading" }
  | { status: "ready"; unit: TeachingUnit; doc: JSONContent | null }
  | { status: "error"; message: string }

type UnitStatus = "draft" | "reviewed" | "classroom_ready" | "coach_ready" | "archived"

/**
 * Valid transitions per spec 012.
 */
const VALID_TRANSITIONS: Record<UnitStatus, UnitStatus[]> = {
  draft: ["reviewed"],
  reviewed: ["classroom_ready", "coach_ready", "archived"],
  classroom_ready: ["archived"],
  coach_ready: ["archived"],
  archived: ["classroom_ready"],
}

const STATUS_LABELS: Record<UnitStatus, string> = {
  draft: "Draft",
  reviewed: "Reviewed",
  classroom_ready: "Classroom Ready",
  coach_ready: "Coach Ready",
  archived: "Archived",
}

/**
 * Tailwind classes for each status badge.
 * Using plain inline classes (no dynamic construction) so Tailwind picks them up.
 */
function statusBadgeClass(status: UnitStatus): string {
  switch (status) {
    case "draft":
      return "inline-flex items-center rounded-md px-2.5 py-0.5 text-xs font-semibold border border-zinc-200 bg-zinc-100 text-zinc-700"
    case "reviewed":
      return "inline-flex items-center rounded-md px-2.5 py-0.5 text-xs font-semibold border border-blue-200 bg-blue-100 text-blue-700"
    case "classroom_ready":
      return "inline-flex items-center rounded-md px-2.5 py-0.5 text-xs font-semibold border border-green-200 bg-green-100 text-green-700"
    case "coach_ready":
      return "inline-flex items-center rounded-md px-2.5 py-0.5 text-xs font-semibold border border-purple-200 bg-purple-100 text-purple-700"
    case "archived":
      return "inline-flex items-center rounded-md px-2.5 py-0.5 text-xs font-semibold border border-red-200 bg-red-100 text-red-700"
    default:
      return "inline-flex items-center rounded-md px-2.5 py-0.5 text-xs font-semibold border border-zinc-200 bg-zinc-100 text-zinc-700"
  }
}

function isKnownStatus(s: string): s is UnitStatus {
  return s === "draft" || s === "reviewed" || s === "classroom_ready" || s === "coach_ready" || s === "archived"
}

function StatusBadge({ status }: { status: string }) {
  const known = isKnownStatus(status)
  const label = known ? STATUS_LABELS[status] : status
  const cls = known ? statusBadgeClass(status) : statusBadgeClass("draft")
  return <span className={cls}>{label}</span>
}

interface TransitionButtonProps {
  unit: TeachingUnit
  onTransitioned: (updated: TeachingUnit) => void
}

function TransitionButton({ unit, onTransitioned }: TransitionButtonProps) {
  const [transitioning, setTransitioning] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const status = unit.status
  const targets = isKnownStatus(status) ? VALID_TRANSITIONS[status] : []

  if (targets.length === 0) {
    return null
  }

  const handleTransition = async (target: UnitStatus) => {
    setTransitioning(true)
    setError(null)
    try {
      const updated = await transitionUnit(unit.id, target)
      onTransitioned(updated)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Transition failed")
    } finally {
      setTransitioning(false)
    }
  }

  return (
    <div className="flex flex-col items-end gap-1">
      <DropdownMenu>
        <DropdownMenuTrigger
          className={buttonVariants({ variant: "outline", size: "sm" })}
          disabled={transitioning}
        >
          {transitioning ? "Updating..." : "Transition"}
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {targets.map((target) => (
            <DropdownMenuItem
              key={target}
              onClick={() => handleTransition(target)}
            >
              {STATUS_LABELS[target]}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      {error && (
        <p className="text-xs text-destructive max-w-48 text-right">{error}</p>
      )}
    </div>
  )
}

type PreviewState =
  | { active: false }
  | { active: true; doc: JSONContent }

export default function EditUnitPage() {
  const { id } = useParams<{ id: string }>()
  const [state, setState] = useState<LoadState>({ status: "loading" })
  const [saveMessage, setSaveMessage] = useState<string | null>(null)
  const [preview, setPreview] = useState<PreviewState>({ active: false })
  const [previewLoading, setPreviewLoading] = useState(false)
  const editorRef = useRef<TeachingUnitEditorHandle>(null)

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
      } catch (err) {
        setSaveMessage("Save failed")
        setTimeout(() => setSaveMessage(null), 3000)
        throw err
      }
    },
    [id]
  )

  const handleTransitioned = useCallback((updated: TeachingUnit) => {
    setState((prev) =>
      prev.status === "ready" ? { ...prev, unit: updated } : prev
    )
  }, [])

  const handlePreviewToggle = useCallback(async () => {
    if (preview.active) {
      setPreview({ active: false })
      return
    }

    // Auto-save before switching to preview
    setPreviewLoading(true)
    try {
      if (editorRef.current) {
        await editorRef.current.save()
      }
      const projected = await fetchProjectedDocument(id, "student")
      if (projected) {
        setPreview({ active: true, doc: projected as JSONContent })
      } else {
        setSaveMessage("Preview unavailable")
        setTimeout(() => setSaveMessage(null), 3000)
      }
    } catch {
      setSaveMessage("Preview failed")
      setTimeout(() => setSaveMessage(null), 3000)
    } finally {
      setPreviewLoading(false)
    }
  }, [id, preview.active])

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
          <StatusBadge status={unit.status} />
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
          <Button
            variant={preview.active ? "default" : "outline"}
            size="sm"
            onClick={handlePreviewToggle}
            disabled={previewLoading}
          >
            {previewLoading ? "Loading..." : preview.active ? "Edit" : "Preview"}
          </Button>
          <TransitionButton unit={unit} onTransitioned={handleTransitioned} />
          <Link
            href={`/teacher/units/${id}`}
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            View
          </Link>
        </div>
      </div>

      {/* Editor or Preview */}
      {preview.active ? (
        <div className="space-y-2">
          <p className="text-xs text-zinc-400 font-medium uppercase tracking-wide">
            Student view (read-only)
          </p>
          <TeachingUnitViewer doc={preview.doc} />
        </div>
      ) : (
        <TeachingUnitEditor
          ref={editorRef}
          initialDoc={doc ?? undefined}
          onSave={handleSave}
        />
      )}
    </div>
  )
}
