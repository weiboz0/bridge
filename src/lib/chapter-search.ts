/**
 * Client-side helper for the chapter search API (GET /api/chapters/search).
 * All parameters are optional; omit to get defaults from the backend.
 *
 * Plan 045: linkableForCourse switches the endpoint into "picker mode"
 * — backend gates to course-edit, returns Chapters linkable to that course
 * (platform OR same-org), and decorates each item with linkedTopicId,
 * linkedTopicTitle, and canLink. The ChapterPickerDialog reads these to
 * disable already-linked rows and show "Already linked" labels.
 */

export interface SearchParams {
  q?: string
  scope?: string
  scopeId?: string
  gradeLevel?: string
  materialType?: string
  tags?: string[]
  status?: string
  limit?: number
  cursor?: string
  linkableForCourse?: string
  /** Filter by book membership. Pass a book UUID or "unfiled" for chapters
   *  with NULL book_id. Omit to disable the filter. */
  bookId?: string
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
  materialType: string
  status: string
  createdBy: string
  createdAt: string
  updatedAt: string
  // Picker-mode decorations (always present in picker mode; default to
  // null/false in non-picker mode where the backend doesn't include them).
  linkedTopicId?: string | null
  linkedTopicTitle?: string | null
  canLink?: boolean
}

export type SearchError = "network" | "server"

export interface SearchResult {
  items: SearchResultItem[]
  nextCursor: string | null
  // null = success; populated = the dialog should render an error banner
  // instead of the empty-state copy. Plan 045 distinguishes these so a
  // 500 doesn't silently look like "no results."
  error: SearchError | null
}

/**
 * Search chapters. Returns { items, nextCursor, error }. On any
 * network/server failure, items is [] and error is set so callers can
 * render an error banner distinct from the genuine empty-state.
 */
export async function searchChapters(params: SearchParams): Promise<SearchResult> {
  const query = new URLSearchParams()

  if (params.q) query.set("q", params.q)
  if (params.scope) query.set("scope", params.scope)
  if (params.scopeId) query.set("scopeId", params.scopeId)
  if (params.gradeLevel) query.set("gradeLevel", params.gradeLevel)
  if (params.materialType) query.set("materialType", params.materialType)
  if (params.tags && params.tags.length > 0) query.set("tags", params.tags.join(","))
  if (params.status) query.set("status", params.status)
  if (params.limit != null) query.set("limit", String(params.limit))
  if (params.cursor) query.set("cursor", params.cursor)
  if (params.linkableForCourse) query.set("linkableForCourse", params.linkableForCourse)
  if (params.bookId) query.set("bookId", params.bookId)

  try {
    const res = await fetch(`/api/chapters/search?${query}`)
    if (!res.ok) {
      console.error("[chapter-search] API error:", res.status, await res.text().catch(() => ""))
      return { items: [], nextCursor: null, error: "server" }
    }
    const body = (await res.json()) as { items: SearchResultItem[]; nextCursor: string | null }
    return { items: body.items, nextCursor: body.nextCursor, error: null }
  } catch {
    return { items: [], nextCursor: null, error: "network" }
  }
}

