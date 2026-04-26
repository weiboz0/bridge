/**
 * Client-side helper for the unit search API (GET /api/units/search).
 * All parameters are optional; omit to get defaults from the backend.
 */

export interface SearchParams {
  q?: string
  scope?: string
  scopeId?: string
  gradeLevel?: string
  tags?: string[]
  status?: string
  limit?: number
  cursor?: string
}

export interface SearchResultItem {
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

export interface SearchResult {
  items: SearchResultItem[]
  nextCursor: string | null
}

/**
 * Search teaching units. Returns { items, nextCursor }.
 * On any network/server error returns { items: [], nextCursor: null } so callers
 * can handle the empty state without crashing.
 */
export async function searchUnits(params: SearchParams): Promise<SearchResult> {
  const query = new URLSearchParams()

  if (params.q) query.set("q", params.q)
  if (params.scope) query.set("scope", params.scope)
  if (params.scopeId) query.set("scopeId", params.scopeId)
  if (params.gradeLevel) query.set("gradeLevel", params.gradeLevel)
  if (params.tags && params.tags.length > 0) query.set("tags", params.tags.join(","))
  if (params.status) query.set("status", params.status)
  if (params.limit != null) query.set("limit", String(params.limit))
  if (params.cursor) query.set("cursor", params.cursor)

  try {
    const res = await fetch(`/api/units/search?${query}`)
    if (!res.ok) {
      console.error("[unit-search] API error:", res.status, await res.text().catch(() => ""))
      return { items: [], nextCursor: null }
    }
    return res.json() as Promise<SearchResult>
  } catch {
    return { items: [], nextCursor: null }
  }
}
