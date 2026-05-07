import { api, ApiError } from "@/lib/api-client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { resolveOrgContext, appendOrgId } from "@/lib/portal/org-context";
import { handleOrgContext } from "@/components/portal/org-context-guard";

interface OrgDashboardData {
  org: { id: string; name: string; type: string; status: string };
  teacherCount: number;
  studentCount: number;
  courseCount: number;
  classCount: number;
}

export default async function OrgDashboard({
  searchParams,
}: {
  searchParams?: Promise<{ orgId?: string }>;
}) {
  const sp = await searchParams;
  const ctx = await resolveOrgContext(sp);
  const handled = handleOrgContext(ctx);
  if (handled.kind === "guard") return handled.element;
  const { orgId } = handled;

  let data: OrgDashboardData;
  try {
    data = await api<OrgDashboardData>(appendOrgId("/api/org/dashboard", orgId));
  } catch (e) {
    if (e instanceof ApiError && (e.status === 403 || e.status === 404)) {
      return (
        <div className="p-6">
          <p className="text-muted-foreground">No organization found.</p>
        </div>
      );
    }
    throw e;
  }

  const { org, teacherCount, studentCount, courseCount, classCount } = data;

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{org.name}</h1>
        <p className="text-muted-foreground">{org.type} · {org.status}</p>
      </div>

      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Teachers</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{teacherCount}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Students</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{studentCount}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Courses</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{courseCount}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Classes</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{classCount}</p></CardContent>
        </Card>
      </div>
    </div>
  );
}
