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
		Units:   store.NewTeachingUnitStore(db),
		Orgs:    orgs,
		Courses: store.NewCourseStore(db),
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
			db.ExecContext(ctx, "DELETE FROM unit_overlays WHERE child_unit_id IN (SELECT id FROM teaching_units WHERE created_by = $1 OR scope_id = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM unit_overlays WHERE parent_unit_id IN (SELECT id FROM teaching_units WHERE created_by = $1 OR scope_id = $1)", u.ID)
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
		fx.sqlDB.ExecContext(ctx, "DELETE FROM unit_overlays WHERE child_unit_id = $1 OR parent_unit_id = $1", u.ID)
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
	// Plan 061: org students are denied UNLESS they have a class
	// membership wired to the unit's topic. The fixture's student1
	// has no class binding, so even classroom_ready is invisible.
	// (Bound-success path is in
	// TestTeachingUnitHandler_Get_OrgStudent_ViaClassBinding_200.)
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
		INSERT INTO topics (id, course_id, title, description, sort_order)
		VALUES ($1, $2, $3, '', 0)
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

// ==================== GetProjectedDocument ====================

// projectedDoc is the document used across projection tests. It contains one
// of each block type exercised by the projection pipeline.
var projectedDoc = map[string]any{
	"type": "doc",
	"content": []any{
		map[string]any{"type": "prose", "attrs": map[string]any{"id": "b01"}},
		map[string]any{"type": "paragraph"},
		map[string]any{"type": "heading", "attrs": map[string]any{"level": 2}},
		map[string]any{"type": "teacher-note", "attrs": map[string]any{"id": "b04"}},
		map[string]any{"type": "live-cue", "attrs": map[string]any{"id": "b05", "trigger": "manual"}},
		map[string]any{"type": "problem-ref", "attrs": map[string]any{"id": "b06", "visibility": "always"}},
		map[string]any{"type": "solution-ref", "attrs": map[string]any{"id": "b07", "reveal": "always"}},
		map[string]any{"type": "solution-ref", "attrs": map[string]any{"id": "b08", "reveal": "after-submit"}},
		map[string]any{"type": "test-case-ref", "attrs": map[string]any{"id": "b09"}},
		map[string]any{"type": "assignment-variant", "attrs": map[string]any{"id": "b10"}},
		map[string]any{"type": "code-snippet", "attrs": map[string]any{"id": "b11", "language": "python"}},
	},
}

// saveProjectedDoc saves projectedDoc to the unit and returns the unit.
func saveProjectedDoc(t *testing.T, fx *unitFixture, unit *store.TeachingUnit) {
	t.Helper()
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+unit.ID+"/document",
		projectedDoc, map[string]string{"id": unit.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)
}

// parseProjectedContent reads the projected response and returns block types.
func parseProjectedContent(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	var resp struct {
		Type    string            `json:"type"`
		Content []json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "doc", resp.Type)

	types := make([]string, 0, len(resp.Content))
	for _, raw := range resp.Content {
		var block struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal(raw, &block))
		types = append(types, block.Type)
	}
	return types
}

func TestTeachingUnitHandler_Projected_Teacher_Default_SeesAll(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Projected Unit", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	types := parseProjectedContent(t, w)
	// Teacher should see all 11 blocks.
	assert.Len(t, types, 11, "teacher should see all blocks")
	assert.Contains(t, types, "teacher-note")
	assert.Contains(t, types, "live-cue")
	assert.Contains(t, types, "assignment-variant")
	assert.Contains(t, types, "solution-ref")
}

func TestTeachingUnitHandler_Projected_Teacher_PreviewAsStudent(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Preview Unit", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	req := httptest.NewRequest(http.MethodGet, "/api/units/"+u.ID+"/projected?role=student", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": u.ID})
	w := httptest.NewRecorder()
	fx.h.GetProjectedDocument(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	types := parseProjectedContent(t, w)
	// Student view: teacher-note, live-cue, assignment-variant, solution-ref(after-submit) omitted.
	assert.NotContains(t, types, "teacher-note")
	assert.NotContains(t, types, "live-cue")
	assert.NotContains(t, types, "assignment-variant")
	// prose, paragraph, heading, problem-ref(always), solution-ref(always), test-case-ref, code-snippet = 7
	assert.Len(t, types, 7, "student projection should show 7 blocks")
}

func TestTeachingUnitHandler_Projected_Admin_Default_SeesAll(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Admin Projected", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	types := parseProjectedContent(t, w)
	assert.Len(t, types, 11, "admin should see all blocks")
}

func TestTeachingUnitHandler_Projected_Student_Default_Filtered(t *testing.T) {
	// Note: students can't view org units in plan-031. Use a platform
	// classroom_ready unit so student can view it.
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "platform", nil, "classroom_ready", "Student Projected", fx.admin.ID)
	// Save doc as admin.
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document",
		projectedDoc, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	// Student requests projected document.
	w2 := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	require.Equal(t, http.StatusOK, w2.Code)

	types := parseProjectedContent(t, w2)
	assert.NotContains(t, types, "teacher-note")
	assert.NotContains(t, types, "live-cue")
	assert.NotContains(t, types, "assignment-variant")
	assert.Len(t, types, 7, "student should see 7 blocks")
}

func TestTeachingUnitHandler_Projected_Student_CannotEscalateRole(t *testing.T) {
	// Student requests ?role=teacher but should still get student projection.
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "platform", nil, "classroom_ready", "Escalation Unit", fx.admin.ID)
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document",
		projectedDoc, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest(http.MethodGet, "/api/units/"+u.ID+"/projected?role=teacher", nil)
	req = withClaims(req, fx.claims(fx.student1, false))
	req = withChiParams(req, map[string]string{"id": u.ID})
	w2 := httptest.NewRecorder()
	fx.h.GetProjectedDocument(w2, req)

	require.Equal(t, http.StatusOK, w2.Code)
	types := parseProjectedContent(t, w2)
	// Student is locked to student role — teacher-notes should still be omitted.
	assert.NotContains(t, types, "teacher-note")
}

func TestTeachingUnitHandler_Projected_SolutionRef_RevealAlways_Student(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "platform", nil, "classroom_ready", "Sol Always", fx.admin.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "solution-ref", "attrs": map[string]any{"id": "sr1", "reveal": "always"}},
		},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document",
		doc, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	w2 := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	require.Equal(t, http.StatusOK, w2.Code)

	types := parseProjectedContent(t, w2)
	assert.Contains(t, types, "solution-ref")
}

func TestTeachingUnitHandler_Projected_SolutionRef_AfterSubmit_NoState_Student_Omitted(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "platform", nil, "classroom_ready", "Sol NoState", fx.admin.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "solution-ref", "attrs": map[string]any{"id": "sr1", "reveal": "after-submit"}},
		},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document",
		doc, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	w2 := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	require.Equal(t, http.StatusOK, w2.Code)

	types := parseProjectedContent(t, w2)
	assert.NotContains(t, types, "solution-ref", "after-submit with no state should be omitted")
}

func TestTeachingUnitHandler_Projected_SolutionRef_AfterSubmit_WithState_Student_Included(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "platform", nil, "classroom_ready", "Sol WithState", fx.admin.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "solution-ref", "attrs": map[string]any{"id": "sr1", "reveal": "after-submit"}},
		},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document",
		doc, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest(http.MethodGet, "/api/units/"+u.ID+"/projected?attemptStates=sr1:submitted", nil)
	req = withClaims(req, fx.claims(fx.student1, false))
	req = withChiParams(req, map[string]string{"id": u.ID})
	w2 := httptest.NewRecorder()
	fx.h.GetProjectedDocument(w2, req)

	require.Equal(t, http.StatusOK, w2.Code)
	types := parseProjectedContent(t, w2)
	assert.Contains(t, types, "solution-ref", "after-submit with submitted state should be included")
}

func TestTeachingUnitHandler_Projected_NonViewer_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "No Access", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Projected_NoAuth_401(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "No Auth", fx.teacher1.ID)

	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTeachingUnitHandler_Projected_InvalidRole_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Bad Role", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	req := httptest.NewRequest(http.MethodGet, "/api/units/"+u.ID+"/projected?role=superuser", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": u.ID})
	w := httptest.NewRecorder()
	fx.h.GetProjectedDocument(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTeachingUnitHandler_Projected_InvalidAttemptState_400(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Bad State", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	req := httptest.NewRequest(http.MethodGet, "/api/units/"+u.ID+"/projected?attemptStates=b1:invalid", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": u.ID})
	w := httptest.NewRecorder()
	fx.h.GetProjectedDocument(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTeachingUnitHandler_Projected_PersonalUnit_OwnerIsTeacher(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	uid := fx.outsider.ID
	u := fx.mkUnit(t, "personal", &uid, "draft", "Personal Proj", fx.outsider.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "teacher-note", "attrs": map[string]any{"id": "tn1"}},
			map[string]any{"type": "prose", "attrs": map[string]any{"id": "p1"}},
		},
	}
	w := doUnitPut(t, fx.h.SaveDocument, "/api/units/"+u.ID+"/document",
		doc, map[string]string{"id": u.ID}, fx.claims(fx.outsider, false))
	require.Equal(t, http.StatusOK, w.Code)

	// Owner should see teacher-notes (they're treated as teacher).
	w2 := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.outsider, false))
	require.Equal(t, http.StatusOK, w2.Code)
	types := parseProjectedContent(t, w2)
	assert.Contains(t, types, "teacher-note", "personal unit owner should be treated as teacher")
}

func TestTeachingUnitHandler_Projected_EmptyDoc(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Empty Doc", fx.teacher1.ID)

	// The default seeded document should be an empty doc.
	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Type    string            `json:"type"`
		Content []json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "doc", resp.Type)
	// Empty or nil content should return an empty array.
	assert.NotNil(t, resp.Content)
}

func TestTeachingUnitHandler_Projected_NotFound_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	fakeID := "00000000-0000-0000-0000-000000000001"
	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+fakeID+"/projected",
		map[string]string{"id": fakeID}, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- Plan 062 — overlay composition ---
//
// /projected previously called GetDocument directly, returning only
// the raw child blocks. For forked (overlay-child) units that
// meant: empty doc when the child has no own blocks, stale doc
// when the child diverged. Plan 062 routes /projected through
// GetComposedDocument so overlays are merged before role
// projection.

// forkChildOf creates a fork of `parent` for the same teacher.
// Returns the child unit with cleanup registered.
func forkChildOf(t *testing.T, fx *unitFixture, parent *store.TeachingUnit) *store.TeachingUnit {
	t.Helper()
	ctx := context.Background()
	child, err := fx.h.Units.ForkUnit(ctx, parent.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)
	return child
}

func TestTeachingUnitHandler_Projected_ForkedUnit_NoOverrides_ShowsParentBlocks(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()

	// Parent has 2 prose blocks; needs to be classroom_ready before
	// the fork can be student-readable.
	parent := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Parent w/ blocks", fx.teacher1.ID)
	parentBlocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"p1"}},{"type":"prose","attrs":{"id":"p2"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, parent.ID, parentBlocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, parent.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, parent.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	// Fork creates a child with NO overrides. Pre-062: /projected
	// would return the child's own (empty) blocks.
	child := forkChildOf(t, fx, parent)

	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+child.ID+"/projected",
		map[string]string{"id": child.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Type    string                   `json:"type"`
		Content []map[string]interface{} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Content, 2, "fork without overrides must show parent's blocks via composition")
}

func TestTeachingUnitHandler_Projected_ForkedUnit_HideOverlay_OmitsBlock(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()

	parent := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Hide Parent", fx.teacher1.ID)
	parentBlocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}},{"type":"prose","attrs":{"id":"b2"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, parent.ID, parentBlocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, parent.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, parent.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	child := forkChildOf(t, fx, parent)
	_, err = fx.h.Units.UpdateOverlay(ctx, child.ID, store.UpdateOverlayInput{
		BlockOverrides: json.RawMessage(`{"b1":{"action":"hide"}}`),
	})
	require.NoError(t, err)

	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+child.ID+"/projected",
		map[string]string{"id": child.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Type    string                   `json:"type"`
		Content []map[string]interface{} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Content, 1, "hide overlay should omit b1")
	attrs := resp.Content[0]["attrs"].(map[string]interface{})
	assert.Equal(t, "b2", attrs["id"])
}

func TestTeachingUnitHandler_Projected_ForkedUnit_ReplaceOverlay_ShowsReplacement(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()

	parent := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Replace Parent", fx.teacher1.ID)
	parentBlocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, parent.ID, parentBlocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, parent.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, parent.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	child := forkChildOf(t, fx, parent)
	_, err = fx.h.Units.UpdateOverlay(ctx, child.ID, store.UpdateOverlayInput{
		BlockOverrides: json.RawMessage(`{"b1":{"action":"replace","block":{"type":"prose","attrs":{"id":"b1-replaced"}}}}`),
	})
	require.NoError(t, err)

	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+child.ID+"/projected",
		map[string]string{"id": child.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Type    string                   `json:"type"`
		Content []map[string]interface{} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Content, 1)
	attrs := resp.Content[0]["attrs"].(map[string]interface{})
	assert.Equal(t, "b1-replaced", attrs["id"])
}

func TestTeachingUnitHandler_Projected_ForkedUnit_TeacherOnlyParent_StudentCantSee(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()

	// Parent has a teacher-note (teacher-only) and a prose block.
	parent := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Teacher-Note Parent", fx.teacher1.ID)
	parentBlocks := json.RawMessage(`{"type":"doc","content":[{"type":"teacher-note","attrs":{"id":"tn1"}},{"type":"prose","attrs":{"id":"p1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, parent.ID, parentBlocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, parent.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, parent.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	child := forkChildOf(t, fx, parent)

	// Teacher previewing-as-student should NOT see the teacher-note,
	// even though composition pulled it in from the parent. This
	// confirms the order is compose-then-filter, not filter-then-
	// compose.
	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+child.ID+"/projected?role=student",
		map[string]string{"id": child.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Type    string                   `json:"type"`
		Content []map[string]interface{} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	for _, b := range resp.Content {
		assert.NotEqual(t, "teacher-note", b["type"], "student must not see teacher-only blocks even after composition")
	}
}

func TestTeachingUnitHandler_Projected_NonForkedUnit_UnchangedBehavior(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()

	// Plain non-forked unit. /projected should still work.
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Plain", fx.teacher1.ID)
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"p1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, u.ID, blocks)
	require.NoError(t, err)

	w := doUnitGet(t, fx.h.GetProjectedDocument, "/api/units/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Type    string                   `json:"type"`
		Content []map[string]interface{} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Content, 1)
}

// ==================== ForkUnit ====================

// forkCleanup deletes the child unit and its overlay/doc/revision rows created
// by a fork. Needed for units created via the handler (bypassing mkUnit).
func (fx *unitFixture) forkCleanup(t *testing.T, childID string) {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() {
		fx.sqlDB.ExecContext(ctx, "DELETE FROM unit_overlays WHERE child_unit_id = $1", childID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM unit_revisions WHERE unit_id = $1", childID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM unit_documents WHERE unit_id = $1", childID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", childID)
	})
}

func TestTeachingUnitHandler_Fork_CreatesChildOverlayDoc(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Fork Source", fx.teacher1.ID)

	w := doUnitPostWithParams(t, fx.h.ForkUnit, "/api/units/"+source.ID+"/fork",
		map[string]any{"scope": "org", "scopeId": fx.org1.ID},
		map[string]string{"id": source.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code)

	var child store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &child))
	fx.forkCleanup(t, child.ID)

	assert.Equal(t, "Fork Source (fork)", child.Title)
	assert.Equal(t, "draft", child.Status)
	assert.Equal(t, "org", child.Scope)

	// Overlay must exist.
	ov, err := fx.h.Units.GetOverlay(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, ov)
	assert.Equal(t, child.ID, ov.ChildUnitID)
	assert.Equal(t, source.ID, ov.ParentUnitID)

	// Document must exist.
	doc, err := fx.h.Units.GetDocument(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, doc)
}

func TestTeachingUnitHandler_Fork_CustomTitle(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Source", fx.teacher1.ID)

	w := doUnitPostWithParams(t, fx.h.ForkUnit, "/api/units/"+source.ID+"/fork",
		map[string]any{"scope": "org", "scopeId": fx.org1.ID, "title": "My Custom Fork"},
		map[string]string{"id": source.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code)

	var child store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &child))
	fx.forkCleanup(t, child.ID)

	assert.Equal(t, "My Custom Fork", child.Title)
}

func TestTeachingUnitHandler_Fork_DefaultScopeInference(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	// teacher1 is in exactly one org (org1), so scope defaults to "org".
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Infer Source", fx.teacher1.ID)

	w := doUnitPostWithParams(t, fx.h.ForkUnit, "/api/units/"+source.ID+"/fork",
		map[string]any{},
		map[string]string{"id": source.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code)

	var child store.TeachingUnit
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &child))
	fx.forkCleanup(t, child.ID)

	assert.Equal(t, "org", child.Scope)
	require.NotNil(t, child.ScopeID)
	assert.Equal(t, fx.org1.ID, *child.ScopeID)
}

func TestTeachingUnitHandler_Fork_SourceNotFound_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	fakeID := "00000000-0000-0000-0000-000000000099"

	w := doUnitPostWithParams(t, fx.h.ForkUnit, "/api/units/"+fakeID+"/fork",
		map[string]any{"scope": "personal", "scopeId": fx.teacher1.ID},
		map[string]string{"id": fakeID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Fork_SourceNotVisible_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	// teacher2 can't view org1 units.
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Hidden Source", fx.teacher1.ID)

	w := doUnitPostWithParams(t, fx.h.ForkUnit, "/api/units/"+source.ID+"/fork",
		map[string]any{"scope": "org", "scopeId": fx.org2.ID},
		map[string]string{"id": source.ID},
		fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Fork_NotAuthorizedForTargetScope_403(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Auth Source", fx.teacher1.ID)

	// teacher1 tries to fork into org2 where they're not a member.
	w := doUnitPostWithParams(t, fx.h.ForkUnit, "/api/units/"+source.ID+"/fork",
		map[string]any{"scope": "org", "scopeId": fx.org2.ID},
		map[string]string{"id": source.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestTeachingUnitHandler_Fork_NoAuth_401(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "NoAuth Source", fx.teacher1.ID)

	w := doUnitPostWithParams(t, fx.h.ForkUnit, "/api/units/"+source.ID+"/fork",
		map[string]any{"scope": "personal"},
		map[string]string{"id": source.ID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== GetOverlay ====================

func TestTeachingUnitHandler_GetOverlay_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Overlay Source", fx.teacher1.ID)

	child, err := fx.h.Units.ForkUnit(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doUnitGet(t, fx.h.GetOverlay, "/api/units/"+child.ID+"/overlay",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var ov store.UnitOverlay
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ov))
	assert.Equal(t, child.ID, ov.ChildUnitID)
	assert.Equal(t, source.ID, ov.ParentUnitID)
}

func TestTeachingUnitHandler_GetOverlay_NonForkedUnit_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "No Overlay", fx.teacher1.ID)

	w := doUnitGet(t, fx.h.GetOverlay, "/api/units/"+u.ID+"/overlay",
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_GetOverlay_NoAccess_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Private Overlay", fx.teacher1.ID)

	child, err := fx.h.Units.ForkUnit(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	// teacher2 (org2) can't view org1 units.
	w := doUnitGet(t, fx.h.GetOverlay, "/api/units/"+child.ID+"/overlay",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== PatchOverlay ====================

func TestTeachingUnitHandler_PatchOverlay_HideOverride(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Patch Source", fx.teacher1.ID)

	child, err := fx.h.Units.ForkUnit(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doUnitPatch(t, fx.h.PatchOverlay, "/api/units/"+child.ID+"/overlay",
		map[string]any{"blockOverrides": map[string]any{"b1": map[string]any{"action": "hide"}}},
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var ov store.UnitOverlay
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ov))
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(ov.BlockOverrides, &parsed))
	assert.Contains(t, parsed, "b1")
}

func TestTeachingUnitHandler_PatchOverlay_PinRevision(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Pin Source", fx.teacher1.ID)

	// Publish to create a revision.
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, source.ID, blocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	revs, err := fx.h.Units.ListRevisions(ctx, source.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)

	child, err := fx.h.Units.ForkUnit(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	// Pin to the revision.
	w := doUnitPatch(t, fx.h.PatchOverlay, "/api/units/"+child.ID+"/overlay",
		map[string]any{"parentRevisionId": revs[0].ID},
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var ov store.UnitOverlay
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ov))
	require.NotNil(t, ov.ParentRevisionID)
	assert.Equal(t, revs[0].ID, *ov.ParentRevisionID)
}

func TestTeachingUnitHandler_PatchOverlay_FloatBack(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Float Source", fx.teacher1.ID)

	// Publish to create a revision.
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, source.ID, blocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	revs, err := fx.h.Units.ListRevisions(ctx, source.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)

	child, err := fx.h.Units.ForkUnit(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	// Pin, then float.
	_, err = fx.h.Units.UpdateOverlay(ctx, child.ID, store.UpdateOverlayInput{
		ParentRevisionID: &revs[0].ID,
	})
	require.NoError(t, err)

	// Float: set parentRevisionId to ""
	w := doUnitPatch(t, fx.h.PatchOverlay, "/api/units/"+child.ID+"/overlay",
		map[string]any{"parentRevisionId": ""},
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var ov store.UnitOverlay
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ov))
	assert.Nil(t, ov.ParentRevisionID, "empty string should set to NULL (floating)")
}

func TestTeachingUnitHandler_PatchOverlay_NotForked_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "No Fork", fx.teacher1.ID)

	w := doUnitPatch(t, fx.h.PatchOverlay, "/api/units/"+u.ID+"/overlay",
		map[string]any{"blockOverrides": map[string]any{}},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_PatchOverlay_NotEditor_403(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "platform", nil, "classroom_ready", "Platform Source", fx.admin.ID)

	child, err := fx.h.Units.ForkUnit(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	// teacher2 is in org2, can't edit org1 units → 404 (can't view).
	w := doUnitPatch(t, fx.h.PatchOverlay, "/api/units/"+child.ID+"/overlay",
		map[string]any{"blockOverrides": map[string]any{}},
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_PatchOverlay_NoAuth_401(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Auth Source", fx.teacher1.ID)

	child, err := fx.h.Units.ForkUnit(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doUnitPatch(t, fx.h.PatchOverlay, "/api/units/"+child.ID+"/overlay",
		map[string]any{},
		map[string]string{"id": child.ID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== GetComposedDocument ====================

func TestTeachingUnitHandler_Composed_ForkedUnit_EqualsParent(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Compose Source", fx.teacher1.ID)

	// Save blocks to source and publish.
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"},"content":[{"type":"text","text":"hello"}]}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, source.ID, blocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	child, err := fx.h.Units.ForkUnit(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doUnitGet(t, fx.h.GetComposedDocument, "/api/units/"+child.ID+"/composed",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	assert.Equal(t, "doc", doc["type"])
	content := doc["content"].([]interface{})
	require.Len(t, content, 1)
}

func TestTeachingUnitHandler_Composed_HideOverride(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Hide Compose", fx.teacher1.ID)

	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}},{"type":"prose","attrs":{"id":"b2"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, source.ID, blocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	child, err := fx.h.Units.ForkUnit(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	// Hide b1.
	_, err = fx.h.Units.UpdateOverlay(ctx, child.ID, store.UpdateOverlayInput{
		BlockOverrides: json.RawMessage(`{"b1":{"action":"hide"}}`),
	})
	require.NoError(t, err)

	w := doUnitGet(t, fx.h.GetComposedDocument, "/api/units/"+child.ID+"/composed",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	content := doc["content"].([]interface{})
	assert.Len(t, content, 1, "hide should omit b1")
}

func TestTeachingUnitHandler_Composed_ReplaceOverride(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Replace Compose", fx.teacher1.ID)

	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, source.ID, blocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	child, err := fx.h.Units.ForkUnit(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	_, err = fx.h.Units.UpdateOverlay(ctx, child.ID, store.UpdateOverlayInput{
		BlockOverrides: json.RawMessage(`{"b1":{"action":"replace","block":{"type":"prose","attrs":{"id":"b1-new"}}}}`),
	})
	require.NoError(t, err)

	w := doUnitGet(t, fx.h.GetComposedDocument, "/api/units/"+child.ID+"/composed",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	content := doc["content"].([]interface{})
	require.Len(t, content, 1)
	b := content[0].(map[string]interface{})
	attrs := b["attrs"].(map[string]interface{})
	assert.Equal(t, "b1-new", attrs["id"])
}

func TestTeachingUnitHandler_Composed_NonForkedUnit(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Plain Unit", fx.teacher1.ID)

	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"p1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, u.ID, blocks)
	require.NoError(t, err)

	w := doUnitGet(t, fx.h.GetComposedDocument, "/api/units/"+u.ID+"/composed",
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	assert.Equal(t, "doc", doc["type"])
}

func TestTeachingUnitHandler_Composed_NoAccess_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Hidden Compose", fx.teacher1.ID)

	w := doUnitGet(t, fx.h.GetComposedDocument, "/api/units/"+u.ID+"/composed",
		map[string]string{"id": u.ID},
		fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Composed_NoAuth_401(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Auth Compose", fx.teacher1.ID)

	w := doUnitGet(t, fx.h.GetComposedDocument, "/api/units/"+u.ID+"/composed",
		map[string]string{"id": u.ID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== GetLineage ====================

func TestTeachingUnitHandler_Lineage_NonForked_JustSelf(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Lone Unit", fx.teacher1.ID)

	w := doUnitGet(t, fx.h.GetLineage, "/api/units/"+u.ID+"/lineage",
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.LineageEntry `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, u.ID, resp.Items[0].UnitID)
}

func TestTeachingUnitHandler_Lineage_ChildParent(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	parent := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Parent", fx.teacher1.ID)

	child, err := fx.h.Units.ForkUnit(ctx, parent.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doUnitGet(t, fx.h.GetLineage, "/api/units/"+child.ID+"/lineage",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.LineageEntry `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2, "should have parent + child")
	assert.Equal(t, parent.ID, resp.Items[0].UnitID, "root-first")
	assert.Equal(t, child.ID, resp.Items[1].UnitID)
}

func TestTeachingUnitHandler_Lineage_ThreeGenerations(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()
	grandparent := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Grandparent", fx.teacher1.ID)

	parentUnit, err := fx.h.Units.ForkUnit(ctx, grandparent.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, parentUnit.ID)

	child, err := fx.h.Units.ForkUnit(ctx, parentUnit.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doUnitGet(t, fx.h.GetLineage, "/api/units/"+child.ID+"/lineage",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.LineageEntry `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 3, "grandparent → parent → child = 3 entries")
	assert.Equal(t, grandparent.ID, resp.Items[0].UnitID)
	assert.Equal(t, parentUnit.ID, resp.Items[1].UnitID)
	assert.Equal(t, child.ID, resp.Items[2].UnitID)
}

func TestTeachingUnitHandler_Lineage_NoAccess_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Hidden Lineage", fx.teacher1.ID)

	w := doUnitGet(t, fx.h.GetLineage, "/api/units/"+u.ID+"/lineage",
		map[string]string{"id": u.ID},
		fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Lineage_NoAuth_401(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Auth Lineage", fx.teacher1.ID)

	w := doUnitGet(t, fx.h.GetLineage, "/api/units/"+u.ID+"/lineage",
		map[string]string{"id": u.ID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== Phase 3 block type validation (plan 037) ====================

func TestValidateBlockDocument_Phase3_Callout_Valid(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "callout",
				"attrs": map[string]any{"id": "blk-callout-1", "variant": "info"},
				"content": []any{
					map[string]any{"type": "paragraph"},
				},
			},
		},
	})
	assert.NoError(t, validateBlockDocument(doc))
}

func TestValidateBlockDocument_Phase3_Callout_MissingID_Error(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "callout",
				"attrs": map[string]any{"id": "", "variant": "warning"},
			},
		},
	})
	err := validateBlockDocument(doc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "callout")
}

func TestValidateBlockDocument_Phase3_ToggleBlock_Valid(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "toggle-block",
				"attrs": map[string]any{"id": "blk-toggle-1", "summary": "Details"},
				"content": []any{
					map[string]any{"type": "paragraph"},
				},
			},
		},
	})
	assert.NoError(t, validateBlockDocument(doc))
}

func TestValidateBlockDocument_Phase3_ToggleBlock_MissingID_Error(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "toggle-block",
				"attrs": map[string]any{"id": "", "summary": "x"},
			},
		},
	})
	err := validateBlockDocument(doc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "toggle-block")
}

func TestValidateBlockDocument_Phase3_Bookmark_Valid(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "bookmark",
				"attrs": map[string]any{"id": "blk-bm-1", "url": "https://example.com"},
			},
		},
	})
	assert.NoError(t, validateBlockDocument(doc))
}

func TestValidateBlockDocument_Phase3_Bookmark_MissingID_Error(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "bookmark",
				"attrs": map[string]any{"id": "", "url": "https://example.com"},
			},
		},
	})
	err := validateBlockDocument(doc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bookmark")
}

func TestValidateBlockDocument_Phase3_TOC_Valid(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "toc",
				"attrs": map[string]any{"id": "blk-toc-1"},
			},
		},
	})
	assert.NoError(t, validateBlockDocument(doc))
}

func TestValidateBlockDocument_Phase3_TOC_MissingID_Error(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "toc",
				"attrs": map[string]any{"id": ""},
			},
		},
	})
	err := validateBlockDocument(doc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "toc")
}

func TestValidateBlockDocument_Phase3_Columns_Valid(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "columns",
				"attrs": map[string]any{"id": "blk-cols-1"},
				"content": []any{
					map[string]any{
						"type": "column",
						"content": []any{
							map[string]any{"type": "paragraph"},
						},
					},
					map[string]any{
						"type": "column",
						"content": []any{
							map[string]any{"type": "paragraph"},
						},
					},
				},
			},
		},
	})
	assert.NoError(t, validateBlockDocument(doc))
}

func TestValidateBlockDocument_Phase3_Columns_MissingID_Error(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "columns",
				"attrs": map[string]any{"id": ""},
			},
		},
	})
	err := validateBlockDocument(doc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "columns")
}

func TestValidateBlockDocument_Phase3_Table_Valid(t *testing.T) {
	// table, tableRow, tableCell, tableHeader are in knownBlockTypes but do NOT require IDs.
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "table",
			},
		},
	})
	assert.NoError(t, validateBlockDocument(doc))
}

func TestValidateBlockDocument_Phase3_TaskList_Valid(t *testing.T) {
	doc := mustMarshal(t, map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "taskList",
			},
		},
	})
	assert.NoError(t, validateBlockDocument(doc))
}

// mustMarshal is a test helper that marshals v to JSON and fails the test on error.
func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// --- Plan 061 — student class-binding access ---
//
// CanViewUnit now lets a student view an org-scope unit when they
// have a class_membership in a class whose course owns the unit's
// topic, AND the unit is in a student-readable status. Below: the
// access matrix.

// wireStudentToUnit creates a course, topic, class, and student
// membership such that `fx.student1` becomes a class member of a
// class whose course's topic is then linked to `unit`. Returns the
// linked unit.
func wireStudentToUnit(t *testing.T, fx *unitFixture, unit *store.TeachingUnit) *store.TeachingUnit {
	t.Helper()
	ctx := context.Background()
	courses := store.NewCourseStore(fx.sqlDB)
	topics := store.NewTopicStore(fx.sqlDB)
	classes := store.NewClassStore(fx.sqlDB)

	course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID:      fx.org1.ID,
		CreatedBy:  fx.teacher1.ID,
		Title:      "Course for Plan 061 " + unit.ID[:8],
		GradeLevel: "K-5",
		Language:   "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.sqlDB.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
	})

	topic, err := topics.CreateTopic(ctx, store.CreateTopicInput{
		CourseID: course.ID,
		Title:    "Topic for Plan 061",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.sqlDB.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID)
	})

	class, err := classes.CreateClass(ctx, store.CreateClassInput{
		CourseID:  course.ID,
		OrgID:     fx.org1.ID,
		Title:     "Class for Plan 061",
		Term:      "fall",
		CreatedBy: fx.teacher1.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.sqlDB.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", class.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", class.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", class.ID)
	})

	_, err = classes.AddClassMember(ctx, store.AddClassMemberInput{
		ClassID: class.ID, UserID: fx.student1.ID, Role: "student",
	})
	require.NoError(t, err)

	linked, err := fx.h.Units.LinkUnitToTopic(ctx, unit.ID, topic.ID)
	require.NoError(t, err)
	require.NotNil(t, linked)
	return linked
}

func TestTeachingUnitHandler_Get_OrgStudent_ViaClassBinding_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "classroom_ready", "Bound Unit", fx.teacher1.ID)
	wireStudentToUnit(t, fx, u)

	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusOK, w.Code, "student in class wired to unit's topic should pass")
}

func TestTeachingUnitHandler_Get_OrgStudent_DraftBound_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	// Status filter blocks even with a binding — drafts/reviewed are
	// teacher-only.
	u := fx.mkUnit(t, "org", &fx.org1.ID, "draft", "Draft Bound", fx.teacher1.ID)
	wireStudentToUnit(t, fx, u)

	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTeachingUnitHandler_Get_OrgStudent_NoBinding_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	// Unit is classroom_ready BUT student has no class membership
	// in a class wired to a topic linked to this unit.
	u := fx.mkUnit(t, "org", &fx.org1.ID, "classroom_ready", "Unbound Unit", fx.teacher1.ID)

	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "student without class binding should be denied")
}

func TestTeachingUnitHandler_Get_OrgStudent_OtherCourseBinding_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	ctx := context.Background()

	// Wire student1 into a class for course-A.
	courses := store.NewCourseStore(fx.sqlDB)
	classes := store.NewClassStore(fx.sqlDB)
	topics := store.NewTopicStore(fx.sqlDB)
	courseA, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: fx.org1.ID, CreatedBy: fx.teacher1.ID, Title: "Course A",
		GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.sqlDB.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", courseA.ID) })
	classA, err := classes.CreateClass(ctx, store.CreateClassInput{
		CourseID: courseA.ID, OrgID: fx.org1.ID, Title: "Class A",
		Term: "fall", CreatedBy: fx.teacher1.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.sqlDB.ExecContext(ctx, "DELETE FROM class_settings WHERE class_id = $1", classA.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM class_memberships WHERE class_id = $1", classA.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM classes WHERE id = $1", classA.ID)
	})
	_, err = classes.AddClassMember(ctx, store.AddClassMemberInput{
		ClassID: classA.ID, UserID: fx.student1.ID, Role: "student",
	})
	require.NoError(t, err)

	// Unit is linked to a topic in course-B (different course).
	courseB, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: fx.org1.ID, CreatedBy: fx.teacher1.ID, Title: "Course B",
		GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.sqlDB.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", courseB.ID) })
	topicB, err := topics.CreateTopic(ctx, store.CreateTopicInput{
		CourseID: courseB.ID, Title: "Topic in B",
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.sqlDB.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topicB.ID) })

	u := fx.mkUnit(t, "org", &fx.org1.ID, "classroom_ready", "B Unit", fx.teacher1.ID)
	_, err = fx.h.Units.LinkUnitToTopic(ctx, u.ID, topicB.ID)
	require.NoError(t, err)

	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "membership in a DIFFERENT course must not grant unit access")
}

func TestTeachingUnitHandler_Get_OrgStudent_NoTopicId_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	// Library content (topic_id NULL) is teacher-only — students
	// see only topic-bound units.
	u := fx.mkUnit(t, "org", &fx.org1.ID, "classroom_ready", "Library Unit", fx.teacher1.ID)
	// (no LinkUnitToTopic call)

	// Make student1 a class member somewhere — proves the test
	// isn't just "no membership at all".
	wireStudentToUnit(t, fx, fx.mkUnit(t, "org", &fx.org1.ID, "classroom_ready", "Wire Bait", fx.teacher1.ID))

	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "unit without topic_id must remain student-invisible")
}

func TestTeachingUnitHandler_Get_OrgStudent_CoachReadyBound_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	u := fx.mkUnit(t, "org", &fx.org1.ID, "coach_ready", "Coach Ready", fx.teacher1.ID)
	wireStudentToUnit(t, fx, u)

	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusOK, w.Code, "coach_ready status should be student-visible")
}

func TestTeachingUnitHandler_Get_OrgStudent_ArchivedBound_200(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	// Archived units stay readable by bound students — read-only
	// historical content. (The plan documents archived alongside
	// classroom_ready and coach_ready.)
	u := fx.mkUnit(t, "org", &fx.org1.ID, "archived", "Archived Unit", fx.teacher1.ID)
	wireStudentToUnit(t, fx, u)

	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusOK, w.Code, "archived status should be student-visible")
}

func TestTeachingUnitHandler_Get_OrgStudent_ReviewedBound_404(t *testing.T) {
	fx := newUnitFixture(t, t.Name())
	// reviewed = teacher-only intermediate state. Even with a
	// binding, students should not see it (matches the plan's
	// "draft/reviewed → student denied" rule).
	u := fx.mkUnit(t, "org", &fx.org1.ID, "reviewed", "Reviewed Unit", fx.teacher1.ID)
	wireStudentToUnit(t, fx, u)

	w := doUnitGet(t, fx.h.GetUnit, "/api/units/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "reviewed status should remain teacher-only")
}
