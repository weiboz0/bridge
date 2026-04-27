package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireAuth_ValidToken(t *testing.T) {
	mw := NewMiddleware(testSecret)
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":    "user-1",
		"email": "test@example.com",
		"name":  "Test",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var gotClaims *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, gotClaims)
	assert.Equal(t, "user-1", gotClaims.UserID)
	assert.Equal(t, "test@example.com", gotClaims.Email)
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	mw := NewMiddleware(testSecret)
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	mw := NewMiddleware(testSecret)
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_Impersonation(t *testing.T) {
	mw := NewMiddleware(testSecret)

	// Admin token
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":              "admin-1",
		"email":           "admin@example.com",
		"name":            "Admin",
		"isPlatformAdmin": true,
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	impData := ImpersonationData{
		OriginalUserID: "admin-1",
		TargetUserID:   "target-1",
		TargetName:     "Target User",
		TargetEmail:    "target@example.com",
	}
	impJSON, _ := json.Marshal(impData)

	var gotClaims *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	// Set cookie via raw header to avoid Go's cookie value validation stripping quotes
	req.AddCookie(&http.Cookie{Name: "bridge-impersonate", Value: url.QueryEscape(string(impJSON))})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, gotClaims)
	assert.Equal(t, "target-1", gotClaims.UserID)
	assert.Equal(t, "target@example.com", gotClaims.Email)
	assert.Equal(t, "Target User", gotClaims.Name)
	assert.False(t, gotClaims.IsPlatformAdmin)
	assert.Equal(t, "admin-1", gotClaims.ImpersonatedBy)
}

func TestRequireAuth_ImpersonationIgnoredForNonAdmin(t *testing.T) {
	mw := NewMiddleware(testSecret)

	tokenStr := makeToken(t, jwt.MapClaims{
		"id":    "user-1",
		"email": "user@example.com",
		"name":  "Regular User",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	impData := ImpersonationData{
		OriginalUserID: "user-1",
		TargetUserID:   "target-1",
		TargetName:     "Target",
		TargetEmail:    "target@example.com",
	}
	impJSON, _ := json.Marshal(impData)

	var gotClaims *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	req.AddCookie(&http.Cookie{Name: "bridge-impersonate", Value: url.QueryEscape(string(impJSON))})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, gotClaims)
	// Should NOT impersonate — not an admin
	assert.Equal(t, "user-1", gotClaims.UserID)
}

func TestRequireAdmin_Allowed(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := ContextWithClaims(req.Context(), &Claims{UserID: "admin-1", IsPlatformAdmin: true})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAdmin_Denied_NotAdmin(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := ContextWithClaims(req.Context(), &Claims{UserID: "user-1", IsPlatformAdmin: false})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireAdmin_Denied_NoClaims(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
}

func TestRequireAdmin_AllowedWhenImpersonating(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Impersonated user is NOT admin, but the impersonator was
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := ContextWithClaims(req.Context(), &Claims{
		UserID:         "target-1",
		IsPlatformAdmin: false,
		ImpersonatedBy: "admin-1",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAuth_CanonicalCookie_HTTP(t *testing.T) {
	mw := NewMiddleware(testSecret)
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":    "user-cookie-http",
		"email": "u@example.com",
		"name":  "U",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var gotClaims *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// HTTP request → canonical cookie name is "authjs.session-token"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieNameHTTP, Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, gotClaims)
	assert.Equal(t, "user-cookie-http", gotClaims.UserID)
}

func TestRequireAuth_CanonicalCookie_HTTPS(t *testing.T) {
	// httptest.NewRequest sets RemoteAddr to 192.0.2.1:1234 — trust that
	// CIDR so X-Forwarded-Proto is honored as if from a real ingress.
	t.Setenv("TRUSTED_PROXY_CIDRS", "192.0.2.0/24")
	mw := NewMiddleware(testSecret)
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":    "user-cookie-https",
		"email": "u@example.com",
		"name":  "U",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var gotClaims *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// X-Forwarded-Proto: https → canonical cookie is __Secure-authjs.session-token
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: CookieNameHTTPS, Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, gotClaims)
	assert.Equal(t, "user-cookie-https", gotClaims.UserID)
}

// Stale variant present (secure cookie on an HTTP request) must be rejected.
// Falling back to the non-canonical cookie is what re-injected stale identity
// in review 002 — this test guards the fix.
func TestRequireAuth_StaleCookieVariant_HTTP_Rejected(t *testing.T) {
	mw := NewMiddleware(testSecret)
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":    "stale-user",
		"email": "stale@example.com",
		"name":  "Stale",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called when only stale variant is present")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// HTTP request, only the secure-name cookie present → stale; reject.
	req.AddCookie(&http.Cookie{Name: CookieNameHTTPS, Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_StaleCookieVariant_HTTPS_Rejected(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "192.0.2.0/24")
	mw := NewMiddleware(testSecret)
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":    "stale-user",
		"email": "stale@example.com",
		"name":  "Stale",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called when only stale variant is present")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	// HTTPS request (trusted proxy), only the non-secure name present → stale; reject.
	req.AddCookie(&http.Cookie{Name: CookieNameHTTP, Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// Authorization header must beat any cookie, including a stale one.
// Confirms the "header path and cookie path are disjoint" invariant.
// XFP from an untrusted source must NOT steer canonical-cookie selection.
// Without this guard, an attacker who can hit Go directly (no trusted proxy)
// could send `X-Forwarded-Proto: https` + a stale __Secure- cookie and have
// it accepted as canonical.
func TestRequireAuth_XForwardedProto_FromUntrustedSource_Ignored(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "") // no trusted proxies
	mw := NewMiddleware(testSecret)
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":    "user-1",
		"email": "u@example.com",
		"name":  "U",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called — untrusted XFP must not pick the secure cookie")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Untrusted source claims https; only the secure-name cookie present.
	// Without trust, canonical name resolves to HTTP, secure cookie is stale.
	req.RemoteAddr = "8.8.8.8:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: CookieNameHTTPS, Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_XForwardedProto_FromTrustedProxy_Honored(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.0/8")
	mw := NewMiddleware(testSecret)
	tokenStr := makeToken(t, jwt.MapClaims{
		"id":    "user-1",
		"email": "u@example.com",
		"name":  "U",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var gotClaims *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: CookieNameHTTPS, Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, gotClaims)
	assert.Equal(t, "user-1", gotClaims.UserID)
}

func TestRequireAuth_HeaderBeatsCookie(t *testing.T) {
	mw := NewMiddleware(testSecret)
	headerToken := makeToken(t, jwt.MapClaims{
		"id":    "header-user",
		"email": "header@example.com",
		"name":  "Header",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)
	cookieToken := makeToken(t, jwt.MapClaims{
		"id":    "cookie-user",
		"email": "cookie@example.com",
		"name":  "Cookie",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var gotClaims *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+headerToken)
	req.AddCookie(&http.Cookie{Name: CookieNameHTTP, Value: cookieToken})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, gotClaims)
	assert.Equal(t, "header-user", gotClaims.UserID)
}

func TestContextWithClaims_And_GetClaims(t *testing.T) {
	claims := &Claims{UserID: "user-1", Email: "test@example.com"}
	ctx := ContextWithClaims(context.Background(), claims)

	got := GetClaims(ctx)
	require.NotNil(t, got)
	assert.Equal(t, "user-1", got.UserID)
}

func TestGetClaims_EmptyContext(t *testing.T) {
	got := GetClaims(context.Background())
	assert.Nil(t, got)
}
