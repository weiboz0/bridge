import { OrgListState, type OrgListError } from "./org-list-state";

export interface OrgCourseRow {
  id: string;
  title: string;
  gradeLevel: string;
  language: string;
  createdAt: string;
}

interface CoursesListProps {
  data: OrgCourseRow[] | null;
  error: OrgListError | null;
}

export function CoursesList({ data, error }: CoursesListProps) {
  return (
    <OrgListState
      data={data}
      error={error}
      emptyMessage="No courses yet. Teachers create courses to organize their content."
      retryHref="/org/courses"
    >
      <div className="border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-muted/50">
            <tr>
              <th className="text-left px-4 py-2">Title</th>
              <th className="text-left px-4 py-2">Grade</th>
              <th className="text-left px-4 py-2">Language</th>
              <th className="text-left px-4 py-2">Created</th>
            </tr>
          </thead>
          <tbody>
            {(data ?? []).map((row) => (
              <tr key={row.id} className="border-t">
                <td className="px-4 py-2">{row.title}</td>
                <td className="px-4 py-2 text-muted-foreground">{row.gradeLevel}</td>
                <td className="px-4 py-2 text-muted-foreground">{row.language}</td>
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
