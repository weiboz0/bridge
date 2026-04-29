import { auth } from "@/lib/auth";
import { redirect } from "next/navigation";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { db } from "@/lib/db";
import { users } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

export default async function OnboardingPage() {
  const session = await auth();

  if (!session?.user?.id) {
    redirect("/login");
  }

  // Read the signup intent the user typed at registration so we can lead
  // with the right path instead of asking them to choose again. Nullable
  // for OAuth-only signups and pre-existing users.
  const [user] = await db
    .select({ intendedRole: users.intendedRole })
    .from(users)
    .where(eq(users.id, session.user.id));
  const intendedRole = user?.intendedRole ?? null;

  // Plan 047 phase 3: pre-047 we redirected teacher signups to
  // /register-org, but that route doesn't exist (review 006 P1 #3).
  // Until plan 048 ships the org self-registration form we lead with
  // the teacher card and tell them their school admin will invite
  // them — accurate for pre-production.
  const isTeacher = intendedRole === "teacher";
  const isStudent = intendedRole === "student";

  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle>Welcome to Bridge, {session.user.name}!</CardTitle>
          <CardDescription>
            {isStudent
              ? "Your teacher will add you to a class. Here's what to expect:"
              : "You're signed in but don't have any roles yet. Here's how to get started:"}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {isStudent && (
            <div className="border rounded-lg p-4 space-y-2 border-primary/40">
              <h3 className="font-medium">You&apos;re set up as a student</h3>
              <p className="text-sm text-muted-foreground">
                Your teacher will add you to a class. Once they do, you&apos;ll see
                your classes appear automatically. Have a join code? Sign in
                and click <span className="font-medium">Join a Class</span>.
              </p>
            </div>
          )}

          {isTeacher && (
            <div className="border rounded-lg p-4 space-y-2 border-primary/40">
              <h3 className="font-medium">You&apos;re set up as a teacher</h3>
              <p className="text-sm text-muted-foreground">
                Your school administrator will invite you to your organization
                — once that happens you&apos;ll see your classes and courses
                appear here. Org self-registration is coming in a future
                release.
              </p>
            </div>
          )}

          {!isStudent && !isTeacher && (
            <>
              <div className="border rounded-lg p-4 space-y-2">
                <h3 className="font-medium">I&apos;m a teacher or school administrator</h3>
                <p className="text-sm text-muted-foreground">
                  Your school administrator will invite you to your
                  organization. If you ARE the school admin, contact us
                  — org self-registration is coming in a future release.
                </p>
              </div>

              <div className="border rounded-lg p-4 space-y-2">
                <h3 className="font-medium">I&apos;m a student</h3>
                <p className="text-sm text-muted-foreground">
                  Your teacher will add you to a class. Once they do, you&apos;ll see your classes here.
                </p>
              </div>
            </>
          )}

          <div className="border rounded-lg p-4 space-y-2">
            <h3 className="font-medium">I&apos;m a parent</h3>
            <p className="text-sm text-muted-foreground">
              Your child&apos;s teacher will link your account. Once linked, you&apos;ll see your child&apos;s progress here.
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
