package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/weiboz0/bridge/platform/internal/sandbox"
	"github.com/weiboz0/bridge/platform/internal/store"
)

// stubPiston implements PistonRunner without booting a real Piston container.
type stubPiston struct {
	respond func(language, code, stdin string) (*sandbox.PistonExecuteResponse, error)
	calls   int
}

func (s *stubPiston) ExecuteWithStdin(_ context.Context, language, code, stdin string) (*sandbox.PistonExecuteResponse, error) {
	s.calls++
	return s.respond(language, code, stdin)
}

func makeStubResp(stdout string) *sandbox.PistonExecuteResponse {
	return &sandbox.PistonExecuteResponse{
		Run: sandbox.PistonStage{Stdout: stdout, Code: 0},
	}
}

// caseFor builds a canonical (no owner) test case with the given stdin and
// optional expected stdout. Pass nil for expected to make it informational.
func caseFor(id, stdin string, expected *string, isExample bool) store.TestCase {
	return store.TestCase{
		ID:             id,
		Stdin:          stdin,
		ExpectedStdout: expected,
		IsExample:      isExample,
	}
}

func ptrStr(s string) *string { return &s }

// ---------- auth-guard tests ----------

func TestRunTest_NoClaims(t *testing.T) {
	h := &AttemptTestHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/attempts/abc/test", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})
	w := httptest.NewRecorder()
	h.RunTest(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRunCaseDiff_NoClaims(t *testing.T) {
	h := &AttemptTestHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/attempts/abc/test/xyz/diff", nil)
	req = withChiParams(req, map[string]string{"id": "abc", "caseId": "xyz"})
	w := httptest.NewRecorder()
	h.RunCaseDiff(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------- executeCases tests (pure, no DB) ----------

func TestExecuteCases_AllPass(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, stdin string) (*sandbox.PistonExecuteResponse, error) {
			return makeStubResp(stdin), nil // echo
		},
	}}
	cases := []store.TestCase{
		caseFor("c1", "hi", ptrStr("hi"), true),
		caseFor("c2", "bye", ptrStr("bye"), false),
	}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 2, s.Summary.Passed)
	assert.Equal(t, 0, s.Summary.Failed)
	assert.Equal(t, 2, s.Summary.Total)
}

func TestExecuteCases_OneFail_HiddenLeaksNoDetail(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, stdin string) (*sandbox.PistonExecuteResponse, error) {
			if stdin == "wrong" {
				return makeStubResp("not what was expected"), nil
			}
			return makeStubResp(stdin), nil
		},
	}}
	cases := []store.TestCase{
		caseFor("ex", "hi", ptrStr("hi"), true),
		caseFor("hidden", "wrong", ptrStr("wrong"), false),
	}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 1, s.Summary.Passed)
	assert.Equal(t, 1, s.Summary.Failed)

	var failedHidden *caseResult
	for i := range s.Cases {
		if s.Cases[i].Status == "fail" {
			failedHidden = &s.Cases[i]
		}
	}
	if assert.NotNil(t, failedHidden) {
		assert.Equal(t, "wrong_output", failedHidden.Reason)
		assert.False(t, failedHidden.IsExample, "the failing case is the hidden one")
	}
}

func TestExecuteCases_NormalizesTrailingNewlineAndCRLF(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, _ string) (*sandbox.PistonExecuteResponse, error) {
			return makeStubResp("hello\n"), nil // Piston-like trailing \n
		},
	}}
	// Expected has CRLF line ending.
	cases := []store.TestCase{caseFor("c1", "x", ptrStr("hello\r\n"), true)}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 1, s.Summary.Passed, "trailing newline + CRLF should normalize away")
}

func TestExecuteCases_NilExpectedIsInformational(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, _ string) (*sandbox.PistonExecuteResponse, error) {
			return makeStubResp("anything goes"), nil
		},
	}}
	cases := []store.TestCase{caseFor("c1", "in", nil, false)}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 1, s.Summary.Passed, "no expected_stdout = pass-on-execute")
}

func TestExecuteCases_TimeoutMarksTimeout(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, _ string) (*sandbox.PistonExecuteResponse, error) {
			time.Sleep(perCaseTimeout + 100*time.Millisecond)
			return nil, context.DeadlineExceeded
		},
	}}
	cases := []store.TestCase{caseFor("c1", "x", ptrStr("y"), true)}
	s := h.executeCases(context.Background(), "python", "code", cases)
	assert.Equal(t, 0, s.Summary.Passed)
	assert.Equal(t, 1, s.Summary.Failed) // timeout counts as failed in summary
	assert.Equal(t, "timeout", s.Cases[0].Status)
}

func TestExecuteCases_PersistsValidJSON(t *testing.T) {
	h := &AttemptTestHandler{Piston: &stubPiston{
		respond: func(_, _, stdin string) (*sandbox.PistonExecuteResponse, error) {
			return makeStubResp(stdin), nil
		},
	}}
	s := h.executeCases(context.Background(), "python", "code",
		[]store.TestCase{caseFor("c1", "hi", ptrStr("hi"), true)})
	blob, err := json.Marshal(s)
	assert.NoError(t, err)
	assert.Contains(t, string(blob), `"passed":1`)
	assert.Contains(t, string(blob), `"total":1`)
}
