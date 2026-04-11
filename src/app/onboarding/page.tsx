import { auth } from "@/lib/auth";
import { redirect } from "next/navigation";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default async function OnboardingPage() {
  const session = await auth();

  if (!session) {
    redirect("/login");
  }

  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle>Welcome to Bridge, {session.user.name}!</CardTitle>
          <CardDescription>
            You're signed in but don't have any roles yet. Here's how to get started:
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="border rounded-lg p-4 space-y-2">
            <h3 className="font-medium">I'm a teacher or school administrator</h3>
            <p className="text-sm text-muted-foreground">
              Register your organization to start creating courses and classes.
            </p>
            <Link href="/register-org" className={buttonVariants({ size: "sm" })}>
              Register Organization
            </Link>
          </div>

          <div className="border rounded-lg p-4 space-y-2">
            <h3 className="font-medium">I'm a student</h3>
            <p className="text-sm text-muted-foreground">
              Your teacher will add you to a class. Once they do, you'll see your classes here.
            </p>
          </div>

          <div className="border rounded-lg p-4 space-y-2">
            <h3 className="font-medium">I'm a parent</h3>
            <p className="text-sm text-muted-foreground">
              Your child's teacher will link your account. Once linked, you'll see your child's progress here.
            </p>
          </div>

          <div className="border rounded-lg p-4 space-y-2">
            <h3 className="font-medium">I was invited as a guest</h3>
            <p className="text-sm text-muted-foreground">
              The teacher who invited you will add you to their class. Check back soon.
            </p>
          </div>
        </CardContent>
      </Card>
    </main>
  );
}
