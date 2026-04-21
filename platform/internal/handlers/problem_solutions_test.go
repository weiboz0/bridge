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

	"github.com/weiboz0/bridge/platform/internal/store"
)

// ---------- unit: 401 guards (no DB) ----------

func TestListSolutions_NoClaims(t *testing.T) {
	h := &SolutionHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/problems/abc/solutions", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.ListSolutions(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateSolution_NoClaims(t *testing.T) {
	h := &SolutionHandler{}
	body, _ := json.Marshal(map[string]string{"language": "python", "code": "pass"})
	req := httptest.NewRequest(http.MethodPost, "/api/problems/abc/solutions", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.CreateSolution(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUpdateSolution_NoClaims(t *testing.T) {
	h := &SolutionHandler{}
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPatch, "/api/problems/abc/solutions/def", bytes.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "abc", "solutionId": "def"})
	w := httptest.NewRecorder()
	h.UpdateSolution(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDeleteSolution_NoClaims(t *testing.T) {
	h := &SolutionHandler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/problems/abc/solutions/def", nil)
	req = withChiParams(req, map[string]string{"id": "abc", "solutionId": "def"})
	w := httptest.NewRecorder()
	h.DeleteSolution(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPublishSolution_NoClaims(t *testing.T) {
	h := &SolutionHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/problems/abc/solutions/def/publish", nil)
	req = withChiParams(req, map[string]string{"id": "abc", "solutionId": "def"})
	w := httptest.NewRecorder()
	h.PublishSolution(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUnpublishSolution_NoClaims(t *testing.T) {
	h := &SolutionHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/problems/abc/solutions/def/unpublish", nil)
	req = withChiParams(req, map[string]string{"id": "abc", "solutionId": "def"})
	w := httptest.NewRecorder()
	h.UnpublishSolution(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------- integration tests ----------

// solutionFixture builds on top of the shared problemFixture world, adding a
// SolutionHandler and a helper to create solutions directly in the DB.
type solutionFixture struct {
	*problemFixture
	sh *SolutionHandler
}

func newSolutionFixture(t *testing.T, suffix string) *solutionFixture {
	t.Helper()
	pf := newProblemFixture(t, suffix)
	sh := &SolutionHandler{
		Problems:      pf.h.Problems,
		Solutions:     pf.h.Solutions,
		Orgs:          pf.h.Orgs,
		TopicProblems: pf.h.TopicProblems,
		Topics:        pf.h.Topics,
		Courses:       pf.h.Courses,
	}
	return &solutionFixture{problemFixture: pf, sh: sh}
}

// mkSolution creates a solution in the DB directly and registers cleanup.
func (fx *solutionFixture) mkSolution(t *testing.T, problemID, language, code string, published bool) *store.ProblemSolution {
	t.Helper()
	ctx := context.Background()
	sol, err := fx.sh.Solutions.CreateSolution(ctx, store.CreateSolutionInput{
		ProblemID: problemID,
		Language:  language,
		Code:      code,
		CreatedBy: fx.teacher1.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, sol)
	if published {
		sol, err = fx.sh.Solutions.SetPublished(ctx, sol.ID, true)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		fx.db.ExecContext(ctx, "DELETE FROM problem_solutions WHERE id = $1", sol.ID)
	})
	return sol
}

// doPatch issues an authed PATCH against a handler function.
func doPatch(t *testing.T, h http.HandlerFunc, path string, body any, params map[string]string, claims interface{ toAuthClaims() interface{} }) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(b))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

// ------------------- ListSolutions visibility -------------------

func TestSolutionHandler_List_StudentSeesPublishedOnly(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	// Published platform problem — student1 can view.
	p := fx.mkProblem(t, "platform", nil, "published", "P")
	ctx := context.Background()

	// Create two solutions: one published, one draft.
	pub := fx.mkSolution(t, p.ID, "python", "print(1)", true)
	draft := fx.mkSolution(t, p.ID, "python", "# draft", false)

	w := doGet(t, fx.sh.ListSolutions, "/api/problems/"+p.ID+"/solutions",
		map[string]string{"id": p.ID},
		fx.claims(fx.student1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var list []store.ProblemSolution
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	ids := map[string]bool{}
	for _, s := range list {
		ids[s.ID] = true
	}
	assert.True(t, ids[pub.ID], "published solution must be visible to student")
	assert.False(t, ids[draft.ID], "draft solution must NOT be visible to student")

	// Cleanup avoids foreign key issues.
	_ = ctx
}

func TestSolutionHandler_List_TeacherSeesDraftsAndPublished(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	// Org problem — teacher1 is an editor.
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "OrgP")

	pub := fx.mkSolution(t, p.ID, "python", "print(1)", true)
	draft := fx.mkSolution(t, p.ID, "python", "# draft", false)

	w := doGet(t, fx.sh.ListSolutions, "/api/problems/"+p.ID+"/solutions",
		map[string]string{"id": p.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusOK, w.Code)

	var list []store.ProblemSolution
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	ids := map[string]bool{}
	for _, s := range list {
		ids[s.ID] = true
	}
	assert.True(t, ids[pub.ID], "teacher should see published")
	assert.True(t, ids[draft.ID], "teacher should see drafts")
}

func TestSolutionHandler_List_HiddenProblem404(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	// Draft org problem — student1 can't view.
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "Hidden")
	w := doGet(t, fx.sh.ListSolutions, "/api/problems/"+p.ID+"/solutions",
		map[string]string{"id": p.ID},
		fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ------------------- CreateSolution -------------------

func TestSolutionHandler_Create_TeacherOK(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "P")
	body := map[string]any{"language": "python", "code": "pass"}
	w := doPostJSON(t, fx.sh.CreateSolution, "/api/problems/"+p.ID+"/solutions",
		body, map[string]string{"id": p.ID},
		fx.claims(fx.teacher1, false))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	var sol store.ProblemSolution
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sol))
	assert.Equal(t, p.ID, sol.ProblemID)
	assert.Equal(t, "python", sol.Language)
	assert.False(t, sol.IsPublished)
	// Cleanup
	fx.db.ExecContext(context.Background(), "DELETE FROM problem_solutions WHERE id = $1", sol.ID)
}

func TestSolutionHandler_Create_StudentForbidden(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "P")
	body := map[string]any{"language": "python", "code": "pass"}
	w := doPostJSON(t, fx.sh.CreateSolution, "/api/problems/"+p.ID+"/solutions",
		body, map[string]string{"id": p.ID},
		fx.claims(fx.student1, false))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSolutionHandler_Create_MissingLanguage400(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "P")
	body := map[string]any{"code": "pass"} // no language
	w := doPostJSON(t, fx.sh.CreateSolution, "/api/problems/"+p.ID+"/solutions",
		body, map[string]string{"id": p.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSolutionHandler_Create_MissingCode400(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "P")
	body := map[string]any{"language": "python"} // no code
	w := doPostJSON(t, fx.sh.CreateSolution, "/api/problems/"+p.ID+"/solutions",
		body, map[string]string{"id": p.ID},
		fx.claims(fx.teacher1, false))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ------------------- UpdateSolution -------------------

func TestSolutionHandler_Update_TeacherOK(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "P")
	sol := fx.mkSolution(t, p.ID, "python", "pass", false)

	newCode := "print('hi')"
	body := map[string]any{"code": newCode}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/problems/"+p.ID+"/solutions/"+sol.ID, bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": p.ID, "solutionId": sol.ID})
	w := httptest.NewRecorder()
	fx.sh.UpdateSolution(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var updated store.ProblemSolution
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, newCode, updated.Code)
}

func TestSolutionHandler_Update_StudentForbidden(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "P")
	sol := fx.mkSolution(t, p.ID, "python", "pass", true)

	body := map[string]any{"code": "new"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/problems/"+p.ID+"/solutions/"+sol.ID, bytes.NewReader(b))
	req = withClaims(req, fx.claims(fx.student1, false))
	req = withChiParams(req, map[string]string{"id": p.ID, "solutionId": sol.ID})
	w := httptest.NewRecorder()
	fx.sh.UpdateSolution(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ------------------- Publish / Unpublish -------------------

func TestSolutionHandler_Publish_FlipsToTrue(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "P")
	sol := fx.mkSolution(t, p.ID, "python", "pass", false)
	assert.False(t, sol.IsPublished)

	req := httptest.NewRequest(http.MethodPost, "/api/problems/"+p.ID+"/solutions/"+sol.ID+"/publish", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": p.ID, "solutionId": sol.ID})
	w := httptest.NewRecorder()
	fx.sh.PublishSolution(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var updated store.ProblemSolution
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.True(t, updated.IsPublished)
}

func TestSolutionHandler_Unpublish_FlipsToFalse(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "P")
	sol := fx.mkSolution(t, p.ID, "python", "pass", true)
	assert.True(t, sol.IsPublished)

	req := httptest.NewRequest(http.MethodPost, "/api/problems/"+p.ID+"/solutions/"+sol.ID+"/unpublish", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": p.ID, "solutionId": sol.ID})
	w := httptest.NewRecorder()
	fx.sh.UnpublishSolution(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var updated store.ProblemSolution
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.False(t, updated.IsPublished)
}

func TestSolutionHandler_Publish_Idempotent(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "P")
	sol := fx.mkSolution(t, p.ID, "python", "pass", true) // already published

	req := httptest.NewRequest(http.MethodPost, "/api/problems/"+p.ID+"/solutions/"+sol.ID+"/publish", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": p.ID, "solutionId": sol.ID})
	w := httptest.NewRecorder()
	fx.sh.PublishSolution(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "publish on already-published is idempotent, not 409")
}

func TestSolutionHandler_Unpublish_Idempotent(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "P")
	sol := fx.mkSolution(t, p.ID, "python", "pass", false) // already unpublished

	req := httptest.NewRequest(http.MethodPost, "/api/problems/"+p.ID+"/solutions/"+sol.ID+"/unpublish", nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": p.ID, "solutionId": sol.ID})
	w := httptest.NewRecorder()
	fx.sh.UnpublishSolution(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "unpublish on already-unpublished is idempotent, not 409")
}

func TestSolutionHandler_Publish_StudentForbidden(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "P")
	sol := fx.mkSolution(t, p.ID, "python", "pass", false)

	req := httptest.NewRequest(http.MethodPost, "/api/problems/"+p.ID+"/solutions/"+sol.ID+"/publish", nil)
	req = withClaims(req, fx.claims(fx.student1, false))
	req = withChiParams(req, map[string]string{"id": p.ID, "solutionId": sol.ID})
	w := httptest.NewRecorder()
	fx.sh.PublishSolution(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ------------------- DeleteSolution -------------------

func TestSolutionHandler_Delete_TeacherOK_204(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "P")
	sol := fx.mkSolution(t, p.ID, "python", "pass", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/problems/"+p.ID+"/solutions/"+sol.ID, nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": p.ID, "solutionId": sol.ID})
	w := httptest.NewRecorder()
	fx.sh.DeleteSolution(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSolutionHandler_Delete_StudentForbidden(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p := fx.mkProblem(t, "org", &fx.org1.ID, "published", "P")
	sol := fx.mkSolution(t, p.ID, "python", "pass", true)

	req := httptest.NewRequest(http.MethodDelete, "/api/problems/"+p.ID+"/solutions/"+sol.ID, nil)
	req = withClaims(req, fx.claims(fx.student1, false))
	req = withChiParams(req, map[string]string{"id": p.ID, "solutionId": sol.ID})
	w := httptest.NewRecorder()
	fx.sh.DeleteSolution(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSolutionHandler_Delete_WrongProblem404(t *testing.T) {
	fx := newSolutionFixture(t, t.Name())
	p1 := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "P1")
	p2 := fx.mkProblem(t, "org", &fx.org1.ID, "draft", "P2")
	sol := fx.mkSolution(t, p1.ID, "python", "pass", false)

	// Try to delete sol via p2's URL — should 404 (different problem).
	req := httptest.NewRequest(http.MethodDelete, "/api/problems/"+p2.ID+"/solutions/"+sol.ID, nil)
	req = withClaims(req, fx.claims(fx.teacher1, false))
	req = withChiParams(req, map[string]string{"id": p2.ID, "solutionId": sol.ID})
	w := httptest.NewRecorder()
	fx.sh.DeleteSolution(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
