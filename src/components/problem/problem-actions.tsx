"use client";

import { useRouter } from "next/navigation";
import Link from "next/link";
import { useState } from "react";
import { Button, buttonVariants } from "@/components/ui/button";
import { ApiError } from "@/lib/api-client";
import type { ProblemDetailData } from "./teacher-problem-detail";

// Plan 066 phase 2 — action buttons for the problem detail page.
// Edit / Publish / Archive / Unarchive / Fork / Delete. Each button
// is a thin client wrapper that POSTs to the matching Go endpoint
// and refreshes the route on success.

interface Props {
  problem: ProblemDetailData;
  canAuthor: boolean;
}

export function ProblemActions({ problem, canAuthor }: Props) {
  const router = useRouter();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Helper to POST to a path and refresh on success. Sends an empty
  // JSON object body — Codex pass-1 caught that the Fork handler
  // (and likely future handlers) call decodeJSON unconditionally,
  // which 400s on a missing/empty body. `{}` is parsed as zero-value
  // for the body struct (the handler then applies defaults), so the
  // body stays minimal but the JSON parse always succeeds.
  async function postAction(action: string, path: string) {
    setBusy(action);
    setError(null);
    try {
      const res = await fetch(path, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: "{}",
      });
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as { error?: string } | null;
        throw new ApiError(res.status, body?.error ?? `Action failed: ${res.status}`, body);
      }
      // Fork redirects to the new copy; everything else just refreshes.
      if (action === "fork") {
        const body = (await res.json()) as { id?: string };
        if (body.id) {
          router.push(`/teacher/problems/${body.id}`);
          return;
        }
      }
      router.refresh();
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e);
      setError(msg);
    } finally {
      setBusy(null);
    }
  }

  async function deleteAction() {
    if (!confirm(
      `Delete "${problem.title}"?\n\nThis cannot be undone. ` +
      `If the problem is attached to any focus areas or has been attempted by students, the delete will be refused — remove those first.`
    )) {
      return;
    }
    setBusy("delete");
    setError(null);
    try {
      const res = await fetch(`/api/problems/${problem.id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as { error?: string } | null;
        // Codex pass-1 §"Delete cascades" — backend returns 409 with
        // wording about "topics"; remap to "focus areas" in the user
        // message since the rest of the teacher portal uses that
        // terminology.
        let msg = body?.error ?? `Delete failed: ${res.status}`;
        msg = msg.replace(/topic(s)?/gi, (match) =>
          match[0] === match[0].toUpperCase() ? "Focus area" + (match.endsWith("s") ? "s" : "") : "focus area" + (match.endsWith("s") ? "s" : "")
        );
        throw new Error(msg);
      }
      router.push("/teacher/problems");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setBusy(null);
    }
  }

  if (!canAuthor) {
    // Non-authors see only Fork. Everyone with read access can clone
    // a problem to their personal scope.
    return (
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          variant="outline"
          onClick={() => postAction("fork", `/api/problems/${problem.id}/fork`)}
          disabled={busy !== null}
        >
          {busy === "fork" ? "Forking…" : "Fork to Personal"}
        </Button>
        {error && <ErrorMessage message={error} onClear={() => setError(null)} />}
      </div>
    );
  }

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <Link
          href={`/teacher/problems/${problem.id}/edit`}
          className={buttonVariants({ size: "sm", variant: "outline" })}
        >
          Edit
        </Link>
        {problem.status === "draft" && (
          <Button
            size="sm"
            onClick={() => postAction("publish", `/api/problems/${problem.id}/publish`)}
            disabled={busy !== null}
          >
            {busy === "publish" ? "Publishing…" : "Publish"}
          </Button>
        )}
        {problem.status === "published" && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => postAction("archive", `/api/problems/${problem.id}/archive`)}
            disabled={busy !== null}
          >
            {busy === "archive" ? "Archiving…" : "Archive"}
          </Button>
        )}
        {problem.status === "archived" && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => postAction("unarchive", `/api/problems/${problem.id}/unarchive`)}
            disabled={busy !== null}
          >
            {busy === "unarchive" ? "Unarchiving…" : "Unarchive"}
          </Button>
        )}
        <Button
          size="sm"
          variant="outline"
          onClick={() => postAction("fork", `/api/problems/${problem.id}/fork`)}
          disabled={busy !== null}
        >
          {busy === "fork" ? "Forking…" : "Fork"}
        </Button>
        <Button
          size="sm"
          variant="destructive"
          onClick={deleteAction}
          disabled={busy !== null}
        >
          {busy === "delete" ? "Deleting…" : "Delete"}
        </Button>
      </div>
      {error && <ErrorMessage message={error} onClear={() => setError(null)} />}
    </div>
  );
}

function ErrorMessage({ message, onClear }: { message: string; onClear: () => void }) {
  return (
    <div
      role="alert"
      className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800 max-w-md"
    >
      <div className="flex items-start justify-between gap-2">
        <span>{message}</span>
        <button
          onClick={onClear}
          className="text-rose-600 hover:text-rose-800"
          aria-label="Dismiss"
        >
          ×
        </button>
      </div>
    </div>
  );
}
