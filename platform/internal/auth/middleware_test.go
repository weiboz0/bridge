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
