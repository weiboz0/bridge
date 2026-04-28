package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/weiboz0/bridge/platform/internal/auth"
)

// Plan 044 phase 3: API contract lock. CreateTopic and UpdateTopic
// reject lessonContent and starterCode in the request body. The columns
// still exist on the table (deprecated; plan 046 drops) but no API
// caller may write through them anymore.

func TestCreateTopic_RejectsLessonContent(t *testing.T) {
	fx := newLinkUnitFixture(t, "create-lc")
	body, _ := json.Marshal(map[string]any{
		"title":         "X",
		"lessonContent": "should be rejected",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/courses/"+fx.courseID+"/topics",
		bytes.NewReader(body))
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.teacher.ID}),
		map[string]string{"courseId": fx.courseID})
	w := httptest.NewRecorder()
	fx.h.CreateTopic(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateTopic_RejectsStarterCode(t *testing.T) {
	fx := newLinkUnitFixture(t, "create-sc")
	body, _ := json.Marshal(map[string]any{
		"title":       "X",
		"starterCode": "print('hi')",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/courses/"+fx.courseID+"/topics",
		bytes.NewReader(body))
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.teacher.ID}),
		map[string]string{"courseId": fx.courseID})
	w := httptest.NewRecorder()
	fx.h.CreateTopic(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateTopic_TitleOnly_OK(t *testing.T) {
	fx := newLinkUnitFixture(t, "create-ok")
	body, _ := json.Marshal(map[string]string{"title": "Loops"})
	req := httptest.NewRequest(http.MethodPost,
		"/api/courses/"+fx.courseID+"/topics",
		bytes.NewReader(body))
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.teacher.ID}),
		map[string]string{"courseId": fx.courseID})
	w := httptest.NewRecorder()
	fx.h.CreateTopic(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestUpdateTopic_RejectsLessonContent(t *testing.T) {
	fx := newLinkUnitFixture(t, "update-lc")
	body, _ := json.Marshal(map[string]any{
		"lessonContent": "should be rejected",
	})
	req := httptest.NewRequest(http.MethodPatch,
		"/api/courses/"+fx.courseID+"/topics/"+fx.topicID,
		bytes.NewReader(body))
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.teacher.ID}),
		map[string]string{"courseId": fx.courseID, "topicId": fx.topicID})
	w := httptest.NewRecorder()
	fx.h.UpdateTopic(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateTopic_RejectsStarterCode(t *testing.T) {
	fx := newLinkUnitFixture(t, "update-sc")
	body, _ := json.Marshal(map[string]any{
		"starterCode": "x = 1",
	})
	req := httptest.NewRequest(http.MethodPatch,
		"/api/courses/"+fx.courseID+"/topics/"+fx.topicID,
		bytes.NewReader(body))
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.teacher.ID}),
		map[string]string{"courseId": fx.courseID, "topicId": fx.topicID})
	w := httptest.NewRecorder()
	fx.h.UpdateTopic(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateTopic_TitleOnly_OK(t *testing.T) {
	fx := newLinkUnitFixture(t, "update-ok")
	body, _ := json.Marshal(map[string]string{"title": "New Title"})
	req := httptest.NewRequest(http.MethodPatch,
		"/api/courses/"+fx.courseID+"/topics/"+fx.topicID,
		bytes.NewReader(body))
	req = withChiParams(withClaims(req, &auth.Claims{UserID: fx.teacher.ID}),
		map[string]string{"courseId": fx.courseID, "topicId": fx.topicID})
	w := httptest.NewRecorder()
	fx.h.UpdateTopic(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
