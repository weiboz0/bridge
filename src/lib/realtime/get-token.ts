// Plan 053 phase 3: client-side mint helper. Single source of truth
// for the 5 token-construction sites (problem editor, teacher watch,
// unit editor, student session, teacher dashboard). Each callsite
// composes a Hocuspocus documentName and asks this helper for a JWT
// scoped to it.
//
// Replaces the legacy `${userId}:role` token construction. The Go
// API (`POST /api/realtime/token`) gates per-doc access at mint time;
// Hocuspocus verifies the signature on connect and rechecks the DB
// in `onLoadDocument` (plan 053 phase 2).

interface CachedToken {
  token: string;
  expiresAt: number; // epoch ms
}

// In-memory cache keyed by documentName. Survives across components
// that subscribe to the same doc — one network call per (doc, tab).
// Cleared on hard navigation since this lives in module state.
const cache = new Map<string, CachedToken>();

// In-flight de-dupe: when N components mount simultaneously and ask
// for the same doc, fold them onto a single network request.
const inflight = new Map<string, Promise<string>>();

// Refresh `LEEWAY_MS` before the JWT's `exp`. The Go mint clamps TTL
// to 30 min; we render at 25 min in the response so a 60s leeway is
// comfortable but doesn't pre-fetch too aggressively.
const LEEWAY_MS = 60_000;

export class RealtimeMintError extends Error {
  constructor(message: string, public status?: number) {
    super(message);
    this.name = "RealtimeMintError";
  }
}

export async function getRealtimeToken(documentName: string): Promise<string> {
  if (!documentName || documentName === "noop") {
    throw new RealtimeMintError("documentName is required");
  }

  const now = Date.now();
  const cached = cache.get(documentName);
  if (cached && cached.expiresAt - now > LEEWAY_MS) {
    return cached.token;
  }

  const existing = inflight.get(documentName);
  if (existing) return existing;

  const promise = mintFresh(documentName)
    .then((minted) => {
      cache.set(documentName, minted);
      return minted.token;
    })
    .finally(() => {
      inflight.delete(documentName);
    });

  inflight.set(documentName, promise);
  return promise;
}

async function mintFresh(documentName: string): Promise<CachedToken> {
  let res: Response;
  try {
    res = await fetch("/api/realtime/token", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ documentName }),
      credentials: "include",
    });
  } catch (err) {
    throw new RealtimeMintError(
      `Realtime token network error: ${err instanceof Error ? err.message : String(err)}`,
    );
  }
  if (res.status === 503) {
    throw new RealtimeMintError(
      "Realtime tokens not configured (HOCUSPOCUS_TOKEN_SECRET unset)",
      503,
    );
  }
  if (!res.ok) {
    let detail = `${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) detail = `${res.status} ${body.error}`;
    } catch {
      /* body not JSON — keep status alone */
    }
    throw new RealtimeMintError(`Realtime token mint failed: ${detail}`, res.status);
  }

  const body = (await res.json()) as { token: string; expiresAt: string };
  if (!body.token || !body.expiresAt) {
    throw new RealtimeMintError("Realtime token response missing fields");
  }
  const expiresAt = new Date(body.expiresAt).getTime();
  if (Number.isNaN(expiresAt)) {
    throw new RealtimeMintError("Realtime token expiresAt is unparseable");
  }
  return { token: body.token, expiresAt };
}

// Test-only — drop the cache + in-flight maps so unit tests don't
// leak state between cases.
export function __resetRealtimeTokenCacheForTesting(): void {
  cache.clear();
  inflight.clear();
}
