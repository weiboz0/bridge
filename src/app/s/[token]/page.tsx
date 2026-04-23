"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { useSession } from "next-auth/react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";

type JoinState =
  | { status: "loading" }
  | { status: "success"; sessionId: string; classId: string | null }
  | { status: "not_found" }
  | { status: "expired" }
  | { status: "error"; message: string };

export default function JoinByTokenPage() {
  const params = useParams<{ token: string }>();
  const router = useRouter();
  const { status: authStatus } = useSession();
  const [state, setState] = useState<JoinState>({ status: "loading" });

  useEffect(() => {
    if (authStatus === "loading") return;

    if (authStatus === "unauthenticated") {
      // Redirect to login with a return URL back here
      const returnUrl = encodeURIComponent(`/s/${params.token}`);
      router.replace(`/login?callbackUrl=${returnUrl}`);
      return;
    }

    async function joinSession() {
      try {
        const res = await fetch(`/api/s/${params.token}/join`, {
          method: "POST",
        });

        if (res.ok) {
          const data = await res.json();
          setState({
            status: "success",
            sessionId: data.sessionId,
            classId: data.classId ?? null,
          });
        } else if (res.status === 404) {
          setState({ status: "not_found" });
        } else if (res.status === 410) {
          setState({ status: "expired" });
        } else {
          const body = await res.json().catch(() => null);
          setState({
            status: "error",
            message: body?.error ?? "Something went wrong. Please try again.",
          });
        }
      } catch {
        setState({
          status: "error",
          message: "Network error. Please check your connection and try again.",
        });
      }
    }

    joinSession();
  }, [authStatus, params.token, router]);

  // Redirect on success
  useEffect(() => {
    if (state.status !== "success") return;

    if (state.classId) {
      router.replace(
        `/student/classes/${state.classId}/session/${state.sessionId}`
      );
    } else {
      // Orphan session (no class) — future support
      router.replace(`/student/session/${state.sessionId}`);
    }
  }, [state, router]);

  return (
    <main className="flex min-h-screen items-center justify-center bg-zinc-50 p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Join Session</CardTitle>
          <CardDescription>
            {state.status === "loading" && "Joining session..."}
            {state.status === "success" && "Redirecting to session..."}
            {state.status === "not_found" &&
              "This invite link is invalid or has been rotated."}
            {state.status === "expired" &&
              "This invite has expired or the session has ended."}
            {state.status === "error" && state.message}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {(state.status === "not_found" ||
            state.status === "expired" ||
            state.status === "error") && (
            <div className="flex flex-col gap-2">
              <Button variant="outline" onClick={() => router.push("/")}>
                Go Home
              </Button>
            </div>
          )}
          {(state.status === "loading" || state.status === "success") && (
            <div className="flex justify-center py-4">
              <div className="h-6 w-6 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-600" />
            </div>
          )}
        </CardContent>
      </Card>
    </main>
  );
}
