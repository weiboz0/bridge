package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 065 Phase 3 — end-to-end verification that the live-admin
// DB lookup actually overwrites claims.IsPlatformAdmin before the
// admin gate runs. Stub-AdminChecker tests in
// internal/auth/middleware_phase3_test.go cover the unit-level
// invariants; this test wires a real DB-backed AdminChecker against
// real users.is_platform_admin values to prove the full chain
// behaves correctly.

const liveAdminTestJWTSecret = "phase3-live-admin-jwt-secret-do-not-use-in-prod"

func makeLiveAdminJWT(t *testing.T, userID string, jwtClaimAdmin bool) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":              userID,
		"email":           "live-admin@example.com",
		"name":            "Live Admin",
		"isPlatformAdmin": jwtClaimAdmin,
		"exp":             time.Now().Add(time.Hour).Unix(),
	})
	signed, err := tok.SignedString([]byte(liveAdminTestJWTSecret))
	require.NoError(t, err)
	return signed
}

func TestLiveAdmin_DBPromotedUserBypassesStaleNonAdminJWT(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	users := store.NewUserStore(db)

	// Create a user, then promote them in the DB.
	user, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "Live Admin Promoted",
		Email:    "live-admin-promoted@example.test",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})
	_, err = db.ExecContext(ctx, "UPDATE users SET is_platform_admin = true WHERE id = $1", user.ID)
	require.NoError(t, err)

	// Build the middleware chain with a real DB-backed AdminChecker.
	checker := auth.NewCachedAdminChecker(&auth.SQLAdminLookup{DB: db})
	mw := auth.NewMiddleware(liveAdminTestJWTSecret)
	mw.WithBridgeSession(nil, false, checker)

	// JWT claims STALE non-admin (mints from before promotion).
	tok := makeLiveAdminJWT(t, user.ID, false)

	// Stand up the AdminHandler with this Mw and hit /api/admin/stats.
	h := &AdminHandler{
		Stats: store.NewStatsStore(db),
		Mw:    mw,
	}
	r := chi.NewRouter()
	h.Routes(r)

	// Wrap with RequireAuth (the route group in cmd/api/main.go does this).
	chain := mw.RequireAuth(r)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"DB-promoted user must reach the admin endpoint despite stale non-admin JWT; body=%s",
		w.Body.String(),
	)
	// Sanity: the response body is a JSON object (the stats payload).
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
}

func TestLiveAdmin_DBDemotedUserGets403DespiteStaleAdminJWT(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	users := store.NewUserStore(db)

	// Create a user. is_platform_admin defaults to false. The JWT
	// will claim true (stale grant from before demote).
	user, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "Live Admin Demoted",
		Email:    "live-admin-demoted@example.test",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})

	// User is NOT promoted in DB.
	checker := auth.NewCachedAdminChecker(&auth.SQLAdminLookup{DB: db})
	mw := auth.NewMiddleware(liveAdminTestJWTSecret)
	mw.WithBridgeSession(nil, false, checker)

	tok := makeLiveAdminJWT(t, user.ID, true) // STALE: claims admin

	h := &AdminHandler{
		Stats: store.NewStatsStore(db),
		Mw:    mw,
	}
	r := chi.NewRouter()
	h.Routes(r)
	chain := mw.RequireAuth(r)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"DB-demoted user must be 403'd at /api/admin/* even with stale admin JWT")
}
