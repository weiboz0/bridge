import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";

/**
 * Plan 094 phase 3: ended sessions link from the teacher dashboard and class
 * detail only to the durable whiteboard archive.  The live workspace remains
 * unavailable after end.
 *
 * Source-text regression: assert each touched file has BOTH
 *  - a `status === "live"` conditional, AND
 *  - a live Link to its live workspace, and an ended Link to
 *    /sessions/{id}/whiteboards
 *
 * This prevents a future change from either stranding archive-eligible users
 * on a dead row or re-opening a live dashboard after session end.
 */

function source(path: string): string {
  return readFileSync(path, "utf8");
}

describe("Plan 094 phase 3 — ended sessions archive link", () => {
  describe("teacher/page.tsx (dashboard)", () => {
    const path = "src/app/(portal)/teacher/page.tsx";
    it("branches on isLive (live → workspace, ended → archive)", () => {
      const src = source(path);
      expect(src).toMatch(/const\s+isLive\s*=\s*session\.status\s*===\s*["']live["']/);
      // Live branch wraps in Link to /teacher/sessions/.
      expect(src).toMatch(/href=\{?\s*[`"]\/teacher\/sessions\/\$\{?session\.id/);
      expect(src).toMatch(/href=\{`\/sessions\/\$\{session\.id\}\/whiteboards`\}/);
    });
  });

  describe("teacher/classes/[id]/page.tsx", () => {
    const path = "src/app/(portal)/teacher/classes/[id]/page.tsx";
    it("branches on isLive for past sessions", () => {
      const src = source(path);
      expect(src).toMatch(/const\s+isLive\s*=\s*s\.status\s*===\s*["']live["']/);
      // Live branch links to /teacher/classes/{id}/session/{id}/dashboard.
      expect(src).toMatch(/\/teacher\/classes\/\$\{id\}\/session\/\$\{s\.id\}\/dashboard/);
      expect(src).toMatch(/href=\{`\/sessions\/\$\{s\.id\}\/whiteboards`\}/);
    });
    it("ended-state badge shows 'Ended' for non-live rows", () => {
      const src = source(path);
      // The non-link branch renders an Ended badge.
      expect(src).toMatch(/>\s*Ended\s*<\/span>/);
    });
  });
});
