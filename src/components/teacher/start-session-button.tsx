"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

interface StartSessionButtonProps {
  classId: string;
}

export function StartSessionButton({ classId }: StartSessionButtonProps) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);

  async function handleStart() {
    setLoading(true);
    const res = await fetch("/api/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ classId }),
    });

    if (res.ok) {
      const session = await res.json();
      router.push(`/teacher/classes/${classId}/session/${session.id}/dashboard`);
    } else {
      const data = await res.json().catch(() => null);
      alert(data?.error || "Failed to start session");
      setLoading(false);
    }
  }

  return (
    <Button onClick={handleStart} disabled={loading}>
      {loading ? "Starting..." : "Start Live Session"}
    </Button>
  );
}
