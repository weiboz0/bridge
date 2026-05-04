// Plan 065 Phase 2 — Edge-runtime-safe helpers for the lazy
// `bridge.session` mint that runs inside `src/middleware.ts`.
//
// The middleware fires on every authenticated request to a portal
// or proxied API path. When `req.auth` is present and the cookie is
// missing or close to expiry, the helper calls Go's
// `POST /api/internal/sessions` (server-to-server, bearer-protected)
// and the middleware attaches the resulting Set-Cookie to the
// response.
//
// Constraints:
//   - Edge runtime: no Node-only APIs. `fetch` and base64 are fine;
//     `crypto.subtle` works but isn't needed here. NO `Buffer`, no
//     `node:*` modules.
//   - Idempotent: middleware may invoke this on every request; the
//     expiry threshold ensures we only call Go when needed (at
//     most once per ~6 days for steady-state users).
//   - Fails closed: any error returns null. Caller decides what to
//     do — the middleware leaves the existing cookie alone and the
//     Go side falls back to JWE during rollout.

const REFRESH_THRESHOLD_MS = 24 * 60 * 60 * 1000; // 24h before expiry

// bridgeSessionExpiringSoon decodes a JWT's payload (without
// verifying the signature — we trust our own cookie's format)
// and returns true when the token is missing, malformed, or its
// `exp` is within REFRESH_THRESHOLD_MS of now. Any error → true,
// so the caller re-mints rather than holds onto a corrupt cookie.
export function bridgeSessionExpiringSoon(token: string | undefined): boolean {
  if (!token) return true;
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return true;
    const payload = JSON.parse(decodeBase64Url(parts[1])) as { exp?: number };
    if (typeof payload.exp !== "number") return true;
    const expiresAtMs = payload.exp * 1000;
    // Use <= so exactly-at-the-threshold re-mints. Over-mint by a
    // hair is preferred over serving a token that flips to expired
    // between this check and the request reaching Go.
    return expiresAtMs - Date.now() <= REFRESH_THRESHOLD_MS;
  } catch {
    return true;
  }
}

// MintResult is what mintBridgeSession returns on success.
export interface MintResult {
  token: string;
  expiresAt: Date;
}

// mintBridgeSession calls Go's bearer-protected mint endpoint and
// returns the signed token + absolute expiry. Returns null on any
// error (network, non-200, malformed body, etc.) — the middleware
// treats null as "leave the cookie alone for this request and try
// again on the next one."
export async function mintBridgeSession(args: {
  email: string;
  name: string;
}): Promise<MintResult | null> {
  const goBaseUrl = process.env.GO_INTERNAL_API_URL || "http://localhost:8002";
  const bearer = process.env.BRIDGE_INTERNAL_SECRET;

  if (!bearer) {
    // Endpoint requires a bearer; without it the request would 503
    // anyway. Skip the network round-trip so dev environments
    // without the secret aren't pelting the Go API with rejections.
    return null;
  }

  try {
    const res = await fetch(`${goBaseUrl}/api/internal/sessions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${bearer}`,
      },
      body: JSON.stringify({ email: args.email, name: args.name }),
      // Edge runtime: keep the timeout tight so a slow/down Go
      // backend doesn't stall every authenticated request. Default
      // fetch has no timeout; AbortController is the Edge-safe
      // way to cap it.
      signal: AbortSignal.timeout(3000),
    });
    if (!res.ok) return null;
    const body = (await res.json()) as { token?: string; expiresAt?: string };
    if (!body.token || !body.expiresAt) return null;
    const expiresAt = new Date(body.expiresAt);
    if (Number.isNaN(expiresAt.getTime())) return null;
    return { token: body.token, expiresAt };
  } catch {
    return null;
  }
}

// decodeBase64Url is the JWT-payload decoder. JWT uses base64url
// (RFC 4648 §5) — '-' and '_' instead of '+' and '/', and no
// padding. Edge's `atob` understands standard base64; we
// translate first.
function decodeBase64Url(s: string): string {
  // Pad to a multiple of 4.
  const padded = s + "=".repeat((4 - (s.length % 4)) % 4);
  const standard = padded.replace(/-/g, "+").replace(/_/g, "/");
  return atob(standard);
}
