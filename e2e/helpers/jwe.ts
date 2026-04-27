import { encode } from "@auth/core/jwt";

const SECURE_COOKIE_NAME = "__Secure-authjs.session-token";
const INSECURE_COOKIE_NAME = "authjs.session-token";

/**
 * Mints a valid Auth.js v5 JWE session token for the given identity. The
 * salt is the cookie name (Auth.js convention); the encryption key is
 * derived from NEXTAUTH_SECRET via the same HKDF the runtime uses.
 *
 * Tokens minted here decrypt correctly under the running Go middleware
 * (`platform/internal/auth/jwt.go::DecryptAuthJSToken`) because both sides
 * call the same algorithm.
 *
 * Use case: Phase 5.1b plants a *valid signed* stale token in the browser
 * jar to prove the canonical-cookie selection in plan 039 actually
 * rejects an attacker-supplied legitimate token from a different user —
 * not just opaque garbage.
 */
export async function mintSessionToken(opts: {
  sub: string;
  email: string;
  name: string;
  cookieName?: typeof SECURE_COOKIE_NAME | typeof INSECURE_COOKIE_NAME;
  expiresInSeconds?: number;
  isPlatformAdmin?: boolean;
}): Promise<string> {
  const secret = process.env.NEXTAUTH_SECRET;
  if (!secret) {
    throw new Error(
      "mintSessionToken requires NEXTAUTH_SECRET — set it in the E2E shell or playwright.config.ts"
    );
  }

  const salt = opts.cookieName ?? SECURE_COOKIE_NAME;

  return encode({
    secret,
    salt,
    maxAge: opts.expiresInSeconds ?? 60 * 60 * 24,
    token: {
      sub: opts.sub,
      id: opts.sub, // Bridge JWT callback also stores `id` — match the runtime shape
      email: opts.email,
      name: opts.name,
      isPlatformAdmin: opts.isPlatformAdmin ?? false,
    },
  });
}

export const AUTH_COOKIE_NAMES = {
  secure: SECURE_COOKIE_NAME,
  insecure: INSECURE_COOKIE_NAME,
} as const;
