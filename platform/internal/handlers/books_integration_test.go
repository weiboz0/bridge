package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

type bookFixture struct {
	db     *sql.DB
	h      *BookHandler
	admin  *store.RegisteredUser
	other  *store.RegisteredUser
	orgID  string
	bookID string
}

func newBookFixture(t *testing.T) *bookFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()
	users := store.NewUserStore(db)
	orgs := store.NewOrgStore(db)
	books := store.NewBookStore(db)
	org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{Name: "Book Org " + t.Name(), Slug: "book-org-" + t.Name(), Type: "school", ContactEmail: "book-org@example.com", ContactName: "Admin"})
	require.NoError(t, err)
	admin, err := users.RegisterUser(ctx, store.RegisterInput{Name: "Admin", Email: "book-admin-" + t.Name() + "@example.com", Password: "testpassword123"})
	require.NoError(t, err)
	other, err := users.RegisterUser(ctx, store.RegisterInput{Name: "Other", Email: "book-other-" + t.Name() + "@example.com", Password: "testpassword123"})
	require.NoError(t, err)
	book, err := books.CreateBook(ctx, store.CreateBookInput{Title: "Existing", Scope: "org", ScopeID: &org.ID, CreatedBy: admin.ID})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM books WHERE created_by IN ($1, $2)", admin.ID, other.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id IN ($1, $2)", admin.ID, other.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id IN ($1, $2)", admin.ID, other.ID)
	})
	return &bookFixture{db: db, h: &BookHandler{Books: books}, admin: admin, other: other, orgID: org.ID, bookID: book.ID}
}

func (fx *bookFixture) claims(admin bool) *auth.Claims {
	u := fx.other
	if admin {
		u = fx.admin
	}
	return &auth.Claims{UserID: u.ID, Email: u.Email, Name: u.Name, IsPlatformAdmin: admin}
}

func callBook(t *testing.T, h http.HandlerFunc, method, path string, body any, claims *auth.Claims, params map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rbody *bytes.Reader
	if body == nil {
		rbody = bytes.NewReader(nil)
	} else {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rbody = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rbody)
	if claims != nil {
		req = withClaims(req, claims)
	}
	if params != nil {
		req = withChiParams(req, params)
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestBookHandler_Create_StatusPaths(t *testing.T) {
	fx := newBookFixture(t)
	body := map[string]any{"title": "Created", "scope": "org", "scopeId": fx.orgID}
	assert.Equal(t, http.StatusUnauthorized, callBook(t, fx.h.CreateBook, http.MethodPost, "/api/books", body, nil, nil).Code)
	assert.Equal(t, http.StatusForbidden, callBook(t, fx.h.CreateBook, http.MethodPost, "/api/books", body, fx.claims(false), nil).Code)
	assert.Equal(t, http.StatusBadRequest, callBook(t, fx.h.CreateBook, http.MethodPost, "/api/books", map[string]any{"title": "", "scope": "org", "scopeId": fx.orgID}, fx.claims(true), nil).Code)
	assert.Equal(t, http.StatusCreated, callBook(t, fx.h.CreateBook, http.MethodPost, "/api/books", body, fx.claims(true), nil).Code)
}

func TestBookHandler_List_StatusPaths(t *testing.T) {
	fx := newBookFixture(t)
	assert.Equal(t, http.StatusUnauthorized, callBook(t, fx.h.ListBooks, http.MethodGet, "/api/books", nil, nil, nil).Code)
	assert.Equal(t, http.StatusForbidden, callBook(t, fx.h.ListBooks, http.MethodGet, "/api/books", nil, fx.claims(false), nil).Code)
	assert.Equal(t, http.StatusOK, callBook(t, fx.h.ListBooks, http.MethodGet, "/api/books?scope=org&scopeId="+fx.orgID, nil, fx.claims(true), nil).Code)
}

func TestBookHandler_Get_StatusPaths(t *testing.T) {
	fx := newBookFixture(t)
	params := map[string]string{"id": fx.bookID}
	assert.Equal(t, http.StatusUnauthorized, callBook(t, fx.h.GetBook, http.MethodGet, "/api/books/"+fx.bookID, nil, nil, params).Code)
	assert.Equal(t, http.StatusForbidden, callBook(t, fx.h.GetBook, http.MethodGet, "/api/books/"+fx.bookID, nil, fx.claims(false), params).Code)
	assert.Equal(t, http.StatusOK, callBook(t, fx.h.GetBook, http.MethodGet, "/api/books/"+fx.bookID, nil, fx.claims(true), params).Code)
	assert.Equal(t, http.StatusNotFound, callBook(t, fx.h.GetBook, http.MethodGet, "/api/books/00000000-0000-0000-0000-000000000000", nil, fx.claims(true), map[string]string{"id": "00000000-0000-0000-0000-000000000000"}).Code)
}

func TestBookHandler_Update_StatusPaths(t *testing.T) {
	fx := newBookFixture(t)
	params := map[string]string{"id": fx.bookID}
	body := map[string]any{"title": "Updated"}
	assert.Equal(t, http.StatusUnauthorized, callBook(t, fx.h.UpdateBook, http.MethodPatch, "/api/books/"+fx.bookID, body, nil, params).Code)
	assert.Equal(t, http.StatusForbidden, callBook(t, fx.h.UpdateBook, http.MethodPatch, "/api/books/"+fx.bookID, body, fx.claims(false), params).Code)
	assert.Equal(t, http.StatusBadRequest, callBook(t, fx.h.UpdateBook, http.MethodPatch, "/api/books/"+fx.bookID, map[string]any{"title": ""}, fx.claims(true), params).Code)
	assert.Equal(t, http.StatusOK, callBook(t, fx.h.UpdateBook, http.MethodPatch, "/api/books/"+fx.bookID, body, fx.claims(true), params).Code)
}

func TestBookHandler_Delete_StatusPaths(t *testing.T) {
	fx := newBookFixture(t)
	params := map[string]string{"id": fx.bookID}
	assert.Equal(t, http.StatusUnauthorized, callBook(t, fx.h.DeleteBook, http.MethodDelete, "/api/books/"+fx.bookID, nil, nil, params).Code)
	assert.Equal(t, http.StatusForbidden, callBook(t, fx.h.DeleteBook, http.MethodDelete, "/api/books/"+fx.bookID, nil, fx.claims(false), params).Code)
	assert.Equal(t, http.StatusNoContent, callBook(t, fx.h.DeleteBook, http.MethodDelete, "/api/books/"+fx.bookID, nil, fx.claims(true), params).Code)
	assert.Equal(t, http.StatusNotFound, callBook(t, fx.h.DeleteBook, http.MethodDelete, "/api/books/"+fx.bookID, nil, fx.claims(true), params).Code)
}
