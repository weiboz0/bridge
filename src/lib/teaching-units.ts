/**
 * Client-side helpers for the teaching units API (/api/units).
 * All functions fetch relative paths that Next.js proxies to the Go backend.
 */

export interface TeachingUnit {
  id: string
  scope: string
  scopeId: string | null
  title: string
  slug: string | null
  summary: string
  gradeLevel: string | null
  subjectTags: string[]
  standardsTags: string[]
  estimatedMinutes: number | null
  status: string
  createdBy: string
  createdAt: string
  updatedAt: string
}

export interface UnitDocument {
  unitId: string
  blocks: unknown
  updatedAt: string
}

export interface CreateUnitInput {
  title: string
  scope: "personal" | "org" | "platform"
  scopeId?: string
  summary?: string
  gradeLevel?: string
  subjectTags?: string[]
  estimatedMinutes?: number
}

/**
 * Fetch a single unit by ID. Returns null on 404/403.
 */
export async function fetchUnit(id: string): Promise<TeachingUnit | null> {
  const res = await fetch(`/api/units/${id}`)
  if (res.status === 404 || res.status === 403) return null
  if (!res.ok) throw new Error(`fetchUnit: ${res.status}`)
  return res.json() as Promise<TeachingUnit>
}

/**
 * Fetch the block document for a unit. Returns null on 404/403.
 */
export async function fetchUnitDocument(id: string): Promise<UnitDocument | null> {
  const res = await fetch(`/api/units/${id}/document`)
  if (res.status === 404 || res.status === 403) return null
  if (!res.ok) throw new Error(`fetchUnitDocument: ${res.status}`)
  return res.json() as Promise<UnitDocument>
}

/**
 * Create a new teaching unit. Returns the created unit row.
 */
export async function createUnit(data: CreateUnitInput): Promise<TeachingUnit> {
  const res = await fetch("/api/units", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      title: data.title,
      scope: data.scope,
      scopeId: data.scopeId ?? null,
      summary: data.summary ?? "",
      gradeLevel: data.gradeLevel ?? null,
      subjectTags: data.subjectTags ?? [],
      estimatedMinutes: data.estimatedMinutes ?? null,
    }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new Error(body?.error ?? `createUnit: ${res.status}`)
  }
  return res.json() as Promise<TeachingUnit>
}

/**
 * Persist the block document for a unit. Returns the updated document row.
 */
export async function saveUnitDocument(id: string, blocks: unknown): Promise<UnitDocument> {
  const res = await fetch(`/api/units/${id}/document`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(blocks),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new Error(body?.error ?? `saveUnitDocument: ${res.status}`)
  }
  return res.json() as Promise<UnitDocument>
}
