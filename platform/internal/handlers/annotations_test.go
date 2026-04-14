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

func TestCreateAnnotation_NoClaims(t *testing.T) {
	h := &AnnotationHandler{}
	body, _ := json.Marshal(map[string]string{"documentId": "d1", "lineStart": "1", "lineEnd": "5", "content": "Fix this"})
	req := httptest.NewRequest(http.MethodPost, "/api/annotations", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateAnnotation(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateAnnotation_MissingDocumentId(t *testing.T) {
	h := &AnnotationHandler{}
	body, _ := json.Marshal(map[string]string{"lineStart": "1", "lineEnd": "5", "content": "Fix this"})
	req := httptest.NewRequest(http.MethodPost, "/api/annotations", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.CreateAnnotation(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateAnnotation_MissingLines(t *testing.T) {
	h := &AnnotationHandler{}
	body, _ := json.Marshal(map[string]string{"documentId": "d1", "content": "Fix this"})
	req := httptest.NewRequest(http.MethodPost, "/api/annotations", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.CreateAnnotation(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateAnnotation_EmptyContent(t *testing.T) {
	h := &AnnotationHandler{}
	body, _ := json.Marshal(map[string]string{"documentId": "d1", "lineStart": "1", "lineEnd": "5"})
	req := httptest.NewRequest(http.MethodPost, "/api/annotations", bytes.NewReader(body))
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.CreateAnnotation(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListAnnotations_NoClaims(t *testing.T) {
	h := &AnnotationHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/annotations?documentId=d1", nil)
	w := httptest.NewRecorder()
	h.ListAnnotations(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListAnnotations_MissingDocumentId(t *testing.T) {
	h := &AnnotationHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/annotations", nil)
	req = withClaims(req, &auth.Claims{UserID: "user-1"})
	w := httptest.NewRecorder()
	h.ListAnnotations(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteAnnotation_NoClaims(t *testing.T) {
	h := &AnnotationHandler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/annotations/a1", nil)
	req = withChiParams(req, map[string]string{"id": "a1"})
	w := httptest.NewRecorder()
	h.DeleteAnnotation(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestResolveAnnotation_NoClaims(t *testing.T) {
	h := &AnnotationHandler{}
	req := httptest.NewRequest(http.MethodPatch, "/api/annotations/a1", nil)
	req = withChiParams(req, map[string]string{"id": "a1"})
	w := httptest.NewRecorder()
	h.ResolveAnnotation(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
