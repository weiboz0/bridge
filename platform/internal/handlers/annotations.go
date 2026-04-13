package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type AnnotationHandler struct {
	Annotations *store.AnnotationStore
}

func (h *AnnotationHandler) Routes(r chi.Router) {
	r.Route("/api/annotations", func(r chi.Router) {
		r.Post("/", h.CreateAnnotation)
		r.Get("/", h.ListAnnotations)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("id"))
			r.Delete("/", h.DeleteAnnotation)
			r.Patch("/", h.ResolveAnnotation)
		})
	})
}

func (h *AnnotationHandler) CreateAnnotation(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body struct {
		DocumentID string `json:"documentId"`
		LineStart  string `json:"lineStart"`
		LineEnd    string `json:"lineEnd"`
		Content    string `json:"content"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.DocumentID == "" {
		writeError(w, http.StatusBadRequest, "documentId is required")
		return
	}
	if body.LineStart == "" || body.LineEnd == "" {
		writeError(w, http.StatusBadRequest, "lineStart and lineEnd are required")
		return
	}
	if body.Content == "" || len(body.Content) > 2000 {
		writeError(w, http.StatusBadRequest, "content is required (max 2000 chars)")
		return
	}

	annotation, err := h.Annotations.CreateAnnotation(r.Context(), store.CreateAnnotationInput{
		DocumentID: body.DocumentID,
		AuthorID:   claims.UserID,
		AuthorType: "teacher",
		LineStart:  body.LineStart,
		LineEnd:    body.LineEnd,
		Content:    body.Content,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create annotation")
		return
	}
	writeJSON(w, http.StatusCreated, annotation)
}

func (h *AnnotationHandler) ListAnnotations(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	documentID := r.URL.Query().Get("documentId")
	if documentID == "" {
		writeError(w, http.StatusBadRequest, "documentId query parameter is required")
		return
	}

	annotations, err := h.Annotations.ListAnnotations(r.Context(), documentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, annotations)
}

func (h *AnnotationHandler) DeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	deleted, err := h.Annotations.DeleteAnnotation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if deleted == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (h *AnnotationHandler) ResolveAnnotation(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	resolved, err := h.Annotations.ResolveAnnotation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if resolved == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	writeJSON(w, http.StatusOK, resolved)
}
