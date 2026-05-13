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

interface UserActionsProps {
  userId: string;
  userName: string;
  userEmail: string;
  status: "active" | "suspended";
  isPlatformAdmin: boolean;
  isSelf: boolean;
}

export function UserActions({
  userId,
  userName,
  userEmail: _userEmail, // eslint-disable-line @typescript-eslint/no-unused-vars
  status,
  isPlatformAdmin,
  isSelf,
}: UserActionsProps) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [suspendOpen, setSuspendOpen] = useState(false);

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
    const message = next
      ? `Make ${userName} a platform admin?`
      : `Remove ${userName}'s platform-admin role?`;
    if (!window.confirm(message)) return;

    setLoading(true);
    try {
      const res = await fetch(`/api/admin/users/${userId}/platform-admin`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ isPlatformAdmin: next }),
      });
      if (res.ok) {
        router.refresh();
      }
    } finally {
      setLoading(false);
    }
  }

  async function handleReactivate() {
    if (!window.confirm(`Reactivate ${userName}? They will be able to sign in again.`)) return;

    setLoading(true);
    try {
      const res = await fetch(`/api/admin/users/${userId}/status`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: "active" }),
      });
      if (res.ok) {
        router.refresh();
      }
    } finally {
      setLoading(false);
    }
  }

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
            <DropdownMenuItem onClick={handleTogglePlatformAdmin}>
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
            <DropdownMenuItem onClick={handleReactivate}>
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
    </>
  );
}
