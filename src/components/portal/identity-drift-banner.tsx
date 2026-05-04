"use client";

import { useEffect, useState } from "react";

// Plan 068 phase 2 — identity-drift warning banner.
//
// Browser review 010 §P0 #1 caught a tunneled review env where Next
// resolved real Auth.js users while the Go API resolved
// `Dev User` via DEV_SKIP_AUTH. The drift was technically detectable
// via `/api/auth/debug` but only by an operator who knew to look. This
// banner surfaces the drift to anyone signed in — a teacher who hits
// weirdness can flag it without first triaging through devtools.
//
// `/api/auth/debug` returns 404 in production (`src/app/api/auth/debug/route.ts:24-26`)
// — the banner silently no-ops on 404 so it never renders in prod
// builds. In dev/staging where the endpoint is live, the banner is
// scoped narrowly: it only renders when Go is running in
// `DEV_SKIP_AUTH=admin` mode (recognized by the well-known dev-user
// placeholder UUID below) AND the Next side has a real session AND
// the user is NOT legitimately impersonating someone (Codex pass-1
// flagged that during impersonation the live identity legitimately
// differs from the JWT — that's not drift, that's the feature).
//
// Other drift modes (Go unreachable, mismatched real users) are
// observable via /api/auth/debug directly but don't trigger the
// banner — the banner copy specifically points at DEV_SKIP_AUTH so
// it would mislead operators on those other failure modes.

// Well-known dev-user placeholder injected by middleware.go when
// DEV_SKIP_AUTH=admin. Matching this exact UUID (rather than just
// "any drift") narrows false positives to the specific ops-discipline
// failure plan 068 was designed around.
const DEV_USER_PLACEHOLDER = "00000000-0000-0000-0000-000000000001";

interface DebugResponse {
  nextAuthUserId: string | null;
  nextAuthEmail: string | null;
  goClaimsUserId: string | null;
  goClaimsEmail: string | null;
  goImpersonatedBy: string | null;
  goError: string | null;
  match: boolean;
}

export function IdentityDriftBanner() {
  const [drift, setDrift] = useState<DebugResponse | null>(null);

  useEffect(() => {
    let cancelled = false;
    async function check() {
      try {
        const res = await fetch("/api/auth/debug", { cache: "no-store" });
        if (res.status === 404) {
          // Production build — endpoint is gone. Silently no-op.
          return;
        }
        if (!res.ok) return;
        const body = (await res.json()) as DebugResponse;
        if (cancelled) return;
        if (
          !body.match &&
          body.nextAuthUserId !== null &&
          body.goClaimsUserId === DEV_USER_PLACEHOLDER &&
          !body.goImpersonatedBy
        ) {
          setDrift(body);
        }
      } catch {
        // Network blip / parse error — silently ignore. The banner is
        // a diagnostic, not load-bearing.
      }
    }
    void check();
    return () => {
      cancelled = true;
    };
  }, []);

  if (!drift) return null;

  const nextLabel = drift.nextAuthEmail ?? drift.nextAuthUserId ?? "(none)";
  const goLabel =
    drift.goClaimsEmail ??
    drift.goClaimsUserId ??
    (drift.goError ? `error: ${drift.goError}` : "(none)");

  return (
    <div
      role="alert"
      className="bg-rose-100 border-b-2 border-rose-500 text-rose-900 px-4 py-2 text-sm"
    >
      <div className="font-semibold">Auth identity mismatch detected</div>
      <div className="text-xs mt-0.5">
        Next.js shell shows <span className="font-mono">{nextLabel}</span>; the Go API
        sees <span className="font-mono">{goLabel}</span>. The Go server is likely
        running with <code>DEV_SKIP_AUTH</code> set on a non-localhost host. Operator
        action required — see <code>docs/setup.md</code> §"Host Exposure Declaration".
      </div>
    </div>
  );
}
