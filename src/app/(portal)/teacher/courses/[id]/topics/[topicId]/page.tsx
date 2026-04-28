"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { isValidUUID } from "@/lib/utils";

interface TopicData {
  id: string;
  title: string;
  description: string;
}

interface LinkedUnit {
  id: string;
  title: string;
  materialType: string;
  status: string;
}

/**
 * Plan 044 phase 2: Topic editor demoted to a syllabus/focus-area
 * organizer. The LessonEditor is gone — teaching material lives in the
 * linked teaching_unit (1:1 via teaching_units.topic_id). This page
 * shows the linked Unit when present and exposes a primitive
 * "paste a Unit ID to link" input so new topics aren't stranded
 * before plan 045 ships the search-and-pick UI.
 */
export default function TopicEditorPage() {
  const params = useParams<{ id: string; topicId: string }>();
  const router = useRouter();
  const [topic, setTopic] = useState<TopicData | null>(null);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [saving, setSaving] = useState(false);

  const [linkedUnit, setLinkedUnit] = useState<LinkedUnit | null>(null);
  const [unitIdInput, setUnitIdInput] = useState("");
  const [linking, setLinking] = useState(false);
  const [linkError, setLinkError] = useState<string | null>(null);

  const loadTopic = useCallback(async () => {
    const res = await fetch(`/api/courses/${params.id}/topics/${params.topicId}`);
    if (res.ok) {
      const data = await res.json();
      setTopic({ id: data.id, title: data.title || "", description: data.description || "" });
      setTitle(data.title || "");
      setDescription(data.description || "");
    }
  }, [params.id, params.topicId]);

  const loadLinkedUnit = useCallback(async () => {
    // GET the unit attached to this topic. The Go side exposes a
    // by-topic lookup at /api/units/by-topic/{topicId}; if missing
    // (404), the topic has no linked Unit.
    const res = await fetch(`/api/units/by-topic/${params.topicId}`);
    if (res.ok) {
      const data = await res.json();
      setLinkedUnit({
        id: data.id,
        title: data.title,
        materialType: data.materialType,
        status: data.status,
      });
    } else {
      setLinkedUnit(null);
    }
  }, [params.topicId]);

  useEffect(() => {
    loadTopic();
    loadLinkedUnit();
  }, [loadTopic, loadLinkedUnit]);

  async function handleSaveMetadata() {
    setSaving(true);
    await fetch(`/api/courses/${params.id}/topics/${params.topicId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title, description }),
    });
    setSaving(false);
  }

  async function handleLinkUnit() {
    setLinkError(null);
    const trimmed = unitIdInput.trim();
    if (!isValidUUID(trimmed)) {
      setLinkError("Please paste a valid Unit ID (UUID).");
      return;
    }
    setLinking(true);
    try {
      const res = await fetch(
        `/api/courses/${params.id}/topics/${params.topicId}/link-unit`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ unitId: trimmed }),
        }
      );
      if (res.ok) {
        setUnitIdInput("");
        await loadLinkedUnit();
      } else {
        const body = await res.json().catch(() => null);
        if (res.status === 404) {
          setLinkError("Unit not found.");
        } else if (res.status === 403) {
          setLinkError("You don't have permission to link that unit.");
        } else if (res.status === 409) {
          setLinkError(body?.error ?? "This topic is already linked to a different unit.");
        } else {
          setLinkError(body?.error ?? "Couldn't link the unit. Try again.");
        }
      }
    } finally {
      setLinking(false);
    }
  }

  if (!topic) {
    return <div className="p-6"><p className="text-muted-foreground">Loading...</p></div>;
  }

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Edit Topic</h1>
        <Button variant="outline" onClick={() => router.push(`/teacher/courses/${params.id}`)}>
          Back to Course
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Topic Details</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
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
          <Button onClick={handleSaveMetadata} disabled={saving} size="sm">
            {saving ? "Saving..." : "Save Details"}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Teaching Unit</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          {linkedUnit ? (
            <div className="flex items-start justify-between rounded-md border p-3">
              <div className="space-y-1">
                <p className="font-medium">{linkedUnit.title}</p>
                <p className="text-xs text-muted-foreground">
                  {linkedUnit.materialType} · {linkedUnit.status}
                </p>
              </div>
              <Link
                href={`/teacher/units/${linkedUnit.id}/edit`}
                className="text-primary text-xs underline"
              >
                Edit Unit →
              </Link>
            </div>
          ) : (
            <div className="space-y-3">
              <p className="text-muted-foreground">
                No teaching unit linked. Paste a Unit ID to attach an existing
                unit, or create one in the{" "}
                <Link href="/teacher/units" className="underline">
                  Units library
                </Link>
                {" "}and copy its ID.
              </p>
              <div className="flex gap-2">
                <Input
                  value={unitIdInput}
                  onChange={(e) => setUnitIdInput(e.target.value)}
                  placeholder="00000000-0000-0000-0000-000000000000"
                  className="font-mono text-xs"
                />
                <Button
                  onClick={handleLinkUnit}
                  disabled={linking || !unitIdInput.trim()}
                  size="sm"
                >
                  {linking ? "Linking..." : "Link"}
                </Button>
              </div>
              {linkError && <p className="text-destructive text-xs">{linkError}</p>}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
