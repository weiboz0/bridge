"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

interface ImpersonateButtonProps {
  userId: string;
  userName: string;
}

export function ImpersonateButton({ userId, userName }: ImpersonateButtonProps) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);

  async function handleImpersonate() {
    setLoading(true);
    const res = await fetch("/api/admin/impersonate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ userId }),
    });

    if (res.ok) {
      // Redirect to root — role detection will send to the right portal
      router.push("/");
      router.refresh();
    }
    setLoading(false);
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={handleImpersonate}
      disabled={loading}
      className="text-xs"
    >
      {loading ? "..." : `Login as ${userName.split(" ")[0]}`}
    </Button>
  );
}
