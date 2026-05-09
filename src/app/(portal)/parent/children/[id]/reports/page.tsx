"use client";

import { useState, useEffect } from "react";
import { useParams } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface Report {
  id: string;
  periodStart: string;
  periodEnd: string;
  content: string;
  createdAt: string;
}

// Plan 080 — copy unstaling. Removed: 501-coming-soon path (Go endpoint
// stopped 501-ing once parent linking shipped in plans 064/070), the
// "we're still building parent-child account linking" copy block, and
// the broken "Generate Weekly Report" button.
//
// Go POST /api/parent/children/{id}/reports requires
// { content, periodStart, periodEnd }; the previous button POSTed an
// empty body and 400'd every time. No UI flow exists yet — system-
// generated reports (LLM agent, scheduled task) are tracked separately.

export default function ParentReportsPage() {
  const params = useParams<{ id: string }>();
  const [reports, setReports] = useState<Report[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    async function fetchReports() {
      try {
        const res = await fetch(`/api/parent/children/${params.id}/reports`);
        if (res.ok) {
          setReports(await res.json());
        } else {
          const data = await res.json().catch(() => ({}));
          setError(data.error || `Failed to load reports (${res.status})`);
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    }
    fetchReports();
  }, [params.id]);

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Progress Reports</h1>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}

      {reports.length === 0 && !error ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No reports yet. Progress reports will appear here once they&apos;re generated.</p>
          </CardContent>
        </Card>
      ) : reports.length > 0 ? (
        <div className="space-y-4">
          {reports.map((report) => (
            <Card key={report.id}>
              <CardHeader>
                <CardTitle className="text-base">
                  {new Date(report.periodStart).toLocaleDateString()} — {new Date(report.periodEnd).toLocaleDateString()}
                </CardTitle>
                <p className="text-xs text-muted-foreground">
                  Generated {new Date(report.createdAt).toLocaleString()}
                </p>
              </CardHeader>
              <CardContent>
                <div className="prose prose-sm dark:prose-invert max-w-none whitespace-pre-wrap">
                  {report.content}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : null}
    </div>
  );
}
