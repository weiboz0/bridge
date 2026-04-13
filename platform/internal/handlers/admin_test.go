package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/weiboz0/bridge/platform/internal/auth"
)

func TestStopImpersonate(t *testing.T) {
	h := &AdminHandler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/impersonate", nil)
	req = withClaims(req, &auth.Claims{UserID: "admin-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.StopImpersonate(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result map[string]bool
	json.Unmarshal(w.Body.Bytes(), &result)
	assert.True(t, result["stopped"])

	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "bridge-impersonate" {
			found = true
			assert.Equal(t, -1, c.MaxAge)
		}
	}
	assert.True(t, found, "should set cookie with MaxAge -1")
}

func TestImpersonateStatus_NotImpersonating(t *testing.T) {
	h := &AdminHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/impersonate/status", nil)
	req = withClaims(req, &auth.Claims{UserID: "admin-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.ImpersonateStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	assert.Nil(t, result["impersonating"])
}

func TestStartImpersonate_MissingUserId(t *testing.T) {
	h := &AdminHandler{}
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/impersonate", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "admin-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.StartImpersonate(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
