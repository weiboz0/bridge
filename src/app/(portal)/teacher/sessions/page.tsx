"use client"

import { useCallback, useEffect, useState } from "react"
import { useSession } from "next-auth/react"
import Link from "next/link"
import { Button } from "@/components/ui/button"

interface SessionItem {
  id: string
  title: string
  classId: string | null
  status: string
  startedAt: string
  endedAt: string | null
}

interface SessionsResponse {
  items: SessionItem[]
  nextCursor?: string | null
}

export default function TeacherSessionsPage() {
  const { data: session } = useSession()
  const [sessions, setSessions] = useState<SessionItem[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<"all" | "live" | "ended">("all")

  const loadSessions = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams({ limit: "50" })
      if (filter !== "all") params.set("status", filter)
      const res = await fetch(`/api/sessions?${params}`)
      if (res.ok) {
        const data: SessionsResponse = await res.json()
        setSessions(data.items ?? [])
      }
    } finally {
      setLoading(false)
    }
  }, [filter])

  useEffect(() => {
    if (session?.user?.id) loadSessions()
  }, [session?.user?.id, loadSessions])

  return (
    <div className="mx-auto max-w-4xl px-4 py-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold text-zinc-900">Sessions</h1>
        <Link href="/teacher">
          <Button variant="outline" size="sm">Start New Session</Button>
        </Link>
      </div>

      {/* Filter tabs */}
      <div className="mb-4 flex gap-1 rounded-lg border border-zinc-200 p-1 w-fit">
        {(["all", "live", "ended"] as const).map((f) => (
          <button
            key={f}
            type="button"
            onClick={() => setFilter(f)}
            className={`rounded-md px-3 py-1 text-sm font-medium transition-colors ${
              filter === f
                ? "bg-zinc-900 text-white"
                : "text-zinc-600 hover:text-zinc-900"
            }`}
          >
            {f === "all" ? "All" : f === "live" ? "Live" : "Ended"}
          </button>
        ))}
      </div>

      {loading ? (
        <p className="py-8 text-center text-sm text-zinc-400">Loading sessions...</p>
      ) : sessions.length === 0 ? (
        <p className="py-8 text-center text-sm text-zinc-400">
          No sessions found. Start one from your class page or dashboard.
        </p>
      ) : (
        <div className="space-y-2">
          {sessions.map((s) => (
            <Link
              key={s.id}
              href={`/teacher/sessions/${s.id}`}
              className="flex items-center justify-between rounded-lg border border-zinc-200 bg-white px-4 py-3 transition-colors hover:border-zinc-300"
            >
              <div className="min-w-0">
                <p className="text-sm font-medium text-zinc-900 truncate">
                  {s.title || "Untitled session"}
                </p>
                <p className="text-xs text-zinc-500">
                  {new Date(s.startedAt).toLocaleDateString(undefined, {
                    weekday: "short",
                    month: "short",
                    day: "numeric",
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                  {s.classId && " · class-linked"}
                  {!s.classId && " · orphan session"}
                </p>
              </div>
              <span
                className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${
                  s.status === "live"
                    ? "bg-green-100 text-green-700"
                    : "bg-zinc-100 text-zinc-500"
                }`}
              >
                {s.status}
              </span>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
