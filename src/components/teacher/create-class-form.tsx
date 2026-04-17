"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface Props {
  courseId: string;
  orgId: string;
}

export function CreateClassForm({ courseId, orgId }: Props) {
  const router = useRouter();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const formData = new FormData(e.currentTarget);
      const res = await fetch("/api/classes", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          courseId,
          orgId,
          title: formData.get("title"),
          term: formData.get("term") || "",
        }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        setError(body?.error || `Failed (${res.status})`);
        return;
      }
      const cls = await res.json();
      router.push(`/teacher/classes/${cls.id}`);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <Label>Class Title</Label>
        <Input name="title" placeholder="e.g., Fall 2026 Period 3" required />
      </div>
      <div>
        <Label>Term (optional)</Label>
        <Input name="term" placeholder="e.g., Fall 2026" />
      </div>
      <Button type="submit" disabled={submitting}>
        {submitting ? "Creating…" : "Create Class"}
      </Button>
      {error && <p className="text-sm text-destructive">{error}</p>}
    </form>
  );
}
