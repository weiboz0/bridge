import { describe, it, expect, vi } from "vitest";
import { sessionEventBus } from "@/lib/sse";

describe("SessionEventBus", () => {
  it("delivers events to subscribers", () => {
    const callback = vi.fn();
    const unsub = sessionEventBus.subscribe("session-1", callback);

    sessionEventBus.emit("session-1", "student_joined", { studentId: "abc" });

    expect(callback).toHaveBeenCalledWith("student_joined", { studentId: "abc" });
    unsub();
  });

  it("does not deliver events to other sessions", () => {
    const callback = vi.fn();
    const unsub = sessionEventBus.subscribe("session-1", callback);

    sessionEventBus.emit("session-2", "student_joined", { studentId: "abc" });

    expect(callback).not.toHaveBeenCalled();
    unsub();
  });

  it("unsubscribe stops delivery", () => {
    const callback = vi.fn();
    const unsub = sessionEventBus.subscribe("session-1", callback);

    unsub();
    sessionEventBus.emit("session-1", "student_joined", { studentId: "abc" });

    expect(callback).not.toHaveBeenCalled();
  });
});
