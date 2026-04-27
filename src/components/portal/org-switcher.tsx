"use client";

import { useRouter, usePathname, useSearchParams } from "next/navigation";

interface OrgOption {
  orgId: string;
  orgName: string;
}

interface OrgSwitcherProps {
  /** Active `org_admin` memberships. Keys must be unique by orgId. */
  options: OrgOption[];
}

/**
 * Plan 043 phase 3: dropdown for picking which organization a multi-org
 * admin is inspecting. Updates the URL `?orgId=<id>` so the choice is
 * shareable and survives refresh; preserves any other query params.
 *
 * Renders nothing when the user has fewer than 2 options — single-org
 * admins shouldn't see chrome they can't act on.
 *
 * Uses a native `<select>` rather than the base-ui Select primitive
 * because (a) no other surface in the app uses that primitive yet, and
 * (b) the rest of the org-portal forms (e.g. teacher/units/new) use
 * native selects. Consistency over novelty.
 */
export function OrgSwitcher({ options }: OrgSwitcherProps) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  if (options.length < 2) return null;

  const urlOrgId = searchParams.get("orgId") ?? undefined;
  const current =
    urlOrgId && options.some((o) => o.orgId === urlOrgId)
      ? urlOrgId
      : options[0].orgId;

  const handleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const next = e.target.value;
    const params = new URLSearchParams(searchParams.toString());
    params.set("orgId", next);
    router.push(`${pathname}?${params.toString()}`);
  };

  return (
    <div className="flex items-center gap-2 px-6 py-3 border-b border-border/50">
      <label htmlFor="org-switcher" className="text-sm text-muted-foreground">
        Organization:
      </label>
      <select
        id="org-switcher"
        value={current}
        onChange={handleChange}
        className="rounded-md border border-input bg-background px-3 py-1 text-sm"
      >
        {options.map((opt) => (
          <option key={opt.orgId} value={opt.orgId}>
            {opt.orgName}
          </option>
        ))}
      </select>
    </div>
  );
}
