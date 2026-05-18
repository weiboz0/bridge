"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { BookEditDialog } from "@/components/books/book-edit-dialog";

interface Props {
  // Orgs the current user may create books in. For platform admins this is all
  // orgs from /api/admin/orgs; for org members it is their own active orgs
  // only (Decision #5, plan 089). The BookEditDialog uses this list to
  // populate the org picker dropdown.
  availableOrgs: { id: string; name: string }[];
}

export function LibraryBookCreateTrigger({ availableOrgs }: Props) {
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
