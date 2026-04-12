"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

export function ImpersonateBanner() {
  const router = useRouter();
  const [impersonating, setImpersonating] = useState<{ targetName: string } | null>(null);

  useEffect(() => {
    // Check for impersonation cookie (read via a lightweight API or cookie)
    // For simplicity, we'll check by trying to read a data attribute
    async function check() {
      const res = await fetch("/api/admin/impersonate/status");
      if (res.ok) {
        const data = await res.json();
        if (data.impersonating) {
          setImpersonating(data.impersonating);
        }
      }
    }
    check();
  }, []);

  if (!impersonating) return null;

  async function stopImpersonating() {
    await fetch("/api/admin/impersonate", { method: "DELETE" });
    router.push("/admin");
    router.refresh();
  }

  return (
    <div className="fixed top-0 left-0 right-0 z-50 bg-yellow-500 text-yellow-950 text-center py-1 px-4 text-sm font-medium flex items-center justify-center gap-3">
      <span>Impersonating: {impersonating.targetName}</span>
      <Button
        size="sm"
        variant="outline"
        className="h-6 text-xs bg-yellow-400 border-yellow-600 hover:bg-yellow-300"
        onClick={stopImpersonating}
      >
        Stop
      </Button>
    </div>
  );
}
