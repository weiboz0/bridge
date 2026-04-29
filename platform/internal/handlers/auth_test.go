package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/store"
)

func TestRegister_MissingName(t *testing.T) {
	h := &AuthHandler{}
	body, _ := json.Marshal(map[string]string{
		"email": "test@example.com", "password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRegister_ShortPassword(t *testing.T) {
	h := &AuthHandler{}
	body, _ := json.Marshal(map[string]string{
		"name": "Test", "email": "test@example.com", "password": "short",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRegister_MissingEmail(t *testing.T) {
	h := &AuthHandler{}
	body, _ := json.Marshal(map[string]string{
		"name": "Test", "password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRegister_InvalidJSON(t *testing.T) {
	h := &AuthHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Plan 047 phase 3 — intendedRole persistence + validation tests.
// These were ported from the deleted tests/integration/auth-register.test.ts
// (which targeted the dead TS route at src/app/api/auth/register/route.ts;
// next.config.ts proxies /api/auth/register to Go, so the TS route was
// never reachable from the browser).

// invalidIntendedRole is rejected with 400 before the user ever hits the DB.
func TestRegister_RejectsInvalidIntendedRole(t *testing.T) {
	h := &AuthHandler{}
	body, _ := json.Marshal(map[string]any{
		"name":         "Test",
		"email":        "invalid-role@example.com",
		"password":     "password123",
		"intendedRole": "admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func registerHandlerForIntegration(t *testing.T) *AuthHandler {
	t.Helper()
	db := integrationDB(t)
	return &AuthHandler{Users: store.NewUserStore(db)}
}

// Each test inserts and cleans up its own user row.
func cleanupUserByEmail(t *testing.T, email string) {
	t.Helper()
	db := integrationDB(t)
	t.Cleanup(func() {
		ctx := context.Background()
		// auth_providers cascades from users via the FK
		db.ExecContext(ctx, "DELETE FROM users WHERE email = $1", email)
	})
}

func TestRegister_PersistsIntendedRole_Teacher(t *testing.T) {
	h := registerHandlerForIntegration(t)
	email := "intent-teacher-" + t.Name() + "@example.com"
	cleanupUserByEmail(t, email)

	body, _ := json.Marshal(map[string]any{
		"name":         "Teacher Intent",
		"email":        email,
		"password":     "password123",
		"intendedRole": "teacher",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Verify intended_role landed in the column.
	db := integrationDB(t)
	var role *string
	err := db.QueryRow("SELECT intended_role FROM users WHERE email = $1", email).Scan(&role)
	require.NoError(t, err)
	require.NotNil(t, role)
	assert.Equal(t, "teacher", *role)
}

func TestRegister_PersistsIntendedRole_Student(t *testing.T) {
	h := registerHandlerForIntegration(t)
	email := "intent-student-" + t.Name() + "@example.com"
	cleanupUserByEmail(t, email)

	body, _ := json.Marshal(map[string]any{
		"name":         "Student Intent",
		"email":        email,
		"password":     "password123",
		"intendedRole": "student",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	db := integrationDB(t)
	var role *string
	err := db.QueryRow("SELECT intended_role FROM users WHERE email = $1", email).Scan(&role)
	require.NoError(t, err)
	require.NotNil(t, role)
	assert.Equal(t, "student", *role)
}

// Missing intendedRole → user created with NULL intended_role. The
// onboarding page falls back to the role-selector menu in that case.
func TestRegister_NoIntendedRole_OK(t *testing.T) {
	h := registerHandlerForIntegration(t)
	email := "intent-none-" + t.Name() + "@example.com"
	cleanupUserByEmail(t, email)

	body, _ := json.Marshal(map[string]any{
		"name":     "No Intent",
		"email":    email,
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	db := integrationDB(t)
	var role *string
	err := db.QueryRow("SELECT intended_role FROM users WHERE email = $1", email).Scan(&role)
	require.NoError(t, err)
	assert.Nil(t, role)
}

// Empty-string intendedRole is normalized to NULL.
func TestRegister_EmptyIntendedRole_NormalizesToNull(t *testing.T) {
	h := registerHandlerForIntegration(t)
	email := "intent-empty-" + t.Name() + "@example.com"
	cleanupUserByEmail(t, email)

	body, _ := json.Marshal(map[string]any{
		"name":         "Empty Intent",
		"email":        email,
		"password":     "password123",
		"intendedRole": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	db := integrationDB(t)
	var role *string
	err := db.QueryRow("SELECT intended_role FROM users WHERE email = $1", email).Scan(&role)
	require.NoError(t, err)
	assert.Nil(t, role)
}
