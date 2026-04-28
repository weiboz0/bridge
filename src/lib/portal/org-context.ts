/**
 * Plan 043 phase 3: shared helpers for multi-org admin context.
 *
 * The org-portal pages take an optional `?orgId=<uuid>` query param so
 * a multi-org admin can pick which organization they're inspecting.
 * These pure helpers extract the param from a server-component
 * `searchParams` shape and append it to API URLs.
 */

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function parseOrgIdFromSearchParams(
  searchParams: { orgId?: string | string[] | undefined } | undefined
): string | undefined {
  if (!searchParams) return undefined;
  const raw = searchParams.orgId;
  // Next.js can deliver duplicate query params as string[]; pick the
  // first sane value. Validate as UUID so a malformed query doesn't
  // surface as a Go 400.
  const candidate = Array.isArray(raw) ? raw[0] : raw;
  if (!candidate) return undefined;
  return UUID_RE.test(candidate) ? candidate : undefined;
}

export function appendOrgId(path: string, orgId: string | undefined): string {
  if (!orgId) return path;
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}orgId=${encodeURIComponent(orgId)}`;
}
