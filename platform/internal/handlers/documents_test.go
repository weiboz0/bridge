package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/weiboz0/bridge/platform/internal/auth"
)

func TestListDocuments_NoClaims(t *testing.T) {
	h := &DocumentHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/documents?classroomId=cr1", nil)
	w := httptest.NewRecorder()
	h.ListDocuments(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListDocuments_MissingFilter_PlatformAdmin(t *testing.T) {
	// Platform admins do not auto-scope, so omitting all filters is rejected.
	h := &DocumentHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	req = withClaims(req, &auth.Claims{UserID: "admin-1", IsPlatformAdmin: true})
	w := httptest.NewRecorder()
	h.ListDocuments(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListDocuments_ForbiddenOtherUser(t *testing.T) {
	// Non-admin cannot request another user's documents.
	h := &DocumentHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/documents?studentId=other-user", nil)
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.ListDocuments(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetDocument_NoClaims(t *testing.T) {
	h := &DocumentHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/documents/d1", nil)
	req = withChiParams(req, map[string]string{"id": "d1"})
	w := httptest.NewRecorder()
	h.GetDocument(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetDocumentContent_NoClaims(t *testing.T) {
	h := &DocumentHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/documents/d1/content", nil)
	req = withChiParams(req, map[string]string{"id": "d1"})
	w := httptest.NewRecorder()
	h.GetDocumentContent(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
