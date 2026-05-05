import { notFound } from "next/navigation";
import Link from "next/link";
import { api, ApiError } from "@/lib/api-client";
import { getIdentity } from "@/lib/identity";
import { ArchiveClassButton } from "@/components/org/archive-class-button";

interface ClassDetail {
  id: string;
  courseId: string;
  courseTitle: string;
  orgId: string;
  title: string;
  term: string;
  joinCode: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}

interface ClassMember {
  id: string;
  classId: string;
  userId: string;
  role: string;
  joinedAt: string;
  name: string;
  email: string;
}

function statusBadge(status: string) {
  if (status === "active")
    return "inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-medium bg-emerald-50 text-emerald-700 border-emerald-200";
  return "inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-medium bg-zinc-50 text-zinc-500 border-zinc-200";
}

function roleBadge(role: string) {
  if (role === "instructor")
    return "inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-medium bg-sky-50 text-sky-700 border-sky-200";
  if (role === "ta")
    return "inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-medium bg-indigo-50 text-indigo-700 border-indigo-200";
  return "inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-medium bg-zinc-50 text-zinc-500 border-zinc-200";
}

export default async function OrgClassDetailPage({
  params,
}: {
  params: Promise<{ classId: string }>;
}) {
  const { classId } = await params;

  const [identity, classResult, membersResult] = await Promise.allSettled([
    getIdentity(),
    api<ClassDetail>(`/api/classes/${classId}`),
    api<ClassMember[]>(`/api/classes/${classId}/members`),
  ]);

  // Class not found or no access → 404
  if (classResult.status === "rejected") {
    const err = classResult.reason;
    if (err instanceof ApiError && (err.status === 404 || err.status === 403 || err.status === 401)) {
      notFound();
    }
    throw err;
  }

  const cls = classResult.value;

  const members: ClassMember[] =
    membersResult.status === "fulfilled" ? membersResult.value : [];

  const identityValue = identity.status === "fulfilled" ? identity.value : null;

  // Determine caller's role in this class
  const myMembership = identityValue
    ? members.find((m) => m.userId === identityValue.userId)
    : undefined;
  const myRole = myMembership?.role ?? null;
  const canOpenTeacherPortal = myRole === "instructor" || myRole === "ta";

  const instructors = members.filter((m) => m.role === "instructor" || m.role === "ta");
  const students = members.filter((m) => m.role !== "instructor" && m.role !== "ta");

  return (
    <div className="p-6 space-y-6 max-w-4xl">
      {/* Header */}
      <div>
        <div className="flex items-center gap-3 flex-wrap">
          <h1 className="text-2xl font-bold">{cls.title}</h1>
          <span className={statusBadge(cls.status)}>{cls.status}</span>
        </div>
        <div className="mt-1 flex items-center gap-4 text-sm text-muted-foreground flex-wrap">
          {cls.courseTitle && (
            <span>Course: <span className="text-foreground">{cls.courseTitle}</span></span>
          )}
          {cls.term && (
            <span>Term: <span className="text-foreground">{cls.term}</span></span>
          )}
          <span>Join code: <span className="font-mono text-foreground">{cls.joinCode}</span></span>
        </div>
      </div>

      {/* Open in teacher portal */}
      {canOpenTeacherPortal && (
        <div>
          <Link
            href={`/teacher/classes/${classId}`}
            className="text-sm text-primary underline-offset-2 hover:underline"
          >
            Open in teacher portal
          </Link>
        </div>
      )}

      {/* Members table */}
      <div className="space-y-2">
        <h2 className="text-base font-semibold">Members ({members.length})</h2>

        {membersResult.status === "rejected" ? (
          <p className="text-sm text-muted-foreground">
            Could not load member list.
          </p>
        ) : members.length === 0 ? (
          <p className="text-sm text-muted-foreground">No members yet.</p>
        ) : (
          <div className="border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-muted/50">
                <tr className="text-left">
                  <th className="px-4 py-2 font-medium">Name</th>
                  <th className="px-4 py-2 font-medium">Email</th>
                  <th className="px-4 py-2 font-medium">Role</th>
                  <th className="px-4 py-2 font-medium">Joined</th>
                </tr>
              </thead>
              <tbody>
                {/* Instructors and TAs first */}
                {instructors.map((m) => (
                  <tr key={m.id} className="border-t">
                    <td className="px-4 py-2 font-medium">{m.name}</td>
                    <td className="px-4 py-2 text-muted-foreground">{m.email}</td>
                    <td className="px-4 py-2">
                      <span className={roleBadge(m.role)}>{m.role}</span>
                    </td>
                    <td className="px-4 py-2 text-muted-foreground">
                      {new Date(m.joinedAt).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
                {/* Students */}
                {students.map((m) => (
                  <tr key={m.id} className="border-t">
                    <td className="px-4 py-2">{m.name}</td>
                    <td className="px-4 py-2 text-muted-foreground">{m.email}</td>
                    <td className="px-4 py-2">
                      <span className={roleBadge(m.role)}>{m.role}</span>
                    </td>
                    <td className="px-4 py-2 text-muted-foreground">
                      {new Date(m.joinedAt).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Archive button — only when active */}
      {cls.status === "active" && (
        <div className="pt-2 border-t">
          <ArchiveClassButton classId={cls.id} title={cls.title} />
        </div>
      )}
    </div>
  );
}
