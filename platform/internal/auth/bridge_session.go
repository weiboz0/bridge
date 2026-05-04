package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Plan 065 Phase 1 — Bridge-issued session JWT.
//
// Auth.js completes Google OAuth / credentials sign-in and Next.js
// middleware lazily mints these via POST /api/internal/sessions.
// The browser then carries `bridge.session` as a cookie alongside
// Auth.js's encrypted JWE. Once Phase 3 ships and the
// BRIDGE_SESSION_AUTH flag flips, Go middleware verifies this JWT
// instead of decrypting the JWE — closing the dual-implementation
// seam that has caused plans 050, 053, and PR #103.
//
// Signing key is BRIDGE_SESSION_SECRETS, deliberately separate from
// HOCUSPOCUS_TOKEN_SECRET, BRIDGE_INTERNAL_SECRET, and
// NEXTAUTH_SECRET. Each compromise stays independently contained.
//
// The secret is parsed as a comma-separated list to enable
// zero-downtime rotation: Sign uses the first entry, Verify tries
// every entry. Operators rotate by prepending the new secret,
// waiting one cookie TTL (7 days), then dropping the old.

// BridgeSessionIssuer is the `iss` claim Bridge stamps onto its own
// session tokens. Verifiers reject other issuers.
const BridgeSessionIssuer = "bridge-platform"

// BridgeSessionClaims is the JWT body for a Bridge session cookie.
// `IsPlatformAdmin` is captured at mint time as a *cosmetic hint*
// — Phase 3's middleware overwrites it from a live DB lookup before
// any handler sees it, so a stolen-and-replayed JWT cannot retain
// admin after a DB-side demote.
type BridgeSessionClaims struct {
	Sub             string `json:"sub"`             // user id (UUID)
	Email           string `json:"email"`           // mint-time email; not authoritative
	Name            string `json:"name"`            // mint-time name; cosmetic
	IsPlatformAdmin bool   `json:"isPlatformAdmin"` // cosmetic hint — see Phase 3
	jwt.RegisteredClaims
}

// MaxBridgeSessionTTL caps how long a single mint can be valid.
// Auth.js's default session cookie is 7 days; we mirror that so a
// browser without re-mint activity for the full TTL is treated the
// same way Auth.js does.
const MaxBridgeSessionTTL = 7 * 24 * time.Hour

// SignBridgeSession mints an HS256-signed Bridge session token using
// the *first* secret in the rotation list. `ttl` is clamped to
// (0, MaxBridgeSessionTTL].
func SignBridgeSession(primarySecret string, sub, email, name string, isPlatformAdmin bool, ttl time.Duration) (string, error) {
	if primarySecret == "" {
		return "", errors.New("auth.SignBridgeSession: primary secret is empty")
	}
	if sub == "" || email == "" {
		return "", errors.New("auth.SignBridgeSession: sub and email are required")
	}
	if ttl <= 0 || ttl > MaxBridgeSessionTTL {
		ttl = MaxBridgeSessionTTL
	}
	now := time.Now()
	claims := BridgeSessionClaims{
		Sub:             sub,
		Email:           email,
		Name:            name,
		IsPlatformAdmin: isPlatformAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    BridgeSessionIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(primarySecret))
	if err != nil {
		return "", fmt.Errorf("auth.SignBridgeSession: %w", err)
	}
	return signed, nil
}

// VerifyBridgeSession parses + verifies a Bridge session token,
// trying each secret in `secrets` in order. The first secret that
// produces a valid signature wins; this is the rotation primitive.
//
// Empty token strings are rejected with an explicit error so
// callers (e.g., Phase 3 middleware) can distinguish "cookie was
// present but had an empty value" — which must 401 — from
// "cookie absent" — which the middleware handles before calling
// here.
//
// Caller must additionally reject claims with an unexpected
// `Sub` shape, but that's a domain concern outside JWT verification.
func VerifyBridgeSession(secrets []string, tokenString string) (*BridgeSessionClaims, error) {
	if len(secrets) == 0 {
		return nil, errors.New("auth.VerifyBridgeSession: no secrets configured")
	}
	if tokenString == "" {
		return nil, errors.New("auth.VerifyBridgeSession: empty token")
	}

	var lastErr error
	for _, secret := range secrets {
		if secret == "" {
			continue // skip empty entries from a sloppy comma-split
		}
		parsed, err := jwt.ParseWithClaims(tokenString, &BridgeSessionClaims{}, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(secret), nil
		})
		if err != nil {
			lastErr = err
			continue
		}
		claims, ok := parsed.Claims.(*BridgeSessionClaims)
		if !ok || !parsed.Valid {
			lastErr = errors.New("auth.VerifyBridgeSession: invalid claims")
			continue
		}
		if claims.Issuer != BridgeSessionIssuer {
			return nil, fmt.Errorf("auth.VerifyBridgeSession: wrong issuer %q", claims.Issuer)
		}
		return claims, nil
	}

	if lastErr == nil {
		return nil, errors.New("auth.VerifyBridgeSession: no usable secrets")
	}
	return nil, lastErr
}
