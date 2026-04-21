package handlers

import (
	"bytes"
	"context"
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

// ---------- unit: 401 guards (no DB) ----------

func TestAttachProblem_NoClaims(t *testing.T) {
	h := &TopicProblemHandler{}
	body, _ := json.Marshal(map[string]string{"problemId": "abc"})
	req := httptest.NewRequest(http.MethodPost, "/api/topics/tid/problems", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"topicId": "tid"})
	w := httptest.NewRecorder()
	h.AttachProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDetachProblem_NoClaims(t *testing.T) {
	h := &TopicProblemHandler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/topics/tid/problems/pid", nil)
	req = withChiParams(req, map[string]string{"topicId": "tid", "problemId": "pid"})
	w := httptest.NewRecorder()
	h.DetachProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestReorderProblem_NoClaims(t *testing.T) {
	h := &TopicProblemHandler{}
	body, _ := json.Marshal(map[string]int{"sortOrder": 1})
	req := httptest.NewRequest(http.MethodPatch, "/api/topics/tid/problems/pid", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"topicId": "tid", "problemId": "pid"})
	w := httptest.NewRecorder()
	h.ReorderProblem(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------- integration tests ----------

// topicProblemFixture wraps problemFixture with a TopicProblemHandler.
type topicProblemFixture struct {
	*problemFixture
	tph *TopicProblemHandler
}

func newTopicProblemFixture(t *testing.T, suffix string) *topicProblemFixture {
	t.Helper()
	pf := newProblemFixture(t, suffix)
	tph := &TopicProblemHandler{
		Problems:      pf.h.Problems,
		TopicProblems: pf.h.TopicProblems,
		Topics:        pf.h.Topics,
		Courses:       pf.h.Courses,
		Orgs:          pf.h.Orgs,
	}
	return &topicProblemFixture{problemFixture: pf, tph: tph}
}

// doPatchJSON issues an authed PATCH with a JSON body against a handler function.
func doPatchJSON(t *testing.T, h http.HandlerFunc, path string, body any, params map[string]string, claims *auth.Claims) *httptest.ResponseRecorder {
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

// ------------------- AttachProblem -------------------

func TestTopicProblemHandler_Attach_StudentForbidden(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "P")
	body := map[string]any{"problemId": p.ID}
	w := doPostJSON(t, fx.tph.AttachProblem,
		fmt.Sprintf("/api/topics/%s/problems", fx.topicID),
		body, map[string]string{"topicId": fx.topicID},
		fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestTopicProblemHandler_Attach_DraftProblemConflict(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	// Draft problem — should return 409 (problem not published).
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "Draft")
	body := map[string]any{"problemId": p.ID}
	w := doPostJSON(t, fx.tph.AttachProblem,
		fmt.Sprintf("/api/topics/%s/problems", fx.topicID),
		body, map[string]string{"topicId": fx.topicID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "not published")
}

func TestTopicProblemHandler_Attach_TeacherOK_AppearInList(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	// Teacher attaches a platform published problem to their org's topic.
	p := fx.mkProblem(t, "platform", nil, "published", "Platform Pub")
	body := map[string]any{"problemId": p.ID, "sortOrder": 5}
	w := doPostJSON(t, fx.tph.AttachProblem,
		fmt.Sprintf("/api/topics/%s/problems", fx.topicID),
		body, map[string]string{"topicId": fx.topicID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var att store.TopicProblemAttachment
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &att))
	assert.Equal(t, fx.topicID, att.TopicID)
	assert.Equal(t, p.ID, att.ProblemID)
	assert.Equal(t, 5, att.SortOrder)

	// Verify it appears in GET list.
	wList := doGet(t, fx.h.ListProblemsByTopic,
		fmt.Sprintf("/api/topics/%s/problems", fx.topicID),
		map[string]string{"topicId": fx.topicID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, wList.Code)
	var list []store.Problem
	require.NoError(t, json.Unmarshal(wList.Body.Bytes(), &list))
	found := false
	for _, pr := range list {
		if pr.ID == p.ID {
			found = true
		}
	}
	assert.True(t, found, "attached problem should appear in topic list")

	// Cleanup
	ctx := context.Background()
	fx.h.TopicProblems.Detach(ctx, fx.topicID, p.ID)
}

func TestTopicProblemHandler_Attach_Duplicate409(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "P")
	body := map[string]any{"problemId": p.ID}

	// First attach.
	w := doPostJSON(t, fx.tph.AttachProblem,
		fmt.Sprintf("/api/topics/%s/problems", fx.topicID),
		body, map[string]string{"topicId": fx.topicID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code)

	// Second attach — should be 409.
	w = doPostJSON(t, fx.tph.AttachProblem,
		fmt.Sprintf("/api/topics/%s/problems", fx.topicID),
		body, map[string]string{"topicId": fx.topicID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "already attached")

	// Cleanup
	ctx := context.Background()
	fx.h.TopicProblems.Detach(ctx, fx.topicID, p.ID)
}

func TestTopicProblemHandler_Attach_MissingProblemID400(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	body := map[string]any{} // no problemId
	w := doPostJSON(t, fx.tph.AttachProblem,
		fmt.Sprintf("/api/topics/%s/problems", fx.topicID),
		body, map[string]string{"topicId": fx.topicID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ------------------- DetachProblem -------------------

func TestTopicProblemHandler_Detach_TeacherOK(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "P")
	ctx := context.Background()
	_, err := fx.h.TopicProblems.Attach(ctx, fx.topicID, p.ID, 0, fx.teacher1.ID)
	require.NoError(t, err)

	w := doDelete(t, fx.tph.DetachProblem,
		fmt.Sprintf("/api/topics/%s/problems/%s", fx.topicID, p.ID),
		map[string]string{"topicId": fx.topicID, "problemId": p.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Re-attach should now succeed.
	body := map[string]any{"problemId": p.ID}
	w = doPostJSON(t, fx.tph.AttachProblem,
		fmt.Sprintf("/api/topics/%s/problems", fx.topicID),
		body, map[string]string{"topicId": fx.topicID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code)
	// Final cleanup.
	fx.h.TopicProblems.Detach(ctx, fx.topicID, p.ID)
}

func TestTopicProblemHandler_Detach_StudentForbidden(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "P")
	ctx := context.Background()
	_, err := fx.h.TopicProblems.Attach(ctx, fx.topicID, p.ID, 0, fx.teacher1.ID)
	require.NoError(t, err)
	t.Cleanup(func() { fx.h.TopicProblems.Detach(ctx, fx.topicID, p.ID) })

	w := doDelete(t, fx.tph.DetachProblem,
		fmt.Sprintf("/api/topics/%s/problems/%s", fx.topicID, p.ID),
		map[string]string{"topicId": fx.topicID, "problemId": p.ID},
		fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestTopicProblemHandler_Detach_NotFound404(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "P")
	// Never attached — detach should return 404.
	w := doDelete(t, fx.tph.DetachProblem,
		fmt.Sprintf("/api/topics/%s/problems/%s", fx.topicID, p.ID),
		map[string]string{"topicId": fx.topicID, "problemId": p.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ------------------- ReorderProblem -------------------

func TestTopicProblemHandler_Reorder_ChangesSortOrder(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "P")
	ctx := context.Background()
	_, err := fx.h.TopicProblems.Attach(ctx, fx.topicID, p.ID, 0, fx.teacher1.ID)
	require.NoError(t, err)
	t.Cleanup(func() { fx.h.TopicProblems.Detach(ctx, fx.topicID, p.ID) })

	w := doPatchJSON(t, fx.tph.ReorderProblem,
		fmt.Sprintf("/api/topics/%s/problems/%s", fx.topicID, p.ID),
		map[string]int{"sortOrder": 42},
		map[string]string{"topicId": fx.topicID, "problemId": p.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var att store.TopicProblemAttachment
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &att))
	assert.Equal(t, 42, att.SortOrder)
}

func TestTopicProblemHandler_Reorder_StudentForbidden(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "P")
	ctx := context.Background()
	_, err := fx.h.TopicProblems.Attach(ctx, fx.topicID, p.ID, 0, fx.teacher1.ID)
	require.NoError(t, err)
	t.Cleanup(func() { fx.h.TopicProblems.Detach(ctx, fx.topicID, p.ID) })

	w := doPatchJSON(t, fx.tph.ReorderProblem,
		fmt.Sprintf("/api/topics/%s/problems/%s", fx.topicID, p.ID),
		map[string]int{"sortOrder": 10},
		map[string]string{"topicId": fx.topicID, "problemId": p.ID},
		fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestTopicProblemHandler_Reorder_NotAttached404(t *testing.T) {
	fx := newTopicProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "P")
	// Not attached.
	w := doPatchJSON(t, fx.tph.ReorderProblem,
		fmt.Sprintf("/api/topics/%s/problems/%s", fx.topicID, p.ID),
		map[string]int{"sortOrder": 10},
		map[string]string{"topicId": fx.topicID, "problemId": p.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ------------------- ListTestCases visibility -------------------

func TestProblemHandler_ListTestCases_StudentSeesShellForHiddenCanonical(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "platform", nil, "published", "P")
	ctx := context.Background()

	// Create a hidden canonical case (no owner_id, is_example=false).
	hiddenCase, err := fx.h.TestCases.CreateTestCase(ctx, store.CreateTestCaseInput{
		ProblemID:      p.ID,
		Name:           "hidden",
		Stdin:          "secret input",
		ExpectedStdout: ptr("secret output"),
		IsExample:      false,
		Order:          1,
		// OwnerID = nil → canonical
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM test_cases WHERE id = $1", hiddenCase.ID) })

	// Create a visible example canonical case.
	exampleCase, err := fx.h.TestCases.CreateTestCase(ctx, store.CreateTestCaseInput{
		ProblemID:      p.ID,
		Name:           "example",
		Stdin:          "public input",
		ExpectedStdout: ptr("public output"),
		IsExample:      true,
		Order:          0,
		// OwnerID = nil → canonical
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM test_cases WHERE id = $1", exampleCase.ID) })

	// Student (non-editor) listing test cases.
	w := doGet(t, fx.h.ListTestCases,
		fmt.Sprintf("/api/problems/%s/test-cases", p.ID),
		map[string]string{"id": p.ID},
		fx.claims(fx.student1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var list []store.TestCase
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))

	// Find the hidden case in the response.
	var hiddenInResponse *store.TestCase
	var exampleInResponse *store.TestCase
	for i := range list {
		if list[i].ID == hiddenCase.ID {
			hiddenInResponse = &list[i]
		}
		if list[i].ID == exampleCase.ID {
			exampleInResponse = &list[i]
		}
	}

	// Hidden canonical case must be present but with blanked I/O.
	require.NotNil(t, hiddenInResponse, "hidden canonical case should be present in response (shell)")
	assert.Equal(t, "", hiddenInResponse.Stdin, "hidden canonical Stdin should be blanked for non-editor")
	assert.Nil(t, hiddenInResponse.ExpectedStdout, "hidden canonical ExpectedStdout should be nil for non-editor")

	// Example canonical case must be present with full I/O.
	require.NotNil(t, exampleInResponse, "example case should be present")
	assert.Equal(t, "public input", exampleInResponse.Stdin, "example Stdin should be visible")
	require.NotNil(t, exampleInResponse.ExpectedStdout)
	assert.Equal(t, "public output", *exampleInResponse.ExpectedStdout)
}

func TestProblemHandler_ListTestCases_TeacherSeesFullContent(t *testing.T) {
	fx := newProblemFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "OrgP")
	ctx := context.Background()

	// Hidden canonical case.
	hiddenCase, err := fx.h.TestCases.CreateTestCase(ctx, store.CreateTestCaseInput{
		ProblemID:      p.ID,
		Name:           "hidden",
		Stdin:          "secret input",
		ExpectedStdout: ptr("secret output"),
		IsExample:      false,
		Order:          1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { fx.db.ExecContext(ctx, "DELETE FROM test_cases WHERE id = $1", hiddenCase.ID) })

	// Teacher (editor) listing test cases.
	w := doGet(t, fx.h.ListTestCases,
		fmt.Sprintf("/api/problems/%s/test-cases", p.ID),
		map[string]string{"id": p.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var list []store.TestCase
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))

	var hiddenInResponse *store.TestCase
	for i := range list {
		if list[i].ID == hiddenCase.ID {
			hiddenInResponse = &list[i]
		}
	}
	require.NotNil(t, hiddenInResponse, "editor must see hidden case")
	assert.Equal(t, "secret input", hiddenInResponse.Stdin, "editor should see full Stdin")
	require.NotNil(t, hiddenInResponse.ExpectedStdout)
	assert.Equal(t, "secret output", *hiddenInResponse.ExpectedStdout)
}

// ptr is a helper to get a pointer to a string literal.
func ptr(s string) *string { return &s }
