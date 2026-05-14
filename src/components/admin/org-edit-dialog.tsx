"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface Props {
  org: { id: string; name: string; contactName: string; contactEmail: string };
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
}

export function OrgEditDialog({ org, open, onClose, onSaved }: Props) {
  const [name, setName] = useState(org.name);
  const [contactName, setContactName] = useState(org.contactName);
  const [contactEmail, setContactEmail] = useState(org.contactEmail);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Stable read of onClose from the keydown listener so the listener isn't
  // re-bound every parent render (callers commonly pass an inline arrow).
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  // Reset form fields and error each time the dialog opens.
  useEffect(() => {
    if (open) {
      setName(org.name);
      setContactName(org.contactName);
      setContactEmail(org.contactEmail);
      setError(null);
    }
  }, [open, org.name, org.contactName, org.contactEmail]);

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

  const trimmedName = name.trim();
  const trimmedContactName = contactName.trim();
  const trimmedContactEmail = contactEmail.trim();

  // Save disabled when:
  // - ALL three fields are unchanged from initial values (no-op), OR
  // - Any field is empty after trim.
  const unchanged =
    trimmedName === org.name.trim() &&
    trimmedContactName === org.contactName.trim() &&
    trimmedContactEmail === org.contactEmail.trim();
  const anyEmpty = !trimmedName || !trimmedContactName || !trimmedContactEmail;
  const saveDisabled = unchanged || anyEmpty || submitting;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (saveDisabled) return;
    setSubmitting(true);
    setError(null);
    let succeeded = false;
    try {
      const res = await fetch(`/api/admin/orgs/${org.id}/details`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: trimmedName,
          contactName: trimmedContactName,
          contactEmail: trimmedContactEmail,
        }),
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
    // Run callbacks AFTER state is settled.
    if (succeeded) {
      onSaved();
      onCloseRef.current();
    }
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="org-edit-dialog-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        if (e.target === e.currentTarget && !submitting) onCloseRef.current();
      }}
    >
      <div className="w-full max-w-md rounded-lg bg-background p-6 shadow-xl">
        <h2 id="org-edit-dialog-title" className="text-lg font-semibold mb-4">
          Edit organization
        </h2>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="org-edit-name">Name</Label>
            <Input
              id="org-edit-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={submitting}
              placeholder="Organization name"
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="org-edit-contact-name">Contact name</Label>
            <Input
              id="org-edit-contact-name"
              value={contactName}
              onChange={(e) => setContactName(e.target.value)}
              disabled={submitting}
              placeholder="Contact name"
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="org-edit-contact-email">Contact email</Label>
            <Input
              id="org-edit-contact-email"
              type="email"
              value={contactEmail}
              onChange={(e) => setContactEmail(e.target.value)}
              disabled={submitting}
              placeholder="contact@example.com"
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
              onClick={() => onCloseRef.current()}
              disabled={submitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={saveDisabled}>
              {submitting ? "Saving…" : "Save"}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
