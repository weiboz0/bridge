package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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
// to open (e.g., `session:{sid}:user:{uid}`, `chapter:{cid}`,
// `attempt:{aid}`, `broadcast:{sid}`).
type RealtimeClaims struct {
	Sub       string `json:"sub"`                 // user id of the holder
	Role      string `json:"role"`                // "teacher" | "user" | "parent"
	Scope     string `json:"scope"`               // exact documentName
	ReadOnly  bool   `json:"readOnly"`            // server-enforced connection write permission
	SessionID string `json:"sessionId,omitempty"` // required only for canvas scopes
	jwt.RegisteredClaims
}

// SignRealtimeToken mints a short-lived HS256 JWT. `ttl` is clamped
// to (0, 30 minutes]. Returns the compact-serialized token.
func SignRealtimeToken(secret string, sub, role, scope string, ttl time.Duration) (string, error) {
	return SignRealtimeTokenWithReadOnly(secret, sub, role, scope, false, ttl)
}

// SignRealtimeTokenWithReadOnly mints a short-lived token with its
// server-enforced write decision. Existing scopes use SignRealtimeToken and
// therefore remain writable by default.
func SignRealtimeTokenWithReadOnly(secret string, sub, role, scope string, readOnly bool, ttl time.Duration) (string, error) {
	return signRealtimeToken(secret, sub, role, scope, readOnly, "", ttl)
}

// SignRealtimeCanvasToken binds a canvas token to the authoritative session
// whose lifecycle lock must be acquired before every authorization read.
func SignRealtimeCanvasToken(secret, sub, role, scope, sessionID string, readOnly bool, ttl time.Duration) (string, error) {
	return signRealtimeToken(secret, sub, role, scope, readOnly, sessionID, ttl)
}

func signRealtimeToken(secret string, sub, role, scope string, readOnly bool, sessionID string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("auth.SignRealtimeToken: HOCUSPOCUS_TOKEN_SECRET is empty")
	}
	if sub == "" || role == "" || scope == "" {
		return "", errors.New("auth.SignRealtimeToken: sub, role, scope are required")
	}
	isCanvas := strings.HasPrefix(scope, "canvas:")
	if isCanvas {
		parsed, err := uuid.Parse(sessionID)
		if err != nil || parsed.String() != sessionID {
			return "", errors.New("auth.SignRealtimeToken: canvas sessionId must be a canonical UUID")
		}
	} else if sessionID != "" {
		return "", errors.New("auth.SignRealtimeToken: sessionId is only valid for canvas scopes")
	}
	if ttl <= 0 || ttl > 30*time.Minute {
		ttl = 30 * time.Minute
	}
	now := time.Now()
	claims := RealtimeClaims{
		Sub:       sub,
		Role:      role,
		Scope:     scope,
		ReadOnly:  readOnly,
		SessionID: sessionID,
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
	isCanvas := strings.HasPrefix(claims.Scope, "canvas:")
	if isCanvas {
		parsedID, parseErr := uuid.Parse(claims.SessionID)
		if parseErr != nil || parsedID.String() != claims.SessionID {
			return nil, errors.New("auth.VerifyRealtimeToken: canvas sessionId must be a canonical UUID")
		}
	} else if claims.SessionID != "" {
		return nil, errors.New("auth.VerifyRealtimeToken: sessionId is only valid for canvas scopes")
	}
	return claims, nil
}
