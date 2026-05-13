"use client";

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
import { SuspendOrgDialog } from "./suspend-org-dialog";

interface Props {
  orgId: string;
  orgName: string;
  status: string;
}

export function OrgActions({ orgId, orgName, status }: Props) {
  const router = useRouter();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [suspendOpen, setSuspendOpen] = useState(false);

  async function patch(next: "active" | "suspended") {
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
        setError(body?.error ?? `Failed (${res.status})`);
        return;
      }
      router.refresh();
    } finally {
      setPending(false);
    }
  }

  async function handleReactivate() {
    if (!window.confirm(`Reactivate ${orgName}? Users in this org will regain access.`)) return;
    await patch("active");
  }

  if (status === "pending") {
    return (
      <div className="flex flex-col items-end gap-1">
        <Button size="sm" disabled={pending} onClick={() => patch("active")}>
          Approve
        </Button>
        {error && <span className="text-xs text-destructive">{error}</span>}
      </div>
    );
  }

  if (status === "active") {
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
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setSuspendOpen(true)}
            >
              Suspend organization…
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <SuspendOrgDialog
          orgId={orgId}
          orgName={orgName}
          open={suspendOpen}
          onClose={() => setSuspendOpen(false)}
          onSuspended={() => router.refresh()}
        />
      </>
    );
  }

  if (status === "suspended") {
    return (
      <div className="flex flex-col items-end gap-1">
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
            <DropdownMenuItem onClick={handleReactivate} disabled={pending}>
              Reactivate organization
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        {error && <span className="text-xs text-destructive">{error}</span>}
      </div>
    );
  }

  return null;
}
