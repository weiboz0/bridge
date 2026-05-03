package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

const (
	testInternalBearer  = "test-internal-bearer-do-not-use-in-prod"
	testInternalSigning = "test-bridge-signing-do-not-use-in-prod"
)

// newSessionsHandler is a tiny constructor for the bearer/secret path
// tests that don't need a DB. The DB-backed tests build their own
// handler with a real UserStore.
func newSessionsHandler(t *testing.T, signing, bearer string) *InternalSessionsHandler {
	t.Helper()
	return &InternalSessionsHandler{
		Users:                nil, // none of the no-DB tests reach the user lookup
		PrimarySigningSecret: signing,
		InternalBearer:       bearer,
	}
}

func TestInternalSessions_503WhenSigningSecretMissing(t *testing.T) {
	h := newSessionsHandler(t, "", testInternalBearer)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer "+testInternalBearer)
	w := httptest.NewRecorder()
	h.Mint(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestInternalSessions_503WhenBearerSecretMissing(t *testing.T) {
	h := newSessionsHandler(t, testInternalSigning, "")
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	h.Mint(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestInternalSessions_401WhenBearerMissing(t *testing.T) {
	h := newSessionsHandler(t, testInternalSigning, testInternalBearer)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions",
		bytes.NewReader([]byte(`{"email":"u@example.com"}`)))
	w := httptest.NewRecorder()
	h.Mint(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestInternalSessions_401WhenBearerWrong(t *testing.T) {
	h := newSessionsHandler(t, testInternalSigning, testInternalBearer)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions",
		bytes.NewReader([]byte(`{"email":"u@example.com"}`)))
	req.Header.Set("Authorization", "Bearer wrong-bearer")
	w := httptest.NewRecorder()
	h.Mint(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestInternalSessions_BearerCheckRunsBeforeBodyParse(t *testing.T) {
	// Sending malformed JSON without a bearer must return 401 (not
	// 400). This proves the auth check is the very first gate, so
	// an unauthenticated caller can't probe payload validation.
	h := newSessionsHandler(t, testInternalSigning, testInternalBearer)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions",
		bytes.NewReader([]byte(`not-json-at-all`)))
	w := httptest.NewRecorder()
	h.Mint(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestInternalSessions_400WhenBodyMalformed(t *testing.T) {
	h := newSessionsHandler(t, testInternalSigning, testInternalBearer)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions",
		bytes.NewReader([]byte(`not-json-at-all`)))
	req.Header.Set("Authorization", "Bearer "+testInternalBearer)
	w := httptest.NewRecorder()
	h.Mint(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestInternalSessions_400WhenEmailMissing(t *testing.T) {
	h := newSessionsHandler(t, testInternalSigning, testInternalBearer)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions",
		bytes.NewReader([]byte(`{"name":"x"}`)))
	req.Header.Set("Authorization", "Bearer "+testInternalBearer)
	w := httptest.NewRecorder()
	h.Mint(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestInternalSessions_400WhenEmailInvalid(t *testing.T) {
	h := newSessionsHandler(t, testInternalSigning, testInternalBearer)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions",
		bytes.NewReader([]byte(`{"email":"not-an-email","name":"x"}`)))
	req.Header.Set("Authorization", "Bearer "+testInternalBearer)
	w := httptest.NewRecorder()
	h.Mint(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- DB-backed tests ---

func TestInternalSessions_404WhenUserMissing(t *testing.T) {
	db := integrationDB(t)
	h := &InternalSessionsHandler{
		Users:                store.NewUserStore(db),
		PrimarySigningSecret: testInternalSigning,
		InternalBearer:       testInternalBearer,
	}
	body, _ := json.Marshal(mintSessionRequest{Email: "absolutely-nobody@example.test", Name: "Nobody"})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testInternalBearer)
	w := httptest.NewRecorder()
	h.Mint(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestInternalSessions_HappyPath_NonAdmin(t *testing.T) {
	db := integrationDB(t)
	users := store.NewUserStore(db)
	ctx := context.Background()

	user, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "Mint Target",
		Email:    "mint-target@example.test",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})

	h := &InternalSessionsHandler{
		Users:                users,
		PrimarySigningSecret: testInternalSigning,
		InternalBearer:       testInternalBearer,
	}
	body, _ := json.Marshal(mintSessionRequest{Email: user.Email, Name: user.Name})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testInternalBearer)
	w := httptest.NewRecorder()
	h.Mint(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	var resp mintSessionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Token)
	require.WithinDuration(t, time.Now().Add(7*24*time.Hour), resp.ExpiresAt, time.Minute)

	claims, err := auth.VerifyBridgeSession([]string{testInternalSigning}, resp.Token)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.Sub)
	assert.Equal(t, user.Email, claims.Email)
	assert.Equal(t, user.Name, claims.Name)
	assert.False(t, claims.IsPlatformAdmin)
}

func TestInternalSessions_HappyPath_AdminFlagPropagates(t *testing.T) {
	db := integrationDB(t)
	users := store.NewUserStore(db)
	ctx := context.Background()

	user, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "Mint Admin",
		Email:    "mint-admin@example.test",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})

	_, err = db.ExecContext(ctx, "UPDATE users SET is_platform_admin = true WHERE id = $1", user.ID)
	require.NoError(t, err)

	h := &InternalSessionsHandler{
		Users:                users,
		PrimarySigningSecret: testInternalSigning,
		InternalBearer:       testInternalBearer,
	}
	body, _ := json.Marshal(mintSessionRequest{Email: user.Email, Name: user.Name})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testInternalBearer)
	w := httptest.NewRecorder()
	h.Mint(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	var resp mintSessionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	claims, err := auth.VerifyBridgeSession([]string{testInternalSigning}, resp.Token)
	require.NoError(t, err)
	assert.True(t, claims.IsPlatformAdmin, "admin flag must propagate at mint time")
	// Plan 065 §"Live admin in claims": this is just the cosmetic
	// hint. Phase 3's middleware overwrites it from a live DB
	// lookup before any handler reads it.
}

func TestInternalSessions_FallsBackToDBNameWhenBodyNameEmpty(t *testing.T) {
	db := integrationDB(t)
	users := store.NewUserStore(db)
	ctx := context.Background()

	user, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "DB Name",
		Email:    "mint-fallback-name@example.test",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})

	h := &InternalSessionsHandler{
		Users:                users,
		PrimarySigningSecret: testInternalSigning,
		InternalBearer:       testInternalBearer,
	}
	body, _ := json.Marshal(mintSessionRequest{Email: user.Email}) // no Name
	req := httptest.NewRequest(http.MethodPost, "/api/internal/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testInternalBearer)
	w := httptest.NewRecorder()
	h.Mint(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp mintSessionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	claims, err := auth.VerifyBridgeSession([]string{testInternalSigning}, resp.Token)
	require.NoError(t, err)
	assert.Equal(t, "DB Name", claims.Name, "should fall back to DB name when request omits it")
}
