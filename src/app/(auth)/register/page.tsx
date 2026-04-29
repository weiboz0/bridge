"use client";

import { Suspense, useState } from "react";
import { signIn } from "next-auth/react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  CardFooter,
} from "@/components/ui/card";

function RegisterForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  // ?invite=<class-join-code> means the user landed here from a class
  // invite link. Default to "student" intent and carry the code through
  // registration so the API can enroll them in one round-trip.
  const inviteCode = searchParams.get("invite") || "";
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<"teacher" | "student">(
    inviteCode ? "student" : "teacher"
  );
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");

    // Plan 047 phase 3: send `intendedRole` (matches the Go handler
    // and the schema column name `users.intended_role`). The previous
    // `role` field name was silently dropped by the Go register
    // endpoint that next.config.ts proxies to.
    const res = await fetch("/api/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, email, password, intendedRole: role }),
    });

    if (!res.ok) {
      const data = await res.json();
      setError(data.error || "Registration failed");
      setLoading(false);
      return;
    }

    const result = await signIn("credentials", {
      email,
      password,
      redirect: false,
    });

    if (result?.error) {
      setError("Account created but sign-in failed. Please log in.");
      setLoading(false);
    } else {
      // Carry the invite code into the destination so the student
      // dashboard can auto-open the join dialog with it prefilled. This
      // avoids any server-side DB-write coupling between register and
      // class-join while still honoring the invite-link intent.
      router.push(inviteCode ? `/student?invite=${encodeURIComponent(inviteCode)}` : "/");
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Create an Account</CardTitle>
          <CardDescription>
            {inviteCode
              ? `Join your class with code ${inviteCode}`
              : "Join Bridge to start teaching or learning"}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Button
            variant="outline"
            className="w-full"
            onClick={async () => {
              // Plan 043 phase 5: persist the user's chosen role + invite
              // code before redirecting to Google, so the Auth.js signIn
              // callback can read them back when the OAuth round-trip
              // creates the user. Best-effort — if the cookie write
              // fails, fall through to the OAuth flow without intent
              // (matches pre-043 behavior).
              try {
                await fetch("/api/auth/signup-intent", {
                  method: "POST",
                  headers: { "Content-Type": "application/json" },
                  body: JSON.stringify({
                    role,
                    ...(inviteCode ? { inviteCode } : {}),
                  }),
                });
              } catch {
                // ignore — proceed with sign-in anyway
              }
              const callbackUrl = inviteCode
                ? `/student?invite=${encodeURIComponent(inviteCode)}`
                : "/";
              await signIn("google", { callbackUrl });
            }}
          >
            Sign up with Google
          </Button>

          <div className="relative">
            <div className="absolute inset-0 flex items-center">
              <span className="w-full border-t" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-background px-2 text-muted-foreground">
                Or with email
              </span>
            </div>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <p className="text-sm text-destructive text-center">{error}</p>
            )}
            <div className="space-y-2">
              <Label htmlFor="name">Full Name</Label>
              <Input
                id="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                minLength={8}
                required
              />
            </div>
            <div className="space-y-2">
              <Label>I am a...</Label>
              <div className="flex gap-4">
                <Button
                  type="button"
                  variant={role === "teacher" ? "default" : "outline"}
                  className="flex-1"
                  onClick={() => setRole("teacher")}
                >
                  Teacher
                </Button>
                <Button
                  type="button"
                  variant={role === "student" ? "default" : "outline"}
                  className="flex-1"
                  onClick={() => setRole("student")}
                >
                  Student
                </Button>
              </div>
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? "Creating account..." : "Create Account"}
            </Button>
          </form>
        </CardContent>
        <CardFooter className="justify-center">
          <p className="text-sm text-muted-foreground">
            Already have an account?{" "}
            <Link href="/login" className="underline">
              Log in
            </Link>
          </p>
        </CardFooter>
      </Card>
    </main>
  );
}

export default function RegisterPage() {
  return (
    <Suspense>
      <RegisterForm />
    </Suspense>
  );
}
