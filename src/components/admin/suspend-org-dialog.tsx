"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface Props {
  orgId: string;
  orgName: string;
  open: boolean;
  onClose: () => void;
  onSuspended: () => void;
}

export function SuspendOrgDialog({ orgId, orgName, open, onClose, onSuspended }: Props) {
  const [typed, setTyped] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Stable read of onClose from the keydown listener so the listener isn't
  // re-bound every parent render (callers commonly pass an inline arrow).
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  // Reset state each time the dialog opens.
  useEffect(() => {
    if (open) {
      setTyped("");
      setError(null);
    }
  }, [open]);

  // Close on Escape (suppress while submitting).
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !submitting) onCloseRef.current();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, submitting]);

  if (!open) return null;

  const confirmed = typed.trim() === orgName.trim();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!confirmed || submitting) return;
    setSubmitting(true);
    setError(null);
    let succeeded = false;
    try {
      const res = await fetch(`/api/admin/orgs/${orgId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: "suspended" }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        setError(body?.error ?? `Request failed (${res.status})`);
        return;
      }
      succeeded = true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
    // Run callbacks AFTER state is settled so the synchronous
    // setSubmitting(false) above isn't a dead update against an
    // already-unmounted dialog (caller's onSuspended → router.refresh
    // can re-render the parent, which then unmounts us via onClose).
    if (succeeded) {
      onSuspended();
      onClose();
    }
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="suspend-dialog-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        if (e.target === e.currentTarget && !submitting) onClose();
      }}
    >
      <div className="w-full max-w-md rounded-lg bg-background p-6 shadow-xl">
        <h2 id="suspend-dialog-title" className="text-lg font-semibold mb-3">
          Suspend organization
        </h2>
        <p className="text-sm text-muted-foreground mb-4">
          This will suspend <strong>{orgName}</strong>. Users in this org will
          lose access. Type the organization name below to confirm.
        </p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="suspend-confirm-input">
              Type organization name to confirm
            </Label>
            <Input
              id="suspend-confirm-input"
              autoFocus
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={orgName}
              disabled={submitting}
            />
          </div>

          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <Button
              type="button"
              variant="ghost"
              onClick={onClose}
              disabled={submitting}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant="destructive"
              disabled={!confirmed || submitting}
            >
              {submitting ? "Suspending…" : "Suspend organization"}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
