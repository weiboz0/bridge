/**
 * Single source of truth for the Auth.js v5 session cookie name.
 *
 * Auth.js writes ONE cookie based on whether the configured site URL
 * is HTTPS. Both `auth.ts` (which configures Auth.js) and `api-client.ts`
 * (which forwards the token to Go) read from this helper so they cannot
 * disagree about which cookie holds the live token.
 *
 * If both cookie variants exist in the browser jar (e.g. a stale
 * `__Secure-` cookie from a prior deployment), only the one named here
 * is canonical. Callers must NOT silently fall back to the other.
 */

const SECURE_COOKIE_NAME = "__Secure-authjs.session-token";
const INSECURE_COOKIE_NAME = "authjs.session-token";

function configuredAuthUrl(): string {
  return process.env.NEXTAUTH_URL || process.env.AUTH_URL || "";
}

export function isSecureAuthScheme(): boolean {
  return configuredAuthUrl().toLowerCase().startsWith("https://");
}

export function getSessionCookieName(): string {
  return isSecureAuthScheme() ? SECURE_COOKIE_NAME : INSECURE_COOKIE_NAME;
}

export function getOtherSessionCookieName(): string {
  return isSecureAuthScheme() ? INSECURE_COOKIE_NAME : SECURE_COOKIE_NAME;
}

export const AUTH_SESSION_COOKIE_NAMES = [SECURE_COOKIE_NAME, INSECURE_COOKIE_NAME] as const;
