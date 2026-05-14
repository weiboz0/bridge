"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { OrgEditDialog } from "@/components/admin/org-edit-dialog";

interface Props {
  org: { id: string; name: string; contactName: string; contactEmail: string };
}

export function OrgEditTrigger({ org }: Props) {
  const router = useRouter();
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button size="sm" onClick={() => setOpen(true)}>
        Edit organization
      </Button>
      <OrgEditDialog
        org={org}
        open={open}
        onClose={() => setOpen(false)}
        onSaved={() => router.refresh()}
      />
    </>
  );
}
