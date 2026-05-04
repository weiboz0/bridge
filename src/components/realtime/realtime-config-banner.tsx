"use client";

// Plan 068 phase 4 — surfaces the "realtime not configured" state
// (HOCUSPOCUS_TOKEN_SECRET unset on the Go API) as an in-page
// banner instead of letting the failure manifest as silent
// disconnects + console errors. Browser review 010 §P0 #3 caught
// this exact case on a tunneled review env.
//
// Render shape: a single yellow banner with explanatory copy. Used
// inline at the top of any session/teacher-watch/parent-viewer
// page that expects realtime collaboration. Hidden when realtime
// is working OR when the page hasn't tried to mint yet.

interface RealtimeConfigBannerProps {
  unavailable: boolean;
}

export function RealtimeConfigBanner({ unavailable }: RealtimeConfigBannerProps) {
  if (!unavailable) return null;
  return (
    <div
      role="alert"
      className="bg-amber-100 border-b border-amber-400 text-amber-900 px-4 py-2 text-sm"
    >
      <div className="font-semibold">Live collaboration is unavailable</div>
      <div className="text-xs mt-0.5">
        The realtime token service is not configured for this environment. Static
        viewing still works; reload the page after the operator sets{" "}
        <code>HOCUSPOCUS_TOKEN_SECRET</code> on the Go API and Hocuspocus
        processes.
      </div>
    </div>
  );
}
