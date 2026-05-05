// Plan 069 phase 4 — shared status pill for org_memberships rows
// (active / pending / suspended). Used by teachers-list.tsx and
// students-list.tsx; previously duplicated. Extracted per GLM 5.1
// post-impl review F7.

interface Props {
  status: string;
}

export function MemberStatusBadge({ status }: Props) {
  const styles: Record<string, string> = {
    active: "bg-emerald-50 text-emerald-700 border border-emerald-200",
    pending: "bg-amber-50 text-amber-700 border border-amber-200",
    suspended: "bg-rose-50 text-rose-700 border border-rose-200",
  };
  const cls =
    styles[status] ?? "bg-muted text-muted-foreground border border-border";
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}
    >
      {status}
    </span>
  );
}
