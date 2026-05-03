package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 064 — admin parent-links CRUD tests. Auth (RequireAdmin
// middleware) is exercised via the route — tests construct a
// full chi router with the AdminHandler attached.

type adminParentLinksFixture struct {
	router      chi.Router
	parent      *store.RegisteredUser
	child       *store.RegisteredUser
	other       *store.RegisteredUser
	admin       *store.RegisteredUser
	parentLinks *store.ParentLinkStore
}

func newAdminParentLinksFixture(t *testing.T, suffix string) *adminParentLinksFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()
	users := store.NewUserStore(db)
	links := store.NewParentLinkStore(db)
	h := &AdminHandler{
		Orgs:        store.NewOrgStore(db),
		Users:       users,
		Stats:       store.NewStatsStore(db),
		ParentLinks: links,
		DB:          db,
	}

	mkUser := func(label string, isAdmin bool) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name:     "AdminLink " + label,
			Email:    "alink-" + label + "-" + uuid.NewString()[:8] + "@example.com",
			Password: "testpassword123",
		})
		require.NoError(t, err)
		if isAdmin {
			_, err = db.ExecContext(ctx, "UPDATE users SET is_platform_admin = true WHERE id = $1", u.ID)
			require.NoError(t, err)
		}
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM parent_links WHERE parent_user_id = $1 OR child_user_id = $1 OR created_by = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	parent := mkUser(suffix+"-parent", false)
	child := mkUser(suffix+"-child", false)
	other := mkUser(suffix+"-other", false)
	admin := mkUser(suffix+"-admin", true)

	r := chi.NewRouter()
	h.Routes(r)

	return &adminParentLinksFixture{
		router: r,
		parent: parent, child: child, other: other, admin: admin,
		parentLinks: links,
	}
}

// adminClaims simulates the platform-admin claims that pass
// auth.RequireAdmin.
func adminClaims(u *store.RegisteredUser) *auth.Claims {
	return &auth.Claims{UserID: u.ID, Email: u.Email, Name: u.Name, IsPlatformAdmin: true}
}

func (fx *adminParentLinksFixture) doRequest(t *testing.T, method, path string, body any, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if claims != nil {
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)
	return w
}

// --- Auth gate (RequireAdmin) ---

func TestAdminParentLinks_NonAdminDenied(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())
	nonAdmin := &auth.Claims{UserID: fx.parent.ID, Email: fx.parent.Email, Name: fx.parent.Name}

	for _, tc := range []struct {
		name, method, path string
		body               any
	}{
		{"list", http.MethodGet, "/api/admin/parent-links?parent=" + fx.parent.ID, nil},
		{"create", http.MethodPost, "/api/admin/parent-links", map[string]string{"parentUserId": fx.parent.ID, "childUserId": fx.child.ID}},
		{"revoke", http.MethodDelete, "/api/admin/parent-links/00000000-0000-0000-0000-000000000099", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := fx.doRequest(t, tc.method, tc.path, tc.body, nonAdmin)
			assert.Equal(t, http.StatusForbidden, w.Code, "non-admin must get 403 from RequireAdmin middleware")
		})
	}
}

// --- Create ---

func TestAdminParentLinks_Create_Success(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/admin/parent-links",
		map[string]string{"parentUserId": fx.parent.ID, "childUserId": fx.child.ID},
		adminClaims(fx.admin))
	assert.Equal(t, http.StatusCreated, w.Code)

	var link store.ParentLink
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &link))
	assert.Equal(t, fx.parent.ID, link.ParentUserID)
	assert.Equal(t, fx.child.ID, link.ChildUserID)
	assert.Equal(t, "active", link.Status)
	assert.Equal(t, fx.admin.ID, link.CreatedBy)
}

func TestAdminParentLinks_Create_DuplicateConflict(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())
	body := map[string]string{"parentUserId": fx.parent.ID, "childUserId": fx.child.ID}

	w := fx.doRequest(t, http.MethodPost, "/api/admin/parent-links", body, adminClaims(fx.admin))
	require.Equal(t, http.StatusCreated, w.Code)

	w = fx.doRequest(t, http.MethodPost, "/api/admin/parent-links", body, adminClaims(fx.admin))
	assert.Equal(t, http.StatusConflict, w.Code, "second active link for same pair → 409")
}

func TestAdminParentLinks_Create_RejectsSelfLink(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/admin/parent-links",
		map[string]string{"parentUserId": fx.parent.ID, "childUserId": fx.parent.ID},
		adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminParentLinks_Create_RejectsMissingFields(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodPost, "/api/admin/parent-links",
		map[string]string{"parentUserId": fx.parent.ID},
		adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- List ---

func TestAdminParentLinks_List_ByParent(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())
	ctx := context.Background()
	_, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.child.ID, fx.admin.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/admin/parent-links?parent="+fx.parent.ID, nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusOK, w.Code)

	var links []store.ParentLink
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &links))
	assert.Len(t, links, 1)
}

func TestAdminParentLinks_List_ByChild(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())
	ctx := context.Background()
	_, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.child.ID, fx.admin.ID)
	require.NoError(t, err)
	_, err = fx.parentLinks.CreateLink(ctx, fx.other.ID, fx.child.ID, fx.admin.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodGet, "/api/admin/parent-links?child="+fx.child.ID, nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusOK, w.Code)

	var links []store.ParentLink
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &links))
	assert.Len(t, links, 2, "child has 2 active parent links")
}

func TestAdminParentLinks_List_RequiresExactlyOneFilter(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())

	// Neither.
	w := fx.doRequest(t, http.MethodGet, "/api/admin/parent-links", nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Both.
	w = fx.doRequest(t, http.MethodGet,
		"/api/admin/parent-links?parent="+fx.parent.ID+"&child="+fx.child.ID,
		nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Revoke ---

func TestAdminParentLinks_Revoke_Success(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())
	ctx := context.Background()
	link, err := fx.parentLinks.CreateLink(ctx, fx.parent.ID, fx.child.ID, fx.admin.ID)
	require.NoError(t, err)

	w := fx.doRequest(t, http.MethodDelete, "/api/admin/parent-links/"+link.ID, nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusOK, w.Code)

	var revoked store.ParentLink
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &revoked))
	assert.Equal(t, "revoked", revoked.Status)
	assert.NotNil(t, revoked.RevokedAt)

	// IsParentOf flips to false post-revoke.
	ok, err := fx.parentLinks.IsParentOf(ctx, fx.parent.ID, fx.child.ID)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestAdminParentLinks_Revoke_NotFound(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodDelete,
		"/api/admin/parent-links/00000000-0000-0000-0000-000000000099",
		nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminParentLinks_Revoke_BadUUID(t *testing.T) {
	fx := newAdminParentLinksFixture(t, t.Name())
	w := fx.doRequest(t, http.MethodDelete, "/api/admin/parent-links/not-a-uuid", nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code, "ValidateUUIDParam rejects non-UUID linkID")
}
