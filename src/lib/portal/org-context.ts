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

/**
 * Plan 069 phase 4 (Codex post-impl Q5) — when an org-admin page
 * needs the orgId for path-style API URLs (e.g.
 * `/api/orgs/{orgId}/members/{memberId}`), the legacy `?orgId=`
 * query approach can't fall back to "the user's first active
 * org_admin org" because the URL goes empty. This helper does the
 * server-side resolution: prefers the URL's `?orgId=` if valid;
 * otherwise picks the caller's first active org_admin membership
 * via `/api/orgs`. Returns `null` when no resolved id is available
 * (caller should render a no-org state).
 *
 * Caller is responsible for rendering a sensible empty state when
 * this returns null.
 */
import { api, ApiError } from "@/lib/api-client";

interface OrgMembership {
  orgId: string;
  orgName: string;
  role: string;
  status: string;
  orgStatus: string;
}

export async function resolveOrgIdServerSide(
  searchParams: { orgId?: string | string[] | undefined } | undefined,
): Promise<string | null> {
  const queried = parseOrgIdFromSearchParams(searchParams);
  if (queried) return queried;
  try {
    const memberships = await api<OrgMembership[]>("/api/orgs");
    const m = memberships.find(
      (m) =>
        m.role === "org_admin" &&
        m.status === "active" &&
        m.orgStatus === "active",
    );
    return m ? m.orgId : null;
  } catch (e) {
    // 401 surfaces back as `null`; caller can choose to redirect to
    // /login or show the no-org state. Other errors fall through
    // similarly; the page-level error handler shows the list error
    // anyway.
    if (e instanceof ApiError) return null;
    return null;
  }
}
