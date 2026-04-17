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

  async function handleDelete() {
    setSubmitting(true);
    try {
      const res = await fetch(`/api/courses/${courseId}/topics/${topicId}`, {
        method: "DELETE",
      });
      if (!res.ok) {
        console.error("Failed to delete topic", res.status);
        return;
      }
      router.refresh();
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      className="text-destructive"
      disabled={submitting}
      onClick={handleDelete}
    >
      ×
    </Button>
  );
}
