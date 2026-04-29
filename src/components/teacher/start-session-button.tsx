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

// Plan 047 phase 4: when CreateSession returns 422 with one of these
// codes the UI shows a confirmation Dialog before re-POSTing with
// confirmUnlinkedTopics: true. The codes match the Go handler's
// checkUnlinkedTopicsGuard at platform/internal/handlers/sessions.go.
type UnlinkedTopicsGuard = {
  code: "all_topics_unlinked" | "some_topics_unlinked";
  unlinkedTopicTitles: string[];
  message: string;
};

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
  const [pendingTitle, setPendingTitle] = useState<string | null>(null);
  const [guard, setGuard] = useState<UnlinkedTopicsGuard | null>(null);

  const isOrphan = mode === "orphan";
  const resolvedButtonLabel =
    buttonLabel ?? (isOrphan ? "Start Session" : "Start Live Session");

  async function createSession(sessionTitle: string, confirmUnlinkedTopics = false) {
    setLoading(true);
    setError(null);

    const body: Record<string, unknown> = classId
      ? { title: sessionTitle, classId }
      : { title: sessionTitle };
    if (confirmUnlinkedTopics) body.confirmUnlinkedTopics = true;

    const res = await fetch("/api/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });

    // Plan 047 phase 4: 422 with an unlinked-topics code means show
    // the confirmation Dialog. Anything else (4xx/5xx) is a regular
    // error path. Decode the body once up-front so we don't try to
    // read it twice (the second .json() would throw).
    if (!res.ok) {
      const data = await res.json().catch(() => null);
      if (
        res.status === 422 &&
        (data?.code === "all_topics_unlinked" || data?.code === "some_topics_unlinked")
      ) {
        setGuard({
          code: data.code,
          unlinkedTopicTitles: data.unlinkedTopicTitles ?? [],
          message: data.error ?? "",
        });
        setPendingTitle(sessionTitle);
        setLoading(false);
        return;
      }
      setError(data?.error || "Failed to start session");
      setLoading(false);
      return;
    }

    const session = await res.json();
    router.push(`/teacher/sessions/${session.id}`);
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

  function dismissGuard() {
    setGuard(null);
    setPendingTitle(null);
  }

  async function confirmStartAnyway() {
    const t = pendingTitle ?? defaultTitle;
    setGuard(null);
    setPendingTitle(null);
    await createSession(t, true);
  }

  // Hand-rolled modal matches the project's existing pattern (see
  // src/components/teacher/unit-picker-dialog.tsx and
  // src/components/editor/tiptap/help-overlay.tsx).
  const guardDialog = guard ? (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="start-session-guard-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) dismissGuard();
      }}
    >
      <div className="w-full max-w-md rounded-lg border bg-background p-5 shadow-xl space-y-3">
        <h2 id="start-session-guard-title" className="text-lg font-semibold">
          {guard.code === "all_topics_unlinked"
            ? "No teaching units linked"
            : "Some focus areas have no material"}
        </h2>
        <p className="text-sm text-muted-foreground">
          {guard.code === "all_topics_unlinked"
            ? "Every focus area in this course has no teaching unit linked. Students will see “No material yet” for everything. Are you sure you want to start anyway?"
            : "These focus areas have no teaching unit linked. Students will see “No material yet” for those:"}
        </p>
        {guard.unlinkedTopicTitles.length > 0 && (
          <ul className="rounded border bg-muted/30 px-3 py-2 text-sm">
            {guard.unlinkedTopicTitles.map((t) => (
              <li key={t} className="font-medium">
                {t}
              </li>
            ))}
          </ul>
        )}
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" onClick={dismissGuard} disabled={loading}>
            Cancel
          </Button>
          <Button
            onClick={confirmStartAnyway}
            disabled={loading}
            className={
              guard.code === "all_topics_unlinked"
                ? "bg-amber-600 text-white hover:bg-amber-700"
                : undefined
            }
          >
            {loading ? "Starting..." : "Start anyway"}
          </Button>
        </div>
      </div>
    </div>
  ) : null;

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
        {guardDialog}
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <Button onClick={handleStart} disabled={loading}>
        {loading ? "Starting..." : resolvedButtonLabel}
      </Button>
      {error && <p className="text-sm text-red-600">{error}</p>}
      {guardDialog}
    </div>
  );
}
