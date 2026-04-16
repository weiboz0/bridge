import { api } from "@/lib/api-client";
import { auth } from "@/lib/auth";
import { UserActions } from "@/components/admin/user-actions";

interface UserItem {
  id: string;
  name: string;
  email: string;
  isPlatformAdmin: boolean;
  createdAt: string;
}

export default async function AdminUsersPage() {
  const session = await auth();
  const userList = await api<UserItem[]>("/api/admin/users");

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
              <th className="text-right px-4 py-2">Actions</th>
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
                <td className="px-4 py-2 text-right">
                  {user.id !== session?.user?.id && (
                    <UserActions userId={user.id} userName={user.name} />
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
