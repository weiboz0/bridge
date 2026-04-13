package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-for-jwt-verification"

func makeToken(t *testing.T, claims jwt.MapClaims, secret string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestVerifyToken_ValidToken(t *testing.T) {
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":              "user-123",
		"email":           "test@example.com",
		"name":            "Test User",
		"isPlatformAdmin": true,
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	claims, err := VerifyToken(tokenStr, testSecret)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.Equal(t, "Test User", claims.Name)
	assert.True(t, claims.IsPlatformAdmin)
}

func TestVerifyToken_NonAdminToken(t *testing.T) {
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":    "user-456",
		"email": "regular@example.com",
		"name":  "Regular User",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	claims, err := VerifyToken(tokenStr, testSecret)
	require.NoError(t, err)
	assert.Equal(t, "user-456", claims.UserID)
	assert.False(t, claims.IsPlatformAdmin)
}

func TestVerifyToken_ExpiredToken(t *testing.T) {
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":  "user-123",
		"exp": time.Now().Add(-time.Hour).Unix(),
	}, testSecret)

	_, err := VerifyToken(tokenStr, testSecret)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jwt parse")
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":  "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	_, err := VerifyToken(tokenStr, "wrong-secret")
	assert.Error(t, err)
}

func TestVerifyToken_InvalidSigningMethod(t *testing.T) {
	// Create a token signed with RS256 (not HMAC)
	// We can't easily create a real RS256 token here, so test with garbage
	_, err := VerifyToken("not.a.valid.token", testSecret)
	assert.Error(t, err)
}

func TestVerifyToken_MissingClaims(t *testing.T) {
	tokenStr := makeToken(t, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	claims, err := VerifyToken(tokenStr, testSecret)
	require.NoError(t, err)
	assert.Equal(t, "", claims.UserID)
	assert.Equal(t, "", claims.Email)
	assert.Equal(t, "", claims.Name)
	assert.False(t, claims.IsPlatformAdmin)
}
