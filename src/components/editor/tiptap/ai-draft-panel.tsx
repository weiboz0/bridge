"use client"

import { useCallback, useState } from "react"
import { Button } from "@/components/ui/button"
import { draftWithAI } from "@/lib/teaching-units"

export interface AIDraftPanelProps {
  /** The unit ID for the API call. */
  unitId: string
  /** Called with the generated blocks on success. */
  onInsertBlocks: (blocks: unknown[]) => void
  /** Called when the panel should be closed. */
  onClose: () => void
}

export function AIDraftPanel({ unitId, onInsertBlocks, onClose }: AIDraftPanelProps) {
  const [intent, setIntent] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleGenerate = useCallback(async () => {
    const trimmed = intent.trim()
    if (!trimmed) return

    setLoading(true)
    setError(null)

    try {
      const result = await draftWithAI(unitId, trimmed)
      if (!result.blocks || result.blocks.length === 0) {
        setError("AI generated no blocks. Try rephrasing your intent.")
        return
      }
      onInsertBlocks(result.blocks)
      setIntent("")
      onClose()
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "An unexpected error occurred"
      setError(message)
    } finally {
      setLoading(false)
    }
  }, [intent, unitId, onInsertBlocks, onClose])

  return (
    <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-3 space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-zinc-700">
          Draft with AI
        </span>
        <button
          type="button"
          onClick={onClose}
          className="text-zinc-400 hover:text-zinc-600 text-sm leading-none"
          aria-label="Close AI panel"
        >
          x
        </button>
      </div>

      <textarea
        value={intent}
        onChange={(e) => setIntent(e.target.value)}
        placeholder="Describe the lesson: e.g. 6th grade, while loops, 45 min, 3 problems easy to medium"
        rows={3}
        disabled={loading}
        className="w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-zinc-300 disabled:opacity-50 resize-none"
      />

      {error && (
        <p className="text-sm text-red-600">{error}</p>
      )}

      <div className="flex items-center gap-2">
        <Button
          onClick={handleGenerate}
          disabled={loading || !intent.trim()}
          size="sm"
        >
          {loading ? (
            <span className="flex items-center gap-1.5">
              <span className="h-3 w-3 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-600" />
              Generating...
            </span>
          ) : (
            "Generate"
          )}
        </Button>
        <Button variant="outline" size="sm" onClick={onClose} disabled={loading}>
          Cancel
        </Button>
      </div>
    </div>
  )
}
