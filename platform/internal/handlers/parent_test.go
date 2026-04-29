package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
)

// Plan 047 phase 2: parent reports endpoints return 501 Not Implemented
// because the auth model requires parent_links (plan 049). The
// previous tests (which exercised request validation paths) are now
// moot — control never reaches the validation code.

func TestListReports_NoClaims(t *testing.T) {
	h := &ParentHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/parent/children/child-1/reports", nil)
	req = withChiParams(req, map[string]string{"childId": "child-1"})
	w := httptest.NewRecorder()
	h.ListReports(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateReport_NoClaims(t *testing.T) {
	h := &ParentHandler{}
	body, _ := json.Marshal(map[string]string{
		"periodStart": "2026-01-01T00:00:00Z",
		"periodEnd":   "2026-01-31T00:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/parent/children/child-1/reports", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"childId": "child-1"})
	w := httptest.NewRecorder()
	h.CreateReport(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// Authenticated callers — even platform admins — get 501 because the
// gate (parent_links) doesn't exist yet. Plan 049 will replace this
// with proper authorization.
func TestListReports_Authenticated_NotImplemented(t *testing.T) {
	h := &ParentHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/parent/children/child-1/reports", nil)
	req = withChiParams(req, map[string]string{"childId": "child-1"})
	req = withClaims(req, &auth.Claims{UserID: "any-user", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.ListReports(w, req)
	assert.Equal(t, http.StatusNotImplemented, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "not_implemented", body["code"])
}

func TestCreateReport_Authenticated_NotImplemented(t *testing.T) {
	h := &ParentHandler{}
	body, _ := json.Marshal(map[string]string{
		"periodStart": "2026-01-01T00:00:00Z",
		"periodEnd":   "2026-01-31T00:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/parent/children/child-1/reports", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"childId": "child-1"})
	req = withClaims(req, &auth.Claims{UserID: "any-user", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.CreateReport(w, req)
	assert.Equal(t, http.StatusNotImplemented, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "not_implemented", resp["code"])
}

// Cross-org isolation regression check: even before plan 047 the
// authorized path was broken (any caller could read any child's
// reports). The 501 return makes that issue moot, but we verify a
// caller with a different UserID still gets 501, not 200 with data.
func TestListReports_DifferentCaller_StillNotImplemented(t *testing.T) {
	h := &ParentHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/parent/children/child-A/reports", nil)
	req = withChiParams(req, map[string]string{"childId": "child-A"})
	req = withClaims(req, &auth.Claims{UserID: "unrelated-user-Z"})
	w := httptest.NewRecorder()
	h.ListReports(w, req)
	assert.Equal(t, http.StatusNotImplemented, w.Code)
}
