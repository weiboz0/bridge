"use client";

import { OrgListState, type OrgListError } from "./org-list-state";
import { MemberRowActions } from "./member-row-actions";
import { MemberStatusBadge } from "./member-status-badge";
import type { OrgMemberRow } from "./teachers-list";

interface StudentsListProps {
  data: OrgMemberRow[] | null;
  error: OrgListError | null;
  orgId: string;
  currentUserId: string;
}

export function StudentsList({
  data,
  error,
  orgId,
  currentUserId,
}: StudentsListProps) {
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
              <th className="text-left px-4 py-2">Status</th>
              <th className="text-left px-4 py-2">Joined</th>
              <th className="text-left px-4 py-2">Actions</th>
            </tr>
          </thead>
          <tbody>
            {(data ?? []).map((row) => (
              <tr key={row.userId} className="border-t">
                <td className="px-4 py-2">{row.name}</td>
                <td className="px-4 py-2 text-muted-foreground">{row.email}</td>
                <td className="px-4 py-2">
                  <MemberStatusBadge status={row.status} />
                </td>
                <td className="px-4 py-2 text-muted-foreground">
                  {new Date(row.joinedAt).toLocaleDateString()}
                </td>
                <td className="px-4 py-2">
                  <MemberRowActions
                    orgId={orgId}
                    member={{
                      membershipId: row.membershipId,
                      userId: row.userId,
                      name: row.name,
                      email: row.email,
                      status: row.status,
                    }}
                    isSelf={row.userId === currentUserId}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </OrgListState>
  );
}
