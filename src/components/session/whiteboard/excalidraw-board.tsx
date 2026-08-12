"use client";

import { useEffect, useRef, type ComponentProps, type DragEvent } from "react";
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

/** True for a native OS drag carrying files (images, other uploads). */
function carriesFiles(event: DragEvent<HTMLDivElement>): boolean {
  return Array.from(event.dataTransfer?.types ?? []).includes("Files");
}

/**
 * Blocks native file drag/drop before it reaches Excalidraw's own drop
 * handler. `stopPropagation` during the capture phase (this wrapper is an
 * ancestor of the Excalidraw root) prevents the event from ever reaching
 * Excalidraw's bubble-phase listener, so no image gets added to the scene.
 */
function rejectFileDrag(event: DragEvent<HTMLDivElement>) {
  if (!carriesFiles(event)) return;
  event.preventDefault();
  event.stopPropagation();
}

/** The dynamically loaded visual surface; synchronization stays in useWhiteboard. */
export function ExcalidrawBoard({ scene, readOnly, onChange }: ExcalidrawBoardProps) {
  const apiRef = useRef<ExcalidrawApi | null>(null);

  useEffect(() => {
    if (!scene || !apiRef.current) return;
    apiRef.current.updateScene({
      elements: scene.elements as never,
      appState: scene.appState as never,
    });
  }, [scene]);

  return (
    <div
      className="h-full min-h-[28rem]"
      data-testid="excalidraw-board"
      onDragOverCapture={rejectFileDrag}
      onDropCapture={rejectFileDrag}
    >
      <Excalidraw
        excalidrawAPI={(api) => { apiRef.current = api; }}
        viewModeEnabled={readOnly}
        onPaste={(data) => {
          // Excalidraw's ClipboardData carries `files` for pasted images.
          // Rejecting here (return false) skips its default paste handling
          // entirely — no image is ever added to the scene.
          if (data.files && Object.keys(data.files).length > 0) return false;
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
