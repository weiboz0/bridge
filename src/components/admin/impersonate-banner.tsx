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
    <div className="fixed top-2 right-2 z-50 inline-flex items-center gap-2 rounded-md border border-amber-300/70 bg-amber-50/95 px-2.5 py-1 text-[12px] font-medium text-amber-900 shadow-sm backdrop-blur">
      <span
        aria-hidden
        className="inline-block size-1.5 rounded-full bg-amber-500 animate-pulse"
      />
      <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-amber-700/80">
        impersonating
      </span>
      <span className="max-w-[140px] truncate">{impersonating.targetName}</span>
      <Button
        size="xs"
        variant="ghost"
        className="h-5 px-1.5 text-[11px] text-amber-800 hover:bg-amber-100"
        onClick={stopImpersonating}
        title="Stop impersonating"
      >
        stop
      </Button>
    </div>
  );
}
