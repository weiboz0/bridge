import { redirect } from "next/navigation";
import type { OrgContext } from "@/lib/portal/org-context";

/**
 * Plan 077 — guard helper for org-portal pages.
 *
 * Each page calls `handleOrgContext(ctx)` once and gets back either:
 *
 *   - `{kind: "render", orgId, orgName}` — page proceeds with these.
 *   - `{kind: "guard", element: <NoOrgState/> | <ErrorState/>}` — page
 *     returns the element directly.
 *
 * Without this, every migrated page would carry 3 conditional render
 * branches (no-org, error, ok) — 21 redundant blocks across 7 pages.
 * GLM 5.1 + Kimi K2.6 plan-review NIT.
 *
 * 401 responses redirect to `/login` server-side via `next/navigation` —
 * matches the existing pattern at `parent-links/page.tsx:71`.
 */
export type HandledOrgContext =
  | { kind: "render"; orgId: string; orgName: string }
  | { kind: "guard"; element: React.ReactElement };

export function handleOrgContext(ctx: OrgContext): HandledOrgContext {
  if (ctx.kind === "ok") {
    return { kind: "render", orgId: ctx.orgId, orgName: ctx.orgName };
  }
  if (ctx.kind === "error") {
    if (ctx.status === 401) {
      // server-side redirect; never returns
      redirect("/login");
    }
    return {
      kind: "guard",
      element: <OrgContextErrorState status={ctx.status} message={ctx.message} />,
    };
  }
  // kind === "no-org"
  return {
    kind: "guard",
    element: <NoOrgState reason={ctx.reason} />,
  };
}

function NoOrgState({
  reason,
}: {
  reason:
    | "no-active-admin-membership"
    | "not-org-admin-at-this-org"
    | "not-a-member";
}) {
  let title = "No organization";
  let body = "You're not currently set as an active org admin in any organization.";
  if (reason === "not-org-admin-at-this-org") {
    title = "Not authorized";
    body =
      "You're a member of this organization but not as an active org admin. If you're a teacher, check your teacher portal at /teacher.";
  } else if (reason === "not-a-member") {
    title = "Not a member";
    body =
      "You don't appear to be a member of the requested organization. Try switching orgs from the dropdown above.";
  }
  return (
    <div className="p-6 max-w-2xl space-y-2">
      <h1 className="text-2xl font-bold">{title}</h1>
      <p className="text-muted-foreground">{body}</p>
    </div>
  );
}

function OrgContextErrorState({
  status,
  message,
}: {
  status: number;
  message: string;
}) {
  return (
    <div className="p-6 max-w-2xl space-y-2">
      <h1 className="text-2xl font-bold">Couldn&apos;t load your organization</h1>
      <p className="text-muted-foreground">
        Something went wrong while looking up your active organization.
      </p>
      <pre className="text-xs bg-muted p-2 rounded">
        {status > 0 ? `HTTP ${status}: ` : ""}
        {message}
      </pre>
      <p className="text-sm text-muted-foreground">
        Try refreshing the page. If this keeps happening, contact support.
      </p>
    </div>
  );
}
