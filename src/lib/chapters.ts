/**
 * Client-side helpers for the chapters API (/api/chapters).
 * All functions fetch relative paths that Next.js proxies to the Go backend.
 */

export interface Chapter {
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

/** @deprecated Use Chapter instead */
export type TeachingUnit = Chapter

export interface ChapterDocument {
  chapterId: string
  blocks: unknown
  updatedAt: string
}

/** @deprecated Use ChapterDocument instead */
export type UnitDocument = ChapterDocument

export interface CreateChapterInput {
  title: string
  scope: "personal" | "org" | "platform"
  scopeId?: string
  summary?: string
  gradeLevel?: string
  subjectTags?: string[]
  estimatedMinutes?: number
  materialType?: "notes" | "slides" | "worksheet" | "reference"
}

/** @deprecated Use CreateChapterInput instead */
export type CreateUnitInput = CreateChapterInput

/**
 * Fetch a single chapter by ID. Returns null on 404/403.
 */
export async function fetchChapter(id: string): Promise<Chapter | null> {
  const res = await fetch(`/api/chapters/${id}`)
  if (res.status === 404 || res.status === 403) return null
  if (!res.ok) throw new Error(`fetchChapter: ${res.status}`)
  return res.json() as Promise<Chapter>
}

/** @deprecated Use fetchChapter instead */
export const fetchUnit = fetchChapter

/**
 * Fetch the block document for a chapter. Returns null on 404/403.
 */
export async function fetchChapterDocument(id: string): Promise<ChapterDocument | null> {
  const res = await fetch(`/api/chapters/${id}/document`)
  if (res.status === 404 || res.status === 403) return null
  if (!res.ok) throw new Error(`fetchChapterDocument: ${res.status}`)
  return res.json() as Promise<ChapterDocument>
}

/** @deprecated Use fetchChapterDocument instead */
export const fetchUnitDocument = fetchChapterDocument

/**
 * Create a new chapter. Returns the created chapter row.
 */
export async function createChapter(data: CreateChapterInput): Promise<Chapter> {
  const res = await fetch("/api/chapters", {
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
      // Plan 054 drift fix — without this line the picker is
      // decorative; the backend defaults every created chapter to
      // `notes` regardless of caller intent.
      materialType: data.materialType ?? "notes",
    }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new Error(body?.error ?? `createChapter: ${res.status}`)
  }
  return res.json() as Promise<Chapter>
}

/** @deprecated Use createChapter instead */
export const createUnit = createChapter

/**
 * Transition a chapter to a new lifecycle status.
 * Throws on invalid transition (409) or any other non-OK response.
 */
export async function transitionChapter(id: string, status: string): Promise<Chapter> {
  const res = await fetch(`/api/chapters/${id}/transition`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ status }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json() as Promise<Chapter>
}

/** @deprecated Use transitionChapter instead */
export const transitionUnit = transitionChapter

/**
 * Fetch the projected (role-filtered) document for a chapter.
 * Returns null if the chapter is inaccessible or the request fails.
 */
export async function fetchProjectedDocument(
  id: string,
  role?: string
): Promise<{ type: string; content: unknown[] } | null> {
  const params = role ? `?role=${encodeURIComponent(role)}` : ""
  const res = await fetch(`/api/chapters/${id}/projected${params}`)
  if (!res.ok) return null
  return res.json() as Promise<{ type: string; content: unknown[] }>
}

// ---------- Fork / overlay / lineage helpers (plan 034) ----------

export interface ChapterOverlay {
  childChapterId: string
  parentChapterId: string
  parentRevisionId: string | null
  blockOverrides: unknown
  createdAt: string
  updatedAt: string
}

/**
 * UnitOverlay is kept for backward compat with code that destructures
 * childUnitId / parentUnitId. The API now returns childChapterId /
 * parentChapterId — callers should migrate to ChapterOverlay.
 * @deprecated Use ChapterOverlay instead
 */
export type UnitOverlay = ChapterOverlay & {
  childUnitId: string
  parentUnitId: string
}

export interface LineageEntry {
  unitId: string
  title: string
  scope: string
  createdAt: string
}

export interface ChapterRevision {
  id: string
  chapterId: string
  blocks: unknown
  reason: string | null
  createdBy: string
  createdAt: string
}

/** @deprecated Use ChapterRevision instead */
export type UnitRevision = ChapterRevision & { unitId: string }

/**
 * Fork a chapter. Returns the new child chapter.
 */
export async function forkChapter(
  id: string,
  data: { scope?: string; scopeId?: string; title?: string }
): Promise<Chapter> {
  const res = await fetch(`/api/chapters/${id}/fork`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json() as Promise<Chapter>
}

/** @deprecated Use forkChapter instead */
export const forkUnit = forkChapter

/**
 * Fetch the overlay for a forked chapter. Returns null if the chapter has no overlay (not a fork).
 */
export async function fetchOverlay(id: string): Promise<ChapterOverlay | null> {
  const res = await fetch(`/api/chapters/${id}/overlay`)
  if (res.status === 404) return null
  if (!res.ok) throw new Error(await res.text())
  return res.json() as Promise<ChapterOverlay>
}

/**
 * Partially update the overlay. Pass parentRevisionId="" to unpin (float).
 */
export async function updateOverlay(
  id: string,
  data: { parentRevisionId?: string | null; blockOverrides?: unknown }
): Promise<ChapterOverlay> {
  const res = await fetch(`/api/chapters/${id}/overlay`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json() as Promise<ChapterOverlay>
}

/**
 * Fetch the overlay-composed document for a chapter. Returns null on any failure.
 */
export async function fetchComposedDocument(
  id: string
): Promise<{ chapterId: string; blocks: unknown; updatedAt: string } | null> {
  const res = await fetch(`/api/chapters/${id}/composed`)
  if (!res.ok) return null
  return res.json() as Promise<{ chapterId: string; blocks: unknown; updatedAt: string }>
}

/**
 * Fetch the overlay lineage (root-first). Returns null on any failure.
 */
export async function fetchLineage(id: string): Promise<{ items: LineageEntry[] } | null> {
  const res = await fetch(`/api/chapters/${id}/lineage`)
  if (!res.ok) return null
  return res.json() as Promise<{ items: LineageEntry[] }>
}

/**
 * List revisions for a chapter. Returns an empty array on failure.
 */
export async function listRevisions(id: string): Promise<ChapterRevision[]> {
  const res = await fetch(`/api/chapters/${id}/revisions`)
  if (!res.ok) return []
  const body = await res.json() as { items: ChapterRevision[] }
  return body.items ?? []
}

// ---------- Block document ----------

/**
 * Persist the block document for a chapter. Returns the updated document row.
 */
export async function saveChapterDocument(id: string, blocks: unknown): Promise<ChapterDocument> {
  const res = await fetch(`/api/chapters/${id}/document`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(blocks),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new Error(body?.error ?? `saveChapterDocument: ${res.status}`)
  }
  return res.json() as Promise<ChapterDocument>
}

/** @deprecated Use saveChapterDocument instead */
export const saveUnitDocument = saveChapterDocument

// ---------- AI drafting (plan 035) ----------

export interface AIDraftResult {
  blocks: unknown[]
}

/**
 * Call the AI drafting endpoint to generate chapter blocks from a
 * natural-language intent description.
 *
 * Returns `{ blocks: [...] }` — the caller inserts these into the editor.
 * Throws on any non-OK response (including 503 when AI is not configured).
 */
export async function draftWithAI(chapterId: string, intent: string): Promise<AIDraftResult> {
  const res = await fetch(`/api/chapters/${chapterId}/draft-with-ai`, {
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
