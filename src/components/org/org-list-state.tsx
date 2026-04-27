import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export interface OrgListError {
  status: number | null;
  message: string;
}

interface OrgListStateProps {
  // Pages pass null when the fetch failed; data when it succeeded.
  // Empty array is "succeeded but the list is empty" — different from null.
  data: unknown[] | null;
  error: OrgListError | null;
  emptyMessage: string;
  retryHref: string;
  children: React.ReactNode;
}

/**
 * Wraps an org list page in the standard error-card / empty-state /
 * populated decision. Page passes the rendered table as children; this
 * component decides whether to show it.
 */
export function OrgListState({
  data,
  error,
  emptyMessage,
  retryHref,
  children,
}: OrgListStateProps) {
  if (error) {
    return (
      <Card className="border-destructive/50">
        <CardHeader>
          <CardTitle className="text-destructive">
            Couldn&rsquo;t load this list
            {error.status ? ` (HTTP ${error.status})` : ""}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <p className="text-muted-foreground">{error.message}</p>
          {error.status === 403 && (
            <p>
              The Go API rejected this admin request. If you just signed
              in, check <code>/api/auth/debug</code> to confirm both
              layers resolved the same identity.
            </p>
          )}
          <div className="flex gap-3 pt-2">
            <Link href={retryHref} className="text-primary underline">
              Retry
            </Link>
          </div>
        </CardContent>
      </Card>
    );
  }

  if (!data || data.length === 0) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          {emptyMessage}
        </CardContent>
      </Card>
    );
  }

  return <>{children}</>;
}
