import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";

/**
 * Plan 048 phase 5: Topic → Focus Area rename in user-visible copy.
 *
 * Surfaces:
 *  - teacher course detail (h2 "Topics" → "Focus Areas", empty-state copy)
 *  - student class detail (h2 "Topics" → "Focus Areas", empty-state, "No material yet for this topic")
 *  - student session view ("No material yet for this topic")
 *  - parent live-session viewer (same)
 *  - teacher dashboard ("No topics linked to this session yet")
 *  - add-topic-form ("Add Topic" button, "New topic title" placeholder)
 *  - unit-picker-dialog ("Linked to another topic" tooltip)
 *  - topic editor heading + card title (covered by phase 4)
 *
 * Tests assert SPECIFIC rendered JSX strings — no blanket /Topic/
 * regex (per Codex review pass 2 IMPORTANT). Code identifiers,
 * routes, and type names that legitimately use "Topic" are not
 * affected by these regressions.
 */

function source(path: string): string {
  return readFileSync(path, "utf8");
}

describe("Plan 048 phase 5 — Topic → Focus Area rename", () => {
  describe("teacher courses/[id]/page.tsx", () => {
    const path = "src/app/(portal)/teacher/courses/[id]/page.tsx";
    it("h2 says 'Focus Areas'", () => {
      expect(source(path)).toMatch(/<h2[^>]*>Focus Areas \(/);
      expect(source(path)).not.toMatch(/<h2[^>]*>Topics \(/);
    });
    it("empty-state references focus areas", () => {
      expect(source(path)).toMatch(/No focus areas yet\. Add your first focus area above\./);
      expect(source(path)).not.toMatch(/No topics yet\. Add your first topic above\./);
    });
  });

  describe("student classes/[id]/page.tsx", () => {
    const path = "src/app/(portal)/student/classes/[id]/page.tsx";
    it("h2 says 'Focus Areas'", () => {
      expect(source(path)).toMatch(/<h2[^>]*>Focus Areas<\/h2>/);
      expect(source(path)).not.toMatch(/<h2[^>]*>Topics<\/h2>/);
    });
    it("empty-state copy", () => {
      expect(source(path)).toMatch(/No focus areas yet\./);
      expect(source(path)).not.toMatch(/<p>No topics yet\./);
    });
    it("'No material yet for this focus area'", () => {
      expect(source(path)).toMatch(/No material yet for this focus area\./);
      expect(source(path)).not.toMatch(/No material yet for this topic\./);
    });
  });

  describe("student-session.tsx", () => {
    const path = "src/components/session/student/student-session.tsx";
    it("'No material yet for this focus area'", () => {
      expect(source(path)).toMatch(/No material yet for this focus area\./);
      expect(source(path)).not.toMatch(/No material yet for this topic\./);
    });
  });

  describe("parent live-session-viewer.tsx", () => {
    const path = "src/components/parent/live-session-viewer.tsx";
    it("'No material yet for this focus area'", () => {
      expect(source(path)).toMatch(/No material yet for this focus area\./);
      expect(source(path)).not.toMatch(/No material yet for this topic\./);
    });
  });

  describe("teacher-dashboard.tsx", () => {
    const path = "src/components/session/teacher/teacher-dashboard.tsx";
    it("'No focus areas linked to this session yet'", () => {
      expect(source(path)).toMatch(/No focus areas linked to this session yet\./);
      expect(source(path)).not.toMatch(/No topics linked to this session yet\./);
    });
  });

  describe("add-topic-form.tsx", () => {
    const path = "src/components/teacher/add-topic-form.tsx";
    it("button label says 'Add Focus Area'", () => {
      expect(source(path)).toMatch(/"Add Focus Area"/);
      expect(source(path)).not.toMatch(/"Add Topic"/);
    });
    it("placeholder mentions focus area", () => {
      expect(source(path)).toMatch(/placeholder="New focus area title"/);
      expect(source(path)).not.toMatch(/placeholder="New topic title"/);
    });
  });

  describe("unit-picker-dialog.tsx", () => {
    const path = "src/components/teacher/unit-picker-dialog.tsx";
    it("tooltip references focus area not topic", () => {
      expect(source(path)).toMatch(/"Linked to another focus area"/);
      expect(source(path)).not.toMatch(/"Linked to another topic"/);
    });
  });

  describe("teacher topics/[topicId]/page.tsx (covered by phase 4)", () => {
    const path = "src/app/(portal)/teacher/courses/[id]/topics/[topicId]/page.tsx";
    it("heading says 'Edit Focus Area'", () => {
      expect(source(path)).toMatch(/Edit Focus Area/);
      expect(source(path)).not.toMatch(/<h1[^>]*>Edit Topic</);
    });
    it("card title says 'Focus Area Details'", () => {
      expect(source(path)).toMatch(/Focus Area Details/);
      expect(source(path)).not.toMatch(/>Topic Details</);
    });
  });
});
