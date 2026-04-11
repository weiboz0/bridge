"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import { LessonEditor } from "@/components/lesson/lesson-editor";
import { LessonRenderer } from "@/components/lesson/lesson-renderer";
import { parseLessonContent, type LessonContent } from "@/lib/lesson-content";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";

interface TopicData {
  id: string;
  title: string;
  description: string;
  starterCode: string | null;
  lessonContent: unknown;
}

export default function TopicEditorPage() {
  const params = useParams<{ id: string; topicId: string }>();
  const router = useRouter();
  const [topic, setTopic] = useState<TopicData | null>(null);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [starterCode, setStarterCode] = useState("");
  const [saving, setSaving] = useState(false);
  const [lessonContent, setLessonContent] = useState<LessonContent>({ blocks: [] });

  useEffect(() => {
    async function loadTopic() {
      const res = await fetch(`/api/courses/${params.id}/topics/${params.topicId}`);
      if (res.ok) {
        const data = await res.json();
        setTopic(data);
        setTitle(data.title || "");
        setDescription(data.description || "");
        setStarterCode(data.starterCode || "");
        setLessonContent(parseLessonContent(data.lessonContent));
      }
    }
    loadTopic();
  }, [params.id, params.topicId]);

  async function handleSaveMetadata() {
    setSaving(true);
    await fetch(`/api/courses/${params.id}/topics/${params.topicId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title, description, starterCode }),
    });
    setSaving(false);
  }

  async function handleSaveLessonContent(content: LessonContent) {
    setSaving(true);
    await fetch(`/api/courses/${params.id}/topics/${params.topicId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ lessonContent: content }),
    });
    setLessonContent(content);
    setSaving(false);
  }

  if (!topic) {
    return <div className="p-6"><p className="text-muted-foreground">Loading...</p></div>;
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Edit Topic</h1>
        <Button variant="outline" onClick={() => router.push(`/teacher/courses/${params.id}`)}>
          Back to Course
        </Button>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Topic Details</h2>
          <div className="space-y-3">
            <div>
              <Label>Title</Label>
              <Input value={title} onChange={(e) => setTitle(e.target.value)} />
            </div>
            <div>
              <Label>Description</Label>
              <textarea
                className="w-full border rounded p-2 text-sm bg-background min-h-[60px]"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </div>
            <div>
              <Label>Starter Code</Label>
              <textarea
                className="w-full border rounded p-2 text-sm font-mono bg-zinc-950 text-zinc-100 min-h-[80px]"
                value={starterCode}
                onChange={(e) => setStarterCode(e.target.value)}
                placeholder="# Code students start with..."
              />
            </div>
            <Button onClick={handleSaveMetadata} disabled={saving} size="sm">
              {saving ? "Saving..." : "Save Details"}
            </Button>
          </div>
        </div>

        <div>
          <LessonEditor
            initialContent={lessonContent}
            onSave={handleSaveLessonContent}
            saving={saving}
          />
        </div>
      </div>
    </div>
  );
}
