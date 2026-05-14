"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { BookEditDialog } from "@/components/books/book-edit-dialog";

interface Props {
  availableOrgs: { id: string; name: string }[];
}

export function BookCreateTrigger({ availableOrgs }: Props) {
  const router = useRouter();
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button size="sm" onClick={() => setOpen(true)}>
        + New book
      </Button>
      <BookEditDialog
        open={open}
        onClose={() => setOpen(false)}
        onSaved={() => router.refresh()}
        availableOrgs={availableOrgs}
      />
    </>
  );
}
