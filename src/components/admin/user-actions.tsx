"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import { SuspendUserDialog } from "@/components/admin/suspend-user-dialog";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";

interface UserActionsProps {
  userId: string;
  userName: string;
  status: "active" | "suspended";
  isPlatformAdmin: boolean;
  isSelf: boolean;
}

export function UserActions({
  userId,
  userName,
  status,
  isPlatformAdmin,
  isSelf,
}: UserActionsProps) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [suspendOpen, setSuspendOpen] = useState(false);
  const [reactivateOpen, setReactivateOpen] = useState(false);
  const [toggleAdminOpen, setToggleAdminOpen] = useState(false);

  async function handleImpersonate() {
    setLoading(true);
    const res = await fetch("/api/admin/impersonate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ userId }),
    });

    if (res.ok) {
      router.push("/");
      router.refresh();
    }
    setLoading(false);
  }

  async function handleTogglePlatformAdmin() {
    const next = !isPlatformAdmin;
    const res = await fetch(`/api/admin/users/${userId}/platform-admin`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ isPlatformAdmin: next }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      throw new Error(body?.error ?? `Request failed (${res.status})`);
    }
    router.refresh();
  }

  async function handleReactivate() {
    const res = await fetch(`/api/admin/users/${userId}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: "active" }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      throw new Error(body?.error ?? `Request failed (${res.status})`);
    }
    router.refresh();
  }

  const toggleAdminTitle = isPlatformAdmin ? "Remove platform-admin role" : "Grant platform-admin role";
  const toggleAdminBody = isPlatformAdmin
    ? `Remove ${userName}'s platform-admin role? They will lose access to /admin.`
    : `Make ${userName} a platform admin? They will have full access to /admin.`;
  const toggleAdminConfirmLabel = isPlatformAdmin ? "Remove" : "Grant";
  const toggleAdminDestructive = isPlatformAdmin;

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button variant="ghost" size="sm" className="h-8 w-8 p-0" disabled={loading}>
              <MoreHorizontal className="h-4 w-4" />
              <span className="sr-only">Actions</span>
            </Button>
          }
        />
        <DropdownMenuContent align="end">
          <DropdownMenuItem render={<Link href={`/admin/users/${userId}`} />}>
            View details
          </DropdownMenuItem>

          {!isSelf && status === "active" && (
            <DropdownMenuItem onClick={handleImpersonate}>
              Login as {userName.split(" ")[0]}
            </DropdownMenuItem>
          )}

          {!isSelf && (
            <DropdownMenuItem onClick={() => setToggleAdminOpen(true)}>
              {isPlatformAdmin ? `Remove ${userName.split(" ")[0]}'s admin role` : `Make ${userName.split(" ")[0]} a platform admin`}
            </DropdownMenuItem>
          )}

          {!isSelf && status === "active" && (
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setSuspendOpen(true)}
            >
              Suspend account…
            </DropdownMenuItem>
          )}

          {!isSelf && status === "suspended" && (
            <DropdownMenuItem onClick={() => setReactivateOpen(true)}>
              Reactivate account
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      {!isSelf && (
        <SuspendUserDialog
          userId={userId}
          userName={userName}
          open={suspendOpen}
          onClose={() => setSuspendOpen(false)}
          onSuspended={() => {
            setSuspendOpen(false);
            router.refresh();
          }}
        />
      )}

      {!isSelf && (
        <ConfirmDialog
          open={reactivateOpen}
          onClose={() => setReactivateOpen(false)}
          onConfirm={handleReactivate}
          title="Reactivate account"
          body={`${userName} will be able to sign in again.`}
          confirmLabel="Reactivate"
          destructive={false}
        />
      )}

      {!isSelf && (
        <ConfirmDialog
          open={toggleAdminOpen}
          onClose={() => setToggleAdminOpen(false)}
          onConfirm={handleTogglePlatformAdmin}
          title={toggleAdminTitle}
          body={toggleAdminBody}
          confirmLabel={toggleAdminConfirmLabel}
          destructive={toggleAdminDestructive}
          typeToConfirm={userName}
        />
      )}
    </>
  );
}
