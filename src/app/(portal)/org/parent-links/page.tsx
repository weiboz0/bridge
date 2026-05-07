import { api, ApiError } from "@/lib/api-client";
import { resolveOrgContext } from "@/lib/portal/org-context";
import { handleOrgContext } from "@/components/portal/org-context-guard";
import { OrgParentLinksView } from "@/components/org/parent-links-view";

// Plan 070 phase 2 — org-admin parent-link management page.
//
// Server component. Resolves the active org from ?orgId= or falls
// back to the caller's first active org_admin membership via
// resolveOrgContext + handleOrgContext (plan 077 phase 2). Fetches
// the link list via /api/orgs/{orgId}/parent-links and the org's
// student roster (used by the create-modal's child picker) via
// /api/orgs/{orgId}/eligible-children.

export interface ParentLinkRow {
  id: string;
  parentUserId: string;
  childUserId: string;
  status: string;
  createdBy: string;
  createdAt: string;
  revokedAt: string | null;
  parentEmail: string;
  parentName: string;
  childEmail: string;
  childName: string;
  classId: string | null;
  className: string | null;
}

export interface OrgStudentRow {
  userId: string;
  email: string;
  name: string;
}

export default async function OrgParentLinksPage({
  searchParams,
}: {
  searchParams?: Promise<{ orgId?: string }>;
}) {
  const sp = await searchParams;
  const ctx = await resolveOrgContext(sp);
  const handled = handleOrgContext(ctx);
  if (handled.kind === "guard") return handled.element;
  const { orgId, orgName } = handled;

  let links: ParentLinkRow[] = [];
  let students: OrgStudentRow[] = [];
  let error: { status: number | null; message: string } | null = null;
  let studentsError: { status: number | null; message: string } | null = null;

  try {
    // Phase 2 — students list is the autocomplete source for the
    // create-modal child picker. Distinct from /api/org/students
    // (which queries org_memberships) — we want users enrolled in
    // any active class. Capture its failure separately from the
    // primary list fetch so a flaky students endpoint doesn't blank
    // the page; instead the create modal surfaces a "couldn't load
    // students" hint.
    const [linksRes, studentsResult] = await Promise.all([
      api<ParentLinkRow[]>(`/api/orgs/${orgId}/parent-links?status=active`),
      api<OrgStudentRow[]>(`/api/orgs/${orgId}/eligible-children`)
        .then((rows) => ({ ok: true as const, rows }))
        .catch((e: unknown) => ({
          ok: false as const,
          status: e instanceof ApiError ? e.status : null,
          message: e instanceof Error ? e.message : String(e),
        })),
    ]);
    links = linksRes;
    if (studentsResult.ok) {
      students = studentsResult.rows;
    } else {
      studentsError = {
        status: studentsResult.status,
        message: studentsResult.message,
      };
    }
  } catch (e) {
    error = {
      status: e instanceof ApiError ? e.status : null,
      message: e instanceof Error ? e.message : String(e),
    };
  }

  return (
    <OrgParentLinksView
      orgId={orgId}
      orgName={orgName}
      initialLinks={links}
      students={students}
      error={error}
      studentsError={studentsError}
    />
  );
}
