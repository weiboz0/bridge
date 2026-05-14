package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// collectionFixture is the world a collection integration test runs against.
type collectionFixture struct {
	unitFx *chapterFixture
	ch     *ChapterCollectionHandler
}

func newCollectionFixture(t *testing.T, suffix string) *collectionFixture {
	t.Helper()
	ufx := newChapterFixture(t, suffix)
	ch := &ChapterCollectionHandler{
		Collections: store.NewChapterCollectionStore(ufx.sqlDB),
		Orgs:        store.NewOrgStore(ufx.sqlDB),
		// Plan 052 PR-C: AddItem now verifies the candidate unit is
		// visible to the caller (CanViewChapter). The store is needed
		// for the GetChapter lookup.
		Chapters: store.NewChapterStore(ufx.sqlDB),
	}

	ctx := context.Background()
	t.Cleanup(func() {
		// Clean up collections created by any test user.
		for _, u := range []*store.RegisteredUser{ufx.admin, ufx.teacher1, ufx.student1, ufx.teacher2, ufx.outsider} {
			ufx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_collection_items WHERE collection_id IN (SELECT id FROM chapter_collections WHERE created_by = $1)", u.ID)
			ufx.sqlDB.ExecContext(ctx, "DELETE FROM chapter_collections WHERE created_by = $1", u.ID)
		}
	})

	return &collectionFixture{unitFx: ufx, ch: ch}
}

func (fx *collectionFixture) claims(u *store.RegisteredUser, isPlatformAdmin bool) *auth.Claims {
	return fx.unitFx.claims(u, isPlatformAdmin)
}

// ── Search endpoint tests ──────────────────────────────────────────────────

func TestSearchChapters_FTS_Match(t *testing.T) {
	fx := newChapterFixture(t, t.Name())
	ctx := context.Background()

	// Create an org-scoped unit with "Python Loops" in the title.
	u := fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Python Loops Basics", fx.teacher1.ID)
	_ = u

	// Search as teacher1 (org1 member).
	req := httptest.NewRequest(http.MethodGet, "/api/chapters/search?q=loops", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	w := httptest.NewRecorder()
	fx.h.SearchChapters(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items      []store.Chapter `json:"items"`
		NextCursor *string         `json:"nextCursor"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotEmpty(t, resp.Items, "FTS search for 'loops' should return results")

	found := false
	for _, item := range resp.Items {
		if item.ID == u.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "should find the unit via FTS")
	_ = ctx
}

func TestSearchChapters_ScopeFilter(t *testing.T) {
	fx := newChapterFixture(t, t.Name())

	// Create units in different scopes.
	fx.mkChapter(t, "org", &fx.org1.ID, "draft", "Org Scope Unit", fx.teacher1.ID)
	uid := fx.teacher1.ID
	fx.mkChapter(t, "personal", &uid, "draft", "Personal Scope Unit", fx.teacher1.ID)

	// Search only org scope.
	req := httptest.NewRequest(http.MethodGet, "/api/chapters/search?scope=org", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	w := httptest.NewRecorder()
	fx.h.SearchChapters(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.Chapter `json:"items"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	for _, item := range resp.Items {
		assert.Equal(t, "org", item.Scope, "all results should be org-scoped")
	}
}

func TestSearchChapters_EmptyResults(t *testing.T) {
	fx := newChapterFixture(t, t.Name())

	req := httptest.NewRequest(http.MethodGet, "/api/chapters/search?q=nonexistentquerythatmatchesnothing", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	w := httptest.NewRecorder()
	fx.h.SearchChapters(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.Chapter `json:"items"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Empty(t, resp.Items)
}

func TestSearchChapters_Unauthorized(t *testing.T) {
	fx := newChapterFixture(t, t.Name())

	req := httptest.NewRequest(http.MethodGet, "/api/chapters/search?q=test", nil)
	// No claims.
	w := httptest.NewRecorder()
	fx.h.SearchChapters(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSearchChapters_InvalidScope(t *testing.T) {
	fx := newChapterFixture(t, t.Name())

	req := httptest.NewRequest(http.MethodGet, "/api/chapters/search?scope=invalid", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	w := httptest.NewRecorder()
	fx.h.SearchChapters(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── Collection CRUD tests ──────────────────────────────────────────────────

func TestCollectionHandler_CreateAndGet(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())

	// Create a collection in org1.
	body := map[string]any{
		"scope":       "org",
		"scopeId":     fx.unitFx.org1.ID,
		"title":       "My Collection",
		"description": "A great collection",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	w := httptest.NewRecorder()
	fx.ch.CreateCollection(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var created store.ChapterCollection
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	assert.Equal(t, "My Collection", created.Title)
	assert.Equal(t, "org", created.Scope)
	assert.NotEmpty(t, created.ID)

	// Get it back.
	getReq := httptest.NewRequest(http.MethodGet, "/api/collections/"+created.ID, nil)
	getReq = withClaims(getReq, fx.claims(fx.unitFx.teacher1, false))
	getReq = withChiParams(getReq, map[string]string{"id": created.ID})
	gw := httptest.NewRecorder()
	fx.ch.GetCollection(gw, getReq)

	assert.Equal(t, http.StatusOK, gw.Code)

	var getResp struct {
		Collection store.ChapterCollection       `json:"collection"`
		Items      []store.ChapterCollectionItem `json:"items"`
	}
	require.NoError(t, json.NewDecoder(gw.Body).Decode(&getResp))
	assert.Equal(t, created.ID, getResp.Collection.ID)
	assert.Empty(t, getResp.Items)
}

func TestCollectionHandler_Create_Unauthorized(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())

	body := map[string]any{
		"scope": "org", "scopeId": fx.unitFx.org1.ID,
		"title": "Unauth Collection",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(b))
	// No claims.
	w := httptest.NewRecorder()
	fx.ch.CreateCollection(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCollectionHandler_Create_Forbidden_WrongOrg(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())

	// teacher1 is in org1, not org2.
	body := map[string]any{
		"scope": "org", "scopeId": fx.unitFx.org2.ID,
		"title": "Wrong Org",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	w := httptest.NewRecorder()
	fx.ch.CreateCollection(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCollectionHandler_Create_Student_Forbidden(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())

	// student1 is in org1 but as student — should be denied.
	body := map[string]any{
		"scope": "org", "scopeId": fx.unitFx.org1.ID,
		"title": "Student Collection",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.unitFx.student1, false))
	w := httptest.NewRecorder()
	fx.ch.CreateCollection(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCollectionHandler_Update(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	col, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "org", ScopeID: &fx.unitFx.org1.ID, Title: "Update Me", CreatedBy: fx.unitFx.teacher1.ID,
	})
	require.NoError(t, err)

	body := map[string]any{"title": "Updated Title"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/collections/"+col.ID, bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": col.ID})
	w := httptest.NewRecorder()
	fx.ch.UpdateCollection(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updated store.ChapterCollection
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	assert.Equal(t, "Updated Title", updated.Title)
}

func TestCollectionHandler_Delete(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	col, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "org", ScopeID: &fx.unitFx.org1.ID, Title: "Delete Me", CreatedBy: fx.unitFx.teacher1.ID,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/api/collections/"+col.ID, nil)
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": col.ID})
	w := httptest.NewRecorder()
	fx.ch.DeleteCollection(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify it's gone.
	gone, err := fx.ch.Collections.GetCollection(ctx, col.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
}

func TestCollectionHandler_Get_NotFound(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())

	req := httptest.NewRequest(http.MethodGet, "/api/collections/00000000-0000-0000-0000-000000000000", nil)
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": "00000000-0000-0000-0000-000000000000"})
	w := httptest.NewRecorder()
	fx.ch.GetCollection(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCollectionHandler_Get_CrossOrg_NotFound(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	// Create in org2.
	col, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "org", ScopeID: &fx.unitFx.org2.ID, Title: "Org2 Collection", CreatedBy: fx.unitFx.teacher2.ID,
	})
	require.NoError(t, err)

	// teacher1 (org1 only) tries to get it.
	req := httptest.NewRequest(http.MethodGet, "/api/collections/"+col.ID, nil)
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": col.ID})
	w := httptest.NewRecorder()
	fx.ch.GetCollection(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCollectionHandler_ListCollections(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	_, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "org", ScopeID: &fx.unitFx.org1.ID, Title: "List Col A", CreatedBy: fx.unitFx.teacher1.ID,
	})
	require.NoError(t, err)

	_, err = fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "org", ScopeID: &fx.unitFx.org1.ID, Title: "List Col B", CreatedBy: fx.unitFx.teacher1.ID,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/collections?scope=org", nil)
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	w := httptest.NewRecorder()
	fx.ch.ListCollections(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.ChapterCollection `json:"items"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.GreaterOrEqual(t, len(resp.Items), 2)
}

// ── Collection item endpoint tests ─────────────────────────────────────────

func TestCollectionHandler_AddAndRemoveItem(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	col, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "org", ScopeID: &fx.unitFx.org1.ID, Title: "Items Col", CreatedBy: fx.unitFx.teacher1.ID,
	})
	require.NoError(t, err)

	unit := fx.unitFx.mkChapter(t, "org", &fx.unitFx.org1.ID, "draft", "Item Unit", fx.unitFx.teacher1.ID)

	// Add item.
	addBody := map[string]any{"chapterId": unit.ID, "sortOrder": 1}
	ab, _ := json.Marshal(addBody)
	addReq := httptest.NewRequest(http.MethodPost, "/api/collections/"+col.ID+"/items", bytes.NewReader(ab))
	addReq = withClaims(addReq, fx.claims(fx.unitFx.teacher1, false))
	addReq = withChiParams(addReq, map[string]string{"id": col.ID})
	aw := httptest.NewRecorder()
	fx.ch.AddItem(aw, addReq)

	assert.Equal(t, http.StatusCreated, aw.Code)

	var item store.ChapterCollectionItem
	require.NoError(t, json.NewDecoder(aw.Body).Decode(&item))
	assert.Equal(t, col.ID, item.CollectionID)
	assert.Equal(t, unit.ID, item.ChapterID)
	assert.Equal(t, 1, item.SortOrder)

	// List items.
	listReq := httptest.NewRequest(http.MethodGet, "/api/collections/"+col.ID+"/items", nil)
	listReq = withClaims(listReq, fx.claims(fx.unitFx.teacher1, false))
	listReq = withChiParams(listReq, map[string]string{"id": col.ID})
	lw := httptest.NewRecorder()
	fx.ch.ListItems(lw, listReq)

	assert.Equal(t, http.StatusOK, lw.Code)

	var listResp struct {
		Items []store.ChapterCollectionItem `json:"items"`
	}
	require.NoError(t, json.NewDecoder(lw.Body).Decode(&listResp))
	assert.Len(t, listResp.Items, 1)

	// Remove item.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/collections/"+col.ID+"/items/"+unit.ID, nil)
	delReq = withClaims(delReq, fx.claims(fx.unitFx.teacher1, false))
	delReq = withChiParams(delReq, map[string]string{"id": col.ID, "chapterId": unit.ID})
	dw := httptest.NewRecorder()
	fx.ch.RemoveItem(dw, delReq)

	assert.Equal(t, http.StatusNoContent, dw.Code)

	// Verify removed.
	items, err := fx.ch.Collections.ListItems(ctx, col.ID)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestCollectionHandler_AddItem_InvalidChapterId(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	col, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "org", ScopeID: &fx.unitFx.org1.ID, Title: "Bad Item Col", CreatedBy: fx.unitFx.teacher1.ID,
	})
	require.NoError(t, err)

	body := map[string]any{"chapterId": "not-a-uuid"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/collections/"+col.ID+"/items", bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": col.ID})
	w := httptest.NewRecorder()
	fx.ch.AddItem(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCollectionHandler_AddItem_NonExistentUnit(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	col, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "org", ScopeID: &fx.unitFx.org1.ID, Title: "FK Col", CreatedBy: fx.unitFx.teacher1.ID,
	})
	require.NoError(t, err)

	body := map[string]any{"chapterId": "00000000-0000-0000-0000-000000000099"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/collections/"+col.ID+"/items", bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": col.ID})
	w := httptest.NewRecorder()
	fx.ch.AddItem(w, req)

	// Plan 052 PR-C: was 400 (FK constraint surfaced from the store)
	// in the pre-PR-C behavior. Now AddItem looks up the unit FIRST
	// and returns 404 on missing/invisible unit — matches the
	// don't-leak-existence convention used by canViewChapter elsewhere.
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCollectionHandler_RemoveItem_NotFound(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	col, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "org", ScopeID: &fx.unitFx.org1.ID, Title: "Remove NF Col", CreatedBy: fx.unitFx.teacher1.ID,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/api/collections/"+col.ID+"/items/00000000-0000-0000-0000-000000000099", nil)
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": col.ID, "chapterId": "00000000-0000-0000-0000-000000000099"})
	w := httptest.NewRecorder()
	fx.ch.RemoveItem(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCollectionHandler_PlatformAdmin_SeesAll(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	// Create a personal collection for teacher1.
	uid := fx.unitFx.teacher1.ID
	_, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "personal", ScopeID: &uid, Title: "Personal Col", CreatedBy: fx.unitFx.teacher1.ID,
	})
	require.NoError(t, err)

	// Platform admin lists all.
	req := httptest.NewRequest(http.MethodGet, "/api/collections", nil)
	req = withClaims(req, fx.claims(fx.unitFx.admin, true))
	w := httptest.NewRecorder()
	fx.ch.ListCollections(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []store.ChapterCollection `json:"items"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotEmpty(t, resp.Items, "platform admin should see collections across scopes")
}

func TestCollectionHandler_Create_PersonalScope(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())

	uid := fx.unitFx.teacher1.ID
	body := map[string]any{
		"scope":   "personal",
		"scopeId": uid,
		"title":   "My Personal Collection",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	w := httptest.NewRecorder()
	fx.ch.CreateCollection(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestCollectionHandler_Create_InvalidScope(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())

	body := map[string]any{
		"scope": "invalid",
		"title": "Bad Scope Collection",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	w := httptest.NewRecorder()
	fx.ch.CreateCollection(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCollectionHandler_Create_EmptyTitle(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())

	body := map[string]any{
		"scope":   "org",
		"scopeId": fx.unitFx.org1.ID,
		"title":   "",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
	w := httptest.NewRecorder()
	fx.ch.CreateCollection(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Plan 052 PR-C: AddItem now verifies the candidate unit is visible
// to the caller via CanViewChapter. Without this check, anyone with
// canEditCollection could attach cross-org / draft / personal units
// they have no right to see, leaking content via the collection's
// ListItems / projection paths.
//
// Caller for the matrix below: teacher1 (org1 teacher; can edit
// collections in org1). Candidate units vary by scope / status /
// owner.
func TestCollectionHandler_AddItem_VisibilityMatrix(t *testing.T) {
	cases := []struct {
		name      string
		mkChapter func(fx *collectionFixture) *store.Chapter
		expected  int
	}{
		{
			name: "org1_draft_visible_to_teacher1",
			mkChapter: func(fx *collectionFixture) *store.Chapter {
				return fx.unitFx.mkChapter(t, "org", &fx.unitFx.org1.ID, "draft", "Org1 Draft", fx.unitFx.teacher1.ID)
			},
			expected: http.StatusCreated,
		},
		{
			name: "org2_draft_invisible_to_teacher1",
			mkChapter: func(fx *collectionFixture) *store.Chapter {
				return fx.unitFx.mkChapter(t, "org", &fx.unitFx.org2.ID, "draft", "Org2 Draft", fx.unitFx.teacher2.ID)
			},
			expected: http.StatusNotFound,
		},
		{
			name: "personal_owned_by_teacher2_invisible",
			mkChapter: func(fx *collectionFixture) *store.Chapter {
				return fx.unitFx.mkChapter(t, "personal", &fx.unitFx.teacher2.ID, "draft", "T2 Personal", fx.unitFx.teacher2.ID)
			},
			expected: http.StatusNotFound,
		},
		{
			name: "personal_owned_by_teacher1_visible",
			mkChapter: func(fx *collectionFixture) *store.Chapter {
				return fx.unitFx.mkChapter(t, "personal", &fx.unitFx.teacher1.ID, "draft", "T1 Personal", fx.unitFx.teacher1.ID)
			},
			expected: http.StatusCreated,
		},
		{
			name: "platform_classroom_ready_visible",
			mkChapter: func(fx *collectionFixture) *store.Chapter {
				return fx.unitFx.mkChapter(t, "platform", nil, "classroom_ready", "Platform CR", fx.unitFx.admin.ID)
			},
			expected: http.StatusCreated,
		},
		{
			name: "platform_draft_invisible_to_non_admin",
			mkChapter: func(fx *collectionFixture) *store.Chapter {
				return fx.unitFx.mkChapter(t, "platform", nil, "draft", "Platform Draft", fx.unitFx.admin.ID)
			},
			expected: http.StatusNotFound,
		},
		{
			name: "platform_reviewed_invisible_to_non_admin",
			mkChapter: func(fx *collectionFixture) *store.Chapter {
				return fx.unitFx.mkChapter(t, "platform", nil, "reviewed", "Platform Reviewed", fx.unitFx.admin.ID)
			},
			expected: http.StatusNotFound,
		},
		{
			name: "org1_archived_visible_to_org_teacher",
			mkChapter: func(fx *collectionFixture) *store.Chapter {
				// Org-scope "archived" is visible to org teachers — the
				// status filter only excludes for platform scope. Adding
				// archived org units to a collection is a librarian
				// workflow, so allow it.
				return fx.unitFx.mkChapter(t, "org", &fx.unitFx.org1.ID, "archived", "Org1 Archived", fx.unitFx.teacher1.ID)
			},
			expected: http.StatusCreated,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newCollectionFixture(t, t.Name())
			ctx := context.Background()

			// teacher1 owns the collection (org1 scope).
			col, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
				Scope: "org", ScopeID: &fx.unitFx.org1.ID, Title: "Vis Col", CreatedBy: fx.unitFx.teacher1.ID,
			})
			require.NoError(t, err)

			candidate := tc.mkChapter(fx)
			body, _ := json.Marshal(map[string]any{"chapterId": candidate.ID, "sortOrder": 0})
			req := httptest.NewRequest(http.MethodPost, "/api/collections/"+col.ID+"/items", bytes.NewReader(body))
			req = withClaims(req, fx.claims(fx.unitFx.teacher1, false))
			req = withChiParams(req, map[string]string{"id": col.ID})
			w := httptest.NewRecorder()
			fx.ch.AddItem(w, req)
			assert.Equal(t, tc.expected, w.Code, "case=%s body=%s", tc.name, w.Body.String())
		})
	}
}

// Student in org1 owns a personal collection. They can edit their own
// personal collection (canEditCollection passes for personal-owner).
// They CANNOT attach an org1 draft because students are denied by
// CanViewChapter's org-scope rule (`m.Role == "org_admin" || "teacher"`
// only). 404 — don't leak unit existence.
func TestCollectionHandler_AddItem_StudentCannotAttachOrgUnit(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	// student1's own personal collection.
	col, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "personal", ScopeID: &fx.unitFx.student1.ID, Title: "Student Personal Col", CreatedBy: fx.unitFx.student1.ID,
	})
	require.NoError(t, err)

	// org1 draft — created by teacher1 in student1's own org. Teachers
	// can see it; students can't (per CanViewChapter org rule).
	candidate := fx.unitFx.mkChapter(t, "org", &fx.unitFx.org1.ID, "draft", "Org1 Draft", fx.unitFx.teacher1.ID)

	body, _ := json.Marshal(map[string]any{"chapterId": candidate.ID, "sortOrder": 0})
	req := httptest.NewRequest(http.MethodPost, "/api/collections/"+col.ID+"/items", bytes.NewReader(body))
	req = withClaims(req, fx.claims(fx.unitFx.student1, false))
	req = withChiParams(req, map[string]string{"id": col.ID})
	w := httptest.NewRecorder()
	fx.ch.AddItem(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, "student should not see org-scope unit even in their own org")
}

// Platform admin bypasses CanViewChapter, so they CAN attach an org2
// draft they have no scope-membership for. Uses a personal-scope
// collection owned by the admin so canEditCollection passes
// independently of the visibility check we're testing.
func TestCollectionHandler_AddItem_PlatformAdminBypassesVisibility(t *testing.T) {
	fx := newCollectionFixture(t, t.Name())
	ctx := context.Background()

	col, err := fx.ch.Collections.CreateCollection(ctx, store.CreateCollectionInput{
		Scope: "personal", ScopeID: &fx.unitFx.admin.ID, Title: "Admin Personal Col", CreatedBy: fx.unitFx.admin.ID,
	})
	require.NoError(t, err)

	// org2 draft — invisible to teacher1, visible to platform admin
	// via CanViewChapter's IsPlatformAdmin bypass.
	candidate := fx.unitFx.mkChapter(t, "org", &fx.unitFx.org2.ID, "draft", "Org2 Draft", fx.unitFx.teacher2.ID)

	body, _ := json.Marshal(map[string]any{"chapterId": candidate.ID, "sortOrder": 0})
	req := httptest.NewRequest(http.MethodPost, "/api/collections/"+col.ID+"/items", bytes.NewReader(body))
	req = withClaims(req, fx.claims(fx.unitFx.admin, true))
	req = withChiParams(req, map[string]string{"id": col.ID})
	w := httptest.NewRecorder()
	fx.ch.AddItem(w, req)
	assert.Equal(t, http.StatusCreated, w.Code, "platform admin should bypass CanViewChapter")
}
