"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { Button } from "@/components/ui/button";

interface Props {
  orgId: string;
  status: string;
}

export function OrgActions({ orgId, status }: Props) {
  const router = useRouter();
  const [pending, setPending] = useState(false);

  async function update(next: "active" | "suspended") {
    setPending(true);
    try {
      const res = await fetch(`/api/admin/orgs/${orgId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: next }),
      });
      if (!res.ok) {
        console.error("Failed to update org", res.status);
        return;
      }
      router.refresh();
    } finally {
      setPending(false);
    }
  }

  if (status === "pending") {
    return (
      <Button size="sm" disabled={pending} onClick={() => update("active")}>
        Approve
      </Button>
    );
  }

  if (status === "active") {
    return (
      <Button size="sm" variant="destructive" disabled={pending} onClick={() => update("suspended")}>
        Suspend
      </Button>
    );
  }

  return null;
}
