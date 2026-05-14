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
import { OrgEditDialog } from "@/components/admin/org-edit-dialog";
import { SuspendOrgDialog } from "@/components/admin/suspend-org-dialog";

interface Props {
  orgId: string;
  orgName: string;
  status: string;
  contactName: string;
  contactEmail: string;
}

export function OrgActions({ orgId, orgName, status, contactName, contactEmail }: Props) {
  const router = useRouter();
  const [approveOpen, setApproveOpen] = useState(false);
  const [reactivateOpen, setReactivateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [suspendOpen, setSuspendOpen] = useState(false);

  async function patchStatus(next: "active" | "suspended") {
    const res = await fetch(`/api/admin/orgs/${orgId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: next }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      throw new Error(body?.error ?? `Request failed (${res.status})`);
    }
    router.refresh();
  }

  if (status === "pending") {
    return (
      <>
        <div className="flex items-center gap-2">
          <Button size="sm" onClick={() => setApproveOpen(true)}>
            Approve
          </Button>
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
              <DropdownMenuItem render={<Link href={`/admin/orgs/${orgId}`} />}>
                View details
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        <ConfirmDialog
          open={approveOpen}
          onClose={() => setApproveOpen(false)}
          onConfirm={() => patchStatus("active")}
          title="Approve organization"
          body={`Activate ${orgName}? Members will gain access.`}
          confirmLabel="Approve"
          destructive={false}
        />
      </>
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
            <DropdownMenuItem render={<Link href={`/admin/orgs/${orgId}`} />}>
              View details
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setEditOpen(true)}>
              Edit organization…
            </DropdownMenuItem>
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setSuspendOpen(true)}
            >
              Suspend organization…
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <OrgEditDialog
          org={{ id: orgId, name: orgName, contactName, contactEmail }}
          open={editOpen}
          onClose={() => setEditOpen(false)}
          onSaved={() => router.refresh()}
        />

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
            <DropdownMenuItem render={<Link href={`/admin/orgs/${orgId}`} />}>
              View details
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setReactivateOpen(true)}>
              Reactivate organization
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <ConfirmDialog
          open={reactivateOpen}
          onClose={() => setReactivateOpen(false)}
          onConfirm={() => patchStatus("active")}
          title="Reactivate organization"
          body={`Reactivate ${orgName}? Members will regain access.`}
          confirmLabel="Reactivate"
          destructive={false}
        />
      </>
    );
  }

  return null;
}
