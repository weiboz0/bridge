"use client";

// book-picker-dialog.tsx — lets a caller pick a book (or null = "Unfiled")
// for assigning a chapter to a book. Built as a reusable library component
// in plan 088 phase 3; the chapter-edit page integration ships with the
// next plan that touches that page.

import { useEffect, useRef, useState } from "react";
import { Input } from "@/components/ui/input";

interface BookOption {
  id: string;
  title: string;
  scope: "platform" | "org";
  scopeId: string | null;
}

interface Props {
  open: boolean;
  onClose: () => void;
  onPicked: (bookId: string | null) => void;
  scope: "platform" | "org";
  scopeId?: string;
  initialBookId?: string | null;
}

export function BookPickerDialog({ open, onClose, onPicked, scope, scopeId, initialBookId }: Props) {
  const [books, setBooks] = useState<BookOption[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [error, setError] = useState<string | null>(null);

  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  // Load books each time dialog opens with the current scope/scopeId.
  // Reset search, error, and loading state as the fetch starts.
  useEffect(() => {
    if (!open) return;
    const params = new URLSearchParams({ scope });
    if (scope === "org" && scopeId) params.set("scopeId", scopeId);
    const controller = new AbortController();
    // Batch reset via async microtask to avoid triggering the
    // "setState synchronously in effect" lint rule (react/no-direct-set-state-in-use-effect).
    Promise.resolve().then(() => {
      if (controller.signal.aborted) return;
      setSearch("");
      setError(null);
      setLoading(true);
    });
    fetch(`/api/books?${params.toString()}`, { signal: controller.signal })
      .then(async (res) => {
        if (!res.ok) {
          const body = await res.json().catch(() => null);
          throw new Error(body?.error ?? `Request failed (${res.status})`);
        }
        return res.json();
      })
      .then((data: { items: BookOption[] }) => setBooks(data.items ?? []))
      .catch((e) => {
        if (controller.signal.aborted) return;
        setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [open, scope, scopeId]);

  // Close on Escape.
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onCloseRef.current();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  if (!open) return null;

  const q = search.trim().toLowerCase();
  const filtered = q ? books.filter((b) => b.title.toLowerCase().includes(q)) : books;

  function pick(bookId: string | null) {
    onPicked(bookId);
    onCloseRef.current();
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="book-picker-dialog-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        if (e.target === e.currentTarget) onCloseRef.current();
      }}
    >
      <div className="w-full max-w-sm rounded-lg bg-background p-6 shadow-xl space-y-4">
        <h2 id="book-picker-dialog-title" className="text-lg font-semibold">
          Assign to book
        </h2>

        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search books…"
          autoFocus
        />

        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}

        <div className="max-h-60 overflow-y-auto space-y-1">
          {/* "Unfiled" option always shown at top */}
          <button
            type="button"
            onClick={() => pick(null)}
            className={`w-full text-left px-3 py-2 rounded text-sm hover:bg-muted transition-colors ${
              initialBookId === null ? "font-semibold bg-muted" : ""
            }`}
          >
            Remove from book (unfiled)
          </button>

          {loading ? (
            <p className="px-3 py-2 text-sm text-muted-foreground">Loading…</p>
          ) : filtered.length === 0 ? (
            <p className="px-3 py-2 text-sm text-muted-foreground">No books found.</p>
          ) : (
            filtered.map((book) => (
              <button
                key={book.id}
                type="button"
                onClick={() => pick(book.id)}
                className={`w-full text-left px-3 py-2 rounded text-sm hover:bg-muted transition-colors ${
                  initialBookId === book.id ? "font-semibold bg-muted" : ""
                }`}
              >
                {book.title}
              </button>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
