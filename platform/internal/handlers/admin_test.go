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

type recordingAdminChecker struct {
	purged []string
}

func (r *recordingAdminChecker) IsAdmin(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (r *recordingAdminChecker) AdminAndStatus(_ context.Context, _ string) (bool, string, error) {
	return true, "active", nil
}

func (r *recordingAdminChecker) Purge(userID string) {
	r.purged = append(r.purged, userID)
}

type adminUsersFixture struct {
	router  chi.Router
	users   *store.UserStore
	orgs    *store.OrgStore
	checker *recordingAdminChecker
	admin   *store.RegisteredUser
	target  *store.RegisteredUser
	other   *store.RegisteredUser
}

func newAdminUsersFixture(t *testing.T) *adminUsersFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()
	users := store.NewUserStore(db)
	orgs := store.NewOrgStore(db)
	checker := &recordingAdminChecker{}
	mw := auth.NewMiddleware("admin-users-test-secret")
	mw.WithBridgeSession(nil, false, checker)
	h := &AdminHandler{
		Orgs:  orgs,
		Users: users,
		Stats: store.NewStatsStore(db),
		Mw:    mw,
	}

	mkUser := func(label string, isAdmin bool) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name:     "Admin Users " + label,
			Email:    "admin-users-" + label + "-" + uuid.NewString()[:8] + "@example.com",
			Password: "testpassword123",
		})
		require.NoError(t, err)
		if isAdmin {
			_, err = db.ExecContext(ctx, "UPDATE users SET is_platform_admin = true WHERE id = $1", u.ID)
			require.NoError(t, err)
		}
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	admin := mkUser("admin", true)
	target := mkUser("target", false)
	other := mkUser("other", false)

	r := chi.NewRouter()
	h.Routes(r)
	return &adminUsersFixture{
		router: r, users: users, orgs: orgs, checker: checker,
		admin: admin, target: target, other: other,
	}
}

func (fx *adminUsersFixture) doRequest(t *testing.T, method, path string, body any, claims *auth.Claims) *httptest.ResponseRecorder {
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

func TestAdminUsers_ListAllUsers_FiltersAndValidation(t *testing.T) {
	fx := newAdminUsersFixture(t)
	w := fx.doRequest(t, http.MethodGet, "/api/admin/users?role=platform_admin", nil, adminClaims(fx.admin))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var users []store.AdminUser
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &users))
	assert.Contains(t, adminUserIDsForHandler(users), fx.admin.ID)

	w = fx.doRequest(t, http.MethodGet, "/api/admin/users?role=bogus", nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = fx.doRequest(t, http.MethodGet, "/api/admin/users?orgId=not-a-uuid", nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminUsers_GetAdminUser(t *testing.T) {
	fx := newAdminUsersFixture(t)
	w := fx.doRequest(t, http.MethodGet, "/api/admin/users/"+fx.target.ID, nil, adminClaims(fx.admin))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var user store.AdminUser
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &user))
	assert.Equal(t, fx.target.ID, user.ID)
	assert.Equal(t, "active", user.Status)
	assert.True(t, user.HasPassword)

	w = fx.doRequest(t, http.MethodGet, "/api/admin/users/not-a-uuid", nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = fx.doRequest(t, http.MethodGet, "/api/admin/users/00000000-0000-0000-0000-000000000000", nil, adminClaims(fx.admin))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminUsers_UpdateUserStatus(t *testing.T) {
	fx := newAdminUsersFixture(t)
	w := fx.doRequest(t, http.MethodPatch, "/api/admin/users/"+fx.target.ID+"/status",
		map[string]string{"status": "suspended"},
		adminClaims(fx.admin))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, fx.checker.purged, fx.target.ID)

	var user store.AdminUser
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &user))
	assert.Equal(t, fx.target.ID, user.ID)
	assert.Equal(t, "suspended", user.Status)
	other, err := fx.users.GetUserByID(context.Background(), fx.other.ID)
	require.NoError(t, err)
	require.NotNil(t, other)
	assert.Equal(t, "active", other.Status, "status update must not affect other users")

	w = fx.doRequest(t, http.MethodPatch, "/api/admin/users/"+fx.target.ID+"/status",
		map[string]string{"status": "deleted"},
		adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = fx.doRequest(t, http.MethodPatch, "/api/admin/users/"+fx.admin.ID+"/status",
		map[string]string{"status": "suspended"},
		adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminUsers_UpdateUserPlatformAdmin(t *testing.T) {
	fx := newAdminUsersFixture(t)
	w := fx.doRequest(t, http.MethodPatch, "/api/admin/users/"+fx.target.ID+"/platform-admin",
		map[string]bool{"isPlatformAdmin": true},
		adminClaims(fx.admin))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, fx.checker.purged, fx.target.ID)

	var user store.AdminUser
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &user))
	assert.Equal(t, fx.target.ID, user.ID)
	assert.True(t, user.IsPlatformAdmin)
	other, err := fx.users.GetUserByID(context.Background(), fx.other.ID)
	require.NoError(t, err)
	require.NotNil(t, other)
	assert.False(t, other.IsPlatformAdmin, "platform-admin update must not affect other users")

	w = fx.doRequest(t, http.MethodPatch, "/api/admin/users/"+fx.admin.ID+"/platform-admin",
		map[string]bool{"isPlatformAdmin": false},
		adminClaims(fx.admin))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminUsers_AuthGate(t *testing.T) {
	fx := newAdminUsersFixture(t)
	w := fx.doRequest(t, http.MethodGet, "/api/admin/users/"+fx.target.ID, nil, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w = fx.doRequest(t, http.MethodPatch, "/api/admin/users/"+fx.target.ID+"/status",
		map[string]string{"status": "suspended"},
		&auth.Claims{UserID: fx.other.ID, Email: fx.other.Email, Name: fx.other.Name, IsPlatformAdmin: false})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func adminUserIDsForHandler(users []store.AdminUser) map[string]bool {
	ids := make(map[string]bool, len(users))
	for _, u := range users {
		ids[u.ID] = true
	}
	return ids
}
