import Link from "next/link";
import { OrgListState, type OrgListError } from "./org-list-state";

export interface OrgClassRow {
  id: string;
  title: string;
  term: string;
  status: string;
  courseId: string;
  courseTitle: string;
  instructorCount: number;
  studentCount: number;
  createdAt: string;
}

interface ClassesListProps {
  data: OrgClassRow[] | null;
  error: OrgListError | null;
  orgId?: string;
}

export function ClassesList({ data, error, orgId }: ClassesListProps) {
  return (
    <OrgListState
      data={data}
      error={error}
      emptyMessage="No classes yet. Teachers create classes from a course to start a term."
      retryHref="/org/classes"
    >
      <div className="border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-muted/50">
            <tr>
              <th className="text-left px-4 py-2">Title</th>
              <th className="text-left px-4 py-2">Course</th>
              <th className="text-left px-4 py-2">Term</th>
              <th className="text-left px-4 py-2">Instructors</th>
              <th className="text-left px-4 py-2">Students</th>
              <th className="text-left px-4 py-2">Created</th>
            </tr>
          </thead>
          <tbody>
            {(data ?? []).map((row) => (
              <tr key={row.id} className="border-t">
                <td className="px-4 py-2">
                  <Link
                    href={`/org/classes/${row.id}${orgId ? `?orgId=${encodeURIComponent(orgId)}` : ""}`}
                    className="text-primary underline-offset-2 hover:underline"
                  >
                    {row.title}
                  </Link>
                </td>
                <td className="px-4 py-2 text-muted-foreground">
                  {row.courseTitle || <span className="italic">unlinked</span>}
                </td>
                <td className="px-4 py-2 text-muted-foreground">{row.term || "—"}</td>
                <td className="px-4 py-2 text-muted-foreground">{row.instructorCount}</td>
                <td className="px-4 py-2 text-muted-foreground">{row.studentCount}</td>
                <td className="px-4 py-2 text-muted-foreground">
                  {new Date(row.createdAt).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </OrgListState>
  );
}
