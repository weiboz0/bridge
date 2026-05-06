"use client";

// Plan 069 phase 3 — editable org settings form.
//
// PATCH body strategy: always-send all four editable fields.
//
// The Go handler (platform/internal/handlers/orgs.go::UpdateOrg) uses *string
// pointer fields with `omitempty` on the STRUCT TAG (not on the JSON key). Go's
// encoding/json only skips *nil* pointer fields on marshal; on *unmarshal*, a
// field absent from the JSON body leaves the pointer as nil (no-change), while
// a field present in the body — even as "" — sets the pointer to that value.
//
// Sending all four fields on every submit is safe and keeps the form simple:
//   - name, contactEmail, contactName → always non-empty (validated client-side)
//   - domain → send trimmed value; empty string explicitly clears the domain
//     (the store writes "" to the DB column, which is semantically "no domain").
//     If you want true NULL clearing, that requires a backend change — for now
//     empty string is the supported way to unset the domain.

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { OrgSettingsData } from "./org-settings-card";

interface Props {
  org: OrgSettingsData;
}

function ReadOnlyField({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="grid grid-cols-3 gap-4 py-2 border-t text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="col-span-2 text-muted-foreground">
        {value || <span className="italic">not set</span>}
      </dd>
    </div>
  );
}

export function OrgSettingsForm({ org }: Props) {
  const router = useRouter();

  const [name, setName] = useState(org.name);
  const [contactEmail, setContactEmail] = useState(org.contactEmail ?? "");
  const [contactName, setContactName] = useState(org.contactName ?? "");
  const [domain, setDomain] = useState(org.domain ?? "");

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<Date | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSavedAt(null);

    const trimmedName = name.trim();
    const trimmedContactEmail = contactEmail.trim();
    const trimmedContactName = contactName.trim();
    const trimmedDomain = domain.trim();

    // Client-side validation mirrors the backend constraints.
    if (!trimmedName || trimmedName.length > 255) {
      setError("Name is required and must be 1–255 characters.");
      return;
    }
    if (!trimmedContactEmail) {
      setError("Contact email is required.");
      return;
    }
    if (!trimmedContactName || trimmedContactName.length > 255) {
      setError("Contact name is required and must be 1–255 characters.");
      return;
    }
    if (trimmedDomain.length > 255) {
      setError("Domain must be 255 characters or fewer.");
      return;
    }

    setSubmitting(true);
    try {
      const res = await fetch(`/api/orgs/${org.id}`, {
        method: "PATCH",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: trimmedName,
          contactEmail: trimmedContactEmail,
          contactName: trimmedContactName,
          domain: trimmedDomain,
        }),
      });

      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as
          | { error?: string }
          | null;
        setError(body?.error ?? `Request failed: ${res.status}`);
        return;
      }

      setSavedAt(new Date());
      // Refresh the server component so updatedAt and values stay in sync.
      router.refresh();

      // Clear the "Saved" feedback after 3 seconds.
      setTimeout(() => setSavedAt(null), 3000);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{org.name}</CardTitle>
      </CardHeader>
      <CardContent>
        {/* Read-only fields — not user-editable from this UI */}
        <dl className="mb-6">
          <ReadOnlyField label="Type" value={org.type} />
          <ReadOnlyField label="Status" value={org.status} />
          <ReadOnlyField
            label="Verified"
            value={org.verifiedAt ? new Date(org.verifiedAt).toLocaleDateString() : null}
          />
        </dl>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="org-name">Name</Label>
            <Input
              id="org-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={255}
              required
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="org-contact-email">Contact email</Label>
            <Input
              id="org-contact-email"
              type="email"
              value={contactEmail}
              onChange={(e) => setContactEmail(e.target.value)}
              required
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="org-contact-name">Contact name</Label>
            <Input
              id="org-contact-name"
              type="text"
              value={contactName}
              onChange={(e) => setContactName(e.target.value)}
              maxLength={255}
              required
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="org-domain">Domain <span className="text-muted-foreground font-normal">(optional)</span></Label>
            <Input
              id="org-domain"
              type="text"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              maxLength={255}
              placeholder="e.g. myschool.edu"
            />
            <p className="text-xs text-muted-foreground">
              Leave blank to clear the domain.
            </p>
          </div>

          {error && (
            <div
              role="alert"
              className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
            >
              {error}
            </div>
          )}

          {savedAt && (
            <p className="text-sm text-emerald-700" role="status">
              Saved at {savedAt.toLocaleTimeString()}
            </p>
          )}

          <div className="flex items-center justify-between pt-2">
            <p className="text-xs text-muted-foreground">
              Last updated: {new Date(org.updatedAt).toLocaleString()}
            </p>
            <Button type="submit" disabled={submitting}>
              {submitting ? "Saving…" : "Save changes"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
