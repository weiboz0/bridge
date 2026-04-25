"use client"

import { useEffect, useState } from "react"
import { useParams } from "next/navigation"
import Link from "next/link"
import type { JSONContent } from "@tiptap/react"
import { TeachingUnitViewer } from "@/components/editor/tiptap/teaching-unit-viewer"
import { fetchUnit, fetchProjectedDocument, type TeachingUnit } from "@/lib/teaching-units"

type LoadState =
  | { status: "loading" }
  | { status: "ready"; unit: TeachingUnit; doc: JSONContent }
  | { status: "unavailable" }
  | { status: "error"; message: string }

type UnitStatus = "draft" | "reviewed" | "classroom_ready" | "coach_ready" | "archived"

function isKnownStatus(s: string): s is UnitStatus {
  return (
    s === "draft" ||
    s === "reviewed" ||
    s === "classroom_ready" ||
    s === "coach_ready" ||
    s === "archived"
  )
}

const STATUS_LABELS: Record<UnitStatus, string> = {
  draft: "Draft",
  reviewed: "Reviewed",
  classroom_ready: "Classroom Ready",
  coach_ready: "Coach Ready",
  archived: "Archived",
}

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

function StatusBadge({ status }: { status: string }) {
  const known = isKnownStatus(status)
  const label = known ? STATUS_LABELS[status] : status
  const cls = known ? statusBadgeClass(status) : statusBadgeClass("draft")
  return <span className={cls}>{label}</span>
}

export default function StudentUnitPage() {
  const { id } = useParams<{ id: string }>()
  const [state, setState] = useState<LoadState>({ status: "loading" })

  useEffect(() => {
    async function load() {
      try {
        const [unit, projected] = await Promise.all([
          fetchUnit(id),
          fetchProjectedDocument(id, "student"),
        ])

        if (!unit) {
          setState({ status: "unavailable" })
          return
        }

        if (!projected) {
          setState({ status: "unavailable" })
          return
        }

        setState({
          status: "ready",
          unit,
          doc: projected as JSONContent,
        })
      } catch {
        setState({ status: "error", message: "Failed to load unit." })
      }
    }

    load()
  }, [id])

  if (state.status === "loading") {
    return (
      <div className="p-6">
        <p className="text-sm text-muted-foreground">Loading...</p>
      </div>
    )
  }

  if (state.status === "unavailable") {
    return (
      <div className="p-6 space-y-4">
        <h1 className="text-lg font-semibold text-zinc-900">Unit not available</h1>
        <p className="text-sm text-zinc-500">
          This unit is not currently accessible. Ask your teacher for access.
        </p>
        <Link href="/student" className="text-sm text-blue-600 underline">
          Back to dashboard
        </Link>
      </div>
    )
  }

  if (state.status === "error") {
    return (
      <div className="p-6 space-y-2">
        <p className="text-sm text-destructive">{state.message}</p>
        <Link href="/student" className="text-sm underline">
          Back to dashboard
        </Link>
      </div>
    )
  }

  const { unit, doc } = state

  return (
    <div className="p-6 space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <Link
            href="/student"
            className="text-sm text-muted-foreground hover:text-foreground shrink-0"
          >
            Dashboard
          </Link>
          <span className="text-muted-foreground shrink-0">/</span>
          <h1 className="text-lg font-semibold truncate">{unit.title}</h1>
          <StatusBadge status={unit.status} />
        </div>
      </div>

      {/* Projected content */}
      <TeachingUnitViewer doc={doc} />
    </div>
  )
}
