package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 045 picker mode: GET /api/units/search?linkableForCourse=<id>
// returns Units linkable to that course (platform OR same-org),
// decorated with linkedTopicId / linkedTopicTitle / canLink.

// pickerSearchFixture extends unitFixture with a course owned by
// teacher1 in org1, used as the picker target.
type pickerSearchFixture struct {
	*unitFixture
	courseID  string
	courseOrg *store.Org
}

func newPickerSearchFixture(t *testing.T, suffix string) *pickerSearchFixture {
	t.Helper()
	ufx := newUnitFixture(t, suffix)
	ctx := context.Background()
	courses := store.NewCourseStore(ufx.sqlDB)
	course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID:      ufx.org1.ID,
		CreatedBy:  ufx.teacher1.ID,
		Title:      "Picker Test Course",
		GradeLevel: "K-5",
		Language:   "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		ufx.sqlDB.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", course.ID)
		ufx.sqlDB.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
	})
	return &pickerSearchFixture{unitFixture: ufx, courseID: course.ID, courseOrg: ufx.org1}
}

func (fx *pickerSearchFixture) callPickerSearch(
	t *testing.T, claims *auth.Claims, params url.Values,
) (int, []byte) {
	t.Helper()
	if params == nil {
		params = url.Values{}
	}
	params.Set("linkableForCourse", fx.courseID)
	req := httptest.NewRequest(http.MethodGet, "/api/units/search?"+params.Encode(), nil)
	if claims != nil {
		req = withClaims(req, claims)
	}
	w := httptest.NewRecorder()
	fx.h.SearchUnits(w, req)
	return w.Code, w.Body.Bytes()
}

type pickerResp struct {
	Items []struct {
		ID               string  `json:"id"`
		Scope            string  `json:"scope"`
		Status           string  `json:"status"`
		Title            string  `json:"title"`
		MaterialType     string  `json:"materialType"`
		LinkedTopicID    *string `json:"linkedTopicId"`
		LinkedTopicTitle *string `json:"linkedTopicTitle"`
		CanLink          bool    `json:"canLink"`
		UpdatedAt        string  `json:"updatedAt"`
	} `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

func decodePickerResp(t *testing.T, body []byte) pickerResp {
	t.Helper()
	var resp pickerResp
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}

// Caller without course-edit access (outsider) → 403.
func TestPickerSearch_NonCourseEditor_Forbidden(t *testing.T) {
	fx := newPickerSearchFixture(t, "noedit")
	code, _ := fx.callPickerSearch(t, fx.claims(fx.outsider, false), nil)
	assert.Equal(t, http.StatusForbidden, code)
}

// Personal-scope Units are excluded from picker results.
func TestPickerSearch_PersonalScopeExcluded(t *testing.T) {
	fx := newPickerSearchFixture(t, "personal")
	personalUnit := fx.mkUnit(t, "personal", &fx.teacher1.ID, "draft", "Mine", fx.teacher1.ID)
	code, body := fx.callPickerSearch(t, fx.claims(fx.teacher1, false), nil)
	require.Equal(t, http.StatusOK, code)
	resp := decodePickerResp(t, body)
	for _, item := range resp.Items {
		assert.NotEqual(t, personalUnit.ID, item.ID, "personal-scope must not appear")
	}
}

// Wrong-org (org2) Units are excluded.
func TestPickerSearch_WrongOrgScopeExcluded(t *testing.T) {
	fx := newPickerSearchFixture(t, "wrongorg")
	wrongOrgUnit := fx.mkUnit(t, "org", &fx.org2.ID, "classroom_ready", "Other Org", fx.teacher2.ID)
	code, body := fx.callPickerSearch(t, fx.claims(fx.teacher1, false), nil)
	require.Equal(t, http.StatusOK, code)
	resp := decodePickerResp(t, body)
	for _, item := range resp.Items {
		assert.NotEqual(t, wrongOrgUnit.ID, item.ID, "org-scoped Unit in another org must not appear")
	}
}

// Codex post-impl review: draft platform-scope Units must NOT appear
// in picker results for non-admin callers (info leak — title and
// summary exposed via picker that the regular search hides). Admins
// still see all statuses.
func TestPickerSearch_DraftPlatformUnit_HiddenFromTeacher(t *testing.T) {
	fx := newPickerSearchFixture(t, "platdraft")
	platDraft := fx.mkUnit(t, "platform", nil, "draft", "Platform Draft", fx.admin.ID)
	code, body := fx.callPickerSearch(t, fx.claims(fx.teacher1, false), nil)
	require.Equal(t, http.StatusOK, code)
	resp := decodePickerResp(t, body)

	for _, item := range resp.Items {
		assert.NotEqual(t, platDraft.ID, item.ID,
			"draft platform Unit must not leak title/summary to non-admin picker")
	}
}

// Same draft Unit IS visible to platform admins (sanity-check the
// admin bypass still works).
func TestPickerSearch_DraftPlatformUnit_VisibleToAdmin(t *testing.T) {
	fx := newPickerSearchFixture(t, "platdraftadm")
	platDraft := fx.mkUnit(t, "platform", nil, "draft", "Platform Draft Adm", fx.admin.ID)
	code, body := fx.callPickerSearch(t, fx.claims(fx.admin, true), nil)
	require.Equal(t, http.StatusOK, code)
	resp := decodePickerResp(t, body)

	found := false
	for _, item := range resp.Items {
		if item.ID == platDraft.ID {
			found = true
			assert.True(t, item.CanLink, "admin can link draft platform Units")
		}
	}
	assert.True(t, found, "admin should see draft platform Units in picker")
}

// Published platform-scope Units have canLink=true for course teachers.
func TestPickerSearch_PublishedPlatformUnit_LinkableForTeacher(t *testing.T) {
	fx := newPickerSearchFixture(t, "platpub")
	platPub := fx.mkUnit(t, "platform", nil, "classroom_ready", "Platform Pub", fx.admin.ID)
	code, body := fx.callPickerSearch(t, fx.claims(fx.teacher1, false), nil)
	require.Equal(t, http.StatusOK, code)
	resp := decodePickerResp(t, body)

	found := false
	for _, item := range resp.Items {
		if item.ID == platPub.ID {
			found = true
			assert.True(t, item.CanLink, "published platform Unit should be canLink=true")
		}
	}
	assert.True(t, found)
}

// Already-linked Units appear in results with linkedTopicId set, and
// the linkedTopicTitle is populated when the linked topic's course is
// in the same org.
func TestPickerSearch_AlreadyLinked_SameOrg_TitlePopulated(t *testing.T) {
	fx := newPickerSearchFixture(t, "linkedsame")
	ctx := context.Background()

	// Make a unit in org1, link it to a topic in fx.courseID (also org1).
	unit := fx.mkUnit(t, "org", &fx.org1.ID, "classroom_ready", "Linked Unit", fx.teacher1.ID)

	// Create a topic in fx.courseID.
	var topicID, topicTitle string
	topicTitle = "Topic A"
	err := fx.sqlDB.QueryRowContext(ctx,
		`INSERT INTO topics (id, course_id, title, sort_order, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, 0, now(), now())
		 RETURNING id`,
		fx.courseID, topicTitle,
	).Scan(&topicID)
	require.NoError(t, err)
	t.Cleanup(func() { fx.sqlDB.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topicID) })

	_, err = fx.h.Units.LinkUnitToTopic(ctx, unit.ID, topicID)
	require.NoError(t, err)

	code, body := fx.callPickerSearch(t, fx.claims(fx.teacher1, false), nil)
	require.Equal(t, http.StatusOK, code)
	resp := decodePickerResp(t, body)

	var found *struct {
		ID               string  `json:"id"`
		Scope            string  `json:"scope"`
		Status           string  `json:"status"`
		Title            string  `json:"title"`
		MaterialType     string  `json:"materialType"`
		LinkedTopicID    *string `json:"linkedTopicId"`
		LinkedTopicTitle *string `json:"linkedTopicTitle"`
		CanLink          bool    `json:"canLink"`
		UpdatedAt        string  `json:"updatedAt"`
	}
	for i := range resp.Items {
		if resp.Items[i].ID == unit.ID {
			found = &resp.Items[i]
			break
		}
	}
	require.NotNil(t, found, "linked unit must appear in picker results")
	require.NotNil(t, found.LinkedTopicID)
	assert.Equal(t, topicID, *found.LinkedTopicID)
	require.NotNil(t, found.LinkedTopicTitle)
	assert.Equal(t, topicTitle, *found.LinkedTopicTitle)
}

// Cross-org leak guard: a platform-scope Unit linked to a topic in a
// different org's course shows linkedTopicId BUT linkedTopicTitle is
// redacted to null when the picker's caller is not a platform admin.
func TestPickerSearch_AlreadyLinked_CrossOrg_TitleRedacted(t *testing.T) {
	fx := newPickerSearchFixture(t, "linkedcross")
	ctx := context.Background()

	// Make a platform-scope Unit, link it to a topic in org2's course.
	unit := fx.mkUnit(t, "platform", nil, "classroom_ready", "Shared", fx.admin.ID)

	courses := store.NewCourseStore(fx.sqlDB)
	otherCourse, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: fx.org2.ID, CreatedBy: fx.teacher2.ID,
		Title: "Other Course", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.sqlDB.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", otherCourse.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", otherCourse.ID)
	})

	var otherTopicID string
	err = fx.sqlDB.QueryRowContext(ctx,
		`INSERT INTO topics (id, course_id, title, sort_order, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, 'Other Org Topic Title', 0, now(), now())
		 RETURNING id`,
		otherCourse.ID,
	).Scan(&otherTopicID)
	require.NoError(t, err)

	_, err = fx.h.Units.LinkUnitToTopic(ctx, unit.ID, otherTopicID)
	require.NoError(t, err)

	// Search as teacher1 (org1, NOT platform admin). Cross-org title
	// should be redacted.
	code, body := fx.callPickerSearch(t, fx.claims(fx.teacher1, false), nil)
	require.Equal(t, http.StatusOK, code)
	resp := decodePickerResp(t, body)

	var found bool
	for _, item := range resp.Items {
		if item.ID == unit.ID {
			found = true
			require.NotNil(t, item.LinkedTopicID, "topicId is not sensitive — surfaced")
			assert.Nil(t, item.LinkedTopicTitle, "cross-org topic title MUST be redacted to null")
		}
	}
	require.True(t, found, "cross-org linked Unit should still appear in results")
}

// Platform admins see the cross-org linked topic title (no redaction).
func TestPickerSearch_AlreadyLinked_CrossOrg_AdminSeesTitle(t *testing.T) {
	fx := newPickerSearchFixture(t, "linkedadmin")
	ctx := context.Background()

	unit := fx.mkUnit(t, "platform", nil, "classroom_ready", "Shared Adm", fx.admin.ID)

	courses := store.NewCourseStore(fx.sqlDB)
	otherCourse, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: fx.org2.ID, CreatedBy: fx.teacher2.ID,
		Title: "Adm Other Course", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		fx.sqlDB.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", otherCourse.ID)
		fx.sqlDB.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", otherCourse.ID)
	})

	const otherTitle = "Adm Cross Org Topic"
	var otherTopicID string
	err = fx.sqlDB.QueryRowContext(ctx,
		`INSERT INTO topics (id, course_id, title, sort_order, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, 0, now(), now())
		 RETURNING id`,
		otherCourse.ID, otherTitle,
	).Scan(&otherTopicID)
	require.NoError(t, err)

	_, err = fx.h.Units.LinkUnitToTopic(ctx, unit.ID, otherTopicID)
	require.NoError(t, err)

	code, body := fx.callPickerSearch(t, fx.claims(fx.admin, true), nil)
	require.Equal(t, http.StatusOK, code)
	resp := decodePickerResp(t, body)

	for _, item := range resp.Items {
		if item.ID == unit.ID {
			require.NotNil(t, item.LinkedTopicTitle, "platform admin should see the cross-org title")
			assert.Equal(t, otherTitle, *item.LinkedTopicTitle)
		}
	}
}

// materialType filter narrows results.
func TestPickerSearch_MaterialTypeFilter(t *testing.T) {
	fx := newPickerSearchFixture(t, "mattype")
	notes := fx.mkUnit(t, "platform", nil, "classroom_ready", "Notes Doc", fx.admin.ID)
	// Override the material type via direct SQL since mkUnit defaults to notes.
	_, err := fx.sqlDB.ExecContext(context.Background(),
		"UPDATE teaching_units SET material_type = 'slides' WHERE id = $1", notes.ID)
	require.NoError(t, err)

	worksheet := fx.mkUnit(t, "platform", nil, "classroom_ready", "Worksheet Doc", fx.admin.ID)
	_, err = fx.sqlDB.ExecContext(context.Background(),
		"UPDATE teaching_units SET material_type = 'worksheet' WHERE id = $1", worksheet.ID)
	require.NoError(t, err)

	code, body := fx.callPickerSearch(t,
		fx.claims(fx.teacher1, false),
		url.Values{"materialType": {"slides"}})
	require.Equal(t, http.StatusOK, code)
	resp := decodePickerResp(t, body)

	var sawSlides, sawWorksheet bool
	for _, item := range resp.Items {
		if item.ID == notes.ID {
			sawSlides = true
		}
		if item.ID == worksheet.ID {
			sawWorksheet = true
		}
	}
	assert.True(t, sawSlides, "slides Unit should match materialType=slides")
	assert.False(t, sawWorksheet, "worksheet Unit should NOT match materialType=slides")
}

// Cursor pagination: page 1 returns nextCursor; page 2 with that
// cursor returns disjoint results in the right order.
func TestPickerSearch_CursorPagination(t *testing.T) {
	fx := newPickerSearchFixture(t, "cursor")
	ctx := context.Background()

	// Create 5 platform-published Units with controlled updated_at so
	// the order is deterministic. Updated_at descending = u4, u3, u2, u1, u0.
	ids := make([]string, 5)
	for i := 0; i < 5; i++ {
		u := fx.mkUnit(t, "platform", nil, "classroom_ready",
			"Cursor Unit "+string(rune('A'+i)), fx.admin.ID)
		ids[i] = u.ID
		_, err := fx.sqlDB.ExecContext(ctx,
			"UPDATE teaching_units SET updated_at = $1 WHERE id = $2",
			time.Date(2026, 4, 27, 12, i, 0, 0, time.UTC), u.ID)
		require.NoError(t, err)
	}

	// Page 1: limit=2, expect [u4, u3] (newest first).
	code, body := fx.callPickerSearch(t,
		fx.claims(fx.teacher1, false),
		url.Values{"limit": {"2"}})
	require.Equal(t, http.StatusOK, code)
	page1 := decodePickerResp(t, body)
	require.Len(t, page1.Items, 2)
	require.NotNil(t, page1.NextCursor, "page 1 of 2 must emit a cursor")
	assert.Equal(t, ids[4], page1.Items[0].ID)
	assert.Equal(t, ids[3], page1.Items[1].ID)

	// Page 2: limit=2, cursor=page1.NextCursor — expect [u2, u1].
	code, body = fx.callPickerSearch(t,
		fx.claims(fx.teacher1, false),
		url.Values{"limit": {"2"}, "cursor": {*page1.NextCursor}})
	require.Equal(t, http.StatusOK, code)
	page2 := decodePickerResp(t, body)
	require.Len(t, page2.Items, 2, "page 2 should return exactly 2 items, no overlap")
	assert.Equal(t, ids[2], page2.Items[0].ID, "page 2 must continue from page 1, not restart")
	assert.Equal(t, ids[1], page2.Items[1].ID)
	require.NotNil(t, page2.NextCursor)

	// Page 3: limit=2, cursor=page2.NextCursor — expect [u0] (only one
	// left), no nextCursor.
	code, body = fx.callPickerSearch(t,
		fx.claims(fx.teacher1, false),
		url.Values{"limit": {"2"}, "cursor": {*page2.NextCursor}})
	require.Equal(t, http.StatusOK, code)
	page3 := decodePickerResp(t, body)
	require.Len(t, page3.Items, 1)
	assert.Equal(t, ids[0], page3.Items[0].ID)
	assert.Nil(t, page3.NextCursor, "final page must not emit a cursor")
}

// Malformed cursor returns 400.
func TestPickerSearch_MalformedCursor_BadRequest(t *testing.T) {
	fx := newPickerSearchFixture(t, "badcursor")
	code, _ := fx.callPickerSearch(t,
		fx.claims(fx.teacher1, false),
		url.Values{"cursor": {"not-a-cursor"}})
	assert.Equal(t, http.StatusBadRequest, code)
}
