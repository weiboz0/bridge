"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface TeacherOrg {
  orgId: string;
  orgName: string;
}

interface Props {
  teacherOrgs: TeacherOrg[];
}

export function CreateCourseForm({ teacherOrgs }: Props) {
  const router = useRouter();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const formData = new FormData(e.currentTarget);
      const res = await fetch("/api/courses", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          title: formData.get("title"),
          orgId: formData.get("orgId"),
          gradeLevel: formData.get("gradeLevel"),
        }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        setError(body?.error || `Failed (${res.status})`);
        return;
      }
      const course = await res.json();
      router.push(`/teacher/courses/${course.id}`);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex gap-3 items-end flex-wrap">
      <div>
        <Label className="text-xs">Title</Label>
        <Input name="title" placeholder="e.g., Intro to Python" required className="w-48" />
      </div>
      <div>
        <Label className="text-xs">Organization</Label>
        <select name="orgId" className="border rounded px-2 py-1.5 text-sm bg-background" required>
          {teacherOrgs.map((o) => (
            <option key={o.orgId} value={o.orgId}>{o.orgName}</option>
          ))}
        </select>
      </div>
      <div>
        <Label className="text-xs">Grade Level</Label>
        <select name="gradeLevel" className="border rounded px-2 py-1.5 text-sm bg-background" required defaultValue="9-12">
          <option value="K-5">K-5</option>
          <option value="6-8">6-8</option>
          <option value="9-12">9-12</option>
        </select>
      </div>
      <Button type="submit" size="sm" disabled={submitting}>
        {submitting ? "Creating…" : "Create"}
      </Button>
      {error && <p className="text-sm text-destructive w-full">{error}</p>}
    </form>
  );
}
