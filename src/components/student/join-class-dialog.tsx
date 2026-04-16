"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function JoinClassDialog() {
  const router = useRouter();
  const [joinCode, setJoinCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);

  async function handleJoin(e: React.FormEvent) {
    e.preventDefault();
    if (!joinCode.trim()) return;

    setLoading(true);
    setError("");

    const res = await fetch("/api/classes/join", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ joinCode: joinCode.trim().toUpperCase() }),
    });

    if (res.ok) {
      setOpen(false);
      setJoinCode("");
      router.refresh();
    } else {
      const data = await res.json().catch(() => null);
      setError(data?.error || "Invalid join code");
    }
    setLoading(false);
  }

  if (!open) {
    return (
      <Button variant="outline" onClick={() => setOpen(true)}>
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
      <Button type="button" variant="ghost" onClick={() => { setOpen(false); setError(""); }}>
        Cancel
      </Button>
      {error && <p className="text-sm text-destructive">{error}</p>}
    </form>
  );
}
