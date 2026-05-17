"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { BookEditDialog } from "@/components/books/book-edit-dialog";

interface Props {
  bookId: string;
  bookTitle: string;
  bookScope: "platform" | "org";
  bookScopeId: string | null;
  bookDescription: string;
  /**
   * Base path prefix for the detail-page link. Defaults to `/library`
   * (the consolidated library page — plan 089 phase 2). The two legacy
   * per-role callers (admin list page, teacher list page) will be deleted
   * in Phase 3; any explicit override they pass keeps working until then.
   */
  detailBasePath?: string;
}

export function BookActions({
  bookId,
  bookTitle,
  bookScope,
  bookScopeId,
  bookDescription,
  detailBasePath = "/library",
}: Props) {
  const router = useRouter();
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  async function handleDelete() {
    const res = await fetch(`/api/books/${bookId}`, { method: "DELETE" });
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      throw new Error(body?.error ?? `Request failed (${res.status})`);
    }
    router.refresh();
  }


  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
              <MoreHorizontal className="h-4 w-4" />
              <span className="sr-only">Actions</span>
            </Button>
          }
        />
        <DropdownMenuContent align="end">
          <DropdownMenuItem render={<Link href={`${detailBasePath}/${bookId}`} />}>
            View details
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setEditOpen(true)}>
            Edit book…
          </DropdownMenuItem>
          <DropdownMenuItem
            className="text-destructive"
            onClick={() => setDeleteOpen(true)}
          >
            Delete book…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <BookEditDialog
        book={{
          id: bookId,
          title: bookTitle,
          description: bookDescription,
          scope: bookScope,
          scopeId: bookScopeId,
        }}
        open={editOpen}
        onClose={() => setEditOpen(false)}
        onSaved={() => router.refresh()}
      />

      <ConfirmDialog
        open={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        onConfirm={handleDelete}
        title="Delete book"
        body={`Permanently delete "${bookTitle}"? Chapters assigned to this book will become unfiled. This cannot be undone.`}
        confirmLabel="Delete"
        confirmingLabel="Deleting…"
        destructive
        typeToConfirm={bookTitle}
        typeToConfirmLabel={`Type "${bookTitle}" to confirm deletion`}
      />
    </>
  );
}
