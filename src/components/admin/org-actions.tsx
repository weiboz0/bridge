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
  const [error, setError] = useState<string | null>(null);

  async function update(next: "active" | "suspended") {
    setPending(true);
    setError(null);
    try {
      const res = await fetch(`/api/admin/orgs/${orgId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: next }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        setError(body?.error || `Failed (${res.status})`);
        return;
      }
      router.refresh();
    } finally {
      setPending(false);
    }
  }

  if (status !== "pending" && status !== "active") return null;

  return (
    <div className="flex flex-col items-end gap-1">
      {status === "pending" ? (
        <Button size="sm" disabled={pending} onClick={() => update("active")}>
          Approve
        </Button>
      ) : (
        <Button size="sm" variant="destructive" disabled={pending} onClick={() => update("suspended")}>
          Suspend
        </Button>
      )}
      {error && <span className="text-xs text-destructive">{error}</span>}
    </div>
  );
}
