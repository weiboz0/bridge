package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const realtimeTestSecret = "realtime-test-secret-do-not-leak-into-production"

func TestSignAndVerifyRealtimeToken_RoundTrip(t *testing.T) {
	tok, err := SignRealtimeToken(realtimeTestSecret, "user-123", "teacher", "unit:abc-123", 5*time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	claims, err := VerifyRealtimeToken(realtimeTestSecret, tok)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.Sub)
	assert.Equal(t, "teacher", claims.Role)
	assert.Equal(t, "unit:abc-123", claims.Scope)
	assert.Equal(t, RealtimeIssuer, claims.Issuer)
	assert.WithinDuration(t, time.Now().Add(5*time.Minute), claims.ExpiresAt.Time, 5*time.Second)
}

func TestSignRealtimeToken_RejectsEmptySecret(t *testing.T) {
	_, err := SignRealtimeToken("", "u", "user", "unit:x", time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HOCUSPOCUS_TOKEN_SECRET is empty")
}

func TestSignRealtimeToken_RejectsEmptyFields(t *testing.T) {
	cases := []struct {
		name           string
		sub, role, sco string
	}{
		{"missing sub", "", "user", "unit:x"},
		{"missing role", "u", "", "unit:x"},
		{"missing scope", "u", "user", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SignRealtimeToken(realtimeTestSecret, tc.sub, tc.role, tc.sco, time.Minute)
			require.Error(t, err)
		})
	}
}

func TestSignRealtimeToken_ClampsTTL(t *testing.T) {
	// Negative TTL → clamped to 30 minutes.
	tok, err := SignRealtimeToken(realtimeTestSecret, "u", "user", "unit:x", -time.Hour)
	require.NoError(t, err)
	claims, err := VerifyRealtimeToken(realtimeTestSecret, tok)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(30*time.Minute), claims.ExpiresAt.Time, 5*time.Second)

	// > 30 min → clamped to 30 minutes.
	tok2, err := SignRealtimeToken(realtimeTestSecret, "u", "user", "unit:x", 2*time.Hour)
	require.NoError(t, err)
	claims2, err := VerifyRealtimeToken(realtimeTestSecret, tok2)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(30*time.Minute), claims2.ExpiresAt.Time, 5*time.Second)
}

func TestVerifyRealtimeToken_RejectsWrongSecret(t *testing.T) {
	tok, err := SignRealtimeToken(realtimeTestSecret, "u", "user", "unit:x", time.Minute)
	require.NoError(t, err)

	_, err = VerifyRealtimeToken("different-secret", tok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature is invalid")
}

func TestVerifyRealtimeToken_RejectsWrongIssuer(t *testing.T) {
	// Hand-mint a token with a different issuer.
	now := time.Now()
	claims := RealtimeClaims{
		Sub:   "u",
		Role:  "user",
		Scope: "unit:x",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "evil-platform",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(realtimeTestSecret))
	require.NoError(t, err)

	_, err = VerifyRealtimeToken(realtimeTestSecret, signed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrong issuer")
}

func TestVerifyRealtimeToken_RejectsExpired(t *testing.T) {
	// Hand-mint a token with exp in the past.
	now := time.Now()
	claims := RealtimeClaims{
		Sub:   "u",
		Role:  "user",
		Scope: "unit:x",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    RealtimeIssuer,
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-30 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(realtimeTestSecret))
	require.NoError(t, err)

	_, err = VerifyRealtimeToken(realtimeTestSecret, signed)
	require.Error(t, err)
}

func TestVerifyRealtimeToken_RejectsMalformed(t *testing.T) {
	_, err := VerifyRealtimeToken(realtimeTestSecret, "not-a-jwt")
	require.Error(t, err)

	_, err = VerifyRealtimeToken(realtimeTestSecret, "ey.bogus.token")
	require.Error(t, err)
}

func TestSignRealtimeToken_ProducesEyPrefix(t *testing.T) {
	// The base64url-encoded JWT header `{"alg":"HS256","typ":"JWT"}`
	// always starts with "ey". Lock this in so the token format
	// stays spec-compliant regardless of library changes.
	tok, err := SignRealtimeToken(realtimeTestSecret, "u", "user", "unit:x", time.Minute)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(tok, "ey"), "token should start with `ey` (base64url JWT header prefix)")
}
