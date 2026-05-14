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

// Plan 044 phase 2: TopicHandler.LinkChapter attaches a teaching_unit to a
// topic via chapters.topic_id (1:1 enforced by unique index).

type linkChapterFixture struct {
	h         *TopicHandler
	teacher   *store.RegisteredUser
	outsider  *store.RegisteredUser
	admin     *store.RegisteredUser
	orgID     string
	courseID  string
	topicID   string
	chapterID string
}

func newLinkChapterFixture(t *testing.T, suffix string) *linkChapterFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()

	orgs := store.NewOrgStore(db)
	users := store.NewUserStore(db)
	courses := store.NewCourseStore(db)
	topics := store.NewTopicStore(db)
	units := store.NewChapterStore(db)

	h := &TopicHandler{
		Topics:   topics,
		Courses:  courses,
		Orgs:     orgs,
		Chapters: units,
	}

	mkUser := func(label string) *store.RegisteredUser {
		u, err := users.RegisterUser(ctx, store.RegisterInput{
			Name:     label,
			Email:    label + suffix + "@example.com",
			Password: "testpassword123",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", u.ID)
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		})
		return u
	}

	fx := &linkChapterFixture{h: h}
	fx.teacher = mkUser("teacher" + suffix)
	fx.outsider = mkUser("outsider" + suffix)
	fx.admin = mkUser("admin" + suffix)

	org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name: "OrgL " + suffix, Slug: "orgl-" + suffix,
		Type: "school", ContactEmail: "x" + suffix + "@e.com", ContactName: "Admin",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})
	_, err = orgs.UpdateOrgStatus(ctx, org.ID, "active")
	require.NoError(t, err)
	fx.orgID = org.ID

	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: org.ID, UserID: fx.teacher.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)

	course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: org.ID, CreatedBy: fx.teacher.ID,
		Title: "C", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID) })
	fx.courseID = course.ID

	topic, err := topics.CreateTopic(ctx, store.CreateTopicInput{
		CourseID: course.ID, Title: "T",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", topic.ID) })
	fx.topicID = topic.ID

	// Create a unit owned by the teacher in the same org.
	scopeID := org.ID
	err = db.QueryRowContext(ctx,
		`INSERT INTO chapters
		 (id, scope, scope_id, title, summary, material_type, status, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'org', $1, 'U', '', 'notes', 'draft', $2, now(), now())
		 RETURNING id`,
		scopeID, fx.teacher.ID,
	).Scan(&fx.chapterID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", fx.chapterID)
	})

	return fx
}

func (fx *linkChapterFixture) callLinkChapter(t *testing.T, claims *auth.Claims, chapterID string) (int, []byte) {
	t.Helper()
	body := map[string]string{"chapterId": chapterID}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		"/api/courses/"+fx.courseID+"/topics/"+fx.topicID+"/link-chapter",
		bytes.NewReader(buf))
	req = withChiParams(withClaims(req, claims), map[string]string{
		"courseId": fx.courseID, "topicId": fx.topicID,
	})
	w := httptest.NewRecorder()
	fx.h.LinkChapter(w, req)
	return w.Code, w.Body.Bytes()
}

func TestLinkChapter_NoClaims(t *testing.T) {
	h := &TopicHandler{}
	req := httptest.NewRequest(http.MethodPost,
		"/api/courses/c/topics/t/link-chapter",
		bytes.NewReader([]byte(`{"chapterId":"u"}`)))
	req = withChiParams(req, map[string]string{"courseId": "c", "topicId": "t"})
	w := httptest.NewRecorder()
	h.LinkChapter(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLinkChapter_TeacherCanLinkOwnUnit(t *testing.T) {
	fx := newLinkChapterFixture(t, "ok")
	code, body := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.chapterID)
	require.Equal(t, http.StatusOK, code)

	var unit map[string]any
	require.NoError(t, json.Unmarshal(body, &unit))
	assert.Equal(t, fx.chapterID, unit["id"])
	assert.Equal(t, fx.topicID, unit["topicId"])
}

func TestLinkChapter_PlatformAdmin(t *testing.T) {
	fx := newLinkChapterFixture(t, "admin")
	code, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}, fx.chapterID)
	require.Equal(t, http.StatusOK, code)
}

// Outsider (not the course creator) should be rejected at the
// course-edit gate before unit auth runs.
func TestLinkChapter_Outsider_Forbidden(t *testing.T) {
	fx := newLinkChapterFixture(t, "outsider")
	code, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.outsider.ID}, fx.chapterID)
	assert.Equal(t, http.StatusForbidden, code)
}

// Re-linking the same unit to the same topic is idempotent (200, not
// 409). The unique index doesn't fire because the row already has the
// correct topic_id.
func TestLinkChapter_Idempotent(t *testing.T) {
	fx := newLinkChapterFixture(t, "idem")
	code1, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.chapterID)
	require.Equal(t, http.StatusOK, code1)
	code2, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.chapterID)
	require.Equal(t, http.StatusOK, code2)
}

// Trying to link a topic that already owns a different chapter returns
// 409, not 500. Tests the LinkChapterToTopic store-level check.
func TestLinkChapter_TopicAlreadyLinked_Conflict(t *testing.T) {
	fx := newLinkChapterFixture(t, "conf")
	ctx := context.Background()
	db := integrationDB(t)

	// Link the topic to fx.chapterID first.
	code1, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.chapterID)
	require.Equal(t, http.StatusOK, code1)

	// Create a SECOND unit (also owned by the teacher in the same org).
	var otherChapterID string
	err := db.QueryRowContext(ctx,
		`INSERT INTO chapters
		 (id, scope, scope_id, title, summary, material_type, status, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'org', $1, 'Other', '', 'notes', 'draft', $2, now(), now())
		 RETURNING id`,
		fx.orgID, fx.teacher.ID,
	).Scan(&otherChapterID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", otherChapterID)
	})

	// Try to link the SAME topic to the SECOND unit → 409.
	code2, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, otherChapterID)
	assert.Equal(t, http.StatusConflict, code2)
}

// Codex post-impl review: a direct POST to /link-chapter must NOT silently
// move a Unit that's already attached to a different topic. The picker
// disables those rows in the UI, but the API can still receive the
// request — return 409 to make the conflict explicit.
func TestLinkChapter_UnitAlreadyLinkedToDifferentTopic_Conflict(t *testing.T) {
	fx := newLinkChapterFixture(t, "movegate")
	ctx := context.Background()
	db := integrationDB(t)

	// Create a SECOND topic in the same course.
	var otherTopicID string
	err := db.QueryRowContext(ctx,
		`INSERT INTO topics (id, course_id, title, sort_order, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, 'Other Topic', 1, now(), now())
		 RETURNING id`,
		fx.courseID,
	).Scan(&otherTopicID)
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM topics WHERE id = $1", otherTopicID) })

	// Link fx.chapterID to fx.topicID first.
	code1, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.chapterID)
	require.Equal(t, http.StatusOK, code1)

	// Now try to link the SAME unit to the OTHER topic via a direct
	// POST — should return 409, not silently move it.
	body := map[string]string{"chapterId": fx.chapterID}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		"/api/courses/"+fx.courseID+"/topics/"+otherTopicID+"/link-chapter",
		bytes.NewReader(buf))
	req = withChiParams(
		withClaims(req, &auth.Claims{UserID: fx.teacher.ID}),
		map[string]string{"courseId": fx.courseID, "topicId": otherTopicID})
	w := httptest.NewRecorder()
	fx.h.LinkChapter(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)

	// Verify the unit's topic_id was NOT changed.
	var topicIDAfter string
	err = db.QueryRowContext(ctx,
		"SELECT topic_id FROM chapters WHERE id = $1", fx.chapterID,
	).Scan(&topicIDAfter)
	require.NoError(t, err)
	assert.Equal(t, fx.topicID, topicIDAfter, "unit's topic_id must not have moved silently")
}

func TestLinkChapter_UnitNotFound(t *testing.T) {
	fx := newLinkChapterFixture(t, "miss")
	bogus := "00000000-0000-0000-0000-000000000abc"
	code, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, bogus)
	assert.Equal(t, http.StatusNotFound, code)
}

func TestLinkChapter_MissingUnitId(t *testing.T) {
	fx := newLinkChapterFixture(t, "missing")
	code, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, "")
	assert.Equal(t, http.StatusBadRequest, code)
}

// A unit scoped to a DIFFERENT org must not be linkable, even if the
// caller is the course creator AND owns the unit. The cross-org
// reachability gate matches the read-side join guard
// (scope='platform' OR scope_id = course.org_id).
func TestLinkChapter_WrongOrgUnit_Forbidden(t *testing.T) {
	fx := newLinkChapterFixture(t, "wrongorg")
	ctx := context.Background()
	db := integrationDB(t)

	// Build a SECOND org and a unit scoped there, both owned by the
	// same teacher (so the unit-edit gate would otherwise pass — only
	// the cross-org guard should reject).
	orgs := store.NewOrgStore(db)
	otherOrg, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name: "OrgL wrongorg-other", Slug: "orgl-wrongorg-other",
		Type: "school", ContactEmail: "wrongorg-other@e.com", ContactName: "Admin",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", otherOrg.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", otherOrg.ID)
	})
	_, err = orgs.UpdateOrgStatus(ctx, otherOrg.ID, "active")
	require.NoError(t, err)
	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: otherOrg.ID, UserID: fx.teacher.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)

	var foreignChapterID string
	err = db.QueryRowContext(ctx,
		`INSERT INTO chapters
		 (id, scope, scope_id, title, summary, material_type, status, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'org', $1, 'Foreign', '', 'notes', 'draft', $2, now(), now())
		 RETURNING id`,
		otherOrg.ID, fx.teacher.ID,
	).Scan(&foreignChapterID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", foreignChapterID)
	})

	code, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, foreignChapterID)
	assert.Equal(t, http.StatusForbidden, code)
}

// Plan 045: a teacher (not platform admin) can attach a published
// platform-scope Unit to their topic. Plan 044 forbade this — only
// platform admins could. Plan 045 widens the gate so library Units
// are usable.
func TestLinkChapter_TeacherLinksPlatformPublishedUnit_Allowed(t *testing.T) {
	fx := newLinkChapterFixture(t, "platpub")
	ctx := context.Background()
	db := integrationDB(t)

	var platChapterID string
	err := db.QueryRowContext(ctx,
		`INSERT INTO chapters
		 (id, scope, scope_id, title, summary, material_type, status, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'platform', NULL, 'Platform Lib', '', 'notes', 'classroom_ready', $1, now(), now())
		 RETURNING id`,
		fx.admin.ID, // created by the platform admin
	).Scan(&platChapterID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", platChapterID)
	})

	code, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, platChapterID)
	assert.Equal(t, http.StatusOK, code)
}

// A draft platform-scope Unit (status not in published-statuses) is
// still admin-only.
func TestLinkChapter_TeacherLinksPlatformDraftUnit_Forbidden(t *testing.T) {
	fx := newLinkChapterFixture(t, "platdraft")
	ctx := context.Background()
	db := integrationDB(t)

	var platChapterID string
	err := db.QueryRowContext(ctx,
		`INSERT INTO chapters
		 (id, scope, scope_id, title, summary, material_type, status, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'platform', NULL, 'Platform Draft', '', 'notes', 'draft', $1, now(), now())
		 RETURNING id`,
		fx.admin.ID,
	).Scan(&platChapterID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", platChapterID)
	})

	code, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, platChapterID)
	assert.Equal(t, http.StatusForbidden, code)
}

// Platform admin can still attach a draft platform Unit (sanity check
// the IsPlatformAdmin bypass survives the gate refactor).
func TestLinkChapter_AdminLinksPlatformDraftUnit_Allowed(t *testing.T) {
	fx := newLinkChapterFixture(t, "platdraftadm")
	ctx := context.Background()
	db := integrationDB(t)

	var platChapterID string
	err := db.QueryRowContext(ctx,
		`INSERT INTO chapters
		 (id, scope, scope_id, title, summary, material_type, status, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'platform', NULL, 'Draft', '', 'notes', 'draft', $1, now(), now())
		 RETURNING id`,
		fx.admin.ID,
	).Scan(&platChapterID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", platChapterID)
	})

	code, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}, platChapterID)
	assert.Equal(t, http.StatusOK, code)
}

// A personal-scope unit owned by someone else should be rejected by
// the cross-org reachability guard before the unit-edit check runs.
// Personal-scope units never satisfy `scope='platform' OR scope_id =
// course.org_id`, so they cannot be linked to any course's topic.
func TestLinkChapter_WrongOwnerPersonalUnit_Forbidden(t *testing.T) {
	fx := newLinkChapterFixture(t, "wrongowner")
	ctx := context.Background()
	db := integrationDB(t)

	var personalChapterID string
	err := db.QueryRowContext(ctx,
		`INSERT INTO chapters
		 (id, scope, scope_id, title, summary, material_type, status, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'personal', $1, 'Mine', '', 'notes', 'draft', $1, now(), now())
		 RETURNING id`,
		fx.outsider.ID,
	).Scan(&personalChapterID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM chapters WHERE id = $1", personalChapterID)
	})

	code, _ := fx.callLinkChapter(t,
		&auth.Claims{UserID: fx.teacher.ID}, personalChapterID)
	assert.Equal(t, http.StatusForbidden, code)
}
