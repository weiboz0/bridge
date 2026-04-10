type EventCallback = (event: string, data: unknown) => void;

class SessionEventBus {
  private listeners = new Map<string, Set<EventCallback>>();

  subscribe(sessionId: string, callback: EventCallback): () => void {
    if (!this.listeners.has(sessionId)) {
      this.listeners.set(sessionId, new Set());
    }
    this.listeners.get(sessionId)!.add(callback);

    return () => {
      const set = this.listeners.get(sessionId);
      if (set) {
        set.delete(callback);
        if (set.size === 0) this.listeners.delete(sessionId);
      }
    };
  }

  emit(sessionId: string, event: string, data: unknown): void {
    const set = this.listeners.get(sessionId);
    if (set) {
      for (const cb of set) {
        cb(event, data);
      }
    }
  }
}

export const sessionEventBus = new SessionEventBus();
