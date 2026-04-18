"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

interface Props {
  courseId: string;
  topicId: string;
}

export function DeleteTopicButton({ courseId, topicId }: Props) {
  const router = useRouter();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleDelete() {
    if (!confirm("Delete this topic?")) return;
    setSubmitting(true);
    setError(null);
    try {
      const res = await fetch(`/api/courses/${courseId}/topics/${topicId}`, {
        method: "DELETE",
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        setError(body?.error || `Failed (${res.status})`);
        return;
      }
      router.refresh();
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col items-end">
      <Button
        variant="ghost"
        size="sm"
        className="text-destructive"
        disabled={submitting}
        onClick={handleDelete}
      >
        ×
      </Button>
      {error && <span className="text-xs text-destructive">{error}</span>}
    </div>
  );
}
