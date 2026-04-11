import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getUserMemberships } from "@/lib/org-memberships";
import { getOrganization } from "@/lib/organizations";
import { listOrgMembers } from "@/lib/org-memberships";
import { listCoursesByOrg } from "@/lib/courses";
import { listClassesByOrg } from "@/lib/classes";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default async function OrgDashboard() {
  const session = await auth();
  const memberships = await getUserMemberships(db, session!.user.id);
  const orgAdminMembership = memberships.find((m) => m.role === "org_admin" && m.status === "active");

  if (!orgAdminMembership) {
    return <div className="p-6"><p className="text-muted-foreground">No organization found.</p></div>;
  }

  const org = await getOrganization(db, orgAdminMembership.orgId);
  if (!org) {
    return <div className="p-6"><p className="text-muted-foreground">Organization not found.</p></div>;
  }

  const [members, courses, classes] = await Promise.all([
    listOrgMembers(db, org.id),
    listCoursesByOrg(db, org.id),
    listClassesByOrg(db, org.id),
  ]);

  const teachers = members.filter((m) => m.role === "teacher");
  const students = members.filter((m) => m.role === "student");

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{org.name}</h1>
        <p className="text-muted-foreground">{org.type} · {org.status}</p>
      </div>

      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Teachers</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{teachers.length}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Students</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{students.length}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Courses</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{courses.length}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-muted-foreground">Classes</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{classes.length}</p></CardContent>
        </Card>
      </div>
    </div>
  );
}
