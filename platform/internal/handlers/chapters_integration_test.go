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

// chapterFixture is the world a Chapter integration test runs against:
// two orgs, a handful of users wired into them in various roles, and a
// fully-built ChapterHandler.
type chapterFixture struct {
	sqlDB    *sql.DB
	h        *ChapterHandler
	org1     *store.Org
	org2     *store.Org
	admin    *store.RegisteredUser // platform admin
	teacher1 *store.RegisteredUser // org1 teacher
	student1 *store.RegisteredUser // org1 student
	teacher2 *store.RegisteredUser // org2 teacher
	outsider *store.RegisteredUser // no orgs
}

// newChapterFixture builds a clean-slate handler + users + orgs.
func newChapterFixture(t *testing.T, suffix string) *chapterFixture {
	t.Helper()
	db := integrationDB(t) // reuses helper from problems_integration_test.go
	ctx := context.Background()

	orgs := store.NewOrgStore(db)
	users := store.NewUserStore(db)

	h := &ChapterHandler{
		Units:   store.NewChapterStore(db),
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
			db.ExecContext(ctx, "DELETE FROM chapter_overlays WHERE child_chapter_id IN (SELECT id FROM chapters WHERE created_by = $1 OR scope_id = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM chapter_overlays WHERE parent_chapter_id IN (SELECT id FROM chapters WHERE created_by = $1 OR scope_id = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM chapter_revisions WHERE chapter_id IN (SELECT id FROM chapters WHERE created_by = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM chapter_revisions WHERE created_by = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM chapter_documents WHERE chapter_id IN (SELECT id FROM chapters WHERE created_by = $1)", u.ID)
			db.ExecContext(ctx, "DELETE FROM chapters WHERE created_by = $1 OR scope_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM org_memberships WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	fx := &chapterFixture{sqlDB: db, h: h}
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

func (fx *chapterFixture) claims(u *store.RegisteredUser, isPlatformAdmin bool) *auth.Claims {
	return &auth.Claims{UserID: u.ID, Email: u.Email, Name: u.Name, IsPlatformAdmin: isPlatformAdmin}
}

// mkChapter creates a teaching unit directly via the store (bypassing the handler)
// and registers a cleanup. Status defaults to "draft".
func (fx *chapterFixture) mkChapter(t *testing.T, scope string, scopeID *string, status, title string, createdBy string) *store.Chapter {
	t.Helper()
	ctx := context.Background()
	u, err := fx.h.Units.CreateChapter(ctx, store.CreateChapterInput{
		Scope:     scope,
		ScopeID:   scopeID,
		Title:     title,
		Status:    status,
		CreatedBy: createdBy,
	})
	require.NoError(t, err)
	require.NotNil(t, u)
	t.Cleanup(func() {
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_overlays WHERE child_chapter_id = $1 OR parent_chapter_id = $1", u.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_revisions WHERE chapter_id = $1", u.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_documents WHERE chapter_id = $1", u.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", u.ID)
	})
	return u
}

// -------------------- HTTP helpers --------------------

func doChapterGet(t *testing.T, h http.HandlerFunc, path string, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
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

func doChapterPost(t *testing.T, h http.HandlerFunc, path string, body any, claims *auth.Claims) *httptest.ResponseRecorder {
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

func doChapterPostWithParams(t *testing.T, h http.HandlerFunc, path string, body any, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
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

func doChapterPatch(t *testing.T, h http.HandlerFunc, path string, body any, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
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

func doChapterDelete(t *testing.T, h http.HandlerFunc, path string, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
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

func doChapterPut(t *testing.T, h http.HandlerFunc, path string, body any, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
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

// ==================== CreateChapter ====================

func TestChapterHandler_Create_PlatformAdmin_201(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	body := map[string]any{"scope": "platform", "title": "Global Unit"}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusCreated, w.Code)
	var resp store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Global Unit", resp.Title)
	assert.Equal(t, "draft", resp.Status)
	if resp.ID != "" {
		ctx := context.Background()
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_documents WHERE chapter_id = $1", resp.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", resp.ID)
	}
}

func TestChapterHandler_Create_OrgTeacher_201(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	body := map[string]any{"scope": "org", "scopeId": fx.org1.ID, "title": "Org Unit"}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusCreated, w.Code)
	var resp store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Org Unit", resp.Title)
	if resp.ID != "" {
		ctx := context.Background()
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_documents WHERE chapter_id = $1", resp.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", resp.ID)
	}
}

func TestChapterHandler_Create_OrgStudent_403(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	body := map[string]any{"scope": "org", "scopeId": fx.org1.ID, "title": "Org Unit"}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestChapterHandler_Create_OrgTeacher_OtherOrg_403(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	// teacher1 is in org1, tries to create in org2.
	body := map[string]any{"scope": "org", "scopeId": fx.org2.ID, "title": "Org2 Unit"}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestChapterHandler_Create_Personal_Self_201(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	body := map[string]any{"scope": "personal", "scopeId": fx.outsider.ID, "title": "My Unit"}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusCreated, w.Code)
	var resp store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	if resp.ID != "" {
		ctx := context.Background()
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_documents WHERE chapter_id = $1", resp.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", resp.ID)
	}
}

func TestChapterHandler_Create_Personal_OtherUser_403(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	body := map[string]any{"scope": "personal", "scopeId": fx.teacher1.ID, "title": "Not Mine"}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestChapterHandler_Create_Platform_NonAdmin_403(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	body := map[string]any{"scope": "platform", "title": "Global Unit"}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestChapterHandler_Create_Platform_WithScopeID_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	body := map[string]any{"scope": "platform", "scopeId": fx.org1.ID, "title": "Bad"}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChapterHandler_Create_EmptyTitle_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	body := map[string]any{"scope": "personal", "scopeId": fx.outsider.ID, "title": ""}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChapterHandler_Create_InvalidScope_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	body := map[string]any{"scope": "galaxy", "title": "Bad Scope"}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChapterHandler_Create_NoAuth_401(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	body := map[string]any{"scope": "personal", "scopeId": fx.outsider.ID, "title": "No auth"}
	w := doChapterPost(t, fx.h.CreateChapter, "/api/chapters", body, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== GetChapter ====================

func TestChapterHandler_Get_OrgTeacher_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Org Draft", fx.teacher1.ID)
	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestChapterHandler_Get_OrgStudent_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "classroom_ready", "Ready Unit", fx.teacher1.ID)
	// Plan 061: org students are denied UNLESS they have a class
	// membership wired to the unit's topic. The fixture's student1
	// has no class binding, so even classroom_ready is invisible.
	// (Bound-success path is in
	// TestChapterHandler_Get_OrgStudent_ViaClassBinding_200.)
	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Get_OrgDraft_OtherOrgTeacher_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Org1 Draft", fx.teacher1.ID)
	// teacher2 is in org2, not org1.
	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Get_PlatformAdmin_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Admin View", fx.teacher1.ID)
	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestChapterHandler_Get_Personal_Owner_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	uid := fx.outsider.ID
	u := fx.mkChapter(t, "personal", &uid, "draft", "My Unit", fx.outsider.ID)
	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestChapterHandler_Get_Personal_NonOwner_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	uid := fx.outsider.ID
	u := fx.mkChapter(t, "personal", &uid, "draft", "My Unit", fx.outsider.ID)
	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Get_Platform_ClassroomReady_AnyAuth_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "platform", nil, "classroom_ready", "Global Ready", fx.admin.ID)
	for _, c := range []*auth.Claims{
		fx.claims(fx.teacher1, false),
		fx.claims(fx.student1, false),
		fx.claims(fx.outsider, false),
	} {
		w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, c)
		assert.Equal(t, http.StatusOK, w.Code, "user %s should see platform classroom_ready unit", c.Email)
	}
}

func TestChapterHandler_Get_Platform_Draft_NonAdmin_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "platform", nil, "draft", "Platform Draft", fx.admin.ID)
	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Get_NotFound_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	fakeID := "00000000-0000-0000-0000-000000000001"
	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+fakeID, map[string]string{"id": fakeID}, fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== UpdateChapter ====================

func TestChapterHandler_Update_OrgTeacher_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Original", fx.teacher1.ID)
	w := doChapterPatch(t, fx.h.UpdateChapter, "/api/chapters/"+u.ID,
		map[string]any{"title": "Updated"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false),
	)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Updated", resp.Title)
}

func TestChapterHandler_Update_OrgStudent_404(t *testing.T) {
	// Students can't view org units in plan 031, so they get 404.
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Org Unit", fx.teacher1.ID)
	w := doChapterPatch(t, fx.h.UpdateChapter, "/api/chapters/"+u.ID,
		map[string]any{"title": "Hack"},
		map[string]string{"id": u.ID},
		fx.claims(fx.student1, false),
	)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Update_Outsider_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Org Unit", fx.teacher1.ID)
	w := doChapterPatch(t, fx.h.UpdateChapter, "/api/chapters/"+u.ID,
		map[string]any{"title": "Hack"},
		map[string]string{"id": u.ID},
		fx.claims(fx.outsider, false),
	)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Update_OrgTeacher_OtherOrg_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Org1 Unit", fx.teacher1.ID)
	// teacher2 is in org2 only — can't view org1 units.
	w := doChapterPatch(t, fx.h.UpdateChapter, "/api/chapters/"+u.ID,
		map[string]any{"title": "Hack"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher2, false),
	)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Update_EmptyTitle_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Original", fx.teacher1.ID)
	w := doChapterPatch(t, fx.h.UpdateChapter, "/api/chapters/"+u.ID,
		map[string]any{"title": ""},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false),
	)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChapterHandler_Update_InvalidStatus_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Original", fx.teacher1.ID)
	w := doChapterPatch(t, fx.h.UpdateChapter, "/api/chapters/"+u.ID,
		map[string]any{"status": "galaxy"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false),
	)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChapterHandler_Update_NoAuth_401(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Org Unit", fx.teacher1.ID)
	w := doChapterPatch(t, fx.h.UpdateChapter, "/api/chapters/"+u.ID,
		map[string]any{"title": "Hack"},
		map[string]string{"id": u.ID},
		nil,
	)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== DeleteChapter ====================

func TestChapterHandler_Delete_OrgTeacher_204(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	// Create directly via store — we don't want the cleanup from mkChapter since deletion is the test.
	ctx := context.Background()
	u, err := fx.h.Units.CreateChapter(ctx, store.CreateChapterInput{
		Scope: "org", ScopeID: &fx.org1.ID,
		Title: "To Delete", Status: "draft", CreatedBy: fx.teacher1.ID,
	})
	require.NoError(t, err)

	w := doChapterDelete(t, fx.h.DeleteChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify the unit is gone.
	got, err := fx.h.Units.GetChapter(ctx, u.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestChapterHandler_Delete_Cascade_DocumentGone(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	u, err := fx.h.Units.CreateChapter(ctx, store.CreateChapterInput{
		Scope: "org", ScopeID: &fx.org1.ID,
		Title: "Has Document", Status: "draft", CreatedBy: fx.teacher1.ID,
	})
	require.NoError(t, err)

	// Confirm chapter_documents row exists after creation.
	doc, err := fx.h.Units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, doc, "CreateChapter should seed a chapter_documents row")

	// Delete via handler.
	w := doChapterDelete(t, fx.h.DeleteChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusNoContent, w.Code)

	// The document row should be cascade-deleted.
	doc2, err := fx.h.Units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	assert.Nil(t, doc2, "chapter_documents row should be deleted by cascade")
}

func TestChapterHandler_Delete_OtherOrgTeacher_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Org1 Unit", fx.teacher1.ID)
	w := doChapterDelete(t, fx.h.DeleteChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Delete_OrgStudent_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Org Unit", fx.teacher1.ID)
	w := doChapterDelete(t, fx.h.DeleteChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== GetDocument ====================

func TestChapterHandler_GetDocument_NewUnit_HasEmptyDoc(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	w := doChapterGet(t, fx.h.GetDocument, "/api/chapters/"+u.ID+"/document", map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
	var resp store.ChapterDocument
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, u.ID, resp.ChapterID)
	assert.NotNil(t, resp.Blocks)
}

func TestChapterHandler_GetDocument_NoAccess_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	w := doChapterGet(t, fx.h.GetDocument, "/api/chapters/"+u.ID+"/document", map[string]string{"id": u.ID}, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== SaveDocument ====================

func TestChapterHandler_SaveDocument_ValidDoc_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)

	validDoc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "prose",
				"attrs": map[string]any{"id": "blk-001"},
			},
		},
	}

	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", validDoc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)

	var resp store.ChapterDocument
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, u.ID, resp.ChapterID)
}

func TestChapterHandler_SaveDocument_Roundtrip(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Roundtrip Unit", fx.teacher1.ID)

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
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	// Read back via GET.
	w2 := doChapterGet(t, fx.h.GetDocument, "/api/chapters/"+u.ID+"/document", map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w2.Code)

	var resp store.ChapterDocument
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))

	var roundtripped map[string]any
	require.NoError(t, json.Unmarshal(resp.Blocks, &roundtripped))
	assert.Equal(t, "doc", roundtripped["type"])
	content, ok := roundtripped["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
}

func TestChapterHandler_SaveDocument_OrgStudent_404(t *testing.T) {
	// Plan-031: org students can't view org units, so they get 404 on document save.
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	doc := map[string]any{"type": "doc", "content": []any{}}
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_SaveDocument_OtherOrgTeacher_NotEditor_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	// teacher2 is in org2, cannot view org1 units → 404.
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	doc := map[string]any{"type": "doc", "content": []any{}}
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---------- Document validation tests ----------

func TestChapterHandler_SaveDocument_InvalidEnvelope_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	badDoc := map[string]any{
		"type":    "not-doc",
		"content": []any{},
	}
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", badDoc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChapterHandler_SaveDocument_BlockMissingAttrsID_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
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
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", badDoc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "index 0")
}

func TestChapterHandler_SaveDocument_BlockMissingAttrsID_AtIndex3_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
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
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "index 3")
}

func TestChapterHandler_SaveDocument_UnknownBlockType_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
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
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", badDoc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "video-embed")
}

func TestChapterHandler_SaveDocument_ProblemRef_Valid(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
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
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestChapterHandler_SaveDocument_EmptyContent_Valid(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Doc Unit", fx.teacher1.ID)
	// An empty content array is valid — no blocks to validate.
	doc := map[string]any{
		"type":    "doc",
		"content": []any{},
	}
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestChapterHandler_SaveDocument_UpdatedAtBumps(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Bump Unit", fx.teacher1.ID)

	before, err := fx.h.Units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, before)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "prose", "attrs": map[string]any{"id": "b1"}},
		},
	}
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc, map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	after, err := fx.h.Units.GetDocument(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, after)
	assert.True(t, !after.UpdatedAt.Before(before.UpdatedAt), "updated_at should be >= before")
}

// ==================== ListChapters ====================

func TestChapterHandler_List_OrgScope_TeacherSeesAll(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	_ = fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Draft", fx.teacher1.ID)
	_ = fx.mkChapter(t, "org", &fx.org1.ID, "classroom_ready", "Ready", fx.teacher1.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/chapters?scope=org&scopeId="+fx.org1.ID, nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	w := httptest.NewRecorder()
	fx.h.ListChapters(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string][]store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp["items"]), 2, "teacher should see at least the 2 units created in this test")
}

func TestChapterHandler_List_OrgScope_StudentSeesNone(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	_ = fx.mkChapter(t, "org", &fx.org1.ID, "classroom_ready", "Ready", fx.teacher1.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/chapters?scope=org&scopeId="+fx.org1.ID, nil)
	req = withClaims(req, fx.claims(fx.student1, false))
	w := httptest.NewRecorder()
	fx.h.ListChapters(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string][]store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// Plan-031: students denied entirely.
	assert.Empty(t, resp["items"])
}

func TestChapterHandler_List_PlatformAdmin_SeesAll(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	_ = fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Draft", fx.teacher1.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/chapters?scope=org&scopeId="+fx.org1.ID, nil)
	req = withClaims(req, fx.claims(fx.admin, true))
	w := httptest.NewRecorder()
	fx.h.ListChapters(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string][]store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp["items"]), 1, "admin should see the draft unit")
}

func TestChapterHandler_List_NoAuth_401(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	req := httptest.NewRequest(http.MethodGet, "/api/chapters?scope=org&scopeId="+fx.org1.ID, nil)
	w := httptest.NewRecorder()
	fx.h.ListChapters(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestChapterHandler_List_InvalidScope_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	req := httptest.NewRequest(http.MethodGet, "/api/chapters?scope=galaxy", nil)
	req = withClaims(req, fx.claims(fx.admin, true))
	w := httptest.NewRecorder()
	fx.h.ListChapters(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==================== GetChapterByTopic ====================

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

func TestChapterHandler_GetChapterByTopic_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()

	// Create an org unit, then link it to a topic.
	u := fx.mkChapter(t, "org", &fx.org1.ID, "classroom_ready", "Topic Unit", fx.teacher1.ID)
	topicID := setupTopicForUnit(t, fx.sqlDB, fx.org1.ID, fx.teacher1.ID, t.Name())

	_, err := fx.sqlDB.ExecContext(ctx,
		`UPDATE chapters SET topic_id = $1 WHERE id = $2`, topicID, u.ID)
	require.NoError(t, err)

	w := doChapterGet(t, fx.h.GetChapterByTopic, "/api/chapters/by-topic/"+topicID,
		map[string]string{"topicId": topicID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)

	var resp store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, u.ID, resp.ID)
	require.NotNil(t, resp.TopicID)
	assert.Equal(t, topicID, *resp.TopicID)
}

func TestChapterHandler_GetChapterByTopic_UnknownID_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	unknownID := "00000000-0000-0000-0000-000000000099"
	w := doChapterGet(t, fx.h.GetChapterByTopic, "/api/chapters/by-topic/"+unknownID,
		map[string]string{"topicId": unknownID},
		fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_GetChapterByTopic_NoAccess_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()

	// Create a personal unit owned by outsider and link it to a topic.
	uid := fx.outsider.ID
	u := fx.mkChapter(t, "personal", &uid, "draft", "Private Unit", fx.outsider.ID)
	topicID := setupTopicForUnit(t, fx.sqlDB, fx.org1.ID, fx.outsider.ID, t.Name())

	_, err := fx.sqlDB.ExecContext(ctx,
		`UPDATE chapters SET topic_id = $1 WHERE id = $2`, topicID, u.ID)
	require.NoError(t, err)

	// teacher1 cannot view a personal unit owned by outsider.
	w := doChapterGet(t, fx.h.GetChapterByTopic, "/api/chapters/by-topic/"+topicID,
		map[string]string{"topicId": topicID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_GetChapterByTopic_NoAuth_401(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	unknownID := "00000000-0000-0000-0000-000000000098"
	w := doChapterGet(t, fx.h.GetChapterByTopic, "/api/chapters/by-topic/"+unknownID,
		map[string]string{"topicId": unknownID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== TransitionChapter ====================

func TestChapterHandler_Transition_DraftToReviewed_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Draft Unit", fx.teacher1.ID)

	w := doChapterPostWithParams(t, fx.h.TransitionChapter, "/api/chapters/"+u.ID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)

	var resp store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "reviewed", resp.Status)
}

func TestChapterHandler_Transition_ReviewedToClassroomReady_200_CreatesRevision(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Publish Unit", fx.teacher1.ID)

	// Save some blocks.
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, u.ID, blocks)
	require.NoError(t, err)

	// draft→reviewed
	w := doChapterPostWithParams(t, fx.h.TransitionChapter, "/api/chapters/"+u.ID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	// reviewed→classroom_ready
	w = doChapterPostWithParams(t, fx.h.TransitionChapter, "/api/chapters/"+u.ID+"/transition",
		map[string]any{"status": "classroom_ready"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "classroom_ready", resp.Status)

	// GET /revisions should show 1 revision.
	w2 := doChapterGet(t, fx.h.ListRevisions, "/api/chapters/"+u.ID+"/revisions",
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w2.Code)

	var revResp struct {
		Items []store.ChapterRevision `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &revResp))
	require.Len(t, revResp.Items, 1)
	require.NotNil(t, revResp.Items[0].Reason)
	assert.Equal(t, "classroom_ready", *revResp.Items[0].Reason)
}

func TestChapterHandler_Transition_InvalidSkip_409(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Skip Unit", fx.teacher1.ID)

	// draft→classroom_ready should be 409 (skips reviewed).
	w := doChapterPostWithParams(t, fx.h.TransitionChapter, "/api/chapters/"+u.ID+"/transition",
		map[string]any{"status": "classroom_ready"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestChapterHandler_Transition_NonEditor_403(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Auth Unit", fx.teacher1.ID)

	// teacher2 is in org2, not org1 — can't even view the unit → 404.
	w := doChapterPostWithParams(t, fx.h.TransitionChapter, "/api/chapters/"+u.ID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)

	// student1 is in org1 but can't view org units in plan-031 → 404.
	w2 := doChapterPostWithParams(t, fx.h.TransitionChapter, "/api/chapters/"+u.ID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": u.ID},
		fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w2.Code)
}

func TestChapterHandler_Transition_NonExistent_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	fakeID := "00000000-0000-0000-0000-000000000001"

	w := doChapterPostWithParams(t, fx.h.TransitionChapter, "/api/chapters/"+fakeID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": fakeID},
		fx.claims(fx.admin, true))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Transition_InvalidTargetStatus_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Bad Status", fx.teacher1.ID)

	w := doChapterPostWithParams(t, fx.h.TransitionChapter, "/api/chapters/"+u.ID+"/transition",
		map[string]any{"status": "galaxy"},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChapterHandler_Transition_NoAuth_401(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "NoAuth Unit", fx.teacher1.ID)

	w := doChapterPostWithParams(t, fx.h.TransitionChapter, "/api/chapters/"+u.ID+"/transition",
		map[string]any{"status": "reviewed"},
		map[string]string{"id": u.ID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== ListRevisions ====================

func TestChapterHandler_ListRevisions_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Rev List Unit", fx.teacher1.ID)

	// Transition through to classroom_ready.
	_, err := fx.h.Units.SetUnitStatus(ctx, u.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, u.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	w := doChapterGet(t, fx.h.ListRevisions, "/api/chapters/"+u.ID+"/revisions",
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.ChapterRevision `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
}

func TestChapterHandler_ListRevisions_NoAccess_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Rev Access Unit", fx.teacher1.ID)

	w := doChapterGet(t, fx.h.ListRevisions, "/api/chapters/"+u.ID+"/revisions",
		map[string]string{"id": u.ID},
		fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== GetRevision ====================

func TestChapterHandler_GetRevision_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Get Rev Unit", fx.teacher1.ID)

	// Create a revision.
	_, err := fx.h.Units.SetUnitStatus(ctx, u.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, u.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	revs, err := fx.h.Units.ListRevisions(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)

	w := doChapterGet(t, fx.h.GetRevision, "/api/chapters/"+u.ID+"/revisions/"+revs[0].ID,
		map[string]string{"id": u.ID, "revisionId": revs[0].ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp store.ChapterRevision
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, revs[0].ID, resp.ID)
	assert.Equal(t, u.ID, resp.ChapterID)
}

func TestChapterHandler_GetRevision_NotFound_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Rev Not Found", fx.teacher1.ID)
	fakeRevID := "00000000-0000-0000-0000-000000000099"

	w := doChapterGet(t, fx.h.GetRevision, "/api/chapters/"+u.ID+"/revisions/"+fakeRevID,
		map[string]string{"id": u.ID, "revisionId": fakeRevID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_GetRevision_WrongUnit_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()

	u1 := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Unit1 Rev", fx.teacher1.ID)
	u2 := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Unit2 Rev", fx.teacher1.ID)

	// Create a revision on u1.
	_, err := fx.h.Units.SetUnitStatus(ctx, u1.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, u1.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	revs, err := fx.h.Units.ListRevisions(ctx, u1.ID)
	require.NoError(t, err)
	require.Len(t, revs, 1)

	// Try to access u1's revision via u2's path → 404.
	w := doChapterGet(t, fx.h.GetRevision, "/api/chapters/"+u2.ID+"/revisions/"+revs[0].ID,
		map[string]string{"id": u2.ID, "revisionId": revs[0].ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== Block Allowlist Expansion (Task 3) ====================

func TestChapterHandler_SaveDocument_TeacherNote_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Note Unit", fx.teacher1.ID)

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

	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc,
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestChapterHandler_SaveDocument_CodeSnippet_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Snippet Unit", fx.teacher1.ID)

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

	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc,
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestChapterHandler_SaveDocument_MediaEmbed_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Media Unit", fx.teacher1.ID)

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

	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc,
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestChapterHandler_SaveDocument_NewBlockMissingID_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "No ID Unit", fx.teacher1.ID)

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

	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document", doc,
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
func saveProjectedDoc(t *testing.T, fx *chapterFixture, unit *store.Chapter) {
	t.Helper()
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+unit.ID+"/document",
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

func TestChapterHandler_Projected_Teacher_Default_SeesAll(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Projected Unit", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+u.ID+"/projected",
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

func TestChapterHandler_Projected_Teacher_PreviewAsStudent(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Preview Unit", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	req := httptest.NewRequest(http.MethodGet, "/api/chapters/"+u.ID+"/projected?role=student", nil)
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

func TestChapterHandler_Projected_Admin_Default_SeesAll(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Admin Projected", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	types := parseProjectedContent(t, w)
	assert.Len(t, types, 11, "admin should see all blocks")
}

func TestChapterHandler_Projected_Student_Default_Filtered(t *testing.T) {
	// Note: students can't view org units in plan-031. Use a platform
	// classroom_ready unit so student can view it.
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "platform", nil, "classroom_ready", "Student Projected", fx.admin.ID)
	// Save doc as admin.
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document",
		projectedDoc, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	// Student requests projected document.
	w2 := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	require.Equal(t, http.StatusOK, w2.Code)

	types := parseProjectedContent(t, w2)
	assert.NotContains(t, types, "teacher-note")
	assert.NotContains(t, types, "live-cue")
	assert.NotContains(t, types, "assignment-variant")
	assert.Len(t, types, 7, "student should see 7 blocks")
}

func TestChapterHandler_Projected_Student_CannotEscalateRole(t *testing.T) {
	// Student requests ?role=teacher but should still get student projection.
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "platform", nil, "classroom_ready", "Escalation Unit", fx.admin.ID)
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document",
		projectedDoc, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest(http.MethodGet, "/api/chapters/"+u.ID+"/projected?role=teacher", nil)
	req = withClaims(req, fx.claims(fx.student1, false))
	req = withChiParams(req, map[string]string{"id": u.ID})
	w2 := httptest.NewRecorder()
	fx.h.GetProjectedDocument(w2, req)

	require.Equal(t, http.StatusOK, w2.Code)
	types := parseProjectedContent(t, w2)
	// Student is locked to student role — teacher-notes should still be omitted.
	assert.NotContains(t, types, "teacher-note")
}

func TestChapterHandler_Projected_SolutionRef_RevealAlways_Student(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "platform", nil, "classroom_ready", "Sol Always", fx.admin.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "solution-ref", "attrs": map[string]any{"id": "sr1", "reveal": "always"}},
		},
	}
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document",
		doc, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	w2 := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	require.Equal(t, http.StatusOK, w2.Code)

	types := parseProjectedContent(t, w2)
	assert.Contains(t, types, "solution-ref")
}

func TestChapterHandler_Projected_SolutionRef_AfterSubmit_NoState_Student_Omitted(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "platform", nil, "classroom_ready", "Sol NoState", fx.admin.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "solution-ref", "attrs": map[string]any{"id": "sr1", "reveal": "after-submit"}},
		},
	}
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document",
		doc, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	w2 := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	require.Equal(t, http.StatusOK, w2.Code)

	types := parseProjectedContent(t, w2)
	assert.NotContains(t, types, "solution-ref", "after-submit with no state should be omitted")
}

func TestChapterHandler_Projected_SolutionRef_AfterSubmit_WithState_Student_Included(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "platform", nil, "classroom_ready", "Sol WithState", fx.admin.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "solution-ref", "attrs": map[string]any{"id": "sr1", "reveal": "after-submit"}},
		},
	}
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document",
		doc, map[string]string{"id": u.ID}, fx.claims(fx.admin, true))
	require.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest(http.MethodGet, "/api/chapters/"+u.ID+"/projected?attemptStates=sr1:submitted", nil)
	req = withClaims(req, fx.claims(fx.student1, false))
	req = withChiParams(req, map[string]string{"id": u.ID})
	w2 := httptest.NewRecorder()
	fx.h.GetProjectedDocument(w2, req)

	require.Equal(t, http.StatusOK, w2.Code)
	types := parseProjectedContent(t, w2)
	assert.Contains(t, types, "solution-ref", "after-submit with submitted state should be included")
}

func TestChapterHandler_Projected_NonViewer_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "No Access", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Projected_NoAuth_401(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "No Auth", fx.teacher1.ID)

	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestChapterHandler_Projected_InvalidRole_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Bad Role", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	req := httptest.NewRequest(http.MethodGet, "/api/chapters/"+u.ID+"/projected?role=superuser", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": u.ID})
	w := httptest.NewRecorder()
	fx.h.GetProjectedDocument(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChapterHandler_Projected_InvalidAttemptState_400(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Bad State", fx.teacher1.ID)
	saveProjectedDoc(t, fx, u)

	req := httptest.NewRequest(http.MethodGet, "/api/chapters/"+u.ID+"/projected?attemptStates=b1:invalid", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": u.ID})
	w := httptest.NewRecorder()
	fx.h.GetProjectedDocument(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChapterHandler_Projected_PersonalUnit_OwnerIsTeacher(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	uid := fx.outsider.ID
	u := fx.mkChapter(t, "personal", &uid, "draft", "Personal Proj", fx.outsider.ID)

	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "teacher-note", "attrs": map[string]any{"id": "tn1"}},
			map[string]any{"type": "prose", "attrs": map[string]any{"id": "p1"}},
		},
	}
	w := doChapterPut(t, fx.h.SaveDocument, "/api/chapters/"+u.ID+"/document",
		doc, map[string]string{"id": u.ID}, fx.claims(fx.outsider, false))
	require.Equal(t, http.StatusOK, w.Code)

	// Owner should see teacher-notes (they're treated as teacher).
	w2 := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.outsider, false))
	require.Equal(t, http.StatusOK, w2.Code)
	types := parseProjectedContent(t, w2)
	assert.Contains(t, types, "teacher-note", "personal unit owner should be treated as teacher")
}

func TestChapterHandler_Projected_EmptyDoc(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Empty Doc", fx.teacher1.ID)

	// The default seeded document should be an empty doc.
	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+u.ID+"/projected",
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

func TestChapterHandler_Projected_NotFound_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	fakeID := "00000000-0000-0000-0000-000000000001"
	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+fakeID+"/projected",
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
func forkChildOf(t *testing.T, fx *chapterFixture, parent *store.Chapter) *store.Chapter {
	t.Helper()
	ctx := context.Background()
	child, err := fx.h.Units.ForkChapter(ctx, parent.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)
	return child
}

func TestChapterHandler_Projected_ForkedUnit_NoOverrides_ShowsParentBlocks(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()

	// Parent has 2 prose blocks; needs to be classroom_ready before
	// the fork can be student-readable.
	parent := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Parent w/ blocks", fx.teacher1.ID)
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

	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+child.ID+"/projected",
		map[string]string{"id": child.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Type    string                   `json:"type"`
		Content []map[string]interface{} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Content, 2, "fork without overrides must show parent's blocks via composition")
}

func TestChapterHandler_Projected_ForkedUnit_HideOverlay_OmitsBlock(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()

	parent := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Hide Parent", fx.teacher1.ID)
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

	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+child.ID+"/projected",
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

func TestChapterHandler_Projected_ForkedUnit_ReplaceOverlay_ShowsReplacement(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()

	parent := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Replace Parent", fx.teacher1.ID)
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

	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+child.ID+"/projected",
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

func TestChapterHandler_Projected_ForkedUnit_TeacherOnlyParent_StudentCantSee(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()

	// Parent has a teacher-note (teacher-only) and a prose block.
	parent := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Teacher-Note Parent", fx.teacher1.ID)
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
	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+child.ID+"/projected?role=student",
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

func TestChapterHandler_Projected_NonForkedUnit_UnchangedBehavior(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()

	// Plain non-forked unit. /projected should still work.
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Plain", fx.teacher1.ID)
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"p1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, u.ID, blocks)
	require.NoError(t, err)

	w := doChapterGet(t, fx.h.GetProjectedDocument, "/api/chapters/"+u.ID+"/projected",
		map[string]string{"id": u.ID}, fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Type    string                   `json:"type"`
		Content []map[string]interface{} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Content, 1)
}

// ==================== ForkChapter ====================

// forkCleanup deletes the child unit and its overlay/doc/revision rows created
// by a fork. Needed for units created via the handler (bypassing mkChapter).
func (fx *chapterFixture) forkCleanup(t *testing.T, childID string) {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() {
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_overlays WHERE child_chapter_id = $1", childID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_revisions WHERE chapter_id = $1", childID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_documents WHERE chapter_id = $1", childID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", childID)
	})
}

func TestChapterHandler_Fork_CreatesChildOverlayDoc(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Fork Source", fx.teacher1.ID)

	w := doChapterPostWithParams(t, fx.h.ForkChapter, "/api/chapters/"+source.ID+"/fork",
		map[string]any{"scope": "org", "scopeId": fx.org1.ID},
		map[string]string{"id": source.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code)

	var child store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &child))
	fx.forkCleanup(t, child.ID)

	assert.Equal(t, "Fork Source (fork)", child.Title)
	assert.Equal(t, "draft", child.Status)
	assert.Equal(t, "org", child.Scope)

	// Overlay must exist.
	ov, err := fx.h.Units.GetOverlay(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, ov)
	assert.Equal(t, child.ID, ov.ChildChapterID)
	assert.Equal(t, source.ID, ov.ParentChapterID)

	// Document must exist.
	doc, err := fx.h.Units.GetDocument(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, doc)
}

func TestChapterHandler_Fork_CustomTitle(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Source", fx.teacher1.ID)

	w := doChapterPostWithParams(t, fx.h.ForkChapter, "/api/chapters/"+source.ID+"/fork",
		map[string]any{"scope": "org", "scopeId": fx.org1.ID, "title": "My Custom Fork"},
		map[string]string{"id": source.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code)

	var child store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &child))
	fx.forkCleanup(t, child.ID)

	assert.Equal(t, "My Custom Fork", child.Title)
}

func TestChapterHandler_Fork_DefaultScopeInference(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	// teacher1 is in exactly one org (org1), so scope defaults to "org".
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Infer Source", fx.teacher1.ID)

	w := doChapterPostWithParams(t, fx.h.ForkChapter, "/api/chapters/"+source.ID+"/fork",
		map[string]any{},
		map[string]string{"id": source.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code)

	var child store.Chapter
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &child))
	fx.forkCleanup(t, child.ID)

	assert.Equal(t, "org", child.Scope)
	require.NotNil(t, child.ScopeID)
	assert.Equal(t, fx.org1.ID, *child.ScopeID)
}

func TestChapterHandler_Fork_SourceNotFound_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	fakeID := "00000000-0000-0000-0000-000000000099"

	w := doChapterPostWithParams(t, fx.h.ForkChapter, "/api/chapters/"+fakeID+"/fork",
		map[string]any{"scope": "personal", "scopeId": fx.teacher1.ID},
		map[string]string{"id": fakeID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Fork_SourceNotVisible_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	// teacher2 can't view org1 units.
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Hidden Source", fx.teacher1.ID)

	w := doChapterPostWithParams(t, fx.h.ForkChapter, "/api/chapters/"+source.ID+"/fork",
		map[string]any{"scope": "org", "scopeId": fx.org2.ID},
		map[string]string{"id": source.ID},
		fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Fork_NotAuthorizedForTargetScope_403(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Auth Source", fx.teacher1.ID)

	// teacher1 tries to fork into org2 where they're not a member.
	w := doChapterPostWithParams(t, fx.h.ForkChapter, "/api/chapters/"+source.ID+"/fork",
		map[string]any{"scope": "org", "scopeId": fx.org2.ID},
		map[string]string{"id": source.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestChapterHandler_Fork_NoAuth_401(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "NoAuth Source", fx.teacher1.ID)

	w := doChapterPostWithParams(t, fx.h.ForkChapter, "/api/chapters/"+source.ID+"/fork",
		map[string]any{"scope": "personal"},
		map[string]string{"id": source.ID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== GetOverlay ====================

func TestChapterHandler_GetOverlay_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Overlay Source", fx.teacher1.ID)

	child, err := fx.h.Units.ForkChapter(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doChapterGet(t, fx.h.GetOverlay, "/api/chapters/"+child.ID+"/overlay",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var ov store.UnitOverlay
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ov))
	assert.Equal(t, child.ID, ov.ChildChapterID)
	assert.Equal(t, source.ID, ov.ParentChapterID)
}

func TestChapterHandler_GetOverlay_NonForkedUnit_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "No Overlay", fx.teacher1.ID)

	w := doChapterGet(t, fx.h.GetOverlay, "/api/chapters/"+u.ID+"/overlay",
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_GetOverlay_NoAccess_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Private Overlay", fx.teacher1.ID)

	child, err := fx.h.Units.ForkChapter(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	// teacher2 (org2) can't view org1 units.
	w := doChapterGet(t, fx.h.GetOverlay, "/api/chapters/"+child.ID+"/overlay",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== PatchOverlay ====================

func TestChapterHandler_PatchOverlay_HideOverride(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Patch Source", fx.teacher1.ID)

	child, err := fx.h.Units.ForkChapter(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doChapterPatch(t, fx.h.PatchOverlay, "/api/chapters/"+child.ID+"/overlay",
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

func TestChapterHandler_PatchOverlay_PinRevision(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Pin Source", fx.teacher1.ID)

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

	child, err := fx.h.Units.ForkChapter(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	// Pin to the revision.
	w := doChapterPatch(t, fx.h.PatchOverlay, "/api/chapters/"+child.ID+"/overlay",
		map[string]any{"parentRevisionId": revs[0].ID},
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var ov store.UnitOverlay
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ov))
	require.NotNil(t, ov.ParentRevisionID)
	assert.Equal(t, revs[0].ID, *ov.ParentRevisionID)
}

func TestChapterHandler_PatchOverlay_FloatBack(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Float Source", fx.teacher1.ID)

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

	child, err := fx.h.Units.ForkChapter(ctx, source.ID, store.ForkTarget{
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
	w := doChapterPatch(t, fx.h.PatchOverlay, "/api/chapters/"+child.ID+"/overlay",
		map[string]any{"parentRevisionId": ""},
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var ov store.UnitOverlay
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ov))
	assert.Nil(t, ov.ParentRevisionID, "empty string should set to NULL (floating)")
}

func TestChapterHandler_PatchOverlay_NotForked_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "No Fork", fx.teacher1.ID)

	w := doChapterPatch(t, fx.h.PatchOverlay, "/api/chapters/"+u.ID+"/overlay",
		map[string]any{"blockOverrides": map[string]any{}},
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_PatchOverlay_NotEditor_403(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "platform", nil, "classroom_ready", "Platform Source", fx.admin.ID)

	child, err := fx.h.Units.ForkChapter(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	// teacher2 is in org2, can't edit org1 units → 404 (can't view).
	w := doChapterPatch(t, fx.h.PatchOverlay, "/api/chapters/"+child.ID+"/overlay",
		map[string]any{"blockOverrides": map[string]any{}},
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher2, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_PatchOverlay_NoAuth_401(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Auth Source", fx.teacher1.ID)

	child, err := fx.h.Units.ForkChapter(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doChapterPatch(t, fx.h.PatchOverlay, "/api/chapters/"+child.ID+"/overlay",
		map[string]any{},
		map[string]string{"id": child.ID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== GetComposedDocument ====================

func TestChapterHandler_Composed_ForkedUnit_EqualsParent(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Compose Source", fx.teacher1.ID)

	// Save blocks to source and publish.
	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"},"content":[{"type":"text","text":"hello"}]}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, source.ID, blocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	child, err := fx.h.Units.ForkChapter(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doChapterGet(t, fx.h.GetComposedDocument, "/api/chapters/"+child.ID+"/composed",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	assert.Equal(t, "doc", doc["type"])
	content := doc["content"].([]interface{})
	require.Len(t, content, 1)
}

func TestChapterHandler_Composed_HideOverride(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Hide Compose", fx.teacher1.ID)

	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}},{"type":"prose","attrs":{"id":"b2"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, source.ID, blocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	child, err := fx.h.Units.ForkChapter(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	// Hide b1.
	_, err = fx.h.Units.UpdateOverlay(ctx, child.ID, store.UpdateOverlayInput{
		BlockOverrides: json.RawMessage(`{"b1":{"action":"hide"}}`),
	})
	require.NoError(t, err)

	w := doChapterGet(t, fx.h.GetComposedDocument, "/api/chapters/"+child.ID+"/composed",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	content := doc["content"].([]interface{})
	assert.Len(t, content, 1, "hide should omit b1")
}

func TestChapterHandler_Composed_ReplaceOverride(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	source := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Replace Compose", fx.teacher1.ID)

	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"b1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, source.ID, blocks)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "reviewed", fx.teacher1.ID)
	require.NoError(t, err)
	_, err = fx.h.Units.SetUnitStatus(ctx, source.ID, "classroom_ready", fx.teacher1.ID)
	require.NoError(t, err)

	child, err := fx.h.Units.ForkChapter(ctx, source.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	_, err = fx.h.Units.UpdateOverlay(ctx, child.ID, store.UpdateOverlayInput{
		BlockOverrides: json.RawMessage(`{"b1":{"action":"replace","block":{"type":"prose","attrs":{"id":"b1-new"}}}}`),
	})
	require.NoError(t, err)

	w := doChapterGet(t, fx.h.GetComposedDocument, "/api/chapters/"+child.ID+"/composed",
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

func TestChapterHandler_Composed_NonForkedUnit(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Plain Unit", fx.teacher1.ID)

	blocks := json.RawMessage(`{"type":"doc","content":[{"type":"prose","attrs":{"id":"p1"}}]}`)
	_, err := fx.h.Units.SaveDocument(ctx, u.ID, blocks)
	require.NoError(t, err)

	w := doChapterGet(t, fx.h.GetComposedDocument, "/api/chapters/"+u.ID+"/composed",
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	assert.Equal(t, "doc", doc["type"])
}

func TestChapterHandler_Composed_NoAccess_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Hidden Compose", fx.teacher1.ID)

	w := doChapterGet(t, fx.h.GetComposedDocument, "/api/chapters/"+u.ID+"/composed",
		map[string]string{"id": u.ID},
		fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Composed_NoAuth_401(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Auth Compose", fx.teacher1.ID)

	w := doChapterGet(t, fx.h.GetComposedDocument, "/api/chapters/"+u.ID+"/composed",
		map[string]string{"id": u.ID},
		nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== GetLineage ====================

func TestChapterHandler_Lineage_NonForked_JustSelf(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Lone Chapter", fx.teacher1.ID)

	w := doChapterGet(t, fx.h.GetLineage, "/api/chapters/"+u.ID+"/lineage",
		map[string]string{"id": u.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.LineageEntry `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, u.ID, resp.Items[0].ChapterID)
}

func TestChapterHandler_Lineage_ChildParent(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	parent := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Parent", fx.teacher1.ID)

	child, err := fx.h.Units.ForkChapter(ctx, parent.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doChapterGet(t, fx.h.GetLineage, "/api/chapters/"+child.ID+"/lineage",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.LineageEntry `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2, "should have parent + child")
	assert.Equal(t, parent.ID, resp.Items[0].ChapterID, "root-first")
	assert.Equal(t, child.ID, resp.Items[1].ChapterID)
}

func TestChapterHandler_Lineage_ThreeGenerations(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()
	grandparent := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Grandparent", fx.teacher1.ID)

	parentUnit, err := fx.h.Units.ForkChapter(ctx, grandparent.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, parentUnit.ID)

	child, err := fx.h.Units.ForkChapter(ctx, parentUnit.ID, store.ForkTarget{
		Scope: "org", ScopeID: &fx.org1.ID, CallerID: fx.teacher1.ID,
	})
	require.NoError(t, err)
	fx.forkCleanup(t, child.ID)

	w := doChapterGet(t, fx.h.GetLineage, "/api/chapters/"+child.ID+"/lineage",
		map[string]string{"id": child.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.LineageEntry `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 3, "grandparent → parent → child = 3 entries")
	assert.Equal(t, grandparent.ID, resp.Items[0].ChapterID)
	assert.Equal(t, parentUnit.ID, resp.Items[1].ChapterID)
	assert.Equal(t, child.ID, resp.Items[2].ChapterID)
}

func TestChapterHandler_Lineage_NoAccess_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Hidden Lineage", fx.teacher1.ID)

	w := doChapterGet(t, fx.h.GetLineage, "/api/chapters/"+u.ID+"/lineage",
		map[string]string{"id": u.ID},
		fx.claims(fx.outsider, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Lineage_NoAuth_401(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Auth Lineage", fx.teacher1.ID)

	w := doChapterGet(t, fx.h.GetLineage, "/api/chapters/"+u.ID+"/lineage",
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
// CanViewChapter now lets a student view an org-scope unit when they
// have a class_membership in a class whose course owns the unit's
// topic, AND the unit is in a student-readable status. Below: the
// access matrix.

// wireStudentToChapter creates a course, topic, class, and student
// membership such that `fx.student1` becomes a class member of a
// class whose course's topic is then linked to `unit`. Returns the
// linked unit.
func wireStudentToChapter(t *testing.T, fx *chapterFixture, unit *store.Chapter) *store.Chapter {
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

	linked, err := fx.h.Units.LinkChapterToTopic(ctx, unit.ID, topic.ID)
	require.NoError(t, err)
	require.NotNil(t, linked)
	return linked
}

func TestChapterHandler_Get_OrgStudent_ViaClassBinding_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "classroom_ready", "Bound Unit", fx.teacher1.ID)
	wireStudentToChapter(t, fx, u)

	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusOK, w.Code, "student in class wired to unit's topic should pass")
}

func TestChapterHandler_Get_OrgStudent_DraftBound_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	// Status filter blocks even with a binding — drafts/reviewed are
	// teacher-only.
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Draft Bound", fx.teacher1.ID)
	wireStudentToChapter(t, fx, u)

	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChapterHandler_Get_OrgStudent_NoBinding_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	// Unit is classroom_ready BUT student has no class membership
	// in a class wired to a topic linked to this unit.
	u := fx.mkChapter(t, "org", &fx.org1.ID, "classroom_ready", "Unbound Unit", fx.teacher1.ID)

	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "student without class binding should be denied")
}

func TestChapterHandler_Get_OrgStudent_OtherCourseBinding_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
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

	u := fx.mkChapter(t, "org", &fx.org1.ID, "classroom_ready", "B Unit", fx.teacher1.ID)
	_, err = fx.h.Units.LinkChapterToTopic(ctx, u.ID, topicB.ID)
	require.NoError(t, err)

	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "membership in a DIFFERENT course must not grant unit access")
}

func TestChapterHandler_Get_OrgStudent_NoTopicId_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	// Library content (topic_id NULL) is teacher-only — students
	// see only topic-bound units.
	u := fx.mkChapter(t, "org", &fx.org1.ID, "classroom_ready", "Library Unit", fx.teacher1.ID)
	// (no LinkChapterToTopic call)

	// Make student1 a class member somewhere — proves the test
	// isn't just "no membership at all".
	wireStudentToChapter(t, fx, fx.mkChapter(t, "org", &fx.org1.ID, "classroom_ready", "Wire Bait", fx.teacher1.ID))

	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "unit without topic_id must remain student-invisible")
}

func TestChapterHandler_Get_OrgStudent_CoachReadyBound_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	u := fx.mkChapter(t, "org", &fx.org1.ID, "coach_ready", "Coach Ready", fx.teacher1.ID)
	wireStudentToChapter(t, fx, u)

	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusOK, w.Code, "coach_ready status should be student-visible")
}

func TestChapterHandler_Get_OrgStudent_ArchivedBound_200(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	// Archived units stay readable by bound students — read-only
	// historical content. (The plan documents archived alongside
	// classroom_ready and coach_ready.)
	u := fx.mkChapter(t, "org", &fx.org1.ID, "archived", "Archived Unit", fx.teacher1.ID)
	wireStudentToChapter(t, fx, u)

	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusOK, w.Code, "archived status should be student-visible")
}

func TestChapterHandler_Get_OrgStudent_ReviewedBound_404(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	// reviewed = teacher-only intermediate state. Even with a
	// binding, students should not see it (matches the plan's
	// "draft/reviewed → student denied" rule).
	u := fx.mkChapter(t, "org", &fx.org1.ID, "reviewed", "Reviewed Unit", fx.teacher1.ID)
	wireStudentToChapter(t, fx, u)

	w := doChapterGet(t, fx.h.GetChapter, "/api/chapters/"+u.ID, map[string]string{"id": u.ID}, fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code, "reviewed status should remain teacher-only")
}
