import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";

/**
 * Plan 048 phase 6: ended sessions stop linking from the teacher
 * dashboard and class detail. The model is the SessionRow component
 * in src/app/(portal)/teacher/sessions/page.tsx (lines 17-72) which
 * branches on `s.status === "live"`.
 *
 * Source-text regression: assert each touched file has BOTH
 *  - a `status === "live"` conditional, AND
 *  - a Link to /teacher/sessions/ or /teacher/classes/.../session/...
 *
 * If a future PR re-introduces an unconditional Link wrapping ended
 * sessions, the live-vs-ended structure would still satisfy these
 * assertions, so the regression is also covered by the explicit
 * end-state branches we add below (looking for the non-link wrapper).
 */

function source(path: string): string {
  return readFileSync(path, "utf8");
}

describe("Plan 048 phase 6 — ended sessions non-link", () => {
  describe("teacher/page.tsx (dashboard)", () => {
    const path = "src/app/(portal)/teacher/page.tsx";
    it("branches on isLive (live → Link, ended → div)", () => {
      const src = source(path);
      expect(src).toMatch(/const\s+isLive\s*=\s*session\.status\s*===\s*["']live["']/);
      // Live branch wraps in Link to /teacher/sessions/.
      expect(src).toMatch(/href=\{?\s*[`"]\/teacher\/sessions\/\$\{?session\.id/);
      // Ended branch is a non-Link div with the row className.
      expect(src).toMatch(/return\s*\(\s*<div\s+key=\{session\.id\}/);
    });
  });

  describe("teacher/classes/[id]/page.tsx", () => {
    const path = "src/app/(portal)/teacher/classes/[id]/page.tsx";
    it("branches on isLive for past sessions", () => {
      const src = source(path);
      expect(src).toMatch(/const\s+isLive\s*=\s*s\.status\s*===\s*["']live["']/);
      // Live branch links to /teacher/classes/{id}/session/{id}/dashboard.
      expect(src).toMatch(/\/teacher\/classes\/\$\{id\}\/session\/\$\{s\.id\}\/dashboard/);
      // Ended branch is a non-Link div keyed on s.id.
      expect(src).toMatch(/return\s*\(\s*<div\s+key=\{s\.id\}/);
    });
    it("ended-state badge shows 'Ended' for non-live rows", () => {
      const src = source(path);
      // The non-link branch renders an Ended badge.
      expect(src).toMatch(/>\s*Ended\s*<\/span>/);
    });
  });
});
