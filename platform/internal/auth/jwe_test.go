package auth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWESecret = "test-secret-for-jwe-decryption-testing"

// createTestJWE creates a JWE token matching Auth.js v5 format for testing.
func createTestJWE(t *testing.T, claims map[string]any, secret, salt string) string {
	t.Helper()

	key, err := deriveEncryptionKey(secret, salt)
	require.NoError(t, err)

	payload, err := json.Marshal(claims)
	require.NoError(t, err)

	encrypter, err := jose.NewEncrypter(
		jose.A256CBC_HS512,
		jose.Recipient{Algorithm: jose.DIRECT, Key: key},
		(&jose.EncrypterOptions{}).WithContentType("JWT"),
	)
	require.NoError(t, err)

	jwe, err := encrypter.Encrypt(payload)
	require.NoError(t, err)

	serialized, err := jwe.CompactSerialize()
	require.NoError(t, err)

	return serialized
}

func TestDeriveEncryptionKey(t *testing.T) {
	key, err := deriveEncryptionKey("my-secret", "authjs.session-token")
	require.NoError(t, err)
	assert.Len(t, key, 64) // A256CBC-HS512 needs 64 bytes

	// Same inputs should produce same key
	key2, err := deriveEncryptionKey("my-secret", "authjs.session-token")
	require.NoError(t, err)
	assert.Equal(t, key, key2)

	// Different salt should produce different key
	key3, err := deriveEncryptionKey("my-secret", "__Secure-authjs.session-token")
	require.NoError(t, err)
	assert.NotEqual(t, key, key3)
}

func TestDecryptAuthJSToken_ValidToken(t *testing.T) {
	token := createTestJWE(t, map[string]any{
		"id":              "user-123",
		"email":           "test@example.com",
		"name":            "Test User",
		"isPlatformAdmin": true,
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testJWESecret, CookieNameHTTP)

	claims, err := DecryptAuthJSToken(token, testJWESecret)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.Equal(t, "Test User", claims.Name)
	assert.True(t, claims.IsPlatformAdmin)
}

func TestDecryptAuthJSToken_SubClaim(t *testing.T) {
	// Auth.js uses "sub" for user ID when "id" is not set
	token := createTestJWE(t, map[string]any{
		"sub":   "user-456",
		"email": "sub@example.com",
		"name":  "Sub User",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testJWESecret, CookieNameHTTP)

	claims, err := DecryptAuthJSToken(token, testJWESecret)
	require.NoError(t, err)
	assert.Equal(t, "user-456", claims.UserID)
}

func TestDecryptAuthJSToken_ExpiredToken(t *testing.T) {
	token := createTestJWE(t, map[string]any{
		"id":  "user-123",
		"exp": time.Now().Add(-time.Hour).Unix(),
	}, testJWESecret, CookieNameHTTP)

	_, err := DecryptAuthJSToken(token, testJWESecret)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestDecryptAuthJSToken_WrongSecret(t *testing.T) {
	token := createTestJWE(t, map[string]any{
		"id":  "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
	}, testJWESecret, CookieNameHTTP)

	_, err := DecryptAuthJSToken(token, "wrong-secret")
	assert.Error(t, err)
}

func TestDecryptAuthJSToken_HTTPSCookie(t *testing.T) {
	// Token encrypted with HTTPS salt should still be decryptable
	token := createTestJWE(t, map[string]any{
		"id":    "user-789",
		"email": "https@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testJWESecret, CookieNameHTTPS)

	claims, err := DecryptAuthJSToken(token, testJWESecret)
	require.NoError(t, err)
	assert.Equal(t, "user-789", claims.UserID)
}

func TestDecryptAuthJSToken_InvalidToken(t *testing.T) {
	_, err := DecryptAuthJSToken("not-a-jwe-token", testJWESecret)
	assert.Error(t, err)
}

func TestVerifyToken_JWEFirst(t *testing.T) {
	// VerifyToken should try JWE first and succeed
	token := createTestJWE(t, map[string]any{
		"id":    "jwe-user",
		"email": "jwe@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testJWESecret, CookieNameHTTP)

	claims, err := VerifyToken(token, testJWESecret)
	require.NoError(t, err)
	assert.Equal(t, "jwe-user", claims.UserID)
}

func TestVerifyToken_FallbackToPlainJWT(t *testing.T) {
	// Plain HS256 JWT should still work (backward compatibility)
	token := makeToken(t, jwt.MapClaims{
		"id":    "plain-user",
		"email": "plain@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testJWESecret)

	claims, err := VerifyToken(token, testJWESecret)
	require.NoError(t, err)
	assert.Equal(t, "plain-user", claims.UserID)
}
