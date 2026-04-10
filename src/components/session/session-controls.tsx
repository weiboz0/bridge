"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

interface SessionControlsProps {
  classroomId: string;
  isTeacher: boolean;
  activeSessionId: string | null;
}

export function SessionControls({
  classroomId,
  isTeacher,
  activeSessionId,
}: SessionControlsProps) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);

  async function startSession() {
    setLoading(true);
    const res = await fetch("/api/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ classroomId }),
    });

    if (res.ok) {
      const session = await res.json();
      router.push(
        `/dashboard/classrooms/${classroomId}/session/${session.id}/dashboard`
      );
    }
    setLoading(false);
  }

  async function joinSession() {
    if (!activeSessionId) return;
    setLoading(true);

    await fetch(`/api/sessions/${activeSessionId}/join`, {
      method: "POST",
    });

    router.push(
      `/dashboard/classrooms/${classroomId}/session/${activeSessionId}`
    );
    setLoading(false);
  }

  if (isTeacher) {
    return (
      <div className="flex gap-2">
        <Button onClick={startSession} disabled={loading}>
          {loading ? "Starting..." : activeSessionId ? "Restart Session" : "Start Session"}
        </Button>
        {activeSessionId && (
          <Button
            variant="outline"
            onClick={() =>
              router.push(
                `/dashboard/classrooms/${classroomId}/session/${activeSessionId}/dashboard`
              )
            }
          >
            Open Dashboard
          </Button>
        )}
      </div>
    );
  }

  if (!activeSessionId) {
    return (
      <p className="text-sm text-muted-foreground">
        No active session. Wait for your teacher to start one.
      </p>
    );
  }

  return (
    <Button onClick={joinSession} disabled={loading}>
      {loading ? "Joining..." : "Join Session"}
    </Button>
  );
}
