"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { InviteMemberModal } from "./invite-member-modal";

// Plan 069 phase 1 — client wrapper that owns the modal-open state for
// the invite-member flow. Used by /org/teachers and /org/students pages.

interface Props {
  orgId: string;
  role: "teacher" | "student";
}

export function InviteMemberButton({ orgId, role }: Props) {
  const router = useRouter();
  const [open, setOpen] = useState(false);

  // DeepSeek post-impl NIT: button label was lowercase ("teacher" /
  // "student") while the modal title is capitalized; align both.
  const label = role === "teacher" ? "Teacher" : "Student";

  return (
    <>
      <Button onClick={() => setOpen(true)}>+ Invite {label}</Button>
      {open && (
        <InviteMemberModal
          orgId={orgId}
          role={role}
          onClose={() => setOpen(false)}
          onSuccess={() => {
            setOpen(false);
            router.refresh();
          }}
        />
      )}
    </>
  );
}
