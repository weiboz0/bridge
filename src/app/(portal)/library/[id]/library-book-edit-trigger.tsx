"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { BookEditDialog } from "@/components/books/book-edit-dialog";

interface Props {
  book: {
    id: string;
    title: string;
    description: string;
    scope: "platform" | "org";
    scopeId: string | null;
  };
}

export function LibraryBookEditTrigger({ book }: Props) {
  const router = useRouter();
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button size="sm" onClick={() => setOpen(true)}>
        Edit book
      </Button>
      <BookEditDialog
        book={book}
        open={open}
        onClose={() => setOpen(false)}
        onSaved={() => router.refresh()}
      />
    </>
  );
}
