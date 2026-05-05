"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { OrgStudentRow } from "@/app/(portal)/org/parent-links/page";

// Plan 070 phase 2 — create-parent-link modal.
//
// Controlled-input form with:
//   - parent email (free text — backend resolves to user_id)
//   - child autocomplete from the org's eligible students
// On submit, POST /api/orgs/{orgId}/parent-links with
// `{ parentEmail, childUserId }`. 404/403/409 surface inline.

interface Props {
  orgId: string;
  students: OrgStudentRow[];
  onClose: () => void;
  onCreated: () => void;
}

export function CreateParentLinkModal({
  orgId,
  students,
  onClose,
  onCreated,
}: Props) {
  const [parentEmail, setParentEmail] = useState("");
  const [childQuery, setChildQuery] = useState("");
  const [childUserId, setChildUserId] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const dialogRef = useRef<HTMLDivElement | null>(null);

  // Autocomplete: case-insensitive match on name OR email.
  const suggestions = useMemo(() => {
    const q = childQuery.trim().toLowerCase();
    if (!q) return [];
    return students
      .filter(
        (s) =>
          s.name.toLowerCase().includes(q) ||
          s.email.toLowerCase().includes(q),
      )
      .slice(0, 8);
  }, [childQuery, students]);

  // Close on Escape.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  function selectChild(s: OrgStudentRow) {
    setChildUserId(s.userId);
    setChildQuery(`${s.name} <${s.email}>`);
  }

  function clearChild() {
    setChildUserId(null);
    setChildQuery("");
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (!parentEmail.trim()) {
      setError("Parent email is required");
      return;
    }
    if (!childUserId) {
      setError("Pick a child from the autocomplete suggestions");
      return;
    }

    setSubmitting(true);
    try {
      const res = await fetch(`/api/orgs/${orgId}/parent-links`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          parentEmail: parentEmail.trim(),
          childUserId,
        }),
      });
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as
          | { error?: string }
          | null;
        const msg = body?.error ?? `Request failed: ${res.status}`;
        if (res.status === 404) {
          setError(
            `${msg} — ask the parent to register at /register first, then try again.`,
          );
        } else {
          setError(msg);
        }
        return;
      }
      onCreated();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Add parent link"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        if (e.target === dialogRef.current) onClose();
      }}
    >
      <div
        ref={dialogRef}
        className="w-full max-w-md rounded-lg bg-background p-6 shadow-xl"
      >
        <h2 className="text-lg font-semibold mb-1">Add parent link</h2>
        <p className="text-sm text-muted-foreground mb-4">
          Wire a parent&apos;s account to a student. The parent will see the
          student&apos;s sessions and progress on next sign-in.
        </p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="parentEmail">Parent email</Label>
            <Input
              id="parentEmail"
              type="email"
              value={parentEmail}
              onChange={(e) => setParentEmail(e.target.value)}
              placeholder="parent@example.com"
              required
              autoFocus
            />
            <p className="text-xs text-muted-foreground">
              The parent must have a registered account first.
            </p>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="childQuery">Child</Label>
            <div className="relative">
              <Input
                id="childQuery"
                value={childQuery}
                onChange={(e) => {
                  setChildQuery(e.target.value);
                  if (childUserId) setChildUserId(null);
                }}
                placeholder="Search by name or email…"
                autoComplete="off"
              />
              {childUserId && (
                <button
                  type="button"
                  onClick={clearChild}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-xs text-muted-foreground hover:text-foreground"
                  aria-label="Clear child selection"
                >
                  ×
                </button>
              )}
              {!childUserId && suggestions.length > 0 && (
                <ul className="absolute z-10 mt-1 w-full max-h-48 overflow-auto rounded-md border border-input bg-background shadow-lg">
                  {suggestions.map((s) => (
                    <li key={s.userId}>
                      <button
                        type="button"
                        onClick={() => selectChild(s)}
                        className="w-full text-left px-3 py-2 text-sm hover:bg-muted"
                      >
                        <div className="font-medium">{s.name}</div>
                        <div className="text-xs text-muted-foreground font-mono">
                          {s.email}
                        </div>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            {students.length === 0 && (
              <p className="text-xs text-rose-700">
                No students enrolled in any active class yet — add students to
                a class first.
              </p>
            )}
          </div>

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
              {submitting ? "Linking…" : "Create link"}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
