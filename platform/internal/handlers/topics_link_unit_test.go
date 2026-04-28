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

// Plan 044 phase 2: TopicHandler.LinkUnit attaches a teaching_unit to a
// topic via teaching_units.topic_id (1:1 enforced by unique index).

type linkUnitFixture struct {
	h        *TopicHandler
	teacher  *store.RegisteredUser
	outsider *store.RegisteredUser
	admin    *store.RegisteredUser
	orgID    string
	courseID string
	topicID  string
	unitID   string
}

func newLinkUnitFixture(t *testing.T, suffix string) *linkUnitFixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()

	orgs := store.NewOrgStore(db)
	users := store.NewUserStore(db)
	courses := store.NewCourseStore(db)
	topics := store.NewTopicStore(db)
	units := store.NewTeachingUnitStore(db)

	h := &TopicHandler{
		Topics:        topics,
		Courses:       courses,
		Orgs:          orgs,
		TeachingUnits: units,
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

	fx := &linkUnitFixture{h: h}
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
		`INSERT INTO teaching_units
		 (id, scope, scope_id, title, summary, material_type, status, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'org', $1, 'U', '', 'notes', 'draft', $2, now(), now())
		 RETURNING id`,
		scopeID, fx.teacher.ID,
	).Scan(&fx.unitID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", fx.unitID)
	})

	return fx
}

func (fx *linkUnitFixture) callLinkUnit(t *testing.T, claims *auth.Claims, unitID string) (int, []byte) {
	t.Helper()
	body := map[string]string{"unitId": unitID}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		"/api/courses/"+fx.courseID+"/topics/"+fx.topicID+"/link-unit",
		bytes.NewReader(buf))
	req = withChiParams(withClaims(req, claims), map[string]string{
		"courseId": fx.courseID, "topicId": fx.topicID,
	})
	w := httptest.NewRecorder()
	fx.h.LinkUnit(w, req)
	return w.Code, w.Body.Bytes()
}

func TestLinkUnit_NoClaims(t *testing.T) {
	h := &TopicHandler{}
	req := httptest.NewRequest(http.MethodPost,
		"/api/courses/c/topics/t/link-unit",
		bytes.NewReader([]byte(`{"unitId":"u"}`)))
	req = withChiParams(req, map[string]string{"courseId": "c", "topicId": "t"})
	w := httptest.NewRecorder()
	h.LinkUnit(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLinkUnit_TeacherCanLinkOwnUnit(t *testing.T) {
	fx := newLinkUnitFixture(t, "ok")
	code, body := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.unitID)
	require.Equal(t, http.StatusOK, code)

	var unit map[string]any
	require.NoError(t, json.Unmarshal(body, &unit))
	assert.Equal(t, fx.unitID, unit["id"])
	assert.Equal(t, fx.topicID, unit["topicId"])
}

func TestLinkUnit_PlatformAdmin(t *testing.T) {
	fx := newLinkUnitFixture(t, "admin")
	code, _ := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true}, fx.unitID)
	require.Equal(t, http.StatusOK, code)
}

// Outsider (not the course creator) should be rejected at the
// course-edit gate before unit auth runs.
func TestLinkUnit_Outsider_Forbidden(t *testing.T) {
	fx := newLinkUnitFixture(t, "outsider")
	code, _ := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.outsider.ID}, fx.unitID)
	assert.Equal(t, http.StatusForbidden, code)
}

// Re-linking the same unit to the same topic is idempotent (200, not
// 409). The unique index doesn't fire because the row already has the
// correct topic_id.
func TestLinkUnit_Idempotent(t *testing.T) {
	fx := newLinkUnitFixture(t, "idem")
	code1, _ := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.unitID)
	require.Equal(t, http.StatusOK, code1)
	code2, _ := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.unitID)
	require.Equal(t, http.StatusOK, code2)
}

// Trying to link a topic that already owns a different unit returns
// 409, not 500. Tests the LinkUnitToTopic store-level check.
func TestLinkUnit_TopicAlreadyLinked_Conflict(t *testing.T) {
	fx := newLinkUnitFixture(t, "conf")
	ctx := context.Background()
	db := integrationDB(t)

	// Link the topic to fx.unitID first.
	code1, _ := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.unitID)
	require.Equal(t, http.StatusOK, code1)

	// Create a SECOND unit (also owned by the teacher in the same org).
	var otherUnitID string
	err := db.QueryRowContext(ctx,
		`INSERT INTO teaching_units
		 (id, scope, scope_id, title, summary, material_type, status, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'org', $1, 'Other', '', 'notes', 'draft', $2, now(), now())
		 RETURNING id`,
		fx.orgID, fx.teacher.ID,
	).Scan(&otherUnitID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", otherUnitID)
	})

	// Try to link the SAME topic to the SECOND unit → 409.
	code2, _ := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.teacher.ID}, otherUnitID)
	assert.Equal(t, http.StatusConflict, code2)
}

func TestLinkUnit_UnitNotFound(t *testing.T) {
	fx := newLinkUnitFixture(t, "miss")
	bogus := "00000000-0000-0000-0000-000000000abc"
	code, _ := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.teacher.ID}, bogus)
	assert.Equal(t, http.StatusNotFound, code)
}

func TestLinkUnit_MissingUnitId(t *testing.T) {
	fx := newLinkUnitFixture(t, "missing")
	code, _ := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.teacher.ID}, "")
	assert.Equal(t, http.StatusBadRequest, code)
}
