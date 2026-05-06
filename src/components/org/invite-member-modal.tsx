"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

// Plan 069 phase 1 — invite-member modal.
//
// Posts to POST /api/orgs/{orgId}/members with { email, role }.
// Handles 200/201 (success), 404 (user not registered), 409 (already
// a member), and generic errors inline.

interface Props {
  orgId: string;
  role: "teacher" | "student";
  onClose: () => void;
  onSuccess: () => void;
}

export function InviteMemberModal({ orgId, role, onClose, onSuccess }: Props) {
  const [email, setEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);
  const [orgDomain, setOrgDomain] = useState<string | null>(null);
  const backdropRef = useRef<HTMLDivElement | null>(null);

  const label = role === "teacher" ? "Teacher" : "Student";

  // Fetch the org's domain once on mount so we can warn on domain mismatches.
  // DeepSeek post-impl review: AbortController prevents the resolved
  // setOrgDomain from firing on an unmounted modal (React state-on-unmount
  // warning + potential memory leak on slow networks).
  useEffect(() => {
    const controller = new AbortController();
    fetch(`/api/orgs/${orgId}`, {
      credentials: "include",
      signal: controller.signal,
    })
      .then((res) => (res.ok ? res.json() : null))
      .then((data: { domain?: string | null } | null) => {
        if (data?.domain) setOrgDomain(data.domain);
      })
      .catch(() => {
        // Fetch failed (network/abort/non-ok body) — domain check simply
        // won't fire; backend still validates the invite itself.
      });
    return () => controller.abort();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Close on Escape key.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setNotFound(false);

    const trimmedEmail = email.trim();
    if (!trimmedEmail) {
      setError("Email is required");
      return;
    }

    // Domain mismatch check — warn before submitting if the org has a domain set.
    if (orgDomain) {
      const inviteeDomain = trimmedEmail.split("@")[1]?.toLowerCase().trim();
      if (inviteeDomain && inviteeDomain !== orgDomain.toLowerCase().trim()) {
        const proceed = window.confirm(
          // Plain double-quotes render cleanly in native confirm() dialogs;
          // backticks (which Markdown uses for inline code) appear as literal
          // backticks in the OS-level dialog (Codex post-impl Q4).
          `This email's domain "${inviteeDomain}" doesn't match the org's domain "${orgDomain}". Continue anyway?`
        );
        if (!proceed) return;
      }
    }

    setSubmitting(true);
    try {
      const res = await fetch(`/api/orgs/${orgId}/members`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: trimmedEmail, role }),
      });

      if (res.ok) {
        onSuccess();
        return;
      }

      const body = (await res.json().catch(() => null)) as
        | { error?: string }
        | null;

      if (res.status === 404) {
        setNotFound(true);
        return;
      }

      if (res.status === 409) {
        setError("Already a member of this org.");
        return;
      }

      setError(body?.error ?? `Request failed: ${res.status}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  function copyRegisterLink() {
    const url = window.location.origin + "/register";
    // Codex post-impl BLOCKER: `navigator.clipboard` is undefined in
    // insecure contexts (HTTP, file://). Without this guard, the
    // writeText() call throws a TypeError synchronously, before the
    // .catch() fallback runs. The inline /register URL in the 404
    // error block remains the manual-copy fallback either way.
    if (typeof navigator !== "undefined" && navigator.clipboard) {
      navigator.clipboard.writeText(url).catch(() => {
        // Clipboard write rejected (permission denied, etc.) — silent fail;
        // user can still copy manually from the inline text.
      });
    }
  }

  return (
    <div
      ref={backdropRef}
      role="dialog"
      aria-modal="true"
      aria-label={`Invite ${label}`}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        if (e.target === backdropRef.current) onClose();
      }}
    >
      <div className="w-full max-w-md rounded-lg bg-background p-6 shadow-xl">
        <h2 className="text-lg font-semibold mb-1">Invite {label}</h2>
        <p className="text-sm text-muted-foreground mb-4">
          Enter the email address of the {role} you want to add to this
          organization. They must have a registered account first.
        </p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="inviteEmail">Email</Label>
            <Input
              id="inviteEmail"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder={`${role}@example.com`}
              required
              autoFocus
            />
          </div>

          {notFound && (
            <div
              role="alert"
              className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 space-y-1.5"
            >
              <p>
                User not found. Ask them to register at{" "}
                <span className="font-mono">/register</span> first, then come
                back and try again.
              </p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={copyRegisterLink}
                className="text-xs"
              >
                Copy registration link
              </Button>
            </div>
          )}

          {error && (
            <div
              role="alert"
              className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
            >
              {error}
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={submitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? "Inviting…" : `Invite ${label}`}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
