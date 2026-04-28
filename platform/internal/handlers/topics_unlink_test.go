package handlers

import (
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

// Plan 045: TopicHandler.UnlinkUnit detaches the teaching_unit
// currently linked to a topic. Idempotent (200 when nothing linked).

func (fx *linkUnitFixture) callUnlinkUnit(t *testing.T, claims *auth.Claims) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete,
		"/api/courses/"+fx.courseID+"/topics/"+fx.topicID+"/link-unit", nil)
	req = withChiParams(withClaims(req, claims), map[string]string{
		"courseId": fx.courseID, "topicId": fx.topicID,
	})
	w := httptest.NewRecorder()
	fx.h.UnlinkUnit(w, req)
	return w.Code, w.Body.Bytes()
}

func TestUnlinkUnit_NoClaims(t *testing.T) {
	h := &TopicHandler{}
	req := httptest.NewRequest(http.MethodDelete,
		"/api/courses/c/topics/t/link-unit", nil)
	req = withChiParams(req, map[string]string{"courseId": "c", "topicId": "t"})
	w := httptest.NewRecorder()
	h.UnlinkUnit(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// Happy path: teacher links a Unit, then unlinks it.
func TestUnlinkUnit_TeacherDetachesOwnUnit(t *testing.T) {
	fx := newLinkUnitFixture(t, "unlinkok")
	claims := &auth.Claims{UserID: fx.teacher.ID}

	// Link first.
	linkCode, _ := fx.callLinkUnit(t, claims, fx.unitID)
	require.Equal(t, http.StatusOK, linkCode)

	// Now unlink.
	unlinkCode, body := fx.callUnlinkUnit(t, claims)
	require.Equal(t, http.StatusOK, unlinkCode)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, true, resp["unlinked"])
	assert.Equal(t, fx.unitID, resp["unitId"])

	// Verify the unit's topic_id is NULL afterwards.
	ctx := context.Background()
	db := integrationDB(t)
	var topicIDAfter *string
	err := db.QueryRowContext(ctx,
		"SELECT topic_id FROM teaching_units WHERE id = $1", fx.unitID).Scan(&topicIDAfter)
	require.NoError(t, err)
	assert.Nil(t, topicIDAfter)
}

// Idempotent: unlinking when nothing is linked returns 200, not 404.
func TestUnlinkUnit_WhenNothingLinked_IsIdempotent(t *testing.T) {
	fx := newLinkUnitFixture(t, "unlinkidem")
	code, body := fx.callUnlinkUnit(t,
		&auth.Claims{UserID: fx.teacher.ID})
	require.Equal(t, http.StatusOK, code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, false, resp["unlinked"])
}

// Outsider (not the course creator) is rejected at the course-edit gate
// even when a Unit is currently linked to the topic.
func TestUnlinkUnit_Outsider_Forbidden(t *testing.T) {
	fx := newLinkUnitFixture(t, "unlinkoutsider")

	// Pre-link as the teacher.
	linkCode, _ := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.unitID)
	require.Equal(t, http.StatusOK, linkCode)

	code, _ := fx.callUnlinkUnit(t,
		&auth.Claims{UserID: fx.outsider.ID})
	assert.Equal(t, http.StatusForbidden, code)
}

// Platform admin can unlink any linked Unit.
func TestUnlinkUnit_PlatformAdmin(t *testing.T) {
	fx := newLinkUnitFixture(t, "unlinkadmin")

	linkCode, _ := fx.callLinkUnit(t,
		&auth.Claims{UserID: fx.teacher.ID}, fx.unitID)
	require.Equal(t, http.StatusOK, linkCode)

	code, _ := fx.callUnlinkUnit(t,
		&auth.Claims{UserID: fx.admin.ID, IsPlatformAdmin: true})
	assert.Equal(t, http.StatusOK, code)
}

// Mismatched topic-course path returns 404 (path traversal guard).
func TestUnlinkUnit_MismatchedTopicCoursePath_NotFound(t *testing.T) {
	fx := newLinkUnitFixture(t, "unlinkpath")

	// Build a SECOND course owned by the same teacher.
	ctx := context.Background()
	db := integrationDB(t)
	courses := store.NewCourseStore(db)
	otherCourse, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: fx.orgID, CreatedBy: fx.teacher.ID,
		Title: "Other", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", otherCourse.ID) })

	// Hit /courses/{otherCourse}/topics/{fx.topicID}/link-unit — topic
	// doesn't belong to that course.
	req := httptest.NewRequest(http.MethodDelete,
		"/api/courses/"+otherCourse.ID+"/topics/"+fx.topicID+"/link-unit", nil)
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.teacher.ID}),
		map[string]string{"courseId": otherCourse.ID, "topicId": fx.topicID})
	w := httptest.NewRecorder()
	fx.h.UnlinkUnit(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// A teacher can detach a published platform Unit they previously
// linked — symmetric with the widened link permission.
func TestUnlinkUnit_TeacherDetachesPlatformPublishedUnit(t *testing.T) {
	fx := newLinkUnitFixture(t, "unlinkplat")
	ctx := context.Background()
	db := integrationDB(t)

	var platUnitID string
	err := db.QueryRowContext(ctx,
		`INSERT INTO teaching_units
		 (id, scope, scope_id, title, summary, material_type, status, created_by, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'platform', NULL, 'Lib', '', 'notes', 'classroom_ready', $1, now(), now())
		 RETURNING id`,
		fx.admin.ID,
	).Scan(&platUnitID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM teaching_units WHERE id = $1", platUnitID)
	})

	teacherClaims := &auth.Claims{UserID: fx.teacher.ID}

	linkCode, _ := fx.callLinkUnit(t, teacherClaims, platUnitID)
	require.Equal(t, http.StatusOK, linkCode)

	unlinkCode, _ := fx.callUnlinkUnit(t, teacherClaims)
	assert.Equal(t, http.StatusOK, unlinkCode)
}
