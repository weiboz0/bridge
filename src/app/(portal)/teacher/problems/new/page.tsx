import { redirect } from "next/navigation";
import { getIdentity } from "@/lib/identity";
import { ProblemForm } from "@/components/problem/problem-form";

// Plan 066 phase 3 — create-problem page. Server component; the form
// itself is a client component. Identity is read here so the form can
// auto-fill `scopeId` for `personal` scope without a second fetch.

export default async function NewProblemPage() {
  const identity = await getIdentity();
  if (!identity) {
    redirect("/login");
  }
  return (
    <ProblemForm
      mode="create"
      identity={{
        userId: identity.userId,
        isPlatformAdmin: identity.isPlatformAdmin,
      }}
    />
  );
}
