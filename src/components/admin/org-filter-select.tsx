"use client";

interface Org {
  id: string;
  name: string;
}

interface Props {
  orgs: Org[];
  current: string | null;
  role: string | null;
}

export function OrgFilterSelect({ orgs, current, role }: Props) {
  return (
    <form method="get" action="/admin/users">
      {role && <input type="hidden" name="role" value={role} />}
      <select
        name="orgId"
        defaultValue={current ?? ""}
        onChange={(e) => e.currentTarget.form?.submit()}
        className="border rounded px-2 py-1 text-sm bg-background text-foreground"
      >
        <option value="">All orgs</option>
        {orgs.map((o) => (
          <option key={o.id} value={o.id}>
            {o.name}
          </option>
        ))}
      </select>
    </form>
  );
}
