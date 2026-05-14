package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type BookHandler struct {
	Books *store.BookStore
	Orgs  *store.OrgStore
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

// canEditBook mirrors canEditChapter: platform admin for platform-scope;
// active org_admin or teacher for org-scope.
func (h *BookHandler) canEditBook(ctx context.Context, c *auth.Claims, scope string, scopeID *string) bool {
	switch scope {
	case "platform":
		return c.IsPlatformAdmin
	case "org":
		if scopeID == nil || *scopeID == "" {
			return false
		}
		if c.IsPlatformAdmin {
			return true
		}
		roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *scopeID, c.UserID)
		for _, m := range roles {
			if m.Status == "active" && (m.Role == "org_admin" || m.Role == "teacher") {
				return true
			}
		}
	}
	return false
}

// canViewBook: platform admins see all; platform-scope books visible to
// any authenticated user; org-scope books visible to active org members
// of any role.
func (h *BookHandler) canViewBook(ctx context.Context, c *auth.Claims, b *store.Book) bool {
	if c.IsPlatformAdmin {
		return true
	}
	if b.Scope == "platform" {
		return true
	}
	if b.Scope == "org" && b.ScopeID != nil {
		roles, _ := h.Orgs.GetUserRolesInOrg(ctx, *b.ScopeID, c.UserID)
		for _, m := range roles {
			if m.Status == "active" {
				return true
			}
		}
	}
	return false
}

func (h *BookHandler) CreateBook(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
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
	if !h.canEditBook(r.Context(), claims, body.Scope, body.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to create books in that scope")
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
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
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
	// Visibility-filter the results.
	visible := make([]store.Book, 0, len(books))
	for i := range books {
		if h.canViewBook(r.Context(), claims, &books[i]) {
			visible = append(visible, books[i])
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": visible})
}

func (h *BookHandler) GetBook(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
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
	if !h.canViewBook(r.Context(), claims, book) {
		// Don't leak existence — return 404 instead of 403.
		writeError(w, http.StatusNotFound, "Book not found")
		return
	}
	writeJSON(w, http.StatusOK, book)
}

func (h *BookHandler) UpdateBook(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	// Load the current row to determine scope for authz.
	existing, err := h.Books.GetBook(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if existing == nil || !h.canViewBook(r.Context(), claims, existing) {
		writeError(w, http.StatusNotFound, "Book not found")
		return
	}
	if !h.canEditBook(r.Context(), claims, existing.Scope, existing.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to edit this book")
		return
	}
	var body struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	book, err := h.Books.UpdateBook(r.Context(), id, store.UpdateBookInput{
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
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.Books.GetBook(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if existing == nil || !h.canViewBook(r.Context(), claims, existing) {
		writeError(w, http.StatusNotFound, "Book not found")
		return
	}
	if !h.canEditBook(r.Context(), claims, existing.Scope, existing.ScopeID) {
		writeError(w, http.StatusForbidden, "Not authorized to delete this book")
		return
	}
	if err := h.Books.DeleteBook(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			writeError(w, http.StatusNotFound, "Book not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
