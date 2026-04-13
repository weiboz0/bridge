package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
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
