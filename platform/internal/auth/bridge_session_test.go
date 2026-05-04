package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testBridgeSecret    = "primary-secret-do-not-use-in-prod"
	testBridgeOldSecret = "old-secret-for-rotation-tests"
)

func TestSignBridgeSession_RoundTrip(t *testing.T) {
	tok, err := SignBridgeSession(testBridgeSecret, "user-1", "u@example.com", "User One", true, time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	claims, err := VerifyBridgeSession([]string{testBridgeSecret}, tok)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Sub)
	assert.Equal(t, "u@example.com", claims.Email)
	assert.Equal(t, "User One", claims.Name)
	assert.True(t, claims.IsPlatformAdmin)
	assert.Equal(t, BridgeSessionIssuer, claims.Issuer)
}

func TestSignBridgeSession_EmptyPrimarySecret(t *testing.T) {
	_, err := SignBridgeSession("", "u", "e@x", "n", false, time.Hour)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary secret is empty")
}

func TestSignBridgeSession_MissingRequired(t *testing.T) {
	_, err := SignBridgeSession(testBridgeSecret, "", "e@x", "n", false, time.Hour)
	require.Error(t, err)

	_, err = SignBridgeSession(testBridgeSecret, "u", "", "n", false, time.Hour)
	require.Error(t, err)
}

func TestSignBridgeSession_TTLClamp(t *testing.T) {
	// Negative or zero → max TTL.
	tok, err := SignBridgeSession(testBridgeSecret, "u", "e@x", "n", false, 0)
	require.NoError(t, err)
	claims, err := VerifyBridgeSession([]string{testBridgeSecret}, tok)
	require.NoError(t, err)
	expiresIn := time.Until(claims.ExpiresAt.Time)
	assert.InDelta(t, MaxBridgeSessionTTL, expiresIn, float64(time.Minute))

	// Excessive → max TTL.
	tok, err = SignBridgeSession(testBridgeSecret, "u", "e@x", "n", false, 100*24*time.Hour)
	require.NoError(t, err)
	claims, err = VerifyBridgeSession([]string{testBridgeSecret}, tok)
	require.NoError(t, err)
	expiresIn = time.Until(claims.ExpiresAt.Time)
	assert.InDelta(t, MaxBridgeSessionTTL, expiresIn, float64(time.Minute))
}

func TestVerifyBridgeSession_NoSecrets(t *testing.T) {
	_, err := VerifyBridgeSession([]string{}, "any.token.here")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no secrets configured")
}

func TestVerifyBridgeSession_AllEmptyEntries(t *testing.T) {
	_, err := VerifyBridgeSession([]string{"", "  ", ""}, "any.token.here")
	require.Error(t, err)
	// Either "no usable secrets" (we skipped them all) or a parse failure
	// — both are acceptable; the point is it doesn't accept the token.
}

func TestVerifyBridgeSession_WrongSecret(t *testing.T) {
	tok, err := SignBridgeSession(testBridgeSecret, "u", "e@x", "n", false, time.Hour)
	require.NoError(t, err)

	_, err = VerifyBridgeSession([]string{"some-other-secret"}, tok)
	require.Error(t, err)
}

func TestVerifyBridgeSession_Expired(t *testing.T) {
	// Sign manually with a backdated exp so we're not waiting on a clock.
	now := time.Now()
	claims := BridgeSessionClaims{
		Sub:   "u",
		Email: "e@x",
		Name:  "n",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    BridgeSessionIssuer,
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
			NotBefore: jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testBridgeSecret))
	require.NoError(t, err)

	_, err = VerifyBridgeSession([]string{testBridgeSecret}, signed)
	require.Error(t, err)
}

func TestVerifyBridgeSession_WrongIssuer(t *testing.T) {
	now := time.Now()
	claims := BridgeSessionClaims{
		Sub:   "u",
		Email: "e@x",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "not-bridge",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testBridgeSecret))
	require.NoError(t, err)

	_, err = VerifyBridgeSession([]string{testBridgeSecret}, signed)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "wrong issuer") ||
			strings.Contains(err.Error(), "not-bridge"),
		"err should reference the wrong issuer, got: %v", err,
	)
}

func TestVerifyBridgeSession_RotationOldVerifiesUnderNewList(t *testing.T) {
	// Plan 065 §"Secret rotation": a token signed with the old secret
	// must still verify when the verifier list contains both new and
	// old. This is the cookie-survives-rotation guarantee.
	signedWithOld, err := SignBridgeSession(testBridgeOldSecret, "u", "e@x", "n", false, time.Hour)
	require.NoError(t, err)

	claims, err := VerifyBridgeSession([]string{testBridgeSecret, testBridgeOldSecret}, signedWithOld)
	require.NoError(t, err)
	assert.Equal(t, "u", claims.Sub)
}

func TestVerifyBridgeSession_RotationNewFailsWithOnlyOldSecret(t *testing.T) {
	// After old secret is dropped (post-rotation cleanup), a token
	// signed with the new secret cannot be verified by an old-only
	// verifier list. This is the inverse direction — confirms the
	// list semantics are "any matches" rather than something looser.
	signedWithNew, err := SignBridgeSession(testBridgeSecret, "u", "e@x", "n", false, time.Hour)
	require.NoError(t, err)

	_, err = VerifyBridgeSession([]string{testBridgeOldSecret}, signedWithNew)
	require.Error(t, err)
}

func TestVerifyBridgeSession_RotationFirstSecretSigns(t *testing.T) {
	// SignBridgeSession ALWAYS uses its single secret arg; the
	// rotation API is just verify-side. Confirm by signing under
	// "primary" and proving the resulting token verifies.
	tok, err := SignBridgeSession(testBridgeSecret, "u", "e@x", "n", false, time.Hour)
	require.NoError(t, err)
	_, err = VerifyBridgeSession([]string{testBridgeSecret, testBridgeOldSecret}, tok)
	require.NoError(t, err)
}

func TestVerifyBridgeSession_MalformedToken(t *testing.T) {
	_, err := VerifyBridgeSession([]string{testBridgeSecret}, "not-even-a-jwt")
	require.Error(t, err)
}

func TestVerifyBridgeSession_NotBeforeSkew(t *testing.T) {
	// SignBridgeSession sets NBF to 30 seconds in the past so
	// modest clock skew between mint and verify doesn't break
	// fresh tokens. Sign and verify immediately — should succeed.
	tok, err := SignBridgeSession(testBridgeSecret, "u", "e@x", "n", false, time.Hour)
	require.NoError(t, err)

	_, err = VerifyBridgeSession([]string{testBridgeSecret}, tok)
	require.NoError(t, err)
}
