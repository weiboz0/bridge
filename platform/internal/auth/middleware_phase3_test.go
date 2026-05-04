package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plan 065 Phase 3 tests for the bridge.session reader path and the
// live-admin claim injection. These cases are net-new behavior added
// in Phase 3; the Phase-1/2 cases in middleware_test.go cover the
// legacy JWE/Bearer path that remains unchanged when the
// BRIDGE_SESSION_AUTH flag is OFF.

const (
	testBridgePrimary = "phase3-primary-secret"
	testBridgeOld     = "phase3-old-secret"
)

// stubAdminChecker is a minimal AdminChecker for middleware tests.
// The result and error are programmable per-instance.
type stubAdminChecker struct {
	result bool
	err    error
	calls  int
}

func (s *stubAdminChecker) IsAdmin(_ context.Context, _ string) (bool, error) {
	s.calls++
	return s.result, s.err
}

// makeBridgeToken signs a Bridge session JWT for tests.
func makeBridgeToken(t *testing.T, secret, sub, email, name string, isAdmin bool) string {
	t.Helper()
	tok, err := SignBridgeSession(secret, sub, email, name, isAdmin, time.Hour)
	require.NoError(t, err)
	return tok
}

// withBridgeMw returns a Middleware with bridge.session reading enabled.
func withBridgeMw(t *testing.T, secrets []string, checker AdminChecker) *Middleware {
	t.Helper()
	mw := NewMiddleware(testSecret)
	mw.WithBridgeSession(secrets, true, checker)
	return mw
}

// --- bridge.session reader path ---

func TestRequireAuth_BridgeSession_PreferredOverJWE(t *testing.T) {
	// Both cookies present. With the flag ON, bridge.session wins —
	// JWE is never consulted.
	mw := withBridgeMw(t, []string{testBridgePrimary}, nil)
	bridgeTok := makeBridgeToken(t, testBridgePrimary, "bridge-user", "b@example.com", "Bridge User", false)
	jweTok := makeToken(t, jwt.MapClaims{
		"id":    "jwe-user",
		"email": "j@example.com",
		"name":  "JWE User",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var got *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClaims(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: BridgeSessionCookie, Value: bridgeTok})
	req.AddCookie(&http.Cookie{Name: CookieNameHTTP, Value: jweTok})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.Equal(t, "bridge-user", got.UserID, "bridge.session must win when both cookies present")
}

func TestRequireAuth_BridgeSession_InvalidReturns401_NoJWEFallback(t *testing.T) {
	// Plan §"RequireAuth logic": present-but-invalid bridge.session
	// MUST return 401 unconditionally, even when JWE is also present
	// and valid. Allowing JWE fallback would create a downgrade
	// attack surface once Phase 5 makes Bridge sessions
	// authoritative for revocation.
	mw := withBridgeMw(t, []string{testBridgePrimary}, nil)
	jweTok := makeToken(t, jwt.MapClaims{
		"id":    "jwe-user",
		"email": "j@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must NOT run with invalid bridge.session, even with valid JWE")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: BridgeSessionCookie, Value: "totally.malformed.token"})
	req.AddCookie(&http.Cookie{Name: CookieNameHTTP, Value: jweTok})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_BridgeSession_PresentButEmpty_Returns401(t *testing.T) {
	// Codex Phase-3 review caught this: a `bridge.session=` cookie
	// (present, empty value) planted by an attacker next to a
	// valid JWE used to fall through to JWE — the same downgrade
	// path the present-but-invalid case was supposed to defend
	// against. Empty value MUST be treated as present-and-invalid
	// → 401, not as absent → JWE fallback.
	mw := withBridgeMw(t, []string{testBridgePrimary}, nil)
	jweTok := makeToken(t, jwt.MapClaims{
		"id":    "jwe-user",
		"email": "j@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must NOT run with present-but-empty bridge.session")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: BridgeSessionCookie, Value: ""})
	req.AddCookie(&http.Cookie{Name: CookieNameHTTP, Value: jweTok})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"empty bridge.session cookie value must 401, not fall back to JWE")
}

func TestRequireAuth_BridgeSession_AbsentFallsBackToJWE(t *testing.T) {
	// No bridge.session cookie at all → JWE legacy path runs.
	// Covers rollout race (Edge mint hasn't fired yet) and
	// non-browser direct-to-Go clients.
	mw := withBridgeMw(t, []string{testBridgePrimary}, nil)
	jweTok := makeToken(t, jwt.MapClaims{
		"id":    "jwe-user",
		"email": "j@example.com",
		"name":  "JWE User",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var got *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClaims(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieNameHTTP, Value: jweTok})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.Equal(t, "jwe-user", got.UserID)
}

func TestRequireAuth_BridgeSession_FlagOff_IgnoresBridgeSession(t *testing.T) {
	// With the flag OFF, bridge.session is ignored even when
	// present. This is the rollout default — Phase 1+2 ship
	// the wiring but the flag keeps it dormant.
	mw := NewMiddleware(testSecret) // BridgeAuthOn=false, BridgeSecrets=nil
	bridgeTok := makeBridgeToken(t, testBridgePrimary, "bridge-user", "b@example.com", "B", false)
	jweTok := makeToken(t, jwt.MapClaims{
		"id":    "jwe-user",
		"email": "j@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var got *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClaims(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: BridgeSessionCookie, Value: bridgeTok})
	req.AddCookie(&http.Cookie{Name: CookieNameHTTP, Value: jweTok})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.Equal(t, "jwe-user", got.UserID, "with flag off, JWE wins regardless of bridge.session")
}

func TestRequireAuth_BridgeSession_RotationFallbackVerifies(t *testing.T) {
	// Token signed with the old secret must still verify when the
	// rotation list contains both new and old. This is the
	// cookie-survives-rotation guarantee from plan §"Secret rotation".
	mw := withBridgeMw(t, []string{testBridgePrimary, testBridgeOld}, nil)
	tok := makeBridgeToken(t, testBridgeOld, "rot-user", "r@example.com", "R", false)

	var got *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClaims(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: BridgeSessionCookie, Value: tok})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.Equal(t, "rot-user", got.UserID)
}

// --- live-admin injection ---

func TestRequireAuth_LiveAdmin_PromotesNonAdminJWTViaDB(t *testing.T) {
	// JWT says non-admin, DB says admin → handler sees admin (live
	// promotion took effect without re-sign-in).
	checker := &stubAdminChecker{result: true}
	mw := NewMiddleware(testSecret)
	mw.WithBridgeSession(nil, false, checker)

	tok := makeToken(t, jwt.MapClaims{
		"id":              "user-1",
		"email":           "u@example.com",
		"isPlatformAdmin": false,
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var got *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClaims(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/anything", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.True(t, got.IsPlatformAdmin, "DB-live admin must overwrite JWT-carried false")
	assert.Equal(t, 1, checker.calls)
}

func TestRequireAuth_LiveAdmin_DemotesAdminJWTViaDB(t *testing.T) {
	// JWT says admin, DB says non-admin → handler sees non-admin
	// (revocation took effect without re-sign-in).
	checker := &stubAdminChecker{result: false}
	mw := NewMiddleware(testSecret)
	mw.WithBridgeSession(nil, false, checker)

	tok := makeToken(t, jwt.MapClaims{
		"id":              "user-1",
		"email":           "u@example.com",
		"isPlatformAdmin": true,
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var got *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClaims(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/anything", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.False(t, got.IsPlatformAdmin, "DB-live non-admin must overwrite JWT-carried true")
}

func TestRequireAuth_LiveAdmin_DBErrorFailsClosed(t *testing.T) {
	// AdminChecker returns an error (DB outage). injectLiveAdmin
	// fails CLOSED — sets IsPlatformAdmin=false rather than
	// trusting the JWT-carried value (which could be true). We'd
	// rather temporarily 403 a real admin than silently grant.
	checker := &stubAdminChecker{err: errors.New("db down")}
	mw := NewMiddleware(testSecret)
	mw.WithBridgeSession(nil, false, checker)

	tok := makeToken(t, jwt.MapClaims{
		"id":              "user-1",
		"email":           "u@example.com",
		"isPlatformAdmin": true,
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var got *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClaims(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/anything", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.False(t, got.IsPlatformAdmin, "DB error must fail-closed to non-admin")
}

func TestRequireAuth_LiveAdmin_NoCheckerSkipsLookup(t *testing.T) {
	// Back-compat: middleware without an AdminChecker (legacy
	// construction or test fixtures) leaves the JWT-carried value
	// in place. This is what makes the existing
	// TestRequireAuth_Impersonation case pass unchanged.
	mw := NewMiddleware(testSecret)
	tok := makeToken(t, jwt.MapClaims{
		"id":              "user-1",
		"email":           "u@example.com",
		"isPlatformAdmin": true,
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var got *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClaims(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/anything", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.True(t, got.IsPlatformAdmin, "JWT value preserved when no AdminChecker is wired")
}

func TestRequireAuth_LiveAdmin_RunsBeforeImpersonationOverlay(t *testing.T) {
	// Codex pass-4 important note: the AdminChecker must run BEFORE
	// the impersonation overlay because the overlay's gate reads
	// claims.IsPlatformAdmin. Test scenario: JWT says non-admin,
	// DB says admin → impersonation cookie is present and matches.
	// Outcome: overlay applies (because live admin = true), and
	// the impersonation switches to the target.
	checker := &stubAdminChecker{result: true}
	mw := NewMiddleware(testSecret)
	mw.WithBridgeSession(nil, false, checker)

	tok := makeToken(t, jwt.MapClaims{
		"id":              "admin-1",
		"email":           "admin@example.com",
		"isPlatformAdmin": false, // STALE — DB says true
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	impData := ImpersonationData{
		OriginalUserID: "admin-1",
		TargetUserID:   "target-1",
		TargetName:     "Target",
		TargetEmail:    "target@example.com",
	}
	impJSON, _ := json.Marshal(impData)

	var got *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClaims(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.AddCookie(&http.Cookie{Name: "bridge-impersonate", Value: url.QueryEscape(string(impJSON))})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.Equal(t, "target-1", got.UserID, "live-admin must run before impersonation so a fresh-admin JWT triggers the overlay")
	assert.Equal(t, "admin-1", got.ImpersonatedBy)
	assert.False(t, got.IsPlatformAdmin, "target user is not admin during impersonation")
}

func TestRequireAuth_LiveAdmin_DemotedAdminCannotImpersonate(t *testing.T) {
	// Inverse of the above: JWT says admin, DB says non-admin
	// (admin was just demoted). The impersonation cookie should
	// NOT take effect — live-admin sets IsPlatformAdmin=false,
	// and the overlay's gate skips the cookie.
	checker := &stubAdminChecker{result: false}
	mw := NewMiddleware(testSecret)
	mw.WithBridgeSession(nil, false, checker)

	tok := makeToken(t, jwt.MapClaims{
		"id":              "ex-admin",
		"email":           "ex@example.com",
		"isPlatformAdmin": true, // STALE — DB says false
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	impData := ImpersonationData{
		OriginalUserID: "ex-admin",
		TargetUserID:   "target-1",
		TargetName:     "Target",
		TargetEmail:    "target@example.com",
	}
	impJSON, _ := json.Marshal(impData)

	var got *Claims
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClaims(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.AddCookie(&http.Cookie{Name: "bridge-impersonate", Value: url.QueryEscape(string(impJSON))})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, got)
	assert.Equal(t, "ex-admin", got.UserID, "demoted admin retains their own user id, no impersonation")
	assert.Empty(t, got.ImpersonatedBy)
	assert.False(t, got.IsPlatformAdmin)
}

// --- Phase 3 + Bridge admin gate (require-admin via the Mw method) ---

func TestRequireAdmin_LiveDemoted_403(t *testing.T) {
	// User signs in as admin, gets demoted in DB, hits an admin
	// endpoint. The Phase-3 RequireAuth → injectLiveAdmin chain
	// overwrites IsPlatformAdmin to false; mw.RequireAdmin then
	// 403s. End-to-end test of the live-revocation guarantee.
	checker := &stubAdminChecker{result: false}
	mw := NewMiddleware(testSecret)
	mw.WithBridgeSession(nil, false, checker)

	tok := makeToken(t, jwt.MapClaims{
		"id":              "ex-admin",
		"email":           "ex@example.com",
		"isPlatformAdmin": true,
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	chain := mw.RequireAuth(mw.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run for demoted admin")
	})))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireAdmin_LivePromoted_200(t *testing.T) {
	// Inverse: JWT says non-admin, DB says admin. RequireAdmin
	// should pass through.
	checker := &stubAdminChecker{result: true}
	mw := NewMiddleware(testSecret)
	mw.WithBridgeSession(nil, false, checker)

	tok := makeToken(t, jwt.MapClaims{
		"id":              "fresh-admin",
		"email":           "f@example.com",
		"isPlatformAdmin": false,
		"exp":             time.Now().Add(time.Hour).Unix(),
	}, testSecret)

	var ran bool
	chain := mw.RequireAuth(mw.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ran = true
	})))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, ran)
}
