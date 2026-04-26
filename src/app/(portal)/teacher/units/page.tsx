"use client"

import { useEffect, useState, useCallback, useRef } from "react"
import Link from "next/link"
import { useSession } from "next-auth/react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { searchUnits, type SearchResultItem } from "@/lib/unit-search"

// ---------- Types ----------

interface OrgMembership {
  orgId: string
  orgName: string
  role: string
  status: string
  orgStatus: string
}

type TabId = "all" | "personal" | "org" | "platform"

const GRADE_LEVELS = ["", "K-5", "6-8", "9-12"] as const
const GRADE_LABELS: Record<string, string> = {
  "": "All grades",
  "K-5": "K-5",
  "6-8": "6–8",
  "9-12": "9–12",
}

// ---------- Status badge helpers ----------

type UnitStatus = "draft" | "reviewed" | "classroom_ready" | "coach_ready" | "archived"

const STATUS_LABELS: Record<UnitStatus, string> = {
  draft: "Draft",
  reviewed: "Reviewed",
  classroom_ready: "Classroom Ready",
  coach_ready: "Coach Ready",
  archived: "Archived",
}

function isKnownStatus(s: string): s is UnitStatus {
  return ["draft", "reviewed", "classroom_ready", "coach_ready", "archived"].includes(s)
}

function statusBadgeClass(status: UnitStatus): string {
  switch (status) {
    case "draft":
      return "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium border border-zinc-200 bg-zinc-100 text-zinc-700"
    case "reviewed":
      return "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium border border-blue-200 bg-blue-100 text-blue-700"
    case "classroom_ready":
      return "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium border border-green-200 bg-green-100 text-green-700"
    case "coach_ready":
      return "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium border border-purple-200 bg-purple-100 text-purple-700"
    case "archived":
      return "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium border border-red-200 bg-red-100 text-red-700"
    default:
      return "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium border border-zinc-200 bg-zinc-100 text-zinc-700"
  }
}

function StatusBadge({ status }: { status: string }) {
  const known = isKnownStatus(status)
  const label = known ? STATUS_LABELS[status] : status
  const cls = known ? statusBadgeClass(status) : statusBadgeClass("draft")
  return <span className={cls}>{label}</span>
}

// ---------- Unit card ----------

function UnitCard({ unit }: { unit: SearchResultItem }) {
  const tags = unit.subjectTags ?? []
  return (
    <Link href={`/teacher/units/${unit.id}`} className="block">
      <div className="border border-zinc-200 rounded-lg p-4 bg-white transition-colors hover:border-zinc-300 hover:bg-zinc-50 space-y-2">
        <div className="flex items-start justify-between gap-2">
          <p className="font-medium text-zinc-900 leading-snug">{unit.title}</p>
          <StatusBadge status={unit.status} />
        </div>

        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-zinc-500">
          {unit.gradeLevel && <span>Grade {unit.gradeLevel}</span>}
          {unit.estimatedMinutes != null && <span>{unit.estimatedMinutes} min</span>}
        </div>

        {unit.summary && (
          <p className="text-sm text-zinc-600 line-clamp-2">{unit.summary}</p>
        )}

        {tags.length > 0 && (
          <div className="flex flex-wrap gap-1 pt-1">
            {tags.map((tag) => (
              <span
                key={tag}
                className="inline-block rounded bg-zinc-100 px-1.5 py-0.5 text-xs font-medium text-zinc-600"
              >
                {tag}
              </span>
            ))}
          </div>
        )}
      </div>
    </Link>
  )
}

// ---------- Main page ----------

export default function UnitLibraryPage() {
  const { data: session } = useSession()

  const [activeTab, setActiveTab] = useState<TabId>("all")
  const [orgs, setOrgs] = useState<OrgMembership[]>([])
  const [selectedOrgId, setSelectedOrgId] = useState<string>("")
  const [orgsLoading, setOrgsLoading] = useState(true)

  const [searchQuery, setSearchQuery] = useState("")
  const [gradeLevel, setGradeLevel] = useState("")
  const [tagsInput, setTagsInput] = useState("")

  const [items, setItems] = useState<SearchResultItem[]>([])
  const [nextCursor, setNextCursor] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)

  // Debounce ref for search query
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [debouncedQuery, setDebouncedQuery] = useState("")

  // Load orgs once
  useEffect(() => {
    async function loadOrgs() {
      try {
        const res = await fetch("/api/orgs")
        if (res.ok) {
          const data: OrgMembership[] = await res.json()
          const activeOrgs = data.filter(
            (m) =>
              m.status === "active" &&
              m.orgStatus === "active" &&
              (m.role === "teacher" || m.role === "org_admin")
          )
          setOrgs(activeOrgs)
          if (activeOrgs.length > 0) {
            setSelectedOrgId(activeOrgs[0].orgId)
          }
        }
      } finally {
        setOrgsLoading(false)
      }
    }
    loadOrgs()
  }, [])

  // Debounce search query
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      setDebouncedQuery(searchQuery)
    }, 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [searchQuery])

  // Build search params from current filter state
  const buildParams = useCallback(
    (cursor?: string) => {
      const userId = session?.user?.id
      const tags = tagsInput
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean)

      let scope: string | undefined
      let scopeId: string | undefined

      switch (activeTab) {
        case "all":
          scope = undefined
          scopeId = undefined
          break
        case "personal":
          scope = "personal"
          scopeId = userId ?? undefined
          break
        case "org":
          scope = "org"
          scopeId = selectedOrgId || undefined
          break
        case "platform":
          scope = "platform"
          scopeId = undefined
          break
      }

      return {
        q: debouncedQuery || undefined,
        scope,
        scopeId,
        gradeLevel: gradeLevel || undefined,
        tags: tags.length > 0 ? tags : undefined,
        limit: 20,
        cursor: cursor || undefined,
      }
    },
    [activeTab, selectedOrgId, debouncedQuery, gradeLevel, tagsInput, session?.user?.id]
  )

  // Fresh search whenever filters change
  useEffect(() => {
    // Don't search on org tab if orgs haven't loaded yet
    if (activeTab === "org" && orgsLoading) return

    let cancelled = false
    setLoading(true)
    setItems([])
    setNextCursor(null)

    searchUnits(buildParams()).then((result) => {
      if (!cancelled) {
        setItems(result.items)
        setNextCursor(result.nextCursor)
        setLoading(false)
      }
    })

    return () => {
      cancelled = true
    }
  }, [buildParams, activeTab, orgsLoading])

  async function handleLoadMore() {
    if (!nextCursor) return
    setLoadingMore(true)
    const result = await searchUnits(buildParams(nextCursor))
    setItems((prev) => [...prev, ...result.items])
    setNextCursor(result.nextCursor)
    setLoadingMore(false)
  }

  const tabs: { id: TabId; label: string }[] = [
    { id: "all", label: "All" },
    { id: "personal", label: "My Units" },
    { id: "org", label: "Org Library" },
    { id: "platform", label: "Platform" },
  ]

  return (
    <div className="space-y-6 p-6">
      {/* Page header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-zinc-900">Unit Library</h1>
        <Link href="/teacher/units/new">
          <Button className="bg-zinc-900 text-white hover:bg-zinc-800">Create Unit</Button>
        </Link>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-zinc-200">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
              activeTab === tab.id
                ? "border-zinc-900 text-zinc-900"
                : "border-transparent text-zinc-500 hover:text-zinc-700"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Org picker for org tab */}
      {activeTab === "org" && (
        <div>
          {orgsLoading ? (
            <p className="text-sm text-zinc-500">Loading organizations...</p>
          ) : orgs.length === 0 ? (
            <p className="text-sm text-zinc-500">
              You are not a member of any active organizations.
            </p>
          ) : orgs.length > 1 ? (
            <div className="flex items-center gap-2">
              <label
                htmlFor="orgPicker"
                className="text-sm font-medium text-zinc-700 whitespace-nowrap"
              >
                Organization
              </label>
              <select
                id="orgPicker"
                value={selectedOrgId}
                onChange={(e) => setSelectedOrgId(e.target.value)}
                className="flex h-8 rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
              >
                {orgs.map((org) => (
                  <option key={org.orgId} value={org.orgId}>
                    {org.orgName}
                  </option>
                ))}
              </select>
            </div>
          ) : (
            <p className="text-sm text-zinc-600">
              Showing units for{" "}
              <span className="font-medium text-zinc-900">{orgs[0].orgName}</span>
            </p>
          )}
        </div>
      )}

      {/* Search + filter row */}
      <div className="flex flex-wrap gap-3">
        <Input
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Search units..."
          className="w-64"
        />
        <select
          value={gradeLevel}
          onChange={(e) => setGradeLevel(e.target.value)}
          className="flex h-9 rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          aria-label="Grade level"
        >
          {GRADE_LEVELS.map((gl) => (
            <option key={gl} value={gl}>
              {GRADE_LABELS[gl]}
            </option>
          ))}
        </select>
        <Input
          value={tagsInput}
          onChange={(e) => setTagsInput(e.target.value)}
          placeholder="Tags (comma-separated)"
          className="w-52"
        />
      </div>

      {/* Results */}
      {loading ? (
        <div className="rounded-lg border border-dashed border-zinc-200 bg-zinc-50 px-4 py-10 text-center text-sm text-zinc-500">
          Loading...
        </div>
      ) : items.length === 0 ? (
        <div className="rounded-lg border border-dashed border-zinc-200 bg-zinc-50 px-4 py-10 text-center">
          <p className="text-sm text-zinc-600">No units found.</p>
          {activeTab === "personal" && (
            <p className="mt-1 text-sm text-zinc-500">
              Create your first unit to get started.{" "}
              <Link href="/teacher/units/new" className="underline text-zinc-700 hover:text-zinc-900">
                Create Unit
              </Link>
            </p>
          )}
        </div>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {items.map((unit) => (
            <UnitCard key={unit.id} unit={unit} />
          ))}
        </div>
      )}

      {/* Load more */}
      {nextCursor && !loading && (
        <div className="flex justify-center pt-2">
          <Button variant="outline" onClick={handleLoadMore} disabled={loadingMore}>
            {loadingMore ? "Loading..." : "Load more"}
          </Button>
        </div>
      )}
    </div>
  )
}
