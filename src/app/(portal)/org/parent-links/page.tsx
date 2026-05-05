import { redirect } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { parseOrgIdFromSearchParams } from "@/lib/portal/org-context";
import { OrgParentLinksView } from "@/components/org/parent-links-view";

// Plan 070 phase 2 — org-admin parent-link management page.
//
// Server component. Resolves the active org from ?orgId= or falls
// back to the caller's first active org_admin membership. Fetches
// the link list via /api/orgs/{orgId}/parent-links and the org's
// student roster (used by the create-modal's child picker) via
// /api/org/students?orgId=.

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

interface OrgMembership {
  orgId: string;
  orgName: string;
  role: string;
  status: string;
  orgStatus: string;
}

export default async function OrgParentLinksPage({
  searchParams,
}: {
  searchParams?: Promise<{ orgId?: string }>;
}) {
  const sp = await searchParams;
  let orgId = parseOrgIdFromSearchParams(sp);
  let orgName = "";

  // No orgId in the URL — fall back to the caller's first active
  // org_admin membership. Mirrors the org dashboard's resolution
  // strategy but keeps the resolved id in scope so we can build
  // the path-style API URL (/api/orgs/{orgId}/parent-links).
  if (!orgId) {
    try {
      const memberships = await api<OrgMembership[]>("/api/orgs");
      const m = memberships.find(
        (m) =>
          m.role === "org_admin" &&
          m.status === "active" &&
          m.orgStatus === "active",
      );
      if (m) {
        orgId = m.orgId;
        orgName = m.orgName;
      }
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) redirect("/login");
      // fall through; we'll show the no-org state below
    }
  }

  if (!orgId) {
    return (
      <div className="p-6 max-w-2xl space-y-2">
        <h1 className="text-2xl font-bold">Parent links</h1>
        <p className="text-muted-foreground">
          You need to be an org admin in at least one active organization to
          manage parent links.
        </p>
      </div>
    );
  }

  let links: ParentLinkRow[] = [];
  let students: OrgStudentRow[] = [];
  let error: { status: number | null; message: string } | null = null;

  try {
    const [linksRes, studentsRes] = await Promise.all([
      api<ParentLinkRow[]>(`/api/orgs/${orgId}/parent-links?status=active`),
      // Phase 2 — students list is the autocomplete source for the
      // create-modal child picker. Distinct from /api/org/students
      // (which queries org_memberships) — we want users enrolled in
      // any active class.
      api<OrgStudentRow[]>(`/api/orgs/${orgId}/eligible-children`).catch(
        () => [] as OrgStudentRow[],
      ),
    ]);
    links = linksRes;
    students = studentsRes;
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) redirect("/login");
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
    />
  );
}
