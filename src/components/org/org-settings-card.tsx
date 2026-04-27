import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import type { OrgListError } from "./org-list-state";

export interface OrgSettingsData {
  id: string;
  name: string;
  type: string;
  status: string;
  contactEmail?: string;
  contactName?: string;
  domain?: string | null;
  verifiedAt?: string | null;
}

interface OrgSettingsCardProps {
  org: OrgSettingsData | null;
  error: OrgListError | null;
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="grid grid-cols-3 gap-4 py-2 border-t text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="col-span-2">{value || <span className="text-muted-foreground italic">not set</span>}</dd>
    </div>
  );
}

export function OrgSettingsCard({ org, error }: OrgSettingsCardProps) {
  if (error) {
    return (
      <Card className="border-destructive/50">
        <CardHeader>
          <CardTitle className="text-destructive">
            Couldn&rsquo;t load settings
            {error.status ? ` (HTTP ${error.status})` : ""}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <p className="text-muted-foreground">{error.message}</p>
          <Link href="/org/settings" className="text-primary underline">
            Retry
          </Link>
        </CardContent>
      </Card>
    );
  }

  if (!org) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          No organization is associated with your account.
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{org.name}</CardTitle>
      </CardHeader>
      <CardContent>
        <dl>
          <Field label="Type" value={org.type} />
          <Field label="Status" value={org.status} />
          <Field label="Contact name" value={org.contactName} />
          <Field label="Contact email" value={org.contactEmail} />
          <Field label="Domain" value={org.domain} />
          <Field
            label="Verified"
            value={org.verifiedAt ? new Date(org.verifiedAt).toLocaleDateString() : null}
          />
        </dl>
        <p className="mt-4 text-xs text-muted-foreground">
          Editing organization settings is coming in a future update.
        </p>
      </CardContent>
    </Card>
  );
}
