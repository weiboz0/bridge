"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { UnitPickerDialog } from "@/components/teacher/unit-picker-dialog";
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

// Plan 048 phase 4: explicit fetch states so the page never silently
// hangs on Loading… for non-2xx responses (review 006/007 P2). The
// previous code only set `topic` when `res.ok`, leaving the page stuck
// on "Loading…" for 400 / 403 / 404 / network errors.
type LoadState =
  | { kind: "loading" }
  | { kind: "ready" }
  | { kind: "invalid-uuid" }
  | { kind: "not-found" }
  | { kind: "forbidden" }
  | { kind: "error"; status: number };

/**
 * Plan 044 phase 2: Topic editor demoted to a syllabus/focus-area
 * organizer; teaching material lives in the linked teaching_unit.
 *
 * Plan 045: paste-Unit-ID input replaced with a real picker dialog
 * (UnitPickerDialog), backed by the picker-mode SearchUnits endpoint.
 * Adds Replace and Unlink affordances when a Unit is currently linked.
 */
export default function TopicEditorPage() {
  const params = useParams<{ id: string; topicId: string }>();
  const router = useRouter();
  const [topic, setTopic] = useState<TopicData | null>(null);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [saving, setSaving] = useState(false);
  const [loadState, setLoadState] = useState<LoadState>({ kind: "loading" });

  const [linkedUnit, setLinkedUnit] = useState<LinkedUnit | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [linkError, setLinkError] = useState<string | null>(null);
  const [unlinking, setUnlinking] = useState(false);

  // Plan 048 phase 4: validate UUIDs before fetch so /teacher/courses/
  // not-a-uuid/topics/also-not falls through to a clean error card
  // instead of hammering the API with a malformed request.
  const paramsValid = isValidUUID(params.id) && isValidUUID(params.topicId);

  const loadTopic = useCallback(async () => {
    if (!paramsValid) {
      setLoadState({ kind: "invalid-uuid" });
      return;
    }
    try {
      const res = await fetch(`/api/courses/${params.id}/topics/${params.topicId}`);
      if (res.ok) {
        const data = await res.json();
        setTopic({ id: data.id, title: data.title || "", description: data.description || "" });
        setTitle(data.title || "");
        setDescription(data.description || "");
        setLoadState({ kind: "ready" });
      } else if (res.status === 404) {
        setLoadState({ kind: "not-found" });
      } else if (res.status === 403) {
        setLoadState({ kind: "forbidden" });
      } else {
        setLoadState({ kind: "error", status: res.status });
      }
    } catch {
      setLoadState({ kind: "error", status: 0 }); // network failure
    }
  }, [params.id, params.topicId, paramsValid]);

  const loadLinkedUnit = useCallback(async () => {
    if (!isValidUUID(params.topicId)) {
      setLinkedUnit(null);
      return;
    }
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

  async function handlePicked(unitId: string) {
    setLinkError(null);
    const res = await fetch(
      `/api/courses/${params.id}/topics/${params.topicId}/link-unit`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ unitId }),
      }
    );
    if (res.ok) {
      await loadLinkedUnit();
    } else {
      const body = await res.json().catch(() => null);
      if (res.status === 404) setLinkError("Unit not found.");
      else if (res.status === 403)
        setLinkError("You don't have permission to link that unit.");
      else if (res.status === 409)
        setLinkError(body?.error ?? "This topic is already linked to a different unit.");
      else setLinkError(body?.error ?? "Couldn't link the unit. Try again.");
    }
  }

  async function handleUnlink() {
    setLinkError(null);
    setUnlinking(true);
    try {
      const res = await fetch(
        `/api/courses/${params.id}/topics/${params.topicId}/link-unit`,
        { method: "DELETE" }
      );
      if (res.ok) {
        await loadLinkedUnit();
      } else {
        const body = await res.json().catch(() => null);
        setLinkError(body?.error ?? "Couldn't detach the unit. Try again.");
      }
    } finally {
      setUnlinking(false);
    }
  }

  // Plan 048 phase 4: explicit failure cards instead of an indefinite
  // "Loading…" hang. Phase 5 will rename "Topic" → "Focus Area" across
  // the rest of the app; this page leads.
  if (loadState.kind === "loading") {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">Loading…</p>
      </div>
    );
  }
  if (loadState.kind !== "ready" || !topic) {
    const backHref = isValidUUID(params.id) ? `/teacher/courses/${params.id}` : "/teacher";
    let message: string;
    let title: string;
    switch (loadState.kind) {
      case "invalid-uuid":
        title = "Invalid focus-area URL";
        message = "The course or focus-area ID in this URL isn't valid.";
        break;
      case "not-found":
        title = "Focus area not found";
        message = "This focus area doesn't exist, or it was removed.";
        break;
      case "forbidden":
        title = "Access denied";
        message = "You don't have permission to edit this focus area.";
        break;
      default:
        title = "Couldn't load focus area";
        message =
          loadState.kind === "error" && loadState.status === 0
            ? "Network error. Check your connection and try again."
            : `Server returned an error${
                loadState.kind === "error" ? ` (${loadState.status})` : ""
              }. Try again in a moment.`;
    }
    return (
      <div className="p-6 max-w-2xl">
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">{title}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm text-muted-foreground">{message}</p>
            <Button variant="outline" size="sm" onClick={() => router.push(backHref)}>
              Back to Course
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Edit Focus Area</h1>
        <Button variant="outline" onClick={() => router.push(`/teacher/courses/${params.id}`)}>
          Back to Course
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Focus Area Details</CardTitle>
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
            <div className="space-y-3">
              <div className="flex items-start justify-between rounded-md border p-3">
                <div className="space-y-1">
                  <p className="font-medium">{linkedUnit.title}</p>
                  <p className="text-xs text-muted-foreground">
                    {linkedUnit.materialType} · {linkedUnit.status}
                  </p>
                </div>
                <Link
                  href={`/teacher/units/${linkedUnit.id}/edit`}
                  className="text-primary text-xs underline self-center"
                >
                  Edit Unit →
                </Link>
              </div>
              <div className="flex gap-2">
                <Button size="sm" variant="outline" onClick={() => setPickerOpen(true)}>
                  Replace…
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={handleUnlink}
                  disabled={unlinking}
                  className="text-destructive"
                >
                  {unlinking ? "Detaching…" : "Unlink"}
                </Button>
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              <p className="text-muted-foreground">
                No teaching unit linked. Search and pick one from the library, or
                create one in the{" "}
                <Link href="/teacher/units" className="underline">
                  Units library
                </Link>
                .
              </p>
              <Button size="sm" onClick={() => setPickerOpen(true)}>
                Pick a unit…
              </Button>
            </div>
          )}
          {linkError && <p className="text-destructive text-xs">{linkError}</p>}
        </CardContent>
      </Card>

      <UnitPickerDialog
        open={pickerOpen}
        onClose={() => setPickerOpen(false)}
        courseId={params.id}
        currentTopicId={params.topicId}
        onPicked={handlePicked}
      />
    </div>
  );
}
