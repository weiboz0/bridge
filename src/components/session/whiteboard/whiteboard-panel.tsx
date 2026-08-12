"use client";

import dynamic from "next/dynamic";
import { useCallback, useEffect, useState } from "react";
import { useSession } from "next-auth/react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { useWhiteboard } from "@/lib/whiteboard/use-whiteboard";

/**
 * One-shot fallback written by the teacher end flow when the durable
 * archive-completeness result is unknown (`teacher-dashboard.tsx`). Reading
 * it here is consume-on-read: a failed durable settings fetch shows it once,
 * then deletes it, so a later remount never repeats a stale warning. A
 * successful durable fetch (any value) also deletes it, since the durable
 * value has superseded the fallback.
 */
function archiveFallbackWarningKey(sessionId: string): string {
  return `whiteboard-archive-fallback:${sessionId}`;
}

function consumeArchiveFallbackWarning(sessionId: string): boolean {
  if (typeof window === "undefined") return false;
  try {
    const key = archiveFallbackWarningKey(sessionId);
    const present = window.sessionStorage.getItem(key) === "1";
    if (present) window.sessionStorage.removeItem(key);
    return present;
  } catch {
    return false;
  }
}

function clearArchiveFallbackWarning(sessionId: string): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.removeItem(archiveFallbackWarningKey(sessionId));
  } catch {
    // Storage may be unavailable (private browsing); the durable value still renders.
  }
}

const ExcalidrawBoard = dynamic(
  () => import("./excalidraw-board").then((module) => module.ExcalidrawBoard),
  { ssr: false, loading: () => <div className="p-4 text-sm text-muted-foreground">Loading whiteboard…</div> },
);

const VISIBILITY_LEVELS = ["private", "host", "participants", "session"] as const;
type Visibility = (typeof VISIBILITY_LEVELS)[number];

const CANVAS_FLOORS = ["private", "host", "participants"] as const;
type CanvasFloor = (typeof CANVAS_FLOORS)[number];

interface CanvasSettings {
  canvasFloor: CanvasFloor;
  whiteboardServerArchiveComplete?: boolean;
}

interface CanvasSettingsState {
  sessionId: string;
  floor: CanvasFloor | null;
  archiveComplete?: boolean;
  error: string | null;
}

/** Strict parse of the dedicated canvas-settings response; malformed shapes fail closed. */
function parseCanvasSettings(payload: unknown): CanvasSettings | null {
  if (typeof payload !== "object" || payload === null) return null;
  const record = payload as Record<string, unknown>;
  if (Object.keys(record).some((key) => key !== "canvasFloor" && key !== "whiteboardServerArchiveComplete")) {
    return null;
  }
  const { canvasFloor, whiteboardServerArchiveComplete } = record;
  if (typeof canvasFloor !== "string" || !CANVAS_FLOORS.includes(canvasFloor as CanvasFloor)) return null;
  if (whiteboardServerArchiveComplete !== undefined && typeof whiteboardServerArchiveComplete !== "boolean") {
    return null;
  }
  return { canvasFloor: canvasFloor as CanvasFloor, whiteboardServerArchiveComplete };
}

function parseCanvasFloorPatch(payload: unknown): { canvasFloor: CanvasFloor } | null {
  if (typeof payload !== "object" || payload === null) return null;
  const record = payload as Record<string, unknown>;
  if (Object.keys(record).length !== 1 || !("canvasFloor" in record)) return null;
  if (typeof record.canvasFloor !== "string" || !CANVAS_FLOORS.includes(record.canvasFloor as CanvasFloor)) {
    return null;
  }
  return { canvasFloor: record.canvasFloor as CanvasFloor };
}

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
  /** Renders the live teacher floor control; the server still authorizes every read/write. */
  teacherControls?: boolean;
}

function visibilityRank(value: Visibility): number {
  return VISIBILITY_LEVELS.indexOf(value);
}

export function WhiteboardPanel({
  sessionId,
  archive = false,
  teacherControls = false,
}: WhiteboardPanelProps) {
  const { data: authSession } = useSession();
  const currentUserId = authSession?.user?.id ?? "";
  const [canvases, setCanvases] = useState<WhiteboardCanvas[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [title, setTitle] = useState("Untitled whiteboard");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [settings, setSettings] = useState<CanvasSettingsState>({
    sessionId: "",
    floor: null,
    error: null,
  });
  const currentSettings = settings.sessionId === sessionId
    ? settings
    : { sessionId, floor: null, archiveComplete: undefined, error: null };
  const [pendingVisibility, setPendingVisibility] = useState<{ canvas: WhiteboardCanvas; visibility: Visibility } | null>(null);

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

  // The dedicated settings route is only fetched when this surface can use it:
  // the archive needs the durable completeness status, the live teacher panel needs the floor.
  useEffect(() => {
    if (!archive && !teacherControls) return;
    let cancelled = false;
    const publish = (next: Omit<CanvasSettingsState, "sessionId">) => {
      if (!cancelled) setSettings({ sessionId, ...next });
    };
    void (async () => {
      try {
        const response = await fetch(`/api/sessions/${sessionId}/canvas-settings`);
        if (response.status === 403) {
          if (archive) clearArchiveFallbackWarning(sessionId);
          publish({ floor: null, archiveComplete: undefined, error: null });
          return;
        }
        if (!response.ok) {
          const fallback = archive && consumeArchiveFallbackWarning(sessionId);
          publish({ floor: null, archiveComplete: fallback ? false : undefined, error: "Unable to load whiteboard settings" });
          return;
        }
        const parsed = parseCanvasSettings(await response.json());
        if (!parsed) {
          const fallback = archive && consumeArchiveFallbackWarning(sessionId);
          publish({ floor: null, archiveComplete: fallback ? false : undefined, error: "Unable to load whiteboard settings" });
          return;
        }
        // A 200 durable response is authoritative regardless of value —
        // it supersedes and consumes any prior fallback warning.
        if (archive) clearArchiveFallbackWarning(sessionId);
        publish({
          floor: parsed.canvasFloor,
          archiveComplete: parsed.whiteboardServerArchiveComplete,
          error: null,
        });
      } catch (cause) {
        const fallback = archive && consumeArchiveFallbackWarning(sessionId);
        publish({
          floor: null,
          archiveComplete: fallback ? false : undefined,
          error: cause instanceof Error ? cause.message : "Unable to load whiteboard settings",
        });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [archive, teacherControls, sessionId]);

  const updateFloor = async (nextFloor: CanvasFloor) => {
    const requestSessionId = sessionId;
    const publishPatchError = () => {
      setSettings((current) => current.sessionId === requestSessionId
        ? { ...current, error: "Unable to update the canvas floor" }
        : current);
    };
    try {
      const response = await fetch(`/api/sessions/${requestSessionId}/canvas-settings`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ canvasFloor: nextFloor }),
      });
      if (!response.ok) {
        publishPatchError();
        return;
      }
      const parsed = parseCanvasFloorPatch(await response.json());
      if (!parsed) {
        publishPatchError();
        return;
      }
      setSettings((current) => ({
        ...current,
        floor: current.sessionId === requestSessionId ? parsed.canvasFloor : current.floor,
        error: current.sessionId === requestSessionId ? null : current.error,
      }));
    } catch {
      publishPatchError();
    }
  };

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

  const applyVisibilityChange = async (canvas: WhiteboardCanvas, visibility: Visibility) => {
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

  // Visibility is loosen-only (Decision 7): any accepted raise is
  // irreversible from this UI, so every raise is confirmed before it fires.
  const requestVisibilityChange = (canvas: WhiteboardCanvas, visibility: Visibility) => {
    if (visibilityRank(visibility) <= visibilityRank(canvas.visibility)) return;
    setPendingVisibility({ canvas, visibility });
  };

  return (
    <section className="flex min-h-[32rem] overflow-hidden rounded-lg border" aria-label={archive ? "Whiteboard archive" : "Whiteboards"}>
      <aside className="flex w-60 shrink-0 flex-col border-r bg-muted/20">
        <div className="border-b px-3 py-3">
          <h2 className="font-semibold">{archive ? "Whiteboard archive" : "Whiteboards"}</h2>
          {archive && <p className="mt-1 text-xs text-muted-foreground">Read-only session archive</p>}
          {archive && currentSettings.archiveComplete === true && (
            <p className="mt-1 text-xs text-muted-foreground">Whiteboard archive confirmed</p>
          )}
          {archive && currentSettings.archiveComplete === false && (
            <p className="mt-1 text-xs text-destructive">Latest whiteboard changes may not have been archived</p>
          )}
          {currentSettings.error && (
            <p className="mt-1 text-xs text-destructive" role="alert">{currentSettings.error}</p>
          )}
          {teacherControls && !archive && currentSettings.floor && (
            <label className="mt-2 block text-xs">
              Canvas floor
              <select
                aria-label="Canvas floor"
                className="ml-2 rounded border bg-background px-2 py-1 text-sm"
                value={currentSettings.floor}
                onChange={(event) => void updateFloor(event.target.value as CanvasFloor)}
              >
                {CANVAS_FLOORS.map((level) => (
                  <option key={level} value={level}>{level}</option>
                ))}
              </select>
            </label>
          )}
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
                    onChange={(event) => requestVisibilityChange(selected, event.target.value as Visibility)}
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
      {pendingVisibility && (
        <ConfirmDialog
          open
          onClose={() => setPendingVisibility(null)}
          onConfirm={() => applyVisibilityChange(pendingVisibility.canvas, pendingVisibility.visibility)}
          title="Raise whiteboard visibility?"
          body={
            <>
              Changing “{pendingVisibility.canvas.title}” from{" "}
              <strong>{pendingVisibility.canvas.visibility}</strong> to{" "}
              <strong>{pendingVisibility.visibility}</strong> cannot be undone from this screen —
              visibility can only be raised, never lowered.
            </>
          }
          confirmLabel="Raise visibility"
          confirmingLabel="Updating…"
        />
      )}
    </section>
  );
}
