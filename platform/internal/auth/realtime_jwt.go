package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Plan 053 — Hocuspocus signed connection tokens.
//
// The server mints these via `POST /api/realtime/token` after
// authenticating the caller and verifying they may access the
// requested document. The Hocuspocus Node process verifies the
// signature in `onAuthenticate` and compares `scope` byte-for-byte
// against the requested `documentName`.
//
// Signing key is `HOCUSPOCUS_TOKEN_SECRET`, deliberately separate
// from `NEXTAUTH_SECRET`: a leak of the WebSocket signing key MUST
// NOT compromise sessions, and vice versa.

// RealtimeIssuer is the `iss` claim value Bridge uses to identify
// its own tokens. Hocuspocus rejects tokens with a different issuer.
const RealtimeIssuer = "bridge-platform"

// RealtimeClaims is the JWT body for a Hocuspocus connection token.
// `Scope` is the FULL Hocuspocus documentName the caller is allowed
// to open (e.g., `session:{sid}:user:{uid}`, `unit:{uid}`,
// `attempt:{aid}`, `broadcast:{sid}`).
type RealtimeClaims struct {
	Sub   string `json:"sub"`   // user id of the holder
	Role  string `json:"role"`  // "teacher" | "user" | "parent"
	Scope string `json:"scope"` // exact documentName
	jwt.RegisteredClaims
}

// SignRealtimeToken mints a short-lived HS256 JWT. `ttl` is clamped
// to (0, 30 minutes]. Returns the compact-serialized token.
func SignRealtimeToken(secret string, sub, role, scope string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("auth.SignRealtimeToken: HOCUSPOCUS_TOKEN_SECRET is empty")
	}
	if sub == "" || role == "" || scope == "" {
		return "", errors.New("auth.SignRealtimeToken: sub, role, scope are required")
	}
	if ttl <= 0 || ttl > 30*time.Minute {
		ttl = 30 * time.Minute
	}
	now := time.Now()
	claims := RealtimeClaims{
		Sub:   sub,
		Role:  role,
		Scope: scope,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    RealtimeIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)), // clock skew
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("auth.SignRealtimeToken: %w", err)
	}
	return signed, nil
}

// VerifyRealtimeToken parses + verifies a Hocuspocus token. Returns
// the claims on success. Caller must additionally verify Scope ==
// requested documentName before granting access.
func VerifyRealtimeToken(secret, token string) (*RealtimeClaims, error) {
	if secret == "" {
		return nil, errors.New("auth.VerifyRealtimeToken: HOCUSPOCUS_TOKEN_SECRET is empty")
	}
	parsed, err := jwt.ParseWithClaims(token, &RealtimeClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*RealtimeClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("auth.VerifyRealtimeToken: invalid claims")
	}
	if claims.Issuer != RealtimeIssuer {
		return nil, fmt.Errorf("auth.VerifyRealtimeToken: wrong issuer %q", claims.Issuer)
	}
	return claims, nil
}
