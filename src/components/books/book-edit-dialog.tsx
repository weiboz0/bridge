"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export interface Book {
  id: string;
  title: string;
  description: string;
  scope: "platform" | "org";
  scopeId: string | null;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

interface Props {
  book?: {
    id: string;
    title: string;
    description: string;
    scope: "platform" | "org";
    scopeId: string | null;
  };
  open: boolean;
  onClose: () => void;
  onSaved: (book: Book) => void;
  availableOrgs?: { id: string; name: string }[];
}

export function BookEditDialog({ book, open, onClose, onSaved, availableOrgs = [] }: Props) {
  const isEdit = book !== undefined;

  const [title, setTitle] = useState(book?.title ?? "");
  const [description, setDescription] = useState(book?.description ?? "");
  const [scope, setScope] = useState<"platform" | "org">(book?.scope ?? "org");
  const [scopeId, setScopeId] = useState<string>(book?.scopeId ?? availableOrgs[0]?.id ?? "");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Stable read of onClose from the keydown listener so the listener isn't
  // re-bound every parent render (callers commonly pass an inline arrow).
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  // Reset form fields and error each time the dialog opens.
  // NOTE: availableOrgs intentionally omitted from deps — it's a new array
  // reference on every parent render (default []), including on every
  // fireEvent.change in tests. Including it would reset the form mid-edit.
  // The initial scopeId value (from useState) captures availableOrgs[0] at
  // mount time; resets on re-open read it via the closure at that moment.
  useEffect(() => {
    if (open) {
      setTitle(book?.title ?? "");
      setDescription(book?.description ?? "");
      setScope(book?.scope ?? "org");
      setScopeId(book?.scopeId ?? availableOrgs[0]?.id ?? "");
      setError(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, book?.title, book?.description, book?.scope, book?.scopeId]);

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

  const trimmedTitle = title.trim();
  const trimmedDescription = description.trim();

  // Save disabled conditions:
  // - Title empty after trim
  // - Create mode + org scope + no org selected
  // - Edit mode + all fields unchanged
  const orgScopeWithoutOrg = !isEdit && scope === "org" && !scopeId;
  let saveDisabled: boolean;
  if (!trimmedTitle) {
    saveDisabled = true;
  } else if (orgScopeWithoutOrg) {
    saveDisabled = true;
  } else if (submitting) {
    saveDisabled = true;
  } else if (isEdit && book) {
    const unchanged =
      trimmedTitle === book.title.trim() &&
      trimmedDescription === (book.description ?? "").trim();
    saveDisabled = unchanged;
  } else {
    saveDisabled = false;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (saveDisabled) return;
    setSubmitting(true);
    setError(null);
    let saved: Book | null = null;
    try {
      let res: Response;
      if (isEdit && book) {
        res = await fetch(`/api/books/${book.id}`, {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ title: trimmedTitle, description: trimmedDescription }),
        });
      } else {
        res = await fetch("/api/books", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            title: trimmedTitle,
            description: trimmedDescription,
            scope,
            scopeId: scope === "org" ? scopeId : null,
          }),
        });
      }
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        setError(body?.error ?? `Request failed (${res.status})`);
        return;
      }
      saved = await res.json();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
    // Run callbacks AFTER state is settled.
    if (saved) {
      onSaved(saved);
      onCloseRef.current();
    }
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="book-edit-dialog-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        if (e.target === e.currentTarget && !submitting) onCloseRef.current();
      }}
    >
      <div className="w-full max-w-md rounded-lg bg-background p-6 shadow-xl">
        <h2 id="book-edit-dialog-title" className="text-lg font-semibold mb-4">
          {isEdit ? "Edit book" : "New book"}
        </h2>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="book-edit-title">Title</Label>
            <Input
              id="book-edit-title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              disabled={submitting}
              placeholder="Book title"
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="book-edit-description">Description</Label>
            <Input
              id="book-edit-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={submitting}
              placeholder="Optional description"
            />
          </div>

          {!isEdit && (
            <div className="space-y-1.5">
              <Label>Scope</Label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input
                    type="radio"
                    name="book-scope"
                    value="org"
                    checked={scope === "org"}
                    onChange={() => setScope("org")}
                    disabled={submitting}
                  />
                  Org
                </label>
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input
                    type="radio"
                    name="book-scope"
                    value="platform"
                    checked={scope === "platform"}
                    onChange={() => setScope("platform")}
                    disabled={submitting}
                  />
                  Platform
                </label>
              </div>
            </div>
          )}

          {!isEdit && scope === "org" && (
            <div className="space-y-1.5">
              <Label htmlFor="book-edit-org">Organization</Label>
              {availableOrgs.length === 0 ? (
                <p className="text-sm text-muted-foreground">No organizations available.</p>
              ) : (
                <select
                  id="book-edit-org"
                  value={scopeId}
                  onChange={(e) => setScopeId(e.target.value)}
                  disabled={submitting}
                  className="flex h-9 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                >
                  <option value="">Select an organization</option>
                  {availableOrgs.map((o) => (
                    <option key={o.id} value={o.id}>
                      {o.name}
                    </option>
                  ))}
                </select>
              )}
            </div>
          )}

          {isEdit && (
            <div className="text-sm text-muted-foreground">
              <span className="font-medium">Scope:</span>{" "}
              {book?.scope === "platform" ? "Platform" : "Org"} (cannot be changed after creation)
            </div>
          )}

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
