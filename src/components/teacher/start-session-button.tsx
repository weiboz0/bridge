"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

interface StartSessionButtonProps {
  classId?: string;
  mode?: "class" | "orphan";
  buttonLabel?: string;
  defaultTitle?: string;
}

export function StartSessionButton({
  classId,
  mode = "class",
  buttonLabel,
  defaultTitle = "Untitled session",
}: StartSessionButtonProps) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState(defaultTitle);
  const [error, setError] = useState<string | null>(null);

  const isOrphan = mode === "orphan";
  const resolvedButtonLabel =
    buttonLabel ?? (isOrphan ? "Start Session" : "Start Live Session");

  async function createSession(sessionTitle: string) {
    setLoading(true);
    setError(null);

    const body = classId
      ? { title: sessionTitle, classId }
      : { title: sessionTitle };

    const res = await fetch("/api/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });

    if (res.ok) {
      const session = await res.json();
      router.push(`/teacher/sessions/${session.id}`);
    } else {
      const data = await res.json().catch(() => null);
      setError(data?.error || "Failed to start session");
      setLoading(false);
    }
  }

  async function handleStart() {
    await createSession(defaultTitle);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const sessionTitle = title.trim();
    if (!sessionTitle) {
      setError("Title is required");
      return;
    }

    await createSession(sessionTitle);
  }

  if (isOrphan) {
    return (
      <div className="space-y-2">
        {!open ? (
          <Button
            onClick={() => {
              setOpen(true);
              setError(null);
            }}
            className="bg-zinc-900 text-white hover:bg-zinc-800"
          >
            {resolvedButtonLabel}
          </Button>
        ) : (
          <form
            onSubmit={handleSubmit}
            className="flex flex-col gap-2 sm:flex-row sm:items-center"
          >
            <Input
              value={title}
              onChange={(event) => {
                setTitle(event.target.value);
                if (error) {
                  setError(null);
                }
              }}
              placeholder="Session title"
              className="w-full border-zinc-200 bg-white text-zinc-900 placeholder:text-zinc-400 sm:w-64"
              autoFocus
            />
            <div className="flex items-center gap-2">
              <Button
                type="submit"
                disabled={loading || title.trim().length === 0}
                className="bg-zinc-900 text-white hover:bg-zinc-800"
              >
                {loading ? "Starting..." : resolvedButtonLabel}
              </Button>
              <Button
                type="button"
                variant="outline"
                className="border-zinc-200 bg-white text-zinc-700 hover:bg-zinc-50 hover:text-zinc-900"
                onClick={() => {
                  setOpen(false);
                  setTitle(defaultTitle);
                  setError(null);
                }}
                disabled={loading}
              >
                Cancel
              </Button>
            </div>
          </form>
        )}
        {error && <p className="text-sm text-red-600">{error}</p>}
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <Button onClick={handleStart} disabled={loading}>
        {loading ? "Starting..." : resolvedButtonLabel}
      </Button>
      {error && <p className="text-sm text-red-600">{error}</p>}
    </div>
  );
}
