package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type DocumentHandler struct {
	Documents *store.DocumentStore
}

func (h *DocumentHandler) Routes(r chi.Router) {
	r.Route("/api/documents", func(r chi.Router) {
		r.Get("/", h.ListDocuments)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("id"))
			r.Get("/", h.GetDocument)
			r.Get("/content", h.GetDocumentContent)
		})
	})
}

// ListDocuments handles GET /api/documents?classroomId=&studentId=&sessionId=
func (h *DocumentHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	filters := store.DocumentFilters{
		OwnerID:   r.URL.Query().Get("studentId"),
		SessionID: r.URL.Query().Get("sessionId"),
	}

	// Non-admin callers are scoped to their own documents. If they passed
	// no filter (or no OwnerID), default to their user ID. This lets a
	// student list their own code with a plain GET /api/documents.
	if !claims.IsPlatformAdmin {
		if filters.OwnerID != "" && filters.OwnerID != claims.UserID {
			writeError(w, http.StatusForbidden, "Cannot view other users' documents")
			return
		}
		if filters.OwnerID == "" {
			filters.OwnerID = claims.UserID
		}
	}

	if filters.OwnerID == "" && filters.SessionID == "" {
		writeError(w, http.StatusBadRequest, "At least one filter (studentId, sessionId) is required")
		return
	}

	docs, err := h.Documents.ListDocuments(r.Context(), filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

// GetDocument handles GET /api/documents/{id}
func (h *DocumentHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	doc, err := h.Documents.GetDocument(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	if !claims.IsPlatformAdmin && doc.OwnerID != claims.UserID {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	writeJSON(w, http.StatusOK, doc)
}

// GetDocumentContent handles GET /api/documents/{id}/content
func (h *DocumentHandler) GetDocumentContent(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	doc, err := h.Documents.GetDocument(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}

	if !claims.IsPlatformAdmin && doc.OwnerID != claims.UserID {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	writeJSON(w, http.StatusOK, store.DocumentContent{
		ID:        doc.ID,
		OwnerID:   doc.OwnerID,
		Language:  doc.Language,
		PlainText: doc.PlainText,
		UpdatedAt: doc.UpdatedAt,
	})
}
