import { redirect, notFound } from "next/navigation";
import { api, ApiError } from "@/lib/api-client";
import { getIdentity } from "@/lib/identity";
import { ProblemForm } from "@/components/problem/problem-form";
import type { ProblemDetailData } from "@/components/problem/teacher-problem-detail";

// Plan 066 phase 3 — edit-problem page. Server component; loads the
// problem so the form can be primed with current values. The form
// PATCHes the same Go endpoint that this page reads from.

export default async function EditProblemPage({
  params,
}: {
  params: Promise<{ problemId: string }>;
}) {
  const { problemId } = await params;

  const identity = await getIdentity();
  if (!identity) {
    redirect("/login");
  }

  let problem: ProblemDetailData;
  try {
    problem = await api<ProblemDetailData>(`/api/problems/${problemId}`);
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) redirect("/login");
    if (e instanceof ApiError && e.status === 404) notFound();
    if (e instanceof ApiError && e.status === 403) {
      return (
        <div className="p-6 max-w-2xl">
          <h1 className="text-2xl font-bold">No access to this problem</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            You don&apos;t have permission to view this problem.
          </p>
        </div>
      );
    }
    throw e;
  }

  return (
    <ProblemForm
      mode="edit"
      identity={{
        userId: identity.userId,
        isPlatformAdmin: identity.isPlatformAdmin,
      }}
      initial={problem}
    />
  );
}
