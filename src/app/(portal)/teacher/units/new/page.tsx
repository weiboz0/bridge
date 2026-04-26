"use client"

import { useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import { useSession } from "next-auth/react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { createUnit } from "@/lib/teaching-units"

interface OrgMembership {
  orgId: string
  orgName: string
  role: string
  status: string
  orgStatus: string
}

export default function CreateUnitPage() {
  const router = useRouter()
  const { data: session } = useSession()

  const [orgs, setOrgs] = useState<OrgMembership[]>([])
  const [orgsLoading, setOrgsLoading] = useState(true)

  // Form fields
  const [title, setTitle] = useState("")
  // Initial scope is "personal"; updated to "org" after orgs load (if any exist).
  const [scope, setScope] = useState<"personal" | "org">("personal")
  const [orgId, setOrgId] = useState("")
  const [summary, setSummary] = useState("")
  const [gradeLevel, setGradeLevel] = useState("")
  const [subjectTags, setSubjectTags] = useState("")
  const [estimatedMinutes, setEstimatedMinutes] = useState("")
  const [materialType, setMaterialType] = useState("notes")

  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function loadOrgs() {
      try {
        const res = await fetch("/api/orgs")
        if (res.ok) {
          const data: OrgMembership[] = await res.json()
          const teacherOrgs = data.filter(
            (m) =>
              m.status === "active" &&
              m.orgStatus === "active" &&
              (m.role === "teacher" || m.role === "org_admin")
          )
          setOrgs(teacherOrgs)
          if (teacherOrgs.length > 0) {
            setOrgId(teacherOrgs[0].orgId)
            // Default to org scope when the user belongs to at least one org
            setScope("org")
          }
        } else {
          setError("Failed to load organizations")
        }
      } catch {
        setError("Failed to load organizations")
      } finally {
        setOrgsLoading(false)
      }
    }
    loadOrgs()
  }, [])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!title.trim()) {
      setError("Title is required.")
      return
    }

    const userId = session?.user?.id
    if (!userId) {
      setError("You must be signed in to create a unit.")
      return
    }

    if (scope === "org" && !orgId) {
      setError("Please select an organization.")
      return
    }

    const tags = subjectTags
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean)

    const mins = estimatedMinutes ? parseInt(estimatedMinutes, 10) : undefined

    setSubmitting(true)
    setError(null)

    try {
      const unit = await createUnit({
        title: title.trim(),
        scope,
        scopeId: scope === "org" ? orgId : userId,
        summary: summary.trim() || undefined,
        gradeLevel: gradeLevel || undefined,
        subjectTags: tags.length > 0 ? tags : undefined,
        estimatedMinutes: mins && !isNaN(mins) ? mins : undefined,
        materialType: materialType || undefined,
      })
      router.push(`/teacher/units/${unit.id}/edit`)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create unit.")
      setSubmitting(false)
    }
  }

  return (
    <div className="p-6 max-w-xl">
      <h1 className="text-2xl font-bold mb-6">Create Unit</h1>

      <form onSubmit={handleSubmit} className="space-y-5">
        {/* Title */}
        <div className="space-y-1.5">
          <Label htmlFor="title">Title</Label>
          <Input
            id="title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Unit title"
            required
          />
        </div>

        {/* Scope */}
        <div className="space-y-1.5">
          <Label htmlFor="scope">Scope</Label>
          <select
            id="scope"
            value={scope}
            onChange={(e) => setScope(e.target.value as "personal" | "org")}
            className="flex h-8 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            <option value="personal">Personal</option>
            <option value="org">Organization</option>
          </select>
        </div>

        {/* Org picker — only when scope=org */}
        {scope === "org" && (
          <div className="space-y-1.5">
            <Label htmlFor="orgId">Organization</Label>
            {orgsLoading ? (
              <p className="text-sm text-muted-foreground">Loading organizations...</p>
            ) : orgs.length === 0 ? (
              <p className="text-sm text-destructive">
                No organizations found. You must be a teacher or org admin in an active organization to create org-scoped units.
              </p>
            ) : (
              <select
                id="orgId"
                value={orgId}
                onChange={(e) => setOrgId(e.target.value)}
                className="flex h-8 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
              >
                {orgs.map((org) => (
                  <option key={org.orgId} value={org.orgId}>
                    {org.orgName}
                  </option>
                ))}
              </select>
            )}
          </div>
        )}

        {/* Summary */}
        <div className="space-y-1.5">
          <Label htmlFor="summary">Summary</Label>
          <textarea
            id="summary"
            value={summary}
            onChange={(e) => setSummary(e.target.value)}
            placeholder="Brief description of the unit"
            rows={3}
            className="flex w-full rounded-lg border border-input bg-transparent px-2.5 py-1.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 resize-none"
          />
        </div>

        {/* Grade Level */}
        <div className="space-y-1.5">
          <Label htmlFor="gradeLevel">Grade Level</Label>
          <select
            id="gradeLevel"
            value={gradeLevel}
            onChange={(e) => setGradeLevel(e.target.value)}
            className="flex h-8 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            <option value="">Not specified</option>
            <option value="K-5">K-5</option>
            <option value="6-8">6-8</option>
            <option value="9-12">9-12</option>
          </select>
        </div>

        {/* Subject Tags */}
        <div className="space-y-1.5">
          <Label htmlFor="subjectTags">Subject Tags</Label>
          <Input
            id="subjectTags"
            value={subjectTags}
            onChange={(e) => setSubjectTags(e.target.value)}
            placeholder="e.g. loops, variables, functions (comma-separated)"
          />
        </div>

        {/* Material Type */}
        <div className="space-y-1.5">
          <Label htmlFor="materialType">Material Type</Label>
          <select
            id="materialType"
            value={materialType}
            onChange={(e) => setMaterialType(e.target.value)}
            className="w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm"
          >
            <option value="notes">Notes (detailed explanations)</option>
            <option value="slides">Slides (concise bullets)</option>
            <option value="worksheet">Worksheet (practice-focused)</option>
            <option value="reference">Reference (cheat sheet)</option>
          </select>
        </div>

        {/* Estimated Minutes */}
        <div className="space-y-1.5">
          <Label htmlFor="estimatedMinutes">Estimated Minutes</Label>
          <Input
            id="estimatedMinutes"
            type="number"
            min={1}
            value={estimatedMinutes}
            onChange={(e) => setEstimatedMinutes(e.target.value)}
            placeholder="e.g. 45"
          />
        </div>

        {error && (
          <p className="text-sm text-destructive">{error}</p>
        )}

        <div className="flex gap-3 pt-1">
          <Button type="submit" disabled={submitting}>
            {submitting ? "Creating..." : "Create Unit"}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => router.back()}
            disabled={submitting}
          >
            Cancel
          </Button>
        </div>
      </form>
    </div>
  )
}
