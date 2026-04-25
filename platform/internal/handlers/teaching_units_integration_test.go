package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// unitFixture is the world a TeachingUnit integration test runs against:
// two orgs, a handful of users wired into them in various roles, and a
// fully-built TeachingUnitHandler.
type unitFixture struct {
	sqlDB    *sql.DB
	h        *TeachingUnitHandler
	org1     *store.Org
	org2     *store.Org
	admin    *store.RegisteredUser // platform admin
	teacher1 *store.RegisteredUser // org1 teacher
	student1 *store.RegisteredUser // org1 student
	teacher2 *store.RegisteredUser // org2 teacher
	outsider *store.RegisteredUser // no orgs
}

// newUnitFixture builds a clean-slate handler + users + orgs.
func newUnitFixture(t *testing.T, suffix string) *unitFixture {
	t.Helper()
	db := integrationDB(t) // reuses helper from problems_integration_test.go
	ctx := context.Background()

	orgs := store.NewOrgStore(db)
	users := store.NewUserStore(db)

	h := &TeachingUnitHandler{
		Units: store.NewTeachingUnitStore(db),
		Orgs:  orgs,
	}

	mkOrg := func(label string) *store.Org {
		org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
			Name:         "UnitOrg " + label,
			Slug:         "unit-org-" + label,
			Type:         "school",
			ContactEmail: "unit-" + label + "@example.com",
			ContactName:  "Admin " + label,
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
			db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
		})
		return org
	}
	mkUser := func(label string) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name:     "UnitUser " + label,
			Email:    "unit-" + label + "@example.com",
			Password: "testpassword123",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM unit_revisions WHERE unit_id IN (SELECT id FROM teaching_units WHERE created_by = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM unit_revisions WHERE created_by = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM unit_documents WHERE unit_id IN (SELECT id FROM teaching_units WHERE created_by = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM teaching_units WHERE created_by = $1 OR scope_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	fx := &unitFixture{sqlDB: db, h: h}
	fx.org1 = mkOrg(suffix + "-1")
	fx.org2 = mkOrg(suffix + "-2")
	fx.admin = mkUser(suffix + "-admin")
	fx.teacher1 = mkUser(suffix + "-teacher1")
	fx.student1 = mkUser(suffix + "-student1")
	fx.teacher2 = mkUser(suffix + "-teacher2")
	fx.outsider = mkUser(suffix + "-outsider")

	addMember := func(org *store.Org, userID, role string) {
		_, err := orgs.AddOrgMember(ctx, store.AddMemberInput{
			OrgID: org.ID, UserID: userID, Role: role, Status: "active",
		})
		require.NoError(t, err)
	}
	addMember(fx.org1, fx.teacher1.ID, "teacher")
	addMember(fx.org1, fx.student1.ID, "student")
	addMember(fx.org2, fx.teacher2.ID, "teacher")

	return fx
}

func (fx *unitFixture) claims(u *store.RegisteredUser, isPlatformAdmin bool) *auth.Claims {
	return &auth.Claims{UserID: u.ID, Email: u.Email, Name: u.Name, IsPlatformAdmin: isPlatformAdmin}
}

// mkUnit creates a teaching unit directly via the store (bypassing the handler)
// and registers a cleanup. Status defaults to "draft".
func (fx *unitFixture) mkUnit(t *testing.T, scope string, scopeID *string, status, title string, createdBy string) *store.TeachingUnit {
	t.Helper()
	ctx := context.Background()
	u, err := fx.h.Units.CreateUnit(ctx, store.CreateTeachingUnitInput{
		Scope:     scope,
		ScopeID:   scopeID,
		Title:     title,
		Status:    status,
		CreatedBy: createdBy,
	})
	require.NoError(t, err)
	require.NotNil(t, u)
	t.Cleanup(func() {
		fx.sqlDB.ExecContext(ctx, "DELETE FROM unit_revisions WHERE unit_id = $1", u.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM unit_documents WHERE unit_id = $1", u.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", u.ID)
	})
	return u
}

// -------------------- HTTP helpers --------------------

func doUnitGet(t *testing.T, h http.HandlerFunc, path string, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
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

func doUnitPost(t *testing.T, h http.HandlerFunc, path string, body any, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	if claims != nil {
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func doUnitPostWithParams(t *testing.T, h http.HandlerFunc, path string, body any, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
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

func doUnitPatch(t *testing.T, h http.HandlerFunc, path string, body any, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(b))
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

func doUnitDelete(t *testing.T, h http.HandlerFunc, path string, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
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

func doUnitPut(t *testing.T, h http.HandlerFunc, path string, body any, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
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

// ==================== CreateUnit ====================

func TestTeachingUnitHandler_Create_PlatformAdmin_201(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	body := map[string]any{"scope": "platform", "title": "Global Unit"}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusCreated, w.Code)
	var resp store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Global Unit", resp.Title)
	assert.Equal(t, "draft", resp.Status)
	if resp.ID != "" {
		ctx := context.Background()
		fx.sqlDB.ExecContext(ctx, "DELETE FROM unit_documents WHERE unit_id = $1", resp.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", resp.ID)
	}
}

func TestTeachingUnitHandler_Create_OrgTeacher_201(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	body := map[string]any{"scope": "org", "scopeId": fx.org1.ID, "title": "Org Unit"}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusCreated, w.Code)
	var resp store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Org Unit", resp.Title)
	if resp.ID != "" {
		ctx := context.Background()
		fx.sqlDB.ExecContext(ctx, "DELETE FROM unit_documents WHERE unit_id = $1", resp.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", resp.ID)
	}
}

func TestTeachingUnitHandler_Create_OrgStudent_403(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	body := map[string]any{"scope": "org", "scopeId": fx.org1.ID, "title": "Org Unit"}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestTeachingUnitHandler_Create_OrgTeacher_OtherOrg_403(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	// teacher1 is in org1, tries to create in org2.
	body := map[string]any{"scope": "org", "scopeId": fx.org2.ID, "title": "Org2 Unit"}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestTeachingUnitHandler_Create_Personal_Self_201(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	body := map[string]any{"scope": "personal", "scopeId": fx.outsider.ID, "title": "My Unit"}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusCreated, w.Code)
	var resp store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	if resp.ID != "" {
		ctx := context.Background()
		fx.sqlDB.ExecContext(ctx, "DELETE FROM unit_documents WHERE unit_id = $1", resp.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", resp.ID)
	}
}

func TestTeachingUnitHandler_Create_Personal_OtherUser_403(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	body := map[string]any{"scope": "personal", "scopeId": fx.teacher1.ID, "title": "Not Mine"}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestTeachingUnitHandler_Create_Platform_NonAdmin_403(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	body := map[string]any{"scope": "platform", "title": "Global Unit"}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestTeachingUnitHandler_Create_Platform_WithScopeID_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	body := map[string]any{"scope": "platform", "scopeId": fx.org1.ID, "title": "Bad"}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTeachingUnitHandler_Create_EmptyTitle_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	body := map[string]any{"scope": "personal", "scopeId": fx.outsider.ID, "title": ""}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTeachingUnitHandler_Create_InvalidScope_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	body := map[string]any{"scope": "galaxy", "title": "Bad Scope"}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTeachingUnitHandler_Create_NoAuth_401(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	body := map[string]any{"scope": "personal", "scopeId": fx.outsider.ID, "title": "No auth"}
	w := doUnitPost(t, fx.h.CreateUnit, "/api/units", body, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== GetUnit ====================

func TestTeachingUnitHandler_Get_OrgTeacher_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Org Draft", fx.teacher1.ID)
	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeachingUnitHandler_Get_OrgStudent_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "classroom_ready", "Ready Unit", fx.teacher1.ID)
	// Plan-031 narrowing: org students are denied regardless of status.
	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Get_OrgDraft_OtherOrgTeacher_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Org1 Draft", fx.teacher1.ID)
	// teacher2 is in org2, not org1.
	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Get_PlatformAdmin_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Admin View", fx.teacher1.ID)
	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeachingUnitHandler_Get_Personal_Owner_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	uid := fx.outsider.ID
	u := fx.mkUnit(t, "personal", &uid, "draft", "My Unit", fx.outsider.ID)
	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeachingUnitHandler_Get_Personal_NonOwner_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	uid := fx.outsider.ID
	u := fx.mkUnit(t, "personal", &uid, "draft", "My Unit", fx.outsider.ID)
	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Get_Platform_ClassroomReady_AnyAuth_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "platform", nil, "classroom_ready", "Global Ready", fx.admin.ID)
	for _, c := range []*auth.Claims{
		fx.claims(fx.teacher1, false),
		fx.claims(fx.student1, false),
		fx.claims(fx.outsider, false),
	} {
		w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, c)
		assert.Equal(t, http.StatusOK, w.Code, "user %s should see platform classroom_ready unit", c.Email)
	}
}

func TestTeachingUnitHandler_Get_Platform_Draft_NonAdmin_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "platform", nil, "draft", "Platform Draft", fx.admin.ID)
	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Get_NotFound_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	fakeID := "00000000-0000-0000-0000-000000000001"
	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+fakeID, map[string]string{"id": fakeID}, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== UpdateUnit ====================

func TestTeachingUnitHandler_Update_OrgTeacher_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Original", fx.teacher1.ID)
	w := doUnitPatch(t, fx.h.UpdateUnit, "/api/units/"+u.ID,
		map[string]any{"title": "Updated"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false),
	)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Updated", resp.Title)
}

func TestTeachingUnitHandler_Update_OrgStudent_404(t *testing.T) {
	// Students can't view org units in plan 031, so they get 404.
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Org Unit", fx.teacher1.ID)
	w := doUnitPatch(t, fx.h.UpdateUnit, "/api/units/"+u.ID,
		map[string]any{"title": "Hack"},
		map[string]string{"id": u.ID},
		fx.claims(fx.student1, false),
	)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Update_Outsider_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Org Unit", fx.teacher1.ID)
	w := doUnitPatch(t, fx.h.UpdateUnit, "/api/units/"+u.ID,
		map[string]any{"title": "Hack"},
		map[string]string{"id": u.ID},
		fx.claims(fx.outsider, false),
	)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Update_OrgTeacher_OtherOrg_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Org1 Unit", fx.teacher1.ID)
	// teacher2 is in org2 only — can't view org1 units.
	w := doUnitPatch(t, fx.h.UpdateUnit, "/api/units/"+u.ID,
		map[string]any{"title": "Hack"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher2, false),
	)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Update_EmptyTitle_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Original", fx.teacher1.ID)
	w := doUnitPatch(t, fx.h.UpdateUnit, "/api/units/"+u.ID,
		map[string]any{"title": ""},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false),
	)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTeachingUnitHandler_Update_InvalidStatus_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Original", fx.teacher1.ID)
	w := doUnitPatch(t, fx.h.UpdateUnit, "/api/units/"+u.ID,
		map[string]any{"status": "galaxy"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false),
	)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTeachingUnitHandler_Update_NoAuth_401(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Org Unit", fx.teacher1.ID)
	w := doUnitPatch(t, fx.h.UpdateUnit, "/api/units/"+u.ID,
		map[string]any{"title": "Hack"},
		map[string]string{"id": u.ID},
		nil,
	)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== DeleteUnit ====================

func TestTeachingUnitHandler_Delete_OrgTeacher_204(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	// Create directly via store — we don't want the cleanup from mkUnit since deletion is the test.
	ctx := context.Background()
	u, err := fx.h.Units.CreateUnit(ctx, store.CreateTeachingUnitInput{
		Scope: "org", ScopeID: &fx.org1.ID,
		Title: "To Delete", Status: "draft", CreatedBy: fx.teacher1.ID,
	})
	require.NoError(t, err)

	w := doUnitDelete(t, fx.h.DeleteUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify the unit is gone.
	got, err := fx.h.Units.GetUnit(ctx, u.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestTeachingUnitHandler_Delete_Cascade_DocumentGone(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	u, err := fx.h.Units.CreateUnit(ctx, store.CreateTeachingUnitInput{
		Scope: "org", ScopeID: &fx.org1.ID,
		Title: "Has Document", Status: "draft", CreatedBy: fx.teacher1.ID,
	})
	require.NoError(t, err)

	// Confirm unit_documents row exists after creation.
	doc, err := fx.h.Units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, doc, "CreateUnit should seed a unit_documents row")

	// Delete via handler.
	w := doUnitDelete(t, fx.h.DeleteUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusNoContent, w.Code)

	// The document row should be cascade-deleted.
	doc2, err := fx.h.Units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	assert.Nil(t, doc2, "unit_documents row should be deleted by cascade")
}

func TestTeachingUnitHandler_Delete_OtherOrgTeacher_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Org1 Unit", fx.teacher1.ID)
	w := doUnitDelete(t, fx.h.DeleteUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Delete_OrgStudent_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Org Unit", fx.teacher1.ID)
	w := doUnitDelete(t, fx.h.DeleteUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== GetDocument ====================

func TestTeachingUnitHandler_GetDocument_NewUnit_HasEmptyDoc(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	w := doUnitGet(t, fx.h.GetDocument, "/api/units/"+u.ID+"/document", map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
	var resp store.UnitDocument
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, u.ID, resp.UnitID)
	assert.NotNil(t, resp.Blocks)
}

func TestTeachingUnitHandler_GetDocument_NoAccess_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	w := doUnitGet(t, fx.h.GetDocument, "/api/units/"+u.ID+"/document", map[string]string{"id": u.ID}, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== SaveDocument ====================

func TestTeachingUnitHandler_SaveDocument_ValidDoc_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)

	validDoc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "prose",
				"attrs": map[string]any{"id": "blk-001"},
			},
		},
	}

	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", validDoc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)

	var resp store.UnitDocument
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, u.ID, resp.UnitID)
}

func TestTeachingUnitHandler_SaveDocument_Roundtrip(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Roundtrip Unit", fx.teacher1.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "prose",
				"attrs": map[string]any{"id": "blk-abc"},
				"content": []any{
					map[string]any{"type": "text", "text": "Hello World"},
				},
			},
		},
	}

	// Save.
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	// Read back via GET.
	w2 := doUnitGet(t, fx.h.GetDocument, "/api/units/"+u.ID+"/document", map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w2.Code)

	var resp store.UnitDocument
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))

	var roundtripped map[string]any
	require.NoError(t, json.Unmarshal(resp.Blocks, &roundtripped))
	assert.Equal(t, "doc", roundtripped["type"])
	content, ok := roundtripped["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
}

func TestTeachingUnitHandler_SaveDocument_OrgStudent_404(t *testing.T) {
	// Plan-031: org students can't view org units, so they get 404 on document save.
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	doc := map[string]any{"type": "doc", "content": []any{}}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_SaveDocument_OtherOrgTeacher_NotEditor_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	// teacher2 is in org2, cannot view org1 units → 404.
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	doc := map[string]any{"type": "doc", "content": []any{}}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---------- Document validation tests ----------

func TestTeachingUnitHandler_SaveDocument_InvalidEnvelope_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	badDoc := map[string]any{
		"type":    "not-doc",
		"content": []any{},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", badDoc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTeachingUnitHandler_SaveDocument_BlockMissingAttrsID_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	// Block at index 0 has no attrs.id.
	badDoc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "prose",
				"attrs": map[string]any{},
			},
		},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", badDoc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "index 0")
}

func TestTeachingUnitHandler_SaveDocument_BlockMissingAttrsID_AtIndex3_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	// Blocks 0–2 valid; block 3 missing attrs.id.
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "prose", "attrs": map[string]any{"id": "b1"}},
			map[string]any{"type": "prose", "attrs": map[string]any{"id": "b2"}},
			map[string]any{"type": "prose", "attrs": map[string]any{"id": "b3"}},
			map[string]any{"type": "prose", "attrs": map[string]any{}},
		},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "index 3")
}

func TestTeachingUnitHandler_SaveDocument_UnknownBlockType_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	// "video-embed" is not in the plan-031 allowlist.
	badDoc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "video-embed",
				"attrs": map[string]any{"id": "blk-xyz"},
			},
		},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", badDoc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "video-embed")
}

func TestTeachingUnitHandler_SaveDocument_ProblemRef_Valid(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	// problem-ref is in the plan-031 allowlist.
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "problem-ref",
				"attrs": map[string]any{"id": "blk-pr1", "problemId": "00000000-0000-0000-0000-000000000999"},
			},
		},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeachingUnitHandler_SaveDocument_EmptyContent_Valid(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	// An empty content array is valid — no blocks to validate.
	doc := map[string]any{
		"type":    "doc",
		"content": []any{},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeachingUnitHandler_SaveDocument_UpdatedAtBumps(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Bump Unit", fx.teacher1.ID)

	before, err := fx.h.Units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, before)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "prose", "attrs": map[string]any{"id": "b1"}},
		},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	after, err := fx.h.Units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, after)
	assert.True(t, !after.UpdatedAt.Before(before.UpdatedAt), "updated_at should be >= before")
}

// ==================== ListUnits ====================

func TestTeachingUnitHandler_List_OrgScope_TeacherSeesAll(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	_ = fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Draft", fx.teacher1.ID)
	_ = fx.mkUnit(t, "org", &fx.org1.ID, "classroom_ready", "Ready", fx.teacher1.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/units?scope=org&scopeId="+fx.org1.ID, nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	w := httptest.NewRecorder()
	fx.h.ListUnits(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string][]store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp["items"]), 2, "teacher should see at least the 2 units created in this test")
}

func TestTeachingUnitHandler_List_OrgScope_StudentSeesNone(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	_ = fx.mkUnit(t, "org", &fx.org1.ID, "classroom_ready", "Ready", fx.teacher1.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/units?scope=org&scopeId="+fx.org1.ID, nil)
	req = withClaims(req, fx.claims(fx.student1, false))
	w := httptest.NewRecorder()
	fx.h.ListUnits(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string][]store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// Plan-031: students denied entirely.
	assert.Empty(t, resp["items"])
}

func TestTeachingUnitHandler_List_PlatformAdmin_SeesAll(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	_ = fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Draft", fx.teacher1.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/units?scope=org&scopeId="+fx.org1.ID, nil)
	req = withClaims(req, fx.claims(fx.admin, true))
	w := httptest.NewRecorder()
	fx.h.ListUnits(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string][]store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp["items"]), 1, "admin should see the draft unit")
}

func TestTeachingUnitHandler_List_NoAuth_401(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	req := httptest.NewRequest(http.MethodGet, "/api/units?scope=org&scopeId="+fx.org1.ID, nil)
	w := httptest.NewRecorder()
	fx.h.ListUnits(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTeachingUnitHandler_List_InvalidScope_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	req := httptest.NewRequest(http.MethodGet, "/api/units?scope=galaxy", nil)
	req = withClaims(req, fx.claims(fx.admin, true))
	w := httptest.NewRecorder()
	fx.h.ListUnits(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==================== GetUnitByTopic ====================

// setupTopicForUnit creates a minimal course + topic in the DB so a unit can
// have its topic_id FK set. Returns the topicID. Registers cleanup.
func setupTopicForUnit(t *testing.T, db *sql.DB, orgID, userID, suffix string) string {
	t.Helper()
	ctx := context.Background()

	// Derive deterministic hex UUIDs from the suffix using FNV-like hashing.
	h1 := fnvHash(suffix + "c")
	h2 := fnvHash(suffix + "t")
	courseID := fmt.Sprintf("00000000-0000-0000-bbbb-%012x", h1)
	topicID := fmt.Sprintf("00000000-0000-0000-bbbb-%012x", h2)

	_, err := db.ExecContext(ctx, `
		INSERT INTO courses (id, org_id, created_by, title, description, grade_level, language, is_published)
		VALUES ($1, $2, $3, $4, '', '9-12', 'python', false)
		ON CONFLICT (id) DO NOTHING`,
		courseID, orgID, userID, "Course-"+suffix)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO topics (id, course_id, title, description, sort_order, lesson_content)
		VALUES ($1, $2, $3, '', 0, '{}'::jsonb)
		ON CONFLICT (id) DO NOTHING`,
		topicID, courseID, "Topic-"+suffix)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM topics WHERE id = $1`, topicID)
		db.ExecContext(ctx, `DELETE FROM courses WHERE id = $1`, courseID)
	})
	return topicID
}

// fnvHash converts a string to a stable uint64 (48-bit masked) suitable for
// building deterministic hex UUID segments in tests.
func fnvHash(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h & 0xffffffffffff
}

func TestTeachingUnitHandler_GetUnitByTopic_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()

	// Create an org unit, then link it to a topic.
	u := fx.mkUnit(t, "org", &fx.org1.ID, "classroom_ready", "Topic Unit", fx.teacher1.ID)
	topicID := setupTopicForUnit(t, fx.sqlDB, fx.org1.ID, fx.teacher1.ID, t.Name())

	_, err := fx.sqlDB.ExecContext(ctx,
		`UPDATE teaching_units SET topic_id = $1 WHERE id = $2`, topicID, u.ID)
	require.NoError(t, err)

	w := doUnitGet(t, fx.h.GetUnitByTopic, "/api/units/by-topic/"+topicID,
		map[string]string{"topicId": topicID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)

	var resp store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, u.ID, resp.ID)
	require.NotNil(t, resp.TopicID)
	assert.Equal(t, topicID, *resp.TopicID)
}

func TestTeachingUnitHandler_GetUnitByTopic_UnknownID_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	unknownID := "00000000-0000-0000-0000-000000000099"
	w := doUnitGet(t, fx.h.GetUnitByTopic, "/api/units/by-topic/"+unknownID,
		map[string]string{"topicId": unknownID},
		fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_GetUnitByTopic_NoAccess_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()

	// Create a personal unit owned by outsider and link it to a topic.
	uid := fx.outsider.ID
	u := fx.mkUnit(t, "personal", &uid, "draft", "Private Unit", fx.outsider.ID)
	topicID := setupTopicForUnit(t, fx.sqlDB, fx.org1.ID, fx.outsider.ID, t.Name())

	_, err := fx.sqlDB.ExecContext(ctx,
		`UPDATE teaching_units SET topic_id = $1 WHERE id = $2`, topicID, u.ID)
	require.NoError(t, err)

	// teacher1 cannot view a personal unit owned by outsider.
	w := doUnitGet(t, fx.h.GetUnitByTopic, "/api/units/by-topic/"+topicID,
		map[string]string{"topicId": topicID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_GetUnitByTopic_NoAuth_401(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	unknownID := "00000000-0000-0000-0000-000000000098"
	w := doUnitGet(t, fx.h.GetUnitByTopic, "/api/units/by-topic/"+unknownID,
		map[string]string{"topicId": unknownID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== TransitionUnit ====================

func TestTeachingUnitHandler_Transition_DraftToReviewed_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Draft Unit", fx.teacher1.ID)

	w := doUnitPostWithParams(t, fx.h.TransitionUnit, "/api/units/"+u.ID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)

	var resp store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "reviewed", resp.Status)
}

func TestTeachingUnitHandler_Transition_ReviewedToClassroomReady_200_CreatesRevision(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Publish Unit", fx.teacher1.ID)

	// Save some blocks.
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, u.ID, blocks)
	require.NoError(t, err)

	// draft→reviewed
	w := doUnitPostWithParams(t, fx.h.TransitionUnit, "/api/units/"+u.ID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	// reviewed→classroom_ready
	w = doUnitPostWithParams(t, fx.h.TransitionUnit, "/api/units/"+u.ID+"/transition",
		map[string]any{"status": "classroom_ready"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "classroom_ready", resp.Status)

	// GET /revisions should show 1 revision.
	w2 := doUnitGet(t, fx.h.ListRevisions, "/api/units/"+u.ID+"/revisions",
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w2.Code)

	var revResp struct {
		Items []store.UnitRevision `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &revResp))
	require.Len(t, revResp.Items, 1)
	require.NotNil(t, revResp.Items[0].Reason)
	assert.Equal(t, "classroom_ready", *revResp.Items[0].Reason)
}

func TestTeachingUnitHandler_Transition_InvalidSkip_409(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Skip Unit", fx.teacher1.ID)

	// draft→classroom_ready should be 409 (skips reviewed).
	w := doUnitPostWithParams(t, fx.h.TransitionUnit, "/api/units/"+u.ID+"/transition",
		map[string]any{"status": "classroom_ready"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestTeachingUnitHandler_Transition_NonEditor_403(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Auth Unit", fx.teacher1.ID)

	// teacher2 is in org2, not org1 — can't even view the unit → 404.
	w := doUnitPostWithParams(t, fx.h.TransitionUnit, "/api/units/"+u.ID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)

	// student1 is in org1 but can't view org units in plan-031 → 404.
	w2 := doUnitPostWithParams(t, fx.h.TransitionUnit, "/api/units/"+u.ID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": u.ID},
		fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w2.Code)
}

func TestTeachingUnitHandler_Transition_NonExistent_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	fakeID := "00000000-0000-0000-0000-000000000001"

	w := doUnitPostWithParams(t, fx.h.TransitionUnit, "/api/units/"+fakeID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": fakeID},
		fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Transition_InvalidTargetStatus_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Bad Status", fx.teacher1.ID)

	w := doUnitPostWithParams(t, fx.h.TransitionUnit, "/api/units/"+u.ID+"/transition",
		map[string]any{"status": "galaxy"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTeachingUnitHandler_Transition_NoAuth_401(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "NoAuth Unit", fx.teacher1.ID)

	w := doUnitPostWithParams(t, fx.h.TransitionUnit, "/api/units/"+u.ID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": u.ID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== ListRevisions ====================

func TestTeachingUnitHandler_ListRevisions_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Rev List Unit", fx.teacher1.ID)

	// Transition through to classroom_ready.
	_, err := fx.h.Units.SetUnitStatus(ctx, u.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, u.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	w := doUnitGet(t, fx.h.ListRevisions, "/api/units/"+u.ID+"/revisions",
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.UnitRevision `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
}

func TestTeachingUnitHandler_ListRevisions_NoAccess_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Rev Access Unit", fx.teacher1.ID)

	w := doUnitGet(t, fx.h.ListRevisions, "/api/units/"+u.ID+"/revisions",
		map[string]string{"id": u.ID},
		fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== GetRevision ====================

func TestTeachingUnitHandler_GetRevision_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Get Rev Unit", fx.teacher1.ID)

	// Create a revision.
	_, err := fx.h.Units.SetUnitStatus(ctx, u.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, u.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	revs, err := fx.h.Units.ListRevisions(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)

	w := doUnitGet(t, fx.h.GetRevision, "/api/units/"+u.ID+"/revisions/"+revs[0].ID,
		map[string]string{"id": u.ID, "revisionId": revs[0].ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp store.UnitRevision
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, revs[0].ID, resp.ID)
	assert.Equal(t, u.ID, resp.UnitID)
}

func TestTeachingUnitHandler_GetRevision_NotFound_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Rev Not Found", fx.teacher1.ID)
	fakeRevID := "00000000-0000-0000-0000-000000000099"

	w := doUnitGet(t, fx.h.GetRevision, "/api/units/"+u.ID+"/revisions/"+fakeRevID,
		map[string]string{"id": u.ID, "revisionId": fakeRevID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_GetRevision_WrongUnit_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()

	u1 := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Unit1 Rev", fx.teacher1.ID)
	u2 := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Unit2 Rev", fx.teacher1.ID)

	// Create a revision on u1.
	_, err := fx.h.Units.SetUnitStatus(ctx, u1.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, u1.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	revs, err := fx.h.Units.ListRevisions(ctx, u1.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)

	// Try to access u1's revision via u2's path → 404.
	w := doUnitGet(t, fx.h.GetRevision, "/api/units/"+u2.ID+"/revisions/"+revs[0].ID,
		map[string]string{"id": u2.ID, "revisionId": revs[0].ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== Block Allowlist Expansion (Task 3) ====================

func TestTeachingUnitHandler_SaveDocument_TeacherNote_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Note Unit", fx.teacher1.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "teacher-note",
				"attrs": map[string]any{"id": "tn-001"},
				"content": []any{
					map[string]any{"type": "paragraph", "content": []any{
						map[string]any{"type": "text", "text": "This is for teachers only"},
					}},
				},
			},
		},
	}

	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc,
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeachingUnitHandler_SaveDocument_CodeSnippet_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Snippet Unit", fx.teacher1.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "code-snippet",
				"attrs": map[string]any{
					"id":       "cs-001",
					"language": "python",
					"code":     "print('hello')",
				},
			},
		},
	}

	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc,
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeachingUnitHandler_SaveDocument_MediaEmbed_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Media Unit", fx.teacher1.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "media-embed",
				"attrs": map[string]any{
					"id":   "me-001",
					"url":  "https://example.com/image.png",
					"alt":  "Example image",
					"type": "image",
				},
			},
		},
	}

	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc,
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeachingUnitHandler_SaveDocument_NewBlockMissingID_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "No ID Unit", fx.teacher1.ID)

	// teacher-note without attrs.id → 400.
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "teacher-note",
				"attrs": map[string]any{},
			},
		},
	}

	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document", doc,
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "teacher-note")
}
