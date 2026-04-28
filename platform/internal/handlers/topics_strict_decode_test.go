package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// Plan 046: replaced the field-specific deprecation rejection tests
// (topics_deprecation_test.go, deleted in this PR) with generic
// unknown-field rejection. CreateTopic and UpdateTopic now use
// decodeJSONStrict, which mirrors the TS-side `.strict()` zod
// schemas. We use `lessonContent` as the canary field so future
// readers grepping for the deprecated name see the contract.

func setupStrictTopicHandler(t *testing.T) (*TopicHandler, *store.Course, *store.Topic, *store.RegisteredUser) {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()

	orgs := store.NewOrgStore(db)
	users := store.NewUserStore(db)
	topics := store.NewTopicStore(db)
	courses := store.NewCourseStore(db)

	user, err := users.RegisterUser(ctx, store.RegisterInput{
		Name:     "StrictTopic",
		Email:    "stricttopic-" + t.Name() + "@example.com",
		Password: "testpassword123",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM auth_providers WHERE user_id = $1", user.ID)
		db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})

	org, err := orgs.CreateOrg(ctx, store.CreateOrgInput{
		Name: "StrictOrg-" + t.Name(), Slug: "strict-" + t.Name(),
		Type: "school", ContactEmail: "strict@e.com", ContactName: "Admin",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM org_memberships WHERE org_id = $1", org.ID)
		db.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})
	_, err = orgs.UpdateOrgStatus(ctx, org.ID, "active")
	require.NoError(t, err)
	_, err = orgs.AddOrgMember(ctx, store.AddMemberInput{
		OrgID: org.ID, UserID: user.ID, Role: "teacher", Status: "active",
	})
	require.NoError(t, err)

	course, err := courses.CreateCourse(ctx, store.CreateCourseInput{
		OrgID: org.ID, CreatedBy: user.ID,
		Title: "C", GradeLevel: "K-5", Language: "python",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM topics WHERE course_id = $1", course.ID)
		db.ExecContext(ctx, "DELETE FROM courses WHERE id = $1", course.ID)
	})

	topic, err := topics.CreateTopic(ctx, store.CreateTopicInput{
		CourseID: course.ID, Title: "T",
	})
	require.NoError(t, err)

	h := &TopicHandler{
		Topics:  topics,
		Courses: courses,
		Orgs:    orgs,
	}
	return h, course, topic, user
}

func TestCreateTopic_RejectsUnknownField(t *testing.T) {
	h, course, _, user := setupStrictTopicHandler(t)
	body := []byte(`{"title":"X","lessonContent":{"blocks":[]}}`)
	req := httptest.NewRequest(http.MethodPost,
		"/api/courses/"+course.ID+"/topics", bytes.NewReader(body))
	req = withChiParams(
		withClaims(req, &auth.Claims{UserID: user.ID}),
		map[string]string{"courseId": course.ID})
	w := httptest.NewRecorder()
	h.CreateTopic(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, "unknown field must be rejected")
}

func TestUpdateTopic_RejectsUnknownField(t *testing.T) {
	h, course, topic, user := setupStrictTopicHandler(t)
	body := []byte(`{"lessonContent":{"blocks":[]}}`)
	req := httptest.NewRequest(http.MethodPatch,
		"/api/courses/"+course.ID+"/topics/"+topic.ID, bytes.NewReader(body))
	req = withChiParams(
		withClaims(req, &auth.Claims{UserID: user.ID}),
		map[string]string{"courseId": course.ID, "topicId": topic.ID})
	w := httptest.NewRecorder()
	h.UpdateTopic(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, "unknown field must be rejected")
}

// Sanity: the happy path still works (no unknown field).
func TestCreateTopic_TitleOnly_OK(t *testing.T) {
	h, course, _, user := setupStrictTopicHandler(t)
	body := []byte(`{"title":"Happy"}`)
	req := httptest.NewRequest(http.MethodPost,
		"/api/courses/"+course.ID+"/topics", bytes.NewReader(body))
	req = withChiParams(
		withClaims(req, &auth.Claims{UserID: user.ID}),
		map[string]string{"courseId": course.ID})
	w := httptest.NewRecorder()
	h.CreateTopic(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestUpdateTopic_TitleOnly_OK(t *testing.T) {
	h, course, topic, user := setupStrictTopicHandler(t)
	body := []byte(`{"title":"NewTitle"}`)
	req := httptest.NewRequest(http.MethodPatch,
		"/api/courses/"+course.ID+"/topics/"+topic.ID, bytes.NewReader(body))
	req = withChiParams(
		withClaims(req, &auth.Claims{UserID: user.ID}),
		map[string]string{"courseId": course.ID, "topicId": topic.ID})
	w := httptest.NewRecorder()
	h.UpdateTopic(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
