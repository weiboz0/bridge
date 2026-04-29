import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";

/**
 * Plan 047 phase 3: the broken `redirect("/register-org")` on the
 * teacher branch was replaced by inline copy. The route doesn't exist
 * (it 404s), so re-introducing the redirect would put every email/
 * password teacher signup back on a broken page.
 *
 * The onboarding page is an async React Server Component that touches
 * `auth()` and the DB, which makes it awkward to render in jsdom for
 * a behavioral assertion. A source-text regression check is the
 * pragmatic fit: it catches the only mistake we care about (someone
 * adding the redirect back) without setting up server-component
 * test infrastructure that would fail for unrelated reasons.
 *
 * If this test ever becomes obsolete because `/register-org` ships
 * (plan 048), delete this file alongside the new redirect.
 */
describe("onboarding/page.tsx — Plan 047 phase 3 no /register-org redirect", () => {
  const source = readFileSync("src/app/onboarding/page.tsx", "utf8");

  it("does not redirect teachers to /register-org", () => {
    expect(source).not.toMatch(/redirect\(\s*["']\/register-org["']\s*\)/);
  });

  it("does not link to /register-org", () => {
    expect(source).not.toMatch(/href=["']\/register-org["']/);
  });

  it("renders teacher-specific copy when intendedRole is 'teacher'", () => {
    // Verifies the page distinguishes the teacher branch (instead of
    // the generic "no role yet" fallback) by referencing `isTeacher`
    // somewhere in the JSX.
    expect(source).toMatch(/isTeacher/);
  });
});
