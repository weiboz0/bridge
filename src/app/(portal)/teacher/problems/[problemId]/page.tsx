import { redirect, notFound } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { getIdentity } from "@/lib/identity";
import {
  TeacherProblemDetail,
  type ProblemDetailData,
  type TestCaseData,
} from "@/components/problem/teacher-problem-detail";

// Plan 066 phase 2 — teacher problem-bank detail page.
//
// Server component. Fetches the problem + its canonical test cases.
// All authorization decisions are deferred to the Go API: a 401 →
// /login, a 403 → "no access" message, a 404 → Next's standard
// not-found page. The "canAuthor" flag drives whether hidden test
// cases render in full vs as a count, and whether the destructive
// action buttons (Edit/Publish/Archive/Delete) appear.
//
// Plan 066 §"Phase 2 NOT /attempts" — the page does NOT fetch
// `/api/problems/{id}/attempts`. That endpoint is caller-scoped
// (returns the viewer's own attempts), not a cross-user activity
// feed; rendering it as "recent activity" would mislead a teacher
// who's never personally attempted their own authored problem.

export default async function ProblemDetailPage({
  params,
}: {
  params: Promise<{ problemId: string }>;
}) {
  const { problemId } = await params;

  let problem: ProblemDetailData;
  let testCases: TestCaseData[];
  try {
    [problem, testCases] = await Promise.all([
      api<ProblemDetailData>(`/api/problems/${problemId}`),
      api<TestCaseData[]>(`/api/problems/${problemId}/test-cases`),
    ]);
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) {
      redirect("/login");
    }
    if (e instanceof ApiError && e.status === 404) {
      notFound();
    }
    if (e instanceof ApiError && e.status === 403) {
      return (
        <div className="p-6 max-w-2xl">
          <h1 className="text-2xl font-bold">No access to this problem</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            You don&apos;t have permission to view this problem. It may be a
            personal problem owned by another teacher, an unpublished problem
            in another org, or a platform problem outside your access scope.
          </p>
          <p className="mt-4">
            <a href="/teacher/problems" className="underline text-primary">
              Back to Problem Bank
            </a>
          </p>
        </div>
      );
    }
    throw e;
  }

  // canAuthor: the viewer is a platform admin OR is the problem's
  // creator (proxied from the scope/scopeId). The Go side enforces
  // edit permission on UpdateProblem; we mirror its rule here only
  // to drive UI affordances. If we get it wrong, the user just sees
  // buttons that 403 — they don't actually escalate.
  const identity = await getIdentity();
  const canAuthor = computeCanAuthor(problem, identity);

  return (
    <TeacherProblemDetail
      problem={problem}
      testCases={testCases ?? []}
      canAuthor={canAuthor}
    />
  );
}

interface IdentityShape {
  userId: string;
  isPlatformAdmin: boolean;
}

function computeCanAuthor(
  problem: ProblemDetailData,
  identity: IdentityShape | null,
): boolean {
  if (!identity) return false;
  // Platform admins can author at any scope.
  if (identity.isPlatformAdmin) return true;
  // Personal-scope: only the owner.
  if (problem.scope === "personal") {
    return problem.scopeId === identity.userId;
  }
  // Codex pass-1: original heuristic returned true for any non-personal
  // problem regardless of viewer role. Too broad — affects hidden-case
  // visibility, not just button enablement. The backend's
  // `authorizedForScope` requires platform-admin for `platform` scope
  // and active org_admin/teacher membership for `org` scope.
  //
  // We don't have the viewer's org memberships in `Identity` (would
  // require a /api/me/memberships fetch), so be CONSERVATIVE: hide
  // Edit/Publish/Archive/Delete affordances by default for org/
  // platform-scope viewers who aren't admins. Fork stays visible
  // (everyone with read access can clone). A legitimate org-scoped
  // teacher can navigate to /teacher/problems/{id}/edit directly —
  // the backend will accept the request. Wiring the org-membership
  // check is a Phase-2-follow-up that adds /api/me/memberships to
  // the identity helper.
  return false;
}
