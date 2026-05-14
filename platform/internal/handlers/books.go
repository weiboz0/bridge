package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type BookHandler struct {
	Books *store.BookStore
}

func (h *BookHandler) Routes(r chi.Router) {
	r.Route("/api/books", func(r chi.Router) {
		r.Get("/", h.ListBooks)
		r.Post("/", h.CreateBook)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(ValidateUUIDParam("id"))
			r.Get("/", h.GetBook)
			r.Patch("/", h.UpdateBook)
			r.Delete("/", h.DeleteBook)
		})
	})
}

func requirePlatformAdmin(w http.ResponseWriter, r *http.Request) (*auth.Claims, bool) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return nil, false
	}
	if !claims.IsPlatformAdmin {
		writeError(w, http.StatusForbidden, "Forbidden")
		return nil, false
	}
	return claims, true
}

func (h *BookHandler) CreateBook(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePlatformAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Scope       string  `json:"scope"`
		ScopeID     *string `json:"scopeId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	book, err := h.Books.CreateBook(r.Context(), store.CreateBookInput{
		Title:       body.Title,
		Description: body.Description,
		Scope:       body.Scope,
		ScopeID:     body.ScopeID,
		CreatedBy:   claims.UserID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, book)
}

func (h *BookHandler) ListBooks(w http.ResponseWriter, r *http.Request) {
	if _, ok := requirePlatformAdmin(w, r); !ok {
		return
	}
	var scopeID *string
	if v := r.URL.Query().Get("scopeId"); v != "" {
		scopeID = &v
	}
	books, err := h.Books.ListBooks(r.Context(), store.BookFilter{
		Scope:   r.URL.Query().Get("scope"),
		ScopeID: scopeID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": books})
}

func (h *BookHandler) GetBook(w http.ResponseWriter, r *http.Request) {
	if _, ok := requirePlatformAdmin(w, r); !ok {
		return
	}
	book, err := h.Books.GetBook(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if book == nil {
		writeError(w, http.StatusNotFound, "Book not found")
		return
	}
	writeJSON(w, http.StatusOK, book)
}

func (h *BookHandler) UpdateBook(w http.ResponseWriter, r *http.Request) {
	if _, ok := requirePlatformAdmin(w, r); !ok {
		return
	}
	var body struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	book, err := h.Books.UpdateBook(r.Context(), chi.URLParam(r, "id"), store.UpdateBookInput{
		Title:       body.Title,
		Description: body.Description,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if book == nil {
		writeError(w, http.StatusNotFound, "Book not found")
		return
	}
	writeJSON(w, http.StatusOK, book)
}

func (h *BookHandler) DeleteBook(w http.ResponseWriter, r *http.Request) {
	if _, ok := requirePlatformAdmin(w, r); !ok {
		return
	}
	if err := h.Books.DeleteBook(r.Context(), chi.URLParam(r, "id")); err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			writeError(w, http.StatusNotFound, "Book not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
