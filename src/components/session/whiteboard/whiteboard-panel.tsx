"use client";

import dynamic from "next/dynamic";
import { useCallback, useEffect, useState } from "react";
import { useSession } from "next-auth/react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useWhiteboard } from "@/lib/whiteboard/use-whiteboard";

const ExcalidrawBoard = dynamic(
  () => import("./excalidraw-board").then((module) => module.ExcalidrawBoard),
  { ssr: false, loading: () => <div className="p-4 text-sm text-muted-foreground">Loading whiteboard…</div> },
);

const VISIBILITY_LEVELS = ["private", "host", "participants", "session"] as const;
type Visibility = (typeof VISIBILITY_LEVELS)[number];

export interface WhiteboardCanvas {
  id: string;
  sessionId: string;
  ownerId: string;
  title: string;
  visibility: Visibility;
}

interface WhiteboardPanelProps {
  sessionId: string;
  /** The archive route is intentionally view-only even during a live session. */
  archive?: boolean;
}

function visibilityRank(value: Visibility): number {
  return VISIBILITY_LEVELS.indexOf(value);
}

export function WhiteboardPanel({ sessionId, archive = false }: WhiteboardPanelProps) {
  const { data: authSession } = useSession();
  const currentUserId = authSession?.user?.id ?? "";
  const [canvases, setCanvases] = useState<WhiteboardCanvas[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [title, setTitle] = useState("Untitled whiteboard");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const loadCanvases = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch(`/api/sessions/${sessionId}/canvases`);
      if (!response.ok) {
        throw new Error("Unable to load whiteboards");
      }
      const payload = await response.json() as { items?: WhiteboardCanvas[] };
      setCanvases(Array.isArray(payload.items) ? payload.items : []);
    } catch (cause) {
      setCanvases([]);
      setError(cause instanceof Error ? cause.message : "Unable to load whiteboards");
    } finally {
      setLoading(false);
    }
  }, [sessionId]);

  useEffect(() => {
    void loadCanvases();
  }, [loadCanvases]);

  const selected = canvases.find((canvas) => canvas.id === selectedId) ?? null;
  // Archive mode wins even for a live-session owner with a write-capable JWT.
  const readOnly = archive || !selected || selected.ownerId !== currentUserId;
  const whiteboard = useWhiteboard({ canvasId: selected?.id ?? null, sessionId: selected?.sessionId ?? sessionId, readOnly });

  const createCanvas = async () => {
    const normalizedTitle = title.trim();
    if (!normalizedTitle) return;
    setError(null);
    const response = await fetch(`/api/sessions/${sessionId}/canvases`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: normalizedTitle, visibility: "private" }),
    });
    if (!response.ok) {
      setError("Unable to create whiteboard");
      return;
    }
    const created = await response.json() as WhiteboardCanvas;
    setCanvases((current) => [...current, created]);
    setSelectedId(created.id);
  };

  const updateVisibility = async (canvas: WhiteboardCanvas, visibility: Visibility) => {
    if (visibilityRank(visibility) <= visibilityRank(canvas.visibility)) return;
    setError(null);
    const response = await fetch(`/api/sessions/${sessionId}/canvases/${canvas.id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ visibility }),
    });
    if (!response.ok) {
      setError("Unable to update whiteboard visibility");
      return;
    }
    const updated = await response.json() as WhiteboardCanvas;
    setCanvases((current) => current.map((item) => item.id === updated.id ? updated : item));
  };

  return (
    <section className="flex min-h-[32rem] overflow-hidden rounded-lg border" aria-label={archive ? "Whiteboard archive" : "Whiteboards"}>
      <aside className="flex w-60 shrink-0 flex-col border-r bg-muted/20">
        <div className="border-b px-3 py-3">
          <h2 className="font-semibold">{archive ? "Whiteboard archive" : "Whiteboards"}</h2>
          {archive && <p className="mt-1 text-xs text-muted-foreground">Read-only session archive</p>}
        </div>
        {!archive && (
          <div className="space-y-2 border-b p-3">
            <Input aria-label="Whiteboard title" value={title} onChange={(event) => setTitle(event.target.value)} />
            <Button className="w-full" size="sm" onClick={() => void createCanvas()}>New whiteboard</Button>
          </div>
        )}
        <div className="min-h-0 flex-1 overflow-auto p-2">
          {loading && <p className="p-2 text-sm text-muted-foreground">Loading…</p>}
          {!loading && canvases.length === 0 && (
            <p className="p-2 text-sm text-muted-foreground">No whiteboards are available.</p>
          )}
          {canvases.map((canvas) => (
            <button
              key={canvas.id}
              type="button"
              className={`w-full rounded-md px-3 py-2 text-left text-sm ${canvas.id === selectedId ? "bg-accent" : "hover:bg-muted"}`}
              onClick={() => setSelectedId(canvas.id)}
            >
              <span className="block truncate font-medium">{canvas.title}</span>
              <span className="text-xs text-muted-foreground">{canvas.visibility}</span>
            </button>
          ))}
        </div>
      </aside>
      <div className="min-w-0 flex-1">
        {error && <p className="p-3 text-sm text-destructive" role="alert">{error}</p>}
        {!selected && !loading && canvases.length > 0 && <p className="p-4 text-sm text-muted-foreground">Select a whiteboard to open it.</p>}
        {selected && (
          <div className="flex h-full flex-col">
            <div className="flex items-center justify-between border-b px-3 py-2">
              <div>
                <p className="font-medium">{selected.title}</p>
                {readOnly && <p className="text-xs text-muted-foreground">View only</p>}
              </div>
              {!archive && selected.ownerId === currentUserId && (
                <label className="text-sm">
                  Visibility
                  <select
                    className="ml-2 rounded border bg-background px-2 py-1"
                    value={selected.visibility}
                    onChange={(event) => void updateVisibility(selected, event.target.value as Visibility)}
                  >
                    {VISIBILITY_LEVELS.map((visibility) => (
                      <option key={visibility} value={visibility} disabled={visibilityRank(visibility) <= visibilityRank(selected.visibility)}>
                        {visibility}
                      </option>
                    ))}
                  </select>
                </label>
              )}
            </div>
            <div className="min-h-0 flex-1">
              <ExcalidrawBoard scene={whiteboard.scene} readOnly={readOnly} onChange={whiteboard.onChange} />
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
