package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

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
}

func NewMiddleware(secret string) *Middleware {
	return &Middleware{Secret: secret}
}

// RequireAuth validates the JWT and injects claims into context.
// Also handles impersonation via bridge-impersonate cookie.
//
// When DEV_SKIP_AUTH is set, auth is bypassed and a dev user is injected.
// The env var value is the user ID to impersonate; set to "admin" for a
// platform admin, or a UUID for a specific user.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dev auth bypass — never use in production
		if devUser := os.Getenv("DEV_SKIP_AUTH"); devUser != "" {
			claims := &Claims{
				UserID:          devUser,
				Name:            "Dev User",
				Email:           "dev@localhost",
				IsPlatformAdmin: devUser == "admin",
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

		// Try Authorization header first, then session cookies
		tokenStr := ""
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			// Read Auth.js session token from cookies (for proxied requests).
			// Match Auth.js cookie selection: prefer non-secure on HTTP,
			// prefer secure on HTTPS. This prevents identity mismatch when
			// both cookies exist in the browser jar.
			isSecure := r.TLS != nil || strings.HasPrefix(strings.ToLower(r.Header.Get("X-Forwarded-Proto")), "https")
			cookieOrder := []string{CookieNameHTTP, CookieNameHTTPS}
			if isSecure {
				cookieOrder = []string{CookieNameHTTPS, CookieNameHTTP}
			}
			for _, name := range cookieOrder {
				if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
					tokenStr = cookie.Value
					break
				}
			}
		}

		if tokenStr == "" {
			writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		claims, err := VerifyToken(tokenStr, m.Secret)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		// Check impersonation cookie
		if cookie, err := r.Cookie("bridge-impersonate"); err == nil && claims.IsPlatformAdmin {
			cookieVal := cookie.Value
			if decoded, err := url.QueryUnescape(cookieVal); err == nil {
				cookieVal = decoded
			}
			var impData ImpersonationData
			if json.Unmarshal([]byte(cookieVal), &impData) == nil && impData.OriginalUserID == claims.UserID {
				claims = &Claims{
					UserID:          impData.TargetUserID,
					Email:           impData.TargetEmail,
					Name:            impData.TargetName,
					IsPlatformAdmin: false,
					ImpersonatedBy:  impData.OriginalUserID,
				}
			}
		}

		ctx := ContextWithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalAuth validates the JWT if present but does not reject requests without one.
// If a valid token is found, claims are injected into context. Otherwise, claims are nil.
func (m *Middleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := ""
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			isSecure := r.TLS != nil || strings.HasPrefix(strings.ToLower(r.Header.Get("X-Forwarded-Proto")), "https")
			cookieOrder := []string{CookieNameHTTP, CookieNameHTTPS}
			if isSecure {
				cookieOrder = []string{CookieNameHTTPS, CookieNameHTTP}
			}
			for _, name := range cookieOrder {
				if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
					tokenStr = cookie.Value
					break
				}
			}
		}
		if tokenStr != "" {
			if claims, err := VerifyToken(tokenStr, m.Secret); err == nil {
				// Check impersonation
				if cookie, err := r.Cookie("bridge-impersonate"); err == nil && claims.IsPlatformAdmin {
					cookieVal := cookie.Value
					if decoded, err := url.QueryUnescape(cookieVal); err == nil {
						cookieVal = decoded
					}
					var impData ImpersonationData
					if json.Unmarshal([]byte(cookieVal), &impData) == nil && impData.OriginalUserID == claims.UserID {
						claims = &Claims{
							UserID:          impData.TargetUserID,
							Email:           impData.TargetEmail,
							Name:            impData.TargetName,
							IsPlatformAdmin: false,
							ImpersonatedBy:  impData.OriginalUserID,
						}
					}
				}
				r = r.WithContext(ContextWithClaims(r.Context(), claims))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdminMiddleware is a method-based admin check middleware.
func (m *Middleware) RequireAdminMiddleware(next http.Handler) http.Handler {
	return RequireAdmin(next)
}

// RequireAdmin checks that the user is a platform admin.
// Admins who are impersonating a non-admin user retain admin access.
func RequireAdmin(next http.Handler) http.Handler {
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
