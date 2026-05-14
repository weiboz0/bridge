"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface ConfirmDialogProps {
  open: boolean;
  onClose: () => void;
  onConfirm: () => Promise<void> | void;
  title: string;
  body: React.ReactNode;
  cancelLabel?: string;
  confirmLabel?: string;
  confirmingLabel?: string;
  destructive?: boolean;
  /**
   * If set, requires the user to type this string exactly (after trim) before
   * the confirm button enables. Used to add extra friction to auth-changing
   * operations that are reversible but high-impact (e.g. promote/demote
   * platform admin). For irreversible ops (suspend), use SuspendUserDialog /
   * SuspendOrgDialog instead — those are dedicated type-to-confirm dialogs.
   */
  typeToConfirm?: string;
  /**
   * Label for the type-to-confirm input. Defaults to "Type {typeToConfirm} to
   * confirm". Provided callers can customize wording (e.g. "Type the user's
   * name to confirm").
   */
  typeToConfirmLabel?: string;
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  body,
  cancelLabel = "Cancel",
  confirmLabel = "Confirm",
  confirmingLabel = "Confirming…",
  destructive = false,
  typeToConfirm,
  typeToConfirmLabel,
}: ConfirmDialogProps) {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [typed, setTyped] = useState("");

  // Stable read of onClose from the keydown listener so the listener isn't
  // re-bound every parent render (callers commonly pass an inline arrow).
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  // Reset error + typed state each time the dialog opens.
  useEffect(() => {
    if (open) {
      setError(null);
      setTyped("");
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

  const typeToConfirmRequired = typeof typeToConfirm === "string" && typeToConfirm.length > 0;
  const typeToConfirmSatisfied = !typeToConfirmRequired || typed.trim() === typeToConfirm.trim();
  const confirmDisabled = submitting || !typeToConfirmSatisfied;

  async function handleConfirm() {
    if (confirmDisabled) return;
    setSubmitting(true);
    setError(null);
    let succeeded = false;
    try {
      await onConfirm();
      succeeded = true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
    // Run onClose AFTER state is settled. Callers do NOT need to call
    // onClose from their onConfirm implementation — just resolve.
    if (succeeded) {
      onCloseRef.current();
    }
  }

  const titleId = "confirm-dialog-title";
  const inputId = "confirm-dialog-type-input";
  const resolvedTypeLabel =
    typeToConfirmLabel ?? `Type ${typeToConfirm ?? ""} to confirm`;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        if (e.target === e.currentTarget && !submitting) onCloseRef.current();
      }}
    >
      <div className="w-full max-w-md rounded-lg bg-background p-6 shadow-xl">
        <h2 id={titleId} className="text-lg font-semibold mb-3">
          {title}
        </h2>
        <div className="text-sm text-muted-foreground mb-4">{body}</div>

        {typeToConfirmRequired && (
          <div className="space-y-1.5 mb-4">
            <Label htmlFor={inputId}>{resolvedTypeLabel}</Label>
            <Input
              id={inputId}
              autoFocus
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={typeToConfirm}
              disabled={submitting}
            />
          </div>
        )}

        {error && (
          <p role="alert" className="text-sm text-destructive mb-4">
            {error}
          </p>
        )}

        <div className="flex justify-end gap-2 pt-2">
          <Button
            type="button"
            variant="ghost"
            onClick={() => onCloseRef.current()}
            disabled={submitting}
          >
            {cancelLabel}
          </Button>
          <Button
            type="button"
            autoFocus={!typeToConfirmRequired}
            variant={destructive ? "destructive" : "default"}
            disabled={confirmDisabled}
            onClick={handleConfirm}
          >
            {submitting ? confirmingLabel : confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
