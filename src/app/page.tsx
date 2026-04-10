import { redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

export default async function Home() {
  const session = await auth();

  if (session) {
    redirect("/dashboard");
  }

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 p-4">
      <h1 className="text-4xl font-bold">Bridge</h1>
      <p className="text-lg text-muted-foreground text-center max-w-md">
        A live-first coding education platform for K-12 classrooms
      </p>
      <div className="flex gap-4">
        <Link href="/login" className={buttonVariants()}>
          Log In
        </Link>
        <Link href="/register" className={buttonVariants({ variant: "outline" })}>
          Sign Up
        </Link>
      </div>
    </main>
  );
}
