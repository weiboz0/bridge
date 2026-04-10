import { describe, it, expect } from "vitest";
import type { OutputLine } from "@/components/editor/output-panel";

describe("OutputLine type", () => {
  it("accepts stdout type", () => {
    const line: OutputLine = { type: "stdout", text: "hello" };
    expect(line.type).toBe("stdout");
    expect(line.text).toBe("hello");
  });

  it("accepts stderr type", () => {
    const line: OutputLine = { type: "stderr", text: "error" };
    expect(line.type).toBe("stderr");
    expect(line.text).toBe("error");
  });
});

describe("worker message protocol", () => {
  it("init message has correct shape", () => {
    const msg = { type: "init" as const };
    expect(msg.type).toBe("init");
  });

  it("run message has correct shape", () => {
    const msg = { type: "run" as const, code: "print('hi')", id: "abc-123" };
    expect(msg.type).toBe("run");
    expect(msg.code).toBe("print('hi')");
    expect(msg.id).toBe("abc-123");
  });
});
