import { createHmac, timingSafeEqual } from "node:crypto";

// TS counterpart to platform/internal/auth/realtime_jwt.go.
//
// Plan 053 phase 2: the Hocuspocus Node process verifies HMAC-SHA256
// JWTs minted by the Go API (`POST /api/realtime/token`) when the
// connection is established. The shared secret must match
// HOCUSPOCUS_TOKEN_SECRET on both sides.

export const REALTIME_ISSUER = "bridge-platform";

export interface RealtimeClaims {
  sub: string;
  role: string;
  scope: string;
  iss: string;
  iat: number;
  exp: number;
}

export class JwtVerifyError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "JwtVerifyError";
  }
}

export function isLikelyJwt(token: string): boolean {
  // The base64url-encoded JWT header `{"alg":"HS256","typ":"JWT"}`
  // ALWAYS starts with `ey` — exploit that for the legacy/JWT split
  // during the phase-4 rollout window. NOT a security check; the
  // signature verify below is the actual gate.
  return token.startsWith("ey") && token.split(".").length === 3;
}

export function verifyRealtimeJwt(token: string, secret: string): RealtimeClaims {
  if (!secret) {
    throw new JwtVerifyError("HOCUSPOCUS_TOKEN_SECRET is empty");
  }
  const parts = token.split(".");
  if (parts.length !== 3) {
    throw new JwtVerifyError("Invalid JWT format");
  }
  const [encodedHeader, encodedPayload, encodedSig] = parts;

  // Verify signature with HS256 unconditionally — NEVER trust the
  // header's `alg` to choose the verification algorithm (alg=none
  // and alg=RS256→HS256 confusion attacks). The header check below
  // is cosmetic spec-compliance after signature passes.
  const expected = createHmac("sha256", secret)
    .update(`${encodedHeader}.${encodedPayload}`)
    .digest();
  let actual: Buffer;
  try {
    actual = Buffer.from(encodedSig, "base64url");
  } catch {
    throw new JwtVerifyError("signature decode failed");
  }
  if (expected.length !== actual.length || !timingSafeEqual(expected, actual)) {
    throw new JwtVerifyError("signature is invalid");
  }

  let header: { alg?: unknown; typ?: unknown };
  let claims: Partial<RealtimeClaims>;
  try {
    header = JSON.parse(Buffer.from(encodedHeader, "base64url").toString("utf8"));
    claims = JSON.parse(Buffer.from(encodedPayload, "base64url").toString("utf8")) as Partial<RealtimeClaims>;
  } catch {
    throw new JwtVerifyError("JWT parse failed");
  }
  if (header.alg !== "HS256") {
    throw new JwtVerifyError("Unexpected JWT alg");
  }
  if (claims.iss !== REALTIME_ISSUER) {
    throw new JwtVerifyError(`wrong issuer (got ${String(claims.iss)})`);
  }
  if (typeof claims.exp !== "number") {
    throw new JwtVerifyError("missing exp");
  }
  if (typeof claims.iat !== "number") {
    throw new JwtVerifyError("missing iat");
  }
  const now = Math.floor(Date.now() / 1000);
  if (claims.exp < now) {
    throw new JwtVerifyError("token expired");
  }
  if (claims.iat > now + 60) {
    // 60s leeway for clock skew between API mint and Hocuspocus verify.
    throw new JwtVerifyError("iat in the future");
  }
  if (!claims.sub || typeof claims.sub !== "string") {
    throw new JwtVerifyError("missing sub");
  }
  if (!claims.role || typeof claims.role !== "string") {
    throw new JwtVerifyError("missing role");
  }
  if (!claims.scope || typeof claims.scope !== "string") {
    throw new JwtVerifyError("missing scope");
  }
  return claims as RealtimeClaims;
}

// Defense-in-depth: after JWT verify passes, ask the Go API whether
// the user STILL has access to the document. Catches the
// "JWT minted, then user demoted" race. Returns true on allow, false
// on deny, throws on infrastructure failure (so Hocuspocus tears
// down the connection rather than silently deny).
export async function rechckDocumentAccess(args: {
  apiBaseUrl: string;
  secret: string;
  documentName: string;
  sub: string;
}): Promise<{ allowed: boolean; reason?: string }> {
  const { apiBaseUrl, secret, documentName, sub } = args;
  const res = await fetch(`${apiBaseUrl}/api/internal/realtime/auth`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${secret}`,
    },
    body: JSON.stringify({ documentName, sub }),
  });
  if (res.status === 200) {
    const body = (await res.json()) as { allowed?: boolean; reason?: string };
    return { allowed: !!body.allowed, reason: body.reason };
  }
  // Anything other than 200 means the recheck couldn't render an
  // authorization decision (4xx malformed input, 404 missing
  // resource, 500 DB outage). Surface as an error — the caller will
  // close the connection rather than fall through to allow.
  let detail = `${res.status}`;
  try {
    const body = (await res.json()) as { error?: string };
    if (body.error) detail = `${res.status} ${body.error}`;
  } catch {
    /* body wasn't JSON */
  }
  throw new JwtVerifyError(`internal recheck failed: ${detail}`);
}
