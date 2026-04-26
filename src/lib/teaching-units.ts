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
  materialType?: "notes" | "slides" | "worksheet" | "reference"
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
 * Transition a unit to a new lifecycle status.
 * Throws on invalid transition (409) or any other non-OK response.
 */
export async function transitionUnit(id: string, status: string): Promise<TeachingUnit> {
  const res = await fetch(`/api/units/${id}/transition`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ status }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json() as Promise<TeachingUnit>
}

/**
 * Fetch the projected (role-filtered) document for a unit.
 * Returns null if the unit is inaccessible or the request fails.
 */
export async function fetchProjectedDocument(
  id: string,
  role?: string
): Promise<{ type: string; content: unknown[] } | null> {
  const params = role ? `?role=${encodeURIComponent(role)}` : ""
  const res = await fetch(`/api/units/${id}/projected${params}`)
  if (!res.ok) return null
  return res.json() as Promise<{ type: string; content: unknown[] }>
}

// ---------- Fork / overlay / lineage helpers (plan 034) ----------

export interface UnitOverlay {
  childUnitId: string
  parentUnitId: string
  parentRevisionId: string | null
  blockOverrides: unknown
  createdAt: string
  updatedAt: string
}

export interface LineageEntry {
  unitId: string
  title: string
  scope: string
  createdAt: string
}

export interface UnitRevision {
  id: string
  unitId: string
  blocks: unknown
  reason: string | null
  createdBy: string
  createdAt: string
}

/**
 * Fork a unit. Returns the new child unit.
 */
export async function forkUnit(
  id: string,
  data: { scope?: string; scopeId?: string; title?: string }
): Promise<TeachingUnit> {
  const res = await fetch(`/api/units/${id}/fork`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json() as Promise<TeachingUnit>
}

/**
 * Fetch the overlay for a forked unit. Returns null if the unit has no overlay (not a fork).
 */
export async function fetchOverlay(id: string): Promise<UnitOverlay | null> {
  const res = await fetch(`/api/units/${id}/overlay`)
  if (res.status === 404) return null
  if (!res.ok) throw new Error(await res.text())
  return res.json() as Promise<UnitOverlay>
}

/**
 * Partially update the overlay. Pass parentRevisionId="" to unpin (float).
 */
export async function updateOverlay(
  id: string,
  data: { parentRevisionId?: string | null; blockOverrides?: unknown }
): Promise<UnitOverlay> {
  const res = await fetch(`/api/units/${id}/overlay`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json() as Promise<UnitOverlay>
}

/**
 * Fetch the overlay-composed document for a unit. Returns null on any failure.
 */
export async function fetchComposedDocument(
  id: string
): Promise<{ unitId: string; blocks: unknown; updatedAt: string } | null> {
  const res = await fetch(`/api/units/${id}/composed`)
  if (!res.ok) return null
  return res.json() as Promise<{ unitId: string; blocks: unknown; updatedAt: string }>
}

/**
 * Fetch the overlay lineage (root-first). Returns null on any failure.
 */
export async function fetchLineage(id: string): Promise<{ items: LineageEntry[] } | null> {
  const res = await fetch(`/api/units/${id}/lineage`)
  if (!res.ok) return null
  return res.json() as Promise<{ items: LineageEntry[] }>
}

/**
 * List revisions for a unit. Returns an empty array on failure.
 */
export async function listRevisions(id: string): Promise<UnitRevision[]> {
  const res = await fetch(`/api/units/${id}/revisions`)
  if (!res.ok) return []
  const body = await res.json() as { items: UnitRevision[] }
  return body.items ?? []
}

// ---------- Block document ----------

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

// ---------- AI drafting (plan 035) ----------

export interface AIDraftResult {
  blocks: unknown[]
}

/**
 * Call the AI drafting endpoint to generate teaching unit blocks from a
 * natural-language intent description.
 *
 * Returns `{ blocks: [...] }` — the caller inserts these into the editor.
 * Throws on any non-OK response (including 503 when AI is not configured).
 */
export async function draftWithAI(unitId: string, intent: string): Promise<AIDraftResult> {
  const res = await fetch(`/api/units/${unitId}/draft-with-ai`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ intent }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new Error(body?.error ?? `draftWithAI: ${res.status}`)
  }
  return res.json() as Promise<AIDraftResult>
}
