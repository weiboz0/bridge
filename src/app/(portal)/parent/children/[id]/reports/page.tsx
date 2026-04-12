"use client";

import { useState, useEffect } from "react";
import { useParams } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

interface Report {
  id: string;
  periodStart: string;
  periodEnd: string;
  content: string;
  createdAt: string;
}

export default function ParentReportsPage() {
  const params = useParams<{ id: string }>();
  const [reports, setReports] = useState<Report[]>([]);
  const [generating, setGenerating] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    async function fetchReports() {
      const res = await fetch(`/api/parent/children/${params.id}/reports`);
      if (res.ok) setReports(await res.json());
    }
    fetchReports();
  }, [params.id]);

  async function handleGenerate() {
    setGenerating(true);
    setError("");

    const res = await fetch(`/api/parent/children/${params.id}/reports`, {
      method: "POST",
    });

    if (res.ok) {
      const report = await res.json();
      setReports((prev) => [report, ...prev]);
    } else {
      const data = await res.json();
      setError(data.error || "Failed to generate report");
    }

    setGenerating(false);
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Progress Reports</h1>
        <Button onClick={handleGenerate} disabled={generating}>
          {generating ? "Generating..." : "Generate Weekly Report"}
        </Button>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}

      {reports.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No reports yet. Click "Generate Weekly Report" to create one.</p>
          </CardContent>
        </Card>
      ) : (
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
      )}
    </div>
  );
}
