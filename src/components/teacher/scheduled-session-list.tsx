"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

interface ScheduledSession {
  id: string;
  classId: string;
  title: string | null;
  scheduledStart: string;
  scheduledEnd: string;
  status: string;
}

interface ScheduledSessionListProps {
  classId: string;
}

// Plan 094 phase 11: starting a scheduled session shares CreateSession's
// class-replacement contract (StartScheduledSession → the same
// session_lifecycle.go replacement path), so a scheduled start must warn
// about a replaced session's whiteboard archive exactly like
// StartSessionButton does before navigating away.
type ReplacedSession = { id: string; whiteboardServerArchiveComplete: boolean };

function describeReplacedSessions(replaced: ReplacedSession[]): string | null {
  if (replaced.length === 0) return null;
  const incomplete = replaced.filter((session) => !session.whiteboardServerArchiveComplete).length;
  const suffix = replaced.length === 1 ? "session" : "sessions";
  return incomplete > 0
    ? `Starting this session ended ${replaced.length} other live ${suffix} for this class. ${incomplete === replaced.length ? "Its" : "Some of its"} whiteboard archive may be incomplete.`
    : `Starting this session ended ${replaced.length} other live ${suffix} for this class.`;
}

export function ScheduledSessionList({ classId }: ScheduledSessionListProps) {
  const router = useRouter();
  const [schedules, setSchedules] = useState<ScheduledSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [startingId, setStartingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [replacedWarning, setReplacedWarning] = useState<{ message: string; destination: string } | null>(null);

  const loadSchedules = useCallback(async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/classes/${classId}/schedule`);
      if (!response.ok) {
        setSchedules([]);
        return;
      }
      const payload = await response.json() as ScheduledSession[];
      setSchedules(Array.isArray(payload) ? payload.filter((item) => item.status === "planned") : []);
    } catch {
      setSchedules([]);
    } finally {
      setLoading(false);
    }
  }, [classId]);

  useEffect(() => {
    void loadSchedules();
  }, [loadSchedules]);

  async function startSchedule(scheduleId: string) {
    setStartingId(scheduleId);
    setError(null);
    try {
      const response = await fetch(`/api/schedule/${scheduleId}/start`, { method: "POST" });
      if (!response.ok) {
        const body = await response.json().catch(() => null) as { error?: string } | null;
        setError(body?.error || "Unable to start this scheduled session");
        return;
      }
      const session = await response.json() as { id: string; classId?: string | null; replacedSessions?: ReplacedSession[] };
      const destination = session.classId
        ? `/teacher/sessions/${session.id}`
        : `/sessions/${session.id}`;
      const warningMessage = describeReplacedSessions(session.replacedSessions ?? []);
      if (warningMessage) {
        setReplacedWarning({ message: warningMessage, destination });
        return;
      }
      router.push(destination);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to start this scheduled session");
    } finally {
      setStartingId(null);
    }
  }

  function acknowledgeReplacedWarning() {
    if (!replacedWarning) return;
    router.push(replacedWarning.destination);
    setReplacedWarning(null);
  }

  if (loading) return null;
  if (schedules.length === 0) return null;

  return (
    <div>
      <h2 className="mb-3 text-lg font-semibold">Scheduled sessions ({schedules.length})</h2>
      <div className="space-y-2">
        {schedules.map((schedule) => (
          <div key={schedule.id} className="flex items-center justify-between rounded-lg border px-4 py-3">
            <div>
              <p className="text-sm font-medium">{schedule.title || "Untitled session"}</p>
              <p className="text-xs text-muted-foreground">
                {new Date(schedule.scheduledStart).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })}
              </p>
            </div>
            <Button
              size="sm"
              disabled={startingId === schedule.id}
              onClick={() => void startSchedule(schedule.id)}
            >
              {startingId === schedule.id ? "Starting…" : "Start now"}
            </Button>
          </div>
        ))}
      </div>
      {error && (
        <p role="alert" className="mt-2 text-sm text-destructive">
          {error}
        </p>
      )}
      {replacedWarning && (
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="scheduled-session-replaced-warning-title"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
        >
          <div className="w-full max-w-md rounded-lg border bg-background p-5 shadow-xl space-y-3">
            <h2 id="scheduled-session-replaced-warning-title" className="text-lg font-semibold">
              Previous session ended
            </h2>
            <p className="text-sm text-muted-foreground">{replacedWarning.message}</p>
            <div className="flex justify-end pt-2">
              <Button onClick={acknowledgeReplacedWarning}>Continue</Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
