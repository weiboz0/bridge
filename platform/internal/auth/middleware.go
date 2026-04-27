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

		// Authorization header is the canonical proxy path; cookies are only
		// for direct browser hits (no proxy). Never combine — falling back to
		// cookies after a header was sent would mask api-client bugs and
		// potentially re-inject a stale identity from the browser jar.
		tokenStr := ""
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			tokenStr = readCanonicalCookieToken(r)
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
			tokenStr = readCanonicalCookieToken(r)
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
