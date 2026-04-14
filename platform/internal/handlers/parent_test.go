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

func TestCreateReport_MissingPeriod(t *testing.T) {
	h := &ParentHandler{}
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/parent/children/child-1/reports", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"childId": "child-1"})
	req = withClaims(req, &auth.Claims{UserID: "parent-1"})
	w := httptest.NewRecorder()
	h.CreateReport(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateReport_InvalidDateFormat(t *testing.T) {
	h := &ParentHandler{}
	body, _ := json.Marshal(map[string]string{
		"periodStart": "not-a-date",
		"periodEnd":   "2026-01-31T00:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/parent/children/child-1/reports", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"childId": "child-1"})
	req = withClaims(req, &auth.Claims{UserID: "parent-1"})
	w := httptest.NewRecorder()
	h.CreateReport(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
