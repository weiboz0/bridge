package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// BridgeSessionCookie is the cookie name Phase 2's Edge middleware
// sets and Phase 3's Go middleware reads when BRIDGE_SESSION_AUTH=1.
const BridgeSessionCookie = "bridge.session"

// canonicalCookieName returns the single Auth.js session cookie name for the
// request's scheme. It does NOT fall back to the other variant — falling back
// re-injects stale identity when a previous deployment left a different
// cookie in the browser jar. This mirrors src/lib/auth-cookie.ts on the Next
// side so both layers agree on which cookie is canonical for a given scheme.
//
// X-Forwarded-Proto is honored ONLY when the immediate peer is in
// TRUSTED_PROXY_CIDRS — otherwise an unauthenticated client could spoof
// `X-Forwarded-Proto: https` and steer us to read the (potentially stale)
// __Secure- cookie instead of the real canonical one. Direct hits with
// r.TLS != nil are always trusted.
func canonicalCookieName(r *http.Request) string {
	isSecure := r.TLS != nil
	if !isSecure && IsTrustedProxy(r.RemoteAddr) {
		isSecure = strings.HasPrefix(strings.ToLower(r.Header.Get("X-Forwarded-Proto")), "https")
	}
	if isSecure {
		return CookieNameHTTPS
	}
	return CookieNameHTTP
}

// readBridgeSessionToken returns the bridge.session cookie value
// AND a presence bool indicating whether the cookie was sent at
// all (regardless of value). Plan 065 — single name, no
// scheme-variant logic; the Edge middleware that sets it always
// uses the same attributes.
//
// Codex Phase-3 review: the present-vs-absent distinction matters
// because plan §"RequireAuth logic" treats them differently.
// "Cookie absent" → fall back to JWE (covers rollout race +
// non-browser direct-to-Go clients). "Cookie present but
// empty/invalid" → 401 unconditionally (downgrade-attack defense).
// Returning just `c.Value` conflated the two cases — a
// `bridge.session=` (empty value) cookie planted by an attacker
// next to a valid JWE would have downgraded to JWE.
func readBridgeSessionToken(r *http.Request) (string, bool) {
	if c, err := r.Cookie(BridgeSessionCookie); err == nil {
		return c.Value, true
	}
	return "", false
}

// readCanonicalCookieToken returns the token from the canonical cookie for
// this request's scheme, or "" if absent. In dev, it logs a warning when a
// stale variant is present without the canonical one.
func readCanonicalCookieToken(r *http.Request) string {
	canonical := canonicalCookieName(r)
	if c, err := r.Cookie(canonical); err == nil && c.Value != "" {
		return c.Value
	}
	// Dev diagnostic: surface the stale-variant scenario.
	if os.Getenv("APP_ENV") != "production" {
		other := CookieNameHTTP
		if canonical == CookieNameHTTP {
			other = CookieNameHTTPS
		}
		if c, err := r.Cookie(other); err == nil && c.Value != "" {
			slog.Warn("stale auth cookie variant present without canonical",
				"canonical", canonical,
				"stalePresent", other,
				"path", r.URL.Path,
			)
		}
	}
	return ""
}

// writeJSONError writes a JSON error response with the given status code.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, message)
}

type contextKey string

const claimsKey contextKey = "claims"

type Middleware struct {
	Secret string

	// Plan 065 phase 1 — fields wired in for the AuthFlag-gated
	// bridge.session reader and the live-admin lookup. Unused
	// until Phase 3 changes RequireAuth to consume them; Phase 1
	// just plumbs the construction surface so cmd/api/main.go
	// is stable across phases.
	BridgeSecrets []string
	BridgeAuthOn  bool
	AdminChecker  AdminChecker
}

func NewMiddleware(secret string) *Middleware {
	return &Middleware{Secret: secret}
}

// WithBridgeSession sets the plan-065 fields on an existing
// Middleware. Returns the receiver for chaining. Safe to call
// once at startup; not safe to mutate concurrently.
func (m *Middleware) WithBridgeSession(secrets []string, internalAuthFlagOn bool, checker AdminChecker) *Middleware {
	m.BridgeSecrets = secrets
	m.BridgeAuthOn = internalAuthFlagOn
	m.AdminChecker = checker
	return m
}

// RequireAuth validates the JWT and injects claims into context.
// Also handles impersonation via bridge-impersonate cookie.
//
// When DEV_SKIP_AUTH is set, auth is bypassed and a dev user is injected.
// The env var value is the user ID to impersonate; set to "admin" for a
// platform admin, or a UUID for a specific user.
//
// Plan 065 phase 3 — verification path:
//
//  1. If BRIDGE_SESSION_AUTH=1 and bridge.session cookie is PRESENT,
//     verify it (HS256 against m.BridgeSecrets). PRESENT-AND-VALID
//     wins; PRESENT-AND-INVALID returns 401 unconditionally
//     (downgrade-attack defense — see plan §"RequireAuth logic").
//  2. Otherwise (flag off, OR bridge.session ABSENT), fall back to
//     the legacy Auth.js JWE / Bearer path. Absent ≠ invalid: this
//     covers the rollout race (Edge mint hasn't fired yet) and
//     non-browser direct-to-Go clients.
//  3. After token verification, AdminChecker overwrites
//     claims.IsPlatformAdmin from a live DB lookup. This MUST run
//     before the impersonation overlay (which gates on the live
//     admin status to decide whether to apply impersonation).
//  4. The impersonation overlay applies bridge-impersonate cookie
//     mutations as before.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dev auth bypass — never use in production. Plan 050: the
		// startup-time guard in cmd/api/main.go panics if
		// DEV_SKIP_AUTH is set with APP_ENV=production, so reaching
		// this branch in prod is impossible. The explicit
		// `ImpersonatedBy: ""` below is documentation/test hardening
		// — Go's struct-literal omission already gives the field
		// its zero value; the explicit line makes intent visible to
		// grep / future readers.
		if devUser := os.Getenv("DEV_SKIP_AUTH"); devUser != "" {
			claims := &Claims{
				UserID:          devUser,
				Name:            "Dev User",
				Email:           "dev@localhost",
				IsPlatformAdmin: devUser == "admin",
				ImpersonatedBy:  "",
			}
			// If it looks like a UUID, use it as-is. Otherwise treat "admin"
			// as a platform admin with a placeholder ID.
			if devUser == "admin" {
				claims.UserID = "00000000-0000-0000-0000-000000000001"
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		claims, status := m.resolveClaims(r)
		if status != 0 {
			writeJSONError(w, status, claimsErrorMessage(status))
			return
		}

		// Plan 065 §"Live admin via middleware injection" — overwrite
		// claims.IsPlatformAdmin from DB before any handler (or the
		// impersonation overlay) reads it. ~80 handler sites read
		// this field; the middleware-layer overwrite makes them all
		// automatically live without per-handler changes.
		m.injectLiveAdmin(r.Context(), claims)

		// Impersonation overlay runs AFTER live-admin injection so
		// the gate (claims.IsPlatformAdmin) reflects the live DB
		// value, not the JWT-carried hint.
		claims = applyImpersonationOverlay(r, claims)

		ctx := ContextWithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveClaims is the shared token-verification path used by
// RequireAuth and OptionalAuth. Returns (claims, 0) on success,
// or (nil, status) when the request must be rejected with the
// given HTTP status. The only non-zero status this currently
// returns is http.StatusUnauthorized — for both "no token at all"
// (caller decides 401 vs pass-through) and "present-but-invalid
// bridge.session" (always reject).
//
// Phase 3 verification order:
//   - If BRIDGE_SESSION_AUTH=1 AND bridge.session cookie is PRESENT
//     (regardless of value), that cookie is authoritative.
//     Valid → claims; invalid (including empty value) → 401.
//   - If the bridge.session cookie is ABSENT (no cookie sent at
//     all), fall back to legacy Authorization-header / Auth.js
//     cookie path. Covers rollout race + non-browser clients.
func (m *Middleware) resolveClaims(r *http.Request) (*Claims, int) {
	if m.BridgeAuthOn && len(m.BridgeSecrets) > 0 {
		if bridgeTok, present := readBridgeSessionToken(r); present {
			// Codex Phase-3 review: present-but-empty must ALSO
			// 401 (not fall through to JWE). VerifyBridgeSession
			// rejects an empty token, so this drops into the
			// invalid-token branch correctly.
			bsClaims, err := VerifyBridgeSession(m.BridgeSecrets, bridgeTok)
			if err != nil {
				// Present-but-invalid → 401 unconditionally. NO JWE
				// fallback (plan §"RequireAuth logic"): allowing it
				// would create a downgrade attack surface once Bridge
				// sessions become authoritative for revocation.
				slog.Warn("bridge.session present but invalid, rejecting",
					"err", err, "path", r.URL.Path)
				return nil, http.StatusUnauthorized
			}
			return bridgeClaimsToCanonical(bsClaims), 0
		}
		// Absent bridge.session → fall through to legacy path.
		// Covers rollout race (Edge mint hasn't fired) + non-browser
		// direct-to-Go clients.
	}

	// Legacy Auth.js JWE path. Authorization header is the canonical
	// proxy path; cookies are only for direct browser hits (no
	// proxy). Never combine — falling back to cookies after a header
	// was sent would mask api-client bugs and potentially re-inject
	// a stale identity from the browser jar.
	tokenStr := ""
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		tokenStr = readCanonicalCookieToken(r)
	}
	if tokenStr == "" {
		return nil, http.StatusUnauthorized
	}
	claims, err := VerifyToken(tokenStr, m.Secret)
	if err != nil {
		return nil, http.StatusUnauthorized
	}
	return claims, 0
}

// claimsErrorMessage maps the resolveClaims status to a JSON error
// message. Keeping the wording stable matches what callers and
// tests already expect.
func claimsErrorMessage(status int) string {
	if status == http.StatusUnauthorized {
		return "Unauthorized"
	}
	return http.StatusText(status)
}

// bridgeClaimsToCanonical converts a verified BridgeSessionClaims
// into the canonical Claims shape handlers consume. The cosmetic
// IsPlatformAdmin from the JWT is preserved here as the initial
// value — the live-admin injection below will overwrite it.
func bridgeClaimsToCanonical(c *BridgeSessionClaims) *Claims {
	return &Claims{
		UserID:          c.Sub,
		Email:           c.Email,
		Name:            c.Name,
		IsPlatformAdmin: c.IsPlatformAdmin,
	}
}

// injectLiveAdmin overwrites claims.IsPlatformAdmin with the live
// value from the DB (cached for 60s by CachedAdminChecker). On any
// error or absence of an AdminChecker, falls CLOSED — sets the
// claim to false. We would rather temporarily 403 a real admin
// during a DB hiccup than silently grant admin to a stale JWT.
//
// Mutates *claims in place; safe because each request gets its
// own claims instance from VerifyToken/VerifyBridgeSession.
func (m *Middleware) injectLiveAdmin(ctx context.Context, claims *Claims) {
	if m.AdminChecker == nil || claims == nil || claims.UserID == "" {
		return
	}
	isAdmin, err := m.AdminChecker.IsAdmin(ctx, claims.UserID)
	if err != nil {
		slog.Warn("live admin lookup failed, fail-closing IsPlatformAdmin",
			"err", err, "userID", claims.UserID)
		claims.IsPlatformAdmin = false
		return
	}
	claims.IsPlatformAdmin = isAdmin
}

// applyImpersonationOverlay returns either `claims` unchanged or a
// new Claims representing the impersonated target. The overlay
// gate reads claims.IsPlatformAdmin (which by Phase 3 is the live
// DB value), so a non-admin token can never trigger impersonation.
func applyImpersonationOverlay(r *http.Request, claims *Claims) *Claims {
	if claims == nil || !claims.IsPlatformAdmin {
		return claims
	}
	cookie, err := r.Cookie("bridge-impersonate")
	if err != nil {
		return claims
	}
	cookieVal := cookie.Value
	if decoded, err := url.QueryUnescape(cookieVal); err == nil {
		cookieVal = decoded
	}
	var impData ImpersonationData
	if json.Unmarshal([]byte(cookieVal), &impData) != nil {
		return claims
	}
	if impData.OriginalUserID != claims.UserID {
		return claims
	}
	return &Claims{
		UserID:          impData.TargetUserID,
		Email:           impData.TargetEmail,
		Name:            impData.TargetName,
		IsPlatformAdmin: false,
		ImpersonatedBy:  impData.OriginalUserID,
	}
}

// OptionalAuth validates the JWT if present but does not reject
// requests without one. If a valid token is found, claims are
// injected into context. Otherwise, claims are nil.
//
// Plan 065 phase 3 — shares the same verification path as
// RequireAuth. A present-but-invalid bridge.session is silently
// dropped here (no claims) rather than 401'd, since OptionalAuth's
// contract is "best effort" — handlers that mount under it are
// expected to handle missing claims.
func (m *Middleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, status := m.resolveClaims(r)
		if status != 0 || claims == nil {
			// No-op: any error is just "no claims"; OptionalAuth
			// passes through. (RequireAuth would 401 on the same
			// path; the contract divergence is by design.)
			next.ServeHTTP(w, r)
			return
		}
		m.injectLiveAdmin(r.Context(), claims)
		claims = applyImpersonationOverlay(r, claims)
		r = r.WithContext(ContextWithClaims(r.Context(), claims))
		next.ServeHTTP(w, r)
	})
}

// RequireAdminMiddleware is a method-based admin check middleware.
//
// Deprecated: use (m *Middleware).RequireAdmin directly. Kept for
// the existing wrapper-call ergonomics.
func (m *Middleware) RequireAdminMiddleware(next http.Handler) http.Handler {
	return m.RequireAdmin(next)
}

// RequireAdmin checks that the user is a platform admin (method
// version, Plan 065). Once Phase 3 lands, claims.IsPlatformAdmin
// is the live DB value because RequireAuth overwrote it before
// chaining here.
//
// Admins who are impersonating a non-admin user retain admin access
// (impersonation overlay sets ImpersonatedBy on the claims; we
// honor either signal).
func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
	return requireAdminHandler(next)
}

// RequireAdmin is the package-level alias retained for any
// external/test callsites that haven't migrated to the method
// version. Behaves identically.
//
// Deprecated: use (m *Middleware).RequireAdmin so future Phase-3+
// changes (live-admin DB lookup wiring) flow through one path.
func RequireAdmin(next http.Handler) http.Handler {
	return requireAdminHandler(next)
}

// requireAdminHandler is the shared inner implementation.
func requireAdminHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		if claims == nil {
			writeJSONError(w, http.StatusForbidden, "Platform admin required")
			return
		}
		// Allow if admin, or if impersonating (impersonator was verified as admin)
		if !claims.IsPlatformAdmin && claims.ImpersonatedBy == "" {
			writeJSONError(w, http.StatusForbidden, "Platform admin required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ContextWithClaims returns a new context with the given claims attached.
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// GetClaims retrieves the authenticated user's claims from context.
func GetClaims(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsKey).(*Claims)
	return claims
}
