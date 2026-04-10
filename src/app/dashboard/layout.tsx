import { redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import Link from "next/link";
import { SignOutButton } from "@/components/sign-out-button";

export default async function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const session = await auth();

  if (!session) {
    redirect("/login");
  }

  return (
    <div className="min-h-screen flex flex-col">
      <header className="border-b">
        <div className="container mx-auto flex items-center justify-between h-14 px-4">
          <div className="flex items-center gap-6">
            <Link href="/dashboard" className="font-bold text-lg">
              Bridge
            </Link>
            <nav className="flex gap-4">
              <Link
                href="/dashboard/classrooms"
                className="text-sm text-muted-foreground hover:text-foreground"
              >
                Classrooms
              </Link>
            </nav>
          </div>
          <div className="flex items-center gap-4">
            <span className="text-sm text-muted-foreground">
              {session.user.name} ({session.user.role})
            </span>
            <SignOutButton />
          </div>
        </div>
      </header>
      <main className="flex-1 container mx-auto p-4">{children}</main>
    </div>
  );
}
