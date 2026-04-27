import { OrgListState, type OrgListError } from "./org-list-state";
import type { OrgMemberRow } from "./teachers-list";

interface StudentsListProps {
  data: OrgMemberRow[] | null;
  error: OrgListError | null;
}

export function StudentsList({ data, error }: StudentsListProps) {
  return (
    <OrgListState
      data={data}
      error={error}
      emptyMessage="No students yet. Students appear here once a teacher adds them to a class or they join via a class code."
      retryHref="/org/students"
    >
      <div className="border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-muted/50">
            <tr>
              <th className="text-left px-4 py-2">Name</th>
              <th className="text-left px-4 py-2">Email</th>
              <th className="text-left px-4 py-2">Joined</th>
            </tr>
          </thead>
          <tbody>
            {(data ?? []).map((row) => (
              <tr key={row.userId} className="border-t">
                <td className="px-4 py-2">{row.name}</td>
                <td className="px-4 py-2 text-muted-foreground">{row.email}</td>
                <td className="px-4 py-2 text-muted-foreground">
                  {new Date(row.joinedAt).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </OrgListState>
  );
}
