"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface JoinResult {
  class: { id: string; title: string };
}

interface MyClass {
  id: string;
  title: string;
}

export function JoinClassDialog() {
  const router = useRouter();
  const [joinCode, setJoinCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [open, setOpen] = useState(false);

  async function handleJoin(e: React.FormEvent) {
    e.preventDefault();
    if (!joinCode.trim()) return;

    setLoading(true);
    setError("");
    setSuccess("");

    let joinedClassId: string | null = null;
    try {
      const joinRes = await fetch("/api/classes/join", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ joinCode: joinCode.trim().toUpperCase() }),
      });
      if (!joinRes.ok) {
        const data = await joinRes.json().catch(() => null);
        setError(data?.error || "Invalid join code");
        setLoading(false);
        return;
      }
      const result = (await joinRes.json()) as JoinResult;
      joinedClassId = result.class?.id ?? null;
      if (!joinedClassId) {
        setError("Server accepted the join but didn't return a class. Please try again.");
        setLoading(false);
        return;
      }
    } catch {
      setError("Couldn't reach the server. Check your connection and try again.");
      setLoading(false);
      return;
    }

    // Verify the joined class shows up in the student's canonical class list.
    // If it doesn't, the dashboard will lie and "no classes yet" persists —
    // this is the review-002 symptom we're locking out.
    try {
      const mineRes = await fetch("/api/classes/mine", { cache: "no-store" });
      if (!mineRes.ok) {
        setError("Joined, but couldn't verify your class list. Refresh the page.");
        setLoading(false);
        return;
      }
      const mine = (await mineRes.json()) as MyClass[];
      const present = mine.some((c) => c.id === joinedClassId);
      if (!present) {
        setError(
          "Joined, but the class isn't showing up. Sign out and back in if this persists."
        );
        setLoading(false);
        return;
      }
    } catch {
      setError("Joined, but verification failed. Refresh the page.");
      setLoading(false);
      return;
    }

    setSuccess("Joined! Loading your class…");
    setJoinCode("");
    setLoading(false);
    setOpen(false);
    router.refresh();
  }

  if (!open) {
    return (
      <Button variant="outline" onClick={() => { setOpen(true); setError(""); setSuccess(""); }}>
        Join a Class
      </Button>
    );
  }

  return (
    <form onSubmit={handleJoin} className="flex items-end gap-2">
      <div className="space-y-1">
        <Label htmlFor="joinCode" className="text-xs text-muted-foreground">
          Enter join code from your teacher
        </Label>
        <Input
          id="joinCode"
          value={joinCode}
          onChange={(e) => setJoinCode(e.target.value)}
          placeholder="e.g. ABCD1234"
          className="w-40 uppercase"
          maxLength={8}
          autoFocus
        />
      </div>
      <Button type="submit" disabled={loading || !joinCode.trim()}>
        {loading ? "Joining..." : "Join"}
      </Button>
      <Button
        type="button"
        variant="ghost"
        onClick={() => { setOpen(false); setError(""); setSuccess(""); }}
      >
        Cancel
      </Button>
      {error && <p className="text-sm text-destructive">{error}</p>}
      {success && <p className="text-sm text-emerald-600">{success}</p>}
    </form>
  );
}
