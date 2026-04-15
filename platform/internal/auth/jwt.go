package auth

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/hkdf"
)

type Claims struct {
	UserID          string `json:"id"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	IsPlatformAdmin bool   `json:"isPlatformAdmin"`
	ImpersonatedBy  string `json:"impersonatedBy,omitempty"`
}

// ImpersonationData is the JSON structure stored in the bridge-impersonate cookie.
type ImpersonationData struct {
	OriginalUserID string `json:"originalUserId"`
	TargetUserID   string `json:"targetUserId"`
	TargetName     string `json:"targetName"`
	TargetEmail    string `json:"targetEmail"`
}

// Auth.js v5 cookie names
const (
	CookieNameHTTP  = "authjs.session-token"
	CookieNameHTTPS = "__Secure-authjs.session-token"
)

// VerifyToken attempts to verify a token, trying JWE decryption first (Auth.js v5),
// then falling back to plain HS256 JWT (for contract tests and synthetic tokens).
func VerifyToken(tokenString string, secret string) (*Claims, error) {
	// Try JWE decryption first (Auth.js v5 tokens)
	claims, err := DecryptAuthJSToken(tokenString, secret)
	if err == nil {
		return claims, nil
	}

	// Fall back to plain HS256 JWT (synthetic tokens, contract tests)
	return verifyPlainJWT(tokenString, secret)
}

// DecryptAuthJSToken decrypts an Auth.js v5 JWE session token.
// Auth.js uses dir+A256CBC-HS512 with a key derived via HKDF from the secret.
func DecryptAuthJSToken(tokenString, secret string) (*Claims, error) {
	// Try both HTTP and HTTPS cookie names as salt
	var firstErr error
	for _, salt := range []string{CookieNameHTTP, CookieNameHTTPS} {
		claims, err := decryptWithSalt(tokenString, secret, salt)
		if err == nil {
			return claims, nil
		}
		// Keep the first error that shows successful decryption but failed validation
		// (e.g., "token expired") over generic crypto errors
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

func decryptWithSalt(tokenString, secret, salt string) (*Claims, error) {
	key, err := deriveEncryptionKey(secret, salt)
	if err != nil {
		return nil, err
	}

	jwe, err := jose.ParseEncrypted(tokenString,
		[]jose.KeyAlgorithm{jose.DIRECT},
		[]jose.ContentEncryption{jose.A256CBC_HS512},
	)
	if err != nil {
		return nil, fmt.Errorf("parse JWE: %w", err)
	}

	plaintext, err := jwe.Decrypt(key)
	if err != nil {
		return nil, fmt.Errorf("decrypt JWE: %w", err)
	}

	// Parse the decrypted JSON payload
	var raw map[string]any
	if err := json.Unmarshal(plaintext, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal JWE payload: %w", err)
	}

	// Check expiration
	if exp, ok := raw["exp"].(float64); ok {
		if time.Unix(int64(exp), 0).Before(time.Now()) {
			return nil, fmt.Errorf("token expired")
		}
	}

	return extractClaims(raw), nil
}

// deriveEncryptionKey derives the 64-byte key for A256CBC-HS512 using HKDF.
// Matches Auth.js v5: hkdf("sha256", secret, salt=cookieName,
// info="Auth.js Generated Encryption Key (<salt>)", length=64)
func deriveEncryptionKey(secret, salt string) ([]byte, error) {
	info := fmt.Sprintf("Auth.js Generated Encryption Key (%s)", salt)
	hkdfReader := hkdf.New(sha256.New, []byte(secret), []byte(salt), []byte(info))
	key := make([]byte, 64)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("hkdf derive: %w", err)
	}
	return key, nil
}

// verifyPlainJWT verifies a plain HS256-signed JWT (used by contract tests).
func verifyPlainJWT(tokenString string, secret string) (*Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("jwt parse: %w", err)
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return extractClaims(mapClaims), nil
}

// extractClaims builds a Claims struct from a map of claims.
// Handles both Auth.js claims (sub, name, email) and custom Bridge claims (id, isPlatformAdmin).
func extractClaims(raw map[string]any) *Claims {
	claims := &Claims{}

	// Auth.js uses "sub" for user ID; Bridge jwt callback also sets "id"
	if v, ok := raw["id"].(string); ok {
		claims.UserID = v
	} else if v, ok := raw["sub"].(string); ok {
		claims.UserID = v
	}

	if v, ok := raw["email"].(string); ok {
		claims.Email = v
	}
	if v, ok := raw["name"].(string); ok {
		claims.Name = v
	}
	if v, ok := raw["isPlatformAdmin"].(bool); ok {
		claims.IsPlatformAdmin = v
	}

	// Handle "picture" → ignore (not needed in Claims)
	_ = strings.TrimSpace // ensure strings is used

	return claims
}
