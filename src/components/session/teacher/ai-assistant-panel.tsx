"use client";

import { HelpQueuePanel } from "@/components/help-queue/help-queue-panel";
import { AiActivityFeed } from "@/components/ai/ai-activity-feed";

interface AiAssistantPanelProps {
  sessionId: string;
  aiInteractions: any[];
}

export function AiAssistantPanel({ sessionId, aiInteractions }: AiAssistantPanelProps) {
  return (
    <div className="h-full flex flex-col">
      <div className="px-3 py-2 border-b">
        <h3 className="text-sm font-medium">AI Assistant</h3>
      </div>
      <div className="flex-1 overflow-auto p-3 space-y-4">
        <div className="p-3 bg-muted/30 rounded-lg">
          <p className="text-xs text-muted-foreground">
            AI recommendations will appear here based on student activity patterns.
          </p>
        </div>
        <HelpQueuePanel sessionId={sessionId} />
        <AiActivityFeed interactions={aiInteractions} />
      </div>
    </div>
  );
}
