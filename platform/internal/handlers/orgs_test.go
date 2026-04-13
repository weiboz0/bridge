package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
)

func withClaims(r *http.Request, claims *auth.Claims) *http.Request {
	return r.WithContext(auth.ContextWithClaims(r.Context(), claims))
}

func withChiParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestListUserOrgs_NoClaims(t *testing.T) {
	h := &OrgHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/orgs", nil)
	w := httptest.NewRecorder()
	h.ListUserOrgs(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateOrg_MissingName(t *testing.T) {
	h := &OrgHandler{}
	body, _ := json.Marshal(map[string]string{
		"slug": "test", "type": "school",
		"contactEmail": "a@b.com", "contactName": "Admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/orgs", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.CreateOrg(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateOrg_InvalidSlug(t *testing.T) {
	h := &OrgHandler{}
	body, _ := json.Marshal(map[string]string{
		"name": "Test", "slug": "INVALID SLUG!", "type": "school",
		"contactEmail": "a@b.com", "contactName": "Admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/orgs", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.CreateOrg(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateOrg_InvalidType(t *testing.T) {
	h := &OrgHandler{}
	body, _ := json.Marshal(map[string]string{
		"name": "Test", "slug": "test", "type": "invalid",
		"contactEmail": "a@b.com", "contactName": "Admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/orgs", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.CreateOrg(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateOrg_NoClaims(t *testing.T) {
	h := &OrgHandler{}
	body, _ := json.Marshal(map[string]string{
		"name": "Test", "slug": "test", "type": "school",
		"contactEmail": "a@b.com", "contactName": "Admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/orgs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateOrg(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetOrg_NoClaims(t *testing.T) {
	h := &OrgHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/orgs/123", nil)
	req = withChiParams(req, map[string]string{"orgID": "123"})
	w := httptest.NewRecorder()
	h.GetOrg(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAddMember_MissingEmail(t *testing.T) {
	h := &OrgHandler{}
	body, _ := json.Marshal(map[string]string{"role": "teacher"})
	req := httptest.NewRequest(http.MethodPost, "/api/orgs/123/members", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "admin-1", IsPlatformAdmin: true})
	req = withChiParams(req, map[string]string{"orgID": "123"})
	w := httptest.NewRecorder()
	h.AddMember(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAddMember_InvalidRole(t *testing.T) {
	h := &OrgHandler{}
	body, _ := json.Marshal(map[string]string{"email": "test@example.com", "role": "superadmin"})
	req := httptest.NewRequest(http.MethodPost, "/api/orgs/123/members", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "admin-1", IsPlatformAdmin: true})
	req = withChiParams(req, map[string]string{"orgID": "123"})
	w := httptest.NewRecorder()
	h.AddMember(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHelpers_WriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "value"})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var result map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, "value", result["key"])
}

func TestHelpers_WriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "bad request")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, "bad request", result["error"])
}

func TestHelpers_DecodeJSON_InvalidJSON(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("not json")))
	var dst map[string]string
	ok := decodeJSON(w, req, &dst)
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
