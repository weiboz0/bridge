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
  fetchLineage,
  fetchOverlay,
  updateOverlay,
  listRevisions,
  type TeachingUnit,
  type UnitOverlay,
  type LineageEntry,
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

  // Lineage and overlay state
  const [lineage, setLineage] = useState<LineageEntry[] | null>(null)
  const [overlay, setOverlay] = useState<UnitOverlay | null>(null)
  const [pinBusy, setPinBusy] = useState(false)
  const [pinError, setPinError] = useState<string | null>(null)

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

  // Load lineage and overlay in parallel. Reset stale state on id change.
  useEffect(() => {
    setLineage(null)
    setOverlay(null)
    let cancelled = false

    fetchLineage(id).then((data) => {
      if (!cancelled && data) setLineage(data.items)
    }).catch(() => {/* ignore */})

    fetchOverlay(id).then((data) => {
      if (!cancelled) setOverlay(data)
    }).catch(() => {/* ignore */})

    return () => { cancelled = true }
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

  const handlePinToCurrent = useCallback(async () => {
    if (!overlay) return
    setPinBusy(true)
    setPinError(null)
    try {
      // Fetch parent's revisions and pick the latest
      const revisions = await listRevisions(overlay.parentUnitId)
      if (revisions.length === 0) {
        setPinError("No published revisions to pin to.")
        return
      }
      // Revisions are ordered newest-first by the API
      const latest = revisions[0]
      const updated = await updateOverlay(id, { parentRevisionId: latest.id })
      setOverlay(updated)
    } catch (err) {
      setPinError(err instanceof Error ? err.message : "Pin failed")
    } finally {
      setPinBusy(false)
    }
  }, [id, overlay])

  const handleUnpin = useCallback(async () => {
    setPinBusy(true)
    setPinError(null)
    try {
      // Empty string signals the Go backend to set parent_revision_id = NULL
      const updated = await updateOverlay(id, { parentRevisionId: "" })
      setOverlay(updated)
    } catch (err) {
      setPinError(err instanceof Error ? err.message : "Unpin failed")
    } finally {
      setPinBusy(false)
    }
  }, [id])

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

      {/* Lineage breadcrumb — only shown when the unit has ancestors */}
      {lineage && lineage.length > 1 && (
        <div className="flex items-center gap-1 text-sm text-muted-foreground flex-wrap">
          {lineage.map((entry, idx) => {
            const isLast = idx === lineage.length - 1
            return (
              <span key={entry.unitId} className="flex items-center gap-1">
                {idx > 0 && <span className="text-zinc-300">&gt;</span>}
                {isLast ? (
                  <span className="text-foreground font-medium">{entry.title}</span>
                ) : (
                  <Link
                    href={`/teacher/units/${entry.unitId}`}
                    className="hover:text-foreground underline underline-offset-2"
                  >
                    {entry.title}
                  </Link>
                )}
              </span>
            )
          })}
        </div>
      )}

      {/* Pin/float toggle — only shown for forked units (overlay exists) */}
      {overlay && (
        <div className="flex items-center gap-3 py-2 px-3 rounded-md border border-zinc-200 bg-zinc-50 text-sm">
          <span className="text-muted-foreground shrink-0">Parent sync:</span>
          {overlay.parentRevisionId ? (
            <>
              <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-semibold border border-yellow-200 bg-yellow-100 text-yellow-800">
                Pinned to revision {new Date(overlay.updatedAt).toLocaleDateString()}
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={handleUnpin}
                disabled={pinBusy}
              >
                {pinBusy ? "Updating..." : "Unpin (follow latest)"}
              </Button>
            </>
          ) : (
            <>
              <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-semibold border border-green-200 bg-green-100 text-green-700">
                Following latest
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={handlePinToCurrent}
                disabled={pinBusy}
              >
                {pinBusy ? "Updating..." : "Pin to current"}
              </Button>
            </>
          )}
          {pinError && (
            <span className="text-xs text-destructive">{pinError}</span>
          )}
        </div>
      )}

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
