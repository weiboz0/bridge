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
  constructor(message: string, public status?: number, public code?: string) {
    super(message);
    this.name = "RealtimeMintError";
  }
}

export async function getRealtimeToken(documentName: string, sessionId?: string, options: { forceRefresh?: boolean } = {}): Promise<string> {
  if (!documentName || documentName === "noop") {
    throw new RealtimeMintError("documentName is required");
  }

  const identityKey = sessionId === undefined ? documentName : `${documentName}\u0000${sessionId}`;
  const now = Date.now();
  if (options.forceRefresh) cache.delete(identityKey);
  const cached = cache.get(identityKey);
  if (cached && cached.expiresAt - now > LEEWAY_MS) {
    return cached.token;
  }

  const existing = inflight.get(identityKey);
  if (existing) return existing;

  const promise = mintFresh(documentName, sessionId)
    .then((minted) => {
      cache.set(identityKey, minted);
      return minted.token;
    })
    .finally(() => {
      inflight.delete(identityKey);
    });

  inflight.set(identityKey, promise);
  return promise;
}

async function mintFresh(documentName: string, sessionId?: string): Promise<CachedToken> {
  let res: Response;
  try {
    res = await fetch("/api/realtime/token", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(sessionId === undefined ? { documentName } : { documentName, sessionId }),
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
    let code: string | undefined;
    try {
      const body = parseMintFailure(await res.json(), res.status);
      if (body.error) detail = `${res.status} ${body.error}`;
      code = body.code;
    } catch (error) {
      if (error instanceof RealtimeMintError) throw error;
      /* body not JSON — keep status alone */
    }
    throw new RealtimeMintError(`Realtime token mint failed: ${detail}`, res.status, code);
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

function parseMintFailure(value: unknown, status: number): { error?: string; code?: string } {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new RealtimeMintError("Realtime token mint error response is invalid", status);
  }
  const body = value as Record<string, unknown>;
  if (Object.keys(body).some((key) => key !== "error" && key !== "code")
    || (body.error !== undefined && typeof body.error !== "string")
    || (body.code !== undefined && typeof body.code !== "string")) {
    throw new RealtimeMintError("Realtime token mint error response is invalid", status);
  }
  return { error: body.error as string | undefined, code: body.code as string | undefined };
}

// Test-only — drop the cache + in-flight maps so unit tests don't
// leak state between cases.
export function __resetRealtimeTokenCacheForTesting(): void {
  cache.clear();
  inflight.clear();
}
