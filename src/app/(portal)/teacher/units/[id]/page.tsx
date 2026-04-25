"use client"

import { useEffect, useState, useCallback } from "react"
import { useParams, useRouter } from "next/navigation"
import Link from "next/link"
import type { JSONContent } from "@tiptap/react"
import { Button, buttonVariants } from "@/components/ui/button"
import { TeachingUnitViewer } from "@/components/editor/tiptap/teaching-unit-viewer"
import { fetchUnit, fetchUnitDocument, forkUnit, type TeachingUnit } from "@/lib/teaching-units"

type LoadState =
  | { status: "loading" }
  | { status: "ready"; unit: TeachingUnit; doc: JSONContent | null }
  | { status: "error"; message: string }

export default function ViewUnitPage() {
  const { id } = useParams<{ id: string }>()
  const router = useRouter()
  const [state, setState] = useState<LoadState>({ status: "loading" })
  const [forking, setForking] = useState(false)
  const [forkError, setForkError] = useState<string | null>(null)

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

  const handleFork = useCallback(async () => {
    setForking(true)
    setForkError(null)
    try {
      const child = await forkUnit(id, {})
      router.push(`/teacher/units/${child.id}/edit`)
    } catch (err) {
      setForkError(err instanceof Error ? err.message : "Fork failed")
      setForking(false)
    }
  }, [id, router])

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

  const gradeBadge = unit.gradeLevel ?? null
  const tags = unit.subjectTags ?? []
  const mins = unit.estimatedMinutes ?? null

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1 min-w-0">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Link href="/teacher/units" className="hover:text-foreground">
              Units
            </Link>
            <span>/</span>
          </div>
          <h1 className="text-2xl font-bold">{unit.title}</h1>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {forkError && (
            <span className="text-xs text-destructive max-w-48 text-right">{forkError}</span>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={handleFork}
            disabled={forking}
          >
            {forking ? "Forking..." : "Fork"}
          </Button>
          <Link
            href={`/teacher/units/${id}/edit`}
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            Edit
          </Link>
        </div>
      </div>

      {/* Metadata */}
      <div className="flex flex-wrap gap-4 text-sm text-muted-foreground border-b border-zinc-200 pb-4">
        <span>
          Scope:{" "}
          <span className="font-medium text-foreground capitalize">{unit.scope}</span>
        </span>
        <span>
          Status:{" "}
          <span className="font-medium text-foreground capitalize">
            {unit.status.replace("_", " ")}
          </span>
        </span>
        {gradeBadge && (
          <span>
            Grade:{" "}
            <span className="font-medium text-foreground">{gradeBadge}</span>
          </span>
        )}
        {mins !== null && (
          <span>
            Duration:{" "}
            <span className="font-medium text-foreground">{mins} min</span>
          </span>
        )}
        {tags.length > 0 && (
          <span className="flex flex-wrap gap-1 items-center">
            <span>Tags:</span>
            {tags.map((tag) => (
              <span
                key={tag}
                className="inline-block rounded bg-zinc-100 px-1.5 py-0.5 text-xs font-medium text-zinc-700"
              >
                {tag}
              </span>
            ))}
          </span>
        )}
      </div>

      {/* Summary */}
      {unit.summary && (
        <p className="text-sm text-muted-foreground max-w-prose">{unit.summary}</p>
      )}

      {/* Block document */}
      <TeachingUnitViewer doc={doc ?? undefined} />
    </div>
  )
}
