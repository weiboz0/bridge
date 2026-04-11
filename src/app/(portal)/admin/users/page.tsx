import { db } from "@/lib/db";
import { listUsers } from "@/lib/users";

export default async function AdminUsersPage() {
  const userList = await listUsers(db);

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Users ({userList.length})</h1>
      <div className="border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-muted/50">
            <tr>
              <th className="text-left px-4 py-2">Name</th>
              <th className="text-left px-4 py-2">Email</th>
              <th className="text-left px-4 py-2">Admin</th>
              <th className="text-left px-4 py-2">Joined</th>
            </tr>
          </thead>
          <tbody>
            {userList.map((user) => (
              <tr key={user.id} className="border-t">
                <td className="px-4 py-2">{user.name}</td>
                <td className="px-4 py-2 text-muted-foreground">{user.email}</td>
                <td className="px-4 py-2">{user.isPlatformAdmin ? "Yes" : ""}</td>
                <td className="px-4 py-2 text-muted-foreground">
                  {new Date(user.createdAt).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
