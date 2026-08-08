"use client";

import { WhiteboardPanel } from "./whiteboard-panel";

export function WhiteboardArchive({ sessionId }: { sessionId: string }) {
  return <WhiteboardPanel sessionId={sessionId} archive />;
}
