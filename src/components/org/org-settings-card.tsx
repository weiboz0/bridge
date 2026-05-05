import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import Link from "next/link";
import type { OrgListError } from "./org-list-state";
import { OrgSettingsForm } from "./org-settings-form";

export interface OrgSettingsData {
  id: string;
  name: string;
  type: string;
  status: string;
  contactEmail?: string;
  contactName?: string;
  domain?: string | null;
  verifiedAt?: string | null;
  updatedAt: string;
}

interface OrgSettingsCardProps {
  org: OrgSettingsData | null;
  error: OrgListError | null;
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

  return <OrgSettingsForm org={org} />;
}
