"use client";

import { useCallback, useEffect, useRef, useState, type ComponentProps, type DragEvent } from "react";
import { Excalidraw } from "@excalidraw/excalidraw";
import "@excalidraw/excalidraw/index.css";
import type { ExcalidrawScene } from "@/lib/whiteboard/excalidraw-yjs";

interface ExcalidrawBoardProps {
  scene: ExcalidrawScene | null;
  readOnly: boolean;
  onChange: (scene: ExcalidrawScene) => void;
}

type ExcalidrawProps = ComponentProps<typeof Excalidraw>;
type ExcalidrawApi = Parameters<NonNullable<ExcalidrawProps["excalidrawAPI"]>>[0];
type ExcalidrawUIOptions = NonNullable<ExcalidrawProps["UIOptions"]>;

// Whiteboards don't persist Excalidraw's binary `files` map, so an inserted
// image would render locally and then vanish for peers and in the archive
// (spec 013). Hiding the toolbar's image tool keeps that path from ever
// starting. Module-level so its identity is stable across renders —
// Excalidraw re-initialises its UI whenever it sees a new UIOptions object.
const EXCALIDRAW_UI_OPTIONS: ExcalidrawUIOptions = { tools: { image: false } };

// How long the rejection notice stays visible before it auto-dismisses.
const IMAGE_REJECTED_NOTICE_MS = 5000;

const IMAGE_REJECTED_MESSAGE = "Images and files can't be added to a whiteboard.";

/** True for a native OS drag carrying files (images, other uploads). */
function carriesFiles(event: DragEvent<HTMLDivElement>): boolean {
  return Array.from(event.dataTransfer?.types ?? []).includes("Files");
}

/** The dynamically loaded visual surface; synchronization stays in useWhiteboard. */
export function ExcalidrawBoard({ scene, readOnly, onChange }: ExcalidrawBoardProps) {
  const apiRef = useRef<ExcalidrawApi | null>(null);
  const [imageRejected, setImageRejected] = useState(false);
  const dismissTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!scene || !apiRef.current) return;
    apiRef.current.updateScene({
      elements: scene.elements as never,
      appState: scene.appState as never,
    });
  }, [scene]);

  // Clear any pending dismiss timer on unmount so it never fires against an
  // unmounted component.
  useEffect(() => {
    return () => {
      if (dismissTimerRef.current) clearTimeout(dismissTimerRef.current);
    };
  }, []);

  // Shows the rejection notice and (re)starts its auto-dismiss timer. A
  // second rejection while the notice is already visible restarts the
  // 5-second window rather than layering timers.
  const showImageRejectedNotice = useCallback(() => {
    if (dismissTimerRef.current) clearTimeout(dismissTimerRef.current);
    setImageRejected(true);
    dismissTimerRef.current = setTimeout(() => {
      setImageRejected(false);
      dismissTimerRef.current = null;
    }, IMAGE_REJECTED_NOTICE_MS);
  }, []);

  // Blocks native file drag/drop before it reaches Excalidraw's own drop
  // handler. `stopPropagation` during the capture phase (this wrapper is an
  // ancestor of the Excalidraw root) prevents the event from ever reaching
  // Excalidraw's bubble-phase listener, so no image gets added to the scene.
  const rejectFileDrag = useCallback((event: DragEvent<HTMLDivElement>) => {
    if (!carriesFiles(event)) return;
    event.preventDefault();
    event.stopPropagation();
    showImageRejectedNotice();
  }, [showImageRejectedNotice]);

  return (
    <div
      className="relative h-full min-h-[28rem]"
      data-testid="excalidraw-board"
      onDragOverCapture={rejectFileDrag}
      onDropCapture={rejectFileDrag}
    >
      {imageRejected && (
        <p
          role="status"
          data-testid="whiteboard-image-rejected"
          className="pointer-events-none absolute top-2 left-1/2 z-10 -translate-x-1/2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-1.5 text-xs text-destructive shadow-sm dark:bg-destructive/20"
        >
          {IMAGE_REJECTED_MESSAGE}
        </p>
      )}
      <Excalidraw
        excalidrawAPI={(api) => { apiRef.current = api; }}
        viewModeEnabled={readOnly}
        UIOptions={EXCALIDRAW_UI_OPTIONS}
        onPaste={(data) => {
          // Excalidraw's ClipboardData carries `files` for pasted images.
          // Rejecting here (return false) skips its default paste handling
          // entirely — no image is ever added to the scene.
          if (data.files && Object.keys(data.files).length > 0) {
            showImageRejectedNotice();
            return false;
          }
          return true;
        }}
        onChange={(elements, appState) => {
          onChange({
            elements,
            appState: appState as unknown as Record<string, unknown>,
          });
        }}
      />
    </div>
  );
}
