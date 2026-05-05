"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { CreateParentLinkModal } from "./create-parent-link-modal";
import type {
  ParentLinkRow,
  OrgStudentRow,
} from "@/app/(portal)/org/parent-links/page";

// Plan 070 phase 2 — client-side wrapper for the org-admin parent
// link list. Owns the create-modal open state and the in-progress
// revoke spinner. The list itself is server-rendered (initialLinks);
// after a successful create or revoke we call router.refresh() to
// re-pull from Go rather than maintaining client-side mutations.

interface Props {
  orgId: string;
  orgName: string;
  initialLinks: ParentLinkRow[];
  students: OrgStudentRow[];
  error: { status: number | null; message: string } | null;
  /** Set when the eligible-children fetch failed. The list still
   *  renders, but the create modal surfaces a hint that the
   *  autocomplete is empty due to a backend error rather than an
   *  empty roster. */
  studentsError: { status: number | null; message: string } | null;
}

export function OrgParentLinksView({
  orgId,
  orgName,
  initialLinks,
  students,
  error,
  studentsError,
}: Props) {
  const router = useRouter();
  const [createOpen, setCreateOpen] = useState(false);
  const [revoking, setRevoking] = useState<string | null>(null);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  async function handleRevoke(link: ParentLinkRow) {
    if (
      !confirm(
        `Revoke link between ${link.parentEmail} and ${link.childName}?\n\n` +
          `The parent loses access to ${link.childName}'s data immediately. ` +
          `Re-creating the link is allowed if needed.`,
      )
    ) {
      return;
    }
    setRevoking(link.id);
    setRevokeError(null);
    try {
      const res = await fetch(
        `/api/orgs/${orgId}/parent-links/${link.id}`,
        { method: "DELETE", credentials: "include" },
      );
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as
          | { error?: string }
          | null;
        throw new Error(body?.error ?? `Revoke failed: ${res.status}`);
      }
      router.refresh();
    } catch (e) {
      setRevokeError(e instanceof Error ? e.message : String(e));
    } finally {
      setRevoking(null);
    }
  }

  return (
    <div className="p-6 max-w-5xl space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">
            Parent links{orgName ? ` · ${orgName}` : ""}
          </h1>
          <p className="text-sm text-muted-foreground">
            Wire a parent&apos;s account to a student so the parent can see
            live sessions and progress reports.
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)}>+ Add parent link</Button>
      </div>

      {error && (
        <Card className="border-rose-200 bg-rose-50">
          <CardHeader>
            <CardTitle className="text-sm text-rose-800">
              Couldn&apos;t load parent links
            </CardTitle>
          </CardHeader>
          <CardContent className="text-sm text-rose-800">
            <p>{error.message}</p>
            {error.status === 403 && (
              <p className="mt-1">
                You may not have org-admin access to this organization.
              </p>
            )}
          </CardContent>
        </Card>
      )}

      {revokeError && (
        <div
          role="alert"
          className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
        >
          {revokeError}
        </div>
      )}

      {initialLinks.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center space-y-3">
            <p className="text-base font-medium">No parent links yet.</p>
            <p className="text-sm text-muted-foreground max-w-md mx-auto">
              Click &ldquo;Add parent link&rdquo; to wire a registered parent
              account to a student in this organization.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr className="text-left">
                <th className="px-4 py-2 font-medium">Parent</th>
                <th className="px-4 py-2 font-medium">Child</th>
                <th className="px-4 py-2 font-medium">Class</th>
                <th className="px-4 py-2 font-medium">Created</th>
                <th className="px-4 py-2 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {initialLinks.map((link) => (
                <tr key={link.id} className="border-t">
                  <td className="px-4 py-2">
                    <div className="font-medium">{link.parentName}</div>
                    <div className="text-xs text-muted-foreground font-mono">
                      {link.parentEmail}
                    </div>
                  </td>
                  <td className="px-4 py-2">
                    <div className="font-medium">{link.childName}</div>
                    <div className="text-xs text-muted-foreground font-mono">
                      {link.childEmail}
                    </div>
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">
                    {link.className ?? "—"}
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">
                    {new Date(link.createdAt).toLocaleDateString()}
                  </td>
                  <td className="px-4 py-2 text-right">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => handleRevoke(link)}
                      disabled={revoking !== null}
                    >
                      {revoking === link.id ? "Revoking…" : "Revoke"}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {createOpen && (
        <CreateParentLinkModal
          orgId={orgId}
          students={students}
          studentsError={studentsError}
          onClose={() => setCreateOpen(false)}
          onCreated={() => {
            setCreateOpen(false);
            router.refresh();
          }}
        />
      )}
    </div>
  );
}
